package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/junaediakbar/quizzz-backend/internal/present"
	"github.com/junaediakbar/quizzz-backend/internal/repositories"
)

type ResultHandler struct {
	db           *sql.DB
	resultRepo   *repositories.ResultRepository
	examRepo     *repositories.ExamRepository
	questionRepo *repositories.QuestionRepository
}

func NewResultHandler(db *sql.DB) *ResultHandler {
	return &ResultHandler{
		db:           db,
		resultRepo:   repositories.NewResultRepository(db),
		examRepo:     repositories.NewExamRepository(db),
		questionRepo: repositories.NewQuestionRepository(db),
	}
}

// ListResults GET /results
func (h *ResultHandler) ListResults(c *fiber.Ctx) error {
	role := c.Locals("user_role").(string)
	userID := c.Locals("user_id").(string)

	f := repositories.ResultListFilters{}
	switch role {
	case "student":
		f.StudentID = userID
	case "teacher":
		f.TeacherID = userID
	}
	if examID := c.Query("exam_id"); examID != "" {
		f.ExamID = examID
	}

	list, err := h.resultRepo.ListWithExamTitle(f)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list results"})
	}

	items := make([]interface{}, 0, len(list))
	for _, row := range list {
		items = append(items, present.EnrichResultListItem(&row.Result))
	}
	return c.JSON(fiber.Map{"results": items, "count": len(items)})
}

// GetResult GET /results/:id
func (h *ResultHandler) GetResult(c *fiber.Ctx) error {
	id := c.Params("id")
	role := c.Locals("user_role").(string)
	userID := c.Locals("user_id").(string)

	res, err := h.resultRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrResultNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Result not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load result"})
	}

	if role == "student" && res.StudentID != userID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}
	if role == "teacher" {
		ex, err := h.examRepo.FindByID(res.ExamID)
		if err != nil || ex.CreatedBy != userID {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
		}
	}

	examMeta, err := h.examRepo.FindByID(res.ExamID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Exam not found"})
	}

	reviews, _ := h.resultRepo.ListAnswerReviews(res.SessionID)
	questions := present.LoadQuestionsForReviews(h.questionRepo, reviews)

	return c.JSON(present.BuildResultDetail(examMeta, res, reviews, questions, role))
}

type gradeBody struct {
	Feedback map[string]string `json:"feedback"` // question_id -> feedback
	Points   map[string]int    `json:"points"`   // question_id -> points override
}

// GradeResult PUT /results/:id/grade — teacher adjusts manual questions and totals.
func (h *ResultHandler) GradeResult(c *fiber.Ctx) error {
	if c.Locals("user_role") != "teacher" && c.Locals("user_role") != "admin" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}
	teacherID := c.Locals("user_id").(string)
	id := c.Params("id")

	res, err := h.resultRepo.FindByID(id)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Result not found"})
	}

	examMeta, err := h.examRepo.FindByID(res.ExamID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Exam not found"})
	}
	if c.Locals("user_role") == "teacher" && examMeta.CreatedBy != teacherID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	var body gradeBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}

	tx, err := h.db.Begin()
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Transaction failed"})
	}
	defer tx.Rollback()

	reviews, err := h.resultRepo.ListAnswerReviews(res.SessionID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load reviews"})
	}

	total := 0
	for _, rev := range reviews {
		pid := rev.QuestionID
		pts := rev.Points
		if body.Points != nil {
			if p, ok := body.Points[pid]; ok {
				pts = p
			}
		}
		var fb *string
		if body.Feedback != nil {
			if f, ok := body.Feedback[pid]; ok {
				s := f
				fb = &s
			}
		} else {
			fb = rev.Feedback
		}
		if err := h.resultRepo.UpdateFeedback(tx, res.SessionID, pid, fb, pts); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update review"})
		}
		total += pts
	}

	maxScore := res.MaxScore
	passed := false
	if maxScore > 0 {
		passed = (total*100/maxScore) >= examMeta.Config.PassingScore
	}

	if err := h.resultRepo.UpdateResultScore(tx, res.ID, total, passed); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update score"})
	}

	if err := tx.Commit(); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Commit failed"})
	}

	return c.JSON(fiber.Map{"message": "Graded", "score": total, "passed": passed})
}

// GetResultsByExam GET /results/exam/:examId
func (h *ResultHandler) GetResultsByExam(c *fiber.Ctx) error {
	examID := c.Params("examId")
	role := c.Locals("user_role").(string)
	userID := c.Locals("user_id").(string)

	ex, err := h.examRepo.FindByID(examID)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Exam not found"})
	}
	if role == "teacher" && ex.CreatedBy != userID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	list, err := h.resultRepo.ListWithExamTitle(repositories.ResultListFilters{ExamID: examID})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list"})
	}
	items := make([]interface{}, 0, len(list))
	for _, row := range list {
		items = append(items, present.EnrichResultListItem(&row.Result))
	}
	return c.JSON(fiber.Map{"results": items, "count": len(items)})
}

// GetResultsByStudent GET /results/student/:studentId
func (h *ResultHandler) GetResultsByStudent(c *fiber.Ctx) error {
	studentID := c.Params("studentId")
	role := c.Locals("user_role").(string)
	userID := c.Locals("user_id").(string)

	if role == "student" && studentID != userID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	f := repositories.ResultListFilters{StudentID: studentID}
	if role == "teacher" {
		f.TeacherID = userID
	}

	list, err := h.resultRepo.ListWithExamTitle(f)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list"})
	}
	items := make([]interface{}, 0, len(list))
	for _, row := range list {
		items = append(items, present.EnrichResultListItem(&row.Result))
	}
	return c.JSON(fiber.Map{"results": items, "count": len(items)})
}

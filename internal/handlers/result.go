package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

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

// ListResults GET /results?exam_id=&limit=&offset=
func (h *ResultHandler) ListResults(c *fiber.Ctx) error {
	page, err := ParsePageParams(c)
	if err != nil {
		return PageError(c, err)
	}

	role := c.Locals("user_role").(string)
	userID := c.Locals("user_id").(string)

	f := repositories.ResultListFilters{
		Limit:  page.Limit,
		Offset: page.Offset,
		Search: strings.TrimSpace(c.Query("search", "")),
	}
	switch role {
	case "student":
		f.StudentID = userID
	case "teacher":
		f.TeacherID = userID
	}
	if examID := c.Query("exam_id"); examID != "" {
		f.ExamID = examID
	}
	if raw := strings.TrimSpace(c.Query("passed")); raw != "" {
		passed := raw == "true" || raw == "1"
		if raw != "true" && raw != "false" && raw != "1" && raw != "0" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "passed harus true atau false"})
		}
		f.Passed = &passed
	}

	list, err := h.resultRepo.ListWithExamTitle(f)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list results"})
	}

	items := make([]interface{}, 0, len(list))
	for _, row := range list {
		items = append(items, present.EnrichResultListItem(&row.Result))
	}

	total, err := h.resultRepo.CountResults(f)
	if err != nil {
		total = page.Offset + len(items)
	}

	body := fiber.Map{"results": items}
	return c.JSON(NewListMeta(len(items), total, page).Apply(body))
}

// GetResult GET /results/:id
func (h *ResultHandler) GetResult(c *fiber.Ctx) error {
	id := c.Params("id")
	if rejectInvalidUUID(c, "id", id) {
		return nil
	}
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
	if rejectInvalidUUID(c, "id", id) {
		return nil
	}
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
		passed = (total * 100 / maxScore) >= examMeta.Config.PassingScore
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
	if rejectInvalidUUID(c, "examId", examID) {
		return nil
	}
	role := c.Locals("user_role").(string)
	userID := c.Locals("user_id").(string)

	ex, err := h.examRepo.FindByID(examID)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Exam not found"})
	}
	if role == "teacher" && ex.CreatedBy != userID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}
	page, err := ParsePageParams(c)
	if err != nil {
		return PageError(c, err)
	}

	// Murid tidak boleh melihat nilai teman sekelasnya — batasi ke hasilnya sendiri.
	filters := repositories.ResultListFilters{ExamID: examID, Limit: page.Limit, Offset: page.Offset}
	if role == "student" {
		filters.StudentID = userID
	}

	list, err := h.resultRepo.ListWithExamTitle(filters)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list"})
	}
	items := make([]interface{}, 0, len(list))
	for _, row := range list {
		items = append(items, present.EnrichResultListItem(&row.Result))
	}

	total, err := h.resultRepo.CountResults(filters)
	if err != nil {
		total = page.Offset + len(items)
	}

	body := fiber.Map{"results": items}
	return c.JSON(NewListMeta(len(items), total, page).Apply(body))
}

// GetResultsByStudent GET /results/student/:studentId
func (h *ResultHandler) GetResultsByStudent(c *fiber.Ctx) error {
	studentID := c.Params("studentId")
	if rejectInvalidUUID(c, "studentId", studentID) {
		return nil
	}
	role := c.Locals("user_role").(string)
	userID := c.Locals("user_id").(string)

	if role == "student" && studentID != userID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	page, err := ParsePageParams(c)
	if err != nil {
		return PageError(c, err)
	}

	f := repositories.ResultListFilters{
		StudentID: studentID,
		Limit:     page.Limit,
		Offset:    page.Offset,
	}
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

	total, err := h.resultRepo.CountResults(f)
	if err != nil {
		total = page.Offset + len(items)
	}

	body := fiber.Map{"results": items}
	return c.JSON(NewListMeta(len(items), total, page).Apply(body))
}

// DeleteResult DELETE /results/:id — teacher/admin removes a submission so the student can retake.
func (h *ResultHandler) DeleteResult(c *fiber.Ctx) error {
	role := c.Locals("user_role").(string)
	if role != "teacher" && role != "admin" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}
	userID := c.Locals("user_id").(string)
	id := c.Params("id")
	if rejectInvalidUUID(c, "id", id) {
		return nil
	}
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Result ID is required"})
	}

	res, err := h.resultRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrResultNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Result not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load result"})
	}

	if role == "teacher" {
		ex, err := h.examRepo.FindByID(res.ExamID)
		if err != nil || ex.CreatedBy != userID {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
		}
	}

	if err := h.resultRepo.DeleteByID(id); err != nil {
		if errors.Is(err, repositories.ErrResultNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Result not found"})
		}
		if errors.Is(err, repositories.ErrSessionNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Session not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete result"})
	}

	return c.Status(http.StatusNoContent).Send(nil)
}

package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/junaediakbar/quizzz-backend/internal/present"
	"github.com/junaediakbar/quizzz-backend/internal/repositories"
	"github.com/junaediakbar/quizzz-backend/models"

	"github.com/google/uuid"
	"github.com/gofiber/fiber/v2"
)

type ExamHandler struct {
	examRepo     *repositories.ExamRepository
	questionRepo *repositories.QuestionRepository
	resultRepo   *repositories.ResultRepository
}

func NewExamHandler(db *sql.DB) *ExamHandler {
	return &ExamHandler{
		examRepo:     repositories.NewExamRepository(db),
		questionRepo: repositories.NewQuestionRepository(db),
		resultRepo:   repositories.NewResultRepository(db),
	}
}

type CreateExamRequest struct {
	Title          string                 `json:"title"`
	Description    string                 `json:"description,omitempty"`
	Subject        string                 `json:"subject"`
	Grade          string                 `json:"grade"`
	Config         models.ExamConfig      `json:"config"`
	QuestionIDs    []string               `json:"question_ids,omitempty"`
	ScheduledStart *time.Time             `json:"scheduled_start,omitempty"`
	ScheduledEnd   *time.Time             `json:"scheduled_end,omitempty"`
}

type UpdateExamRequest struct {
	Title          *string            `json:"title,omitempty"`
	Description    *string            `json:"description,omitempty"`
	Subject        *string            `json:"subject,omitempty"`
	Grade          *string            `json:"grade,omitempty"`
	Config         *models.ExamConfig `json:"config,omitempty"`
	Status         *string            `json:"status,omitempty"`
	QuestionIDs    []string           `json:"question_ids,omitempty"`
	ScheduledStart *time.Time         `json:"scheduled_start,omitempty"`
	ScheduledEnd   *time.Time         `json:"scheduled_end,omitempty"`
}

// ListExams handles GET /exams
func (h *ExamHandler) ListExams(c *fiber.Ctx) error {
	// Get query parameters
	filters := repositories.ExamFilters{
		Status:  c.Query("status", ""),
		Subject: c.Query("subject", ""),
	}

	if limit := c.Query("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			filters.Limit = l
		}
	}

	userRole := c.Locals("user_role").(string)
	userID := c.Locals("user_id").(string)

	switch userRole {
	case "student":
		filters.PublishedOnly = true
		if cb := c.Query("created_by"); cb != "" {
			filters.CreatedBy = cb
		}
	case "admin":
		filters.CreatedBy = c.Query("created_by", "")
	default:
		filters.CreatedBy = userID
	}

	exams, err := h.examRepo.List(filters)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch exams",
		})
	}

	return c.JSON(fiber.Map{
		"exams": exams,
		"count": len(exams),
	})
}

// GetExam handles GET /exams/:id
func (h *ExamHandler) GetExam(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Exam ID is required",
		})
	}

	exam, err := h.examRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrExamNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{
				"error": "Exam not found",
			})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch exam",
		})
	}

	role := c.Locals("user_role").(string)
	userID := c.Locals("user_id").(string)

	if role == "student" {
		if exam.Status != "published" && exam.Status != "active" {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Exam not available"})
		}
		type studentExam struct {
			models.Exam
			Questions []interface{} `json:"questions"`
		}
		return c.JSON(studentExam{Exam: *exam, Questions: []interface{}{}})
	}

	if role == "teacher" && exam.CreatedBy != userID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	links, err := h.examRepo.GetExamQuestionLinks(id)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load questions"})
	}
	var qs []map[string]interface{}
	for _, link := range links {
		q, err := h.questionRepo.FindByID(link.QuestionID)
		if err != nil {
			continue
		}
		m := present.QuestionFullJSON(q)
		m["order"] = link.OrderIndex
		m["exam_points"] = link.Points
		qs = append(qs, m)
	}

	type teacherExam struct {
		models.Exam
		Questions []map[string]interface{} `json:"questions"`
	}
	return c.JSON(teacherExam{Exam: *exam, Questions: qs})
}

// CreateExam handles POST /exams
func (h *ExamHandler) CreateExam(c *fiber.Ctx) error {
	var req CreateExamRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if req.Title == "" || req.Subject == "" || req.Grade == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Title, subject, and grade are required",
		})
	}

	// Get user ID from context
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not authenticated",
		})
	}

	// Create exam
	exam := &models.Exam{
		ID:            uuid.New().String(),
		Title:         req.Title,
		Description:   &req.Description,
		Subject:       req.Subject,
		Grade:         req.Grade,
		Config:        req.Config,
		Status:        "draft",
		CreatedBy:     userID.(string),
		ScheduledStart: req.ScheduledStart,
		ScheduledEnd:   req.ScheduledEnd,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := h.examRepo.Create(exam); err != nil {
		log.Printf("CreateExam: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error":  "Failed to create exam",
			"detail": err.Error(),
		})
	}

	// Add questions to exam
	var linkErrors []string
	linked := 0
	for i, questionID := range req.QuestionIDs {
		question, qerr := h.questionRepo.FindByID(questionID)
		points := 5
		if qerr == nil {
			points = question.Points
		} else {
			log.Printf("CreateExam: question %s not found: %v", questionID, qerr)
		}

		if err := h.examRepo.AddQuestionToExam(exam.ID, questionID, i+1, points); err != nil {
			linkErrors = append(linkErrors, fmt.Sprintf("%s: %v", questionID, err))
			log.Printf("AddQuestionToExam: %v", err)
			continue
		}
		linked++
	}

	if len(req.QuestionIDs) > 0 && linked == 0 {
		if delErr := h.examRepo.Delete(exam.ID); delErr != nil {
			log.Printf("CreateExam rollback delete exam %s: %v", exam.ID, delErr)
		}
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error":  "Could not link any questions to the exam",
			"detail": strings.Join(linkErrors, "; "),
		})
	}

	return c.Status(http.StatusCreated).JSON(exam)
}

// UpdateExam handles PUT /exams/:id
func (h *ExamHandler) UpdateExam(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Exam ID is required",
		})
	}

	var req UpdateExamRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Get existing exam
	exam, err := h.examRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrExamNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Exam not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load exam"})
	}

	role := c.Locals("user_role").(string)
	userID := c.Locals("user_id").(string)
	if role == "teacher" && exam.CreatedBy != userID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	// Update fields
	if req.Title != nil {
		exam.Title = *req.Title
	}
	if req.Description != nil {
		exam.Description = req.Description
	}
	if req.Subject != nil {
		exam.Subject = *req.Subject
	}
	if req.Grade != nil {
		exam.Grade = *req.Grade
	}
	if req.Config != nil {
		exam.Config = *req.Config
	}
	if req.Status != nil {
		exam.Status = *req.Status
	}
	if req.ScheduledStart != nil {
		exam.ScheduledStart = req.ScheduledStart
	}
	if req.ScheduledEnd != nil {
		exam.ScheduledEnd = req.ScheduledEnd
	}
	exam.UpdatedAt = time.Now()

	if err := h.examRepo.Update(exam); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update exam",
		})
	}

	if req.QuestionIDs != nil {
		if len(req.QuestionIDs) == 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error": "At least one question is required",
			})
		}
		links := make([]repositories.ExamQuestionLink, 0, len(req.QuestionIDs))
		for i, questionID := range req.QuestionIDs {
			points := 5
			if question, qerr := h.questionRepo.FindByID(questionID); qerr == nil {
				points = question.Points
			}
			links = append(links, repositories.ExamQuestionLink{
				QuestionID: questionID,
				OrderIndex: i + 1,
				Points:     points,
			})
		}
		if err := h.examRepo.ReplaceExamQuestions(id, links); err != nil {
			log.Printf("ReplaceExamQuestions: %v", err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update exam questions",
			})
		}
	}

	links, err := h.examRepo.GetExamQuestionLinks(id)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load questions"})
	}
	var qs []map[string]interface{}
	for _, link := range links {
		q, err := h.questionRepo.FindByID(link.QuestionID)
		if err != nil {
			continue
		}
		m := present.QuestionFullJSON(q)
		m["order"] = link.OrderIndex
		m["exam_points"] = link.Points
		qs = append(qs, m)
	}

	type teacherExam struct {
		models.Exam
		Questions []map[string]interface{} `json:"questions"`
	}
	return c.JSON(teacherExam{Exam: *exam, Questions: qs})
}

// DeleteExam handles DELETE /exams/:id
func (h *ExamHandler) DeleteExam(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Exam ID is required",
		})
	}

	exam, err := h.examRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrExamNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Exam not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load exam"})
	}

	role := c.Locals("user_role").(string)
	userID := c.Locals("user_id").(string)
	if role == "teacher" && exam.CreatedBy != userID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	if err := h.examRepo.Delete(id); err != nil {
		if errors.Is(err, repositories.ErrExamNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Exam not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete exam",
		})
	}

	return c.Status(http.StatusNoContent).Send(nil)
}

// PublishExam handles POST /exams/:id/publish
func (h *ExamHandler) PublishExam(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Exam ID is required",
		})
	}

	exam, err := h.examRepo.FindByID(id)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Exam not found",
		})
	}

	links, err := h.examRepo.GetExamQuestionLinks(id)
	if err != nil {
		log.Printf("PublishExam GetExamQuestionLinks: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to verify exam questions",
			"detail": err.Error(),
		})
	}
	if len(links) == 0 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot publish an exam with no questions. Add at least one question first.",
		})
	}

	exam.Status = "published"
	exam.UpdatedAt = time.Now()

	if err := h.examRepo.Update(exam); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to publish exam",
		})
	}

	return c.JSON(exam)
}

// Analytics handles GET /exams/:id/analytics
func (h *ExamHandler) Analytics(c *fiber.Ctx) error {
	id := c.Params("id")
	userID := c.Locals("user_id").(string)
	role := c.Locals("user_role").(string)

	exam, err := h.examRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrExamNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Exam not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load exam"})
	}
	if role == "teacher" && exam.CreatedBy != userID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	results, err := h.resultRepo.List(repositories.ResultListFilters{ExamID: id})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load results"})
	}

	if len(results) == 0 {
		return c.JSON(fiber.Map{
			"exam_id":            id,
			"total_participants": 0,
			"average_score_pct":  0,
			"pass_rate_pct":      0,
			"highest_score_pct":  0,
			"lowest_score_pct":   0,
		})
	}

	sum := 0.0
	pass := 0
	high := 0.0
	low := 101.0
	for _, r := range results {
		pct := 0.0
		if r.MaxScore > 0 {
			pct = float64(r.Score) * 100 / float64(r.MaxScore)
		}
		sum += pct
		if r.Passed {
			pass++
		}
		if pct > high {
			high = pct
		}
		if pct < low {
			low = pct
		}
	}

	return c.JSON(fiber.Map{
		"exam_id":            id,
		"total_participants": len(results),
		"average_score_pct":  sum / float64(len(results)),
		"pass_rate_pct":      float64(pass) * 100 / float64(len(results)),
		"highest_score_pct":  high,
		"lowest_score_pct":   low,
	})
}

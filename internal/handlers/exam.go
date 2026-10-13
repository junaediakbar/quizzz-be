package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/junaediakbar/quizzz-backend/internal/present"
	"github.com/junaediakbar/quizzz-backend/internal/repositories"
	"github.com/junaediakbar/quizzz-backend/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
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

type ExamPartInput struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	QuestionIDs []string `json:"question_ids,omitempty"`
	// PointsPerQuestion > 0 overrides the bank points for every question in the part.
	PointsPerQuestion int `json:"points_per_question,omitempty"`
}

type CreateExamRequest struct {
	Title          string            `json:"title"`
	Description    string            `json:"description,omitempty"`
	Subject        string            `json:"subject"`
	Grade          string            `json:"grade"`
	Config         models.ExamConfig `json:"config"`
	QuestionIDs    []string          `json:"question_ids,omitempty"`
	Parts          []ExamPartInput   `json:"parts,omitempty"`
	ScheduledStart *time.Time        `json:"scheduled_start,omitempty"`
	ScheduledEnd   *time.Time        `json:"scheduled_end,omitempty"`
}

type UpdateExamRequest struct {
	Title          *string            `json:"title,omitempty"`
	Description    *string            `json:"description,omitempty"`
	Subject        *string            `json:"subject,omitempty"`
	Grade          *string            `json:"grade,omitempty"`
	Config         *models.ExamConfig `json:"config,omitempty"`
	Status         *string            `json:"status,omitempty"`
	QuestionIDs    []string           `json:"question_ids,omitempty"`
	Parts          []ExamPartInput    `json:"parts,omitempty"`
	ScheduledStart *time.Time         `json:"scheduled_start,omitempty"`
	ScheduledEnd   *time.Time         `json:"scheduled_end,omitempty"`
}

// loadExamParts attaches the exam sections to an exam (no-op when there are none).
func (h *ExamHandler) loadExamParts(exam *models.Exam) {
	parts, err := h.examRepo.GetExamParts(exam.ID)
	if err != nil {
		log.Printf("GetExamParts %s: %v", exam.ID, err)
		return
	}
	if len(parts) > 0 {
		exam.Parts = parts
	}
}

// teacherExamJSON serialises an exam together with its ordered questions.
func teacherExamJSON(exam *models.Exam, qs []map[string]interface{}) fiber.Map {
	out := fiber.Map{
		"id":              exam.ID,
		"title":           exam.Title,
		"description":     exam.Description,
		"subject":         exam.Subject,
		"grade":           exam.Grade,
		"config":          exam.Config,
		"status":          exam.Status,
		"created_by":      exam.CreatedBy,
		"scheduled_start": exam.ScheduledStart,
		"scheduled_end":   exam.ScheduledEnd,
		"created_at":      exam.CreatedAt,
		"updated_at":      exam.UpdatedAt,
		"questions":       qs,
	}
	if len(exam.Parts) > 0 {
		out["parts"] = exam.Parts
	}
	return out
}

// buildPartSpecs converts the request parts into repository specs. Question IDs
// are de-duplicated (the DB has UNIQUE(exam_id, question_id)) and empty parts are
// dropped so no blank section is created.
func (h *ExamHandler) buildPartSpecs(parts []ExamPartInput) []repositories.ExamPartSpec {
	specs := make([]repositories.ExamPartSpec, 0, len(parts))
	seen := map[string]bool{}
	order := 0
	for _, p := range parts {
		spec := repositories.ExamPartSpec{
			Title:       truncateRunes(p.Title, 255),
			Description: truncateRunes(p.Description, 2000),
		}
		for _, qid := range p.QuestionIDs {
			qid = strings.TrimSpace(qid)
			if qid == "" || seen[qid] {
				continue
			}
			seen[qid] = true
			order++
			points := p.PointsPerQuestion
			if points <= 0 {
				points = 5
				if q, err := h.questionRepo.FindByID(qid); err == nil && q.Points > 0 {
					points = q.Points
				}
			}
			spec.Links = append(spec.Links, repositories.ExamQuestionLink{
				QuestionID: qid,
				OrderIndex: order,
				Points:     points,
			})
		}
		if len(spec.Links) > 0 {
			specs = append(specs, spec)
		}
	}
	return specs
}

// unknownQuestionIDs returns ids that do not exist (or are not UUIDs).
func (h *ExamHandler) unknownQuestionIDs(ids []string) []string {
	bad := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if !validUUID(id) {
			bad = append(bad, id)
			continue
		}
		if _, err := h.questionRepo.FindByID(id); err != nil {
			bad = append(bad, id)
		}
	}
	return bad
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return string(r)
	}
	return strings.TrimSpace(string(r[:max]))
}

// ListExams handles GET /exams
//
// Query: status, subject, created_by, search, limit, offset.
func (h *ExamHandler) ListExams(c *fiber.Ctx) error {
	page, err := ParsePageParams(c)
	if err != nil {
		return PageError(c, err)
	}

	filters := repositories.ExamFilters{
		Status:  c.Query("status", ""),
		Subject: c.Query("subject", ""),
		Search:  c.Query("search", ""),
		Limit:   page.Limit,
		Offset:  page.Offset,
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

	total, err := h.examRepo.Count(filters)
	if err != nil {
		total = page.Offset + len(exams)
	}

	body := fiber.Map{"exams": exams}
	return c.JSON(NewListMeta(len(exams), total, page).Apply(body))
}

// GetExam handles GET /exams/:id
func (h *ExamHandler) GetExam(c *fiber.Ctx) error {
	id := c.Params("id")
	if rejectInvalidUUID(c, "id", id) {
		return nil
	}
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
		// Student preflight needs the question count and the section titles
		// (the questions themselves are only handed out when the session starts).
		if links, err := h.examRepo.GetExamQuestionLinks(id); err == nil {
			exam.QuestionCount = len(links)
		}
		h.loadExamParts(exam)
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
		if link.PartID != "" {
			m["part_id"] = link.PartID
		}
		qs = append(qs, m)
	}

	h.loadExamParts(exam)
	return c.JSON(teacherExamJSON(exam, qs))
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
		ID:             uuid.New().String(),
		Title:          req.Title,
		Description:    &req.Description,
		Subject:        req.Subject,
		Grade:          req.Grade,
		Config:         req.Config,
		Status:         "draft",
		CreatedBy:      userID.(string),
		ScheduledStart: req.ScheduledStart,
		ScheduledEnd:   req.ScheduledEnd,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := h.examRepo.Create(exam); err != nil {
		log.Printf("CreateExam: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error":  "Failed to create exam",
			"detail": err.Error(),
		})
	}

	// Add questions to exam — either grouped into parts, or as a flat list.
	partSpecs := h.buildPartSpecs(req.Parts)
	var linkErrors []string
	linked := 0

	if len(partSpecs) > 0 {
		if err := h.examRepo.ReplaceExamContent(exam.ID, partSpecs, nil); err != nil {
			log.Printf("CreateExam parts: %v", err)
			if delErr := h.examRepo.Delete(exam.ID); delErr != nil {
				log.Printf("CreateExam rollback delete exam %s: %v", exam.ID, delErr)
			}
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error":  "Could not add the questions to the exam",
				"detail": err.Error(),
			})
		}
		for _, spec := range partSpecs {
			linked += len(spec.Links)
		}
	} else {
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
	}

	if (len(req.QuestionIDs) > 0 || len(partSpecs) > 0) && linked == 0 {
		if delErr := h.examRepo.Delete(exam.ID); delErr != nil {
			log.Printf("CreateExam rollback delete exam %s: %v", exam.ID, delErr)
		}
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error":  "Could not link any questions to the exam",
			"detail": strings.Join(linkErrors, "; "),
		})
	}

	h.loadExamParts(exam)
	return c.Status(http.StatusCreated).JSON(teacherExamJSON(exam, []map[string]interface{}{}))
}

// UpdateExam handles PUT /exams/:id
func (h *ExamHandler) UpdateExam(c *fiber.Ctx) error {
	id := c.Params("id")
	if rejectInvalidUUID(c, "id", id) {
		return nil
	}
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
		status := strings.ToLower(strings.TrimSpace(*req.Status))
		if !allowedExamStatus(status) {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error":  "Invalid status",
				"detail": "expected draft, published, active, completed or archived",
			})
		}
		// Publish lewat PUT harus melewati validasi yang sama dengan /publish:
		// ujian tanpa soal tidak boleh berstatus published.
		if status == "published" && exam.Status != "published" {
			links, lerr := h.examRepo.GetExamQuestionLinks(id)
			if lerr != nil {
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to verify exam questions"})
			}
			if len(links) == 0 {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{
					"error": "Cannot publish an exam with no questions. Add at least one question first.",
				})
			}
		}
		exam.Status = status
	}
	if req.ScheduledStart != nil {
		exam.ScheduledStart = req.ScheduledStart
	}
	if req.ScheduledEnd != nil {
		exam.ScheduledEnd = req.ScheduledEnd
	}
	exam.UpdatedAt = time.Now()

	// Validasi id soal lebih dulu: id yang tidak ada akan melanggar FK dan
	// berubah menjadi 500, padahal ini kesalahan input (400).
	var idsToCheck []string
	if len(req.Parts) > 0 {
		for _, p := range req.Parts {
			idsToCheck = append(idsToCheck, p.QuestionIDs...)
		}
	} else {
		idsToCheck = req.QuestionIDs
	}
	if bad := h.unknownQuestionIDs(idsToCheck); len(bad) > 0 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error":  "Unknown or invalid question ids",
			"detail": strings.Join(bad, ", "),
		})
	}

	if err := h.examRepo.Update(exam); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update exam",
		})
	}

	partSpecs := h.buildPartSpecs(req.Parts)
	if len(req.Parts) > 0 {
		if len(partSpecs) == 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error": "At least one question is required in the exam parts",
			})
		}
		if err := h.examRepo.ReplaceExamContent(id, partSpecs, nil); err != nil {
			log.Printf("ReplaceExamContent: %v", err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update exam questions",
			})
		}
	} else if req.QuestionIDs != nil {
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
		if link.PartID != "" {
			m["part_id"] = link.PartID
		}
		qs = append(qs, m)
	}

	h.loadExamParts(exam)
	return c.JSON(teacherExamJSON(exam, qs))
}

// DeleteExam handles DELETE /exams/:id
func (h *ExamHandler) DeleteExam(c *fiber.Ctx) error {
	id := c.Params("id")
	if rejectInvalidUUID(c, "id", id) {
		return nil
	}
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
	if rejectInvalidUUID(c, "id", id) {
		return nil
	}
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
			"error":  "Failed to verify exam questions",
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
	if rejectInvalidUUID(c, "id", id) {
		return nil
	}
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

// allowedExamStatus mirrors the DB CHECK constraint on exams.status so an
// invalid value returns 400 instead of a Postgres 500.
func allowedExamStatus(s string) bool {
	switch s {
	case "draft", "published", "active", "completed", "archived":
		return true
	}
	return false
}

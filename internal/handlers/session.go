package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/junaediakbar/quizzz-backend/internal/present"
	"github.com/junaediakbar/quizzz-backend/internal/repositories"
	"github.com/junaediakbar/quizzz-backend/models"
)

type SessionHandler struct {
	db              *sql.DB
	examRepo        *repositories.ExamRepository
	questionRepo    *repositories.QuestionRepository
	sessionRepo     *repositories.SessionRepository
	resultRepo      *repositories.ResultRepository
	userRepo        *repositories.UserRepository
	proctoringRepo  *repositories.ProctoringRepository
}

func NewSessionHandler(db *sql.DB) *SessionHandler {
	return &SessionHandler{
		db:             db,
		examRepo:       repositories.NewExamRepository(db),
		questionRepo:   repositories.NewQuestionRepository(db),
		sessionRepo:    repositories.NewSessionRepository(db),
		resultRepo:     repositories.NewResultRepository(db),
		userRepo:       repositories.NewUserRepository(db),
		proctoringRepo: repositories.NewProctoringRepository(db),
	}
}

type startSessionBody struct {
	ExamID string `json:"exam_id"`
}

type submitAnswerBody struct {
	QuestionID string `json:"question_id"`
	Answer     string `json:"answer"`
}

// StartSession POST /sessions
func (h *SessionHandler) StartSession(c *fiber.Ctx) error {
	if c.Locals("user_role") != "student" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Only students can start an exam session"})
	}
	studentID := c.Locals("user_id").(string)

	var body startSessionBody
	if err := c.BodyParser(&body); err != nil || body.ExamID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "exam_id is required"})
	}

	exam, err := h.examRepo.FindByID(body.ExamID)
	if err != nil {
		if errors.Is(err, repositories.ErrExamNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Exam not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load exam"})
	}
	if exam.Status != "published" && exam.Status != "active" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Exam is not available"})
	}

	links, err := h.examRepo.GetExamQuestionLinks(body.ExamID)
	if err != nil {
		log.Printf("StartSession GetExamQuestionLinks: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error":  "Failed to load exam questions",
			"detail": err.Error(),
		})
	}
	if len(links) == 0 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Exam has no questions",
			"hint":  "This exam has no questions attached. A teacher must edit the exam and add questions from the question bank before students can start.",
		})
	}

	if exam.Config.ShuffleQuestions {
		rand.Shuffle(len(links), func(i, j int) {
			links[i], links[j] = links[j], links[i]
		})
	}

	existing, err := h.sessionRepo.FindByExamAndStudent(body.ExamID, studentID)
	if err != nil && !errors.Is(err, repositories.ErrSessionNotFound) {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load session"})
	}
	if err == nil && existing.Status == "submitted" {
		// 200 agar klien bisa mengarahkan ke hasil / dashboard, bukan error generik
		payload := fiber.Map{
			"already_completed": true,
			"session_id":        existing.ID,
			"exam_id":           exam.ID,
			"exam_title":        exam.Title,
		}
		if existing.Score != nil {
			payload["score"] = *existing.Score
		}
		if res, resErr := h.resultRepo.FindBySessionID(existing.ID); resErr == nil {
			payload["result_id"] = res.ID
		}
		return c.JSON(payload)
	}

	var session *models.ExamSession
	if err == nil && (existing.Status == "in-progress" || existing.Status == "not-started") {
		session = existing
		if session.Status == "not-started" {
			now := time.Now()
			session.Status = "in-progress"
			session.StartedAt = &now
			if err := h.sessionRepo.Update(session); err != nil {
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update session"})
			}
		}
	} else {
		now := time.Now()
		session = &models.ExamSession{
			ID:        uuid.New().String(),
			ExamID:    body.ExamID,
			StudentID: studentID,
			Answers:   "{}",
			Status:    "in-progress",
			StartedAt: &now,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := h.sessionRepo.Create(session); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create session"})
		}
	}

	questions, err := h.buildStudentQuestions(exam, links)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load questions"})
	}

	answersMap := map[string]string{}
	_ = json.Unmarshal([]byte(session.Answers), &answersMap)
	if answersMap == nil {
		answersMap = map[string]string{}
	}

	var startedAt time.Time
	if session.StartedAt != nil {
		startedAt = *session.StartedAt
	} else {
		startedAt = time.Now()
	}

	return c.JSON(fiber.Map{
		"session_id": session.ID,
		"exam_id":    exam.ID,
		"exam_title": exam.Title,
		"questions":  questions,
		"duration":   exam.Config.Duration,
		"started_at": startedAt,
		"answers":    answersMap,
	})
}

func (h *SessionHandler) buildStudentQuestions(exam *models.Exam, links []repositories.ExamQuestionLink) ([]map[string]interface{}, error) {
	questions := make([]map[string]interface{}, 0, len(links))

	for _, link := range links {
		q, err := h.questionRepo.FindByID(link.QuestionID)
		if err != nil {
			continue
		}
		// Get options as slice
		opts := present.OptionsSlice(q.Options)
		// Shuffle options if configured
		if exam.Config.ShuffleOptions && len(opts) > 1 {
			rand.Shuffle(len(opts), func(i, j int) { opts[i], opts[j] = opts[j], opts[i] })
		}
		// Build student question JSON with shuffled options
		studentQ := map[string]interface{}{
			"id":          q.ID,
			"type":        q.Type,
			"title":       q.Title,
			"content":     q.Content,
			"difficulty":  q.Difficulty,
			"points":      link.Points,
			"created_by":  q.CreatedBy,
			"created_at":  q.CreatedAt,
			"updated_at":  q.UpdatedAt,
		}
		// Add options as array (not string)
		if len(opts) > 0 {
			studentQ["options"] = opts
		}
		// Add image URLs if present
		imgs := present.ImageURLsSlice(q.ImageURLs)
		if len(imgs) > 0 {
			studentQ["image_urls"] = imgs
		}
		if q.CategoryID != nil {
			studentQ["category_id"] = *q.CategoryID
		}
		questions = append(questions, studentQ)
	}

	return questions, nil
}

// GetSession GET /sessions/:id
func (h *SessionHandler) GetSession(c *fiber.Ctx) error {
	id := c.Params("id")
	userID := c.Locals("user_id").(string)
	role := c.Locals("user_role").(string)

	s, err := h.sessionRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrSessionNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Session not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load session"})
	}

	if role == "student" && s.StudentID != userID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	var answers map[string]string
	_ = json.Unmarshal([]byte(s.Answers), &answers)
	if answers == nil {
		answers = map[string]string{}
	}

	return c.JSON(fiber.Map{
		"id":           s.ID,
		"exam_id":      s.ExamID,
		"student_id":   s.StudentID,
		"status":       s.Status,
		"answers":      answers,
		"started_at":   s.StartedAt,
		"submitted_at": s.SubmittedAt,
		"time_spent":   s.TimeSpent,
		"score":        s.Score,
	})
}

// SubmitAnswer PUT /sessions/:id/answer
func (h *SessionHandler) SubmitAnswer(c *fiber.Ctx) error {
	if c.Locals("user_role") != "student" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Only students can submit answers"})
	}
	studentID := c.Locals("user_id").(string)
	id := c.Params("id")

	var body submitAnswerBody
	if err := c.BodyParser(&body); err != nil || body.QuestionID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "question_id and answer are required"})
	}

	s, err := h.sessionRepo.FindByID(id)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Session not found"})
	}
	if s.StudentID != studentID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}
	if s.Status != "in-progress" && s.Status != "not-started" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Session is not active"})
	}

	var answers map[string]string
	_ = json.Unmarshal([]byte(s.Answers), &answers)
	if answers == nil {
		answers = map[string]string{}
	}
	answers[body.QuestionID] = body.Answer
	raw, _ := json.Marshal(answers)
	s.Answers = string(raw)
	s.UpdatedAt = time.Now()

	if err := h.sessionRepo.Update(s); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save answer"})
	}

	return c.JSON(fiber.Map{"message": "Answer saved"})
}

// SubmitExam PUT /sessions/:id/submit
func (h *SessionHandler) SubmitExam(c *fiber.Ctx) error {
	if c.Locals("user_role") != "student" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Only students can submit"})
	}
	studentID := c.Locals("user_id").(string)
	id := c.Params("id")

	s, err := h.sessionRepo.FindByID(id)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Session not found"})
	}
	if s.StudentID != studentID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}
	if s.Status == "submitted" || s.Status == "graded" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Already submitted"})
	}

	exam, err := h.examRepo.FindByID(s.ExamID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Exam not found"})
	}

	links, err := h.examRepo.GetExamQuestionLinks(s.ExamID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to grade exam"})
	}

	var answers map[string]string
	_ = json.Unmarshal([]byte(s.Answers), &answers)
	if answers == nil {
		answers = map[string]string{}
	}

	student, err := h.userRepo.FindByID(studentID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Student not found"})
	}

	tx, err := h.db.Begin()
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Transaction failed"})
	}
	defer tx.Rollback()

	now := time.Now()
	timeSpent := 0
	if s.StartedAt != nil {
		secs := int(now.Sub(*s.StartedAt).Seconds())
		if secs > 0 {
			timeSpent = secs
		}
	}

	totalScore := 0
	maxScore := 0
	for _, link := range links {
		maxScore += link.Points
	}

	for _, link := range links {
		q, err := h.questionRepo.FindByID(link.QuestionID)
		if err != nil {
			continue
		}
		stuAns := strings.TrimSpace(answers[link.QuestionID])
		pts := 0
		maxPts := link.Points
		correct := q.CorrectAnswer
		isCorrect := false

		switch q.Type {
		case "multiple-choice", "true-false", "short-answer", "fill-blank":
			if present.AnswersMatch(q.Type, stuAns, correct) {
				isCorrect = true
				pts = maxPts
				totalScore += pts
			}
		case "essay", "matching":
			// Manual grading — no auto points
			pts = 0
		default:
			if present.AnswersMatch(q.Type, stuAns, correct) {
				isCorrect = true
				pts = maxPts
				totalScore += pts
			}
		}

		if err := h.resultRepo.InsertAnswerReviewWithTx(tx, s.ID, link.QuestionID, stuAns, correct, isCorrect, pts, maxPts, nil); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save reviews"})
		}
	}

	passed := false
	if maxScore > 0 {
		pct := totalScore * 100 / maxScore
		passed = pct >= exam.Config.PassingScore
	}

	resultID := uuid.New().String()
	result := &models.ExamResult{
		ID:          resultID,
		SessionID:   s.ID,
		ExamID:      s.ExamID,
		StudentID:   studentID,
		StudentName: student.Name,
		ExamTitle:   exam.Title,
		ExamGrade:   exam.Grade,
		Score:       totalScore,
		MaxScore:    maxScore,
		Passed:      passed,
		TimeSpent:   timeSpent,
		SubmittedAt: now,
	}

	if err := h.resultRepo.CreateResultWithTx(tx, result); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save result"})
	}

	s.Status = "submitted"
	s.SubmittedAt = &now
	s.TimeSpent = timeSpent
	s.Score = &totalScore
	if err := h.sessionRepo.UpdateWithTx(tx, s); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to finalize session"})
	}

	if err := tx.Commit(); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Commit failed"})
	}

	return c.JSON(fiber.Map{"result_id": resultID})
}

// GetSessionResult GET /sessions/:id/result
func (h *SessionHandler) GetSessionResult(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	role := c.Locals("user_role").(string)
	id := c.Params("id")

	s, err := h.sessionRepo.FindByID(id)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Session not found"})
	}
	if role == "student" && s.StudentID != userID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	res, err := h.resultRepo.FindBySessionID(s.ID)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Result not ready"})
	}

	exam, err := h.examRepo.FindByID(res.ExamID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Exam not found"})
	}

	reviews, err := h.resultRepo.ListAnswerReviews(s.ID)
	if err != nil {
		reviews = nil
	}
	questions := present.LoadQuestionsForReviews(h.questionRepo, reviews)

	return c.JSON(present.BuildResultDetail(exam, res, reviews, questions, role))
}

type proctoringEventBody struct {
	EventType string          `json:"event_type"`
	Detail    json.RawMessage `json:"detail"`
}

// LogProctoringEvent POST /sessions/:id/proctoring-events
func (h *SessionHandler) LogProctoringEvent(c *fiber.Ctx) error {
	if c.Locals("user_role") != "student" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Only students can log events"})
	}
	studentID := c.Locals("user_id").(string)
	id := c.Params("id")
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Session id required"})
	}

	s, err := h.sessionRepo.FindByID(id)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Session not found"})
	}
	if s.StudentID != studentID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	var body proctoringEventBody
	if err := c.BodyParser(&body); err != nil || body.EventType == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "event_type required"})
	}

	if err := h.proctoringRepo.LogEvent(s.ID, body.EventType, body.Detail); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to log event"})
	}

	return c.Status(http.StatusCreated).JSON(fiber.Map{"ok": true})
}

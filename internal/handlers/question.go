package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
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

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const maxQuestionTitleRunes = 500

func truncateQuestionTitle(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxQuestionTitleRunes {
		return string(r)
	}
	return string(r[:maxQuestionTitleRunes])
}

// optionalValidCategoryID returns nil unless s is a valid UUID (avoids FK failures from AI typos like "biology").
func optionalValidCategoryID(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if _, err := uuid.Parse(s); err != nil {
		return nil
	}
	return &s
}

// formatDBError surfaces Postgres messages for import UX (wrapped fmt.Errorf chains included).
func formatDBError(err error) string {
	if err == nil {
		return ""
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		msg := strings.TrimSpace(pqErr.Message)
		if pqErr.Detail != "" {
			msg += " — " + strings.TrimSpace(pqErr.Detail)
		}
		return fmt.Sprintf("%s [%s]", msg, pqErr.Code)
	}
	return err.Error()
}

type QuestionHandler struct {
	questionRepo *repositories.QuestionRepository
}

func NewQuestionHandler(db *sql.DB) *QuestionHandler {
	return &QuestionHandler{
		questionRepo: repositories.NewQuestionRepository(db),
	}
}

type CreateQuestionRequest struct {
	Type          string   `json:"type"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Options       []string `json:"options,omitempty"`
	CorrectAnswer string   `json:"correct_answer"`
	Explanation   string   `json:"explanation,omitempty"`
	Difficulty    string   `json:"difficulty"`
	Points        int      `json:"points"`
	Tags          []string `json:"tags,omitempty"`
	CategoryID    string   `json:"category_id,omitempty"`
	// image_urls: string[] (legacy) atau [{url, position, option_index?}]
	ImageURLs json.RawMessage `json:"image_urls,omitempty"`
}

type UpdateQuestionRequest struct {
	Type          string   `json:"type"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Options       []string `json:"options,omitempty"`
	CorrectAnswer string   `json:"correct_answer"`
	Explanation   *string  `json:"explanation,omitempty"`
	Difficulty    string   `json:"difficulty"`
	Points        int      `json:"points"`
	Tags          []string `json:"tags,omitempty"`
	CategoryID    *string  `json:"category_id,omitempty"`
	ImageURLs *json.RawMessage `json:"image_urls,omitempty"` // nil = leave unchanged
}

// normalizeQuestionPayload fixes AI/import quirks so INSERT matches DB CHECK constraints.
func normalizeQuestionPayload(req *CreateQuestionRequest) {
	req.Type = normalizeQuestionType(req.Type)
	req.Difficulty = normalizeDifficulty(req.Difficulty)
	if req.Type == "multiple-choice" && len(req.Options) >= 2 && req.CorrectAnswer != "" {
		req.CorrectAnswer = resolveLetterOrShortMCQAnswer(req.CorrectAnswer, req.Options)
	}
}

func normalizeQuestionType(t string) string {
	s := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(t, "_", "-")))
	switch s {
	case "mcq", "multiple choice", "multiplechoice", "multi-choice":
		return "multiple-choice"
	case "tf", "true/false", "true_false", "boolean":
		return "true-false"
	case "shortanswer", "short answer":
		return "short-answer"
	case "fillblank", "fill blank", "fill-in-the-blank", "fill_in_the_blank":
		return "fill-blank"
	default:
		return s
	}
}

func isAllowedQuestionType(t string) bool {
	switch t {
	case "multiple-choice", "true-false", "short-answer", "essay", "matching", "fill-blank":
		return true
	default:
		return false
	}
}

func normalizeDifficulty(d string) string {
	s := strings.ToLower(strings.TrimSpace(d))
	switch s {
	case "easy", "medium", "hard":
		return s
	default:
		if s != "" {
			log.Printf("normalizeDifficulty: unknown %q, using medium", d)
		}
		return "medium"
	}
}

// resolveLetterOrShortMCQAnswer maps "b" / "B)" / "(b)" to the full option text when possible.
func resolveLetterOrShortMCQAnswer(answer string, options []string) string {
	a := strings.TrimSpace(answer)
	if len(options) == 0 {
		return answer
	}
	low := strings.ToLower(a)
	if len(low) == 1 && low[0] >= 'a' && low[0] <= 'z' {
		idx := int(low[0] - 'a')
		if idx >= 0 && idx < len(options) {
			return options[idx]
		}
	}
	// "b)" or "(b)"
	if len(low) >= 3 && (strings.HasPrefix(low, "(") && strings.HasSuffix(low, ")")) {
		if inner := strings.Trim(low, "()"); len(inner) == 1 && inner[0] >= 'a' && inner[0] <= 'z' {
			idx := int(inner[0] - 'a')
			if idx >= 0 && idx < len(options) {
				return options[idx]
			}
		}
	}
	if len(low) >= 2 && strings.HasSuffix(low, ")") && low[0] >= 'a' && low[0] <= 'z' {
		idx := int(low[0] - 'a')
		if idx >= 0 && idx < len(options) {
			return options[idx]
		}
	}
	return answer
}

// ListQuestions handles GET /questions
func (h *QuestionHandler) ListQuestions(c *fiber.Ctx) error {
	// Get query parameters
	filters := repositories.QuestionFilters{
		Type:       c.Query("type", ""),
		Difficulty: c.Query("difficulty", ""),
		CategoryID: c.Query("category_id", ""),
		CreatedBy:  c.Query("created_by", ""),
		Search:     c.Query("search", ""),
	}

	if limit := c.Query("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			filters.Limit = l
		}
	}

	if offset := c.Query("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil {
			filters.Offset = o
		}
	}

	questions, err := h.questionRepo.List(filters)
	if err != nil {
		log.Printf("ListQuestions: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch questions",
		})
	}

	// Convert to present format to ensure options are arrays
	questionsJSON := make([]map[string]interface{}, len(questions))
	for i, q := range questions {
		questionsJSON[i] = present.QuestionFullJSON(q)
	}

	return c.JSON(fiber.Map{
		"questions": questionsJSON,
		"count":     len(questionsJSON),
	})
}

// GetQuestion handles GET /questions/:id
func (h *QuestionHandler) GetQuestion(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Question ID is required",
		})
	}

	question, err := h.questionRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrQuestionNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{
				"error": "Question not found",
			})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch question",
		})
	}

	return c.JSON(present.QuestionFullJSON(question))
}

// CreateQuestion handles POST /questions
func (h *QuestionHandler) CreateQuestion(c *fiber.Ctx) error {
	var req CreateQuestionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}
	normalizeQuestionPayload(&req)

	// Validate required fields
	if req.Title == "" || req.Content == "" || req.Type == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Title, content, and type are required",
		})
	}
	if !isAllowedQuestionType(req.Type) {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid question type",
			"type":  req.Type,
		})
	}

	// Validate multiple-choice questions have options
	if req.Type == "multiple-choice" && len(req.Options) < 2 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Multiple choice questions must have at least 2 options",
		})
	}

	// Get user ID from context
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not authenticated",
		})
	}

	// Create question
	categoryID := optionalValidCategoryID(req.CategoryID)
	question := &models.Question{
		ID:            uuid.New().String(),
		Type:          req.Type,
		Title:         truncateQuestionTitle(req.Title),
		Content:       req.Content,
		CorrectAnswer: req.CorrectAnswer,
		Explanation:   &req.Explanation,
		Difficulty:    req.Difficulty,
		Points:        req.Points,
		CategoryID:    categoryID,
		CreatedBy:     userID.(string),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Handle options
	if len(req.Options) > 0 {
		optionsJSON, _ := json.Marshal(req.Options)
		opts := string(optionsJSON)
		question.Options = &opts
	}

	// Handle tags
	if len(req.Tags) > 0 {
		tagsJSON, _ := json.Marshal(req.Tags)
		tags := string(tagsJSON)
		question.Tags = &tags
	}

	if len(req.ImageURLs) > 0 {
		s, err := present.NormalizeImageURLsInput(req.ImageURLs)
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid image_urls"})
		}
		question.ImageURLs = s
	}

	if err := h.questionRepo.Create(question); err != nil {
		log.Printf("CreateQuestion: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error":  "Failed to create question",
			"detail": formatDBError(err),
		})
	}

	return c.Status(http.StatusCreated).JSON(present.QuestionFullJSON(question))
}

// UpdateQuestion handles PUT /questions/:id
func (h *QuestionHandler) UpdateQuestion(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Question ID is required",
		})
	}

	var req UpdateQuestionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate multiple-choice questions have options
	if req.Type == "multiple-choice" && len(req.Options) < 2 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Multiple choice questions must have at least 2 options",
		})
	}

	// Get existing question
	question, err := h.questionRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrQuestionNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{
				"error": "Question not found",
			})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to load question",
		})
	}

	// Update fields
	question.Type = req.Type
	question.Title = req.Title
	question.Content = req.Content
	question.CorrectAnswer = req.CorrectAnswer
	question.Explanation = req.Explanation
	question.Difficulty = req.Difficulty
	question.Points = req.Points
	question.CategoryID = req.CategoryID
	question.UpdatedAt = time.Now()

	// Handle options
	if len(req.Options) > 0 {
		optionsJSON, _ := json.Marshal(req.Options)
		opts := string(optionsJSON)
		question.Options = &opts
	}

	// Handle tags
	if len(req.Tags) > 0 {
		tagsJSON, _ := json.Marshal(req.Tags)
		tags := string(tagsJSON)
		question.Tags = &tags
	}

	if req.ImageURLs != nil {
		if len(*req.ImageURLs) == 0 || string(*req.ImageURLs) == "null" {
			question.ImageURLs = nil
		} else {
			s, err := present.NormalizeImageURLsInput(*req.ImageURLs)
			if err != nil {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid image_urls"})
			}
			question.ImageURLs = s
		}
	}

	if err := h.questionRepo.Update(question); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update question",
		})
	}

	return c.JSON(present.QuestionFullJSON(question))
}

// DeleteQuestion handles DELETE /questions/:id
func (h *QuestionHandler) DeleteQuestion(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Question ID is required",
		})
	}

	if err := h.questionRepo.Delete(id); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete question",
		})
	}

	return c.Status(http.StatusNoContent).Send(nil)
}

type importFailure struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

// ImportQuestions handles POST /questions/import
// Body: JSON array [...] or wrapper {"questions":[...]} (same shape as CreateQuestionRequest).
func (h *QuestionHandler) ImportQuestions(c *fiber.Ctx) error {
	raw := bytes.TrimSpace(c.Body())
	if len(raw) == 0 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Empty body",
		})
	}

	var questions []CreateQuestionRequest
	if raw[0] == '{' {
		var wrapped struct {
			Questions []CreateQuestionRequest `json:"questions"`
		}
		if err := json.Unmarshal(raw, &wrapped); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error": `Invalid JSON object: expected {"questions":[...]}`,
			})
		}
		questions = wrapped.Questions
	} else {
		if err := json.Unmarshal(raw, &questions); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error": `Invalid JSON array: expected [...] of CreateQuestionRequest`,
			})
		}
	}

	if len(questions) == 0 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "No questions to import",
		})
	}

	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not authenticated",
		})
	}

	created := []*models.Question{}
	var failed []importFailure

	for i := range questions {
		normalizeQuestionPayload(&questions[i])
		req := questions[i]
		if req.Title == "" || req.Content == "" || req.Type == "" {
			failed = append(failed, importFailure{Index: i, Error: "title, content, and type are required"})
			continue
		}
		if !isAllowedQuestionType(req.Type) {
			failed = append(failed, importFailure{Index: i, Error: fmt.Sprintf("unknown type %q (expected multiple-choice, true-false, …)", req.Type)})
			continue
		}
		// Validate multiple-choice questions have options
		if req.Type == "multiple-choice" && len(req.Options) < 2 {
			failed = append(failed, importFailure{Index: i, Error: "multiple choice questions must have at least 2 options"})
			continue
		}
		if req.Difficulty == "" {
			req.Difficulty = "medium"
		}
		if req.Points <= 0 {
			req.Points = 5
		}

		question := &models.Question{
			ID:            uuid.New().String(),
			Type:          req.Type,
			Title:         truncateQuestionTitle(req.Title),
			Content:       req.Content,
			CorrectAnswer: req.CorrectAnswer,
			Difficulty:    req.Difficulty,
			Points:        req.Points,
			CreatedBy:     userID.(string),
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		if len(req.Options) > 0 {
			optionsJSON, _ := json.Marshal(req.Options)
			opts := string(optionsJSON)
			question.Options = &opts
		}

		if len(req.Tags) > 0 {
			tagsJSON, _ := json.Marshal(req.Tags)
			tags := string(tagsJSON)
			question.Tags = &tags
		}

		if req.Explanation != "" {
			question.Explanation = &req.Explanation
		}
		if cid := optionalValidCategoryID(req.CategoryID); cid != nil {
			question.CategoryID = cid
		}

		if len(req.ImageURLs) > 0 {
			s, err := present.NormalizeImageURLsInput(req.ImageURLs)
			if err != nil {
				failed = append(failed, importFailure{Index: i, Error: "Invalid image_urls"})
				continue
			}
			question.ImageURLs = s
		}

		if err := h.questionRepo.Create(question); err != nil {
			log.Printf("ImportQuestions create row %d: %v", i, err)
			failed = append(failed, importFailure{Index: i, Error: formatDBError(err)})
			continue
		}
		created = append(created, question)
	}

	// Convert to present format to ensure options are arrays
	createdJSON := make([]map[string]interface{}, len(created))
	for i, q := range created {
		createdJSON[i] = present.QuestionFullJSON(q)
	}

	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"imported":  len(created),
		"questions": createdJSON,
		"failed":    failed,
	})
}

// ExportQuestions handles GET /questions/export
func (h *QuestionHandler) ExportQuestions(c *fiber.Ctx) error {
	filters := repositories.QuestionFilters{
		CreatedBy: c.Query("created_by", ""),
	}

	questions, err := h.questionRepo.List(filters)
	if err != nil {
		log.Printf("ExportQuestions: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch questions",
		})
	}

	out := make([]map[string]interface{}, len(questions))
	for i, q := range questions {
		out[i] = present.QuestionFullJSON(q)
	}

	// Set headers for JSON download
	c.Set("Content-Type", "application/json")
	c.Set("Content-Disposition", "attachment; filename=questions.json")

	return c.JSON(fiber.Map{
		"questions":   out,
		"count":       len(out),
		"exported_at": time.Now(),
	})
}

// GetQuestionsMissingOptions handles GET /questions/missing-options
// Returns multiple-choice questions that don't have options set
func (h *QuestionHandler) GetQuestionsMissingOptions(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	role, ok := c.Locals("user_role").(string)
	if !ok {
		role = "teacher"
	}

	var createdBy string
	if role != "admin" {
		createdBy = userID
	}

	questions, err := h.questionRepo.FindQuestionsMissingOptions(createdBy)
	if err != nil {
		log.Printf("GetQuestionsMissingOptions error: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch questions",
			"detail": err.Error(),
		})
	}

	// Convert to present format
	questionsJSON := make([]map[string]interface{}, len(questions))
	for i, q := range questions {
		questionsJSON[i] = present.QuestionFullJSON(q)
	}

	return c.JSON(fiber.Map{
		"questions": questionsJSON,
		"count":     len(questionsJSON),
	})
}

// BulkFixQuestionsOptions handles POST /questions/bulk-fix-options
// Allows fixing options for multiple questions at once
type FixQuestionOptions struct {
	QuestionID string   `json:"question_id"`
	Options    []string `json:"options"`
}

func (h *QuestionHandler) BulkFixQuestionsOptions(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	role, ok := c.Locals("user_role").(string)
	if !ok {
		role = "teacher"
	}

	var fixes []FixQuestionOptions
	if err := c.BodyParser(&fixes); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	updated := 0
	var failed []string

	for _, fix := range fixes {
		if fix.QuestionID == "" {
			failed = append(failed, "missing question_id")
			continue
		}

		// Find the question
		question, err := h.questionRepo.FindByID(fix.QuestionID)
		if err != nil {
			failed = append(failed, fix.QuestionID)
			continue
		}

		// Check ownership
		if role != "admin" && question.CreatedBy != userID {
			failed = append(failed, fix.QuestionID)
			continue
		}

		// Update options
		if len(fix.Options) > 0 {
			optionsJSON, _ := json.Marshal(fix.Options)
			opts := string(optionsJSON)
			question.Options = &opts
		}

		question.UpdatedAt = time.Now()

		if err := h.questionRepo.Update(question); err != nil {
			failed = append(failed, fix.QuestionID)
			continue
		}

		updated++
	}

	return c.JSON(fiber.Map{
		"updated": updated,
		"failed":  failed,
	})
}

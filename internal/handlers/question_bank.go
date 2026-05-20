package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/junaediakbar/quizzz-backend/internal/present"
	"github.com/junaediakbar/quizzz-backend/internal/repositories"
	"github.com/junaediakbar/quizzz-backend/models"
)

type QuestionBankHandler struct {
	bankRepo     *repositories.QuestionBankRepository
	questionRepo *repositories.QuestionRepository
}

func NewQuestionBankHandler(db *sql.DB) *QuestionBankHandler {
	return &QuestionBankHandler{
		bankRepo:     repositories.NewQuestionBankRepository(db),
		questionRepo: repositories.NewQuestionRepository(db),
	}
}

func (h *QuestionBankHandler) List(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	role, ok := c.Locals("user_role").(string)
	if !ok {
		role = "teacher" // default role
	}

	var createdBy string
	if role != "admin" {
		createdBy = userID
	}

	banks, err := h.bankRepo.List(createdBy)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list banks"})
	}
	return c.JSON(fiber.Map{"banks": banks, "count": len(banks)})
}

func (h *QuestionBankHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	b, err := h.bankRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrBankNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load"})
	}
	qids, err := h.bankRepo.ListQuestionIDs(id)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load questions"})
	}
	var questions []map[string]interface{}
	for _, qid := range qids {
		q, err := h.questionRepo.FindByID(qid)
		if err == nil {
			// Use present.QuestionFullJSON to ensure options are arrays, not JSON strings
			questions = append(questions, present.QuestionFullJSON(q))
		}
	}
	return c.JSON(fiber.Map{"bank": b, "questions": questions})
}

type createBankBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
}

func (h *QuestionBankHandler) Create(c *fiber.Ctx) error {
	var body createBankBody
	if err := c.BodyParser(&body); err != nil || body.Name == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}
	userID, ok := c.Locals("user_id").(string)
	if !ok {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	now := time.Now()
	var desc *string
	if body.Description != "" {
		desc = &body.Description
	}
	bank := &models.QuestionBank{
		ID:          uuid.New().String(),
		Name:        body.Name,
		Description: desc,
		CreatedBy:   userID,
		IsPublic:    body.IsPublic,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.bankRepo.Create(bank); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create"})
	}
	return c.Status(http.StatusCreated).JSON(bank)
}

func (h *QuestionBankHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var body createBankBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}
	b, err := h.bankRepo.FindByID(id)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Not found"})
	}
	if body.Name != "" {
		b.Name = body.Name
	}
	if body.Description != "" {
		d := body.Description
		b.Description = &d
	}
	b.IsPublic = body.IsPublic
	if err := h.bankRepo.Update(b); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update"})
	}
	return c.JSON(b)
}

func (h *QuestionBankHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	userID, ok := c.Locals("user_id").(string)
	if !ok {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	role, _ := c.Locals("user_role").(string)

	b, err := h.bankRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrBankNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load bank"})
	}
	if role != "admin" && b.CreatedBy != userID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "You can only delete your own question banks"})
	}

	if err := h.bankRepo.Delete(id); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete"})
	}
	return c.SendStatus(http.StatusNoContent)
}

type addQBody struct {
	QuestionID string `json:"question_id"`
	Order      int    `json:"order"`
}

func (h *QuestionBankHandler) AddQuestion(c *fiber.Ctx) error {
	bankID := c.Params("id")
	var body addQBody
	if err := c.BodyParser(&body); err != nil || body.QuestionID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "question_id required"})
	}
	if _, err := h.bankRepo.FindByID(bankID); err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Bank not found"})
	}
	if err := h.bankRepo.AddQuestion(bankID, body.QuestionID, body.Order); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to add"})
	}
	return c.JSON(fiber.Map{"message": "Added"})
}

func (h *QuestionBankHandler) RemoveQuestion(c *fiber.Ctx) error {
	bankID := c.Params("id")
	qid := c.Params("questionId")
	if err := h.bankRepo.RemoveQuestion(bankID, qid); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed"})
	}
	return c.SendStatus(http.StatusNoContent)
}

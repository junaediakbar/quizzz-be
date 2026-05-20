package handlers

import (
	"net/http"

	"github.com/junaediakbar/quizzz-backend/internal/services"
	"github.com/gofiber/fiber/v2"
)

type ParserHandler struct {
	// Can be extended with repositories if needed
}

func NewParserHandler() *ParserHandler {
	return &ParserHandler{}
}

type ParseRequest struct {
	Text string `json:"text"`
	// Data URLs: data:image/png;base64,... (kirim dari parser UI)
	Images []string `json:"images,omitempty"`
}

func (h *ParserHandler) ParseQuestions(c *fiber.Ctx) error {
	var req ParseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Text == "" && len(req.Images) == 0 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Provide text and/or images",
		})
	}

	// Gunakan selalu HTTP 200 + body ParseResult agar frontend bisa menampilkan errors[].errors (bukan gagal fetch).
	result, err := services.ParseQuestionsWithAI(req.Text, req.Images)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to parse questions",
			"details": err.Error(),
		})
	}

	return c.JSON(result)
}

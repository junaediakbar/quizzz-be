package handlers

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/junaediakbar/quizzz-backend/internal/services"
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
	// Mode: "auto" (default, AI lalu fallback ke parser aturan), "ai" (AI saja),
	// "manual" (hanya parser aturan sistem, tanpa memanggil AI).
	Mode string `json:"mode,omitempty"`
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

	switch strings.ToLower(strings.TrimSpace(req.Mode)) {
	case "", services.ParseModeAuto, services.ParseModeAI, services.ParseModeManual:
	default:
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "mode harus salah satu dari: auto, ai, manual",
		})
	}

	// Gunakan selalu HTTP 200 + body ParseResult agar frontend bisa menampilkan errors[].errors (bukan gagal fetch).
	result, err := services.ParseQuestionsWithAI(req.Text, req.Images, req.Mode)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Failed to parse questions",
			"details": err.Error(),
		})
	}

	return c.JSON(result)
}

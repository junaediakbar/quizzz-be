package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/junaediakbar/quizzz-backend/internal/repositories"
	"github.com/junaediakbar/quizzz-backend/internal/services"
	"github.com/junaediakbar/quizzz-backend/models"
)

// TeacherStudentHandler exposes student directory management for teachers only (not admin routes).
type TeacherStudentHandler struct {
	userRepo *repositories.UserRepository
}

func NewTeacherStudentHandler(db *sql.DB) *TeacherStudentHandler {
	return &TeacherStudentHandler{
		userRepo: repositories.NewUserRepository(db),
	}
}

// ListStudents GET /teacher/students?search=
func (h *TeacherStudentHandler) ListStudents(c *fiber.Ctx) error {
	search := strings.TrimSpace(c.Query("search", ""))
	users, err := h.userRepo.ListStudents(search)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list students"})
	}
	for _, u := range users {
		u.Password = ""
	}
	return c.JSON(fiber.Map{"students": users, "count": len(users)})
}

// GetStudent GET /teacher/students/:id
func (h *TeacherStudentHandler) GetStudent(c *fiber.Ctx) error {
	id := c.Params("id")
	u, err := h.userRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrUserNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Student not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load user"})
	}
	if u.Role != "student" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Student not found"})
	}
	u.Password = ""
	return c.JSON(u)
}

type teacherUpdateStudentBody struct {
	Name   string  `json:"name"`
	Email  string  `json:"email"`
	Avatar *string `json:"avatar"`
}

// UpdateStudent PUT /teacher/students/:id — name, email, avatar only; target must be student.
func (h *TeacherStudentHandler) UpdateStudent(c *fiber.Ctx) error {
	id := c.Params("id")
	var body teacherUpdateStudentBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}

	u, err := h.userRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrUserNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Student not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load user"})
	}
	if u.Role != "student" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Teachers can only edit student accounts"})
	}

	if strings.TrimSpace(body.Name) != "" {
		u.Name = strings.TrimSpace(body.Name)
	}
	if strings.TrimSpace(body.Email) != "" {
		u.Email = strings.TrimSpace(body.Email)
	}
	if body.Avatar != nil {
		u.Avatar = body.Avatar
	}

	if err := h.userRepo.Update(u); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update student"})
	}
	u.Password = ""
	return c.JSON(u)
}

type createStudentBody struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// CreateStudent POST /teacher/students — teachers can create student accounts.
func (h *TeacherStudentHandler) CreateStudent(c *fiber.Ctx) error {
	var body createStudentBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}

	name := strings.TrimSpace(body.Name)
	email := strings.TrimSpace(body.Email)

	if name == "" || email == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Name and email are required"})
	}

	// Check if email already exists
	_, err := h.userRepo.FindByEmail(email)
	if err == nil {
		return c.Status(http.StatusConflict).JSON(fiber.Map{"error": "Email already registered"})
	}

	// Hash password or generate default
	password := strings.TrimSpace(body.Password)
	if password == "" {
		// Generate default password: student123 (can be changed on first login)
		password = "student123"
	}

	hashedPassword, err := services.HashPassword(password)
	if err != nil {
		log.Printf("Failed to hash password: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to process password"})
	}

	user := &models.User{
		ID:        uuid.New().String(),
		Name:      name,
		Email:     email,
		Password:  hashedPassword,
		Role:      "student",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := h.userRepo.Create(user); err != nil {
		log.Printf("Failed to create student: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create student"})
	}

	user.Password = ""
	return c.Status(http.StatusCreated).JSON(user)
}

// DeleteStudent DELETE /teacher/students/:id — teachers can delete student accounts.
func (h *TeacherStudentHandler) DeleteStudent(c *fiber.Ctx) error {
	id := c.Params("id")

	u, err := h.userRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrUserNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Student not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load user"})
	}

	if u.Role != "student" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Teachers can only delete student accounts"})
	}

	if err := h.userRepo.Delete(id); err != nil {
		log.Printf("Failed to delete student: %v", err)
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete student"})
	}

	return c.SendStatus(http.StatusNoContent)
}

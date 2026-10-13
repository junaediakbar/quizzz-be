package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/junaediakbar/quizzz-backend/internal/repositories"
	"github.com/junaediakbar/quizzz-backend/internal/services"
	"github.com/junaediakbar/quizzz-backend/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type AuthHandler struct {
	userRepo *repositories.UserRepository
}

func NewAuthHandler(db *sql.DB) *AuthHandler {
	return &AuthHandler{
		userRepo: repositories.NewUserRepository(db),
	}
}

type RegisterRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"` // teacher, student, admin
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

type UpdateProfileRequest struct {
	Name   *string `json:"name"`
	Avatar *string `json:"avatar"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// Register handles user registration
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate input
	if req.Name == "" || req.Email == "" || req.Password == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Name, email, and password are required",
		})
	}

	// Validate role. Admin accounts must NOT be creatable from the public
	// registration form (privilege escalation) unless explicitly enabled.
	log.Printf("Register request - Role received: '%s'", req.Role)
	switch strings.ToLower(strings.TrimSpace(req.Role)) {
	case "teacher", "student":
		req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	case "admin":
		if !strings.EqualFold(os.Getenv("ALLOW_ADMIN_SELF_REGISTER"), "true") {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{
				"error": "Pendaftaran akun admin tidak diizinkan. Minta admin yang ada untuk membuatkan akun.",
			})
		}
		req.Role = "admin"
	default:
		log.Printf("Invalid role, defaulting to student")
		req.Role = "student"
	}

	// Check if user already exists
	_, err := h.userRepo.FindByEmail(req.Email)
	if err == nil {
		return c.Status(http.StatusConflict).JSON(fiber.Map{
			"error": "Email already registered",
		})
	}

	// Hash password
	hashedPassword, err := services.HashPassword(req.Password)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to process password",
		})
	}

	// Create user
	user := &models.User{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Email:     req.Email,
		Password:  hashedPassword,
		Role:      req.Role,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	log.Printf("Creating user with role: '%s'", user.Role)
	if err := h.userRepo.Create(user); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create user",
		})
	}
	log.Printf("User created with role: '%s'", user.Role)

	// Generate token
	token, err := services.GenerateToken(user.ID, user.Email, user.Role)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate token",
		})
	}

	// Clear password from response
	user.Password = ""

	return c.Status(http.StatusCreated).JSON(AuthResponse{
		Token: token,
		User:  user,
	})
}

// Login handles user login
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate input
	if req.Email == "" || req.Password == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Email and password are required",
		})
	}

	// Find user
	user, err := h.userRepo.FindByEmail(req.Email)
	if err != nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid credentials",
		})
	}

	// Check password
	if !services.CheckPassword(req.Password, user.Password) {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid credentials",
		})
	}

	// Generate token
	token, err := services.GenerateToken(user.ID, user.Email, user.Role)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to generate token",
		})
	}

	// Clear password from response
	user.Password = ""

	return c.JSON(AuthResponse{
		Token: token,
		User:  user,
	})
}

// GetCurrentUser returns the authenticated user's information
func (h *AuthHandler) GetCurrentUser(c *fiber.Ctx) error {
	// Get user from context (set by auth middleware)
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	user, err := h.userRepo.FindByID(userID.(string))
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Clear password from response
	user.Password = ""

	return c.JSON(user)
}

// UpdateCurrentUser updates the authenticated user's name and/or avatar
func (h *AuthHandler) UpdateCurrentUser(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	var req UpdateProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	user, err := h.userRepo.FindByID(userID.(string))
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error": "Name cannot be empty",
			})
		}
		user.Name = name
	}

	if req.Avatar != nil {
		avatar := strings.TrimSpace(*req.Avatar)
		if avatar == "" {
			user.Avatar = nil
		} else {
			user.Avatar = &avatar
		}
	}

	if err := h.userRepo.Update(user); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update profile",
		})
	}

	// Reload so the response carries the trigger-updated updated_at
	updated, err := h.userRepo.FindByID(user.ID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to load updated profile",
		})
	}

	// Clear password from response
	updated.Password = ""

	return c.JSON(updated)
}

// ChangePassword updates the authenticated user's password
func (h *AuthHandler) ChangePassword(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "Not authenticated",
		})
	}

	var req ChangePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Current password and new password are required",
		})
	}

	if len(req.NewPassword) < 6 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "New password must be at least 6 characters",
		})
	}

	user, err := h.userRepo.FindByID(userID.(string))
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// 400, not 401: the session is valid, only the submitted body is wrong.
	// A 401 here would make the frontend client wipe the token and force a re-login.
	if !services.CheckPassword(req.CurrentPassword, user.Password) {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Current password is incorrect",
		})
	}

	hashedPassword, err := services.HashPassword(req.NewPassword)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to process password",
		})
	}

	if err := h.userRepo.UpdatePassword(user.ID, hashedPassword); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update password",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Password updated successfully",
	})
}

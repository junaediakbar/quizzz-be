package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/junaediakbar/quizzz-backend/internal/repositories"
)

type UserAdminHandler struct {
	userRepo *repositories.UserRepository
}

func NewUserAdminHandler(db *sql.DB) *UserAdminHandler {
	return &UserAdminHandler{
		userRepo: repositories.NewUserRepository(db),
	}
}

func (h *UserAdminHandler) ListUsers(c *fiber.Ctx) error {
	role := c.Query("role", "")
	users, err := h.userRepo.List(role)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list users"})
	}
	for _, u := range users {
		u.Password = ""
	}
	return c.JSON(fiber.Map{"users": users, "count": len(users)})
}

func (h *UserAdminHandler) GetUser(c *fiber.Ctx) error {
	id := c.Params("id")
	u, err := h.userRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repositories.ErrUserNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load user"})
	}
	u.Password = ""
	return c.JSON(u)
}

type updateUserBody struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Avatar string `json:"avatar"`
}

func (h *UserAdminHandler) UpdateUser(c *fiber.Ctx) error {
	id := c.Params("id")
	var body updateUserBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid body"})
	}

	u, err := h.userRepo.FindByID(id)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}
	if body.Name != "" {
		u.Name = body.Name
	}
	if body.Email != "" {
		u.Email = body.Email
	}
	if body.Role != "" {
		if body.Role != "teacher" && body.Role != "student" && body.Role != "admin" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid role"})
		}
		u.Role = body.Role
	}
	if body.Avatar != "" {
		a := body.Avatar
		u.Avatar = &a
	}

	if err := h.userRepo.AdminUpdate(u); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update user"})
	}
	u.Password = ""
	return c.JSON(u)
}

func (h *UserAdminHandler) DeleteUser(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.userRepo.Delete(id); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete user"})
	}
	return c.SendStatus(http.StatusNoContent)
}

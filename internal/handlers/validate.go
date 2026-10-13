package handlers

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// validUUID reports whether s is a well-formed UUID. Route params and body
// fields that end up in SQL against UUID columns must be checked first,
// otherwise Postgres returns a type error and the client sees a 500.
func validUUID(s string) bool {
	_, err := uuid.Parse(strings.TrimSpace(s))
	return err == nil
}

// rejectInvalidUUID writes a 400 JSON response when the route param is not a
// UUID and reports whether the request should stop.
func rejectInvalidUUID(c *fiber.Ctx, name string, value string) bool {
	if validUUID(value) {
		return false
	}
	_ = c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
		"error": "Invalid " + name + ": harus berupa UUID",
	})
	return true
}

// isUniqueViolation reports whether err is a Postgres unique-constraint error
// (code 23505), so handlers can answer 409 instead of 500.
func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	return false
}

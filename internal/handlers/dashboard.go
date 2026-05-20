package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/junaediakbar/quizzz-backend/internal/present"
	"github.com/junaediakbar/quizzz-backend/internal/repositories"
	"github.com/junaediakbar/quizzz-backend/models"
)

type DashboardHandler struct {
	db           *sql.DB
	examRepo     *repositories.ExamRepository
	questionRepo *repositories.QuestionRepository
	resultRepo   *repositories.ResultRepository
	userRepo     *repositories.UserRepository
}

func NewDashboardHandler(db *sql.DB) *DashboardHandler {
	return &DashboardHandler{
		db:           db,
		examRepo:     repositories.NewExamRepository(db),
		questionRepo: repositories.NewQuestionRepository(db),
		resultRepo:   repositories.NewResultRepository(db),
		userRepo:     repositories.NewUserRepository(db),
	}
}

// TeacherDashboard GET /dashboard/teacher/:id
func (h *DashboardHandler) TeacherDashboard(c *fiber.Ctx) error {
	requestedID := c.Params("id")
	userID := c.Locals("user_id").(string)
	role := c.Locals("user_role").(string)

	if requestedID != userID && role != "admin" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	exams, err := h.examRepo.List(repositories.ExamFilters{CreatedBy: requestedID, Limit: 500})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load exams"})
	}

	active := 0
	for _, e := range exams {
		if e.Status == "published" || e.Status == "active" {
			active++
		}
	}

	nStudents, _ := h.userRepo.CountByRole("student")
	nQuestions, _ := h.questionRepo.CountByCreator(requestedID)

	var activity []models.Activity
	for i, e := range exams {
		if i >= 5 {
			break
		}
		activity = append(activity, models.Activity{
			ID:          e.ID,
			Type:        "exam-created",
			Title:       e.Title,
			Description: e.Subject,
			Timestamp:   e.UpdatedAt,
		})
	}

	return c.JSON(models.TeacherDashboardStats{
		TotalExams:     len(exams),
		ActiveExams:    active,
		TotalStudents:  nStudents,
		TotalQuestions: nQuestions,
		RecentActivity: activity,
	})
}

// StudentDashboard GET /dashboard/student/:id
func (h *DashboardHandler) StudentDashboard(c *fiber.Ctx) error {
	requestedID := c.Params("id")
	userID := c.Locals("user_id").(string)
	role := c.Locals("user_role").(string)

	if requestedID != userID && role != "admin" {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "Forbidden"})
	}

	upcoming, err := h.examRepo.List(repositories.ExamFilters{PublishedOnly: true, Limit: 20})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load exams"})
	}

	completedRows, err := h.resultRepo.ListWithExamTitle(repositories.ResultListFilters{StudentID: requestedID})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load results"})
	}

	completed := make([]interface{}, 0, len(completedRows))
	avg := 0.0
	if len(completedRows) > 0 {
		sum := 0.0
		for _, row := range completedRows {
			if row.Result.MaxScore > 0 {
				sum += float64(row.Result.Score) * 100 / float64(row.Result.MaxScore)
			}
			completed = append(completed, present.EnrichResultListItem(&row.Result))
		}
		avg = sum / float64(len(completedRows))
	}

	// models.StudentDashboardStats uses []Exam for upcoming — map minimal fields
	var upcomingExams []models.Exam
	for _, e := range upcoming {
		upcomingExams = append(upcomingExams, *e)
	}

	return c.JSON(fiber.Map{
		"upcoming_exams":     upcomingExams,
		"completed_exams":    completed,
		"average_score":      avg,
		"total_exams_taken":  len(completedRows),
	})
}

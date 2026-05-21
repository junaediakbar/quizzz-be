package main

import (
	_ "embed"
	"log"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"

	"github.com/junaediakbar/quizzz-backend/internal/database"
	"github.com/junaediakbar/quizzz-backend/internal/handlers"
	"github.com/junaediakbar/quizzz-backend/internal/middleware"
)

//go:embed docs/openapi.yaml
var openAPISpec []byte

const docsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Quizzz API — Swagger</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" crossorigin />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js" crossorigin></script>
  <script>
    window.onload = function() {
      SwaggerUIBundle({
        url: window.location.origin + "/api/docs/openapi.yaml",
        dom_id: "#swagger-ui",
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis],
        layout: "BaseLayout"
      });
    };
  </script>
</body>
</html>`

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Connect to database
	if err := database.Connect(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	bodyLimit := 12 * 1024 * 1024
	if v := os.Getenv("MAX_UPLOAD_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			bodyLimit = (n + 2) * 1024 * 1024
		}
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "QuizApp API",
		ServerHeader: "QuizApp",
		BodyLimit:    bodyLimit,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     getEnv("FRONTEND_URL", "http://localhost:3000"),
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowMethods:     "GET, POST, PUT, DELETE, PATCH",
		AllowCredentials: true,
	}))

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "healthy",
			"service": "quizapp-api",
		})
	})

	// OpenAPI + Swagger UI
	app.Get("/api/docs", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(docsHTML)
	})
	app.Get("/api/docs/openapi.yaml", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "application/yaml; charset=utf-8")
		return c.Send(openAPISpec)
	})

	// API Routes
	api := app.Group("/api/v1")

	db := database.GetDB()

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(db)
	questionHandler := handlers.NewQuestionHandler(db)
	examHandler := handlers.NewExamHandler(db)
	parserHandler := handlers.NewParserHandler()
	sessionHandler := handlers.NewSessionHandler(db)
	resultHandler := handlers.NewResultHandler(db)
	dashboardHandler := handlers.NewDashboardHandler(db)
	userAdminHandler := handlers.NewUserAdminHandler(db)
	teacherStudentHandler := handlers.NewTeacherStudentHandler(db)
	bankHandler := handlers.NewQuestionBankHandler(db)

	mediaHandler, err := handlers.NewMediaHandler()
	if err != nil {
		log.Fatalf("media handler: %v", err)
	}

	// Auth routes (public)
	auth := api.Group("/auth")
	auth.Post("/register", authHandler.Register)
	auth.Post("/login", authHandler.Login)

	// Protected auth routes
	authProtected := auth.Group("/")
	authProtected.Use(middleware.AuthMiddleware)
	authProtected.Get("/me", authHandler.GetCurrentUser)
	authProtected.Post("/logout", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Logged out successfully"})
	})

	// User routes (admin)
	users := api.Group("/users")
	users.Use(middleware.AuthMiddleware)
	users.Use(middleware.RequireRole("admin"))
	users.Get("/", userAdminHandler.ListUsers)
	users.Get("/:id", userAdminHandler.GetUser)
	users.Put("/:id", userAdminHandler.UpdateUser)
	users.Delete("/:id", userAdminHandler.DeleteUser)

	// Teacher — student directory (teachers only; not admin /users)
	teacherStudents := api.Group("/teacher/students")
	teacherStudents.Use(middleware.AuthMiddleware)
	teacherStudents.Use(middleware.RequireRole("teacher"))
	teacherStudents.Get("/", teacherStudentHandler.ListStudents)
	teacherStudents.Post("/", teacherStudentHandler.CreateStudent)
	teacherStudents.Get("/:id", teacherStudentHandler.GetStudent)
	teacherStudents.Put("/:id", teacherStudentHandler.UpdateStudent)
	teacherStudents.Delete("/:id", teacherStudentHandler.DeleteStudent)

	// Questions (teacher/admin)
	questions := api.Group("/questions")
	questions.Use(middleware.AuthMiddleware)
	questions.Use(middleware.RequireRole("teacher", "admin"))
	// Register static path segments before /:id — otherwise e.g. "export" and "missing-options" match :id.
	questions.Get("/", questionHandler.ListQuestions)
	questions.Get("/export", questionHandler.ExportQuestions)
	questions.Get("/missing-options", questionHandler.GetQuestionsMissingOptions)
	questions.Post("/import", questionHandler.ImportQuestions)
	questions.Post("/bulk-fix-options", questionHandler.BulkFixQuestionsOptions)
	questions.Get("/:id", questionHandler.GetQuestion)
	questions.Post("/", questionHandler.CreateQuestion)
	questions.Put("/:id", questionHandler.UpdateQuestion)
	questions.Delete("/:id", questionHandler.DeleteQuestion)

	// Media: Cloudinary image upload (teacher/admin)
	media := api.Group("/media")
	media.Use(middleware.AuthMiddleware)
	media.Use(middleware.RequireRole("teacher", "admin"))
	media.Post("/upload", mediaHandler.UploadImage)

	// Public media upload (no auth) for parser/integration use-cases
	api.Post("/public/media/upload", mediaHandler.UploadImagePublic)

	// Question banks (teacher/admin)
	banks := api.Group("/question-banks")
	banks.Use(middleware.AuthMiddleware)
	banks.Use(middleware.RequireRole("teacher", "admin"))
	banks.Get("/", bankHandler.List)
	banks.Post("/", bankHandler.Create)
	banks.Get("/:id", bankHandler.Get)
	banks.Put("/:id", bankHandler.Update)
	banks.Delete("/:id", bankHandler.Delete)
	banks.Post("/:id/questions", bankHandler.AddQuestion)
	banks.Delete("/:id/questions/:questionId", bankHandler.RemoveQuestion)

	// Exams: read for all authenticated; mutations for teacher/admin
	examsRead := api.Group("/exams")
	examsRead.Use(middleware.AuthMiddleware)
	examsRead.Get("/", examHandler.ListExams)
	examsRead.Get("/:id", examHandler.GetExam)

	examsWrite := api.Group("/exams")
	examsWrite.Use(middleware.AuthMiddleware)
	examsWrite.Use(middleware.RequireRole("teacher", "admin"))
	examsWrite.Post("/", examHandler.CreateExam)
	examsWrite.Put("/:id", examHandler.UpdateExam)
	examsWrite.Delete("/:id", examHandler.DeleteExam)
	examsWrite.Post("/:id/publish", examHandler.PublishExam)
	examsWrite.Get("/:id/analytics", examHandler.Analytics)

	// Sessions (authenticated; handler enforces student for start/submit)
	sessions := api.Group("/sessions")
	sessions.Use(middleware.AuthMiddleware)
	sessions.Post("/", sessionHandler.StartSession)
	sessions.Post("/:id/proctoring-events", sessionHandler.LogProctoringEvent)
	sessions.Get("/:id", sessionHandler.GetSession)
	sessions.Put("/:id/answer", sessionHandler.SubmitAnswer)
	sessions.Put("/:id/submit", sessionHandler.SubmitExam)
	sessions.Get("/:id/result", sessionHandler.GetSessionResult)

	// Results
	results := api.Group("/results")
	results.Use(middleware.AuthMiddleware)
	results.Get("/", resultHandler.ListResults)
	results.Get("/exam/:examId", resultHandler.GetResultsByExam)
	results.Get("/student/:studentId", resultHandler.GetResultsByStudent)
	results.Get("/:id", resultHandler.GetResult)
	results.Put("/:id/grade", resultHandler.GradeResult)
	results.Delete("/:id", resultHandler.DeleteResult)

	// AI Parser
	parser := api.Group("/parser")
	parser.Use(middleware.AuthMiddleware)
	parser.Use(middleware.RequireRole("teacher", "admin"))
	parser.Post("/parse", parserHandler.ParseQuestions)

	// Dashboard
	dashboard := api.Group("/dashboard")
	dashboard.Use(middleware.AuthMiddleware)
	dashboard.Get("/teacher/:id", dashboardHandler.TeacherDashboard)
	dashboard.Get("/student/:id", dashboardHandler.StudentDashboard)

	// Start server
	port := getEnv("PORT", "8080")
	log.Printf("Server starting on port %s", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatal(err)
	}
}

func getEnv(key, defaultVal string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultVal
	}
	return value
}

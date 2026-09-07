package models

import (
	"time"
)

// User represents a user in the system
type User struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Email     string    `json:"email" db:"email"`
	Password  string    `json:"-" db:"password"`
	Role      string    `json:"role" db:"role"` // teacher, student, admin
	Avatar    *string   `json:"avatar,omitempty" db:"avatar"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Question represents a quiz question
type Question struct {
	ID            string        `json:"id" db:"id"`
	Type          string        `json:"type" db:"type"` // multiple-choice, true-false, short-answer, essay, matching, fill-blank
	Title         string        `json:"title" db:"title"`
	Content       string        `json:"content" db:"content"`
	Options       *string       `json:"options,omitempty" db:"options"` // JSON array
	CorrectAnswer string        `json:"correct_answer" db:"correct_answer"`
	Explanation   *string       `json:"explanation,omitempty" db:"explanation"`
	Difficulty    string        `json:"difficulty" db:"difficulty"` // easy, medium, hard
	Points        int           `json:"points" db:"points"`
	Tags          *string       `json:"tags,omitempty" db:"tags"` // JSON array
	CategoryID    *string       `json:"category_id,omitempty" db:"category_id"`
	ImageURLs     *string       `json:"image_urls,omitempty" db:"image_urls"` // JSON array of URLs
	CreatedBy     string        `json:"created_by" db:"created_by"`
	CreatedAt     time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at" db:"updated_at"`
}

// QuestionBank represents a collection of questions
type QuestionBank struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description *string   `json:"description,omitempty" db:"description"`
	CreatedBy   string    `json:"created_by" db:"created_by"`
	IsPublic    bool      `json:"is_public" db:"is_public"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// QuestionBankQuestion represents a question in a bank
type QuestionBankQuestion struct {
	ID         string `json:"id" db:"id"`
	BankID     string `json:"bank_id" db:"bank_id"`
	QuestionID string `json:"question_id" db:"question_id"`
	Order      int    `json:"order" db:"order"`
}

// ExamConfig represents exam configuration
type ExamConfig struct {
	Duration         int    `json:"duration" db:"duration"` // in minutes
	ShuffleQuestions bool   `json:"shuffle_questions" db:"shuffle_questions"`
	ShuffleOptions   bool   `json:"shuffle_options" db:"shuffle_options"`
	ShowResults      string `json:"show_results" db:"show_results"` // immediate, after-review, manual
	AllowReview      bool   `json:"allow_review" db:"allow_review"`
	MaxAttempts      int    `json:"max_attempts" db:"max_attempts"`
	PassingScore     int    `json:"passing_score" db:"passing_score"`
	// Proctoring / integrity (per exam)
	SecurityEnabled   bool `json:"security_enabled" db:"security_enabled"`
	MaxViolations     int  `json:"max_violations" db:"max_violations"` // 0 = warn only, never auto-lock
	RequireFullscreen bool `json:"require_fullscreen" db:"require_fullscreen"`
	BlockCopyPaste    bool `json:"block_copy_paste" db:"block_copy_paste"`
	DetectFocusLoss   bool `json:"detect_focus_loss" db:"detect_focus_loss"`
}

// Exam represents an exam
type Exam struct {
	ID             string      `json:"id" db:"id"`
	Title          string      `json:"title" db:"title"`
	Description    *string     `json:"description,omitempty" db:"description"`
	Subject        string      `json:"subject" db:"subject"`
	Grade          string      `json:"grade" db:"grade"`
	Config         ExamConfig  `json:"config" db:"config"`
	Status         string      `json:"status" db:"status"` // draft, published, active, completed, archived
	CreatedBy      string      `json:"created_by" db:"created_by"`
	ScheduledStart *time.Time  `json:"scheduled_start,omitempty" db:"scheduled_start"`
	ScheduledEnd   *time.Time  `json:"scheduled_end,omitempty" db:"scheduled_end"`
	CreatedAt      time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at" db:"updated_at"`
	// QuestionCount is set on list responses (not stored in DB).
	QuestionCount  int         `json:"question_count,omitempty" db:"-"`
}

// ExamQuestion represents a question in an exam
type ExamQuestion struct {
	ID         string `json:"id" db:"id"`
	ExamID     string `json:"exam_id" db:"exam_id"`
	QuestionID string `json:"question_id" db:"question_id"`
	Order      int    `json:"order" db:"order"`
	Points     int    `json:"points" db:"points"`
}

// ExamSession represents a student's exam session
type ExamSession struct {
	ID         string     `json:"id" db:"id"`
	ExamID     string     `json:"exam_id" db:"exam_id"`
	StudentID  string     `json:"student_id" db:"student_id"`
	Answers    string     `json:"answers" db:"answers"` // JSON object
	AttemptNumber int     `json:"attempt_number" db:"attempt_number"`
	Status     string     `json:"status" db:"status"`   // not-started, in-progress, submitted, graded
	StartedAt  *time.Time `json:"started_at,omitempty" db:"started_at"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty" db:"submitted_at"`
	TimeSpent  int        `json:"time_spent" db:"time_spent"` // in seconds
	Score      *int       `json:"score,omitempty" db:"score"`
	GradedBy   *string    `json:"graded_by,omitempty" db:"graded_by"`
	GradedAt   *time.Time `json:"graded_at,omitempty" db:"graded_at"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
}

// AnswerReview represents a review of a student's answer
type AnswerReview struct {
	SessionID    string  `json:"session_id" db:"session_id"`
	QuestionID   string  `json:"question_id" db:"question_id"`
	StudentAnswer string  `json:"student_answer" db:"student_answer"`
	CorrectAnswer string  `json:"correct_answer" db:"correct_answer"`
	IsCorrect    bool    `json:"is_correct" db:"is_correct"`
	Points       int     `json:"points" db:"points"`
	MaxPoints    int     `json:"max_points" db:"max_points"`
	Feedback     *string `json:"feedback,omitempty" db:"feedback"`
}

// ExamResult represents a completed exam result
type ExamResult struct {
	ID          string    `json:"id" db:"id"`
	SessionID   string    `json:"session_id" db:"session_id"`
	ExamID      string    `json:"exam_id" db:"exam_id"`
	StudentID   string    `json:"student_id" db:"student_id"`
	StudentName string    `json:"student_name" db:"student_name"`
	ExamTitle   string    `json:"exam_title,omitempty" db:"exam_title"`
	ExamGrade   string    `json:"exam_grade,omitempty" db:"exam_grade"`
	Score       int       `json:"score" db:"score"`
	MaxScore    int       `json:"max_score" db:"max_score"`
	Passed      bool      `json:"passed" db:"passed"`
	TimeSpent   int       `json:"time_spent" db:"time_spent"`
	SubmittedAt time.Time `json:"submitted_at" db:"submitted_at"`
	GradedAt    *time.Time `json:"graded_at,omitempty" db:"graded_at"`
}

// Category represents a question category
type Category struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Slug        string    `json:"slug" db:"slug"`
	Description *string   `json:"description,omitempty" db:"description"`
	ParentID    *string   `json:"parent_id,omitempty" db:"parent_id"`
	Color       *string   `json:"color,omitempty" db:"color"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// Dashboard Stats
type TeacherDashboardStats struct {
	TotalExams    int       `json:"total_exams"`
	ActiveExams   int       `json:"active_exams"`
	TotalStudents int       `json:"total_students"`
	TotalQuestions int      `json:"total_questions"`
	RecentActivity []Activity `json:"recent_activity"`
}

type StudentDashboardStats struct {
	UpcomingExams    []Exam       `json:"upcoming_exams"`
	CompletedExams   []ExamResult `json:"completed_exams"`
	AverageScore     float64      `json:"average_score"`
	TotalExamsTaken  int          `json:"total_exams_taken"`
}

type Activity struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	UserID      *string   `json:"user_id,omitempty"`
	UserName    *string   `json:"user_name,omitempty"`
}

// QuestionAnalytics represents analytics for a question
type QuestionAnalytics struct {
	QuestionID      string  `json:"question_id"`
	QuestionTitle   string  `json:"question_title"`
	CorrectCount    int     `json:"correct_count"`
	IncorrectCount  int     `json:"incorrect_count"`
	SkipCount       int     `json:"skip_count"`
	CorrectRate     float64 `json:"correct_rate"`
	AverageTime     int     `json:"average_time"` // in seconds
	Discrimination  float64 `json:"disciscrimination"`
}

// AI Parser types
type ParseRequest struct {
	Text     string `json:"text"`
	Format   string `json:"format"` // auto, plain, markdown
}

type ParsedQuestion struct {
	Type          string   `json:"type"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Options       []string `json:"options,omitempty"`
	CorrectAnswer string   `json:"correct_answer"`
	Explanation   string   `json:"explanation,omitempty"`
	Difficulty    string   `json:"difficulty"`
	Points        int      `json:"points"`
	Tags          []string `json:"tags"`
}

type ParseResult struct {
	Success  bool             `json:"success"`
	Questions []ParsedQuestion `json:"questions"`
	Errors   []string         `json:"errors,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
}

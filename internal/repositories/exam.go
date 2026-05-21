package repositories

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/junaediakbar/quizzz-backend/models"
)

type ExamRepository struct {
	db *sql.DB
}

func NewExamRepository(db *sql.DB) *ExamRepository {
	return &ExamRepository{db: db}
}

// Create inserts a new exam
func (r *ExamRepository) Create(exam *models.Exam) error {
	configJSON, err := json.Marshal(exam.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	query := `
		INSERT INTO exams (id, title, description, subject, grade, config, status, created_by, scheduled_start, scheduled_end)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	// Pass JSON as string for JSONB — []byte is encoded as bytea and triggers "unsupported jsonb version number 91".
	cfgArg := interface{}(nil)
	if len(configJSON) > 0 {
		cfgArg = string(configJSON)
	}
	_, err = r.db.Exec(query,
		exam.ID, exam.Title, exam.Description, exam.Subject, exam.Grade,
		cfgArg, exam.Status, exam.CreatedBy, exam.ScheduledStart, exam.ScheduledEnd,
	)
	if err != nil {
		return fmt.Errorf("failed to create exam: %w", err)
	}
	return nil
}

// FindByID retrieves an exam by ID
func (r *ExamRepository) FindByID(id string) (*models.Exam, error) {
	exam := &models.Exam{}
	var configJSON []byte

	query := `
		SELECT id, title, description, subject, grade, config, status, created_by, scheduled_start, scheduled_end, created_at, updated_at
		FROM exams WHERE id = $1
	`
	err := r.db.QueryRow(query, id).Scan(
		&exam.ID, &exam.Title, &exam.Description, &exam.Subject, &exam.Grade,
		&configJSON, &exam.Status, &exam.CreatedBy, &exam.ScheduledStart, &exam.ScheduledEnd,
		&exam.CreatedAt, &exam.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrExamNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find exam: %w", err)
	}

	// Parse config JSON
	if err := json.Unmarshal(configJSON, &exam.Config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return exam, nil
}

// List retrieves exams with optional filters
func (r *ExamRepository) List(filters ExamFilters) ([]*models.Exam, error) {
	query := `
		SELECT e.id, e.title, e.description, e.subject, e.grade, e.config, e.status, e.created_by,
			e.scheduled_start, e.scheduled_end, e.created_at, e.updated_at,
			COALESCE(qc.cnt, 0)::int
		FROM exams e
		LEFT JOIN (
			SELECT exam_id, COUNT(*)::int AS cnt
			FROM exam_questions
			GROUP BY exam_id
		) qc ON qc.exam_id = e.id
		WHERE 1=1
	`
	args := []interface{}{}
	argCount := 1

	if filters.PublishedOnly {
		query += fmt.Sprintf(" AND e.status IN ($%d, $%d)", argCount, argCount+1)
		args = append(args, "published", "active")
		argCount += 2
	}

	if filters.Status != "" {
		query += fmt.Sprintf(" AND e.status = $%d", argCount)
		args = append(args, filters.Status)
		argCount++
	}

	if filters.CreatedBy != "" {
		query += fmt.Sprintf(" AND e.created_by = $%d", argCount)
		args = append(args, filters.CreatedBy)
		argCount++
	}

	if filters.Subject != "" {
		query += fmt.Sprintf(" AND e.subject = $%d", argCount)
		args = append(args, filters.Subject)
		argCount++
	}

	query += " ORDER BY e.created_at DESC"

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, filters.Limit)
		argCount++
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list exams: %w", err)
	}
	defer rows.Close()

	var exams []*models.Exam
	for rows.Next() {
		exam := &models.Exam{}
		var configJSON []byte

		err := rows.Scan(
			&exam.ID, &exam.Title, &exam.Description, &exam.Subject, &exam.Grade,
			&configJSON, &exam.Status, &exam.CreatedBy, &exam.ScheduledStart, &exam.ScheduledEnd,
			&exam.CreatedAt, &exam.UpdatedAt, &exam.QuestionCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan exam: %w", err)
		}

		// Parse config JSON
		if err := json.Unmarshal(configJSON, &exam.Config); err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}

		exams = append(exams, exam)
	}

	return exams, nil
}

// ExamQuestionLink is one row in exam_questions.
type ExamQuestionLink struct {
	QuestionID string
	OrderIndex int
	Points     int
}

// GetExamQuestionLinks returns ordered questions for an exam.
func (r *ExamRepository) GetExamQuestionLinks(examID string) ([]ExamQuestionLink, error) {
	q := `
		SELECT question_id, order_index, points
		FROM exam_questions
		WHERE exam_id = $1
		ORDER BY order_index ASC
	`
	rows, err := r.db.Query(q, examID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []ExamQuestionLink
	for rows.Next() {
		var l ExamQuestionLink
		if err := rows.Scan(&l.QuestionID, &l.OrderIndex, &l.Points); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, nil
}

// Update updates an exam
func (r *ExamRepository) Update(exam *models.Exam) error {
	configJSON, err := json.Marshal(exam.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	query := `
		UPDATE exams SET
			title = $2, description = $3, subject = $4, grade = $5,
			config = $6, status = $7, scheduled_start = $8, scheduled_end = $9
		WHERE id = $1
	`
	cfgArg := interface{}(nil)
	if len(configJSON) > 0 {
		cfgArg = string(configJSON)
	}
	_, err = r.db.Exec(query,
		exam.ID, exam.Title, exam.Description, exam.Subject, exam.Grade,
		cfgArg, exam.Status, exam.ScheduledStart, exam.ScheduledEnd,
	)
	if err != nil {
		return fmt.Errorf("failed to update exam: %w", err)
	}
	return nil
}

// Delete deletes an exam by ID
func (r *ExamRepository) Delete(id string) error {
	query := `DELETE FROM exams WHERE id = $1`
	res, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete exam: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrExamNotFound
	}
	return nil
}

// ReplaceExamQuestions replaces all questions linked to an exam (transactional).
func (r *ExamRepository) ReplaceExamQuestions(examID string, links []ExamQuestionLink) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM exam_questions WHERE exam_id = $1`, examID); err != nil {
		return fmt.Errorf("clear exam questions: %w", err)
	}

	for _, link := range links {
		q := `
			INSERT INTO exam_questions (id, exam_id, question_id, order_index, points)
			VALUES ($1, $2, $3, $4, $5)
		`
		if _, err := tx.Exec(q, generateUUID(), examID, link.QuestionID, link.OrderIndex, link.Points); err != nil {
			return fmt.Errorf("add question %s: %w", link.QuestionID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// AddQuestionToExam adds a question to an exam
func (r *ExamRepository) AddQuestionToExam(examID, questionID string, order, points int) error {
	query := `
		INSERT INTO exam_questions (id, exam_id, question_id, order_index, points)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (exam_id, question_id) DO UPDATE SET
			order_index = EXCLUDED.order_index, points = EXCLUDED.points
	`
	_, err := r.db.Exec(query, generateUUID(), examID, questionID, order, points)
	if err != nil {
		return fmt.Errorf("failed to add question to exam: %w", err)
	}
	return nil
}

// GetExamQuestions retrieves all questions for an exam
func (r *ExamRepository) GetExamQuestions(examID string) ([]string, error) {
	query := `
		SELECT question_id FROM exam_questions WHERE exam_id = $1 ORDER BY order_index
	`
	rows, err := r.db.Query(query, examID)
	if err != nil {
		return nil, fmt.Errorf("failed to get exam questions: %w", err)
	}
	defer rows.Close()

	var questionIDs []string
	for rows.Next() {
		var questionID string
		if err := rows.Scan(&questionID); err != nil {
			return nil, fmt.Errorf("failed to scan question ID: %w", err)
		}
		questionIDs = append(questionIDs, questionID)
	}

	return questionIDs, nil
}

// ExamFilters represents filters for listing exams
type ExamFilters struct {
	Status         string
	CreatedBy      string
	Subject        string
	Limit          int
	PublishedOnly  bool // student catalogue: published + active
}

func generateUUID() string {
	return uuid.New().String()
}

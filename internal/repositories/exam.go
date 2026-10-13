package repositories

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

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
	models.ApplyExamConfigDefaults(&exam.Config, configJSON)

	return exam, nil
}

// buildExamWhere menyusun klausa WHERE + argumen dari filter (dipakai List dan Count).
func buildExamWhere(filters ExamFilters) (string, []interface{}) {
	where := ""
	args := []interface{}{}
	argCount := 1

	if filters.PublishedOnly {
		where += fmt.Sprintf(" AND e.status IN ($%d, $%d)", argCount, argCount+1)
		args = append(args, "published", "active")
		argCount += 2
	}

	if filters.Status != "" {
		where += fmt.Sprintf(" AND e.status = $%d", argCount)
		args = append(args, filters.Status)
		argCount++
	}

	if filters.CreatedBy != "" {
		where += fmt.Sprintf(" AND e.created_by = $%d", argCount)
		args = append(args, filters.CreatedBy)
		argCount++
	}

	if filters.Subject != "" {
		where += fmt.Sprintf(" AND e.subject = $%d", argCount)
		args = append(args, filters.Subject)
		argCount++
	}

	if filters.Search != "" {
		where += fmt.Sprintf(" AND (e.title ILIKE $%d OR COALESCE(e.description,'') ILIKE $%d)", argCount, argCount+1)
		pattern := "%" + filters.Search + "%"
		args = append(args, pattern, pattern)
		argCount += 2
	}

	return where, args
}

// Count mengembalikan jumlah ujian yang cocok dengan filter.
func (r *ExamRepository) Count(filters ExamFilters) (int, error) {
	where, args := buildExamWhere(filters)
	var total int
	q := "SELECT COUNT(*) FROM exams e WHERE 1=1" + where
	if err := r.db.QueryRow(q, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count exams: %w", err)
	}
	return total, nil
}

// List retrieves exams with optional filters
func (r *ExamRepository) List(filters ExamFilters) ([]*models.Exam, error) {
	where, args := buildExamWhere(filters)
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
		WHERE 1=1` + where

	query += " ORDER BY e.created_at DESC, e.id DESC"

	argCount := len(args) + 1
	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, filters.Limit)
		argCount++
	}
	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, filters.Offset)
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
		models.ApplyExamConfigDefaults(&exam.Config, configJSON)

		exams = append(exams, exam)
	}

	return exams, nil
}

// ExamQuestionLink is one row in exam_questions.
type ExamQuestionLink struct {
	QuestionID string
	OrderIndex int
	Points     int
	// PartID is empty when the exam has no parts.
	PartID string
}

// ExamPartSpec describes one section of an exam and the questions inside it.
type ExamPartSpec struct {
	Title       string
	Description string
	Links       []ExamQuestionLink
}

// GetExamQuestionLinks returns ordered questions for an exam.
func (r *ExamRepository) GetExamQuestionLinks(examID string) ([]ExamQuestionLink, error) {
	q := `
		SELECT question_id, order_index, points, part_id
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
		var partID sql.NullString
		if err := rows.Scan(&l.QuestionID, &l.OrderIndex, &l.Points, &partID); err != nil {
			return nil, err
		}
		if partID.Valid {
			l.PartID = partID.String
		}
		links = append(links, l)
	}
	return links, nil
}

// GetExamParts returns the sections of an exam ordered by order_index.
func (r *ExamRepository) GetExamParts(examID string) ([]models.ExamPart, error) {
	q := `
		SELECT p.id, p.exam_id, p.title, p.description, p.order_index, p.created_at,
			COALESCE(qc.cnt, 0)::int
		FROM exam_parts p
		LEFT JOIN (
			SELECT part_id, COUNT(*)::int AS cnt
			FROM exam_questions
			GROUP BY part_id
		) qc ON qc.part_id = p.id
		WHERE p.exam_id = $1
		ORDER BY p.order_index ASC, p.created_at ASC
	`
	rows, err := r.db.Query(q, examID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	parts := []models.ExamPart{}
	for rows.Next() {
		var p models.ExamPart
		if err := rows.Scan(&p.ID, &p.ExamID, &p.Title, &p.Description, &p.OrderIndex, &p.CreatedAt, &p.QuestionCount); err != nil {
			return nil, err
		}
		parts = append(parts, p)
	}
	return parts, nil
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
// Existing parts are removed as well: the exam becomes part-less.
func (r *ExamRepository) ReplaceExamQuestions(examID string, links []ExamQuestionLink) error {
	return r.ReplaceExamContent(examID, nil, links)
}

// ReplaceExamContent replaces the parts and the question links of an exam in one
// transaction. When parts is empty the plain links are stored with part_id = NULL.
func (r *ExamRepository) ReplaceExamContent(examID string, parts []ExamPartSpec, links []ExamQuestionLink) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM exam_questions WHERE exam_id = $1`, examID); err != nil {
		return fmt.Errorf("clear exam questions: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM exam_parts WHERE exam_id = $1`, examID); err != nil {
		return fmt.Errorf("clear exam parts: %w", err)
	}

	if len(parts) == 0 {
		if err := insertExamQuestionLinks(tx, examID, links); err != nil {
			return err
		}
	} else {
		for i, part := range parts {
			partID := generateUUID()
			title := strings.TrimSpace(part.Title)
			if title == "" {
				title = fmt.Sprintf("Bagian %d", i+1)
			}
			var desc interface{}
			if s := strings.TrimSpace(part.Description); s != "" {
				desc = s
			}
			if _, err := tx.Exec(
				`INSERT INTO exam_parts (id, exam_id, title, description, order_index) VALUES ($1, $2, $3, $4, $5)`,
				partID, examID, title, desc, i+1,
			); err != nil {
				return fmt.Errorf("create part %d: %w", i+1, err)
			}
			for _, link := range part.Links {
				link.PartID = partID
				if err := insertExamQuestionLink(tx, examID, link); err != nil {
					return err
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func insertExamQuestionLinks(tx *sql.Tx, examID string, links []ExamQuestionLink) error {
	for _, link := range links {
		if err := insertExamQuestionLink(tx, examID, link); err != nil {
			return err
		}
	}
	return nil
}

func insertExamQuestionLink(tx *sql.Tx, examID string, link ExamQuestionLink) error {
	var partID interface{}
	if strings.TrimSpace(link.PartID) != "" {
		partID = link.PartID
	}
	q := `
		INSERT INTO exam_questions (id, exam_id, question_id, order_index, points, part_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	if _, err := tx.Exec(q, generateUUID(), examID, link.QuestionID, link.OrderIndex, link.Points, partID); err != nil {
		return fmt.Errorf("add question %s: %w", link.QuestionID, err)
	}
	return nil
}

// AddQuestionToExam adds a question to an exam
func (r *ExamRepository) AddQuestionToExam(examID, questionID string, order, points int) error {
	return r.AddQuestionToExamPart(examID, questionID, order, points, "")
}

// AddQuestionToExamPart adds a question to an exam, optionally inside a part.
func (r *ExamRepository) AddQuestionToExamPart(examID, questionID string, order, points int, partID string) error {
	var pid interface{}
	if strings.TrimSpace(partID) != "" {
		pid = partID
	}
	query := `
		INSERT INTO exam_questions (id, exam_id, question_id, order_index, points, part_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (exam_id, question_id) DO UPDATE SET
			order_index = EXCLUDED.order_index, points = EXCLUDED.points, part_id = EXCLUDED.part_id
	`
	_, err := r.db.Exec(query, generateUUID(), examID, questionID, order, points, pid)
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
	Status        string
	CreatedBy     string
	Subject       string
	Search        string
	Limit         int
	Offset        int
	PublishedOnly bool // student catalogue: published + active
}

func generateUUID() string {
	return uuid.New().String()
}

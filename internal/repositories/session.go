package repositories

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/junaediakbar/quizzz-backend/models"
)

type SessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(session *models.ExamSession) error {
	var err error
	answersStr := "{}"
	if session.Answers != "" {
		answersStr = session.Answers
	}

	query := `
		INSERT INTO exam_sessions (id, exam_id, student_id, answers, attempt_number, status, started_at, submitted_at, time_spent, score, graded_by, graded_at)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	attemptNumber := session.AttemptNumber
	if attemptNumber <= 0 {
		attemptNumber = 1
	}
	_, err = r.db.Exec(query,
		session.ID, session.ExamID, session.StudentID, answersStr, attemptNumber,
		session.Status, session.StartedAt, session.SubmittedAt, session.TimeSpent,
		session.Score, session.GradedBy, session.GradedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

func (r *SessionRepository) FindByID(id string) (*models.ExamSession, error) {
	query := `
		SELECT id, exam_id, student_id, COALESCE(answers::text, '{}'), status, started_at, submitted_at, time_spent, score, graded_by, graded_at, created_at, updated_at
		FROM exam_sessions WHERE id = $1
	`
	s := &models.ExamSession{}
	var answersStr string
	err := r.db.QueryRow(query, id).Scan(
		&s.ID, &s.ExamID, &s.StudentID, &answersStr, &s.Status,
		&s.StartedAt, &s.SubmittedAt, &s.TimeSpent, &s.Score, &s.GradedBy, &s.GradedAt,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find session: %w", err)
	}
	s.Answers = answersStr
	return s, nil
}

func (r *SessionRepository) FindByExamAndStudent(examID, studentID string) (*models.ExamSession, error) {
	query := `
		SELECT id, exam_id, student_id, COALESCE(answers::text, '{}'), attempt_number, status, started_at, submitted_at, time_spent, score, graded_by, graded_at, created_at, updated_at
		FROM exam_sessions WHERE exam_id = $1 AND student_id = $2
		ORDER BY attempt_number DESC LIMIT 1
	`
	s := &models.ExamSession{}
	var answersStr string
	err := r.db.QueryRow(query, examID, studentID).Scan(
		&s.ID, &s.ExamID, &s.StudentID, &answersStr, &s.AttemptNumber, &s.Status,
		&s.StartedAt, &s.SubmittedAt, &s.TimeSpent, &s.Score, &s.GradedBy, &s.GradedAt,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find session: %w", err)
	}
	s.Answers = answersStr
	return s, nil
}

func (r *SessionRepository) CountAttempts(examID, studentID string) (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM exam_sessions WHERE exam_id = $1 AND student_id = $2`, examID, studentID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count attempts: %w", err)
	}
	return count, nil
}

func (r *SessionRepository) Update(session *models.ExamSession) error {
	answersStr := "{}"
	if session.Answers != "" {
		answersStr = session.Answers
	}

	query := `
		UPDATE exam_sessions SET
			answers = $2::jsonb, status = $3, started_at = $4, submitted_at = $5,
			time_spent = $6, score = $7, graded_by = $8, graded_at = $9
		WHERE id = $1
	`
	_, err := r.db.Exec(query,
		session.ID, answersStr, session.Status,
		session.StartedAt, session.SubmittedAt, session.TimeSpent,
		session.Score, session.GradedBy, session.GradedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}
	return nil
}

func (r *SessionRepository) UpdateWithTx(tx *sql.Tx, session *models.ExamSession) error {
	answersStr := "{}"
	if session.Answers != "" && json.Valid([]byte(session.Answers)) {
		answersStr = session.Answers
	}

	query := `
		UPDATE exam_sessions SET
			answers = $2::jsonb, status = $3, started_at = $4, submitted_at = $5,
			time_spent = $6, score = $7, graded_by = $8, graded_at = $9
		WHERE id = $1
	`
	_, err := tx.Exec(query,
		session.ID, answersStr, session.Status,
		session.StartedAt, session.SubmittedAt, session.TimeSpent,
		session.Score, session.GradedBy, session.GradedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}
	return nil
}

// LiveSessionRow is a student session of one exam joined with the student's name.
type LiveSessionRow struct {
	SessionID     string
	StudentID     string
	StudentName   string
	AttemptNumber int
	Status        string
	StartedAt     *time.Time
	SubmittedAt   *time.Time
	UpdatedAt     time.Time
	Answers       string
}

// ListLiveByExam returns all sessions of an exam (latest attempt first per student
// is not enforced here; the handler decides what to show).
func (r *SessionRepository) ListLiveByExam(examID string) ([]LiveSessionRow, error) {
	query := `
		SELECT s.id, s.student_id, COALESCE(u.name, ''), s.attempt_number, s.status,
		       s.started_at, s.submitted_at, s.updated_at, COALESCE(s.answers::text, '{}')
		FROM exam_sessions s
		LEFT JOIN users u ON u.id = s.student_id
		WHERE s.exam_id = $1
		ORDER BY s.updated_at DESC
	`
	rows, err := r.db.Query(query, examID)
	if err != nil {
		return nil, fmt.Errorf("failed to list live sessions: %w", err)
	}
	defer rows.Close()

	out := []LiveSessionRow{}
	for rows.Next() {
		var row LiveSessionRow
		if err := rows.Scan(&row.SessionID, &row.StudentID, &row.StudentName, &row.AttemptNumber,
			&row.Status, &row.StartedAt, &row.SubmittedAt, &row.UpdatedAt, &row.Answers); err != nil {
			return nil, fmt.Errorf("failed to scan live session: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

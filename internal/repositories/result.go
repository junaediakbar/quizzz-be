package repositories

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/junaediakbar/quizzz-backend/models"
)

type ResultRepository struct {
	db *sql.DB
}

func NewResultRepository(db *sql.DB) *ResultRepository {
	return &ResultRepository{db: db}
}

func (r *ResultRepository) CreateResultWithTx(tx *sql.Tx, result *models.ExamResult) error {
	query := `
		INSERT INTO exam_results (id, session_id, exam_id, student_id, student_name, exam_title, exam_grade, score, max_score, passed, time_spent, submitted_at, graded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := tx.Exec(query,
		result.ID, result.SessionID, result.ExamID, result.StudentID, result.StudentName,
		result.ExamTitle, result.ExamGrade,
		result.Score, result.MaxScore, result.Passed, result.TimeSpent, result.SubmittedAt, result.GradedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create result: %w", err)
	}
	return nil
}

func (r *ResultRepository) InsertAnswerReviewWithTx(tx *sql.Tx, sessionID, questionID, studentAns, correctAns string, isCorrect bool, points, maxPts int, feedback *string) error {
	id := uuid.New().String()
	query := `
		INSERT INTO answer_reviews (id, session_id, question_id, student_answer, correct_answer, is_correct, points, max_points, feedback)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (session_id, question_id) DO UPDATE SET
			student_answer = EXCLUDED.student_answer,
			correct_answer = EXCLUDED.correct_answer,
			is_correct = EXCLUDED.is_correct,
			points = EXCLUDED.points,
			max_points = EXCLUDED.max_points,
			feedback = EXCLUDED.feedback
	`
	_, err := tx.Exec(query, id, sessionID, questionID, studentAns, correctAns, isCorrect, points, maxPts, feedback)
	if err != nil {
		return fmt.Errorf("failed to insert answer review: %w", err)
	}
	return nil
}

func (r *ResultRepository) FindByID(id string) (*models.ExamResult, error) {
	query := `
		SELECT er.id, er.session_id, er.exam_id, er.student_id, er.student_name,
			COALESCE(NULLIF(er.exam_title, ''), e.title),
			COALESCE(NULLIF(er.exam_grade, ''), e.grade),
			er.score, er.max_score, er.passed, er.time_spent, er.submitted_at, er.graded_at
		FROM exam_results er
		JOIN exams e ON e.id = er.exam_id
		WHERE er.id = $1
	`
	res := &models.ExamResult{}
	err := r.db.QueryRow(query, id).Scan(
		&res.ID, &res.SessionID, &res.ExamID, &res.StudentID, &res.StudentName,
		&res.ExamTitle, &res.ExamGrade,
		&res.Score, &res.MaxScore, &res.Passed, &res.TimeSpent, &res.SubmittedAt, &res.GradedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrResultNotFound
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (r *ResultRepository) FindBySessionID(sessionID string) (*models.ExamResult, error) {
	query := `
		SELECT er.id, er.session_id, er.exam_id, er.student_id, er.student_name,
			COALESCE(NULLIF(er.exam_title, ''), e.title),
			COALESCE(NULLIF(er.exam_grade, ''), e.grade),
			er.score, er.max_score, er.passed, er.time_spent, er.submitted_at, er.graded_at
		FROM exam_results er
		JOIN exams e ON e.id = er.exam_id
		WHERE er.session_id = $1
	`
	res := &models.ExamResult{}
	err := r.db.QueryRow(query, sessionID).Scan(
		&res.ID, &res.SessionID, &res.ExamID, &res.StudentID, &res.StudentName,
		&res.ExamTitle, &res.ExamGrade,
		&res.Score, &res.MaxScore, &res.Passed, &res.TimeSpent, &res.SubmittedAt, &res.GradedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrResultNotFound
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

type ResultListFilters struct {
	ExamID    string
	StudentID string
	TeacherID string // exams.created_by
}

// ExamResultRow wraps a result with resolved title/grade for list endpoints.
type ExamResultRow struct {
	Result models.ExamResult
}

func (r *ResultRepository) List(filters ResultListFilters) ([]*models.ExamResult, error) {
	rows, err := r.ListWithExamTitle(filters)
	if err != nil {
		return nil, err
	}
	out := make([]*models.ExamResult, len(rows))
	for i, row := range rows {
		res := row.Result
		out[i] = &res
	}
	return out, nil
}

func (r *ResultRepository) ListWithExamTitle(filters ResultListFilters) ([]ExamResultRow, error) {
	query := `
		SELECT er.id, er.session_id, er.exam_id, er.student_id, er.student_name,
			COALESCE(NULLIF(er.exam_title, ''), e.title),
			COALESCE(NULLIF(er.exam_grade, ''), e.grade),
			er.score, er.max_score, er.passed, er.time_spent, er.submitted_at, er.graded_at
		FROM exam_results er
		JOIN exams e ON e.id = er.exam_id
		WHERE 1=1
	`
	args := []interface{}{}
	argN := 1

	if filters.ExamID != "" {
		query += fmt.Sprintf(" AND er.exam_id = $%d", argN)
		args = append(args, filters.ExamID)
		argN++
	}
	if filters.StudentID != "" {
		query += fmt.Sprintf(" AND er.student_id = $%d", argN)
		args = append(args, filters.StudentID)
		argN++
	}
	if filters.TeacherID != "" {
		query += fmt.Sprintf(" AND e.created_by = $%d", argN)
		args = append(args, filters.TeacherID)
		argN++
	}

	query += " ORDER BY er.submitted_at DESC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ExamResultRow
	for rows.Next() {
		res := models.ExamResult{}
		if err := rows.Scan(
			&res.ID, &res.SessionID, &res.ExamID, &res.StudentID, &res.StudentName,
			&res.ExamTitle, &res.ExamGrade,
			&res.Score, &res.MaxScore, &res.Passed, &res.TimeSpent, &res.SubmittedAt, &res.GradedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, ExamResultRow{Result: res})
	}
	return out, nil
}

func (r *ResultRepository) ListAnswerReviews(sessionID string) ([]models.AnswerReview, error) {
	query := `
		SELECT session_id, question_id, COALESCE(student_answer,''), COALESCE(correct_answer,''),
			COALESCE(is_correct,false), points, max_points, feedback
		FROM answer_reviews WHERE session_id = $1
	`
	rows, err := r.db.Query(query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []models.AnswerReview
	for rows.Next() {
		var ar models.AnswerReview
		if err := rows.Scan(&ar.SessionID, &ar.QuestionID, &ar.StudentAnswer, &ar.CorrectAnswer,
			&ar.IsCorrect, &ar.Points, &ar.MaxPoints, &ar.Feedback); err != nil {
			return nil, err
		}
		reviews = append(reviews, ar)
	}
	return reviews, nil
}

func (r *ResultRepository) UpdateFeedback(tx *sql.Tx, sessionID, questionID string, feedback *string, points int) error {
	q := `UPDATE answer_reviews SET feedback = $3, points = $4 WHERE session_id = $1 AND question_id = $2`
	_, err := tx.Exec(q, sessionID, questionID, feedback, points)
	return err
}

func (r *ResultRepository) UpdateResultScore(tx *sql.Tx, resultID string, score int, passed bool) error {
	q := `UPDATE exam_results SET score = $2, passed = $3, graded_at = NOW() WHERE id = $1`
	_, err := tx.Exec(q, resultID, score, passed)
	return err
}

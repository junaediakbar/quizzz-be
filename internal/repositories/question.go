package repositories

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/junaediakbar/quizzz-backend/models"
)

type QuestionRepository struct {
	db *sql.DB
}

func NewQuestionRepository(db *sql.DB) *QuestionRepository {
	return &QuestionRepository{db: db}
}

// jsonbStringArg passes JSON text as Go string so lib/pq encodes it as text for Postgres.
// Passing []byte makes pq send bytea; Postgres then treats JSON as binary jsonb and errors:
// "unsupported jsonb version number 91" ('[' == 0x5b == 91).
func jsonbStringArg(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

// Create inserts a new question
func (r *QuestionRepository) Create(question *models.Question) error {
	// options/tags/image_urls: *string holds raw JSON text; pass as string for JSONB columns (not []byte).

	query := `
		INSERT INTO questions (id, type, title, content, options, correct_answer, explanation, difficulty, points, tags, category_id, image_urls, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := r.db.Exec(query,
		question.ID, question.Type, question.Title, question.Content,
		jsonbStringArg(question.Options), question.CorrectAnswer, question.Explanation,
		question.Difficulty, question.Points, jsonbStringArg(question.Tags),
		question.CategoryID, jsonbStringArg(question.ImageURLs), question.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to create question: %w", err)
	}
	return nil
}

// FindByID retrieves a question by ID
func (r *QuestionRepository) FindByID(id string) (*models.Question, error) {
	question := &models.Question{}
	var optionsJSON, tagsJSON []byte

	var imageJSON []byte

	query := `
		SELECT id, type, title, content, options, correct_answer, explanation, difficulty, points, tags, category_id, image_urls, created_by, created_at, updated_at
		FROM questions WHERE id = $1
	`
	var difficulty sql.NullString
	var points sql.NullInt64

	err := r.db.QueryRow(query, id).Scan(
		&question.ID, &question.Type, &question.Title, &question.Content,
		&optionsJSON, &question.CorrectAnswer, &question.Explanation,
		&difficulty, &points, &tagsJSON,
		&question.CategoryID, &imageJSON, &question.CreatedBy, &question.CreatedAt, &question.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrQuestionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find question: %w", err)
	}

	if difficulty.Valid && difficulty.String != "" {
		question.Difficulty = difficulty.String
	} else {
		question.Difficulty = "medium"
	}
	if points.Valid {
		question.Points = int(points.Int64)
	} else {
		question.Points = 5
	}

	// Parse JSON fields (options/tags stored as JSON arrays)
	if len(optionsJSON) > 0 {
		var optArr []string
		if err := json.Unmarshal(optionsJSON, &optArr); err == nil {
			raw, _ := json.Marshal(optArr)
			s := string(raw)
			question.Options = &s
		}
	}
	if len(tagsJSON) > 0 {
		var tagArr []string
		if err := json.Unmarshal(tagsJSON, &tagArr); err == nil {
			raw, _ := json.Marshal(tagArr)
			s := string(raw)
			question.Tags = &s
		}
	}
	if len(imageJSON) > 0 {
		s := string(imageJSON)
		question.ImageURLs = &s
	}

	return question, nil
}

// List retrieves questions with optional filters
func (r *QuestionRepository) List(filters QuestionFilters) ([]*models.Question, error) {
	query := `
		SELECT id, type, title, content, options, correct_answer, explanation, difficulty, points, tags, category_id, image_urls, created_by, created_at, updated_at
		FROM questions WHERE 1=1
	`
	args := []interface{}{}
	argCount := 1

	if filters.Type != "" {
		query += fmt.Sprintf(" AND type = $%d", argCount)
		args = append(args, filters.Type)
		argCount++
	}

	if filters.Difficulty != "" {
		query += fmt.Sprintf(" AND difficulty = $%d", argCount)
		args = append(args, filters.Difficulty)
		argCount++
	}

	if filters.CategoryID != "" {
		query += fmt.Sprintf(" AND category_id = $%d", argCount)
		args = append(args, filters.CategoryID)
		argCount++
	}

	if filters.CreatedBy != "" {
		query += fmt.Sprintf(" AND created_by = $%d", argCount)
		args = append(args, filters.CreatedBy)
		argCount++
	}

	if filters.Search != "" {
		query += fmt.Sprintf(" AND (title ILIKE $%d OR content ILIKE $%d)", argCount, argCount+1)
		searchPattern := "%" + filters.Search + "%"
		args = append(args, searchPattern, searchPattern)
		argCount += 2
	}

	query += " ORDER BY created_at DESC"

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
		return nil, fmt.Errorf("failed to list questions: %w", err)
	}
	defer rows.Close()

	var questions []*models.Question
	for rows.Next() {
		question := &models.Question{}
		var optionsJSON, tagsJSON, imageJSON []byte
		var difficulty sql.NullString
		var points sql.NullInt64

		err := rows.Scan(
			&question.ID, &question.Type, &question.Title, &question.Content,
			&optionsJSON, &question.CorrectAnswer, &question.Explanation,
			&difficulty, &points, &tagsJSON,
			&question.CategoryID, &imageJSON, &question.CreatedBy, &question.CreatedAt, &question.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan question: %w", err)
		}

		if difficulty.Valid && difficulty.String != "" {
			question.Difficulty = difficulty.String
		} else {
			question.Difficulty = "medium"
		}
		if points.Valid {
			question.Points = int(points.Int64)
		} else {
			question.Points = 5
		}

		if len(optionsJSON) > 0 {
			var optArr []string
			if err := json.Unmarshal(optionsJSON, &optArr); err == nil {
				raw, _ := json.Marshal(optArr)
				s := string(raw)
				question.Options = &s
			}
		}
		if len(tagsJSON) > 0 {
			var tagArr []string
			if err := json.Unmarshal(tagsJSON, &tagArr); err == nil {
				raw, _ := json.Marshal(tagArr)
				s := string(raw)
				question.Tags = &s
			}
		}
		if len(imageJSON) > 0 {
			s := string(imageJSON)
			question.ImageURLs = &s
		}

		questions = append(questions, question)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list questions rows: %w", err)
	}

	return questions, nil
}

// Update updates a question
func (r *QuestionRepository) Update(question *models.Question) error {
	query := `
		UPDATE questions SET
			type = $2, title = $3, content = $4, options = $5,
			correct_answer = $6, explanation = $7, difficulty = $8,
			points = $9, tags = $10, category_id = $11, image_urls = $12
		WHERE id = $1
	`
	_, err := r.db.Exec(query,
		question.ID, question.Type, question.Title, question.Content,
		jsonbStringArg(question.Options), question.CorrectAnswer, question.Explanation,
		question.Difficulty, question.Points, jsonbStringArg(question.Tags),
		question.CategoryID, jsonbStringArg(question.ImageURLs),
	)
	if err != nil {
		return fmt.Errorf("failed to update question: %w", err)
	}
	return nil
}

// Delete deletes a question by ID
func (r *QuestionRepository) Delete(id string) error {
	query := `DELETE FROM questions WHERE id = $1`
	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete question: %w", err)
	}
	return nil
}

// CountByCreator returns how many questions were created by a user.
func (r *QuestionRepository) CountByCreator(createdBy string) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM questions WHERE created_by = $1`, createdBy).Scan(&n)
	return n, err
}

// QuestionFilters represents filters for listing questions
type QuestionFilters struct {
	Type       string
	Difficulty string
	CategoryID string
	CreatedBy  string
	Search     string
	Limit      int
	Offset     int
	MissingOptions bool // Find MCQ questions without options
}

// FindQuestionsMissingOptions finds multiple-choice questions that don't have options set
func (r *QuestionRepository) FindQuestionsMissingOptions(createdBy string) ([]*models.Question, error) {
	query := `
		SELECT id, type, title, content, options, correct_answer, explanation, difficulty, points, tags, category_id, image_urls, created_by, created_at, updated_at
		FROM questions
		WHERE type = 'multiple-choice'
		AND (options IS NULL OR options = '[]'::jsonb)
	`
	args := []interface{}{}

	if createdBy != "" {
		query += " AND created_by = $1"
		args = append(args, createdBy)
	}

	query += " ORDER BY created_at DESC"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to find questions missing options: %w", err)
	}
	defer rows.Close()

	var questions []*models.Question
	for rows.Next() {
		question := &models.Question{}
		var optionsJSON []byte
		var tagsJSON []byte
		var imageJSON []byte
		var difficulty sql.NullString
		var points sql.NullInt64

		err := rows.Scan(
			&question.ID, &question.Type, &question.Title, &question.Content,
			&optionsJSON, &question.CorrectAnswer, &question.Explanation,
			&difficulty, &points, &tagsJSON,
			&question.CategoryID, &imageJSON, &question.CreatedBy, &question.CreatedAt, &question.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan question: %w", err)
		}
		_ = optionsJSON // row matched MCQ with null/empty options; model stays nil below

		if difficulty.Valid && difficulty.String != "" {
			question.Difficulty = difficulty.String
		} else {
			question.Difficulty = "medium"
		}
		if points.Valid {
			question.Points = int(points.Int64)
		} else {
			question.Points = 5
		}

		// Options are empty by definition for this query
		question.Options = nil

		if len(tagsJSON) > 0 {
			var tagArr []string
			if err := json.Unmarshal(tagsJSON, &tagArr); err == nil {
				raw, _ := json.Marshal(tagArr)
				s := string(raw)
				question.Tags = &s
			}
		}
		if len(imageJSON) > 0 {
			s := string(imageJSON)
			question.ImageURLs = &s
		}

		questions = append(questions, question)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find questions missing options rows: %w", err)
	}

	return questions, nil
}

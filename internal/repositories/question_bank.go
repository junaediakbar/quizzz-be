package repositories

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/junaediakbar/quizzz-backend/models"
)

type QuestionBankRepository struct {
	db *sql.DB
}

func NewQuestionBankRepository(db *sql.DB) *QuestionBankRepository {
	return &QuestionBankRepository{db: db}
}

func (r *QuestionBankRepository) Create(bank *models.QuestionBank) error {
	q := `INSERT INTO question_banks (id, name, description, created_by, is_public) VALUES ($1,$2,$3,$4,$5)`
	_, err := r.db.Exec(q, bank.ID, bank.Name, bank.Description, bank.CreatedBy, bank.IsPublic)
	if err != nil {
		return fmt.Errorf("failed to create question bank: %w", err)
	}
	return nil
}

func (r *QuestionBankRepository) FindByID(id string) (*models.QuestionBank, error) {
	q := `SELECT id, name, description, created_by, is_public, created_at, updated_at FROM question_banks WHERE id = $1`
	b := &models.QuestionBank{}
	var isPublic sql.NullBool
	err := r.db.QueryRow(q, id).Scan(&b.ID, &b.Name, &b.Description, &b.CreatedBy, &isPublic, &b.CreatedAt, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrBankNotFound
	}
	if err != nil {
		return nil, err
	}
	b.IsPublic = isPublic.Valid && isPublic.Bool
	return b, nil
}

func bankWhere(createdBy string) (string, []interface{}) {
	if createdBy == "" {
		return "", nil
	}
	return " WHERE created_by = $1", []interface{}{createdBy}
}

// Count mengembalikan jumlah bank soal milik pembuat tsb.
func (r *QuestionBankRepository) Count(createdBy string) (int, error) {
	where, args := bankWhere(createdBy)
	var total int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM question_banks"+where, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count banks: %w", err)
	}
	return total, nil
}

// List mengambil bank soal dengan pagination (limit 0 = tanpa batas).
func (r *QuestionBankRepository) List(createdBy string, limit, offset int) ([]*models.QuestionBank, error) {
	where, args := bankWhere(createdBy)
	q := `SELECT id, name, description, created_by, is_public, created_at, updated_at FROM question_banks` + where
	q += " ORDER BY created_at DESC, id DESC"

	argN := len(args) + 1
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT $%d", argN)
		args = append(args, limit)
		argN++
	}
	if offset > 0 {
		q += fmt.Sprintf(" OFFSET $%d", argN)
		args = append(args, offset)
	}

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list banks: %w", err)
	}
	defer rows.Close()

	var banks []*models.QuestionBank
	for rows.Next() {
		b := &models.QuestionBank{}
		if err := rows.Scan(&b.ID, &b.Name, &b.Description, &b.CreatedBy, &b.IsPublic, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan bank: %w", err)
		}
		banks = append(banks, b)
	}

	return banks, nil
}

func (r *QuestionBankRepository) Update(bank *models.QuestionBank) error {
	q := `UPDATE question_banks SET name = $2, description = $3, is_public = $4, updated_at = $5 WHERE id = $1`
	_, err := r.db.Exec(q, bank.ID, bank.Name, bank.Description, bank.IsPublic, time.Now())
	return err
}

func (r *QuestionBankRepository) Delete(id string) error {
	res, err := r.db.Exec(`DELETE FROM question_banks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrBankNotFound
	}
	return nil
}

func (r *QuestionBankRepository) AddQuestion(bankID, questionID string, order int) error {
	id := generateUUID()
	q := `
		INSERT INTO question_bank_questions (id, bank_id, question_id, order_index)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (bank_id, question_id) DO UPDATE SET order_index = EXCLUDED.order_index
	`
	_, err := r.db.Exec(q, id, bankID, questionID, order)
	return err
}

func (r *QuestionBankRepository) RemoveQuestion(bankID, questionID string) error {
	res, err := r.db.Exec(`DELETE FROM question_bank_questions WHERE bank_id = $1 AND question_id = $2`, bankID, questionID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrQuestionNotInBank
	}
	return nil
}

func (r *QuestionBankRepository) ListQuestionIDs(bankID string) ([]string, error) {
	rows, err := r.db.Query(`SELECT question_id FROM question_bank_questions WHERE bank_id = $1 ORDER BY order_index`, bankID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

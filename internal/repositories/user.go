package repositories

import (
	"database/sql"
	"fmt"

	"github.com/junaediakbar/quizzz-backend/models"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create inserts a new user into the database
func (r *UserRepository) Create(user *models.User) error {
	query := `
		INSERT INTO users (id, name, email, password_hash, role, avatar)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.Exec(query, user.ID, user.Name, user.Email, user.Password, user.Role, user.Avatar)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

// FindByID retrieves a user by ID
func (r *UserRepository) FindByID(id string) (*models.User, error) {
	query := `
		SELECT id, name, email, password_hash, role, avatar, created_at, updated_at
		FROM users WHERE id = $1
	`
	user := &models.User{}
	err := r.db.QueryRow(query, id).Scan(
		&user.ID, &user.Name, &user.Email, &user.Password, &user.Role,
		&user.Avatar, &user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user: %w", err)
	}
	return user, nil
}

// FindByEmail retrieves a user by email
func (r *UserRepository) FindByEmail(email string) (*models.User, error) {
	query := `
		SELECT id, name, email, password_hash, role, avatar, created_at, updated_at
		FROM users WHERE email = $1
	`
	user := &models.User{}
	err := r.db.QueryRow(query, email).Scan(
		&user.ID, &user.Name, &user.Email, &user.Password, &user.Role,
		&user.Avatar, &user.CreatedAt, &user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by email: %w", err)
	}
	return user, nil
}

// Update updates a user's information
func (r *UserRepository) Update(user *models.User) error {
	query := `
		UPDATE users SET name = $2, email = $3, avatar = $4
		WHERE id = $1
	`
	_, err := r.db.Exec(query, user.ID, user.Name, user.Email, user.Avatar)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

// UpdatePassword updates only a user's password hash
func (r *UserRepository) UpdatePassword(id string, passwordHash string) error {
	query := `
		UPDATE users SET password_hash = $2
		WHERE id = $1
	`
	_, err := r.db.Exec(query, id, passwordHash)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	return nil
}

// AdminUpdate updates name, email, role, and avatar (admin).
func (r *UserRepository) AdminUpdate(user *models.User) error {
	query := `
		UPDATE users SET name = $2, email = $3, role = $4, avatar = $5
		WHERE id = $1
	`
	_, err := r.db.Exec(query, user.ID, user.Name, user.Email, user.Role, user.Avatar)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

// CountByRole returns number of users with the given role.
func (r *UserRepository) CountByRole(role string) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE role = $1`
	var n int
	err := r.db.QueryRow(query, role).Scan(&n)
	return n, err
}

// Delete deletes a user by ID
func (r *UserRepository) Delete(id string) error {
	query := `DELETE FROM users WHERE id = $1`
	res, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// List retrieves all users with optional role filter
// UserFilters menyaring daftar pengguna (admin).
type UserFilters struct {
	Role   string
	Search string // ILIKE pada name/email
	Limit  int
	Offset int
}

func buildUserWhere(filters UserFilters) (string, []interface{}) {
	where := ""
	args := []interface{}{}
	argN := 1
	if filters.Role != "" {
		where += fmt.Sprintf(" AND role = $%d", argN)
		args = append(args, filters.Role)
		argN++
	}
	if filters.Search != "" {
		where += fmt.Sprintf(" AND (name ILIKE $%d OR email ILIKE $%d)", argN, argN+1)
		pattern := "%" + filters.Search + "%"
		args = append(args, pattern, pattern)
		argN += 2
	}
	return where, args
}

// CountUsers mengembalikan jumlah pengguna yang cocok dengan filter.
func (r *UserRepository) CountUsers(filters UserFilters) (int, error) {
	where, args := buildUserWhere(filters)
	var total int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM users WHERE 1=1"+where, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}
	return total, nil
}

// List mengambil pengguna dengan filter + pagination.
func (r *UserRepository) List(filters UserFilters) ([]*models.User, error) {
	where, args := buildUserWhere(filters)
	query := `
		SELECT id, name, email, password_hash, role, avatar, created_at, updated_at
		FROM users
		WHERE 1=1` + where + ` ORDER BY name ASC, id ASC`

	argN := len(args) + 1
	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argN)
		args = append(args, filters.Limit)
		argN++
	}
	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argN)
		args = append(args, filters.Offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user := &models.User{}
		err := rows.Scan(
			&user.ID, &user.Name, &user.Email, &user.Password, &user.Role,
			&user.Avatar, &user.CreatedAt, &user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, user)
	}

	return users, nil
}

// ListStudents returns users with role student, optional ILIKE search on name or email.
// ListStudents returns users with role student, optional ILIKE search, with pagination.
func (r *UserRepository) ListStudents(search string, limit, offset int) ([]*models.User, error) {
	return r.List(UserFilters{Role: "student", Search: search, Limit: limit, Offset: offset})
}

// CountStudents mengembalikan jumlah murid yang cocok dengan pencarian.
func (r *UserRepository) CountStudents(search string) (int, error) {
	return r.CountUsers(UserFilters{Role: "student", Search: search})
}

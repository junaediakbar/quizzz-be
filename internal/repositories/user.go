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
	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// List retrieves all users with optional role filter
func (r *UserRepository) List(role string) ([]*models.User, error) {
	query := `
		SELECT id, name, email, password_hash, role, avatar, created_at, updated_at
		FROM users
	`
	args := []interface{}{}
	if role != "" {
		query += " WHERE role = $1"
		args = append(args, role)
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
func (r *UserRepository) ListStudents(search string) ([]*models.User, error) {
	query := `
		SELECT id, name, email, password_hash, role, avatar, created_at, updated_at
		FROM users
		WHERE role = 'student'
	`
	args := []interface{}{}
	if search != "" {
		query += ` AND (name ILIKE $1 OR email ILIKE $1)`
		args = append(args, "%"+search+"%")
	}
	query += ` ORDER BY name ASC`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list students: %w", err)
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

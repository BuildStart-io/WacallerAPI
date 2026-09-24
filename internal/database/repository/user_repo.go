package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"wacallerapi/internal/database"
	"wacallerapi/internal/domain"
)

type userRepo struct {
	db *database.DB
}

func NewUserRepository(db *database.DB) UserRepository {
	return &userRepo{db: db}
}

func (r *userRepo) Create(ctx context.Context, user *domain.User) error {
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = time.Now()
	}
	if user.Status == "" {
		user.Status = "active"
	}
	if user.Role == "" {
		user.Role = "developer"
	}
	if user.PasswordVersion <= 0 {
		user.PasswordVersion = 2
	}

	query := `
		INSERT INTO users (id, email, name, role, password_hash, password_version, email_verified, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.Email, user.Name, user.Role, user.PasswordHash, user.PasswordVersion, user.EmailVerified, user.Status, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *userRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	query := `
		SELECT id, email, name, role, password_hash, password_version, email_verified, status, created_at, updated_at
		FROM users WHERE id = $1
	`
	u := &domain.User{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &u.PasswordVersion, &u.EmailVerified, &u.Status, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

func (r *userRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, email, name, role, password_hash, password_version, email_verified, status, created_at, updated_at
		FROM users WHERE email = $1
	`
	u := &domain.User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &u.PasswordVersion, &u.EmailVerified, &u.Status, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

func (r *userRepo) Update(ctx context.Context, user *domain.User) error {
	user.UpdatedAt = time.Now()
	query := `
		UPDATE users
		SET email = $1, name = $2, role = $3, password_hash = $4, password_version = $5, email_verified = $6, status = $7, updated_at = $8
		WHERE id = $9
	`
	res, err := r.db.ExecContext(ctx, query,
		user.Email, user.Name, user.Role, user.PasswordHash, user.PasswordVersion, user.EmailVerified, user.Status, user.UpdatedAt, user.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *userRepo) List(ctx context.Context, limit, offset int) ([]*domain.User, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, email, name, role, password_hash, password_version, email_verified, status, created_at, updated_at
		FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		u := &domain.User{}
		if err := rows.Scan(
			&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &u.PasswordVersion, &u.EmailVerified, &u.Status, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

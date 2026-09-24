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

type orgRepo struct {
	db *database.DB
}

func NewOrgRepository(db *database.DB) OrgRepository {
	return &orgRepo{db: db}
}

func (r *orgRepo) Create(ctx context.Context, org *domain.Organization) error {
	if org.CreatedAt.IsZero() {
		org.CreatedAt = time.Now()
	}
	if org.UpdatedAt.IsZero() {
		org.UpdatedAt = time.Now()
	}
	if org.Status == "" {
		org.Status = "active"
	}

	query := `
		INSERT INTO organizations (id, name, slug, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.ExecContext(ctx, query, org.ID, org.Name, org.Slug, org.Status, org.CreatedAt, org.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create organization: %w", err)
	}
	return nil
}

func (r *orgRepo) GetByID(ctx context.Context, id string) (*domain.Organization, error) {
	query := `SELECT id, name, slug, status, created_at, updated_at FROM organizations WHERE id = $1`
	org := &domain.Organization{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(&org.ID, &org.Name, &org.Slug, &org.Status, &org.CreatedAt, &org.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get organization by id: %w", err)
	}
	return org, nil
}

func (r *orgRepo) GetBySlug(ctx context.Context, slug string) (*domain.Organization, error) {
	query := `SELECT id, name, slug, status, created_at, updated_at FROM organizations WHERE slug = $1`
	org := &domain.Organization{}
	err := r.db.QueryRowContext(ctx, query, slug).Scan(&org.ID, &org.Name, &org.Slug, &org.Status, &org.CreatedAt, &org.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get organization by slug: %w", err)
	}
	return org, nil
}

func (r *orgRepo) Update(ctx context.Context, org *domain.Organization) error {
	org.UpdatedAt = time.Now()
	query := `
		UPDATE organizations
		SET name = $1, slug = $2, status = $3, updated_at = $4
		WHERE id = $5
	`
	res, err := r.db.ExecContext(ctx, query, org.Name, org.Slug, org.Status, org.UpdatedAt, org.ID)
	if err != nil {
		return fmt.Errorf("update organization: %w", err)
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

func (r *orgRepo) List(ctx context.Context, limit, offset int) ([]*domain.Organization, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, name, slug, status, created_at, updated_at FROM organizations ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()

	var orgs []*domain.Organization
	for rows.Next() {
		org := &domain.Organization{}
		if err := rows.Scan(&org.ID, &org.Name, &org.Slug, &org.Status, &org.CreatedAt, &org.UpdatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	return orgs, rows.Err()
}

func (r *orgRepo) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM organizations WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete organization: %w", err)
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

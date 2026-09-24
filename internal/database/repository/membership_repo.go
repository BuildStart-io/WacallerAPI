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

type membershipRepo struct {
	db *database.DB
}

func NewMembershipRepository(db *database.DB) MembershipRepository {
	return &membershipRepo{db: db}
}

func (r *membershipRepo) Create(ctx context.Context, m *domain.Membership) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.Role == "" {
		m.Role = "developer"
	}

	query := `
		INSERT INTO memberships (id, user_id, organization_id, role, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(ctx, query, m.ID, m.UserID, m.OrganizationID, m.Role, m.CreatedAt)
	if err != nil {
		return fmt.Errorf("create membership: %w", err)
	}
	return nil
}

func (r *membershipRepo) GetByID(ctx context.Context, id string) (*domain.Membership, error) {
	query := `SELECT id, user_id, organization_id, role, created_at FROM memberships WHERE id = $1`
	m := &domain.Membership{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(&m.ID, &m.UserID, &m.OrganizationID, &m.Role, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get membership by id: %w", err)
	}
	return m, nil
}

func (r *membershipRepo) GetByUserAndOrg(ctx context.Context, userID, orgID string) (*domain.Membership, error) {
	query := `SELECT id, user_id, organization_id, role, created_at FROM memberships WHERE user_id = $1 AND organization_id = $2`
	m := &domain.Membership{}
	err := r.db.QueryRowContext(ctx, query, userID, orgID).Scan(&m.ID, &m.UserID, &m.OrganizationID, &m.Role, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get membership by user and org: %w", err)
	}
	return m, nil
}

func (r *membershipRepo) ListByOrg(ctx context.Context, orgID string) ([]*domain.Membership, error) {
	query := `SELECT id, user_id, organization_id, role, created_at FROM memberships WHERE organization_id = $1 ORDER BY created_at ASC`
	rows, err := r.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("list memberships by org: %w", err)
	}
	defer rows.Close()

	var list []*domain.Membership
	for rows.Next() {
		m := &domain.Membership{}
		if err := rows.Scan(&m.ID, &m.UserID, &m.OrganizationID, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (r *membershipRepo) ListByUser(ctx context.Context, userID string) ([]*domain.Membership, error) {
	query := `SELECT id, user_id, organization_id, role, created_at FROM memberships WHERE user_id = $1 ORDER BY created_at ASC`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list memberships by user: %w", err)
	}
	defer rows.Close()

	var list []*domain.Membership
	for rows.Next() {
		m := &domain.Membership{}
		if err := rows.Scan(&m.ID, &m.UserID, &m.OrganizationID, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (r *membershipRepo) UpdateRole(ctx context.Context, id, role string) error {
	query := `UPDATE memberships SET role = $1 WHERE id = $2`
	res, err := r.db.ExecContext(ctx, query, role, id)
	if err != nil {
		return fmt.Errorf("update membership role: %w", err)
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

func (r *membershipRepo) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM memberships WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete membership: %w", err)
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

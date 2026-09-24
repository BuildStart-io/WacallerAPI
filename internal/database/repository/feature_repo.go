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

type featureRepo struct {
	db *database.DB
}

func NewFeatureRepository(db *database.DB) FeatureRepository {
	return &featureRepo{db: db}
}

func (r *featureRepo) GetFlag(ctx context.Context, name string, orgID string) (*domain.FeatureFlag, error) {
	// First check org-level override flag if orgID is provided
	if orgID != "" {
		query := `
			SELECT id, name, enabled, description, scope, target_org_id, updated_by, updated_at
			FROM feature_flags WHERE name = $1 AND target_org_id = $2
		`
		flag := &domain.FeatureFlag{}
		err := r.db.QueryRowContext(ctx, query, name, orgID).Scan(
			&flag.ID, &flag.Name, &flag.Enabled, &flag.Description, &flag.Scope, &flag.TargetOrgID, &flag.UpdatedBy, &flag.UpdatedAt,
		)
		if err == nil {
			return flag, nil
		}
	}

	// Fallback to global flag
	query := `
		SELECT id, name, enabled, description, scope, target_org_id, updated_by, updated_at
		FROM feature_flags WHERE name = $1 AND (scope = 'global' OR target_org_id IS NULL)
	`
	flag := &domain.FeatureFlag{}
	err := r.db.QueryRowContext(ctx, query, name).Scan(
		&flag.ID, &flag.Name, &flag.Enabled, &flag.Description, &flag.Scope, &flag.TargetOrgID, &flag.UpdatedBy, &flag.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get feature flag: %w", err)
	}
	return flag, nil
}

func (r *featureRepo) SetFlag(ctx context.Context, flag *domain.FeatureFlag) error {
	flag.UpdatedAt = time.Now()
	if flag.Scope == "" {
		flag.Scope = "global"
	}

	query := `
		INSERT INTO feature_flags (id, name, enabled, description, scope, target_org_id, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (name) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			description = EXCLUDED.description,
			scope = EXCLUDED.scope,
			target_org_id = EXCLUDED.target_org_id,
			updated_by = EXCLUDED.updated_by,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.ExecContext(ctx, query,
		flag.ID, flag.Name, flag.Enabled, flag.Description, flag.Scope, flag.TargetOrgID, flag.UpdatedBy, flag.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("set feature flag: %w", err)
	}
	return nil
}

func (r *featureRepo) ListFlags(ctx context.Context) ([]*domain.FeatureFlag, error) {
	query := `
		SELECT id, name, enabled, description, scope, target_org_id, updated_by, updated_at
		FROM feature_flags ORDER BY name ASC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list feature flags: %w", err)
	}
	defer rows.Close()

	var list []*domain.FeatureFlag
	for rows.Next() {
		flag := &domain.FeatureFlag{}
		if err := rows.Scan(
			&flag.ID, &flag.Name, &flag.Enabled, &flag.Description, &flag.Scope, &flag.TargetOrgID, &flag.UpdatedBy, &flag.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, flag)
	}
	return list, rows.Err()
}

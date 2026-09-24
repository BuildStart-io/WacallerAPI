package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"wacallerapi/internal/database"
	"wacallerapi/internal/domain"
)

type credentialRepo struct {
	db *database.DB
}

func NewCredentialRepository(db *database.DB) CredentialRepository {
	return &credentialRepo{db: db}
}

func (r *credentialRepo) Create(ctx context.Context, cred *domain.APICredential) error {
	if cred.CreatedAt.IsZero() {
		cred.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO api_credentials (id, organization_id, name, key_hash, key_prefix, scopes, created_by, expires_at, last_used_at, revoked_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := r.db.ExecContext(ctx, query,
		cred.ID, cred.OrganizationID, cred.Name, cred.KeyHash, cred.KeyPrefix, pq.Array(cred.Scopes), cred.CreatedBy, cred.ExpiresAt, cred.LastUsedAt, cred.RevokedAt, cred.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create api credential: %w", err)
	}
	return nil
}

func (r *credentialRepo) GetByHash(ctx context.Context, keyHash string) (*domain.APICredential, error) {
	query := `
		SELECT id, organization_id, name, key_hash, key_prefix, scopes, created_by, expires_at, last_used_at, revoked_at, created_at
		FROM api_credentials WHERE key_hash = $1
	`
	cred := &domain.APICredential{}
	var scopes pq.StringArray
	err := r.db.QueryRowContext(ctx, query, keyHash).Scan(
		&cred.ID, &cred.OrganizationID, &cred.Name, &cred.KeyHash, &cred.KeyPrefix, &scopes, &cred.CreatedBy, &cred.ExpiresAt, &cred.LastUsedAt, &cred.RevokedAt, &cred.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get api credential by hash: %w", err)
	}
	cred.Scopes = []string(scopes)
	return cred, nil
}

func (r *credentialRepo) GetByID(ctx context.Context, id string) (*domain.APICredential, error) {
	query := `
		SELECT id, organization_id, name, key_hash, key_prefix, scopes, created_by, expires_at, last_used_at, revoked_at, created_at
		FROM api_credentials WHERE id = $1
	`
	cred := &domain.APICredential{}
	var scopes pq.StringArray
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&cred.ID, &cred.OrganizationID, &cred.Name, &cred.KeyHash, &cred.KeyPrefix, &scopes, &cred.CreatedBy, &cred.ExpiresAt, &cred.LastUsedAt, &cred.RevokedAt, &cred.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get api credential by id: %w", err)
	}
	cred.Scopes = []string(scopes)
	return cred, nil
}

func (r *credentialRepo) ListByOrg(ctx context.Context, orgID string) ([]*domain.APICredential, error) {
	query := `
		SELECT id, organization_id, name, key_hash, key_prefix, scopes, created_by, expires_at, last_used_at, revoked_at, created_at
		FROM api_credentials WHERE organization_id = $1 ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("list api credentials by org: %w", err)
	}
	defer rows.Close()

	var list []*domain.APICredential
	for rows.Next() {
		cred := &domain.APICredential{}
		var scopes pq.StringArray
		if err := rows.Scan(
			&cred.ID, &cred.OrganizationID, &cred.Name, &cred.KeyHash, &cred.KeyPrefix, &scopes, &cred.CreatedBy, &cred.ExpiresAt, &cred.LastUsedAt, &cred.RevokedAt, &cred.CreatedAt,
		); err != nil {
			return nil, err
		}
		cred.Scopes = []string(scopes)
		list = append(list, cred)
	}
	return list, rows.Err()
}

func (r *credentialRepo) Revoke(ctx context.Context, id string) error {
	now := time.Now()
	query := `UPDATE api_credentials SET revoked_at = $1 WHERE id = $2`
	res, err := r.db.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("revoke api credential: %w", err)
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

func (r *credentialRepo) UpdateLastUsed(ctx context.Context, id string, t time.Time) error {
	query := `UPDATE api_credentials SET last_used_at = $1 WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, t, id)
	if err != nil {
		return fmt.Errorf("update api credential last used: %w", err)
	}
	return nil
}

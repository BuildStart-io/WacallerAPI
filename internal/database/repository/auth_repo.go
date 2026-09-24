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

type authRepo struct {
	db *database.DB
}

func NewAuthRepository(db *database.DB) AuthRepository {
	return &authRepo{db: db}
}

func (r *authRepo) CreateIdentity(ctx context.Context, identity *domain.Identity) error {
	if identity.CreatedAt.IsZero() {
		identity.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO identities (id, user_id, provider, provider_id, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.ExecContext(ctx, query, identity.ID, identity.UserID, identity.Provider, identity.ProviderID, identity.Metadata, identity.CreatedAt)
	if err != nil {
		return fmt.Errorf("create identity: %w", err)
	}
	return nil
}

func (r *authRepo) GetIdentityByProvider(ctx context.Context, provider, providerID string) (*domain.Identity, error) {
	query := `SELECT id, user_id, provider, provider_id, metadata, created_at FROM identities WHERE provider = $1 AND provider_id = $2`
	ident := &domain.Identity{}
	err := r.db.QueryRowContext(ctx, query, provider, providerID).Scan(&ident.ID, &ident.UserID, &ident.Provider, &ident.ProviderID, &ident.Metadata, &ident.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get identity by provider: %w", err)
	}
	return ident, nil
}

func (r *authRepo) CreateSession(ctx context.Context, session *domain.AuthSession) error {
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO auth_sessions (id, user_id, token_hash, ip_address, user_agent, expires_at, revoked_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.ExecContext(ctx, query, session.ID, session.UserID, session.TokenHash, session.IPAddress, session.UserAgent, session.ExpiresAt, session.RevokedAt, session.CreatedAt)
	if err != nil {
		return fmt.Errorf("create auth session: %w", err)
	}
	return nil
}

func (r *authRepo) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*domain.AuthSession, error) {
	query := `
		SELECT id, user_id, token_hash, ip_address, user_agent, expires_at, revoked_at, created_at
		FROM auth_sessions WHERE token_hash = $1
	`
	s := &domain.AuthSession{}
	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(&s.ID, &s.UserID, &s.TokenHash, &s.IPAddress, &s.UserAgent, &s.ExpiresAt, &s.RevokedAt, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get auth session by token hash: %w", err)
	}
	return s, nil
}

func (r *authRepo) RevokeSession(ctx context.Context, id string) error {
	now := time.Now()
	query := `UPDATE auth_sessions SET revoked_at = $1 WHERE id = $2`
	res, err := r.db.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("revoke auth session: %w", err)
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

func (r *authRepo) RevokeUserSessions(ctx context.Context, userID string) error {
	now := time.Now()
	query := `UPDATE auth_sessions SET revoked_at = $1 WHERE user_id = $2 AND revoked_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, now, userID)
	if err != nil {
		return fmt.Errorf("revoke user auth sessions: %w", err)
	}
	return nil
}

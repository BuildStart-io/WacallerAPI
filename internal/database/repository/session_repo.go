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

type sessionRepo struct {
	db *database.DB
}

func NewSessionRepository(db *database.DB) SessionRepository {
	return &sessionRepo{db: db}
}

func (r *sessionRepo) Create(ctx context.Context, sess *domain.WhatsAppSession) error {
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now()
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = time.Now()
	}
	if sess.Status == "" {
		sess.Status = "disconnected"
	}
	if sess.DesiredState == "" {
		sess.DesiredState = "connected"
	}

	query := `
		INSERT INTO whatsapp_sessions (id, organization_id, name, jid, phone_number, status, desired_state, worker_id, lease_version, last_heartbeat, last_error, webhook_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`
	_, err := r.db.ExecContext(ctx, query,
		sess.ID, sess.OrganizationID, sess.Name, sess.JID, sess.PhoneNumber, sess.Status, sess.DesiredState, sess.WorkerID, sess.LeaseVersion, sess.LastHeartbeat, sess.LastError, sess.WebhookURL, sess.CreatedAt, sess.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create whatsapp session: %w", err)
	}
	return nil
}

func (r *sessionRepo) GetByID(ctx context.Context, id string) (*domain.WhatsAppSession, error) {
	query := `
		SELECT id, organization_id, name, jid, phone_number, status, desired_state, worker_id, lease_version, last_heartbeat, last_error, webhook_url, created_at, updated_at
		FROM whatsapp_sessions WHERE id = $1
	`
	s := &domain.WhatsAppSession{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&s.ID, &s.OrganizationID, &s.Name, &s.JID, &s.PhoneNumber, &s.Status, &s.DesiredState, &s.WorkerID, &s.LeaseVersion, &s.LastHeartbeat, &s.LastError, &s.WebhookURL, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get whatsapp session by id: %w", err)
	}
	return s, nil
}

func (r *sessionRepo) ListByOrg(ctx context.Context, orgID string) ([]*domain.WhatsAppSession, error) {
	query := `
		SELECT id, organization_id, name, jid, phone_number, status, desired_state, worker_id, lease_version, last_heartbeat, last_error, webhook_url, created_at, updated_at
		FROM whatsapp_sessions WHERE organization_id = $1 ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("list whatsapp sessions by org: %w", err)
	}
	defer rows.Close()

	var list []*domain.WhatsAppSession
	for rows.Next() {
		s := &domain.WhatsAppSession{}
		if err := rows.Scan(
			&s.ID, &s.OrganizationID, &s.Name, &s.JID, &s.PhoneNumber, &s.Status, &s.DesiredState, &s.WorkerID, &s.LeaseVersion, &s.LastHeartbeat, &s.LastError, &s.WebhookURL, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func (r *sessionRepo) ListAll(ctx context.Context) ([]*domain.WhatsAppSession, error) {
	query := `
		SELECT id, organization_id, name, jid, phone_number, status, desired_state, worker_id, lease_version, last_heartbeat, last_error, webhook_url, created_at, updated_at
		FROM whatsapp_sessions ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list all whatsapp sessions: %w", err)
	}
	defer rows.Close()

	var list []*domain.WhatsAppSession
	for rows.Next() {
		s := &domain.WhatsAppSession{}
		if err := rows.Scan(
			&s.ID, &s.OrganizationID, &s.Name, &s.JID, &s.PhoneNumber, &s.Status, &s.DesiredState, &s.WorkerID, &s.LeaseVersion, &s.LastHeartbeat, &s.LastError, &s.WebhookURL, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func (r *sessionRepo) UpdateStatus(ctx context.Context, id, status, jid, phoneNumber, lastError string) error {
	now := time.Now()
	query := `
		UPDATE whatsapp_sessions
		SET status = $1, jid = COALESCE(NULLIF($2, ''), jid), phone_number = COALESCE(NULLIF($3, ''), phone_number), last_error = $4, updated_at = $5
		WHERE id = $6
	`
	res, err := r.db.ExecContext(ctx, query, status, jid, phoneNumber, lastError, now, id)
	if err != nil {
		return fmt.Errorf("update whatsapp session status: %w", err)
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

func (r *sessionRepo) UpdateHeartbeat(ctx context.Context, id, workerID string, leaseVersion int64) error {
	now := time.Now()
	query := `
		UPDATE whatsapp_sessions
		SET worker_id = $1, lease_version = $2, last_heartbeat = $3, updated_at = $3
		WHERE id = $4
	`
	_, err := r.db.ExecContext(ctx, query, workerID, leaseVersion, now, id)
	if err != nil {
		return fmt.Errorf("update whatsapp session heartbeat: %w", err)
	}
	return nil
}

func (r *sessionRepo) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM whatsapp_sessions WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete whatsapp session: %w", err)
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

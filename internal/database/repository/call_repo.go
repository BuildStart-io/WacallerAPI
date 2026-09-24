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

type callRepo struct {
	db *database.DB
}

func NewCallRepository(db *database.DB) CallRepository {
	return &callRepo{db: db}
}

func (r *callRepo) Create(ctx context.Context, call *domain.Call) error {
	if call.CreatedAt.IsZero() {
		call.CreatedAt = time.Now()
	}
	if call.StartedAt.IsZero() {
		call.StartedAt = time.Now()
	}

	query := `
		INSERT INTO calls (id, organization_id, session_id, direction, peer_number, status, duration_seconds, started_at, connected_at, ended_at, end_reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := r.db.ExecContext(ctx, query,
		call.ID, call.OrganizationID, call.SessionID, call.Direction, call.PeerNumber, call.Status, call.DurationSeconds, call.StartedAt, call.ConnectedAt, call.EndedAt, call.EndReason, call.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create call record: %w", err)
	}
	return nil
}

func (r *callRepo) GetByID(ctx context.Context, id string) (*domain.Call, error) {
	query := `
		SELECT id, organization_id, session_id, direction, peer_number, status, duration_seconds, started_at, connected_at, ended_at, end_reason, created_at
		FROM calls WHERE id = $1
	`
	c := &domain.Call{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.OrganizationID, &c.SessionID, &c.Direction, &c.PeerNumber, &c.Status, &c.DurationSeconds, &c.StartedAt, &c.ConnectedAt, &c.EndedAt, &c.EndReason, &c.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get call record by id: %w", err)
	}
	return c, nil
}

func (r *callRepo) ListByOrg(ctx context.Context, orgID string, limit, offset int) ([]*domain.Call, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, organization_id, session_id, direction, peer_number, status, duration_seconds, started_at, connected_at, ended_at, end_reason, created_at
		FROM calls WHERE organization_id = $1 ORDER BY started_at DESC LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, orgID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list calls by org: %w", err)
	}
	defer rows.Close()

	var calls []*domain.Call
	for rows.Next() {
		c := &domain.Call{}
		if err := rows.Scan(
			&c.ID, &c.OrganizationID, &c.SessionID, &c.Direction, &c.PeerNumber, &c.Status, &c.DurationSeconds, &c.StartedAt, &c.ConnectedAt, &c.EndedAt, &c.EndReason, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		calls = append(calls, c)
	}
	return calls, rows.Err()
}

func (r *callRepo) ListBySession(ctx context.Context, sessionID string, limit, offset int) ([]*domain.Call, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, organization_id, session_id, direction, peer_number, status, duration_seconds, started_at, connected_at, ended_at, end_reason, created_at
		FROM calls WHERE session_id = $1 ORDER BY started_at DESC LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, sessionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list calls by session: %w", err)
	}
	defer rows.Close()

	var calls []*domain.Call
	for rows.Next() {
		c := &domain.Call{}
		if err := rows.Scan(
			&c.ID, &c.OrganizationID, &c.SessionID, &c.Direction, &c.PeerNumber, &c.Status, &c.DurationSeconds, &c.StartedAt, &c.ConnectedAt, &c.EndedAt, &c.EndReason, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		calls = append(calls, c)
	}
	return calls, rows.Err()
}

func (r *callRepo) UpdateStatus(ctx context.Context, id, status string) error {
	var query string
	now := time.Now()
	if status == "active" {
		query = `UPDATE calls SET status = $1, connected_at = COALESCE(connected_at, $2) WHERE id = $3`
		_, err := r.db.ExecContext(ctx, query, status, now, id)
		return err
	}
	query = `UPDATE calls SET status = $1 WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, status, id)
	return err
}

func (r *callRepo) EndCall(ctx context.Context, id, status, endReason string, duration int, endedAt time.Time) error {
	if endedAt.IsZero() {
		endedAt = time.Now()
	}
	query := `
		UPDATE calls
		SET status = $1, end_reason = $2, duration_seconds = $3, ended_at = $4
		WHERE id = $5
	`
	_, err := r.db.ExecContext(ctx, query, status, endReason, duration, endedAt, id)
	if err != nil {
		return fmt.Errorf("end call record: %w", err)
	}
	return nil
}

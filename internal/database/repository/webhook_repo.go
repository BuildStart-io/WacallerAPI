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

type webhookRepo struct {
	db *database.DB
}

func NewWebhookRepository(db *database.DB) WebhookRepository {
	return &webhookRepo{db: db}
}

func (r *webhookRepo) CreateEndpoint(ctx context.Context, ep *domain.WebhookEndpoint) error {
	if ep.CreatedAt.IsZero() {
		ep.CreatedAt = time.Now()
	}
	if ep.UpdatedAt.IsZero() {
		ep.UpdatedAt = time.Now()
	}
	if ep.Status == "" {
		ep.Status = "active"
	}

	query := `
		INSERT INTO webhook_endpoints (id, organization_id, url, secret, events, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.ExecContext(ctx, query,
		ep.ID, ep.OrganizationID, ep.URL, ep.Secret, pq.Array(ep.Events), ep.Status, ep.CreatedAt, ep.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create webhook endpoint: %w", err)
	}
	return nil
}

func (r *webhookRepo) GetEndpointByID(ctx context.Context, id string) (*domain.WebhookEndpoint, error) {
	query := `
		SELECT id, organization_id, url, secret, events, status, created_at, updated_at
		FROM webhook_endpoints WHERE id = $1
	`
	ep := &domain.WebhookEndpoint{}
	var events pq.StringArray
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&ep.ID, &ep.OrganizationID, &ep.URL, &ep.Secret, &events, &ep.Status, &ep.CreatedAt, &ep.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get webhook endpoint by id: %w", err)
	}
	ep.Events = []string(events)
	return ep, nil
}

func (r *webhookRepo) ListEndpointsByOrg(ctx context.Context, orgID string) ([]*domain.WebhookEndpoint, error) {
	query := `
		SELECT id, organization_id, url, secret, events, status, created_at, updated_at
		FROM webhook_endpoints WHERE organization_id = $1 ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("list webhook endpoints by org: %w", err)
	}
	defer rows.Close()

	var list []*domain.WebhookEndpoint
	for rows.Next() {
		ep := &domain.WebhookEndpoint{}
		var events pq.StringArray
		if err := rows.Scan(
			&ep.ID, &ep.OrganizationID, &ep.URL, &ep.Secret, &events, &ep.Status, &ep.CreatedAt, &ep.UpdatedAt,
		); err != nil {
			return nil, err
		}
		ep.Events = []string(events)
		list = append(list, ep)
	}
	return list, rows.Err()
}

func (r *webhookRepo) UpdateEndpoint(ctx context.Context, ep *domain.WebhookEndpoint) error {
	ep.UpdatedAt = time.Now()
	query := `
		UPDATE webhook_endpoints
		SET url = $1, secret = $2, events = $3, status = $4, updated_at = $5
		WHERE id = $6
	`
	res, err := r.db.ExecContext(ctx, query, ep.URL, ep.Secret, pq.Array(ep.Events), ep.Status, ep.UpdatedAt, ep.ID)
	if err != nil {
		return fmt.Errorf("update webhook endpoint: %w", err)
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

func (r *webhookRepo) DeleteEndpoint(ctx context.Context, id string) error {
	query := `DELETE FROM webhook_endpoints WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete webhook endpoint: %w", err)
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

func (r *webhookRepo) CreateDelivery(ctx context.Context, del *domain.WebhookDelivery) error {
	if del.CreatedAt.IsZero() {
		del.CreatedAt = time.Now()
	}
	if del.Status == "" {
		del.Status = "pending"
	}
	if del.MaxAttempts <= 0 {
		del.MaxAttempts = 5
	}

	query := `
		INSERT INTO webhook_deliveries (id, endpoint_id, event_id, event_type, status, attempt_count, max_attempts, next_attempt_at, last_status_code, last_error, payload, created_at, delivered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := r.db.ExecContext(ctx, query,
		del.ID, del.EndpointID, del.EventID, del.EventType, del.Status, del.AttemptCount, del.MaxAttempts, del.NextAttemptAt, del.LastStatusCode, del.LastError, del.Payload, del.CreatedAt, del.DeliveredAt,
	)
	if err != nil {
		return fmt.Errorf("create webhook delivery: %w", err)
	}
	return nil
}

func (r *webhookRepo) UpdateDeliveryStatus(ctx context.Context, id, status string, statusCode int, lastErr string, nextAttempt *time.Time, deliveredAt *time.Time) error {
	query := `
		UPDATE webhook_deliveries
		SET status = $1, attempt_count = attempt_count + 1, last_status_code = $2, last_error = $3, next_attempt_at = $4, delivered_at = $5
		WHERE id = $6
	`
	_, err := r.db.ExecContext(ctx, query, status, statusCode, lastErr, nextAttempt, deliveredAt, id)
	if err != nil {
		return fmt.Errorf("update webhook delivery status: %w", err)
	}
	return nil
}

func (r *webhookRepo) ListPendingDeliveries(ctx context.Context, limit int) ([]*domain.WebhookDelivery, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT id, endpoint_id, event_id, event_type, status, attempt_count, max_attempts, next_attempt_at, last_status_code, last_error, payload, created_at, delivered_at
		FROM webhook_deliveries
		WHERE status IN ('pending', 'retry_wait') AND (next_attempt_at IS NULL OR next_attempt_at <= NOW())
		ORDER BY created_at ASC LIMIT $1
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending webhook deliveries: %w", err)
	}
	defer rows.Close()

	var list []*domain.WebhookDelivery
	for rows.Next() {
		del := &domain.WebhookDelivery{}
		if err := rows.Scan(
			&del.ID, &del.EndpointID, &del.EventID, &del.EventType, &del.Status, &del.AttemptCount, &del.MaxAttempts, &del.NextAttemptAt, &del.LastStatusCode, &del.LastError, &del.Payload, &del.CreatedAt, &del.DeliveredAt,
		); err != nil {
			return nil, err
		}
		list = append(list, del)
	}
	return list, rows.Err()
}

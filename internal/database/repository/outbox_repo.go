package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
	"wacallerapi/internal/database"
	"wacallerapi/internal/domain"
)

type outboxRepo struct {
	db *database.DB
}

func NewOutboxRepository(db *database.DB) OutboxRepository {
	return &outboxRepo{db: db}
}

func (r *outboxRepo) CreateEvent(ctx context.Context, evt *domain.OutboxEvent) error {
	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = time.Now()
	}
	if evt.SchemaVersion <= 0 {
		evt.SchemaVersion = 1
	}

	query := `
		INSERT INTO outbox_events (organization_id, aggregate_type, aggregate_id, event_type, schema_version, payload, published, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`
	err := r.db.QueryRowContext(ctx, query,
		evt.OrganizationID, evt.AggregateType, evt.AggregateID, evt.EventType, evt.SchemaVersion, evt.Payload, evt.Published, evt.OccurredAt,
	).Scan(&evt.ID)
	if err != nil {
		return fmt.Errorf("create outbox event: %w", err)
	}
	return nil
}

func (r *outboxRepo) ListUnpublished(ctx context.Context, limit int) ([]*domain.OutboxEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT id, organization_id, aggregate_type, aggregate_id, event_type, schema_version, payload, published, occurred_at, published_at
		FROM outbox_events
		WHERE published = FALSE
		ORDER BY occurred_at ASC LIMIT $1
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list unpublished outbox events: %w", err)
	}
	defer rows.Close()

	var list []*domain.OutboxEvent
	for rows.Next() {
		evt := &domain.OutboxEvent{}
		if err := rows.Scan(
			&evt.ID, &evt.OrganizationID, &evt.AggregateType, &evt.AggregateID, &evt.EventType, &evt.SchemaVersion, &evt.Payload, &evt.Published, &evt.OccurredAt, &evt.PublishedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, evt)
	}
	return list, rows.Err()
}

func (r *outboxRepo) MarkPublished(ctx context.Context, ids []int64, publishedAt time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	if publishedAt.IsZero() {
		publishedAt = time.Now()
	}

	query := `
		UPDATE outbox_events
		SET published = TRUE, published_at = $1
		WHERE id = ANY($2)
	`
	_, err := r.db.ExecContext(ctx, query, publishedAt, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("mark outbox events published: %w", err)
	}
	return nil
}

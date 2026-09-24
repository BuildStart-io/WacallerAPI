package repository

import (
	"context"
	"fmt"
	"time"

	"wacallerapi/internal/database"
	"wacallerapi/internal/domain"
)

type auditRepo struct {
	db *database.DB
}

func NewAuditRepository(db *database.DB) AuditRepository {
	return &auditRepo{db: db}
}

func (r *auditRepo) RecordEvent(ctx context.Context, evt *domain.AuditEvent) error {
	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = time.Now()
	}
	if evt.Result == "" {
		evt.Result = "success"
	}

	query := `
		INSERT INTO audit_events (actor_id, actor_type, organization_id, action, target_type, target_id, result, ip_address, user_agent, request_id, metadata, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`
	err := r.db.QueryRowContext(ctx, query,
		evt.ActorID, evt.ActorType, evt.OrganizationID, evt.Action, evt.TargetType, evt.TargetID, evt.Result, evt.IPAddress, evt.UserAgent, evt.RequestID, evt.Metadata, evt.OccurredAt,
	).Scan(&evt.ID)
	if err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}

func (r *auditRepo) ListByOrg(ctx context.Context, orgID string, limit, offset int) ([]*domain.AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, actor_id, actor_type, organization_id, action, target_type, target_id, result, ip_address, user_agent, request_id, metadata, occurred_at
		FROM audit_events WHERE organization_id = $1 ORDER BY occurred_at DESC LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, orgID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list audit events by org: %w", err)
	}
	defer rows.Close()

	var list []*domain.AuditEvent
	for rows.Next() {
		evt := &domain.AuditEvent{}
		if err := rows.Scan(
			&evt.ID, &evt.ActorID, &evt.ActorType, &evt.OrganizationID, &evt.Action, &evt.TargetType, &evt.TargetID, &evt.Result, &evt.IPAddress, &evt.UserAgent, &evt.RequestID, &evt.Metadata, &evt.OccurredAt,
		); err != nil {
			return nil, err
		}
		list = append(list, evt)
	}
	return list, rows.Err()
}

func (r *auditRepo) ListByActor(ctx context.Context, actorID string, limit, offset int) ([]*domain.AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, actor_id, actor_type, organization_id, action, target_type, target_id, result, ip_address, user_agent, request_id, metadata, occurred_at
		FROM audit_events WHERE actor_id = $1 ORDER BY occurred_at DESC LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, actorID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list audit events by actor: %w", err)
	}
	defer rows.Close()

	var list []*domain.AuditEvent
	for rows.Next() {
		evt := &domain.AuditEvent{}
		if err := rows.Scan(
			&evt.ID, &evt.ActorID, &evt.ActorType, &evt.OrganizationID, &evt.Action, &evt.TargetType, &evt.TargetID, &evt.Result, &evt.IPAddress, &evt.UserAgent, &evt.RequestID, &evt.Metadata, &evt.OccurredAt,
		); err != nil {
			return nil, err
		}
		list = append(list, evt)
	}
	return list, rows.Err()
}

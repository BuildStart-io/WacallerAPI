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

type subscriptionRepo struct {
	db *database.DB
}

func NewSubscriptionRepository(db *database.DB) SubscriptionRepository {
	return &subscriptionRepo{db: db}
}

func (r *subscriptionRepo) CreateOrUpdate(ctx context.Context, sub *domain.Subscription) error {
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = time.Now()
	}
	sub.UpdatedAt = time.Now()

	query := `
		INSERT INTO subscriptions (
			id, organization_id, plan, status, stripe_customer_id, stripe_subscription_id, stripe_session_id,
			amount_cents, currency, current_period_start, current_period_end, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO UPDATE SET
			plan = EXCLUDED.plan,
			status = EXCLUDED.status,
			stripe_customer_id = EXCLUDED.stripe_customer_id,
			stripe_subscription_id = EXCLUDED.stripe_subscription_id,
			stripe_session_id = EXCLUDED.stripe_session_id,
			amount_cents = EXCLUDED.amount_cents,
			currency = EXCLUDED.currency,
			current_period_start = EXCLUDED.current_period_start,
			current_period_end = EXCLUDED.current_period_end,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.ExecContext(ctx, query,
		sub.ID, sub.OrganizationID, sub.Plan, sub.Status, sub.StripeCustomerID, sub.StripeSubscriptionID, sub.StripeSessionID,
		sub.AmountCents, sub.Currency, sub.CurrentPeriodStart, sub.CurrentPeriodEnd, sub.CreatedAt, sub.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert subscription: %w", err)
	}
	return nil
}

func (r *subscriptionRepo) GetByOrg(ctx context.Context, orgID string) (*domain.Subscription, error) {
	query := `
		SELECT id, organization_id, plan, status, stripe_customer_id, stripe_subscription_id, stripe_session_id,
		       amount_cents, currency, current_period_start, current_period_end, created_at, updated_at
		FROM subscriptions WHERE organization_id = $1 ORDER BY created_at DESC LIMIT 1
	`
	sub := &domain.Subscription{}
	err := r.db.QueryRowContext(ctx, query, orgID).Scan(
		&sub.ID, &sub.OrganizationID, &sub.Plan, &sub.Status, &sub.StripeCustomerID, &sub.StripeSubscriptionID, &sub.StripeSessionID,
		&sub.AmountCents, &sub.Currency, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.CreatedAt, &sub.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get subscription by org: %w", err)
	}
	return sub, nil
}

func (r *subscriptionRepo) GetByStripeSubID(ctx context.Context, stripeSubID string) (*domain.Subscription, error) {
	query := `
		SELECT id, organization_id, plan, status, stripe_customer_id, stripe_subscription_id, stripe_session_id,
		       amount_cents, currency, current_period_start, current_period_end, created_at, updated_at
		FROM subscriptions WHERE stripe_subscription_id = $1
	`
	sub := &domain.Subscription{}
	err := r.db.QueryRowContext(ctx, query, stripeSubID).Scan(
		&sub.ID, &sub.OrganizationID, &sub.Plan, &sub.Status, &sub.StripeCustomerID, &sub.StripeSubscriptionID, &sub.StripeSessionID,
		&sub.AmountCents, &sub.Currency, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.CreatedAt, &sub.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get subscription by stripe subscription id: %w", err)
	}
	return sub, nil
}

func (r *subscriptionRepo) SetEntitlement(ctx context.Context, ent *domain.Entitlement) error {
	if ent.CreatedAt.IsZero() {
		ent.CreatedAt = time.Now()
	}
	if ent.Source == "" {
		ent.Source = "subscription"
	}

	query := `
		INSERT INTO entitlements (id, organization_id, feature, limit_value, source, override_reason, override_by, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (organization_id, feature, source) DO UPDATE SET
			limit_value = EXCLUDED.limit_value,
			override_reason = EXCLUDED.override_reason,
			override_by = EXCLUDED.override_by,
			expires_at = EXCLUDED.expires_at
	`
	_, err := r.db.ExecContext(ctx, query,
		ent.ID, ent.OrganizationID, ent.Feature, ent.LimitValue, ent.Source, ent.OverrideReason, ent.OverrideBy, ent.ExpiresAt, ent.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("set entitlement: %w", err)
	}
	return nil
}

func (r *subscriptionRepo) GetEntitlement(ctx context.Context, orgID, feature string) (*domain.Entitlement, error) {
	query := `
		SELECT id, organization_id, feature, limit_value, source, override_reason, override_by, expires_at, created_at
		FROM entitlements WHERE organization_id = $1 AND feature = $2
		ORDER BY CASE WHEN source = 'override' THEN 1 ELSE 2 END ASC LIMIT 1
	`
	ent := &domain.Entitlement{}
	err := r.db.QueryRowContext(ctx, query, orgID, feature).Scan(
		&ent.ID, &ent.OrganizationID, &ent.Feature, &ent.LimitValue, &ent.Source, &ent.OverrideReason, &ent.OverrideBy, &ent.ExpiresAt, &ent.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get entitlement: %w", err)
	}
	return ent, nil
}

func (r *subscriptionRepo) ListEntitlements(ctx context.Context, orgID string) ([]*domain.Entitlement, error) {
	query := `
		SELECT id, organization_id, feature, limit_value, source, override_reason, override_by, expires_at, created_at
		FROM entitlements WHERE organization_id = $1 ORDER BY feature ASC
	`
	rows, err := r.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("list entitlements: %w", err)
	}
	defer rows.Close()

	var list []*domain.Entitlement
	for rows.Next() {
		ent := &domain.Entitlement{}
		if err := rows.Scan(
			&ent.ID, &ent.OrganizationID, &ent.Feature, &ent.LimitValue, &ent.Source, &ent.OverrideReason, &ent.OverrideBy, &ent.ExpiresAt, &ent.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, ent)
	}
	return list, rows.Err()
}

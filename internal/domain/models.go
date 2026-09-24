package domain

import (
	"encoding/json"
	"time"
)

// Organization represents the primary tenant boundary.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"` // active, suspended, deleted
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// User represents a system user account.
type User struct {
	ID              string    `json:"id"`
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	Role            string    `json:"role"`             // developer, superadmin
	PasswordHash    string    `json:"-"`                // Never serialize password hash
	PasswordVersion int       `json:"password_version"` // 1 = legacy SHA-256, 2 = bcrypt
	EmailVerified   bool      `json:"email_verified"`
	Status          string    `json:"status"` // active, suspended, deleted
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Membership maps a user to an organization with a specific role.
type Membership struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	OrganizationID string    `json:"organization_id"`
	Role           string    `json:"role"` // owner, admin, developer, viewer
	CreatedAt      time.Time `json:"created_at"`
}

// Identity supports multiple auth providers per user (e.g., email, google).
type Identity struct {
	ID         string          `json:"id"`
	UserID     string          `json:"user_id"`
	Provider   string          `json:"provider"`    // email, google
	ProviderID string          `json:"provider_id"` // email address or OAuth sub
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// AuthSession represents a server-side web session.
type AuthSession struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	TokenHash string     `json:"-"`
	IPAddress string     `json:"ip_address,omitempty"`
	UserAgent string     `json:"user_agent,omitempty"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// APICredential represents a scoped API key bound to an organization.
type APICredential struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	Name           string     `json:"name"`
	KeyHash        string     `json:"-"`
	KeyPrefix      string     `json:"key_prefix"`
	Scopes         []string   `json:"scopes"`
	CreatedBy      string     `json:"created_by"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// WhatsAppSession holds control plane metadata for a WhatsApp session.
type WhatsAppSession struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	Name           string     `json:"name"`
	JID            string     `json:"jid,omitempty"`
	PhoneNumber    string     `json:"phone_number,omitempty"`
	Status         string     `json:"status"`        // connected, disconnected, qr_ready, connecting
	DesiredState   string     `json:"desired_state"` // connected, disconnected
	WorkerID       string     `json:"worker_id,omitempty"`
	LeaseVersion   int64      `json:"lease_version"`
	LastHeartbeat  *time.Time `json:"last_heartbeat,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	WebhookURL     string     `json:"webhook_url,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Call represents a WhatsApp voice/video call record.
type Call struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organization_id"`
	SessionID       string     `json:"session_id"`
	Direction       string     `json:"direction"` // inbound, outbound
	PeerNumber      string     `json:"peer_number"`
	Status          string     `json:"status"` // ringing, active, ended, failed
	DurationSeconds int        `json:"duration_seconds"`
	StartedAt       time.Time  `json:"started_at"`
	ConnectedAt     *time.Time `json:"connected_at,omitempty"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	EndReason       string     `json:"end_reason,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// Message represents a WhatsApp text/media message record.
type Message struct {
	ID             int64     `json:"id"`
	OrganizationID string    `json:"organization_id"`
	SessionID      string    `json:"session_id"`
	MessageID      string    `json:"message_id"`
	Direction      string    `json:"direction"` // inbound, outbound
	PeerNumber     string    `json:"peer_number"`
	MsgType        string    `json:"msg_type"` // text, audio, image, video, document
	Content        string    `json:"content,omitempty"`
	MediaURL       string    `json:"media_url,omitempty"`
	Status         string    `json:"status"` // sent, delivered, read, failed
	Timestamp      time.Time `json:"timestamp"`
}

// Subscription represents an organization's plan and billing status.
type Subscription struct {
	ID                   string     `json:"id"`
	OrganizationID       string     `json:"organization_id"`
	Plan                 string     `json:"plan"`   // basic, pro, enterprise
	Status               string     `json:"status"` // active, past_due, canceled
	StripeCustomerID     string     `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID string     `json:"stripe_subscription_id,omitempty"`
	StripeSessionID      string     `json:"stripe_session_id,omitempty"`
	AmountCents          int        `json:"amount_cents"`
	Currency             string     `json:"currency"`
	CurrentPeriodStart   *time.Time `json:"current_period_start,omitempty"`
	CurrentPeriodEnd     *time.Time `json:"current_period_end,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// Entitlement represents specific feature limits for an organization.
type Entitlement struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	Feature        string     `json:"feature"` // max_sessions, max_calls_per_day, etc.
	LimitValue     int        `json:"limit_value"`
	Source         string     `json:"source"` // subscription, override
	OverrideReason string     `json:"override_reason,omitempty"`
	OverrideBy     string     `json:"override_by,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// UsageRecord tracks feature consumption per organization for billing.
type UsageRecord struct {
	ID             int64     `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Metric         string    `json:"metric"` // call_seconds, messages_sent
	Count          int       `json:"count"`
	PeriodStart    time.Time `json:"period_start"`
	PeriodEnd      time.Time `json:"period_end"`
	RecordedAt     time.Time `json:"recorded_at"`
}

// WebhookEndpoint represents a registered webhook URL for an organization.
type WebhookEndpoint struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	URL            string    `json:"url"`
	Secret         string    `json:"secret"`
	Events         []string  `json:"events"`
	Status         string    `json:"status"` // active, disabled
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// WebhookDelivery tracks the delivery status of a webhook payload.
type WebhookDelivery struct {
	ID             string          `json:"id"`
	EndpointID     string          `json:"endpoint_id"`
	EventID        string          `json:"event_id"`
	EventType      string          `json:"event_type"`
	Status         string          `json:"status"` // pending, delivered, failed, retry_wait
	AttemptCount   int             `json:"attempt_count"`
	MaxAttempts    int             `json:"max_attempts"`
	NextAttemptAt  *time.Time      `json:"next_attempt_at,omitempty"`
	LastStatusCode int             `json:"last_status_code,omitempty"`
	LastError      string          `json:"last_error,omitempty"`
	Payload        json.RawMessage `json:"payload"`
	CreatedAt      time.Time       `json:"created_at"`
	DeliveredAt    *time.Time      `json:"delivered_at,omitempty"`
}

// OutboxEvent represents a transactional outbox entry for events.
type OutboxEvent struct {
	ID             int64           `json:"id"`
	OrganizationID string          `json:"organization_id"`
	AggregateType  string          `json:"aggregate_type"`
	AggregateID    string          `json:"aggregate_id"`
	EventType      string          `json:"event_type"`
	SchemaVersion  int             `json:"schema_version"`
	Payload        json.RawMessage `json:"payload"`
	Published      bool            `json:"published"`
	OccurredAt     time.Time       `json:"occurred_at"`
	PublishedAt    *time.Time      `json:"published_at,omitempty"`
}

// AuditEvent represents an immutable record of administrative or security actions.
type AuditEvent struct {
	ID             int64           `json:"id"`
	ActorID        string          `json:"actor_id,omitempty"`
	ActorType      string          `json:"actor_type"` // user, api_key, system
	OrganizationID string          `json:"organization_id,omitempty"`
	Action         string          `json:"action"`
	TargetType     string          `json:"target_type,omitempty"`
	TargetID       string          `json:"target_id,omitempty"`
	Result         string          `json:"result"` // success, failure
	IPAddress      string          `json:"ip_address,omitempty"`
	UserAgent      string          `json:"user_agent,omitempty"`
	RequestID      string          `json:"request_id,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	OccurredAt     time.Time       `json:"occurred_at"`
}

// FeatureFlag represents a dynamic system feature flag.
type FeatureFlag struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Enabled     bool      `json:"enabled"`
	Description string    `json:"description,omitempty"`
	Scope       string    `json:"scope"` // global, organization
	TargetOrgID string    `json:"target_org_id,omitempty"`
	UpdatedBy   string    `json:"updated_by,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// StripeEvent prevents duplicate processing of Stripe webhooks.
type StripeEvent struct {
	EventID     string    `json:"event_id"`
	EventType   string    `json:"event_type"`
	ProcessedAt time.Time `json:"processed_at"`
}

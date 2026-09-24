package repository

import (
	"context"
	"time"

	"wacallerapi/internal/domain"
)

type OrgRepository interface {
	Create(ctx context.Context, org *domain.Organization) error
	GetByID(ctx context.Context, id string) (*domain.Organization, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Organization, error)
	Update(ctx context.Context, org *domain.Organization) error
	List(ctx context.Context, limit, offset int) ([]*domain.Organization, error)
	Delete(ctx context.Context, id string) error
}

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	List(ctx context.Context, limit, offset int) ([]*domain.User, error)
}

type MembershipRepository interface {
	Create(ctx context.Context, m *domain.Membership) error
	GetByID(ctx context.Context, id string) (*domain.Membership, error)
	GetByUserAndOrg(ctx context.Context, userID, orgID string) (*domain.Membership, error)
	ListByOrg(ctx context.Context, orgID string) ([]*domain.Membership, error)
	ListByUser(ctx context.Context, userID string) ([]*domain.Membership, error)
	UpdateRole(ctx context.Context, id, role string) error
	Delete(ctx context.Context, id string) error
}

type AuthRepository interface {
	CreateIdentity(ctx context.Context, identity *domain.Identity) error
	GetIdentityByProvider(ctx context.Context, provider, providerID string) (*domain.Identity, error)
	CreateSession(ctx context.Context, session *domain.AuthSession) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*domain.AuthSession, error)
	RevokeSession(ctx context.Context, id string) error
	RevokeUserSessions(ctx context.Context, userID string) error
}

type CredentialRepository interface {
	Create(ctx context.Context, cred *domain.APICredential) error
	GetByHash(ctx context.Context, keyHash string) (*domain.APICredential, error)
	GetByID(ctx context.Context, id string) (*domain.APICredential, error)
	ListByOrg(ctx context.Context, orgID string) ([]*domain.APICredential, error)
	Revoke(ctx context.Context, id string) error
	UpdateLastUsed(ctx context.Context, id string, t time.Time) error
}

type SessionRepository interface {
	Create(ctx context.Context, sess *domain.WhatsAppSession) error
	GetByID(ctx context.Context, id string) (*domain.WhatsAppSession, error)
	ListByOrg(ctx context.Context, orgID string) ([]*domain.WhatsAppSession, error)
	ListAll(ctx context.Context) ([]*domain.WhatsAppSession, error)
	UpdateStatus(ctx context.Context, id, status, jid, phoneNumber, lastError string) error
	UpdateHeartbeat(ctx context.Context, id, workerID string, leaseVersion int64) error
	Delete(ctx context.Context, id string) error
}

type CallRepository interface {
	Create(ctx context.Context, call *domain.Call) error
	GetByID(ctx context.Context, id string) (*domain.Call, error)
	ListByOrg(ctx context.Context, orgID string, limit, offset int) ([]*domain.Call, error)
	ListBySession(ctx context.Context, sessionID string, limit, offset int) ([]*domain.Call, error)
	UpdateStatus(ctx context.Context, id, status string) error
	EndCall(ctx context.Context, id, status, endReason string, duration int, endedAt time.Time) error
}

type MessageRepository interface {
	Save(ctx context.Context, msg *domain.Message) error
	GetByID(ctx context.Context, id int64) (*domain.Message, error)
	ListByOrg(ctx context.Context, orgID string, limit, offset int) ([]*domain.Message, error)
	ListBySession(ctx context.Context, sessionID string, limit, offset int) ([]*domain.Message, error)
}

type SubscriptionRepository interface {
	CreateOrUpdate(ctx context.Context, sub *domain.Subscription) error
	GetByOrg(ctx context.Context, orgID string) (*domain.Subscription, error)
	GetByStripeSubID(ctx context.Context, stripeSubID string) (*domain.Subscription, error)
	SetEntitlement(ctx context.Context, ent *domain.Entitlement) error
	GetEntitlement(ctx context.Context, orgID, feature string) (*domain.Entitlement, error)
	ListEntitlements(ctx context.Context, orgID string) ([]*domain.Entitlement, error)
}

type WebhookRepository interface {
	CreateEndpoint(ctx context.Context, ep *domain.WebhookEndpoint) error
	GetEndpointByID(ctx context.Context, id string) (*domain.WebhookEndpoint, error)
	ListEndpointsByOrg(ctx context.Context, orgID string) ([]*domain.WebhookEndpoint, error)
	UpdateEndpoint(ctx context.Context, ep *domain.WebhookEndpoint) error
	DeleteEndpoint(ctx context.Context, id string) error
	CreateDelivery(ctx context.Context, del *domain.WebhookDelivery) error
	UpdateDeliveryStatus(ctx context.Context, id, status string, statusCode int, lastErr string, nextAttempt *time.Time, deliveredAt *time.Time) error
	ListPendingDeliveries(ctx context.Context, limit int) ([]*domain.WebhookDelivery, error)
}

type OutboxRepository interface {
	CreateEvent(ctx context.Context, evt *domain.OutboxEvent) error
	ListUnpublished(ctx context.Context, limit int) ([]*domain.OutboxEvent, error)
	MarkPublished(ctx context.Context, ids []int64, publishedAt time.Time) error
}

type AuditRepository interface {
	RecordEvent(ctx context.Context, evt *domain.AuditEvent) error
	ListByOrg(ctx context.Context, orgID string, limit, offset int) ([]*domain.AuditEvent, error)
	ListByActor(ctx context.Context, actorID string, limit, offset int) ([]*domain.AuditEvent, error)
}

type FeatureRepository interface {
	GetFlag(ctx context.Context, name string, orgID string) (*domain.FeatureFlag, error)
	SetFlag(ctx context.Context, flag *domain.FeatureFlag) error
	ListFlags(ctx context.Context) ([]*domain.FeatureFlag, error)
}

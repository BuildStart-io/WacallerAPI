package repository

import (
	"wacallerapi/internal/database"
)

// Repositories aggregates all domain repositories for clean dependency injection.
type Repositories struct {
	Orgs          OrgRepository
	Users         UserRepository
	Memberships   MembershipRepository
	Auth          AuthRepository
	Credentials   CredentialRepository
	Sessions      SessionRepository
	Calls         CallRepository
	Messages      MessageRepository
	Subscriptions SubscriptionRepository
	Webhooks      WebhookRepository
	Outbox        OutboxRepository
	Audit         AuditRepository
	Features      FeatureRepository
}

// NewRepositories constructs all repositories using the provided PostgreSQL database instance.
func NewRepositories(db *database.DB) *Repositories {
	return &Repositories{
		Orgs:          NewOrgRepository(db),
		Users:         NewUserRepository(db),
		Memberships:   NewMembershipRepository(db),
		Auth:          NewAuthRepository(db),
		Credentials:   NewCredentialRepository(db),
		Sessions:      NewSessionRepository(db),
		Calls:         NewCallRepository(db),
		Messages:      NewMessageRepository(db),
		Subscriptions: NewSubscriptionRepository(db),
		Webhooks:      NewWebhookRepository(db),
		Outbox:        NewOutboxRepository(db),
		Audit:         NewAuditRepository(db),
		Features:      NewFeatureRepository(db),
	}
}

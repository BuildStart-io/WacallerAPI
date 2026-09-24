package service

import (
	"wacallerapi/internal/database/repository"
)

// Services aggregates all application services for simple dependency injection.
type Services struct {
	Audit   *AuditService
	User    *UserService
	Org     *OrgService
	Auth    *AuthService
	Session *SessionService
	Call    *CallService
	Message *MessageService
	Billing *BillingService
	Webhook *WebhookService
}

// NewServices constructs all application services backed by the provided repositories.
func NewServices(repos *repository.Repositories) *Services {
	audit := NewAuditService(repos.Audit)
	user := NewUserService(repos.Users, audit)
	org := NewOrgService(repos.Orgs, repos.Memberships, audit)
	auth := NewAuthService(repos.Users, repos.Auth, repos.Credentials, audit)
	session := NewSessionService(repos.Sessions, audit)
	call := NewCallService(repos.Calls)
	msg := NewMessageService(repos.Messages)
	billing := NewBillingService(repos.Subscriptions)
	webhook := NewWebhookService(repos.Webhooks)

	return &Services{
		Audit:   audit,
		User:    user,
		Org:     org,
		Auth:    auth,
		Session: session,
		Call:    call,
		Message: msg,
		Billing: billing,
		Webhook: webhook,
	}
}

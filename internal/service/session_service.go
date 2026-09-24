package service

import (
	"context"

	"github.com/google/uuid"
	"wacallerapi/internal/database/repository"
	"wacallerapi/internal/domain"
)

type SessionService struct {
	sessionRepo repository.SessionRepository
	audit       *AuditService
}

func NewSessionService(sessionRepo repository.SessionRepository, audit *AuditService) *SessionService {
	return &SessionService{
		sessionRepo: sessionRepo,
		audit:       audit,
	}
}

func (s *SessionService) CreateSession(ctx context.Context, actorID, orgID, name, webhookURL string) (*domain.WhatsAppSession, error) {
	sessionID := "was_" + uuid.New().String()
	sess := &domain.WhatsAppSession{
		ID:             sessionID,
		OrganizationID: orgID,
		Name:           name,
		Status:         "disconnected",
		DesiredState:   "connected",
		WebhookURL:     webhookURL,
	}

	if err := s.sessionRepo.Create(ctx, sess); err != nil {
		return nil, err
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, actorID, "user", orgID, "whatsapp.session.create", "whatsapp_session", sessionID, "success", "", "", "", nil)
	}

	return sess, nil
}

func (s *SessionService) GetByID(ctx context.Context, id string) (*domain.WhatsAppSession, error) {
	return s.sessionRepo.GetByID(ctx, id)
}

func (s *SessionService) ListByOrg(ctx context.Context, orgID string) ([]*domain.WhatsAppSession, error) {
	return s.sessionRepo.ListByOrg(ctx, orgID)
}

func (s *SessionService) ListAll(ctx context.Context) ([]*domain.WhatsAppSession, error) {
	return s.sessionRepo.ListAll(ctx)
}

func (s *SessionService) UpdateStatus(ctx context.Context, id, status, jid, phoneNumber, lastError string) error {
	return s.sessionRepo.UpdateStatus(ctx, id, status, jid, phoneNumber, lastError)
}

func (s *SessionService) DeleteSession(ctx context.Context, actorID, id string) error {
	sess, err := s.sessionRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.sessionRepo.Delete(ctx, id); err != nil {
		return err
	}

	if s.audit != nil {
		_ = s.audit.Log(ctx, actorID, "user", sess.OrganizationID, "whatsapp.session.delete", "whatsapp_session", id, "success", "", "", "", nil)
	}

	return nil
}

package service

import (
	"context"

	"github.com/google/uuid"
	"wacallerapi/internal/database/repository"
	"wacallerapi/internal/domain"
)

type WebhookService struct {
	webhookRepo repository.WebhookRepository
}

func NewWebhookService(webhookRepo repository.WebhookRepository) *WebhookService {
	return &WebhookService{webhookRepo: webhookRepo}
}

func (s *WebhookService) RegisterEndpoint(ctx context.Context, orgID, url, secret string, events []string) (*domain.WebhookEndpoint, error) {
	if url == "" {
		return nil, domain.ErrInvalidInput
	}

	epID := "wep_" + uuid.New().String()
	if secret == "" {
		secret = "whsec_" + uuid.New().String()
	}

	ep := &domain.WebhookEndpoint{
		ID:             epID,
		OrganizationID: orgID,
		URL:            url,
		Secret:         secret,
		Events:         events,
		Status:         "active",
	}

	if err := s.webhookRepo.CreateEndpoint(ctx, ep); err != nil {
		return nil, err
	}

	return ep, nil
}

func (s *WebhookService) ListEndpoints(ctx context.Context, orgID string) ([]*domain.WebhookEndpoint, error) {
	return s.webhookRepo.ListEndpointsByOrg(ctx, orgID)
}

func (s *WebhookService) DeleteEndpoint(ctx context.Context, id string) error {
	return s.webhookRepo.DeleteEndpoint(ctx, id)
}

package service

import (
	"context"
	"time"

	"wacallerapi/internal/database/repository"
	"wacallerapi/internal/domain"
)

type CallService struct {
	callRepo repository.CallRepository
}

func NewCallService(callRepo repository.CallRepository) *CallService {
	return &CallService{callRepo: callRepo}
}

func (s *CallService) RecordCallStart(ctx context.Context, id, orgID, sessionID, direction, peerNumber string) (*domain.Call, error) {
	call := &domain.Call{
		ID:             id,
		OrganizationID: orgID,
		SessionID:      sessionID,
		Direction:      direction,
		PeerNumber:     peerNumber,
		Status:         "ringing",
		StartedAt:      time.Now(),
	}

	if err := s.callRepo.Create(ctx, call); err != nil {
		return nil, err
	}

	return call, nil
}

func (s *CallService) UpdateCallStatus(ctx context.Context, id, status string) error {
	return s.callRepo.UpdateStatus(ctx, id, status)
}

func (s *CallService) EndCall(ctx context.Context, id, status, endReason string, duration int) error {
	return s.callRepo.EndCall(ctx, id, status, endReason, duration, time.Now())
}

func (s *CallService) ListByOrg(ctx context.Context, orgID string, limit, offset int) ([]*domain.Call, error) {
	return s.callRepo.ListByOrg(ctx, orgID, limit, offset)
}

func (s *CallService) ListBySession(ctx context.Context, sessionID string, limit, offset int) ([]*domain.Call, error) {
	return s.callRepo.ListBySession(ctx, sessionID, limit, offset)
}

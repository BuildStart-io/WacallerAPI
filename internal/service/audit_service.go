package service

import (
	"context"
	"encoding/json"
	"time"

	"wacallerapi/internal/database/repository"
	"wacallerapi/internal/domain"
)

type AuditService struct {
	auditRepo repository.AuditRepository
}

func NewAuditService(auditRepo repository.AuditRepository) *AuditService {
	return &AuditService{auditRepo: auditRepo}
}

func (s *AuditService) Log(ctx context.Context, actorID, actorType, orgID, action, targetType, targetID, result, ip, userAgent, requestID string, metadata map[string]any) error {
	var metaJSON json.RawMessage
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			metaJSON = b
		}
	}

	evt := &domain.AuditEvent{
		ActorID:        actorID,
		ActorType:      actorType,
		OrganizationID: orgID,
		Action:         action,
		TargetType:     targetType,
		TargetID:       targetID,
		Result:         result,
		IPAddress:      ip,
		UserAgent:      userAgent,
		RequestID:      requestID,
		Metadata:       metaJSON,
		OccurredAt:     time.Now(),
	}

	return s.auditRepo.RecordEvent(ctx, evt)
}

func (s *AuditService) ListByOrg(ctx context.Context, orgID string, limit, offset int) ([]*domain.AuditEvent, error) {
	return s.auditRepo.ListByOrg(ctx, orgID, limit, offset)
}

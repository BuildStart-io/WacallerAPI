package service

import (
	"context"
	"time"

	"wacallerapi/internal/database/repository"
	"wacallerapi/internal/domain"
)

type MessageService struct {
	msgRepo repository.MessageRepository
}

func NewMessageService(msgRepo repository.MessageRepository) *MessageService {
	return &MessageService{msgRepo: msgRepo}
}

func (s *MessageService) RecordMessage(ctx context.Context, orgID, sessionID, msgID, direction, peerNumber, msgType, content, mediaURL, status string) (*domain.Message, error) {
	msg := &domain.Message{
		OrganizationID: orgID,
		SessionID:      sessionID,
		MessageID:      msgID,
		Direction:      direction,
		PeerNumber:     peerNumber,
		MsgType:        msgType,
		Content:        content,
		MediaURL:       mediaURL,
		Status:         status,
		Timestamp:      time.Now(),
	}

	if err := s.msgRepo.Save(ctx, msg); err != nil {
		return nil, err
	}

	return msg, nil
}

func (s *MessageService) ListByOrg(ctx context.Context, orgID string, limit, offset int) ([]*domain.Message, error) {
	return s.msgRepo.ListByOrg(ctx, orgID, limit, offset)
}

func (s *MessageService) ListBySession(ctx context.Context, sessionID string, limit, offset int) ([]*domain.Message, error) {
	return s.msgRepo.ListBySession(ctx, sessionID, limit, offset)
}

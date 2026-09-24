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

type messageRepo struct {
	db *database.DB
}

func NewMessageRepository(db *database.DB) MessageRepository {
	return &messageRepo{db: db}
}

func (r *messageRepo) Save(ctx context.Context, msg *domain.Message) error {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	query := `
		INSERT INTO messages (organization_id, session_id, message_id, direction, peer_number, msg_type, content, media_url, status, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`
	err := r.db.QueryRowContext(ctx, query,
		msg.OrganizationID, msg.SessionID, msg.MessageID, msg.Direction, msg.PeerNumber, msg.MsgType, msg.Content, msg.MediaURL, msg.Status, msg.Timestamp,
	).Scan(&msg.ID)
	if err != nil {
		return fmt.Errorf("save message record: %w", err)
	}
	return nil
}

func (r *messageRepo) GetByID(ctx context.Context, id int64) (*domain.Message, error) {
	query := `
		SELECT id, organization_id, session_id, message_id, direction, peer_number, msg_type, content, media_url, status, timestamp
		FROM messages WHERE id = $1
	`
	m := &domain.Message{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&m.ID, &m.OrganizationID, &m.SessionID, &m.MessageID, &m.Direction, &m.PeerNumber, &m.MsgType, &m.Content, &m.MediaURL, &m.Status, &m.Timestamp,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message record by id: %w", err)
	}
	return m, nil
}

func (r *messageRepo) ListByOrg(ctx context.Context, orgID string, limit, offset int) ([]*domain.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, organization_id, session_id, message_id, direction, peer_number, msg_type, content, media_url, status, timestamp
		FROM messages WHERE organization_id = $1 ORDER BY timestamp DESC LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, orgID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list messages by org: %w", err)
	}
	defer rows.Close()

	var messages []*domain.Message
	for rows.Next() {
		m := &domain.Message{}
		if err := rows.Scan(
			&m.ID, &m.OrganizationID, &m.SessionID, &m.MessageID, &m.Direction, &m.PeerNumber, &m.MsgType, &m.Content, &m.MediaURL, &m.Status, &m.Timestamp,
		); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (r *messageRepo) ListBySession(ctx context.Context, sessionID string, limit, offset int) ([]*domain.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, organization_id, session_id, message_id, direction, peer_number, msg_type, content, media_url, status, timestamp
		FROM messages WHERE session_id = $1 ORDER BY timestamp DESC LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, sessionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list messages by session: %w", err)
	}
	defer rows.Close()

	var messages []*domain.Message
	for rows.Next() {
		m := &domain.Message{}
		if err := rows.Scan(
			&m.ID, &m.OrganizationID, &m.SessionID, &m.MessageID, &m.Direction, &m.PeerNumber, &m.MsgType, &m.Content, &m.MediaURL, &m.Status, &m.Timestamp,
		); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

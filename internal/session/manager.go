package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"wacallerapi/internal/store"
	"wacallerapi/internal/webhook"

	"go.mau.fi/whatsmeow"
	waStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type SessionManager struct {
	appCtx     context.Context
	container  *sqlstore.Container
	store      *store.Store
	dispatcher *webhook.Dispatcher
	log        *slog.Logger
	maxCalls   int

	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewSessionManager(ctx context.Context, container *sqlstore.Container, st *store.Store, disp *webhook.Dispatcher, log *slog.Logger, maxCalls int) *SessionManager {
	return &SessionManager{
		appCtx:     ctx,
		container:  container,
		store:      st,
		dispatcher: disp,
		log:        log,
		maxCalls:   maxCalls,
		sessions:   make(map[string]*Session),
	}
}

func (m *SessionManager) CreateSession(ctx context.Context, name, webhookURL, userID string) (*Session, error) {
	id := newSessionID()
	if name == "" {
		name = "WhatsApp Account"
	}

	apiKey := "wac_sess_" + newSessionID()
	device := m.container.NewDevice()
	client := whatsmeow.NewClient(device, waLog.Stdout("WA", "INFO", true))

	sess := newSession(id, name, apiKey, client, m.store, m.dispatcher, m.log, m.maxCalls, webhookURL, userID)

	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	if err := m.store.UpsertSession(ctx, id, name, "", string(StatusDisconnected), webhookURL, apiKey, userID); err != nil {
		return nil, err
	}

	if err := sess.Connect(ctx); err != nil {
		m.log.Warn("failed to connect session immediately", "id", id, "err", err)
	}

	return sess, nil
}

func (m *SessionManager) GetSession(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *SessionManager) GetSessionByAPIKey(ctx context.Context, apiKey string) (*Session, bool) {
	m.mu.RLock()
	for _, s := range m.sessions {
		if s.apiKey == apiKey {
			m.mu.RUnlock()
			return s, true
		}
	}
	m.mu.RUnlock()

	// Check database
	rec, err := m.store.GetSessionByAPIKey(ctx, apiKey)
	if err == nil && rec != nil {
		return m.GetSession(rec.ID)
	}
	return nil, false
}

func (m *SessionManager) FirstConnectedSession() (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		if s.status == StatusConnected {
			return s, true
		}
	}
	for _, s := range m.sessions {
		return s, true
	}
	return nil, false
}

func (m *SessionManager) ListSessions() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s.Info())
	}
	return out
}

func (m *SessionManager) ListSessionsByUser(userID string) []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]SessionInfo, 0)
	for _, s := range m.sessions {
		if s.userID == userID || (userID == "" && s.userID == "") {
			out = append(out, s.Info())
		}
	}
	return out
}

func (m *SessionManager) DeleteSession(ctx context.Context, id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	if !ok {
		return errors.New("session not found")
	}

	_ = s.Logout(ctx)
	if s.client != nil && s.client.Store != nil {
		_ = s.client.Store.Delete(ctx)
	}

	return m.store.DeleteSession(ctx, id)
}

func (m *SessionManager) Restore(ctx context.Context) error {
	records, err := m.store.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("failed to list sessions for restore: %w", err)
	}

	devices, err := m.container.GetAllDevices(ctx)
	if err != nil {
		return fmt.Errorf("failed to get whatsmeow devices: %w", err)
	}

	deviceMap := make(map[string]*types.JID)
	for _, d := range devices {
		if d.ID != nil {
			deviceMap[d.ID.String()] = d.ID
		}
	}

	for _, rec := range records {
		var device *types.JID
		if rec.JID != "" {
			device = deviceMap[rec.JID]
		}

		var waDevice *waStore.Device
		if device != nil {
			waDevice, err = m.container.GetDevice(ctx, *device)
			if err != nil {
				m.log.Warn("failed to load whatsmeow device", "jid", rec.JID, "err", err)
				continue
			}
		} else {
			waDevice = m.container.NewDevice()
		}

		apiKey := rec.APIKey
		if apiKey == "" {
			apiKey = "wac_sess_" + newSessionID()
			_ = m.store.UpsertSession(ctx, rec.ID, rec.Name, rec.JID, rec.Status, rec.WebhookURL, apiKey, rec.UserID)
		}

		client := whatsmeow.NewClient(waDevice, waLog.Stdout("WA", "INFO", true))
		sess := newSession(rec.ID, rec.Name, apiKey, client, m.store, m.dispatcher, m.log, m.maxCalls, rec.WebhookURL, rec.UserID)

		m.mu.Lock()
		m.sessions[rec.ID] = sess
		m.mu.Unlock()

		if waDevice.ID != nil {
			go func(s *Session) {
				if err := s.Connect(ctx); err != nil {
					m.log.Warn("failed to reconnect existing session", "id", s.id, "err", err)
				}
			}(sess)
		}
	}

	return nil
}

func (m *SessionManager) DisconnectAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, s := range m.sessions {
		s.Disconnect()
	}
}

func newSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

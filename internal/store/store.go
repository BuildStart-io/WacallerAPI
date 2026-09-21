package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Role         string    `json:"role"` // "superadmin" or "developer"
	PasswordHash string    `json:"-"`
	PATToken     string    `json:"pat_token"`
	TrialEndsAt  time.Time `json:"trial_ends_at"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserWithSubscription struct {
	ID          string              `json:"id"`
	Email       string              `json:"email"`
	Name        string              `json:"name"`
	Role        string              `json:"role"`
	PATToken    string              `json:"pat_token"`
	TrialEndsAt time.Time           `json:"trial_ends_at"`
	IsTrial     bool                `json:"is_trial"`
	DaysLeft    int                 `json:"days_left"`
	CreatedAt   time.Time           `json:"created_at"`
	Sub         *SubscriptionRecord `json:"subscription,omitempty"`
}

type APIKey struct {
	ID         string     `json:"id"`
	Key        string     `json:"key,omitempty"` // only shown on creation
	KeyHash    string     `json:"-"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	Enabled    bool       `json:"enabled"`
}

type SessionRecord struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	JID        string    `json:"jid"`
	Status     string    `json:"status"`
	WebhookURL string    `json:"webhook_url,omitempty"`
	APIKey     string    `json:"api_key"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type CallRecord struct {
	CallID          string     `json:"call_id"`
	SessionID       string     `json:"session_id"`
	Direction       string     `json:"direction"` // "outbound" or "inbound"
	PeerNumber      string     `json:"peer_number"`
	Status          string     `json:"status"` // "ringing", "active", "completed", "failed", "rejected"
	DurationSeconds int        `json:"duration_seconds"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	Reason          string     `json:"reason,omitempty"`
}

type MessageRecord struct {
	ID         int64     `json:"id"`
	SessionID  string    `json:"session_id"`
	MessageID  string    `json:"message_id"`
	Direction  string    `json:"direction"` // "outbound" or "inbound"
	PeerNumber string    `json:"peer_number"`
	MsgType    string    `json:"type"` // "text", "image", "audio", "video", "document", "location"
	Content    string    `json:"content,omitempty"`
	MediaURL   string    `json:"media_url,omitempty"`
	Status     string    `json:"status"` // "pending", "sent", "delivered", "read", "failed"
	Timestamp  time.Time `json:"timestamp"`
}

type WebhookLogRecord struct {
	ID         int64     `json:"id"`
	SessionID  string    `json:"session_id,omitempty"`
	Event      string    `json:"event"`
	TargetURL  string    `json:"target_url"`
	StatusCode int       `json:"status_code"`
	Payload    string    `json:"payload"`
	Error      string    `json:"error,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path))
	if err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) migrate(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			pat_token TEXT UNIQUE NOT NULL,
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_users_pat ON users(pat_token);`,
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			key_hash TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			last_used_at DATETIME,
			enabled INTEGER NOT NULL DEFAULT 1
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			jid TEXT,
			status TEXT NOT NULL DEFAULT 'DISCONNECTED',
			webhook_url TEXT,
			api_key TEXT UNIQUE,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_api_key ON sessions(api_key);`,
		`CREATE TABLE IF NOT EXISTS calls (
			call_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			direction TEXT NOT NULL,
			peer_number TEXT NOT NULL,
			status TEXT NOT NULL,
			duration_seconds INTEGER NOT NULL DEFAULT 0,
			started_at DATETIME NOT NULL,
			ended_at DATETIME,
			reason TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_calls_session ON calls(session_id, started_at DESC);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			message_id TEXT NOT NULL,
			direction TEXT NOT NULL,
			peer_number TEXT NOT NULL,
			msg_type TEXT NOT NULL,
			content TEXT,
			media_url TEXT,
			status TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, timestamp DESC);`,
		`CREATE TABLE IF NOT EXISTS webhook_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			event TEXT NOT NULL,
			target_url TEXT NOT NULL,
			status_code INTEGER NOT NULL,
			payload TEXT NOT NULL,
			error TEXT,
			timestamp DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_logs_timestamp ON webhook_logs(timestamp DESC);`,
		`CREATE TABLE IF NOT EXISTS subscriptions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			plan TEXT NOT NULL DEFAULT 'basic',
			status TEXT NOT NULL DEFAULT 'active',
			max_sessions INTEGER NOT NULL DEFAULT 1,
			stripe_customer_id TEXT,
			stripe_subscription_id TEXT,
			stripe_session_id TEXT,
			amount_cents INTEGER NOT NULL DEFAULT 600,
			currency TEXT NOT NULL DEFAULT 'usd',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_subscriptions_user ON subscriptions(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_subscriptions_stripe ON subscriptions(stripe_subscription_id);`,
	}

	for _, q := range queries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			// If index on api_key failed because column doesn't exist yet, alter table and retry
			if strings.Contains(err.Error(), "no such column: api_key") {
				_, _ = s.db.ExecContext(ctx, "ALTER TABLE sessions ADD COLUMN api_key TEXT;")
				if _, retryErr := s.db.ExecContext(ctx, q); retryErr != nil {
					return fmt.Errorf("migration query failed after alter: %w", retryErr)
				}
				continue
			}
			return fmt.Errorf("migration query failed: %w", err)
		}
	}
	_, _ = s.db.ExecContext(ctx, "ALTER TABLE sessions ADD COLUMN api_key TEXT;")
	_, _ = s.db.ExecContext(ctx, "ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'developer';")
	_, _ = s.db.ExecContext(ctx, "ALTER TABLE users ADD COLUMN trial_ends_at DATETIME;")

	// Seed default admin developer account if none exists
	var userCount int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&userCount)
	if userCount == 0 {
		defaultPAT := "wac_pat_demo_master_token"
		now := time.Now().UTC()
		trialEnd := now.Add(365 * 24 * time.Hour)
		_, _ = s.db.ExecContext(ctx,
			`INSERT INTO users (id, email, name, role, password_hash, pat_token, trial_ends_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			"usr_admin_yasiru", "yasirubandaraprivate@gmail.com", "Yasiru Bandara Private", "superadmin", hashKey("password123"), defaultPAT, trialEnd, now,
		)
		_ = s.CreateOrUpdateSubscription(ctx, &SubscriptionRecord{
			ID:          "sub_admin_master",
			UserID:      "usr_admin_yasiru",
			Plan:        "business",
			Status:      "active",
			MaxSessions: 100,
			AmountCents: 4900,
			Currency:    "usd",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	return nil
}

// ----------------- Developer Users & PATs -----------------

func (s *Store) CreateUser(ctx context.Context, name, email, rawPassword string) (*User, error) {
	rawBytes := make([]byte, 20)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, err
	}
	userID := "usr_" + hex.EncodeToString(rawBytes[:8])
	patToken := "wac_pat_" + hex.EncodeToString(rawBytes)
	pwdHash := hashKey(rawPassword)
	now := time.Now().UTC()
	trialEndsAt := now.Add(7 * 24 * time.Hour) // 7-day free trial like WasenderAPI
	role := "developer"

	if strings.Contains(strings.ToLower(email), "admin") || strings.EqualFold(email, "yasirubandaraprivate@gmail.com") {
		role = "superadmin"
		trialEndsAt = now.Add(365 * 24 * time.Hour)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, name, role, password_hash, pat_token, trial_ends_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, email, name, role, pwdHash, patToken, trialEndsAt, now,
	)
	if err != nil {
		return nil, err
	}

	plan := "basic"
	status := "trialing"
	if role == "superadmin" {
		plan = "business"
		status = "active"
	}
	_ = s.CreateOrUpdateSubscription(ctx, &SubscriptionRecord{
		UserID:      userID,
		Plan:        plan,
		Status:      status,
		MaxSessions: 1,
		AmountCents: 600,
		Currency:    "usd",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	return &User{
		ID:          userID,
		Email:       email,
		Name:        name,
		Role:        role,
		PATToken:    patToken,
		TrialEndsAt: trialEndsAt,
		CreatedAt:   now,
	}, nil
}

func (s *Store) AuthenticateUser(ctx context.Context, email, rawPassword string) (*User, error) {
	pwdHash := hashKey(rawPassword)
	var u User
	var role sql.NullString
	var trialEnds sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, COALESCE(role, 'developer'), pat_token, trial_ends_at, created_at FROM users WHERE email = ? AND password_hash = ?`,
		email, pwdHash,
	).Scan(&u.ID, &u.Email, &u.Name, &role, &u.PATToken, &trialEnds, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}
	if role.Valid && role.String != "" {
		u.Role = role.String
	} else {
		u.Role = "developer"
	}
	if trialEnds.Valid {
		u.TrialEndsAt = trialEnds.Time
	} else {
		u.TrialEndsAt = u.CreatedAt.Add(7 * 24 * time.Hour)
	}
	return &u, nil
}

func (s *Store) GetUserByPAT(ctx context.Context, pat string) (*User, error) {
	var u User
	var role sql.NullString
	var trialEnds sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, COALESCE(role, 'developer'), pat_token, trial_ends_at, created_at FROM users WHERE pat_token = ?`,
		pat,
	).Scan(&u.ID, &u.Email, &u.Name, &role, &u.PATToken, &trialEnds, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if role.Valid && role.String != "" {
		u.Role = role.String
	} else {
		u.Role = "developer"
	}
	if trialEnds.Valid {
		u.TrialEndsAt = trialEnds.Time
	} else {
		u.TrialEndsAt = u.CreatedAt.Add(7 * 24 * time.Hour)
	}
	return &u, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	var role sql.NullString
	var trialEnds sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, COALESCE(role, 'developer'), pat_token, trial_ends_at, created_at FROM users WHERE email = ?`,
		email,
	).Scan(&u.ID, &u.Email, &u.Name, &role, &u.PATToken, &trialEnds, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if role.Valid && role.String != "" {
		u.Role = role.String
	} else {
		u.Role = "developer"
	}
	if trialEnds.Valid {
		u.TrialEndsAt = trialEnds.Time
	} else {
		u.TrialEndsAt = u.CreatedAt.Add(7 * 24 * time.Hour)
	}
	return &u, nil
}

func (s *Store) ListUsersWithSubscriptions(ctx context.Context) ([]*UserWithSubscription, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, email, name, COALESCE(role, 'developer'), pat_token, trial_ends_at, created_at
		FROM users ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*UserWithSubscription
	now := time.Now().UTC()

	for rows.Next() {
		var u UserWithSubscription
		var role sql.NullString
		var trialEnds sql.NullTime
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &role, &u.PATToken, &trialEnds, &u.CreatedAt); err != nil {
			return nil, err
		}
		if role.Valid && role.String != "" {
			u.Role = role.String
		} else {
			u.Role = "developer"
		}
		if trialEnds.Valid {
			u.TrialEndsAt = trialEnds.Time
		} else {
			u.TrialEndsAt = u.CreatedAt.Add(7 * 24 * time.Hour)
		}

		u.IsTrial = u.TrialEndsAt.After(now)
		if u.IsTrial {
			u.DaysLeft = int(time.Until(u.TrialEndsAt).Hours()/24) + 1
			if u.DaysLeft < 0 {
				u.DaysLeft = 0
			}
		}

		sub, _ := s.GetSubscriptionByUserID(ctx, u.ID)
		u.Sub = sub

		result = append(result, &u)
	}
	return result, nil
}

func (s *Store) UpdateUserProfile(ctx context.Context, userID, name, email string) (*User, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET name = ?, email = ? WHERE id = ?`, name, email, userID)
	if err != nil {
		return nil, err
	}
	var u User
	var role sql.NullString
	var trialEnds sql.NullTime
	err = s.db.QueryRowContext(ctx,
		`SELECT id, email, name, COALESCE(role, 'developer'), pat_token, trial_ends_at, created_at FROM users WHERE id = ?`,
		userID,
	).Scan(&u.ID, &u.Email, &u.Name, &role, &u.PATToken, &trialEnds, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if role.Valid && role.String != "" {
		u.Role = role.String
	} else {
		u.Role = "developer"
	}
	return &u, nil
}

func (s *Store) GrantPackage(ctx context.Context, userID, plan string, maxSessions int) error {
	now := time.Now().UTC()
	sub, _ := s.GetSubscriptionByUserID(ctx, userID)
	if sub == nil {
		sub = &SubscriptionRecord{
			UserID:    userID,
			CreatedAt: now,
		}
	}
	sub.Plan = plan
	sub.Status = "active"
	sub.MaxSessions = maxSessions
	sub.UpdatedAt = now
	return s.CreateOrUpdateSubscription(ctx, sub)
}

func (s *Store) DeleteUser(ctx context.Context, userID string) error {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE user_id = ?`, userID)
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
	return err
}

func (s *Store) RegeneratePAT(ctx context.Context, userID string) (string, error) {
	rawBytes := make([]byte, 20)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", err
	}
	newPAT := "wac_pat_" + hex.EncodeToString(rawBytes)
	_, err := s.db.ExecContext(ctx, `UPDATE users SET pat_token = ? WHERE id = ?`, newPAT, userID)
	if err != nil {
		return "", err
	}
	return newPAT, nil
}

// ----------------- API Keys -----------------

func hashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateAPIKey(ctx context.Context, name string) (*APIKey, error) {
	rawBytes := make([]byte, 24)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, err
	}
	rawKey := "wac_" + hex.EncodeToString(rawBytes)
	keyID := "key_" + hex.EncodeToString(rawBytes[:8])
	hash := hashKey(rawKey)
	now := time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, key_hash, name, created_at, enabled) VALUES (?, ?, ?, ?, 1)`,
		keyID, hash, name, now,
	)
	if err != nil {
		return nil, err
	}

	return &APIKey{
		ID:        keyID,
		Key:       rawKey,
		Name:      name,
		CreatedAt: now,
		Enabled:   true,
	}, nil
}

func (s *Store) ValidateAPIKey(ctx context.Context, rawKey string) (bool, *APIKey) {
	hash := hashKey(rawKey)
	var k APIKey
	var lastUsed sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, created_at, last_used_at, enabled FROM api_keys WHERE key_hash = ? AND enabled = 1`,
		hash,
	).Scan(&k.ID, &k.Name, &k.CreatedAt, &lastUsed, &k.Enabled)
	if err != nil {
		return false, nil
	}
	if lastUsed.Valid {
		k.LastUsedAt = &lastUsed.Time
	}

	now := time.Now().UTC()
	_, _ = s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = ? WHERE id = ?`, now, k.ID)

	return true, &k
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, created_at, last_used_at, enabled FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var lastUsed sql.NullTime
		if err := rows.Scan(&k.ID, &k.Name, &k.CreatedAt, &lastUsed, &k.Enabled); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			k.LastUsedAt = &lastUsed.Time
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (s *Store) DeleteAPIKey(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	return err
}

func (s *Store) UpsertSession(ctx context.Context, id, name, jid, status, webhookURL, apiKey string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, name, jid, status, webhook_url, api_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = CASE WHEN excluded.name != '' THEN excluded.name ELSE sessions.name END,
			jid = CASE WHEN excluded.jid != '' THEN excluded.jid ELSE sessions.jid END,
			status = excluded.status,
			webhook_url = CASE WHEN excluded.webhook_url != '' THEN excluded.webhook_url ELSE sessions.webhook_url END,
			api_key = CASE WHEN excluded.api_key != '' THEN excluded.api_key ELSE sessions.api_key END,
			updated_at = excluded.updated_at
	`, id, name, jid, status, webhookURL, apiKey, now, now)
	return err
}

func (s *Store) GetSession(ctx context.Context, id string) (*SessionRecord, error) {
	var r SessionRecord
	var jid, webhook, apiKey sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, jid, status, webhook_url, api_key, created_at, updated_at FROM sessions WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.Name, &jid, &r.Status, &webhook, &apiKey, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if jid.Valid {
		r.JID = jid.String
	}
	if webhook.Valid {
		r.WebhookURL = webhook.String
	}
	if apiKey.Valid {
		r.APIKey = apiKey.String
	}
	return &r, nil
}

func (s *Store) GetSessionByAPIKey(ctx context.Context, key string) (*SessionRecord, error) {
	var r SessionRecord
	var jid, webhook, apiKey sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, jid, status, webhook_url, api_key, created_at, updated_at FROM sessions WHERE api_key = ?`,
		key,
	).Scan(&r.ID, &r.Name, &jid, &r.Status, &webhook, &apiKey, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if jid.Valid {
		r.JID = jid.String
	}
	if webhook.Valid {
		r.WebhookURL = webhook.String
	}
	if apiKey.Valid {
		r.APIKey = apiKey.String
	}
	return &r, nil
}

func (s *Store) ListSessions(ctx context.Context) ([]SessionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, jid, status, webhook_url, api_key, created_at, updated_at FROM sessions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []SessionRecord
	for rows.Next() {
		var r SessionRecord
		var jid, webhook, apiKey sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &jid, &r.Status, &webhook, &apiKey, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if jid.Valid {
			r.JID = jid.String
		}
		if webhook.Valid {
			r.WebhookURL = webhook.String
		}
		if apiKey.Valid {
			r.APIKey = apiKey.String
		}
		records = append(records, r)
	}
	return records, nil
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// ----------------- Calls -----------------

func (s *Store) InsertCall(ctx context.Context, call CallRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO calls (call_id, session_id, direction, peer_number, status, duration_seconds, started_at, reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, call.CallID, call.SessionID, call.Direction, call.PeerNumber, call.Status, call.DurationSeconds, call.StartedAt, call.Reason)
	return err
}

func (s *Store) UpdateCallStatus(ctx context.Context, callID, status string, durationSec int, reason string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE calls SET status = ?, duration_seconds = ?, ended_at = ?, reason = ? WHERE call_id = ?
	`, status, durationSec, now, reason, callID)
	return err
}

func (s *Store) ListCalls(ctx context.Context, sessionID string, limit int) ([]CallRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if sessionID != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT call_id, session_id, direction, peer_number, status, duration_seconds, started_at, ended_at, reason
			FROM calls WHERE session_id = ? ORDER BY started_at DESC LIMIT ?
		`, sessionID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT call_id, session_id, direction, peer_number, status, duration_seconds, started_at, ended_at, reason
			FROM calls ORDER BY started_at DESC LIMIT ?
		`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []CallRecord
	for rows.Next() {
		var c CallRecord
		var endedAt sql.NullTime
		var reason sql.NullString
		if err := rows.Scan(&c.CallID, &c.SessionID, &c.Direction, &c.PeerNumber, &c.Status, &c.DurationSeconds, &c.StartedAt, &endedAt, &reason); err != nil {
			return nil, err
		}
		if endedAt.Valid {
			c.EndedAt = &endedAt.Time
		}
		if reason.Valid {
			c.Reason = reason.String
		}
		list = append(list, c)
	}
	return list, nil
}

// ----------------- Messages -----------------

func (s *Store) InsertMessage(ctx context.Context, msg MessageRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messages (session_id, message_id, direction, peer_number, msg_type, content, media_url, status, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, msg.SessionID, msg.MessageID, msg.Direction, msg.PeerNumber, msg.MsgType, msg.Content, msg.MediaURL, msg.Status, msg.Timestamp)
	return err
}

func (s *Store) ListMessages(ctx context.Context, sessionID string, limit int) ([]MessageRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if sessionID != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, session_id, message_id, direction, peer_number, msg_type, content, media_url, status, timestamp
			FROM messages WHERE session_id = ? ORDER BY timestamp DESC LIMIT ?
		`, sessionID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, session_id, message_id, direction, peer_number, msg_type, content, media_url, status, timestamp
			FROM messages ORDER BY timestamp DESC LIMIT ?
		`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []MessageRecord
	for rows.Next() {
		var m MessageRecord
		var content, mediaURL sql.NullString
		if err := rows.Scan(&m.ID, &m.SessionID, &m.MessageID, &m.Direction, &m.PeerNumber, &m.MsgType, &content, &mediaURL, &m.Status, &m.Timestamp); err != nil {
			return nil, err
		}
		if content.Valid {
			m.Content = content.String
		}
		if mediaURL.Valid {
			m.MediaURL = mediaURL.String
		}
		list = append(list, m)
	}
	return list, nil
}

// ----------------- Webhook Logs -----------------

func (s *Store) LogWebhook(ctx context.Context, sessionID, event, targetURL string, statusCode int, payload, errMsg string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO webhook_logs (session_id, event, target_url, status_code, payload, error, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, event, targetURL, statusCode, payload, errMsg, now)
	return err
}

func (s *Store) ListWebhookLogs(ctx context.Context, limit int) ([]WebhookLogRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, event, target_url, status_code, payload, error, timestamp
		FROM webhook_logs ORDER BY timestamp DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []WebhookLogRecord
	for rows.Next() {
		var l WebhookLogRecord
		var sessID, errStr sql.NullString
		if err := rows.Scan(&l.ID, &sessID, &l.Event, &l.TargetURL, &l.StatusCode, &l.Payload, &errStr, &l.Timestamp); err != nil {
			return nil, err
		}
		if sessID.Valid {
			l.SessionID = sessID.String
		}
		if errStr.Valid {
			l.Error = errStr.String
		}
		list = append(list, l)
	}
	return list, nil
}

func (s *Store) CleanLID(lid string) string {
	cleaned := strings.TrimSuffix(lid, "@lid")
	cleaned = strings.TrimSuffix(cleaned, "@s.whatsapp.net")
	if idx := strings.Index(cleaned, ":"); idx != -1 {
		cleaned = cleaned[:idx]
	}
	return strings.TrimPrefix(cleaned, "+")
}

type SystemStats struct {
	TotalUsers       int `json:"total_users"`
	TotalAPIKeys     int `json:"total_api_keys"`
	TotalSessions    int `json:"total_sessions"`
	TotalCalls       int `json:"total_calls"`
	TotalMessages    int `json:"total_messages"`
	TotalWebhookLogs int `json:"total_webhook_logs"`
	SuccessWebhooks  int `json:"success_webhooks"`
}

func (s *Store) GetSystemStats(ctx context.Context) (SystemStats, error) {
	var stats SystemStats
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&stats.TotalUsers)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM api_keys").Scan(&stats.TotalAPIKeys)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&stats.TotalSessions)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM calls").Scan(&stats.TotalCalls)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM messages").Scan(&stats.TotalMessages)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM webhook_logs").Scan(&stats.TotalWebhookLogs)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM webhook_logs WHERE status_code >= 200 AND status_code < 300").Scan(&stats.SuccessWebhooks)
	return stats, nil
}

// ----------------- Subscriptions -----------------

type SubscriptionRecord struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"user_id"`
	Plan                 string    `json:"plan"` // "basic", "pro", "plus", "business"
	Status               string    `json:"status"` // "active", "trialing", "canceled", "past_due"
	MaxSessions          int       `json:"max_sessions"`
	StripeCustomerID     string    `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID string    `json:"stripe_subscription_id,omitempty"`
	StripeSessionID      string    `json:"stripe_session_id,omitempty"`
	AmountCents          int       `json:"amount_cents"`
	Currency             string    `json:"currency"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (s *Store) CreateOrUpdateSubscription(ctx context.Context, sub *SubscriptionRecord) error {
	now := time.Now().UTC()
	if sub.ID == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		sub.ID = "sub_" + hex.EncodeToString(b)
	}
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	sub.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, user_id, plan, status, max_sessions, stripe_customer_id, stripe_subscription_id, stripe_session_id, amount_cents, currency, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			plan = excluded.plan,
			status = excluded.status,
			max_sessions = excluded.max_sessions,
			stripe_customer_id = excluded.stripe_customer_id,
			stripe_subscription_id = excluded.stripe_subscription_id,
			stripe_session_id = excluded.stripe_session_id,
			amount_cents = excluded.amount_cents,
			updated_at = excluded.updated_at
	`, sub.ID, sub.UserID, sub.Plan, sub.Status, sub.MaxSessions, sub.StripeCustomerID, sub.StripeSubscriptionID, sub.StripeSessionID, sub.AmountCents, sub.Currency, sub.CreatedAt, sub.UpdatedAt)
	return err
}

func (s *Store) GetSubscriptionByUserID(ctx context.Context, userID string) (*SubscriptionRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, plan, status, max_sessions, stripe_customer_id, stripe_subscription_id, stripe_session_id, amount_cents, currency, created_at, updated_at
		FROM subscriptions WHERE user_id = ? ORDER BY created_at DESC LIMIT 1
	`, userID)

	var sub SubscriptionRecord
	var custID, subID, sessID sql.NullString
	if err := row.Scan(&sub.ID, &sub.UserID, &sub.Plan, &sub.Status, &sub.MaxSessions, &custID, &subID, &sessID, &sub.AmountCents, &sub.Currency, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			// Default Basic plan
			return &SubscriptionRecord{
				ID:          "sub_default",
				UserID:      userID,
				Plan:        "basic",
				Status:      "active",
				MaxSessions: 1,
				AmountCents: 600,
				Currency:    "usd",
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			}, nil
		}
		return nil, err
	}
	if custID.Valid { sub.StripeCustomerID = custID.String }
	if subID.Valid { sub.StripeSubscriptionID = subID.String }
	if sessID.Valid { sub.StripeSessionID = sessID.String }
	return &sub, nil
}

func (s *Store) GetSubscriptionByStripeSessionID(ctx context.Context, sessionID string) (*SubscriptionRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, plan, status, max_sessions, stripe_customer_id, stripe_subscription_id, stripe_session_id, amount_cents, currency, created_at, updated_at
		FROM subscriptions WHERE stripe_session_id = ? LIMIT 1
	`, sessionID)

	var sub SubscriptionRecord
	var custID, subID, sessID sql.NullString
	if err := row.Scan(&sub.ID, &sub.UserID, &sub.Plan, &sub.Status, &sub.MaxSessions, &custID, &subID, &sessID, &sub.AmountCents, &sub.Currency, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
		return nil, err
	}
	if custID.Valid { sub.StripeCustomerID = custID.String }
	if subID.Valid { sub.StripeSubscriptionID = subID.String }
	if sessID.Valid { sub.StripeSessionID = sessID.String }
	return &sub, nil
}


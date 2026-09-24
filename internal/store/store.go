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

	"golang.org/x/crypto/bcrypt"
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
	UserID     string     `json:"user_id,omitempty"`
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
	UserID     string    `json:"user_id,omitempty"`
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
	_, _ = s.db.ExecContext(ctx, "ALTER TABLE sessions ADD COLUMN user_id TEXT;")
	_, _ = s.db.ExecContext(ctx, "ALTER TABLE api_keys ADD COLUMN user_id TEXT;")
	_, _ = s.db.ExecContext(ctx, "ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'developer';")
	_, _ = s.db.ExecContext(ctx, "ALTER TABLE users ADD COLUMN trial_ends_at DATETIME;")
	// Phase 0: Password version column (1 = SHA-256 legacy, 2 = bcrypt)
	_, _ = s.db.ExecContext(ctx, "ALTER TABLE users ADD COLUMN password_version INTEGER NOT NULL DEFAULT 1;")

	// Phase 0: Stripe event deduplication table
	_, _ = s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS stripe_events (
		event_id TEXT PRIMARY KEY,
		event_type TEXT NOT NULL,
		processed_at DATETIME NOT NULL
	);`)

	// Phase 0: Removed hardcoded superadmin credentials (security fix).
	// Use CLI bootstrap command or manual admin creation instead.

	return nil
}

// ----------------- Developer Users & PATs -----------------

func (s *Store) CreateUser(ctx context.Context, name, email, rawPassword string) (*User, error) {
	rawBytes := make([]byte, 20)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, err
	}
	pat := "wac_pat_" + hex.EncodeToString(rawBytes)
	userID := "usr_" + hex.EncodeToString(rawBytes[:8])
	// Phase 0: Use bcrypt instead of SHA-256
	pwdHash, err := hashPassword(rawPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	now := time.Now().UTC()
	trialEnd := now.Add(7 * 24 * time.Hour) // 7-day trial

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, name, role, password_hash, password_version, pat_token, trial_ends_at, created_at) VALUES (?, ?, ?, 'developer', ?, 2, ?, ?, ?)`,
		userID, email, name, pwdHash, pat, trialEnd, now,
	)
	if err != nil {
		return nil, fmt.Errorf("user creation failed: %w", err)
	}

	// Auto-create active 7-day trial subscription record
	_ = s.CreateOrUpdateSubscription(ctx, &SubscriptionRecord{
		ID:          "sub_trial_" + hex.EncodeToString(rawBytes[:6]),
		UserID:      userID,
		Plan:        "basic",
		Status:      "active",
		MaxSessions: 1,
		AmountCents: 0,
		Currency:    "usd",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	return &User{
		ID:          userID,
		Email:       email,
		Name:        name,
		Role:        "developer",
		PATToken:    pat,
		TrialEndsAt: trialEnd,
		CreatedAt:   now,
	}, nil
}

func (s *Store) AuthenticateUser(ctx context.Context, email, rawPassword string) (*User, error) {
	// Phase 0: Fetch stored hash and password version for migration support
	var u User
	var trialEnd sql.NullTime
	var storedHash string
	var pwdVersion int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, role, password_hash, COALESCE(password_version, 1), pat_token, trial_ends_at, created_at FROM users WHERE email = ?`,
		email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &storedHash, &pwdVersion, &u.PATToken, &trialEnd, &u.CreatedAt)
	if err != nil {
		return nil, err
	}

	// Phase 0: Verify password based on version
	var passwordValid bool
	if pwdVersion == 2 {
		// bcrypt
		passwordValid = verifyPassword(storedHash, rawPassword)
	} else {
		// Legacy SHA-256 — verify and then rehash with bcrypt
		legacyHash := hashKey(rawPassword)
		passwordValid = storedHash == legacyHash
		if passwordValid {
			// Transparently upgrade to bcrypt
			if newHash, err := hashPassword(rawPassword); err == nil {
				_, _ = s.db.ExecContext(ctx,
					`UPDATE users SET password_hash = ?, password_version = 2 WHERE id = ?`,
					newHash, u.ID,
				)
			}
		}
	}

	if !passwordValid {
		return nil, fmt.Errorf("invalid credentials")
	}

	if trialEnd.Valid {
		u.TrialEndsAt = trialEnd.Time
	}
	return &u, nil
}

func (s *Store) GetUserByPAT(ctx context.Context, pat string) (*User, error) {
	var u User
	var trialEnd sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, role, pat_token, trial_ends_at, created_at FROM users WHERE pat_token = ?`,
		pat,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.PATToken, &trialEnd, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if trialEnd.Valid {
		u.TrialEndsAt = trialEnd.Time
	}
	return &u, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	var trialEnd sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, role, pat_token, trial_ends_at, created_at FROM users WHERE email = ?`,
		email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.PATToken, &trialEnd, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if trialEnd.Valid {
		u.TrialEndsAt = trialEnd.Time
	}
	return &u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*User, error) {
	var u User
	var trialEnd sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, role, pat_token, trial_ends_at, created_at FROM users WHERE id = ?`,
		id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.PATToken, &trialEnd, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if trialEnd.Valid {
		u.TrialEndsAt = trialEnd.Time
	}
	return &u, nil
}

func (s *Store) UpdateUserProfile(ctx context.Context, id, name, email string) (*User, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET name = ?, email = ? WHERE id = ?`, name, email, id)
	if err != nil {
		return nil, err
	}
	return s.GetUserByID(ctx, id)
}

func (s *Store) GrantPackage(ctx context.Context, userID, plan string, maxSessions, amountCents int) (*SubscriptionRecord, error) {
	sub, err := s.GetSubscriptionByUserID(ctx, userID)
	if err != nil || sub == nil {
		sub = &SubscriptionRecord{
			UserID: userID,
		}
	}
	sub.Plan = plan
	sub.Status = "active"
	sub.MaxSessions = maxSessions
	sub.AmountCents = amountCents
	sub.Currency = "usd"
	if err := s.CreateOrUpdateSubscription(ctx, sub); err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *Store) RegeneratePAT(ctx context.Context, id string) (string, error) {
	rawBytes := make([]byte, 20)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", err
	}
	newPAT := "wac_pat_" + hex.EncodeToString(rawBytes)
	_, err := s.db.ExecContext(ctx, `UPDATE users SET pat_token = ? WHERE id = ?`, newPAT, id)
	return newPAT, err
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

func (s *Store) ListUsersWithSubscriptions(ctx context.Context) ([]UserWithSubscription, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.email, u.name, u.role, u.pat_token, u.trial_ends_at, u.created_at,
		       s.id, s.plan, s.status, s.max_sessions, s.amount_cents, s.currency, s.created_at, s.updated_at
		FROM users u
		LEFT JOIN subscriptions s ON u.id = s.user_id
		ORDER BY u.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []UserWithSubscription
	now := time.Now().UTC()
	for rows.Next() {
		var u UserWithSubscription
		var trialEnd sql.NullTime
		var subID, subPlan, subStatus, subCurr sql.NullString
		var subMax, subAmt sql.NullInt64
		var subCreated, subUpdated sql.NullTime

		if err := rows.Scan(
			&u.ID, &u.Email, &u.Name, &u.Role, &u.PATToken, &trialEnd, &u.CreatedAt,
			&subID, &subPlan, &subStatus, &subMax, &subAmt, &subCurr, &subCreated, &subUpdated,
		); err != nil {
			return nil, err
		}

		if trialEnd.Valid {
			u.TrialEndsAt = trialEnd.Time
			if trialEnd.Time.After(now) {
				u.IsTrial = true
				u.DaysLeft = int(trialEnd.Time.Sub(now).Hours()/24) + 1
			}
		}

		if subID.Valid {
			u.Sub = &SubscriptionRecord{
				ID:          subID.String,
				UserID:      u.ID,
				Plan:        subPlan.String,
				Status:      subStatus.String,
				MaxSessions: int(subMax.Int64),
				AmountCents: int(subAmt.Int64),
				Currency:    subCurr.String,
				CreatedAt:   subCreated.Time,
				UpdatedAt:   subUpdated.Time,
			}
		}
		list = append(list, u)
	}
	return list, nil
}

// ----------------- API Keys -----------------

func (s *Store) CreateAPIKey(ctx context.Context, name, userID string) (*APIKey, error) {
	rawBytes := make([]byte, 24)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, err
	}
	rawKey := "wac_" + hex.EncodeToString(rawBytes)
	keyID := "key_" + hex.EncodeToString(rawBytes[:8])
	hash := hashKey(rawKey)
	now := time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, user_id, key_hash, name, created_at, enabled) VALUES (?, ?, ?, ?, ?, 1)`,
		keyID, userID, hash, name, now,
	)
	if err != nil {
		return nil, err
	}

	return &APIKey{
		ID:        keyID,
		UserID:    userID,
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
	var userID sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, created_at, last_used_at, enabled FROM api_keys WHERE key_hash = ? AND enabled = 1`,
		hash,
	).Scan(&k.ID, &userID, &k.Name, &k.CreatedAt, &lastUsed, &k.Enabled)
	if err != nil {
		return false, nil
	}
	if userID.Valid {
		k.UserID = userID.String
	}
	if lastUsed.Valid {
		k.LastUsedAt = &lastUsed.Time
	}

	now := time.Now().UTC()
	_, _ = s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = ? WHERE id = ?`, now, k.ID)

	return true, &k
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, name, created_at, last_used_at, enabled FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var lastUsed sql.NullTime
		var userID sql.NullString
		if err := rows.Scan(&k.ID, &userID, &k.Name, &k.CreatedAt, &lastUsed, &k.Enabled); err != nil {
			return nil, err
		}
		if userID.Valid {
			k.UserID = userID.String
		}
		if lastUsed.Valid {
			k.LastUsedAt = &lastUsed.Time
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (s *Store) ListAPIKeysByUser(ctx context.Context, userID string) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, name, created_at, last_used_at, enabled FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var lastUsed sql.NullTime
		var uid sql.NullString
		if err := rows.Scan(&k.ID, &uid, &k.Name, &k.CreatedAt, &lastUsed, &k.Enabled); err != nil {
			return nil, err
		}
		if uid.Valid {
			k.UserID = uid.String
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

// Phase 0: GetAPIKeyByID retrieves an API key by ID for ownership verification.
func (s *Store) GetAPIKeyByID(ctx context.Context, id string) (*APIKey, error) {
	var k APIKey
	var lastUsed sql.NullTime
	var userID sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, created_at, last_used_at, enabled FROM api_keys WHERE id = ?`,
		id,
	).Scan(&k.ID, &userID, &k.Name, &k.CreatedAt, &lastUsed, &k.Enabled)
	if err != nil {
		return nil, err
	}
	if userID.Valid {
		k.UserID = userID.String
	}
	if lastUsed.Valid {
		k.LastUsedAt = &lastUsed.Time
	}
	return &k, nil
}

// ----------------- Sessions -----------------

func (s *Store) UpsertSession(ctx context.Context, id, name, jid, status, webhookURL, apiKey, userID string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, name, user_id, jid, status, webhook_url, api_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = CASE WHEN excluded.name != '' THEN excluded.name ELSE sessions.name END,
			user_id = CASE WHEN excluded.user_id != '' THEN excluded.user_id ELSE sessions.user_id END,
			jid = CASE WHEN excluded.jid != '' THEN excluded.jid ELSE sessions.jid END,
			status = excluded.status,
			webhook_url = CASE WHEN excluded.webhook_url != '' THEN excluded.webhook_url ELSE sessions.webhook_url END,
			api_key = CASE WHEN excluded.api_key != '' THEN excluded.api_key ELSE sessions.api_key END,
			updated_at = excluded.updated_at
	`, id, name, userID, jid, status, webhookURL, apiKey, now, now)
	return err
}

func (s *Store) GetSession(ctx context.Context, id string) (*SessionRecord, error) {
	var r SessionRecord
	var jid, webhook, apiKey, userID sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, user_id, jid, status, webhook_url, api_key, created_at, updated_at FROM sessions WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.Name, &userID, &jid, &r.Status, &webhook, &apiKey, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if userID.Valid {
		r.UserID = userID.String
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
	var jid, webhook, apiKey, userID sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, user_id, jid, status, webhook_url, api_key, created_at, updated_at FROM sessions WHERE api_key = ?`,
		key,
	).Scan(&r.ID, &r.Name, &userID, &jid, &r.Status, &webhook, &apiKey, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if userID.Valid {
		r.UserID = userID.String
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
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, user_id, jid, status, webhook_url, api_key, created_at, updated_at FROM sessions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []SessionRecord
	for rows.Next() {
		var r SessionRecord
		var jid, webhook, apiKey, userID sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &userID, &jid, &r.Status, &webhook, &apiKey, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if userID.Valid {
			r.UserID = userID.String
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

func (s *Store) ListSessionsByUser(ctx context.Context, userID string) ([]SessionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, user_id, jid, status, webhook_url, api_key, created_at, updated_at FROM sessions WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []SessionRecord
	for rows.Next() {
		var r SessionRecord
		var jid, webhook, apiKey, uid sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &uid, &jid, &r.Status, &webhook, &apiKey, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if uid.Valid {
			r.UserID = uid.String
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

func (s *Store) ListCallsByUser(ctx context.Context, userID string, sessionID string, limit int) ([]CallRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if sessionID != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT c.call_id, c.session_id, c.direction, c.peer_number, c.status, c.duration_seconds, c.started_at, c.ended_at, c.reason
			FROM calls c
			INNER JOIN sessions s ON c.session_id = s.id
			WHERE s.user_id = ? AND c.session_id = ?
			ORDER BY c.started_at DESC LIMIT ?
		`, userID, sessionID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT c.call_id, c.session_id, c.direction, c.peer_number, c.status, c.duration_seconds, c.started_at, c.ended_at, c.reason
			FROM calls c
			INNER JOIN sessions s ON c.session_id = s.id
			WHERE s.user_id = ?
			ORDER BY c.started_at DESC LIMIT ?
		`, userID, limit)
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

func (s *Store) ListMessagesByUser(ctx context.Context, userID string, sessionID string, limit int) ([]MessageRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if sessionID != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT m.id, m.session_id, m.message_id, m.direction, m.peer_number, m.msg_type, m.content, m.media_url, m.status, m.timestamp
			FROM messages m
			INNER JOIN sessions s ON m.session_id = s.id
			WHERE s.user_id = ? AND m.session_id = ?
			ORDER BY m.timestamp DESC LIMIT ?
		`, userID, sessionID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT m.id, m.session_id, m.message_id, m.direction, m.peer_number, m.msg_type, m.content, m.media_url, m.status, m.timestamp
			FROM messages m
			INNER JOIN sessions s ON m.session_id = s.id
			WHERE s.user_id = ?
			ORDER BY m.timestamp DESC LIMIT ?
		`, userID, limit)
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

func (s *Store) InsertWebhookLog(ctx context.Context, log WebhookLogRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO webhook_logs (session_id, event, target_url, status_code, payload, error, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, log.SessionID, log.Event, log.TargetURL, log.StatusCode, log.Payload, log.Error, log.Timestamp)
	return err
}

func (s *Store) LogWebhook(ctx context.Context, sessionID, event, targetURL string, statusCode int, payload, errMsg string) error {
	return s.InsertWebhookLog(ctx, WebhookLogRecord{
		SessionID:  sessionID,
		Event:      event,
		TargetURL:  targetURL,
		StatusCode: statusCode,
		Payload:    payload,
		Error:      errMsg,
		Timestamp:  time.Now().UTC(),
	})
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

func (s *Store) ListWebhookLogsByUser(ctx context.Context, userID string, limit int) ([]WebhookLogRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT w.id, w.session_id, w.event, w.target_url, w.status_code, w.payload, w.error, w.timestamp
		FROM webhook_logs w
		INNER JOIN sessions s ON w.session_id = s.id
		WHERE s.user_id = ?
		ORDER BY w.timestamp DESC LIMIT ?
	`, userID, limit)
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

type UserStats struct {
	TotalSessions     int `json:"total_sessions"`
	ConnectedSessions int `json:"connected_sessions"`
	TotalCalls        int `json:"total_calls"`
	TotalMessages     int `json:"total_messages"`
	TotalWebhookLogs  int `json:"total_webhook_logs"`
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

func (s *Store) GetUserStats(ctx context.Context, userID string) (UserStats, error) {
	var stats UserStats
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions WHERE user_id = ?", userID).Scan(&stats.TotalSessions)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions WHERE user_id = ? AND status = 'CONNECTED'", userID).Scan(&stats.ConnectedSessions)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM calls c INNER JOIN sessions s ON c.session_id = s.id WHERE s.user_id = ?", userID).Scan(&stats.TotalCalls)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM messages m INNER JOIN sessions s ON m.session_id = s.id WHERE s.user_id = ?", userID).Scan(&stats.TotalMessages)
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM webhook_logs w INNER JOIN sessions s ON w.session_id = s.id WHERE s.user_id = ?", userID).Scan(&stats.TotalWebhookLogs)
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

func hashKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// Phase 0: bcrypt password hashing (replaces SHA-256 for passwords)
func hashPassword(raw string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(raw), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyPassword(hash, raw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(raw)) == nil
}

// Phase 0: Stripe event deduplication
func (s *Store) IsStripeEventProcessed(ctx context.Context, eventID string) bool {
	var count int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stripe_events WHERE event_id = ?`, eventID).Scan(&count)
	return count > 0
}

func (s *Store) MarkStripeEventProcessed(ctx context.Context, eventID, eventType string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO stripe_events (event_id, event_type, processed_at) VALUES (?, ?, ?)`,
		eventID, eventType, time.Now().UTC(),
	)
	return err
}

// Phase 0: Bootstrap admin creation (replaces hardcoded credentials)
func (s *Store) BootstrapAdmin(ctx context.Context, email, rawPassword string) (*User, error) {
	var userCount int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&userCount)
	if userCount > 0 {
		return nil, fmt.Errorf("cannot bootstrap admin: users already exist")
	}

	rawBytes := make([]byte, 20)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, err
	}
	pat := "wac_pat_" + hex.EncodeToString(rawBytes)
	userID := "usr_" + hex.EncodeToString(rawBytes[:8])
	pwdHash, err := hashPassword(rawPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	now := time.Now().UTC()

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, name, role, password_hash, password_version, pat_token, created_at) VALUES (?, ?, ?, 'superadmin', ?, 2, ?, ?)`,
		userID, email, "Admin", pwdHash, pat, now,
	)
	if err != nil {
		return nil, fmt.Errorf("admin creation failed: %w", err)
	}

	_ = s.CreateOrUpdateSubscription(ctx, &SubscriptionRecord{
		ID:          "sub_admin_" + hex.EncodeToString(rawBytes[:6]),
		UserID:      userID,
		Plan:        "business",
		Status:      "active",
		MaxSessions: 100,
		AmountCents: 0,
		Currency:    "usd",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	return &User{
		ID:        userID,
		Email:     email,
		Name:      "Admin",
		Role:      "superadmin",
		PATToken:  pat,
		CreatedAt: now,
	}, nil
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

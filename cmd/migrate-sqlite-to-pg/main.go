package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func main() {
	sqlitePath := flag.String("sqlite", "wacaller.db", "Path to legacy SQLite database file")
	pgDSN := flag.String("postgres-dsn", "", "PostgreSQL DSN string")
	flag.Parse()

	if *pgDSN == "" {
		log.Fatal("❌ Error: -postgres-dsn flag is required")
	}

	ctx := context.Background()

	log.Printf("🔌 Connecting to SQLite source: %s", *sqlitePath)
	sqliteDB, err := sql.Open("sqlite", *sqlitePath)
	if err != nil {
		log.Fatalf("Failed to open SQLite source: %v", err)
	}
	defer sqliteDB.Close()

	log.Printf("🔌 Connecting to PostgreSQL destination...")
	pgDB, err := sql.Open("pgx", *pgDSN)
	if err != nil {
		log.Fatalf("Failed to open PostgreSQL target: %v", err)
	}
	defer pgDB.Close()

	if err := pgDB.PingContext(ctx); err != nil {
		log.Fatalf("Failed to ping PostgreSQL target: %v", err)
	}

	log.Println("🚀 Starting SQLite → PostgreSQL data migration...")

	// 1. Create Default Organization
	defaultOrgID := "org_default"
	defaultOrgSlug := "default"
	log.Printf("🏢 Ensuring Default Organization (%s) exists...", defaultOrgID)
	_, err = pgDB.ExecContext(ctx, `
		INSERT INTO organizations (id, name, slug, status, created_at, updated_at)
		VALUES ($1, 'Default Organization', $2, 'active', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, defaultOrgID, defaultOrgSlug)
	if err != nil {
		log.Fatalf("Failed to create default organization: %v", err)
	}

	// 2. Migrate Users
	userMap := make(map[string]string) // email -> user_id
	rows, err := sqliteDB.QueryContext(ctx, `SELECT id, email, name, role, password_hash, password_version, created_at FROM users`)
	if err == nil {
		defer rows.Close()
		userCount := 0
		for rows.Next() {
			var id, email, name, role, passHash string
			var passVer int
			var createdAt time.Time
			if err := rows.Scan(&id, &email, &name, &role, &passHash, &passVer, &createdAt); err != nil {
				log.Printf("⚠️ Warning scanning user: %v", err)
				continue
			}

			if id == "" {
				id = "usr_" + uuid.New().String()
			}
			userMap[email] = id

			_, err = pgDB.ExecContext(ctx, `
				INSERT INTO users (id, email, name, role, password_hash, password_version, status, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, NOW())
				ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, role = EXCLUDED.role
			`, id, email, name, role, passHash, passVer, createdAt)
			if err != nil {
				log.Printf("⚠️ Failed to migrate user %s: %v", email, err)
			} else {
				userCount++
				// Create owner membership in default org
				memID := "mem_" + uuid.New().String()
				_, _ = pgDB.ExecContext(ctx, `
					INSERT INTO memberships (id, user_id, organization_id, role, created_at)
					VALUES ($1, $2, $3, 'owner', NOW())
					ON CONFLICT (user_id, organization_id) DO NOTHING
				`, memID, id, defaultOrgID)
			}
		}
		log.Printf("✅ Migrated %d users to PostgreSQL", userCount)
	} else {
		log.Printf("ℹ️ No users table in SQLite or query error: %v", err)
	}

	// 3. Migrate API Keys -> api_credentials
	keyRows, err := sqliteDB.QueryContext(ctx, `SELECT id, name, key_hash, created_at FROM api_keys`)
	if err == nil {
		defer keyRows.Close()
		keyCount := 0
		// Pick first user or system as created_by
		createdBy := "system"
		for _, uid := range userMap {
			createdBy = uid
			break
		}
		for keyRows.Next() {
			var id, name, keyHash string
			var createdAt time.Time
			if err := keyRows.Scan(&id, &name, &keyHash, &createdAt); err != nil {
				continue
			}
			prefix := keyHash
			if len(keyHash) > 8 {
				prefix = keyHash[:8]
			}
			credID := "cred_" + uuid.New().String()
			_, err = pgDB.ExecContext(ctx, `
				INSERT INTO api_credentials (id, organization_id, name, key_hash, key_prefix, scopes, created_by, created_at)
				VALUES ($1, $2, $3, $4, $5, '{}', $6, $7)
				ON CONFLICT (key_hash) DO NOTHING
			`, credID, defaultOrgID, name, keyHash, prefix, createdBy, createdAt)
			if err == nil {
				keyCount++
			}
		}
		log.Printf("✅ Migrated %d API keys to api_credentials", keyCount)
	}

	// 4. Migrate WhatsApp Sessions -> whatsapp_sessions
	sessRows, err := sqliteDB.QueryContext(ctx, `SELECT id, name, jid, status, webhook_url, created_at FROM sessions`)
	if err == nil {
		defer sessRows.Close()
		sessCount := 0
		for sessRows.Next() {
			var id, name, jid, status, webhookURL string
			var createdAt time.Time
			if err := sessRows.Scan(&id, &name, &jid, &status, &webhookURL, &createdAt); err != nil {
				continue
			}
			_, err = pgDB.ExecContext(ctx, `
				INSERT INTO whatsapp_sessions (id, organization_id, name, jid, status, webhook_url, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
				ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, jid = EXCLUDED.jid, status = EXCLUDED.status
			`, id, defaultOrgID, name, jid, status, webhookURL, createdAt)
			if err == nil {
				sessCount++
			}
		}
		log.Printf("✅ Migrated %d WhatsApp sessions", sessCount)
	}

	// 5. Migrate Calls -> calls
	callRows, err := sqliteDB.QueryContext(ctx, `SELECT id, session_id, direction, peer_number, status, duration_seconds, started_at, ended_at, end_reason FROM call_records`)
	if err == nil {
		defer callRows.Close()
		callCount := 0
		for callRows.Next() {
			var id, sessionID, direction, peerNumber, status, endReason string
			var duration int
			var startedAt time.Time
			var endedAt sql.NullTime
			if err := callRows.Scan(&id, &sessionID, &direction, &peerNumber, &status, &duration, &startedAt, &endedAt, &endReason); err != nil {
				continue
			}
			var endedAtPtr *time.Time
			if endedAt.Valid {
				endedAtPtr = &endedAt.Time
			}
			_, err = pgDB.ExecContext(ctx, `
				INSERT INTO calls (id, organization_id, session_id, direction, peer_number, status, duration_seconds, started_at, ended_at, end_reason, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
				ON CONFLICT (id) DO NOTHING
			`, id, defaultOrgID, sessionID, direction, peerNumber, status, duration, startedAt, endedAtPtr, endReason)
			if err == nil {
				callCount++
			}
		}
		log.Printf("✅ Migrated %d call records", callCount)
	}

	// 6. Migrate Messages -> messages
	msgRows, err := sqliteDB.QueryContext(ctx, `SELECT session_id, message_id, direction, peer_number, msg_type, content, media_url, status, timestamp FROM message_records`)
	if err == nil {
		defer msgRows.Close()
		msgCount := 0
		for msgRows.Next() {
			var sessionID, messageID, direction, peerNumber, msgType, content, mediaURL, status string
			var timestamp time.Time
			if err := msgRows.Scan(&sessionID, &messageID, &direction, &peerNumber, &msgType, &content, &mediaURL, &status, &timestamp); err != nil {
				continue
			}
			_, err = pgDB.ExecContext(ctx, `
				INSERT INTO messages (organization_id, session_id, message_id, direction, peer_number, msg_type, content, media_url, status, timestamp)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			`, defaultOrgID, sessionID, messageID, direction, peerNumber, msgType, content, mediaURL, status, timestamp)
			if err == nil {
				msgCount++
			}
		}
		log.Printf("✅ Migrated %d message records", msgCount)
	}

	log.Println("🎉 Migration completed successfully!")
}

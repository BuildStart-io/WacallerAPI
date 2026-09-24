package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"wacallerapi/internal/store"
)

type Middleware struct {
	store        *store.Store
	masterAPIKey string
	log          *slog.Logger
}

func New(st *store.Store, masterKey string, log *slog.Logger) *Middleware {
	return &Middleware{
		store:        st,
		masterAPIKey: masterKey,
		log:          log,
	}
}

func (m *Middleware) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Expose-Headers", "*")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		if !strings.HasPrefix(r.URL.Path, "/static") && r.URL.Path != "/favicon.ico" {
			m.log.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.status,
				"duration", time.Since(start).String(),
			)
		}
	})
}

func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Api-Key")
		if key == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				key = strings.TrimPrefix(auth, "Bearer ")
			} else if auth != "" {
				key = auth
			}
		}
		if key == "" {
			key = r.URL.Query().Get("api_key")
		}
		if key == "" {
			key = r.URL.Query().Get("apiKey")
		}

		// 1. Check if key is a session-specific API key (WasenderAPI style: wac_sess_...)
		if sessRec, err := m.store.GetSessionByAPIKey(r.Context(), key); err == nil && sessRec != nil {
			ctx := context.WithValue(r.Context(), "session_id", sessRec.ID)
			ctx = context.WithValue(ctx, "session_api_key", key)
			if sessRec.UserID != "" {
				ctx = context.WithValue(ctx, "user_id", sessRec.UserID)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// 2. Check if key is a Personal Access Token (PAT: wac_pat_...)
		if user, err := m.store.GetUserByPAT(r.Context(), key); err == nil && user != nil {
			ctx := context.WithValue(r.Context(), "user_id", user.ID)
			ctx = context.WithValue(ctx, "user_email", user.Email)
			ctx = context.WithValue(ctx, "user_name", user.Name)
			ctx = context.WithValue(ctx, "role", user.Role)
			ctx = context.WithValue(ctx, "is_pat", true)
			if user.Role == "superadmin" {
				ctx = context.WithValue(ctx, "is_admin", true)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// 3. Check if key is master key / M2M key
		if m.masterAPIKey != "" && key == m.masterAPIKey {
			ctx := context.WithValue(r.Context(), "is_admin", true)
			ctx = context.WithValue(ctx, "is_m2m", true)
			ctx = context.WithValue(ctx, "role", "superadmin")
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// 4. Check developer API key (wac_...)
		if valid, k := m.store.ValidateAPIKey(r.Context(), key); valid && k != nil {
			ctx := context.WithValue(r.Context(), "api_key_id", k.ID)
			if k.UserID != "" {
				ctx = context.WithValue(ctx, "user_id", k.UserID)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// 5. If no master key is set and no key passed, check if database has no custom keys
		if key == "" && m.masterAPIKey == "" {
			keys, _ := m.store.ListAPIKeys(r.Context())
			if len(keys) == 0 {
				next.ServeHTTP(w, r)
				return
			}
		}

		m.unauthorized(w, "invalid or missing API key (provide header 'Authorization: Bearer <key>' or 'X-Api-Key: <key>')")
	})
}

// RequireOptionalAuth inspects the API key / PAT / session key if present, injecting context, but does not block if omitted.
func (m *Middleware) RequireOptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Api-Key")
		if key == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				key = strings.TrimPrefix(auth, "Bearer ")
			} else if auth != "" {
				key = auth
			}
		}
		if key == "" {
			key = r.URL.Query().Get("api_key")
		}
		if key == "" {
			key = r.URL.Query().Get("apiKey")
		}

		if key != "" {
			if sessRec, err := m.store.GetSessionByAPIKey(r.Context(), key); err == nil && sessRec != nil {
				ctx := context.WithValue(r.Context(), "session_id", sessRec.ID)
				ctx = context.WithValue(ctx, "session_api_key", key)
				if sessRec.UserID != "" {
					ctx = context.WithValue(ctx, "user_id", sessRec.UserID)
				}
				r = r.WithContext(ctx)
			} else if user, err := m.store.GetUserByPAT(r.Context(), key); err == nil && user != nil {
				ctx := context.WithValue(r.Context(), "user_id", user.ID)
				ctx = context.WithValue(ctx, "user_email", user.Email)
				ctx = context.WithValue(ctx, "user_name", user.Name)
				ctx = context.WithValue(ctx, "role", user.Role)
				ctx = context.WithValue(ctx, "is_pat", true)
				if user.Role == "superadmin" {
					ctx = context.WithValue(ctx, "is_admin", true)
				}
				r = r.WithContext(ctx)
			} else if valid, k := m.store.ValidateAPIKey(r.Context(), key); valid && k != nil {
				ctx := context.WithValue(r.Context(), "api_key_id", k.ID)
				if k.UserID != "" {
					ctx = context.WithValue(ctx, "user_id", k.UserID)
				}
				r = r.WithContext(ctx)
			} else if m.masterAPIKey != "" && key == m.masterAPIKey {
				ctx := context.WithValue(r.Context(), "is_admin", true)
				ctx = context.WithValue(ctx, "is_m2m", true)
				ctx = context.WithValue(ctx, "role", "superadmin")
				r = r.WithContext(ctx)
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":   "unauthorized",
		"message": msg,
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

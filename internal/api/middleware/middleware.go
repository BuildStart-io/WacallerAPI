package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"wacallerapi/internal/store"

	"golang.org/x/time/rate"
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Api-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Phase 0 Task 13: Limit request payload size to maxBytes to prevent memory exhaustion attacks
func (m *Middleware) BodySizeLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
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

		// 3. Check if key is master key
		if m.masterAPIKey != "" && key == m.masterAPIKey {
			ctx := context.WithValue(r.Context(), "is_admin", true)
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

// Phase 0 Task 11: Rate limiting middleware

// RateLimiterMap manages per-key rate limiters with automatic cleanup.
type RateLimiterMap struct {
	mu       sync.RWMutex
	limiters map[string]*rateLimiterEntry
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newRateLimiterMap() *RateLimiterMap {
	rl := &RateLimiterMap{
		limiters: make(map[string]*rateLimiterEntry),
	}
	// Cleanup stale entries every 5 minutes
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			rl.cleanup()
		}
	}()
	return rl
}

func (rl *RateLimiterMap) getLimiter(key string, rps rate.Limit, burst int) *rate.Limiter {
	rl.mu.RLock()
	entry, exists := rl.limiters[key]
	rl.mu.RUnlock()
	if exists {
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	// Double-check after acquiring write lock
	if entry, exists := rl.limiters[key]; exists {
		entry.lastSeen = time.Now()
		return entry.limiter
	}
	limiter := rate.NewLimiter(rps, burst)
	rl.limiters[key] = &rateLimiterEntry{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	return limiter
}

func (rl *RateLimiterMap) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	threshold := time.Now().Add(-10 * time.Minute)
	for key, entry := range rl.limiters {
		if entry.lastSeen.Before(threshold) {
			delete(rl.limiters, key)
		}
	}
}

var globalRateLimiters = newRateLimiterMap()

// RateLimit creates a rate-limiting middleware that limits requests per IP.
// rps is requests per second, burst is the maximum burst size.
func (m *Middleware) RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)
			limiter := globalRateLimiters.getLimiter(ip, rate.Limit(rps), burst)
			if !limiter.Allow() {
				w.Header().Set("Retry-After", "60")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":   "rate_limit_exceeded",
					"message": "Too many requests. Please slow down.",
				})
				m.log.Warn("rate limit exceeded", "ip", ip, "path", r.URL.Path)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractIP(r *http.Request) string {
	// Check X-Forwarded-For first (reverse proxy)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to remote address
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

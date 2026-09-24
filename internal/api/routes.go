package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"wacallerapi/internal/api/middleware"
	"wacallerapi/internal/audio"
	"wacallerapi/internal/config"
	"wacallerapi/internal/service"
	"wacallerapi/internal/session"
	"wacallerapi/internal/store"
	"wacallerapi/internal/voip/media"
	"wacallerapi/internal/webhook"

	"github.com/coder/websocket"
)

type API struct {
	cfg        *config.Config
	store      *store.Store
	sessions   *session.SessionManager
	dispatcher *webhook.Dispatcher
	mw         *middleware.Middleware
	services   *service.Services
	log        *slog.Logger
	startTime  time.Time
}

func New(cfg *config.Config, st *store.Store, sm *session.SessionManager, disp *webhook.Dispatcher, log *slog.Logger) *API {
	mw := middleware.New(st, cfg.MasterAPIKey, log)
	return &API{
		cfg:        cfg,
		store:      st,
		sessions:   sm,
		dispatcher: disp,
		mw:         mw,
		log:        log,
		startTime:  time.Now().UTC(),
	}
}

// WithServices attaches the enterprise service layer to the API handler.
func (a *API) WithServices(services *service.Services) *API {
	a.services = services
	return a
}

func (a *API) Routes() http.Handler {
	rootMux := http.NewServeMux()

	// 1. Management & Public Sub-Router (uses RequireOptionalAuth)
	publicMux := http.NewServeMux()
	publicMux.HandleFunc("GET /health", a.handleHealth)
	publicMux.HandleFunc("GET /system/stats", a.handleSystemStats)
	publicMux.HandleFunc("GET /openapi.json", a.handleOpenAPISpec)

	// Developer authentication & account (optional auth)
	publicMux.HandleFunc("POST /auth/register", a.handleAuthRegister)
	publicMux.HandleFunc("POST /auth/login", a.handleAuthLogin)
	publicMux.HandleFunc("GET /auth/me", a.handleAuthMe)
	publicMux.HandleFunc("PUT /auth/profile", a.handleUpdateProfile)
	publicMux.HandleFunc("POST /auth/profile", a.handleUpdateProfile)
	publicMux.HandleFunc("DELETE /auth/account", a.handleDeleteAccount)
	publicMux.HandleFunc("POST /auth/pat/regenerate", a.handleRegeneratePAT)
	publicMux.HandleFunc("GET /keys/pat", a.handleGetPATDetails)

	// Superadmin User & Package Management
	publicMux.HandleFunc("GET /admin/users", a.handleAdminListUsers)
	publicMux.HandleFunc("POST /admin/users/{userId}/grant-plan", a.handleAdminGrantPlan)
	publicMux.HandleFunc("DELETE /admin/users/{userId}", a.handleAdminDeleteUser)

	// Stripe Subscriptions & Billing
	publicMux.HandleFunc("POST /billing/checkout", a.handleStripeCheckout)
	publicMux.HandleFunc("POST /billing/webhook", a.handleStripeWebhook)
	publicMux.HandleFunc("GET /billing/subscription", a.handleGetSubscription)
	// REMOVED: Direct plan activation removed from public routes (Phase 0 — Security)
	// Plan activation is now admin-only via /admin/users/{userId}/grant-plan

	// API Keys Management (Zero friction - no PAT required!)
	publicMux.HandleFunc("GET /keys", a.handleListKeys)
	publicMux.HandleFunc("POST /keys", a.handleCreateKey)
	publicMux.HandleFunc("POST /generate-key", a.handleCreateKey)
	publicMux.HandleFunc("DELETE /keys/{keyId}", a.handleDeleteKey)

	// Sessions Management (Zero friction - no PAT required!)
	publicMux.HandleFunc("GET /sessions", a.handleSessionList)
	publicMux.HandleFunc("POST /sessions", a.handleSessionCreate)
	publicMux.HandleFunc("POST /create-session", a.handleSessionCreate)
	publicMux.HandleFunc("GET /sessions/{id}", a.handleSessionGet)
	publicMux.HandleFunc("GET /sessions/{id}/qr", a.handleSessionQR)
	publicMux.HandleFunc("POST /sessions/{id}/pair", a.handleSessionPair)
	publicMux.HandleFunc("POST /sessions/{id}/logout", a.handleSessionLogout)
	publicMux.HandleFunc("DELETE /sessions/{id}", a.handleSessionDelete)
	publicMux.HandleFunc("POST /sessions/{id}/delete", a.handleSessionDelete)

	// Webhooks
	publicMux.HandleFunc("GET /webhooks/logs", a.handleListWebhookLogs)
	publicMux.HandleFunc("POST /webhooks/test", a.handleTestWebhook)

	// 2. Action / Protected Sub-Router (Requires valid API key: wac_sess_..., wac_key_..., wac_pat_..., or master key)
	protectedMux := http.NewServeMux()

	// WasenderAPI Universal Messaging & Calling
	protectedMux.HandleFunc("POST /send-message", a.handleWasenderSendMessage)
	protectedMux.HandleFunc("POST /send-call", a.handleWasenderSendCall)
	protectedMux.HandleFunc("GET /calls", a.handleTopLevelListCalls)
	protectedMux.HandleFunc("GET /calls/{callId}", a.handleTopLevelGetCall)
	protectedMux.HandleFunc("POST /calls/{callId}/accept", a.handleTopLevelAcceptCall)
	protectedMux.HandleFunc("POST /calls/{callId}/reject", a.handleTopLevelRejectCall)
	protectedMux.HandleFunc("POST /calls/{callId}/hangup", a.handleTopLevelEndCall)
	protectedMux.HandleFunc("DELETE /calls/{callId}", a.handleTopLevelEndCall)
	protectedMux.HandleFunc("POST /calls/{callId}/play", a.handleTopLevelPlayAudio)
	protectedMux.HandleFunc("GET /calls/{callId}/stream", a.handleTopLevelAudioStreamWS)

	// Scoped Session Messaging & VoIP
	protectedMux.HandleFunc("POST /sessions/{id}/messages/text", a.handleSendTextMessage)
	protectedMux.HandleFunc("POST /sessions/{id}/messages/media", a.handleSendMediaMessage)
	protectedMux.HandleFunc("POST /sessions/{id}/messages/location", a.handleSendLocationMessage)
	protectedMux.HandleFunc("GET /sessions/{id}/messages", a.handleListMessages)

	protectedMux.HandleFunc("POST /sessions/{id}/calls", a.handleStartCall)
	protectedMux.HandleFunc("GET /sessions/{id}/calls", a.handleListCalls)
	protectedMux.HandleFunc("GET /sessions/{id}/calls/{callId}", a.handleGetCall)
	protectedMux.HandleFunc("POST /sessions/{id}/calls/{callId}/accept", a.handleAcceptCall)
	protectedMux.HandleFunc("POST /sessions/{id}/calls/{callId}/reject", a.handleRejectCall)
	protectedMux.HandleFunc("DELETE /sessions/{id}/calls/{callId}", a.handleEndCall)
	protectedMux.HandleFunc("POST /sessions/{id}/calls/{callId}/play", a.handlePlayAudio)
	protectedMux.HandleFunc("POST /sessions/{id}/calls/{callId}/webrtc", a.handleWebRTCExchange)
	protectedMux.HandleFunc("GET /sessions/{id}/calls/{callId}/stream", a.handleAudioStreamWS)

	publicHandler := a.mw.RequireOptionalAuth(publicMux)
	protectedHandler := a.mw.RequireAuth(protectedMux)

	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		trimmed := path
		if strings.HasPrefix(trimmed, "/api/v1") {
			trimmed = strings.TrimPrefix(trimmed, "/api/v1")
		} else if strings.HasPrefix(trimmed, "/api") {
			trimmed = strings.TrimPrefix(trimmed, "/api")
		}

		if isActionProtected(trimmed, r.Method) {
			protectedHandler.ServeHTTP(w, r)
		} else {
			publicHandler.ServeHTTP(w, r)
		}
	})

	rootMux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiHandler))
	rootMux.Handle("/api/", http.StripPrefix("/api", apiHandler))

	// Phase 0 Task 11 & 13: Apply rate limiting and 10MB body size limit
	bodyLimited := a.mw.BodySizeLimit(10 * 1024 * 1024)(rootMux)
	rateLimited := a.mw.RateLimit(100.0/60.0, 20)(a.mw.CORS(a.mw.Logger(bodyLimited)))
	return rateLimited.(http.Handler)
}

func isActionProtected(path string, method string) bool {
	if strings.HasPrefix(path, "/send-message") ||
		strings.HasPrefix(path, "/send-call") ||
		strings.HasPrefix(path, "/calls") ||
		strings.Contains(path, "/messages") ||
		(strings.Contains(path, "/calls") && !strings.Contains(path, "/webhooks")) {
		return true
	}
	return false
}

// ---------------- Response Utilities ----------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]any{
		"success": false,
		"error":   message,
	})
}

// ---------------- Health & OpenAPI ----------------

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "healthy",
		"service":   "WacallerAPI",
		"version":   "1.0.0",
		"timestamp": time.Now().UTC(),
	})
}

func (a *API) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	userID, _ := r.Context().Value("user_id").(string)

	// For superadmin / master key: full hardware, memory, DB telemetry & platform stats
	if isAdmin {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		dbStats, _ := a.store.GetSystemStats(r.Context())
		sessions := a.sessions.ListSessions()
		connectedCount := 0
		for _, s := range sessions {
			if s.Status == "CONNECTED" {
				connectedCount++
			}
		}

		var dbFileSize int64
		if fi, err := os.Stat("wacaller.db"); err == nil {
			dbFileSize = fi.Size()
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"success":  true,
			"is_admin": true,
			"system": map[string]any{
				"status":          "operational",
				"uptime":          time.Since(a.startTime).Round(time.Second).String(),
				"started_at":      a.startTime,
				"goroutines":      runtime.NumGoroutine(),
				"num_cpu":         runtime.NumCPU(),
				"go_version":      runtime.Version(),
				"memory_alloc_mb": float64(mem.Alloc) / 1024 / 1024,
				"memory_sys_mb":   float64(mem.Sys) / 1024 / 1024,
				"gc_cycles":       mem.NumGC,
			},
			"database": map[string]any{
				"path":               "wacaller.db",
				"file_size_bytes":    dbFileSize,
				"file_size_kb":       float64(dbFileSize) / 1024,
				"journal_mode":       "WAL",
				"concurrency":        "multi-reader single-writer",
				"total_users":        dbStats.TotalUsers,
				"total_api_keys":     dbStats.TotalAPIKeys,
				"total_sessions":     dbStats.TotalSessions,
				"total_calls":        dbStats.TotalCalls,
				"total_messages":     dbStats.TotalMessages,
				"total_webhook_logs": dbStats.TotalWebhookLogs,
				"success_webhooks":   dbStats.SuccessWebhooks,
			},
			"sessions_summary": map[string]any{
				"total":     len(sessions),
				"connected": connectedCount,
			},
		})
		return
	}

	// For authenticated developers: tenant-isolated metrics
	if userID != "" {
		uStats, _ := a.store.GetUserStats(r.Context(), userID)
		userSessions := a.sessions.ListSessionsByUser(userID)
		connectedCount := 0
		for _, s := range userSessions {
			if s.Status == "CONNECTED" {
				connectedCount++
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"success":  true,
			"is_admin": false,
			"status":   "operational",
			"database": map[string]any{
				"total_sessions":     len(userSessions),
				"total_calls":        uStats.TotalCalls,
				"total_messages":     uStats.TotalMessages,
				"total_webhook_logs": uStats.TotalWebhookLogs,
			},
			"sessions_summary": map[string]any{
				"total":     len(userSessions),
				"connected": connectedCount,
			},
		})
		return
	}

	// For guest / unauthenticated:
	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"is_admin": false,
		"status":   "operational",
		"database": map[string]any{
			"total_sessions":     0,
			"total_calls":        0,
			"total_messages":     0,
			"total_webhook_logs": 0,
		},
		"sessions_summary": map[string]any{
			"total":     0,
			"connected": 0,
		},
	})
}

// ---------------- Stripe Subscriptions & Billing ----------------

type PlanInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PriceUSD    int    `json:"price_usd"`
	AmountCents int    `json:"amount_cents"`
	MaxSessions int    `json:"max_sessions"`
	Description string `json:"description"`
}

var AvailablePlans = map[string]PlanInfo{
	"basic": {
		ID:          "basic",
		Name:        "Basic",
		PriceUSD:    6,
		AmountCents: 600,
		MaxSessions: 1,
		Description: "1 Connected WhatsApp Number • Unlimited Messages & Calls • Webhook Notifications",
	},
	"pro": {
		ID:          "pro",
		Name:        "Pro",
		PriceUSD:    15,
		AmountCents: 1500,
		MaxSessions: 3,
		Description: "3 Connected WhatsApp Numbers • 16 kHz Audio Streaming • Priority WebSockets",
	},
	"plus": {
		ID:          "plus",
		Name:        "Plus",
		PriceUSD:    30,
		AmountCents: 3000,
		MaxSessions: 6,
		Description: "6 Connected WhatsApp Numbers • Advanced Webhook Retries • Priority Support",
	},
	"business": {
		ID:          "business",
		Name:        "Business",
		PriceUSD:    45,
		AmountCents: 4500,
		MaxSessions: 10,
		Description: "10 Connected WhatsApp Numbers • Dedicated WebRTC Infra • 99.9% SLA",
	},
}

func (a *API) handleStripeCheckout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plan       string `json:"plan"`
		SuccessURL string `json:"success_url"`
		CancelURL  string `json:"cancel_url"`
		Email      string `json:"email"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	planID := strings.ToLower(strings.TrimSpace(body.Plan))
	if planID == "" {
		planID = "basic"
	}
	planInfo, exists := AvailablePlans[planID]
	if !exists {
		planInfo = AvailablePlans["basic"]
	}

	userID, _ := r.Context().Value("user_id").(string)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Please sign in or register before subscribing to a plan.")
		return
	}

	successURL := body.SuccessURL
	if successURL == "" {
		successURL = fmt.Sprintf("http://%s/?billing=success&plan=%s", r.Host, planID)
	}
	cancelURL := body.CancelURL
	if cancelURL == "" {
		cancelURL = fmt.Sprintf("http://%s/?billing=cancel", r.Host)
	}

	// 1. If real Stripe secret key is configured, create live Stripe Checkout Session
	if a.cfg.StripeSecretKey != "" {
		data := url.Values{}
		data.Set("mode", "subscription")
		data.Set("success_url", successURL)
		data.Set("cancel_url", cancelURL)
		data.Set("client_reference_id", userID)
		if body.Email != "" {
			data.Set("customer_email", body.Email)
		}
		data.Set("line_items[0][price_data][currency]", "usd")
		data.Set("line_items[0][price_data][product_data][name]", fmt.Sprintf("WacallerAPI %s Plan", planInfo.Name))
		data.Set("line_items[0][price_data][product_data][description]", planInfo.Description)
		data.Set("line_items[0][price_data][unit_amount]", strconv.Itoa(planInfo.AmountCents))
		data.Set("line_items[0][price_data][recurring][interval]", "month")
		data.Set("line_items[0][quantity]", "1")
		data.Set("metadata[plan]", planID)
		data.Set("metadata[user_id]", userID)

		req, err := http.NewRequestWithContext(r.Context(), "POST", "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(data.Encode()))
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+a.cfg.StripeSecretKey)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				defer resp.Body.Close()
				var stripeRes struct {
					ID  string `json:"id"`
					URL string `json:"url"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&stripeRes); err == nil && stripeRes.URL != "" {
					_ = a.store.CreateOrUpdateSubscription(r.Context(), &store.SubscriptionRecord{
						UserID:          userID,
						Plan:            planID,
						Status:          "pending",
						MaxSessions:     planInfo.MaxSessions,
						StripeSessionID: stripeRes.ID,
						AmountCents:     planInfo.AmountCents,
						Currency:        "usd",
					})

					writeJSON(w, http.StatusOK, map[string]any{
						"success":      true,
						"checkout_url": stripeRes.URL,
						"session_id":   stripeRes.ID,
						"mode":         "stripe_live",
						"plan":         planInfo,
					})
					return
				}
			}
		}
	}

	// Phase 0: Billing fallback removed — no free plan activation
	writeError(w, http.StatusServiceUnavailable, "Billing is not configured or Stripe request failed. Please contact support.")
}

func (a *API) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	// Phase 0: Read raw body for signature verification
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB max
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	// Phase 0: Verify Stripe webhook signature
	if a.cfg.StripeWebhookSecret != "" {
		sigHeader := r.Header.Get("Stripe-Signature")
		if sigHeader == "" {
			a.log.Warn("stripe webhook: missing Stripe-Signature header")
			writeError(w, http.StatusBadRequest, "missing Stripe-Signature header")
			return
		}
		if !verifyStripeSignature(rawBody, sigHeader, a.cfg.StripeWebhookSecret) {
			a.log.Warn("stripe webhook: invalid signature")
			writeError(w, http.StatusBadRequest, "invalid webhook signature")
			return
		}
	} else {
		a.log.Warn("stripe webhook: STRIPE_WEBHOOK_SECRET not configured, rejecting webhook")
		writeError(w, http.StatusServiceUnavailable, "webhook verification not configured")
		return
	}

	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object map[string]any `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	// Phase 0: Event ID deduplication
	if event.ID != "" {
		if a.store.IsStripeEventProcessed(r.Context(), event.ID) {
			a.log.Info("stripe webhook: duplicate event, skipping", "event_id", event.ID)
			writeJSON(w, http.StatusOK, map[string]any{"received": true})
			return
		}
	}

	switch event.Type {
	case "checkout.session.completed":
		sessionObj := event.Data.Object
		sessionID, _ := sessionObj["id"].(string)
		custID, _ := sessionObj["customer"].(string)
		subID, _ := sessionObj["subscription"].(string)
		metadata, _ := sessionObj["metadata"].(map[string]any)
		planID := "basic"
		userID := ""
		if metadata != nil {
			if p, ok := metadata["plan"].(string); ok {
				planID = p
			}
			if u, ok := metadata["user_id"].(string); ok {
				userID = u
			}
		}
		planInfo, exists := AvailablePlans[planID]
		if !exists {
			planInfo = AvailablePlans["basic"]
		}

		if userID != "" {
			_ = a.store.CreateOrUpdateSubscription(r.Context(), &store.SubscriptionRecord{
				UserID:               userID,
				Plan:                 planID,
				Status:               "active",
				MaxSessions:          planInfo.MaxSessions,
				StripeCustomerID:     custID,
				StripeSubscriptionID: subID,
				StripeSessionID:      sessionID,
				AmountCents:          planInfo.AmountCents,
				Currency:             "usd",
			})
		}

	case "customer.subscription.deleted":
		subObj := event.Data.Object
		subID, _ := subObj["id"].(string)
		if subID != "" {
			_ = a.store.CreateOrUpdateSubscription(r.Context(), &store.SubscriptionRecord{
				ID:     subID,
				Status: "canceled",
			})
		}
	}

	// Mark event as processed for deduplication
	if event.ID != "" {
		_ = a.store.MarkStripeEventProcessed(r.Context(), event.ID, event.Type)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"received": true,
	})
}

func (a *API) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value("user_id").(string)
	if userID == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"success":         true,
			"subscription":    nil,
			"current_plan":    nil,
			"available_plans": AvailablePlans,
			"stripe_enabled":  a.cfg.StripeSecretKey != "",
			"publishable_key": a.cfg.StripePublishableKey,
		})
		return
	}

	sub, err := a.store.GetSubscriptionByUserID(r.Context(), userID)
	if err != nil || sub == nil {
		sub = &store.SubscriptionRecord{
			ID:          "sub_trial",
			UserID:      userID,
			Plan:        "basic",
			Status:      "trial",
			MaxSessions: 1,
			AmountCents: 0,
			Currency:    "usd",
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}
	}

	currentPlan := AvailablePlans[sub.Plan]
	if currentPlan.ID == "" {
		currentPlan = AvailablePlans["basic"]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"subscription":    sub,
		"current_plan":    currentPlan,
		"available_plans": AvailablePlans,
		"stripe_enabled":  a.cfg.StripeSecretKey != "",
		"publishable_key": a.cfg.StripePublishableKey,
	})
}

// Phase 0: Direct plan activation removed from customer-facing routes.
// Use /admin/users/{userId}/grant-plan for admin-only plan grants.

// verifyStripeSignature verifies a Stripe webhook signature.
// Stripe signs webhooks using HMAC-SHA256 with the format:
// t=<timestamp>,v1=<signature>
func verifyStripeSignature(payload []byte, sigHeader, secret string) bool {
	var timestamp, signature string
	for _, part := range strings.Split(sigHeader, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signature = kv[1]
		}
	}
	if timestamp == "" || signature == "" {
		return false
	}

	// Reject events older than 5 minutes to prevent replay attacks
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if time.Since(time.Unix(ts, 0)).Abs() > 5*time.Minute {
		return false
	}

	// Compute expected signature: HMAC-SHA256(secret, "timestamp.payload")
	signed := fmt.Sprintf("%s.%s", timestamp, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

// requireSessionOwnership checks that the authenticated principal owns the session.
// Returns true if access is DENIED (caller should return immediately).
// Phase 0: Fix for broken ownership checks that let unauthenticated users bypass authorization.
func (a *API) requireSessionOwnership(w http.ResponseWriter, r *http.Request, sess *session.Session) bool {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	if isAdmin {
		return false // admin has access
	}
	userID, _ := r.Context().Value("user_id").(string)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required")
		return true // denied
	}
	if sess.UserID() != "" && sess.UserID() != userID {
		writeError(w, http.StatusForbidden, "access denied to this session")
		return true // denied
	}
	return false // allowed
}

// requireAuth checks that the request has an authenticated principal.
// Returns the userID or writes an error and returns "" if unauthenticated.
func (a *API) requireAuth(w http.ResponseWriter, r *http.Request) (string, bool) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	if isAdmin {
		return "__admin__", true
	}
	userID, _ := r.Context().Value("user_id").(string)
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required")
		return "", false
	}
	return userID, true
}
// ---------------- Sessions ----------------

func (a *API) handleSessionList(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	userID, _ := r.Context().Value("user_id").(string)

	var sessions []session.SessionInfo
	if isAdmin {
		sessions = a.sessions.ListSessions()
	} else if userID != "" {
		sessions = a.sessions.ListSessionsByUser(userID)
	} else {
		sessions = []session.SessionInfo{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"sessions": sessions,
	})
}

func (a *API) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	userID, _ := r.Context().Value("user_id").(string)

	var body struct {
		Name       string `json:"name"`
		WebhookURL string `json:"webhook_url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if !isAdmin && userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required. Please sign in or pass a valid Personal Access Token (PAT).")
		return
	}

	// Enforce quota for non-admins
	if !isAdmin && userID != "" {
		sub, _ := a.store.GetSubscriptionByUserID(r.Context(), userID)
		maxLines := 1
		if sub != nil && sub.MaxSessions > 0 {
			maxLines = sub.MaxSessions
		}
		userSessions := a.sessions.ListSessionsByUser(userID)
		if len(userSessions) >= maxLines {
			writeError(w, http.StatusForbidden, fmt.Sprintf("WhatsApp lines quota reached for your plan (%d maximum). Please upgrade your subscription.", maxLines))
			return
		}
	}

	sess, err := a.sessions.CreateSession(r.Context(), body.Name, body.WebhookURL, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	info := sess.Info()
	writeJSON(w, http.StatusCreated, map[string]any{
		"success":   true,
		"session":   info,
		"apiKey":    info.APIKey,
		"sessionId": info.ID,
		"status":    info.Status,
		"qr":        info.QR,
	})
}

func (a *API) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Fixed ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"session": sess.Info(),
	})
}

func (a *API) handleSessionQR(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Fixed ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	qr, status := sess.GetQR(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"status":  status,
		"qr":      qr,
	})
}

func (a *API) handleSessionPair(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Fixed ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	var body struct {
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Phone == "" {
		writeError(w, http.StatusBadRequest, "phone number is required (e.g. +94771234567)")
		return
	}

	code, err := sess.PairPhone(r.Context(), body.Phone)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"pairing_code": code,
		"message":      "Enter this code into WhatsApp > Linked Devices > Link with phone number",
	})
}

func (a *API) handleSessionLogout(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Fixed ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	if err := sess.Logout(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "session logged out successfully",
	})
}

func (a *API) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if sess, ok := a.sessions.GetSession(id); ok {
		// Phase 0: Fixed ownership check
		if a.requireSessionOwnership(w, r, sess) {
			return
		}
	} else {
		// Phase 0: Require auth even if session doesn't exist in memory
		if _, ok := a.requireAuth(w, r); !ok {
			return
		}
	}

	if err := a.sessions.DeleteSession(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "session deleted successfully",
	})
}

// ---------------- Messaging ----------------

func (a *API) handleSendTextMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Added ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	var req session.TextMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}
	if req.To == "" || req.Text == "" {
		writeError(w, http.StatusBadRequest, "'to' and 'text' fields are required")
		return
	}

	msgID, err := sess.SendTextMessage(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message_id": msgID,
	})
}

func (a *API) handleSendMediaMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Added ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	var req session.MediaMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}
	if req.To == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "'to' and 'url' fields are required")
		return
	}
	if req.Type == "" {
		req.Type = "image"
	}

	msgID, err := sess.SendMediaMessage(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message_id": msgID,
	})
}

func (a *API) handleSendLocationMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Added ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	var req session.LocationMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}
	if req.To == "" {
		writeError(w, http.StatusBadRequest, "'to' field is required")
		return
	}

	msgID, err := sess.SendLocationMessage(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"message_id": msgID,
	})
}

func (a *API) handleListMessages(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	userID, _ := r.Context().Value("user_id").(string)
	id := r.PathValue("id")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	var messages []store.MessageRecord
	var err error

	if id != "" {
		if sess, ok := a.sessions.GetSession(id); ok {
			// Phase 0: Fixed ownership check
			if a.requireSessionOwnership(w, r, sess) {
				return
			}
		}
		messages, err = a.store.ListMessages(r.Context(), id, limit)
	} else if isAdmin {
		messages, err = a.store.ListMessages(r.Context(), "", limit)
	} else if userID != "" {
		messages, err = a.store.ListMessagesByUser(r.Context(), userID, "", limit)
	} else {
		messages = []store.MessageRecord{}
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"messages": messages,
	})
}

// ---------------- VoIP Calling (Core Differentiator) ----------------

func (a *API) handleStartCall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Added ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	var opts session.DialOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}
	if opts.To == "" {
		writeError(w, http.StatusBadRequest, "'to' phone number is required")
		return
	}

	callCtx, err := sess.StartCall(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	host := r.Host
	scheme := "ws"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "wss"
	}
	streamWSURL := fmt.Sprintf("%s://%s/api/v1/sessions/%s/calls/%s/stream", scheme, host, id, callCtx.CallID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"success":    true,
		"call":       callCtx.Info(),
		"stream_url": streamWSURL,
	})
}

func (a *API) handleListCalls(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Added ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	// Active calls from memory
	activeCalls := sess.ListCalls()

	// Past calls from database
	history, _ := a.store.ListCalls(r.Context(), id, 30)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"active_calls": activeCalls,
		"history":      history,
	})
}

func (a *API) handleGetCall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	callID := r.PathValue("callId")

	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Added ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	callCtx, found := sess.GetCall(callID)
	if found {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"call":    callCtx.Info(),
		})
		return
	}

	// Check DB
	calls, err := a.store.ListCalls(r.Context(), id, 50)
	if err == nil {
		for _, c := range calls {
			if c.CallID == callID {
				writeJSON(w, http.StatusOK, map[string]any{
					"success": true,
					"call":    c,
				})
				return
			}
		}
	}

	writeError(w, http.StatusNotFound, "call not found")
}

func (a *API) handleAcceptCall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	callID := r.PathValue("callId")

	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Added ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	if err := sess.AcceptCall(r.Context(), callID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "call accepted",
	})
}

func (a *API) handleRejectCall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	callID := r.PathValue("callId")

	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Added ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	if err := sess.RejectCall(r.Context(), callID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "call rejected",
	})
}

func (a *API) handleEndCall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	callID := r.PathValue("callId")

	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Added ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	if err := sess.EndCall(callID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "call terminated",
	})
}

func (a *API) handlePlayAudio(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	callID := r.PathValue("callId")

	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Added ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	callCtx, found := sess.GetCall(callID)
	if !found {
		writeError(w, http.StatusNotFound, "active call not found")
		return
	}

	var body struct {
		AudioURL string `json:"audio_url"`
		WAVData  string `json:"wav_base64"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if body.AudioURL != "" {
		if err := callCtx.PlayAudioURL(context.Background(), body.AudioURL); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to play audio url: "+err.Error())
			return
		}
	} else if body.WAVData != "" {
		// Phase 0 Task 13: Limit decoded audio payload to 5MB
		if len(body.WAVData) > 7*1024*1024 {
			writeError(w, http.StatusRequestEntityTooLarge, "audio base64 payload exceeds limit")
			return
		}
		wavBytes, err := base64.StdEncoding.DecodeString(body.WAVData)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid wav_base64 data")
			return
		}
		if len(wavBytes) > 5*1024*1024 {
			writeError(w, http.StatusRequestEntityTooLarge, "decoded audio payload exceeds 5MB limit")
			return
		}
		samples, err := audio.DecodeWAVToPCM16k(strings.NewReader(string(wavBytes)))
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to decode wav: "+err.Error())
			return
		}
		if err := callCtx.PlayAudioSamples(context.Background(), samples); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		writeError(w, http.StatusBadRequest, "either 'audio_url' or 'wav_base64' is required")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "audio playback started",
	})
}

func (a *API) handleWebRTCExchange(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	callID := r.PathValue("callId")

	sess, ok := a.sessions.GetSession(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Phase 0: Added ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	var body struct {
		SDP string `json:"sdp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SDP == "" {
		writeError(w, http.StatusBadRequest, "offer sdp is required")
		return
	}

	answerSDP, err := sess.AttachWebRTC(callID, body.SDP)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"sdp":     answerSDP,
	})
}

// ---------------- WebSocket Audio Streaming Gateway ----------------

// handleAudioStreamWS allows developers and AI bots to stream bi-directional 16kHz audio in real-time.
func (a *API) handleAudioStreamWS(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	callID := r.PathValue("callId")

	sess, ok := a.sessions.GetSession(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	// Phase 0: Added ownership check
	if a.requireSessionOwnership(w, r, sess) {
		return
	}

	callCtx, found := sess.GetCall(callID)
	if !found {
		http.Error(w, "active call not found", http.StatusNotFound)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		a.log.Warn("websocket accept failed", "err", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "call stream ended")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	subID := fmt.Sprintf("ws_%d", time.Now().UnixNano())
	audioCh := callCtx.SubscribeAudio(subID)
	defer callCtx.UnsubscribeAudio(subID)

	a.log.Info("audio stream websocket connected", "call_id", callID, "sub_id", subID)

	// Downlink Goroutine: WhatsApp Peer Audio -> WebSocket Client
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pcm, ok := <-audioCh:
				if !ok {
					return
				}
				// Convert float32 to 16 kHz 16-bit linear PCM
				pcmBytes := media.PCMFloat32ToInt16LE(pcm)
				if err := conn.Write(ctx, websocket.MessageBinary, pcmBytes); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// Uplink Loop: WebSocket Client -> WhatsApp Audio Injection
	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			break
		}

		if msgType == websocket.MessageBinary {
			// Raw 16 kHz 16-bit linear PCM binary frame
			samples := media.PCMInt16LEToFloat32(data)
			callCtx.InjectAudio(samples)
		} else if msgType == websocket.MessageText {
			// Optional JSON protocol support: {"event":"media","media":{"payload":"<base64>"}}
			var event struct {
				Event string `json:"event"`
				Media struct {
					Payload string `json:"payload"`
				} `json:"media"`
			}
			if err := json.Unmarshal(data, &event); err == nil {
				if event.Event == "media" && event.Media.Payload != "" {
					if rawBytes, err := base64.StdEncoding.DecodeString(event.Media.Payload); err == nil {
						samples := media.PCMInt16LEToFloat32(rawBytes)
						callCtx.InjectAudio(samples)
					}
				} else if event.Event == "stop" {
					break
				}
			}
		}
	}

	a.log.Info("audio stream websocket closed", "call_id", callID, "sub_id", subID)
}

// ---------------- API Keys ----------------

func (a *API) handleListKeys(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	userID, _ := r.Context().Value("user_id").(string)

	var keys []store.APIKey
	var err error
	if isAdmin {
		keys, err = a.store.ListAPIKeys(r.Context())
	} else if userID != "" {
		keys, err = a.store.ListAPIKeysByUser(r.Context(), userID)
	} else {
		keys = []store.APIKey{}
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"keys":    keys,
	})
}

func (a *API) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	userID, _ := r.Context().Value("user_id").(string)

	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Name == "" {
		body.Name = "Developer API Key"
	}

	if !isAdmin && userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required to generate custom API keys")
		return
	}

	key, err := a.store.CreateAPIKey(r.Context(), body.Name, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"success": true,
		"key":     key,
		"apiKey":  key.Key,
		"name":    key.Name,
		"warning": "Save this key now! The raw key will never be shown again.",
	})
}

func (a *API) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	userID, _ := r.Context().Value("user_id").(string)
	keyID := r.PathValue("keyId")

	// Phase 0: Verify ownership before deletion
	if !isAdmin {
		if userID == "" {
			writeError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		// Check that this key belongs to the authenticated user
		key, err := a.store.GetAPIKeyByID(r.Context(), keyID)
		if err != nil {
			writeError(w, http.StatusNotFound, "API key not found")
			return
		}
		if key.UserID != userID {
			writeError(w, http.StatusForbidden, "access denied to this API key")
			return
		}
	}

	if err := a.store.DeleteAPIKey(r.Context(), keyID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "API key revoked",
	})
}

// ---------------- Developer Auth & Personal Access Tokens (PAT) ----------------

func (a *API) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "name, email, and password are required")
		return
	}
	if body.Name == "" {
		body.Name = strings.Split(body.Email, "@")[0]
	}

	user, err := a.store.CreateUser(r.Context(), body.Name, body.Email, body.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "registration failed: "+err.Error())
		return
	}

	sub, _ := a.store.GetSubscriptionByUserID(r.Context(), user.ID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"success": true,
		"message": "account created successfully (7-day free trial active)",
		"user": map[string]any{
			"id":            user.ID,
			"name":          user.Name,
			"email":         user.Email,
			"role":          user.Role,
			"pat_token":     user.PATToken,
			"trial_ends_at": user.TrialEndsAt,
			"is_trial":      true,
			"days_left":     7,
			"created_at":    user.CreatedAt,
		},
		"subscription": sub,
		"token":        user.PATToken,
	})
}

func (a *API) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := a.store.AuthenticateUser(r.Context(), body.Email, body.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	sub, _ := a.store.GetSubscriptionByUserID(r.Context(), user.ID)
	now := time.Now().UTC()
	isTrial := user.TrialEndsAt.After(now)
	daysLeft := 0
	if isTrial {
		daysLeft = int(time.Until(user.TrialEndsAt).Hours()/24) + 1
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"user": map[string]any{
			"id":            user.ID,
			"name":          user.Name,
			"email":         user.Email,
			"role":          user.Role,
			"pat_token":     user.PATToken,
			"trial_ends_at": user.TrialEndsAt,
			"is_trial":      isTrial,
			"days_left":     daysLeft,
			"created_at":    user.CreatedAt,
		},
		"subscription": sub,
		"token":        user.PATToken,
	})
}

func (a *API) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value("user_id").(string)

	var user *store.User
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		user, _ = a.store.GetUserByPAT(r.Context(), token)
	}
	if user == nil && userID != "" {
		user, _ = a.store.GetUserByID(r.Context(), userID)
	}
	if user == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"user":    nil,
		})
		return
	}

	sub, _ := a.store.GetSubscriptionByUserID(r.Context(), user.ID)
	now := time.Now().UTC()
	isTrial := user.TrialEndsAt.After(now)
	daysLeft := 0
	if isTrial {
		daysLeft = int(time.Until(user.TrialEndsAt).Hours()/24) + 1
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"user": map[string]any{
			"id":            user.ID,
			"name":          user.Name,
			"email":         user.Email,
			"role":          user.Role,
			"pat_token":     user.PATToken,
			"trial_ends_at": user.TrialEndsAt,
			"is_trial":      isTrial,
			"days_left":     daysLeft,
			"created_at":    user.CreatedAt,
		},
		"subscription": sub,
	})
}

func (a *API) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value("user_id").(string)
	if userID == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if u, err := a.store.GetUserByPAT(r.Context(), token); err == nil && u != nil {
				userID = u.ID
			}
		}
	}
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required to update profile")
		return
	}

	var body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.Email == "" {
		writeError(w, http.StatusBadRequest, "name and email are required")
		return
	}

	user, err := a.store.UpdateUserProfile(r.Context(), userID, body.Name, body.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update profile: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "profile updated successfully",
		"user":    user,
	})
}

func (a *API) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value("user_id").(string)
	if userID == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if u, err := a.store.GetUserByPAT(r.Context(), token); err == nil && u != nil {
				userID = u.ID
			}
		}
	}
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	_ = a.store.DeleteUser(r.Context(), userID)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "account deleted successfully",
	})
}

func (a *API) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	if !isAdmin {
		writeError(w, http.StatusForbidden, "Admin access required")
		return
	}

	users, err := a.store.ListUsersWithSubscriptions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"users":   users,
		"total":   len(users),
	})
}

func (a *API) handleAdminGrantPlan(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	if !isAdmin {
		writeError(w, http.StatusForbidden, "Admin access required")
		return
	}

	userId := r.PathValue("userId")
	if userId == "" {
		writeError(w, http.StatusBadRequest, "user id required")
		return
	}

	var body struct {
		Plan        string `json:"plan"`         // "basic", "pro", "plus", "business", "lifetime"
		MaxSessions int    `json:"max_sessions"` // 1, 3, 10, 50, etc.
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body.Plan = "pro"
		body.MaxSessions = 3
	}
	if body.Plan == "" {
		body.Plan = "pro"
	}
	if body.MaxSessions <= 0 {
		switch body.Plan {
		case "basic":
			body.MaxSessions = 1
		case "pro":
			body.MaxSessions = 3
		case "plus":
			body.MaxSessions = 10
		case "business", "lifetime":
			body.MaxSessions = 50
		default:
			body.MaxSessions = 1
		}
	}

	_, err := a.store.GrantPackage(r.Context(), userId, body.Plan, body.MaxSessions, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to grant package: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": fmt.Sprintf("Granted %s plan (%d sessions) to user %s", strings.ToUpper(body.Plan), body.MaxSessions, userId),
	})
}

func (a *API) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	if !isAdmin {
		writeError(w, http.StatusForbidden, "Admin access required")
		return
	}

	userId := r.PathValue("userId")
	if userId == "" {
		writeError(w, http.StatusBadRequest, "user id required")
		return
	}
	_ = a.store.DeleteUser(r.Context(), userId)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "user deleted",
	})
}

func (a *API) handleRegeneratePAT(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value("user_id").(string)
	if userID == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if u, err := a.store.GetUserByPAT(r.Context(), token); err == nil && u != nil {
				userID = u.ID
			}
		}
	}
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Authentication required to regenerate PAT")
		return
	}

	newPAT, err := a.store.RegeneratePAT(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"pat_token": newPAT,
		"message":   "new Personal Access Token generated",
	})
}

func (a *API) handleGetPATDetails(w http.ResponseWriter, r *http.Request) {
	var user *store.User
	userID, _ := r.Context().Value("user_id").(string)
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		user, _ = a.store.GetUserByPAT(r.Context(), token)
	}
	if user == nil && userID != "" {
		user, _ = a.store.GetUserByID(r.Context(), userID)
	}

	if user == nil {
		writeError(w, http.StatusUnauthorized, "Authentication required to view PAT details")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"pat": map[string]any{
			"token":       user.PATToken,
			"name":        fmt.Sprintf("%s's Personal Access Token", user.Name),
			"created_at":  user.CreatedAt,
			"permissions": []string{"sessions:manage", "calls:full", "messages:send", "webhooks:read"},
		},
	})
}

// ---------------- Webhooks ----------------

func (a *API) handleListWebhookLogs(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	userID, _ := r.Context().Value("user_id").(string)

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	var logs []store.WebhookLogRecord
	var err error
	if isAdmin {
		logs, err = a.store.ListWebhookLogs(r.Context(), limit)
	} else if userID != "" {
		logs, err = a.store.ListWebhookLogsByUser(r.Context(), userID, limit)
	} else {
		logs = []store.WebhookLogRecord{}
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"logs":    logs,
	})
}

func (a *API) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetURL string `json:"target_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TargetURL == "" {
		writeError(w, http.StatusBadRequest, "target_url is required")
		return
	}

	a.dispatcher.Dispatch("test_session", body.TargetURL, webhook.EventType("webhook.test_ping"), map[string]any{
		"message":   "Hello from WacallerAPI webhook test!",
		"timestamp": time.Now().Unix(),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "test webhook dispatched to queue",
	})
}

// ---------------- OpenAPI Spec JSON ----------------

func (a *API) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	spec := GetOpenAPISpec()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(spec))
}

// ---------------- WasenderAPI Session Resolver & Helpers ----------------

func (a *API) resolveSession(r *http.Request) (*session.Session, error) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	userID, _ := r.Context().Value("user_id").(string)

	// 1. Path param "id"
	if id := r.PathValue("id"); id != "" {
		if sess, ok := a.sessions.GetSession(id); ok {
			if !isAdmin && userID != "" && sess.UserID() != "" && sess.UserID() != userID {
				return nil, fmt.Errorf("access denied to session %s", id)
			}
			return sess, nil
		}
	}

	// 2. Context session_id from middleware (session-specific API key)
	if ctxSessID, ok := r.Context().Value("session_id").(string); ok && ctxSessID != "" {
		if sess, ok := a.sessions.GetSession(ctxSessID); ok {
			return sess, nil
		}
	}

	// 3. Header X-Session-ID
	if headerID := r.Header.Get("X-Session-ID"); headerID != "" {
		if sess, ok := a.sessions.GetSession(headerID); ok {
			if !isAdmin && userID != "" && sess.UserID() != "" && sess.UserID() != userID {
				return nil, fmt.Errorf("access denied to session %s", headerID)
			}
			return sess, nil
		}
	}

	// 4. Query param session_id
	if queryID := r.URL.Query().Get("session_id"); queryID != "" {
		if sess, ok := a.sessions.GetSession(queryID); ok {
			if !isAdmin && userID != "" && sess.UserID() != "" && sess.UserID() != userID {
				return nil, fmt.Errorf("access denied to session %s", queryID)
			}
			return sess, nil
		}
	}

	// 5. Fall back to user's first connected session or first created session
	if !isAdmin && userID != "" {
		userSessions := a.sessions.ListSessionsByUser(userID)
		for _, s := range userSessions {
			if s.Status == "CONNECTED" {
				if realSess, ok := a.sessions.GetSession(s.ID); ok {
					return realSess, nil
				}
			}
		}
		for _, s := range userSessions {
			if realSess, ok := a.sessions.GetSession(s.ID); ok {
				return realSess, nil
			}
		}
		return nil, fmt.Errorf("no active WhatsApp session found for your account; please create and link a session first")
	}

	if sess, ok := a.sessions.FirstConnectedSession(); ok {
		return sess, nil
	}

	return nil, fmt.Errorf("no active WhatsApp session found; please create and link a session first")
}

func (a *API) findCallContext(callID string) (*session.Session, *session.CallContext, bool) {
	for _, s := range a.sessions.ListSessions() {
		sess, ok := a.sessions.GetSession(s.ID)
		if !ok {
			continue
		}
		if c, found := sess.GetCall(callID); found {
			return sess, c, true
		}
	}
	return nil, nil, false
}

// ---------------- WasenderAPI Top-Level Unified Handlers ----------------

// handleWasenderSendMessage handles WasenderAPI's POST /api/send-message (text, imageUrl, videoUrl, audioUrl, documentUrl, location)
func (a *API) handleWasenderSendMessage(w http.ResponseWriter, r *http.Request) {
	sess, err := a.resolveSession(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req session.UnifiedMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}
	if req.To == "" {
		writeError(w, http.StatusBadRequest, "'to' field is required")
		return
	}

	msgID, err := sess.SendUnifiedMessage(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"messageId": msgID,
		"status":    "sent",
	})
}

// handleWasenderSendCall handles WasenderAPI-style calling: POST /api/send-call
func (a *API) handleWasenderSendCall(w http.ResponseWriter, r *http.Request) {
	sess, err := a.resolveSession(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		To       string `json:"to"`
		AudioURL string `json:"audioUrl"`
		IsVideo  bool   `json:"isVideo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.To == "" {
		writeError(w, http.StatusBadRequest, "'to' phone number is required")
		return
	}

	callCtx, err := sess.StartCall(r.Context(), session.DialOptions{
		To:       body.To,
		AudioURL: body.AudioURL,
		IsVideo:  body.IsVideo,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	host := r.Host
	scheme := "ws"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "wss"
	}
	streamWSURL := fmt.Sprintf("%s://%s/api/calls/%s/stream", scheme, host, callCtx.CallID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"success":   true,
		"callId":    callCtx.CallID,
		"status":    "initiating",
		"streamUrl": streamWSURL,
		"call":      callCtx.Info(),
	})
}

// Top-level calls handlers (auto-resolved across sessions or by call ID)

func (a *API) handleTopLevelListCalls(w http.ResponseWriter, r *http.Request) {
	isAdmin, _ := r.Context().Value("is_admin").(bool)
	userID, _ := r.Context().Value("user_id").(string)

	if isAdmin {
		allActive := []session.CallInfo{}
		for _, s := range a.sessions.ListSessions() {
			if realSess, ok := a.sessions.GetSession(s.ID); ok {
				allActive = append(allActive, realSess.ListCalls()...)
			}
		}
		history, _ := a.store.ListCalls(r.Context(), "", 50)
		writeJSON(w, http.StatusOK, map[string]any{
			"success":      true,
			"active_calls": allActive,
			"history":      history,
		})
		return
	}

	if userID != "" {
		userSessions := a.sessions.ListSessionsByUser(userID)
		allActive := []session.CallInfo{}
		for _, s := range userSessions {
			if realSess, ok := a.sessions.GetSession(s.ID); ok {
				allActive = append(allActive, realSess.ListCalls()...)
			}
		}
		history, _ := a.store.ListCallsByUser(r.Context(), userID, "", 50)
		writeJSON(w, http.StatusOK, map[string]any{
			"success":      true,
			"active_calls": allActive,
			"history":      history,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"active_calls": []session.CallInfo{},
		"history":      []store.CallRecord{},
	})
}

func (a *API) handleTopLevelGetCall(w http.ResponseWriter, r *http.Request) {
	callID := r.PathValue("callId")
	if _, callCtx, found := a.findCallContext(callID); found {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"call":    callCtx.Info(),
		})
		return
	}

	// Check DB
	calls, err := a.store.ListCalls(r.Context(), "", 100)
	if err == nil {
		for _, c := range calls {
			if c.CallID == callID {
				writeJSON(w, http.StatusOK, map[string]any{
					"success": true,
					"call":    c,
				})
				return
			}
		}
	}

	writeError(w, http.StatusNotFound, "call not found")
}

func (a *API) handleTopLevelAcceptCall(w http.ResponseWriter, r *http.Request) {
	callID := r.PathValue("callId")
	sess, _, found := a.findCallContext(callID)
	if !found {
		writeError(w, http.StatusNotFound, "call not found")
		return
	}

	if err := sess.AcceptCall(r.Context(), callID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "call accepted",
	})
}

func (a *API) handleTopLevelRejectCall(w http.ResponseWriter, r *http.Request) {
	callID := r.PathValue("callId")
	sess, _, found := a.findCallContext(callID)
	if !found {
		writeError(w, http.StatusNotFound, "call not found")
		return
	}

	if err := sess.RejectCall(r.Context(), callID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "call rejected",
	})
}

func (a *API) handleTopLevelEndCall(w http.ResponseWriter, r *http.Request) {
	callID := r.PathValue("callId")
	sess, _, found := a.findCallContext(callID)
	if !found {
		writeError(w, http.StatusNotFound, "call not found")
		return
	}

	if err := sess.EndCall(callID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "call terminated",
	})
}

func (a *API) handleTopLevelPlayAudio(w http.ResponseWriter, r *http.Request) {
	callID := r.PathValue("callId")
	_, callCtx, found := a.findCallContext(callID)
	if !found {
		writeError(w, http.StatusNotFound, "active call not found")
		return
	}

	var body struct {
		AudioURL string `json:"audioUrl"`
		WAVData  string `json:"wavBase64"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if body.AudioURL != "" {
		if err := callCtx.PlayAudioURL(context.Background(), body.AudioURL); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to play audio: "+err.Error())
			return
		}
	} else if body.WAVData != "" {
		wavBytes, err := base64.StdEncoding.DecodeString(body.WAVData)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid wavBase64 data")
			return
		}
		samples, err := audio.DecodeWAVToPCM16k(strings.NewReader(string(wavBytes)))
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to decode wav: "+err.Error())
			return
		}
		if err := callCtx.PlayAudioSamples(context.Background(), samples); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		writeError(w, http.StatusBadRequest, "'audioUrl' or 'wavBase64' is required")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "audio playback started",
	})
}

func (a *API) handleTopLevelAudioStreamWS(w http.ResponseWriter, r *http.Request) {
	callID := r.PathValue("callId")
	_, callCtx, found := a.findCallContext(callID)
	if !found {
		http.Error(w, "active call not found", http.StatusNotFound)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		a.log.Warn("websocket accept failed", "err", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "call stream ended")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	subID := fmt.Sprintf("ws_%d", time.Now().UnixNano())
	audioCh := callCtx.SubscribeAudio(subID)
	defer callCtx.UnsubscribeAudio(subID)

	a.log.Info("top-level audio stream websocket connected", "call_id", callID, "sub_id", subID)

	// Downlink Goroutine: WhatsApp Peer Audio -> WebSocket Client
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pcm, ok := <-audioCh:
				if !ok {
					return
				}
				pcmBytes := media.PCMFloat32ToInt16LE(pcm)
				if err := conn.Write(ctx, websocket.MessageBinary, pcmBytes); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// Uplink Loop: WebSocket Client -> WhatsApp Audio Injection
	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			break
		}

		if msgType == websocket.MessageBinary {
			samples := media.PCMInt16LEToFloat32(data)
			callCtx.InjectAudio(samples)
		} else if msgType == websocket.MessageText {
			var event struct {
				Event string `json:"event"`
				Media struct {
					Payload string `json:"payload"`
				} `json:"media"`
			}
			if err := json.Unmarshal(data, &event); err == nil {
				if event.Event == "media" && event.Media.Payload != "" {
					if rawBytes, err := base64.StdEncoding.DecodeString(event.Media.Payload); err == nil {
						samples := media.PCMInt16LEToFloat32(rawBytes)
						callCtx.InjectAudio(samples)
					}
				} else if event.Event == "stop" {
					break
				}
			}
		}
	}

	a.log.Info("top-level audio stream websocket closed", "call_id", callID, "sub_id", subID)
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"wacallerapi/internal/config"
	"wacallerapi/internal/session"
	"wacallerapi/internal/store"
	"wacallerapi/internal/webhook"

	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func setupTestAPI(t *testing.T) (*API, *store.Store, func()) {
	ctx := context.Background()
	dbFile := "test_api.db"
	_ = os.Remove(dbFile)

	st, err := store.Open(ctx, dbFile)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	container := sqlstore.NewWithDB(st.DB(), "sqlite3", waLog.Noop)
	if err := container.Upgrade(ctx); err != nil {
		t.Fatalf("failed to upgrade sqlstore: %v", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfg := &config.Config{Addr: ":8080"}
	disp := webhook.NewDispatcher(st, "", "", log)
	sm := session.NewSessionManager(ctx, container, st, disp, log, 4)

	api := New(cfg, st, sm, disp, log)

	cleanup := func() {
		_ = st.Close()
		_ = os.Remove(dbFile)
	}

	return api, st, cleanup
}

func TestHealthEndpoint(t *testing.T) {
	api, _, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()

	api.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "healthy" {
		t.Fatalf("expected status healthy, got %v", resp["status"])
	}
}

func TestOpenAPISpecEndpoint(t *testing.T) {
	api, _, cleanup := setupTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	w := httptest.NewRecorder()

	api.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var spec map[string]any
	if err := json.NewDecoder(w.Body).Decode(&spec); err != nil {
		t.Fatalf("invalid json openapi spec: %v", err)
	}
	if spec["openapi"] != "3.0.3" {
		t.Fatalf("expected openapi 3.0.3, got %v", spec["openapi"])
	}
}

func TestAPIKeyCreationAndProtection(t *testing.T) {
	api, st, cleanup := setupTestAPI(t)
	defer cleanup()

	// 1. Initial state (no keys exist) -> open access works
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	w := httptest.NewRecorder()
	api.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for open access, got %d", w.Code)
	}

	// 2. Create an API Key
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/keys", bytes.NewReader([]byte(`{"name":"Dev Key"}`)))
	createReq.Header.Set("Content-Type", "application/json")
	wCreate := httptest.NewRecorder()
	api.Routes().ServeHTTP(wCreate, createReq)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 created, got %d: %s", wCreate.Code, wCreate.Body.String())
	}

	var keyResp struct {
		Success bool `json:"success"`
		Key     struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"key"`
	}
	if err := json.NewDecoder(wCreate.Body).Decode(&keyResp); err != nil {
		t.Fatalf("failed to decode key: %v", err)
	}
	rawKey := keyResp.Key.Key
	if rawKey == "" {
		t.Fatal("expected non-empty key")
	}

	// 3. Action endpoints like /send-message require auth
	unauthReq := httptest.NewRequest(http.MethodPost, "/api/v1/send-message", bytes.NewReader([]byte(`{"to":"+94771234567","text":"Hi"}`)))
	unauthReq.Header.Set("Content-Type", "application/json")
	wUnauth := httptest.NewRecorder()
	api.Routes().ServeHTTP(wUnauth, unauthReq)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized for unauthenticated /send-message, got %d", wUnauth.Code)
	}

	// 4. Request with X-Api-Key to /send-message should pass authentication
	authReq := httptest.NewRequest(http.MethodPost, "/api/v1/send-message", bytes.NewReader([]byte(`{"to":"+94771234567","text":"Hi"}`)))
	authReq.Header.Set("Content-Type", "application/json")
	authReq.Header.Set("X-Api-Key", rawKey)
	wAuth := httptest.NewRecorder()
	api.Routes().ServeHTTP(wAuth, authReq)
	if wAuth.Code == http.StatusUnauthorized {
		t.Fatalf("expected request to pass auth with valid key, got %d", wAuth.Code)
	}

	// Verify key in store
	keys, err := st.ListAPIKeys(context.Background())
	if err != nil || len(keys) != 1 {
		t.Fatalf("expected 1 key in store, got %d, err: %v", len(keys), err)
	}
}

func TestWasenderAPIStyleEndpoints(t *testing.T) {
	api, _, cleanup := setupTestAPI(t)
	defer cleanup()

	// 1. Create a session via POST /api/sessions
	createSessReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{"name":"Main Number"}`)))
	createSessReq.Header.Set("Content-Type", "application/json")
	wSess := httptest.NewRecorder()
	api.Routes().ServeHTTP(wSess, createSessReq)
	if wSess.Code != http.StatusCreated {
		t.Fatalf("expected 201 created, got %d: %s", wSess.Code, wSess.Body.String())
	}

	var sessResp struct {
		Success bool `json:"success"`
		Session struct {
			ID     string `json:"id"`
			APIKey string `json:"api_key"`
		} `json:"session"`
	}
	if err := json.NewDecoder(wSess.Body).Decode(&sessResp); err != nil {
		t.Fatalf("failed to decode session: %v", err)
	}

	sessionAPIKey := sessResp.Session.APIKey
	if sessionAPIKey == "" {
		t.Fatal("expected session to have a generated api_key (WasenderAPI style)")
	}

	// 2. Test WasenderAPI POST /api/send-message endpoint with Bearer <sessionAPIKey>
	msgPayload := `{"to":"+94771234567","text":"Hello WasenderAPI style!"}`
	msgReq := httptest.NewRequest(http.MethodPost, "/api/send-message", bytes.NewReader([]byte(msgPayload)))
	msgReq.Header.Set("Content-Type", "application/json")
	msgReq.Header.Set("Authorization", "Bearer "+sessionAPIKey)
	wMsg := httptest.NewRecorder()
	api.Routes().ServeHTTP(wMsg, msgReq)

	// Since WhatsApp is not physically connected to a phone in this mock unit test, whatsmeow returns that device JID is not yet paired
	// But it proves authentication succeeded with the session API key and route /api/send-message resolved the session!
	var msgResult map[string]any
	_ = json.NewDecoder(wMsg.Body).Decode(&msgResult)
	if wMsg.Code == http.StatusUnauthorized {
		t.Fatal("expected request to be authenticated with session API key")
	}
	if msgResult["error"] == nil {
		t.Fatal("expected an error response since device is not yet paired")
	}

	// 3. Test WasenderAPI POST /api/send-call endpoint with Bearer <sessionAPIKey>
	callCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	callPayload := `{"to":"+94771234567","audioUrl":"https://example.com/test.wav"}`
	callReq := httptest.NewRequest(http.MethodPost, "/api/send-call", bytes.NewReader([]byte(callPayload))).WithContext(callCtx)
	callReq.Header.Set("Content-Type", "application/json")
	callReq.Header.Set("Authorization", "Bearer "+sessionAPIKey)
	wCall := httptest.NewRecorder()
	api.Routes().ServeHTTP(wCall, callReq)

	var callResult map[string]any
	_ = json.NewDecoder(wCall.Body).Decode(&callResult)
	// Authenticated and routed successfully to calling engine
	if wCall.Code == http.StatusUnauthorized {
		t.Fatal("expected call request to be authenticated with session API key")
	}
}

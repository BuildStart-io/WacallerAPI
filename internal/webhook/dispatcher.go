package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"wacallerapi/internal/safenet"
	"wacallerapi/internal/store"
)

// EventType defines the type of webhook event.
type EventType string

const (
	EventSessionQR           EventType = "session.qr"
	EventSessionConnected    EventType = "session.connected"
	EventSessionDisconnected EventType = "session.disconnected"

	EventMessageReceived EventType = "message.received"
	EventMessageStatus   EventType = "message.status"

	EventCallIncoming  EventType = "call.incoming"
	EventCallConnected EventType = "call.connected"
	EventCallEnded     EventType = "call.ended"
)

type Payload struct {
	Event     EventType `json:"event"`
	SessionID string    `json:"session_id"`
	Timestamp int64     `json:"timestamp"`
	Data      any       `json:"data"`
}

type Dispatcher struct {
	client        *http.Client
	store         *store.Store
	defaultURL    string
	signingSecret string
	workCh        chan job
	log           *slog.Logger
}

type job struct {
	sessionID string
	targetURL string
	event     EventType
	payload   Payload
}

func NewDispatcher(st *store.Store, defaultURL, secret string, log *slog.Logger) *Dispatcher {
	d := &Dispatcher{
		client:        safenet.NewSafeClient(),
		store:         st,
		defaultURL:    defaultURL,
		signingSecret: secret,
		workCh:        make(chan job, 1000),
		log:           log,
	}

	for i := 0; i < 4; i++ {
		go d.worker()
	}

	return d
}

func (d *Dispatcher) Dispatch(sessionID, sessionWebhookURL string, event EventType, data any) {
	targetURL := sessionWebhookURL
	if targetURL == "" {
		targetURL = d.defaultURL
	}
	if targetURL == "" {
		return
	}

	p := Payload{
		Event:     event,
		SessionID: sessionID,
		Timestamp: time.Now().Unix(),
		Data:      data,
	}

	select {
	case d.workCh <- job{
		sessionID: sessionID,
		targetURL: targetURL,
		event:     event,
		payload:   p,
	}:
	default:
		d.log.Warn("webhook work queue full, dropping event", "event", event, "session_id", sessionID)
	}
}

func (d *Dispatcher) worker() {
	for j := range d.workCh {
		d.send(j)
	}
}

func (d *Dispatcher) send(j job) {
	body, err := json.Marshal(j.payload)
	if err != nil {
		d.log.Error("failed to marshal webhook payload", "err", err)
		return
	}

	deliveryID := newUUID()
	req, err := http.NewRequest(http.MethodPost, j.targetURL, bytes.NewReader(body))
	if err != nil {
		d.log.Error("invalid webhook url", "url", j.targetURL, "err", err)
		_ = d.store.LogWebhook(context.Background(), j.sessionID, string(j.event), j.targetURL, 0, string(body), err.Error())
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "WacallerAPI-Webhook/1.0")
	req.Header.Set("X-Wacaller-Event", string(j.event))
	req.Header.Set("X-Wacaller-Delivery", deliveryID)
	req.Header.Set("X-Wacaller-Timestamp", fmt.Sprintf("%d", j.payload.Timestamp))

	if d.signingSecret != "" {
		mac := hmac.New(sha256.New, []byte(d.signingSecret))
		mac.Write(body)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Wacaller-Signature", "sha256="+signature)
	}

	resp, err := d.client.Do(req)
	var statusCode int
	var errMsg string
	if err != nil {
		errMsg = err.Error()
		d.log.Warn("webhook delivery failed", "url", j.targetURL, "event", j.event, "err", err)
	} else {
		statusCode = resp.StatusCode
		_ = resp.Body.Close()
		d.log.Debug("webhook delivered", "url", j.targetURL, "event", j.event, "status", statusCode)
	}

	_ = d.store.LogWebhook(context.Background(), j.sessionID, string(j.event), j.targetURL, statusCode, string(body), errMsg)
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

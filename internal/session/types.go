package session

import (
	"time"
)

type SessionStatus string

const (
	StatusDisconnected SessionStatus = "DISCONNECTED"
	StatusScanQR       SessionStatus = "SCAN_QR"
	StatusConnecting   SessionStatus = "CONNECTING"
	StatusConnected    SessionStatus = "CONNECTED"
	StatusLoggedOut    SessionStatus = "LOGGED_OUT"
)

type CallStatus string

const (
	CallStatusInitiating CallStatus = "initiating"
	CallStatusRinging    CallStatus = "ringing"
	CallStatusActive     CallStatus = "active"
	CallStatusEnded      CallStatus = "ended"
	CallStatusFailed     CallStatus = "failed"
	CallStatusRejected   CallStatus = "rejected"
)

type SessionInfo struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	UserID     string        `json:"user_id,omitempty"`
	JID        string        `json:"jid,omitempty"`
	Phone      string        `json:"phone,omitempty"`
	Status     SessionStatus `json:"status"`
	QR         string        `json:"qr,omitempty"`
	WebhookURL string        `json:"webhook_url,omitempty"`
	APIKey     string        `json:"api_key,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

type CallInfo struct {
	CallID          string     `json:"call_id"`
	SessionID       string     `json:"session_id"`
	Direction       string     `json:"direction"`
	Peer            string     `json:"peer"`
	PeerNumber      string     `json:"peer_number"`
	Status          CallStatus `json:"status"`
	StartedAt       time.Time  `json:"started_at"`
	ConnectedAt     *time.Time `json:"connected_at,omitempty"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	DurationSeconds int        `json:"duration_seconds"`
	EndReason       string     `json:"end_reason,omitempty"`
}

type DialOptions struct {
	To              string `json:"to"`
	AudioURL        string `json:"audio_url,omitempty"`
	StreamURL       string `json:"stream_url,omitempty"`
	WebhookURL      string `json:"webhook_url,omitempty"`
	IsVideo         bool   `json:"is_video,omitempty"`
	MaxDurationSecs int    `json:"max_duration_seconds,omitempty"`
}

// UnifiedMessageRequest matches WasenderAPI's POST /api/send-message payload specification
type UnifiedMessageRequest struct {
	To          string  `json:"to"`
	Text        string  `json:"text,omitempty"`
	ImageURL    string  `json:"imageUrl,omitempty"`
	VideoURL    string  `json:"videoUrl,omitempty"`
	AudioURL    string  `json:"audioUrl,omitempty"`
	DocumentURL string  `json:"documentUrl,omitempty"`
	FileName    string  `json:"fileName,omitempty"`
	ViewOnce    bool    `json:"viewOnce,omitempty"`
	PTT         bool    `json:"ptt,omitempty"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
	Name        string  `json:"name,omitempty"`
	Address     string  `json:"address,omitempty"`
}

type TextMessageRequest struct {
	To   string `json:"to"`
	Text string `json:"text"`
}

type MediaMessageRequest struct {
	To       string `json:"to"`
	Type     string `json:"type"` // image, audio, video, document
	URL      string `json:"url"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
	PTT      bool   `json:"ptt,omitempty"` // for voice notes
}

type LocationMessageRequest struct {
	To        string  `json:"to"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name,omitempty"`
	Address   string  `json:"address,omitempty"`
}

package agent

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"wacallerapi/internal/audio"
)

type mockCallProvider struct {
	mu            sync.Mutex
	audioCh       chan []float32
	isPlaying     bool
	playedSamples []float32
	stoppedCount  int
}

func newMockCallProvider() *mockCallProvider {
	return &mockCallProvider{
		audioCh: make(chan []float32, 100),
	}
}

func (m *mockCallProvider) SubscribeAudio(subID string) <-chan []float32 {
	return m.audioCh
}

func (m *mockCallProvider) UnsubscribeAudio(subID string) {
}

func (m *mockCallProvider) PlayAudioSamples(ctx context.Context, samples []float32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isPlaying = true
	m.playedSamples = append(m.playedSamples, samples...)
	return nil
}

func (m *mockCallProvider) StopAudioPlayback() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isPlaying = false
	m.stoppedCount++
}

func (m *mockCallProvider) IsPlaying() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isPlaying
}

func TestVoiceAgentLifecycle(t *testing.T) {
	secret := "test_secret_123"

	// Create sample WAV response audio
	dummySamples := make([]float32, 1600) // 100ms at 16kHz
	for i := range dummySamples {
		dummySamples[i] = 0.5
	}
	wavBytes := audio.EncodePCM16kToWAV(dummySamples)
	b64Audio := base64.StdEncoding.EncodeToString(wavBytes)

	receivedActions := make([]AgentRequest, 0)
	var actionsMu sync.Mutex

	// Mock Agent Server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			return
		}

		// Verify Signature
		sigHeader := r.Header.Get("X-Wacaller-Signature")
		if sigHeader == "" {
			t.Errorf("missing X-Wacaller-Signature header")
		}

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if sigHeader != expectedSig {
			t.Errorf("signature mismatch: got %s, expected %s", sigHeader, expectedSig)
		}

		var req AgentRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("invalid request json: %v", err)
			return
		}

		actionsMu.Lock()
		receivedActions = append(receivedActions, req)
		actionsMu.Unlock()

		resp := AgentResponse{}
		if req.Action == "start" || req.Action == "turn" {
			resp.AudioBase64 = b64Audio
			resp.Format = "wav"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	os.Setenv("WACALLER_AGENT_URL", ts.URL)
	defer os.Unsetenv("WACALLER_AGENT_URL")

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	provider := newMockCallProvider()

	ag := NewAgentSession("sess_001", "call_001", secret, provider, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ag.Start(ctx)

	// Wait for "start" action to be processed
	time.Sleep(100 * time.Millisecond)

	actionsMu.Lock()
	if len(receivedActions) < 1 || receivedActions[0].Action != "start" {
		t.Fatalf("expected first action to be 'start', got: %v", receivedActions)
	}
	actionsMu.Unlock()

	// Simulate caller speech followed by 700ms silence
	speechFrame := make([]float32, 320) // 20ms at 16kHz
	for i := range speechFrame {
		speechFrame[i] = 0.1 // RMS > 0.01 threshold
	}

	silenceFrame := make([]float32, 320) // 20ms at 16kHz (all 0s)

	// Send speech (10 frames = 200ms of speech)
	for i := 0; i < 10; i++ {
		provider.audioCh <- speechFrame
	}

	// Send silence (36 frames = 720ms of silence)
	for i := 0; i < 36; i++ {
		provider.audioCh <- silenceFrame
	}

	// Wait for VAD and "turn" action to complete
	time.Sleep(200 * time.Millisecond)

	actionsMu.Lock()
	foundTurn := false
	for _, a := range receivedActions {
		if a.Action == "turn" {
			foundTurn = true
			if a.AudioBase64 == "" {
				t.Errorf("turn action missing audio_base64 payload")
			}
			if a.Format != "wav" {
				t.Errorf("expected format 'wav', got %s", a.Format)
			}
		}
	}
	actionsMu.Unlock()

	if !foundTurn {
		t.Errorf("expected 'turn' action to be sent after 700ms silence")
	}

	// Stop session (should send "end")
	ag.Stop()

	time.Sleep(100 * time.Millisecond)

	actionsMu.Lock()
	foundEnd := false
	for _, a := range receivedActions {
		if a.Action == "end" {
			foundEnd = true
		}
	}
	actionsMu.Unlock()

	if !foundEnd {
		t.Errorf("expected 'end' action to be sent when session stopped")
	}
}

package agent

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"sync"
	"time"

	"wacallerapi/internal/audio"
)

const DefaultAgentURL = "https://wacallerapi.com/api/public/engine/agent"

type CallProvider interface {
	SubscribeAudio(subID string) <-chan []float32
	UnsubscribeAudio(subID string)
	PlayAudioSamples(ctx context.Context, samples []float32) error
	StopAudioPlayback()
	IsPlaying() bool
}

type AgentRequest struct {
	Action      string `json:"action"`
	SessionID   string `json:"session_id"`
	CallID      string `json:"call_id"`
	AudioBase64 string `json:"audio_base64,omitempty"`
	Format      string `json:"format,omitempty"`
}

type AgentResponse struct {
	AudioBase64 string `json:"audio_base64"`
	Format      string `json:"format"`
}

type AgentSession struct {
	sessionID     string
	callID        string
	agentURL      string
	signingSecret string
	provider      CallProvider
	log           *slog.Logger
	httpClient    *http.Client

	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
}

func NewAgentSession(sessionID, callID, secret string, provider CallProvider, log *slog.Logger) *AgentSession {
	agentURL := os.Getenv("WACALLER_AGENT_URL")
	if agentURL == "" {
		agentURL = DefaultAgentURL
	}
	if secret == "" {
		secret = os.Getenv("WACALLER_WEBHOOK_SECRET")
	}

	return &AgentSession{
		sessionID:     sessionID,
		callID:        callID,
		agentURL:      agentURL,
		signingSecret: secret,
		provider:      provider,
		log:           log.With("session_id", sessionID, "call_id", callID),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (a *AgentSession) Start(parentCtx context.Context) {
	ctx, cancel := context.WithCancel(parentCtx)
	a.cancelFunc = cancel

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.run(ctx)
	}()
}

func (a *AgentSession) Stop() {
	if a.cancelFunc != nil {
		a.cancelFunc()
	}

	// Send "end" action asynchronously when call ends
	go a.sendEndAction()

	a.wg.Wait()
}

func (a *AgentSession) sendAction(req AgentRequest) (*AgentResponse, error) {
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal agent request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, a.agentURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create agent http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "WacallerAPI-VoiceAgent/1.0")

	if a.signingSecret != "" {
		mac := hmac.New(sha256.New, []byte(a.signingSecret))
		mac.Write(bodyBytes)
		sig := hex.EncodeToString(mac.Sum(nil))
		httpReq.Header.Set("X-Wacaller-Signature", "sha256="+sig)
	}

	a.log.Info("sending voice agent action", "action", req.Action, "url", a.agentURL)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		a.log.Error("voice agent request failed", "action", req.Action, "err", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		a.log.Warn("voice agent request non-200 status", "action", req.Action, "status", resp.StatusCode)
		return nil, fmt.Errorf("voice agent HTTP %d", resp.StatusCode)
	}

	var agentResp AgentResponse
	if err := json.NewDecoder(resp.Body).Decode(&agentResp); err != nil {
		a.log.Error("failed to decode voice agent response", "action", req.Action, "err", err)
		return nil, err
	}

	return &agentResp, nil
}

func (a *AgentSession) sendEndAction() {
	req := AgentRequest{
		Action:    "end",
		SessionID: a.sessionID,
		CallID:    a.callID,
	}
	_, _ = a.sendAction(req)
}

func (a *AgentSession) playResponseAudio(ctx context.Context, agentResp *AgentResponse) {
	if agentResp == nil || agentResp.AudioBase64 == "" {
		return
	}

	audioData, err := base64.StdEncoding.DecodeString(agentResp.AudioBase64)
	if err != nil {
		a.log.Error("failed to decode audio_base64 string", "err", err)
		return
	}

	samples, err := audio.DecodeWAVToPCM16k(bytes.NewReader(audioData))
	if err != nil {
		a.log.Error("failed to decode audio WAV data", "err", err)
		return
	}

	if len(samples) > 0 {
		a.log.Info("playing voice agent response audio", "samples_count", len(samples))
		_ = a.provider.PlayAudioSamples(ctx, samples)
	}
}

func (a *AgentSession) run(ctx context.Context) {
	// 1. Send "start" action immediately when call is answered / active
	startReq := AgentRequest{
		Action:    "start",
		SessionID: a.sessionID,
		CallID:    a.callID,
	}
	resp, err := a.sendAction(startReq)
	if err == nil && resp != nil {
		a.playResponseAudio(ctx, resp)
	}

	// 2. Subscribe to caller audio stream for VAD & utterance turn processing
	audioChan := a.provider.SubscribeAudio("agent_" + a.callID)
	defer a.provider.UnsubscribeAudio("agent_" + a.callID)

	var (
		utteranceBuffer []float32
		isSpeaking      bool
		silenceDuration time.Duration
		speechThreshold float32 = 0.01 // ~ -40dBFS RMS
		sampleRate              = 16000
	)

	silenceTarget := 700 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return
		case pcm, ok := <-audioChan:
			if !ok {
				return
			}
			if len(pcm) == 0 {
				continue
			}

			rms := calculateRMS(pcm)
			frameDuration := time.Duration(float64(len(pcm))/float64(sampleRate)*1000) * time.Millisecond

			if rms >= speechThreshold {
				// Caller is speaking! Stop agent playback if playing (barge-in support)
				if a.provider.IsPlaying() {
					a.log.Info("caller interrupted agent playback (barge-in)")
					a.provider.StopAudioPlayback()
				}

				isSpeaking = true
				silenceDuration = 0
				utteranceBuffer = append(utteranceBuffer, pcm...)
			} else {
				// Silence frame
				if isSpeaking {
					utteranceBuffer = append(utteranceBuffer, pcm...)
					silenceDuration += frameDuration

					if silenceDuration >= silenceTarget {
						// Utterance completed (700ms of silence detected after speech)!
						isSpeaking = false
						samplesToSend := make([]float32, len(utteranceBuffer))
						copy(samplesToSend, utteranceBuffer)

						utteranceBuffer = nil
						silenceDuration = 0

						if len(samplesToSend) >= 1600 { // at least 100ms of audio
							a.processTurn(ctx, samplesToSend)
						}
					}
				}
			}
		}
	}
}

func (a *AgentSession) processTurn(ctx context.Context, pcmSamples []float32) {
	wavBytes := audio.EncodePCM16kToWAV(pcmSamples)
	base64Wav := base64.StdEncoding.EncodeToString(wavBytes)

	turnReq := AgentRequest{
		Action:      "turn",
		SessionID:   a.sessionID,
		CallID:      a.callID,
		AudioBase64: base64Wav,
		Format:      "wav",
	}

	resp, err := a.sendAction(turnReq)
	if err == nil && resp != nil {
		a.playResponseAudio(ctx, resp)
	}
}

func calculateRMS(pcm []float32) float32 {
	if len(pcm) == 0 {
		return 0
	}
	var sum float64
	for _, s := range pcm {
		sum += float64(s * s)
	}
	return float32(math.Sqrt(sum / float64(len(pcm))))
}

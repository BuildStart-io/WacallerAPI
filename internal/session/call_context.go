package session

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"wacallerapi/internal/agent"
	"wacallerapi/internal/audio"
	"wacallerapi/internal/voip/call"

	"go.mau.fi/whatsmeow/types"
)

type CallContext struct {
	CallID          string
	SessionID       string
	Direction       string
	PeerJID         types.JID
	PeerNumber      string
	Status          CallStatus
	StartedAt       time.Time
	ConnectedAt     *time.Time
	EndedAt         *time.Time
	DurationSeconds int
	EndReason       string

	cm           *call.CallManager
	bridge       *Bridge
	agentSession *agent.AgentSession
	log          *slog.Logger

	mu         sync.RWMutex
	audioSubs  map[string]chan []float32
	playCancel context.CancelFunc
	playbackID uint64
}

func newCallContext(callID, sessionID, direction string, peer types.JID, peerNum string, cm *call.CallManager, log *slog.Logger) *CallContext {
	return &CallContext{
		CallID:     callID,
		SessionID:  sessionID,
		Direction:  direction,
		PeerJID:    peer,
		PeerNumber: peerNum,
		Status:     CallStatusInitiating,
		StartedAt:  time.Now().UTC(),
		cm:         cm,
		log:        log,
		audioSubs:  make(map[string]chan []float32),
	}
}

func (c *CallContext) Info() CallInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	dur := c.DurationSeconds
	if c.Status == CallStatusActive && c.ConnectedAt != nil {
		dur = int(time.Since(*c.ConnectedAt).Seconds())
	}

	return CallInfo{
		CallID:          c.CallID,
		SessionID:       c.SessionID,
		Direction:       c.Direction,
		Peer:            c.PeerJID.String(),
		PeerNumber:      c.PeerNumber,
		Status:          c.Status,
		StartedAt:       c.StartedAt,
		ConnectedAt:     c.ConnectedAt,
		EndedAt:         c.EndedAt,
		DurationSeconds: dur,
		EndReason:       c.EndReason,
	}
}

func (c *CallContext) SubscribeAudio(subID string) <-chan []float32 {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch := make(chan []float32, 100)
	c.audioSubs[subID] = ch
	return ch
}

func (c *CallContext) UnsubscribeAudio(subID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ch, ok := c.audioSubs[subID]; ok {
		delete(c.audioSubs, subID)
		close(ch)
	}
}

func (c *CallContext) BroadcastPeerAudio(pcm []float32) {
	if len(pcm) == 0 {
		return
	}

	// 1. Send to WebRTC bridge if active
	c.mu.RLock()
	br := c.bridge
	c.mu.RUnlock()
	if br != nil {
		_ = br.WritePCM(pcm)
	}

	// 2. Broadcast to WebSocket audio subscribers
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, ch := range c.audioSubs {
		select {
		case ch <- pcm:
		default:
			// avoid blocking if subscriber is slow
		}
	}
}

func (c *CallContext) InjectAudio(pcm []float32) {
	if c.cm != nil && len(pcm) > 0 {
		c.cm.FeedCapturedPCM(pcm)
	}
}

func (c *CallContext) PlayAudioURL(ctx context.Context, audioURL string) error {
	samples, err := audio.FetchAudioFromURL(audioURL)
	if err != nil {
		return err
	}
	return c.PlayAudioSamples(ctx, samples)
}

func (c *CallContext) PlayAudioSamples(parentCtx context.Context, samples []float32) error {
	c.StopAudioPlayback()

	ctx, cancel := context.WithCancel(parentCtx)
	c.mu.Lock()
	c.playbackID++
	currentID := c.playbackID
	c.playCancel = cancel
	c.mu.Unlock()

	go func() {
		defer func() {
			c.mu.Lock()
			if c.playbackID == currentID {
				c.playCancel = nil
			}
			c.mu.Unlock()
			cancel()
		}()
		frameSize := 960 // 60ms at 16 kHz
		ticker := time.NewTicker(60 * time.Millisecond)
		defer ticker.Stop()

		for offset := 0; offset < len(samples); offset += frameSize {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				end := offset + frameSize
				if end > len(samples) {
					end = len(samples)
				}
				frame := samples[offset:end]
				c.InjectAudio(frame)
			}
		}
	}()

	return nil
}

func (c *CallContext) StopAudioPlayback() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.playCancel != nil {
		c.playCancel()
		c.playCancel = nil
	}
}

func (c *CallContext) IsPlaying() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.playCancel != nil
}

func (c *CallContext) StartVoiceAgent(secret string) {
	c.mu.Lock()
	if c.agentSession != nil {
		c.mu.Unlock()
		return
	}
	ag := agent.NewAgentSession(c.SessionID, c.CallID, secret, c, c.log)
	c.agentSession = ag
	c.mu.Unlock()

	ag.Start(context.Background())
}

func (c *CallContext) StopVoiceAgent() {
	c.mu.Lock()
	ag := c.agentSession
	c.agentSession = nil
	c.mu.Unlock()

	if ag != nil {
		ag.Stop()
	}
}

func (c *CallContext) Close() {
	c.StopVoiceAgent()
	c.StopAudioPlayback()

	c.mu.Lock()
	defer c.mu.Unlock()

	for id, ch := range c.audioSubs {
		delete(c.audioSubs, id)
		close(ch)
	}

	if c.bridge != nil {
		c.bridge.Close()
		c.bridge = nil
	}
}


package session

import (
	"log/slog"
	"sync/atomic"

	"wacallerapi/internal/voip/media"

	"github.com/pion/webrtc/v4"
)

const pcmChannelLabel = "pcm"

// Bridge is the browser-leg WebRTC adapter: it carries raw PCM between a WebRTC peer
// and the CallManager over a WebRTC data channel.
type Bridge struct {
	pc  *webrtc.PeerConnection
	dc  atomic.Pointer[webrtc.DataChannel]
	log *slog.Logger

	OnBrowserPCM  func(pcm []float32)
	OnTerminalICE func()
}

func NewBridge(offerSDP string, log *slog.Logger) (*Bridge, string, error) {
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, "", err
	}
	br := &Bridge{pc: pc, log: log}

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != pcmChannelLabel {
			return
		}
		br.dc.Store(dc)
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			if cb := br.OnBrowserPCM; cb != nil && len(msg.Data) > 0 {
				cb(media.PCMInt16LEToFloat32(msg.Data))
			}
		})
	})

	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		log.Debug("browser ice state", "state", s.String())
		if s == webrtc.ICEConnectionStateFailed || s == webrtc.ICEConnectionStateClosed {
			if br.OnTerminalICE != nil {
				br.OnTerminalICE()
			}
		}
	})

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}); err != nil {
		_ = pc.Close()
		return nil, "", err
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return nil, "", err
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		return nil, "", err
	}
	<-gatherComplete

	return br, pc.LocalDescription().SDP, nil
}

func (b *Bridge) WritePCM(pcm []float32) error {
	dc := b.dc.Load()
	if dc == nil || len(pcm) == 0 {
		return nil
	}
	return dc.Send(media.PCMFloat32ToInt16LE(pcm))
}

func (b *Bridge) Close() {
	if b.pc != nil {
		_ = b.pc.Close()
	}
}

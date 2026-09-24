package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"wacallerapi/internal/safenet"
	"wacallerapi/internal/store"
	"wacallerapi/internal/voip/call"
	"wacallerapi/internal/voip/core"
	"wacallerapi/internal/voip/signaling"
	"wacallerapi/internal/wa"
	"wacallerapi/internal/webhook"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

type Session struct {
	id         string
	name       string
	userID     string
	apiKey     string
	client     *whatsmeow.Client
	store      *store.Store
	dispatcher *webhook.Dispatcher
	log        *slog.Logger
	maxCalls   int

	mu         sync.RWMutex
	status     SessionStatus
	connecting bool
	currentQR  string
	webhookURL string
	calls      map[string]*CallContext
}

func newSession(id, name, apiKey string, client *whatsmeow.Client, st *store.Store, disp *webhook.Dispatcher, log *slog.Logger, maxCalls int, webhookURL, userID string) *Session {
	s := &Session{
		id:         id,
		name:       name,
		userID:     userID,
		apiKey:     apiKey,
		client:     client,
		store:      st,
		dispatcher: disp,
		log:        log.With("session_id", id),
		maxCalls:   maxCalls,
		status:     StatusDisconnected,
		webhookURL: webhookURL,
		calls:      make(map[string]*CallContext),
	}

	client.AddEventHandler(s.handleEvent)
	return s
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) UserID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userID
}

func (s *Session) Name() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.name
}

func (s *Session) APIKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apiKey
}

func (s *Session) WebhookURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.webhookURL
}

func (s *Session) SetWebhookURL(ctx context.Context, u string) error {
	s.mu.Lock()
	s.webhookURL = u
	s.mu.Unlock()
	return s.store.UpsertSession(ctx, s.id, s.name, s.getJID(), string(s.status), u, s.apiKey, s.userID)
}

func (s *Session) getJID() string {
	if s.client != nil && s.client.Store != nil && s.client.Store.ID != nil {
		return s.client.Store.ID.String()
	}
	return ""
}

func (s *Session) Info() SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jid := s.getJID()
	var phone string
	if jid != "" {
		phone = s.store.CleanLID(jid)
	}

	return SessionInfo{
		ID:         s.id,
		Name:       s.name,
		UserID:     s.userID,
		JID:        jid,
		Phone:      phone,
		Status:     s.status,
		QR:         s.currentQR,
		WebhookURL: s.webhookURL,
		APIKey:     s.apiKey,
	}
}

// ---------------- Connection & Pairing ----------------

func (s *Session) Connect(ctx context.Context) error {
	s.mu.Lock()
	if s.client.IsConnected() || s.connecting {
		s.mu.Unlock()
		return nil
	}
	s.connecting = true
	s.status = StatusConnecting
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.connecting = false
		s.mu.Unlock()
	}()

	if s.client.Store.ID == nil {
		// Needs pairing via QR - use background context so it survives HTTP request lifetime!
		qrChan, err := s.client.GetQRChannel(context.Background())
		if err != nil {
			if !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
				s.log.Warn("failed to get QR channel", "err", err)
			}
		} else {
			go s.listenQR(qrChan)
		}
	}

	err := s.client.Connect()
	if err != nil {
		s.mu.Lock()
		s.status = StatusDisconnected
		s.currentQR = ""
		s.mu.Unlock()
		return err
	}
	return nil
}

func (s *Session) GetQR(ctx context.Context) (string, SessionStatus) {
	s.mu.RLock()
	qr := s.currentQR
	status := s.status
	isConn := s.client.IsConnected()
	isConnecting := s.connecting
	s.mu.RUnlock()

	// Only trigger connect if completely disconnected and not currently attempting
	if s.client.Store.ID == nil && qr == "" && !isConn && !isConnecting {
		go func() {
			_ = s.Connect(context.Background())
		}()
	}

	return qr, status
}

func (s *Session) listenQR(qrChan <-chan whatsmeow.QRChannelItem) {
	for item := range qrChan {
		switch item.Event {
		case "code":
			s.mu.Lock()
			s.status = StatusScanQR
			s.currentQR = item.Code
			s.mu.Unlock()

			s.log.Info("fresh WhatsApp QR code generated", "qr", item.Code)
			s.dispatcher.Dispatch(s.id, s.webhookURL, webhook.EventSessionQR, map[string]string{
				"qr": item.Code,
			})
		case "timeout":
			s.mu.Lock()
			s.currentQR = ""
			s.status = StatusDisconnected
			s.mu.Unlock()
		case "success":
			s.mu.Lock()
			s.status = StatusConnected
			s.currentQR = ""
			s.mu.Unlock()
			s.log.Info("WhatsApp session paired and connected successfully!", "session_id", s.id)
		}
	}
}

func (s *Session) PairPhone(ctx context.Context, phone string) (string, error) {
	if s.client.Store.ID != nil {
		return "", errors.New("session already paired to an account")
	}
	if !s.client.IsConnected() {
		if err := s.client.Connect(); err != nil {
			return "", fmt.Errorf("connect failed: %w", err)
		}
	}
	code, err := s.client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Mac OS)")
	if err != nil {
		return "", err
	}
	return code, nil
}

func (s *Session) Logout(ctx context.Context) error {
	if s.client.IsConnected() {
		_ = s.client.Logout(ctx)
	}
	s.mu.Lock()
	s.status = StatusLoggedOut
	s.currentQR = ""
	s.mu.Unlock()

	_ = s.store.UpsertSession(ctx, s.id, s.name, "", string(StatusLoggedOut), s.webhookURL, s.apiKey, s.userID)
	s.dispatcher.Dispatch(s.id, s.webhookURL, webhook.EventSessionDisconnected, map[string]string{
		"reason": "user_logout",
	})
	return nil
}

func (s *Session) Disconnect() {
	if s.client.IsConnected() {
		s.client.Disconnect()
	}
	s.mu.Lock()
	s.status = StatusDisconnected
	s.mu.Unlock()
}

// ---------------- Event Handling ----------------

func (s *Session) handleEvent(rawEvt any) {
	ctx := context.Background()

	switch evt := rawEvt.(type) {
	case *events.Connected:
		s.mu.Lock()
		s.status = StatusConnected
		s.currentQR = ""
		jid := s.getJID()
		s.mu.Unlock()

		_ = s.store.UpsertSession(ctx, s.id, s.name, jid, string(StatusConnected), s.webhookURL, s.apiKey, s.userID)
		s.log.Info("WhatsApp connected", "jid", jid)
		s.dispatcher.Dispatch(s.id, s.webhookURL, webhook.EventSessionConnected, map[string]string{
			"jid":   jid,
			"phone": s.store.CleanLID(jid),
		})

	case *events.LoggedOut:
		s.mu.Lock()
		s.status = StatusLoggedOut
		s.currentQR = ""
		s.mu.Unlock()

		_ = s.store.UpsertSession(ctx, s.id, s.name, "", string(StatusLoggedOut), s.webhookURL, s.apiKey, s.userID)
		s.log.Warn("WhatsApp logged out by phone", "reason", evt.Reason.String())
		s.dispatcher.Dispatch(s.id, s.webhookURL, webhook.EventSessionDisconnected, map[string]string{
			"reason": evt.Reason.String(),
		})

	case *events.Disconnected:
		s.mu.Lock()
		if s.status != StatusLoggedOut {
			s.status = StatusDisconnected
		}
		s.mu.Unlock()
		s.dispatcher.Dispatch(s.id, s.webhookURL, webhook.EventSessionDisconnected, map[string]string{
			"reason": "network_disconnected",
		})

	case *events.Message:
		s.handleIncomingMessage(evt)

	case *events.CallOffer:
		s.handleIncomingCallOffer(ctx, evt)

	case *events.CallAccept:
		if c := s.callForNode(evt.From, evt.Data); c != nil && c.cm != nil {
			c.cm.HandleCallAccept(ctx, wrapCall(evt.From, evt.Data), evt.From)
		}

	case *events.CallTransport:
		if c := s.callForNode(evt.From, evt.Data); c != nil && c.cm != nil {
			c.cm.HandleCallTransport(ctx, wrapCall(evt.From, evt.Data), evt.From)
		}

	case *events.CallTerminate:
		if c := s.callForNode(evt.From, evt.Data); c != nil && c.cm != nil {
			c.cm.HandleCallTerminate(wrapCall(evt.From, evt.Data), evt.From)
		}

	case *events.CallReject:
		if c := s.callForNode(evt.From, evt.Data); c != nil && c.cm != nil {
			c.cm.HandleCallTerminate(wrapCall(evt.From, evt.Data), evt.From)
		}
	}
}

// ---------------- Messaging Subsystem ----------------

func (s *Session) handleIncomingMessage(evt *events.Message) {
	if evt.Message == nil || evt.Info.IsFromMe {
		return
	}

	sender := evt.Info.Sender.ToNonAD().String()
	senderNum := s.store.CleanLID(sender)
	msgID := evt.Info.ID
	msgType := "text"
	content := ""
	mediaURL := ""

	if text := evt.Message.GetConversation(); text != "" {
		content = text
	} else if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
		content = ext.GetText()
	} else if img := evt.Message.GetImageMessage(); img != nil {
		msgType = "image"
		content = img.GetCaption()
	} else if audio := evt.Message.GetAudioMessage(); audio != nil {
		msgType = "audio"
	} else if doc := evt.Message.GetDocumentMessage(); doc != nil {
		msgType = "document"
		content = doc.GetTitle()
	} else if loc := evt.Message.GetLocationMessage(); loc != nil {
		msgType = "location"
		content = fmt.Sprintf("%f,%f", loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
	}

	rec := store.MessageRecord{
		SessionID:  s.id,
		MessageID:  msgID,
		Direction:  "inbound",
		PeerNumber: senderNum,
		MsgType:    msgType,
		Content:    content,
		MediaURL:   mediaURL,
		Status:     "received",
		Timestamp:  evt.Info.Timestamp,
	}
	_ = s.store.InsertMessage(context.Background(), rec)

	s.dispatcher.Dispatch(s.id, s.webhookURL, webhook.EventMessageReceived, map[string]any{
		"message_id": msgID,
		"from":       senderNum,
		"from_jid":   sender,
		"type":       msgType,
		"content":    content,
		"timestamp":  evt.Info.Timestamp.Unix(),
		"push_name":  evt.Info.PushName,
		"is_group":   evt.Info.IsGroup,
	})
}

func (s *Session) parseRecipient(to string) (types.JID, error) {
	clean := strings.TrimPrefix(to, "+")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return types.EmptyJID, errors.New("recipient phone number is empty")
	}
	if !strings.Contains(clean, "@") {
		clean = clean + "@" + types.DefaultUserServer
	}
	jid, err := types.ParseJID(clean)
	if err != nil {
		return types.EmptyJID, err
	}
	return jid.ToNonAD(), nil
}

func (s *Session) SendTextMessage(ctx context.Context, req TextMessageRequest) (string, error) {
	target, err := s.parseRecipient(req.To)
	if err != nil {
		return "", err
	}
	if !s.client.IsConnected() {
		return "", errors.New("session is not connected to WhatsApp")
	}

	msg := &waE2E.Message{
		Conversation: proto.String(req.Text),
	}
	resp, err := s.client.SendMessage(ctx, target, msg)
	if err != nil {
		return "", err
	}

	_ = s.store.InsertMessage(ctx, store.MessageRecord{
		SessionID:  s.id,
		MessageID:  resp.ID,
		Direction:  "outbound",
		PeerNumber: s.store.CleanLID(target.String()),
		MsgType:    "text",
		Content:    req.Text,
		Status:     "sent",
		Timestamp:  resp.Timestamp,
	})

	return resp.ID, nil
}

func (s *Session) SendMediaMessage(ctx context.Context, req MediaMessageRequest) (string, error) {
	target, err := s.parseRecipient(req.To)
	if err != nil {
		return "", err
	}
	if !s.client.IsConnected() {
		return "", errors.New("session is not connected to WhatsApp")
	}

	// 1. Fetch media content from URL (SSRF-safe)
	data, err := safenet.SafeGet(ctx, req.URL)
	if err != nil {
		return "", fmt.Errorf("failed to download media (SSRF check failed or network error): %w", err)
	}

	mimeType := http.DetectContentType(data)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// 2. Upload to WhatsApp
	var appInfo whatsmeow.MediaType
	switch strings.ToLower(req.Type) {
	case "image":
		appInfo = whatsmeow.MediaImage
	case "audio":
		appInfo = whatsmeow.MediaAudio
	case "video":
		appInfo = whatsmeow.MediaVideo
	default:
		appInfo = whatsmeow.MediaDocument
	}

	upResp, err := s.client.Upload(ctx, data, appInfo)
	if err != nil {
		return "", fmt.Errorf("failed to upload media to WhatsApp: %w", err)
	}

	// 3. Construct message
	var msg *waE2E.Message
	switch appInfo {
	case whatsmeow.MediaImage:
		msg = &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				URL:           &upResp.URL,
				DirectPath:    &upResp.DirectPath,
				MediaKey:      upResp.MediaKey,
				FileEncSHA256: upResp.FileEncSHA256,
				FileSHA256:    upResp.FileSHA256,
				FileLength:    &upResp.FileLength,
				Mimetype:      proto.String(mimeType),
				Caption:       proto.String(req.Caption),
			},
		}
	case whatsmeow.MediaAudio:
		msg = &waE2E.Message{
			AudioMessage: &waE2E.AudioMessage{
				URL:           &upResp.URL,
				DirectPath:    &upResp.DirectPath,
				MediaKey:      upResp.MediaKey,
				FileEncSHA256: upResp.FileEncSHA256,
				FileSHA256:    upResp.FileSHA256,
				FileLength:    &upResp.FileLength,
				Mimetype:      proto.String(mimeType),
				PTT:           proto.Bool(req.PTT),
			},
		}
	case whatsmeow.MediaVideo:
		msg = &waE2E.Message{
			VideoMessage: &waE2E.VideoMessage{
				URL:           &upResp.URL,
				DirectPath:    &upResp.DirectPath,
				MediaKey:      upResp.MediaKey,
				FileEncSHA256: upResp.FileEncSHA256,
				FileSHA256:    upResp.FileSHA256,
				FileLength:    &upResp.FileLength,
				Mimetype:      proto.String(mimeType),
				Caption:       proto.String(req.Caption),
			},
		}
	default:
		title := req.Filename
		if title == "" {
			title = "document"
		}
		msg = &waE2E.Message{
			DocumentMessage: &waE2E.DocumentMessage{
				URL:           &upResp.URL,
				DirectPath:    &upResp.DirectPath,
				MediaKey:      upResp.MediaKey,
				FileEncSHA256: upResp.FileEncSHA256,
				FileSHA256:    upResp.FileSHA256,
				FileLength:    &upResp.FileLength,
				Mimetype:      proto.String(mimeType),
				Title:         proto.String(title),
				FileName:      proto.String(title),
				Caption:       proto.String(req.Caption),
			},
		}
	}

	sendResp, err := s.client.SendMessage(ctx, target, msg)
	if err != nil {
		return "", err
	}

	_ = s.store.InsertMessage(ctx, store.MessageRecord{
		SessionID:  s.id,
		MessageID:  sendResp.ID,
		Direction:  "outbound",
		PeerNumber: s.store.CleanLID(target.String()),
		MsgType:    req.Type,
		Content:    req.Caption,
		MediaURL:   req.URL,
		Status:     "sent",
		Timestamp:  sendResp.Timestamp,
	})

	return sendResp.ID, nil
}

func (s *Session) SendLocationMessage(ctx context.Context, req LocationMessageRequest) (string, error) {
	target, err := s.parseRecipient(req.To)
	if err != nil {
		return "", err
	}
	if !s.client.IsConnected() {
		return "", errors.New("session is not connected to WhatsApp")
	}

	msg := &waE2E.Message{
		LocationMessage: &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(req.Latitude),
			DegreesLongitude: proto.Float64(req.Longitude),
			Name:             proto.String(req.Name),
			Address:          proto.String(req.Address),
		},
	}
	resp, err := s.client.SendMessage(ctx, target, msg)
	if err != nil {
		return "", err
	}

	_ = s.store.InsertMessage(ctx, store.MessageRecord{
		SessionID:  s.id,
		MessageID:  resp.ID,
		Direction:  "outbound",
		PeerNumber: s.store.CleanLID(target.String()),
		MsgType:    "location",
		Content:    fmt.Sprintf("%f,%f (%s)", req.Latitude, req.Longitude, req.Name),
		Status:     "sent",
		Timestamp:  resp.Timestamp,
	})

	return resp.ID, nil
}

// SendUnifiedMessage handles WasenderAPI unified payload (text, imageUrl, videoUrl, audioUrl, documentUrl, location)
func (s *Session) SendUnifiedMessage(ctx context.Context, req UnifiedMessageRequest) (string, error) {
	if req.ImageURL != "" {
		return s.SendMediaMessage(ctx, MediaMessageRequest{
			To:      req.To,
			Type:    "image",
			URL:     req.ImageURL,
			Caption: req.Text,
		})
	}
	if req.VideoURL != "" {
		return s.SendMediaMessage(ctx, MediaMessageRequest{
			To:      req.To,
			Type:    "video",
			URL:     req.VideoURL,
			Caption: req.Text,
		})
	}
	if req.AudioURL != "" {
		return s.SendMediaMessage(ctx, MediaMessageRequest{
			To:   req.To,
			Type: "audio",
			URL:  req.AudioURL,
			PTT:  req.PTT,
		})
	}
	if req.DocumentURL != "" {
		return s.SendMediaMessage(ctx, MediaMessageRequest{
			To:       req.To,
			Type:     "document",
			URL:      req.DocumentURL,
			Filename: req.FileName,
			Caption:  req.Text,
		})
	}
	if req.Latitude != 0 || req.Longitude != 0 {
		return s.SendLocationMessage(ctx, LocationMessageRequest{
			To:        req.To,
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
			Name:      req.Name,
			Address:   req.Address,
		})
	}
	return s.SendTextMessage(ctx, TextMessageRequest{
		To:   req.To,
		Text: req.Text,
	})
}

// ---------------- VoIP Calling Subsystem ----------------

func (s *Session) createCallContext(callID, dir string, peer types.JID) *CallContext {
	socket := wa.NewSocket(s.client)
	cm := call.NewCallManager(socket, s.log)
	callCtx := newCallContext(callID, s.id, dir, peer, s.store.CleanLID(peer.String()), cm, s.log)

	s.mu.Lock()
	s.calls[callID] = callCtx
	s.mu.Unlock()

	cm.OnPeerAudio = func(pcm16 []float32) {
		callCtx.BroadcastPeerAudio(pcm16)
	}

	cm.OnStateChange = func(c *call.CallInfo) {
		s.handleCallStateChange(callCtx, c)
	}

	return callCtx
}

func (s *Session) handleCallStateChange(callCtx *CallContext, c *call.CallInfo) {
	callCtx.mu.Lock()
	oldStatus := callCtx.Status
	switch c.StateData.State {
	case core.CallStateActive:
		callCtx.Status = CallStatusActive
		if callCtx.ConnectedAt == nil {
			now := time.Now().UTC()
			callCtx.ConnectedAt = &now
		}
	case core.CallStateEnded:
		callCtx.Status = CallStatusEnded
		if callCtx.EndedAt == nil {
			now := time.Now().UTC()
			callCtx.EndedAt = &now
		}
		if callCtx.ConnectedAt != nil {
			callCtx.DurationSeconds = int(time.Since(*callCtx.ConnectedAt).Seconds())
		}
		callCtx.EndReason = string(c.StateData.EndReason)
	case core.CallStateInitiating:
		callCtx.Status = CallStatusInitiating
	default:
		callCtx.Status = CallStatusRinging
	}
	newStatus := callCtx.Status
	dur := callCtx.DurationSeconds
	reason := callCtx.EndReason
	callCtx.mu.Unlock()

	if oldStatus != newStatus {
		_ = s.store.UpdateCallStatus(context.Background(), callCtx.CallID, string(newStatus), dur, reason)

		if newStatus == CallStatusActive {
			s.dispatcher.Dispatch(s.id, s.webhookURL, webhook.EventCallConnected, map[string]any{
				"call_id":     callCtx.CallID,
				"peer_number": callCtx.PeerNumber,
				"direction":   callCtx.Direction,
			})
		} else if newStatus == CallStatusEnded {
			s.dispatcher.Dispatch(s.id, s.webhookURL, webhook.EventCallEnded, map[string]any{
				"call_id":          callCtx.CallID,
				"peer_number":      callCtx.PeerNumber,
				"direction":        callCtx.Direction,
				"duration_seconds": dur,
				"reason":           reason,
			})
			go func() {
				time.Sleep(5 * time.Second)
				s.removeCall(callCtx.CallID)
			}()
		}
	}
}

func (s *Session) removeCall(callID string) {
	s.mu.Lock()
	c, ok := s.calls[callID]
	if ok {
		delete(s.calls, callID)
	}
	s.mu.Unlock()

	if ok && c != nil {
		c.Close()
	}
}

func (s *Session) StartCall(ctx context.Context, opts DialOptions) (*CallContext, error) {
	if s.maxCalls > 0 {
		s.mu.RLock()
		activeCount := len(s.calls)
		s.mu.RUnlock()
		if activeCount >= s.maxCalls {
			return nil, fmt.Errorf("session reached maximum concurrent calls (%d)", s.maxCalls)
		}
	}

	peer, err := s.parseRecipient(opts.To)
	if err != nil {
		return nil, err
	}

	callID := signaling.GenerateCallID()
	callCtx := s.createCallContext(callID, "outbound", peer)

	_ = s.store.InsertCall(ctx, store.CallRecord{
		CallID:          callID,
		SessionID:       s.id,
		Direction:       "outbound",
		PeerNumber:      callCtx.PeerNumber,
		Status:          string(CallStatusInitiating),
		DurationSeconds: 0,
		StartedAt:       callCtx.StartedAt,
	})

	if err := callCtx.cm.StartCall(ctx, callID, peer, opts.IsVideo); err != nil {
		s.removeCall(callID)
		_ = s.store.UpdateCallStatus(ctx, callID, string(CallStatusFailed), 0, err.Error())
		return nil, err
	}

	// If an initial audio URL was specified, play it once call becomes active
	if opts.AudioURL != "" {
		go func() {
			for i := 0; i < 60; i++ { // wait up to 30s
				time.Sleep(500 * time.Millisecond)
				callCtx.mu.RLock()
				status := callCtx.Status
				callCtx.mu.RUnlock()
				if status == CallStatusActive {
					_ = callCtx.PlayAudioURL(context.Background(), opts.AudioURL)
					break
				}
				if status == CallStatusEnded || status == CallStatusFailed {
					break
				}
			}
		}()
	}

	return callCtx, nil
}

func (s *Session) handleIncomingCallOffer(ctx context.Context, evt *events.CallOffer) {
	node := wrapCall(evt.From, evt.Data)
	callID := callIDFromNode(node)
	if callID == "" {
		return
	}

	peer := evt.From.ToNonAD()
	peerNum := s.store.CleanLID(peer.String())

	s.mu.RLock()
	activeCount := len(s.calls)
	s.mu.RUnlock()

	if s.maxCalls > 0 && activeCount >= s.maxCalls {
		s.rejectOffer(ctx, node, evt.From)
		return
	}

	callCtx := s.createCallContext(callID, "inbound", peer)
	callCtx.Status = CallStatusRinging

	_ = s.store.InsertCall(ctx, store.CallRecord{
		CallID:          callID,
		SessionID:       s.id,
		Direction:       "inbound",
		PeerNumber:      peerNum,
		Status:          string(CallStatusRinging),
		DurationSeconds: 0,
		StartedAt:       callCtx.StartedAt,
	})

	s.dispatcher.Dispatch(s.id, s.webhookURL, webhook.EventCallIncoming, map[string]any{
		"call_id":   callID,
		"from":      peerNum,
		"from_jid":  peer.String(),
		"timestamp": time.Now().Unix(),
	})
}

func (s *Session) AcceptCall(ctx context.Context, callID string) error {
	s.mu.RLock()
	c, ok := s.calls[callID]
	s.mu.RUnlock()
	if !ok || c.cm == nil {
		return errors.New("call not found")
	}
	return c.cm.AcceptCall(ctx, callID)
}

func (s *Session) RejectCall(ctx context.Context, callID string) error {
	s.mu.RLock()
	c, ok := s.calls[callID]
	s.mu.RUnlock()
	if !ok || c.cm == nil {
		return errors.New("call not found")
	}
	err := c.cm.RejectCall(ctx, callID, core.EndCallReasonDeclined)
	s.removeCall(callID)
	return err
}

func (s *Session) EndCall(callID string) error {
	s.mu.RLock()
	c, ok := s.calls[callID]
	s.mu.RUnlock()
	if !ok || c.cm == nil {
		return errors.New("call not found")
	}
	_ = c.cm.EndCall(context.Background(), core.EndCallReasonUserEnded)
	s.removeCall(callID)
	return nil
}

func (s *Session) GetCall(callID string) (*CallContext, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.calls[callID]
	return c, ok
}

func (s *Session) ListCalls() []CallInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]CallInfo, 0, len(s.calls))
	for _, c := range s.calls {
		out = append(out, c.Info())
	}
	return out
}

func (s *Session) AttachWebRTC(callID, offerSDP string) (string, error) {
	s.mu.RLock()
	c, ok := s.calls[callID]
	s.mu.RUnlock()
	if !ok {
		return "", errors.New("call not found")
	}

	br, answerSDP, err := NewBridge(offerSDP, s.log)
	if err != nil {
		return "", err
	}

	br.OnBrowserPCM = func(pcm []float32) {
		c.InjectAudio(pcm)
	}

	c.mu.Lock()
	if c.bridge != nil {
		c.bridge.Close()
	}
	c.bridge = br
	c.mu.Unlock()

	return answerSDP, nil
}

func (s *Session) rejectOffer(ctx context.Context, offer *waBinary.Node, peer types.JID) {
	callID := callIDFromNode(offer)
	stanza := signaling.BuildRejectStanza(peer, callID, peer)
	socket := wa.NewSocket(s.client)
	_ = socket.SendNode(ctx, stanza)
}

func (s *Session) callForNode(from types.JID, data *waBinary.Node) *CallContext {
	node := wrapCall(from, data)
	callID := callIDFromNode(node)
	if callID == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.calls[callID]
}

func wrapCall(from types.JID, inner *waBinary.Node) *waBinary.Node {
	content := []waBinary.Node{}
	if inner != nil {
		content = append(content, *inner)
	}
	return &waBinary.Node{
		Tag:     "call",
		Attrs:   waBinary.Attrs{"from": from},
		Content: content,
	}
}

func callIDFromNode(node *waBinary.Node) string {
	info := signaling.ExtractNodeInfo(node)
	if info == nil {
		return ""
	}
	return info.CallID
}

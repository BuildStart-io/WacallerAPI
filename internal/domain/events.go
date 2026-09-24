package domain

// Domain event type constants for outbox and webhook dispatch.
const (
	EventOrgCreated         = "org.created"
	EventOrgUpdated         = "org.updated"
	EventUserRegistered     = "user.registered"
	EventMemberAdded        = "member.added"
	EventMemberRemoved      = "member.removed"
	EventSessionConnected   = "whatsapp.session.connected"
	EventSessionDisconnected = "whatsapp.session.disconnected"
	EventSessionQR          = "whatsapp.session.qr"
	EventCallCreated        = "call.created"
	EventCallEnded          = "call.ended"
	EventMessageSent        = "message.sent"
	EventMessageReceived    = "message.received"
	EventSubscriptionUpdated = "subscription.updated"
)

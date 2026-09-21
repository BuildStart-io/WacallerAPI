# 📞 WacallerAPI

**The Developer WhatsApp API Platform for Voice Calls and Messaging.**

Think **WasenderAPI** or **WAHA**, but with native **WhatsApp Voice Calling** support:
Place outbound calls, receive inbound calls, stream live 16 kHz bi-directional audio over WebSockets, play pre-recorded WAV audio files, and send rich text/media messages through a unified, high-performance REST API.

Built in **pure Go** with zero external C/C++ dependencies (`CGO_ENABLED=0`), featuring a vendored pure-Go **MLow** audio codec and `whatsmeow`.

---

## 🌟 Why WacallerAPI?

| Feature | WasenderAPI / WAHA / Evolution | **WacallerAPI** |
| :--- | :--- | :--- |
| **Send WhatsApp Text & Media** | ✅ Yes | ✅ **Yes** |
| **Multi-Session / Multi-Number** | ✅ Yes | ✅ **Yes** |
| **Event Webhooks** | ✅ Yes | ✅ **Yes** |
| **Place WhatsApp Voice Calls** | ❌ No | ✅ **Yes (`POST /api/v1/sessions/{id}/calls`)** |
| **Accept / Reject Inbound Calls** | ❌ No (auto-reject) | ✅ **Yes (Programmatic control)** |
| **Live 16 kHz Audio Streaming** | ❌ No | ✅ **Yes (WebSocket bi-directional PCM)** |
| **Audio File Injection into Call** | ❌ No | ✅ **Yes (WAV playback from URL / bytes)** |
| **Embedded Developer Dashboard** | ⚠️ Basic or none | ✅ **Yes (Built-in Web Portal & Swagger)** |

---

## 🏗️ Architecture

```
                  ┌──────────────────────────────────────────────┐
                  │          Developer Applications              │
                  │  (Node.js, Python, Laravel, n8n, AI Bots)    │
                  └──────────────────────┬───────────────────────┘
                                         │
            ┌────────────────────────────┴────────────────────────────┐
            │ REST API (Bearer / X-Api-Key)   WebSocket Audio Stream  │
            │ POST /api/v1/sessions/.../calls ws://.../calls/.../stream│
            │ POST /api/v1/messages/text      Webhook Events (HTTP)   │
            └────────────────────────────┬────────────────────────────┘
                                         ▼
┌──────────────────────────────────────────────────────────────────────────────────┐
│                           WacallerAPI High-Performance Go Engine                 │
│                                                                                  │
│   ┌─────────────────────┐  ┌─────────────────────┐  ┌────────────────────────┐   │
│   │ API Key Auth Store  │  │  Webhook Dispatcher │  │ OpenAPI 3.0 / Swagger  │   │
│   └─────────────────────┘  └─────────────────────┘  └────────────────────────┘   │
│                                                                                  │
│   ┌──────────────────────────────────────────────────────────────────────────┐   │
│   │                           Session Manager                                │   │
│   │                                                                          │   │
│   │   ┌──────────────────────────────┐   ┌───────────────────────────────┐   │   │
│   │   │     Messaging Subsystem      │   │         VoIP Engine           │   │   │
│   │   │ - Text, Images, Voice notes  │   │ - Outbound / Inbound calls    │   │   │
│   │   │ - Document & Location pins   │   │ - 16 kHz MLow Audio Codec     │   │   │
│   │   │ - Read receipts & ACKs       │   │ - SRTP / STUN / Relay Mesh    │   │   │
│   │   └──────────────────────────────┘   │ - WebSocket Stream Hub        │   │   │
│   │                                      └───────────────────────────────┘   │   │
│   └──────────────────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────┬─────────────────────────────────────────┘
                                         │
                                         ▼
                             ┌───────────────────────┐
                             │    WhatsApp Network   │
                             └───────────────────────┘
```

---

## 🚀 Quick Start

### 1. Run Server
```bash
# Build standalone binary
go build -o wacaller ./cmd/wacaller

# Start server on port 8080
./wacaller -addr :8080
```

Open `http://localhost:8080` in your browser to access the **WacallerAPI Developer Portal**.

### 2. Connect an Account
1. In the dashboard, click **+ New Session** (or call `POST /api/v1/sessions`).
2. Scan the QR code with WhatsApp on your phone (**Linked devices**).
3. Your session is now online and ready to call and message!

---

## 📡 REST API Reference

All routes are prefixed with `/api/v1`.

### 1. Sessions Management

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/sessions` | List all WhatsApp accounts |
| `POST` | `/sessions` | Create account container (`{"name":"Line 1","webhook_url":"..."}`) |
| `GET` | `/sessions/{id}` | Get session status & JID |
| `GET` | `/sessions/{id}/qr` | Fetch current QR code string |
| `POST` | `/sessions/{id}/pair` | Request 8-digit pairing code (`{"phone":"+94..."}`) |
| `POST` | `/sessions/{id}/logout` | Log out from WhatsApp |
| `DELETE` | `/sessions/{id}` | Delete account & credentials |

### 2. VoIP Calling (The Core Feature)

#### Start Outbound Call
```http
POST /api/v1/sessions/{id}/calls
Content-Type: application/json

{
  "to": "+94771234567",
  "audio_url": "https://example.com/welcome.wav"
}
```
**Response:**
```json
{
  "success": true,
  "call": {
    "call_id": "c_9f81a2",
    "status": "initiating",
    "direction": "outbound",
    "peer_number": "94771234567"
  },
  "stream_url": "ws://localhost:8080/api/v1/sessions/{id}/calls/c_9f81a2/stream"
}
```

#### Real-Time Audio Streaming Gateway (WebSocket)
Connect to:
`ws://localhost:8080/api/v1/sessions/{id}/calls/{callId}/stream`
- **Downlink (WhatsApp → You)**: The server sends binary frames of raw **16 kHz 16-bit mono linear PCM** containing the caller's voice.
- **Uplink (You → WhatsApp)**: Send binary frames of raw **16 kHz 16-bit mono linear PCM** to speak to the caller.
- Also supports Twilio-style JSON:
  ```json
  { "event": "media", "media": { "payload": "<base64 PCM>" } }
  ```

#### Call Controls
- **Accept Inbound Call**: `POST /api/v1/sessions/{id}/calls/{callId}/accept`
- **Reject Inbound Call**: `POST /api/v1/sessions/{id}/calls/{callId}/reject`
- **End Call / Hangup**: `DELETE /api/v1/sessions/{id}/calls/{callId}`
- **Play Audio into Call**: `POST /api/v1/sessions/{id}/calls/{callId}/play`
  ```json
  { "audio_url": "https://example.com/announcement.wav" }
  ```

---

### 3. Messaging

#### Send Text Message
```http
POST /api/v1/sessions/{id}/messages/text
Content-Type: application/json

{
  "to": "+94771234567",
  "text": "Hello from WacallerAPI!"
}
```

#### Send Media (Image, Audio, Video, Document)
```http
POST /api/v1/sessions/{id}/messages/media
Content-Type: application/json

{
  "to": "+94771234567",
  "type": "image",
  "url": "https://example.com/receipt.jpg",
  "caption": "Your receipt"
}
```

---

### 4. Webhooks

Configure a `webhook_url` on your session (or globally via `-webhook-url`). WacallerAPI sends real-time POST events:
- `call.incoming`: Caller is ringing (`call_id`, `from`)
- `call.connected`: Call accepted & audio media established
- `call.ended`: Call terminated (`duration_seconds`, `reason`)
- `message.received`: Incoming WhatsApp text or media
- `session.qr`: QR code updated
- `session.connected`: Session authenticated

Payloads are signed with HMAC-SHA256 in header `X-Wacaller-Signature: sha256=...` when `-webhook-secret` is configured.

---

## 💻 Developer SDK Examples

### Node.js / JavaScript
```javascript
const axios = require('axios');

const API_KEY = 'wac_xxxxxxxxxxxx';
const client = axios.create({
  baseURL: 'http://localhost:8080/api/v1',
  headers: { 'X-Api-Key': API_KEY }
});

// 1. Send Message
await client.post('/sessions/sess_123/messages/text', {
  to: '+94771234567',
  text: 'Hello from Node.js!'
});

// 2. Place Voice Call
const { data } = await client.post('/sessions/sess_123/calls', {
  to: '+94771234567',
  audio_url: 'https://example.com/greeting.wav'
});
console.log('Call started:', data.call.call_id);
```

### Python
```python
import requests

API_KEY = "wac_xxxxxxxxxxxx"
headers = {"X-Api-Key": API_KEY}

# Place Call
resp = requests.post("http://localhost:8080/api/v1/sessions/sess_123/calls", headers=headers, json={
    "to": "+94771234567",
    "audio_url": "https://example.com/greeting.wav"
})
print("Call status:", resp.json())
```

---

## 🔒 Configuration & Flags

| Flag | Env Variable | Default | Description |
| :--- | :--- | :--- | :--- |
| `-addr` | `WACALLER_ADDR` | `:8080` | HTTP & WebSocket listen address |
| `-db` | `WACALLER_DB` | `wacaller.db` | SQLite database path |
| `-api-key` | `WACALLER_API_KEY` | `""` | Master API Key (enables auth enforcement) |
| `-webhook-url`| `WACALLER_WEBHOOK_URL`| `""` | Global fallback webhook URL |
| `-webhook-secret`| `WACALLER_WEBHOOK_SECRET`| `""` | Secret for signing webhook payloads |
| `-max-calls-per-session`| `WACALLER_MAX_CALLS`| `8` | Max concurrent calls per session |
| `-debug` | `WACALLER_DEBUG` | `false` | Verbose debug logging |

---

## 📜 License
MIT License.

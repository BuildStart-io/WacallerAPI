package api

func GetOpenAPISpec() string {
	return `{
  "openapi": "3.0.3",
  "info": {
    "title": "WacallerAPI",
    "description": "Developer-facing WhatsApp API platform supporting both voice calls and messaging, modeled after WasenderAPI.",
    "version": "1.0.0"
  },
  "servers": [
    {
      "url": "/api",
      "description": "WasenderAPI Standard Base URL"
    },
    {
      "url": "/api/v1",
      "description": "Versioned Base URL"
    }
  ],
  "components": {
    "securitySchemes": {
      "ApiKeyAuth": {
        "type": "apiKey",
        "in": "header",
        "name": "X-Api-Key"
      },
      "BearerAuth": {
        "type": "http",
        "scheme": "bearer",
        "description": "Session API Key (wac_sess_...) or Master API Key"
      }
    }
  },
  "security": [
    { "ApiKeyAuth": [] },
    { "BearerAuth": [] }
  ],
  "paths": {
    "/send-message": {
      "post": {
        "summary": "Send WhatsApp message (WasenderAPI compatible)",
        "description": "Universal messaging endpoint for text, image, video, audio, document, and location.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["to"],
                "properties": {
                  "to": { "type": "string", "example": "+94771234567", "description": "Recipient phone number" },
                  "text": { "type": "string", "example": "Hello from WacallerAPI!", "description": "Message text or media caption" },
                  "imageUrl": { "type": "string", "example": "https://example.com/photo.jpg", "description": "Public URL for image" },
                  "videoUrl": { "type": "string", "example": "https://example.com/video.mp4", "description": "Public URL for video" },
                  "audioUrl": { "type": "string", "example": "https://example.com/voice.mp3", "description": "Public URL for audio / voice note" },
                  "documentUrl": { "type": "string", "example": "https://example.com/invoice.pdf", "description": "Public URL for document" },
                  "fileName": { "type": "string", "example": "invoice.pdf" },
                  "viewOnce": { "type": "boolean", "default": false },
                  "ptt": { "type": "boolean", "default": false, "description": "Send audio as voice note" },
                  "latitude": { "type": "number", "example": 6.9271 },
                  "longitude": { "type": "number", "example": 79.8612 },
                  "name": { "type": "string", "example": "Colombo" }
                }
              }
            }
          }
        },
        "responses": {
          "200": { "description": "Message successfully dispatched" }
        }
      }
    },
    "/send-call": {
      "post": {
        "summary": "Initiate WhatsApp voice call (Core Superpower)",
        "description": "Places an outbound WhatsApp call with optional audio playback or real-time WebSocket audio streaming.",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["to"],
                "properties": {
                  "to": { "type": "string", "example": "+94771234567" },
                  "audioUrl": { "type": "string", "example": "https://example.com/greeting.wav", "description": "Optional WAV URL to play when answered" },
                  "isVideo": { "type": "boolean", "default": false }
                }
              }
            }
          }
        },
        "responses": {
          "201": { "description": "Call initiated, returns callId and streamUrl" }
        }
      }
    },
    "/calls": {
      "get": {
        "summary": "List active and historical voice calls",
        "responses": { "200": { "description": "Call list" } }
      }
    },
    "/calls/{callId}": {
      "get": {
        "summary": "Get call details and live status",
        "responses": { "200": { "description": "Call details" } }
      },
      "delete": {
        "summary": "Terminate active call",
        "responses": { "200": { "description": "Call terminated" } }
      }
    },
    "/calls/{callId}/accept": {
      "post": {
        "summary": "Accept an incoming WhatsApp call",
        "responses": { "200": { "description": "Call accepted" } }
      }
    },
    "/calls/{callId}/reject": {
      "post": {
        "summary": "Reject an incoming WhatsApp call",
        "responses": { "200": { "description": "Call rejected" } }
      }
    },
    "/calls/{callId}/hangup": {
      "post": {
        "summary": "Hang up / end active call",
        "responses": { "200": { "description": "Call ended" } }
      }
    },
    "/calls/{callId}/play": {
      "post": {
        "summary": "Inject audio playback into an active call",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "audioUrl": { "type": "string", "example": "https://example.com/announcement.wav" },
                  "wavBase64": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Audio playback started" } }
      }
    },
    "/calls/{callId}/stream": {
      "get": {
        "summary": "Real-time Bi-Directional WebSocket Audio Stream (16 kHz PCM)",
        "description": "WebSocket endpoint to stream live audio frames to and from the WhatsApp caller.",
        "responses": { "101": { "description": "WebSocket upgraded" } }
      }
    },
    "/sessions": {
      "get": {
        "summary": "List WhatsApp sessions and API keys",
        "responses": { "200": { "description": "Sessions list" } }
      },
      "post": {
        "summary": "Create a new WhatsApp session",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "name": { "type": "string", "example": "Support Line" },
                  "webhook_url": { "type": "string", "example": "https://example.com/webhook" }
                }
              }
            }
          }
        },
        "responses": { "201": { "description": "Session created with dedicated API key" } }
      }
    },
    "/sessions/{id}": {
      "get": {
        "summary": "Get session info and status",
        "responses": { "200": { "description": "Session info" } }
      },
      "delete": {
        "summary": "Delete session",
        "responses": { "200": { "description": "Session deleted" } }
      }
    },
    "/sessions/{id}/qr": {
      "get": {
        "summary": "Get QR code for WhatsApp linking",
        "responses": { "200": { "description": "QR code payload" } }
      }
    },
    "/sessions/{id}/pair": {
      "post": {
        "summary": "Request 8-character pairing code for phone number",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["phone"],
                "properties": {
                  "phone": { "type": "string", "example": "+94771234567" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Pairing code generated" } }
      }
    },
    "/sessions/{id}/logout": {
      "post": {
        "summary": "Disconnect and log out session from WhatsApp",
        "responses": { "200": { "description": "Logged out" } }
      }
    },
    "/keys": {
      "get": {
        "summary": "List account API keys",
        "responses": { "200": { "description": "Keys list" } }
      },
      "post": {
        "summary": "Generate a new account API key",
        "responses": { "201": { "description": "API Key generated" } }
      }
    }
  }
}`
}

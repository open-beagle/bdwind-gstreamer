# WebRTC Selkies Protocol Compatibility

This document describes the server-side changes made to support the selkies-gstreamer signaling protocol alongside the existing custom protocol.

## Overview

The golang signaling server has been updated to support both the original custom protocol and the selkies-gstreamer standard protocol. This enables compatibility with selkies-based frontend implementations while maintaining backward compatibility.

## Protocol Support

### Selkies Protocol

The selkies protocol uses a simpler message format:

1. **HELLO Message**: `HELLO ${peer_id} ${btoa(JSON.stringify(meta))}`
2. **SDP Messages**: `{"sdp": {"type": "offer/answer", "sdp": "..."}}`
3. **ICE Messages**: `{"ice": {"candidate": "...", "sdpMid": "...", "sdpMLineIndex": 0}}`
4. **Server Responses**: Simple text responses like `"HELLO"` or `"ERROR message"`

### Original Protocol

The original protocol uses structured JSON messages with metadata:

```json
{
  "type": "message_type",
  "peer_id": "client_id",
  "data": {...},
  "message_id": "unique_id",
  "timestamp": 1234567890
}
```

## Implementation Details

### Client Detection

The server automatically detects selkies clients by:
1. Monitoring for `HELLO` messages in the selkies format
2. Setting the `IsSelkies` flag on the client when detected
3. Using different message formats based on this flag

### Message Handling

#### Selkies Message Processing

```go
func (c *SignalingClient) handleSelkiesMessage(messageText string) bool {
    // Handle HELLO messages
    if strings.HasPrefix(messageText, "HELLO ") {
        c.handleSelkiesHello(messageText)
        return true
    }
    
    // Handle JSON SDP/ICE messages
    var jsonMsg map[string]any
    if err := json.Unmarshal([]byte(messageText), &jsonMsg); err == nil {
        if sdpData, exists := jsonMsg["sdp"]; exists {
            c.handleSelkiesSDP(sdpData)
            return true
        }
        if iceData, exists := jsonMsg["ice"]; exists {
            c.handleSelkiesICE(iceData)
            return true
        }
    }
    
    return false
}
```

#### Response Format Selection

The server sends responses in the appropriate format:

```go
// For selkies clients
if c.isSelkiesClient() {
    selkiesOffer := map[string]any{
        "sdp": map[string]any{
            "type": "offer",
            "sdp":  offer.SDP,
        },
    }
    // Send JSON directly
} else {
    // Use original SignalingMessage format
}
```

### API Endpoints

#### Standard Configuration API

- **Endpoint**: `/api/webrtc/config`
- **Method**: GET
- **Response**: Full WebRTC configuration with metadata

```json
{
  "success": true,
  "data": {
    "iceServers": [...],
    "videoEnabled": true,
    "audioEnabled": false,
    "videoCodec": "h264",
    "connectionTimeout": 10000,
    "iceTransportPolicy": "all"
  },
  "timestamp": 1234567890
}
```

#### Selkies-Compatible ICE Servers API

- **Endpoint**: `/api/webrtc/ice-servers`
- **Method**: GET
- **Response**: Simple ICE servers configuration

```json
{
  "iceServers": [
    {
      "urls": ["stun:stun.l.google.com:19302"]
    },
    {
      "urls": ["turn:example.com:3478"],
      "username": "user",
      "credential": "pass"
    }
  ],
  "iceTransportPolicy": "all"
}
```

## Configuration

### ICE Servers

ICE servers are configured in the WebRTC configuration section:

```yaml
webrtc:
  ice_servers:
    - urls:
        - "stun:stun.l.google.com:19302"
    - urls:
        - "turn:example.com:3478"
      username: "turnuser"
      credential: "turnpass"
  signaling_port: 8081
  signaling_path: "/ws"
```

### TURN Server Support

TURN servers can be enabled with authentication:

```yaml
webrtc:
  enable_turn: true
  turn:
    host: "turn.example.com"
    port: 3478
    username: "turnuser"
    password: "turnpass"
    realm: "example.com"
```

## Backward Compatibility

The implementation maintains full backward compatibility:

1. **Existing Clients**: Continue to work with the original protocol
2. **Message Validation**: Both protocols are validated appropriately
3. **Error Handling**: Errors are sent in the expected format for each protocol
4. **API Endpoints**: Original endpoints remain unchanged

## Testing

### Manual Testing

1. **Selkies Client**: Connect using selkies-gstreamer frontend
2. **Original Client**: Connect using the existing frontend
3. **Mixed Environment**: Both client types can coexist

### API Testing

```bash
# Test standard config API
curl http://localhost:8080/api/webrtc/config

# Test selkies ICE servers API
curl http://localhost:8080/api/webrtc/ice-servers

# Test WebSocket signaling
wscat -c ws://localhost:8080/ws
```

## Troubleshooting

### Common Issues

1. **Protocol Mismatch**: Check client logs for protocol detection
2. **ICE Server Configuration**: Verify STUN/TURN server accessibility
3. **WebSocket Connection**: Ensure proper CORS and upgrade headers

### Debug Logging

Enable detailed logging to monitor protocol detection:

```go
log.Printf("🔄 Selkies HELLO message received from client %s: %s", c.ID, messageText)
log.Printf("📞 Selkies SDP message received from client %s", c.ID)
log.Printf("🧊 Selkies ICE message received from client %s", c.ID)
```

## Future Enhancements

1. **Protocol Negotiation**: Automatic protocol detection during handshake
2. **Configuration Validation**: Enhanced validation for selkies-specific settings
3. **Performance Optimization**: Protocol-specific optimizations
4. **Monitoring**: Protocol-aware metrics and monitoring
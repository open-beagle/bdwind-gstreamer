# WebRTC Protocol Adapters

This package implements protocol adapters for WebRTC signaling, providing standardized message handling and protocol conversion between different signaling formats.

## Overview

The protocol adapter system consists of several key components:

- **Protocol Adapters**: Handle parsing and formatting of different protocol formats
- **Message Validator**: Validates message format, content, and sequences
- **Protocol Manager**: Coordinates adapters and provides unified interface
- **Standard Message Format**: Common message structure for internal processing

## Supported Protocols

### GStreamer Standard Protocol (gstreamer-1.0)
- Full JSON-based message format
- Comprehensive metadata support
- Strict validation rules
- Version negotiation support

### Selkies Compatibility Protocol (selkies)
- Backward compatibility with Selkies format
- Text-based HELLO messages
- JSON SDP/ICE messages
- Legacy protocol mapping

## Key Features

### 1. Protocol Auto-Detection
```go
manager := NewProtocolManager(nil)
result, err := manager.ParseMessage(data) // Auto-detects protocol
```

### 2. Message Validation
- Format validation (required fields, data types)
- Content validation (SDP structure, ICE candidates)
- Sequence validation (handshake order, timing)

### 3. Protocol Conversion
```go
// Convert Selkies message to GStreamer format
converted, err := manager.ConvertMessage(data, ProtocolVersionSelkies, ProtocolVersionGStreamer10)
```

### 4. Extensible Architecture
```go
// Register custom adapter
customAdapter := NewCustomAdapter()
manager.RegisterAdapter("custom-1.0", customAdapter)
```

## Message Types

### Standard Message Types
- `hello`: Initial handshake
- `offer`: WebRTC offer (SDP)
- `answer`: WebRTC answer (SDP)
- `ice-candidate`: ICE candidate exchange
- `error`: Error messages
- `ping`/`pong`: Keep-alive
- `stats`: Statistics reporting

### Message Structure
```go
type StandardMessage struct {
    Version   ProtocolVersion    `json:"version"`
    Type      MessageType        `json:"type"`
    ID        string             `json:"id"`
    Timestamp int64              `json:"timestamp"`
    PeerID    string             `json:"peer_id,omitempty"`
    Data      any                `json:"data,omitempty"`
    Metadata  *MessageMetadata   `json:"metadata,omitempty"`
    Error     *MessageError      `json:"error,omitempty"`
}
```

## Usage Examples

### Basic Usage
```go
// Create protocol manager
config := DefaultManagerConfig()
manager := NewProtocolManager(config)

// Parse incoming message
result, err := manager.ParseMessage(messageData)
if err != nil {
    log.Printf("Parse error: %v", err)
    return
}

// Access parsed message
message := result.Message
log.Printf("Received %s message from %s", message.Type, message.PeerID)

// Format outgoing message
helloData := HelloData{
    PeerID: "client-123",
    Capabilities: []string{"webrtc", "input"},
}
outgoingMsg := NewStandardMessage(MessageTypeHello, "client-123", helloData)
data, err := manager.FormatMessage(outgoingMsg)
```

### Protocol-Specific Parsing
```go
// Parse with specific protocol
result, err := manager.ParseMessage(data, ProtocolVersionSelkies)

// Format with specific protocol
data, err := manager.FormatMessage(message, ProtocolVersionGStreamer10)
```

### Validation
```go
// Validate message
validator := NewMessageValidator(nil)
validationResult := validator.ValidateMessage(message)

if !validationResult.Valid {
    for _, error := range validationResult.Errors {
        log.Printf("Validation error: %s", error.Message)
    }
}
```

### Custom Adapter
```go
type CustomAdapter struct {
    // Implementation
}

func (c *CustomAdapter) ParseMessage(data []byte) (*StandardMessage, error) {
    // Custom parsing logic
}

func (c *CustomAdapter) FormatMessage(msg *StandardMessage) ([]byte, error) {
    // Custom formatting logic
}

// Register custom adapter
manager.RegisterAdapter("custom-1.0", &CustomAdapter{})
```

## Configuration

### Manager Configuration
```go
config := &ManagerConfig{
    DefaultProtocol:     ProtocolVersionGStreamer10,
    AutoDetection:       true,
    StrictValidation:    true,
    EnableLogging:       true,
    SupportedProtocols:  []ProtocolVersion{...},
    ValidatorConfig:     validatorConfig,
}
```

### Validator Configuration
```go
validatorConfig := &ValidatorConfig{
    StrictMode:          true,
    MaxMessageSize:      64 * 1024,
    MaxHistorySize:      100,
    EnableSequenceCheck: true,
    AllowedProtocols:    []ProtocolVersion{...},
}
```

### Adapter Configuration
```go
adapterConfig := &AdapterConfig{
    Version:             ProtocolVersionGStreamer10,
    StrictValidation:    true,
    MaxMessageSize:      64 * 1024,
    EnableCompression:   false,
    SupportedExtensions: []string{},
}
```

## Error Handling

### Validation Errors
```go
type ValidationError struct {
    Code        string       `json:"code"`
    Message     string       `json:"message"`
    Field       string       `json:"field,omitempty"`
    Value       any          `json:"value,omitempty"`
    Severity    RuleSeverity `json:"severity"`
    Suggestion  string       `json:"suggestion,omitempty"`
}
```

### Error Severities
- `SeverityError`: Critical errors that prevent processing
- `SeverityWarning`: Issues that should be addressed but don't prevent processing
- `SeverityInfo`: Informational messages for debugging

## Testing

Run the test suite:
```bash
go test -v ./internal/webrtc/protocol
```

### Test Coverage
- Unit tests for all adapters
- Integration tests for protocol conversion
- Validation rule testing
- Compatibility testing

## Performance Considerations

- Message parsing is optimized for common cases
- Validation can be disabled for performance-critical paths
- Message history is limited to prevent memory leaks
- Protocol detection uses confidence scoring for accuracy

## Thread Safety

- Protocol manager is thread-safe
- Individual adapters are stateless and thread-safe
- Message validator maintains thread-safe history

## Future Extensions

The architecture supports:
- Additional protocol adapters
- Custom validation rules
- Protocol versioning
- Message compression
- Encryption support
- Custom message types

## Dependencies

- Standard Go library only
- No external dependencies for core functionality
- JSON marshaling/unmarshaling for message processing
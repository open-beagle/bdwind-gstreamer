package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewStandardMessage(t *testing.T) {
	data := map[string]any{
		"test": "value",
	}

	msg := NewStandardMessage(MessageTypeHello, "test-peer", data)

	if msg.Type != MessageTypeHello {
		t.Errorf("Expected type %s, got %s", MessageTypeHello, msg.Type)
	}

	if msg.PeerID != "test-peer" {
		t.Errorf("Expected peer ID 'test-peer', got '%s'", msg.PeerID)
	}

	// Compare data by converting to JSON strings since maps can't be compared directly
	expectedJSON, _ := json.Marshal(data)
	actualJSON, _ := json.Marshal(msg.Data)
	if string(expectedJSON) != string(actualJSON) {
		t.Errorf("Expected data %v, got %v", data, msg.Data)
	}

	if msg.Version != ProtocolVersionGStreamer10 {
		t.Errorf("Expected version %s, got %s", ProtocolVersionGStreamer10, msg.Version)
	}

	if msg.ID == "" {
		t.Error("Expected non-empty message ID")
	}

	if msg.Timestamp <= 0 {
		t.Error("Expected positive timestamp")
	}
}

func TestNewErrorMessage(t *testing.T) {
	msg := NewErrorMessage("TEST_ERROR", "Test error message", "Test details")

	if msg.Type != MessageTypeError {
		t.Errorf("Expected type %s, got %s", MessageTypeError, msg.Type)
	}

	if msg.Error == nil {
		t.Fatal("Expected error information")
	}

	if msg.Error.Code != "TEST_ERROR" {
		t.Errorf("Expected error code 'TEST_ERROR', got '%s'", msg.Error.Code)
	}

	if msg.Error.Message != "Test error message" {
		t.Errorf("Expected error message 'Test error message', got '%s'", msg.Error.Message)
	}

	if msg.Error.Details != "Test details" {
		t.Errorf("Expected error details 'Test details', got '%s'", msg.Error.Details)
	}
}

func TestStandardMessage_IsValid(t *testing.T) {
	// Valid message
	validMsg := &StandardMessage{
		Type:      MessageTypeHello,
		ID:        "test-id",
		Timestamp: time.Now().Unix(),
	}

	if !validMsg.IsValid() {
		t.Error("Expected valid message to return true")
	}

	// Invalid message - missing type
	invalidMsg1 := &StandardMessage{
		ID:        "test-id",
		Timestamp: time.Now().Unix(),
	}

	if invalidMsg1.IsValid() {
		t.Error("Expected invalid message (missing type) to return false")
	}

	// Invalid message - missing ID
	invalidMsg2 := &StandardMessage{
		Type:      MessageTypeHello,
		Timestamp: time.Now().Unix(),
	}

	if invalidMsg2.IsValid() {
		t.Error("Expected invalid message (missing ID) to return false")
	}

	// Invalid message - missing timestamp
	invalidMsg3 := &StandardMessage{
		Type: MessageTypeHello,
		ID:   "test-id",
	}

	if invalidMsg3.IsValid() {
		t.Error("Expected invalid message (missing timestamp) to return false")
	}
}

func TestStandardMessage_GetDataAs(t *testing.T) {
	// Test with valid data
	helloData := HelloData{
		PeerID:       "test-peer",
		Capabilities: []string{"webrtc", "input"},
		Metadata:     map[string]any{"version": "1.0"},
	}

	msg := NewStandardMessage(MessageTypeHello, "test-peer", helloData)

	var retrievedData HelloData
	err := msg.GetDataAs(&retrievedData)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if retrievedData.PeerID != helloData.PeerID {
		t.Errorf("Expected peer ID '%s', got '%s'", helloData.PeerID, retrievedData.PeerID)
	}

	if len(retrievedData.Capabilities) != len(helloData.Capabilities) {
		t.Errorf("Expected %d capabilities, got %d", len(helloData.Capabilities), len(retrievedData.Capabilities))
	}

	// Test with nil data
	msgWithNilData := &StandardMessage{
		Type: MessageTypeHello,
		ID:   "test-id",
		Data: nil,
	}

	var nilData HelloData
	err = msgWithNilData.GetDataAs(&nilData)
	if err == nil {
		t.Error("Expected error for nil data")
	}
}

func TestStandardMessage_AddCapability(t *testing.T) {
	msg := NewStandardMessage(MessageTypeHello, "test-peer", nil)

	// Add first capability
	msg.AddCapability("webrtc")

	if !msg.HasCapability("webrtc") {
		t.Error("Expected message to have 'webrtc' capability")
	}

	// Add second capability
	msg.AddCapability("input")

	if !msg.HasCapability("input") {
		t.Error("Expected message to have 'input' capability")
	}

	// Try to add duplicate capability
	msg.AddCapability("webrtc")

	// Count capabilities
	count := 0
	for _, cap := range msg.Metadata.Capabilities {
		if cap == "webrtc" {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Expected 1 'webrtc' capability, got %d", count)
	}
}

func TestGenerateMessageID(t *testing.T) {
	id1 := generateMessageID()
	id2 := generateMessageID()

	if id1 == "" {
		t.Error("Expected non-empty message ID")
	}

	if id2 == "" {
		t.Error("Expected non-empty message ID")
	}

	if id1 == id2 {
		t.Error("Expected different message IDs")
	}

	// Check format
	if len(id1) < 10 {
		t.Error("Expected message ID to be at least 10 characters")
	}
}

func TestDefaultAdapterConfig(t *testing.T) {
	config := DefaultAdapterConfig(ProtocolVersionGStreamer10)

	if config.Version != ProtocolVersionGStreamer10 {
		t.Errorf("Expected version %s, got %s", ProtocolVersionGStreamer10, config.Version)
	}

	if !config.StrictValidation {
		t.Error("Expected strict validation to be enabled by default")
	}

	if config.MaxMessageSize <= 0 {
		t.Error("Expected positive max message size")
	}

	if config.SupportedExtensions == nil {
		t.Error("Expected non-nil supported extensions")
	}
}

func TestMessageSerialization(t *testing.T) {
	// Create a message with various data types
	originalMsg := NewStandardMessage(MessageTypeHello, "test-peer", HelloData{
		PeerID:       "test-peer",
		Capabilities: []string{"webrtc", "input"},
		Metadata: map[string]any{
			"version": "1.0",
			"number":  42,
			"boolean": true,
		},
	})

	// Serialize to JSON
	data, err := json.Marshal(originalMsg)
	if err != nil {
		t.Fatalf("Failed to serialize message: %v", err)
	}

	// Deserialize from JSON
	var deserializedMsg StandardMessage
	err = json.Unmarshal(data, &deserializedMsg)
	if err != nil {
		t.Fatalf("Failed to deserialize message: %v", err)
	}

	// Verify basic fields
	if deserializedMsg.Type != originalMsg.Type {
		t.Errorf("Expected type %s, got %s", originalMsg.Type, deserializedMsg.Type)
	}

	if deserializedMsg.ID != originalMsg.ID {
		t.Errorf("Expected ID %s, got %s", originalMsg.ID, deserializedMsg.ID)
	}

	if deserializedMsg.PeerID != originalMsg.PeerID {
		t.Errorf("Expected peer ID %s, got %s", originalMsg.PeerID, deserializedMsg.PeerID)
	}

	if deserializedMsg.Timestamp != originalMsg.Timestamp {
		t.Errorf("Expected timestamp %d, got %d", originalMsg.Timestamp, deserializedMsg.Timestamp)
	}

	// Verify data can be extracted
	var helloData HelloData
	err = deserializedMsg.GetDataAs(&helloData)
	if err != nil {
		t.Fatalf("Failed to extract hello data: %v", err)
	}

	if helloData.PeerID != "test-peer" {
		t.Errorf("Expected peer ID 'test-peer', got '%s'", helloData.PeerID)
	}
}

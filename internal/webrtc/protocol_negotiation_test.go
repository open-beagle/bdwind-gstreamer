package webrtc

import (
	"encoding/json"
	"testing"
	"time"
)

// TestProtocolNegotiation 测试协议协商功能
func TestProtocolNegotiation(t *testing.T) {
	// 创建测试客户端
	client := &SignalingClient{
		ID:           "test-client-1",
		AppName:      "test-app",
		LastSeen:     time.Now(),
		ConnectedAt:  time.Now(),
		State:        ClientStateConnected,
		MessageCount: 0,
		ErrorCount:   0,
	}

	// 测试协议协商
	clientProtocols := []string{"gstreamer-1.0", "selkies"}
	serverProtocols := []string{"gstreamer-1.0", "selkies", "legacy"}

	selectedProtocol := client.negotiateProtocol(clientProtocols, serverProtocols)

	if selectedProtocol != "gstreamer-1.0" {
		t.Errorf("Expected gstreamer-1.0, got %s", selectedProtocol)
	}
}

// TestProtocolCompatibility 测试协议兼容性检查
func TestProtocolCompatibility(t *testing.T) {
	client := &SignalingClient{ID: "test-client"}

	testCases := []struct {
		server   string
		client   string
		expected bool
	}{
		{"gstreamer-1.0", "gstreamer-1.0", true},
		{"gstreamer-1.0", "gstreamer", true},
		{"selkies", "selkies", true},
		{"selkies", "selkies-1.0", true},
		{"legacy", "unknown", true},
		{"gstreamer-1.0", "selkies", false},
		{"selkies", "gstreamer-1.0", false},
	}

	for _, tc := range testCases {
		result := client.isProtocolCompatible(tc.server, tc.client)
		if result != tc.expected {
			t.Errorf("isProtocolCompatible(%s, %s) = %v, expected %v",
				tc.server, tc.client, result, tc.expected)
		}
	}
}

// TestProtocolDetection 测试协议检测功能
func TestProtocolDetection(t *testing.T) {
	client := &SignalingClient{ID: "test-client"}

	testCases := []struct {
		message  string
		expected string
	}{
		{`HELLO 1`, "selkies"},
		{`{"version":"1.0","type":"ping"}`, "gstreamer-webrtc"},
		{`{"sdp":{"type":"offer"}}`, "selkies"},
		{`{"ice":{"candidate":"test"}}`, "selkies"},
		{`{"type":"protocol-negotiation"}`, "gstreamer-webrtc"},
		{`invalid json`, "unknown"},
	}

	for _, tc := range testCases {
		result := client.detectProtocolFromMessage([]byte(tc.message))
		if result != tc.expected {
			t.Errorf("detectProtocolFromMessage(%s) = %s, expected %s",
				tc.message, result, tc.expected)
		}
	}
}

// TestProtocolDowngrade 测试协议降级功能
func TestProtocolDowngrade(t *testing.T) {
	client := &SignalingClient{
		ID:           "test-client",
		MessageCount: 0,
	}

	testCases := []struct {
		current  string
		expected string
	}{
		{"gstreamer-1.0", "selkies"},
		{"selkies", "legacy"},
		{"legacy", "legacy"},   // 已经是最低级，不能再降级
		{"unknown", "unknown"}, // 未知协议，不能降级
	}

	for _, tc := range testCases {
		result := client.downgradeProtocol(tc.current, "test_reason")
		if result != tc.expected {
			t.Errorf("downgradeProtocol(%s) = %s, expected %s",
				tc.current, result, tc.expected)
		}
	}
}

// TestProtocolCapabilities 测试协议能力获取
func TestProtocolCapabilities(t *testing.T) {
	client := &SignalingClient{ID: "test-client"}

	// 测试 gstreamer-1.0 协议能力
	capabilities := client.getProtocolCapabilities("gstreamer-1.0")
	expectedCapabilities := []string{
		"webrtc", "datachannel", "video-h264", "video-vp8", "video-vp9",
		"audio-opus", "input-events", "statistics", "error-recovery",
	}

	if len(capabilities) != len(expectedCapabilities) {
		t.Errorf("Expected %d capabilities, got %d", len(expectedCapabilities), len(capabilities))
	}

	// 检查是否包含关键能力
	hasWebRTC := false
	for _, cap := range capabilities {
		if cap == "webrtc" {
			hasWebRTC = true
			break
		}
	}

	if !hasWebRTC {
		t.Error("Expected webrtc capability in gstreamer-1.0 protocol")
	}
}

// TestProtocolVersionInfo 测试协议版本信息
func TestProtocolVersionInfo(t *testing.T) {
	client := &SignalingClient{ID: "test-client"}

	testCases := []struct {
		protocol string
		expected string
	}{
		{"gstreamer-1.0", "1.0"},
		{"selkies", "1.0"},
		{"legacy", "0.9"},
		{"unknown", "1.0"}, // 默认版本
	}

	for _, tc := range testCases {
		result := client.getProtocolVersion(tc.protocol)
		if result != tc.expected {
			t.Errorf("getProtocolVersion(%s) = %s, expected %s",
				tc.protocol, result, tc.expected)
		}
	}
}

// TestDetectionConfidence 测试协议检测置信度
func TestDetectionConfidence(t *testing.T) {
	client := &SignalingClient{ID: "test-client"}

	testCases := []struct {
		protocol string
		message  string
		minConf  float64
	}{
		{"selkies", `HELLO 1`, 0.90},
		{"selkies", `{"sdp":{"type":"offer"}}`, 0.80},
		{"gstreamer-webrtc", `{"version":"1.0","metadata":{"protocol":"gstreamer"}}`, 0.90},
		{"gstreamer-webrtc", `{"version":"1.0"}`, 0.70},
	}

	for _, tc := range testCases {
		confidence := client.getDetectionConfidence(tc.protocol, []byte(tc.message))
		if confidence < tc.minConf {
			t.Errorf("getDetectionConfidence(%s, %s) = %.2f, expected >= %.2f",
				tc.protocol, tc.message, confidence, tc.minConf)
		}
	}
}

// TestProtocolNegotiationMessage 测试协议协商消息处理
func TestProtocolNegotiationMessage(t *testing.T) {
	// 创建协商消息
	negotiationData := map[string]any{
		"supported_protocols": []any{"gstreamer-1.0", "selkies"},
		"preferred_protocol":  "gstreamer-1.0",
		"client_capabilities": []string{"webrtc", "video", "audio"},
	}

	message := SignalingMessage{
		Type:      "protocol-negotiation",
		MessageID: "test-msg-1",
		Data:      negotiationData,
	}

	// 验证消息结构
	messageBytes, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("Failed to marshal negotiation message: %v", err)
	}

	// 验证消息可以被解析
	var parsedMessage SignalingMessage
	if err := json.Unmarshal(messageBytes, &parsedMessage); err != nil {
		t.Fatalf("Failed to unmarshal negotiation message: %v", err)
	}

	if parsedMessage.Type != "protocol-negotiation" {
		t.Errorf("Expected type protocol-negotiation, got %s", parsedMessage.Type)
	}
}

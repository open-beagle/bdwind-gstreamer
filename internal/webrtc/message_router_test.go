package webrtc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc/protocol"
)

// TestMessageRouter_RouteMessage 测试消息路由功能
func TestMessageRouter_RouteMessage(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	// 测试用例
	tests := []struct {
		name         string
		messageData  []byte
		clientID     string
		expectError  bool
		expectedType protocol.MessageType
		validateFunc func(t *testing.T, result *RouteResult)
	}{
		{
			name: "Route request-offer message",
			messageData: []byte(`{
				"type": "request-offer",
				"id": "test-msg-1",
				"timestamp": 1640995200,
				"peer_id": "test-client",
				"data": {
					"constraints": {
						"video": true,
						"audio": true
					},
					"codec_preferences": ["H264", "VP8"]
				}
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypeRequestOffer,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.NotNil(t, result.Message.Data)
				data, ok := result.Message.Data.(map[string]any)
				require.True(t, ok)
				assert.Contains(t, data, "constraints")
				assert.Contains(t, data, "codec_preferences")
			},
		},
		{
			name: "Route ping message",
			messageData: []byte(`{
				"type": "ping",
				"id": "ping-msg-1",
				"timestamp": 1640995200,
				"data": {
					"timestamp": 1640995200,
					"client_state": "active"
				}
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypePing,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.NotNil(t, result.Message.Data)
				data, ok := result.Message.Data.(map[string]any)
				require.True(t, ok)
				assert.Contains(t, data, "timestamp")
				assert.Contains(t, data, "client_state")
			},
		},
		{
			name: "Route offer message",
			messageData: []byte(`{
				"type": "offer",
				"id": "offer-msg-1",
				"timestamp": 1640995200,
				"data": {
					"type": "offer",
					"sdp": "v=0\r\no=- 123456789 2 IN IP4 127.0.0.1\r\n..."
				}
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypeOffer,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.NotNil(t, result.Message.Data)
				data, ok := result.Message.Data.(map[string]any)
				require.True(t, ok)
				assert.Contains(t, data, "type")
				assert.Contains(t, data, "sdp")
				assert.Equal(t, "offer", data["type"])
			},
		},
		{
			name: "Route answer message",
			messageData: []byte(`{
				"type": "answer",
				"id": "answer-msg-1",
				"timestamp": 1640995200,
				"data": {
					"type": "answer",
					"sdp": "v=0\r\no=- 987654321 2 IN IP4 127.0.0.1\r\n..."
				}
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypeAnswer,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.NotNil(t, result.Message.Data)
				data, ok := result.Message.Data.(map[string]any)
				require.True(t, ok)
				assert.Contains(t, data, "type")
				assert.Contains(t, data, "sdp")
				assert.Equal(t, "answer", data["type"])
			},
		},
		{
			name: "Route ICE candidate message",
			messageData: []byte(`{
				"type": "ice-candidate",
				"id": "ice-msg-1",
				"timestamp": 1640995200,
				"data": {
					"candidate": "candidate:1 1 UDP 2130706431 192.168.1.100 54400 typ host",
					"sdpMid": "0",
					"sdpMLineIndex": 0
				}
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypeICECandidate,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.NotNil(t, result.Message.Data)
				data, ok := result.Message.Data.(map[string]any)
				require.True(t, ok)
				assert.Contains(t, data, "candidate")
				assert.Contains(t, data, "sdpMid")
				assert.Contains(t, data, "sdpMLineIndex")
			},
		},
		{
			name: "Route message with missing client ID",
			messageData: []byte(`{
				"type": "ping",
				"id": "ping-msg-2",
				"timestamp": 1640995200
			}`),
			clientID:     "auto-assigned-client",
			expectError:  false,
			expectedType: protocol.MessageTypePing,
			validateFunc: func(t *testing.T, result *RouteResult) {
				// 验证客户端ID被自动设置
				assert.Equal(t, "auto-assigned-client", result.Message.PeerID)
			},
		},
		{
			name: "Route message with missing timestamp",
			messageData: []byte(`{
				"type": "ping",
				"id": "ping-msg-3"
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypePing,
			validateFunc: func(t *testing.T, result *RouteResult) {
				// 验证时间戳被自动设置
				assert.Greater(t, result.Message.Timestamp, int64(0))
			},
		},
		{
			name: "Route message with missing ID",
			messageData: []byte(`{
				"type": "ping",
				"timestamp": 1640995200
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypePing,
			validateFunc: func(t *testing.T, result *RouteResult) {
				// 验证消息ID被自动设置
				assert.NotEmpty(t, result.Message.ID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := router.RouteMessage(tt.messageData, tt.clientID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				require.NotNil(t, result.Message)

				// 验证基本字段
				assert.Equal(t, tt.expectedType, result.Message.Type)
				assert.Equal(t, tt.clientID, result.Message.PeerID)
				assert.Greater(t, result.Message.Timestamp, int64(0))
				assert.NotEmpty(t, result.Message.ID)

				// 运行自定义验证函数
				if tt.validateFunc != nil {
					tt.validateFunc(t, result)
				}
			}
		})
	}
}

// TestMessageRouter_FormatResponse 测试响应格式化功能
func TestMessageRouter_FormatResponse(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	// 测试用例
	tests := []struct {
		name           string
		message        *protocol.StandardMessage
		targetProtocol protocol.ProtocolVersion
		expectError    bool
		validateFunc   func(t *testing.T, data []byte)
	}{
		{
			name: "Format pong response",
			message: &protocol.StandardMessage{
				Type:      protocol.MessageTypePong,
				ID:        "pong-msg-1",
				Timestamp: time.Now().Unix(),
				PeerID:    "test-client",
				Data: map[string]any{
					"timestamp":   time.Now().Unix(),
					"server_time": time.Now().Unix(),
					"client_id":   "test-client",
				},
			},
			targetProtocol: protocol.ProtocolVersionGStreamer10,
			expectError:    false,
			validateFunc: func(t *testing.T, data []byte) {
				var response map[string]any
				err := json.Unmarshal(data, &response)
				require.NoError(t, err)

				assert.Equal(t, "pong", response["type"])
				assert.Contains(t, response, "data")
				assert.Contains(t, response, "timestamp")
				assert.Contains(t, response, "peer_id")
			},
		},
		{
			name: "Format offer response",
			message: &protocol.StandardMessage{
				Type:      protocol.MessageTypeOffer,
				ID:        "offer-msg-1",
				Timestamp: time.Now().Unix(),
				PeerID:    "test-client",
				Data: map[string]any{
					"type": "offer",
					"sdp":  "v=0\r\no=- 123456789 2 IN IP4 127.0.0.1\r\n...",
				},
			},
			targetProtocol: protocol.ProtocolVersionGStreamer10,
			expectError:    false,
			validateFunc: func(t *testing.T, data []byte) {
				var response map[string]any
				err := json.Unmarshal(data, &response)
				require.NoError(t, err)

				assert.Equal(t, "offer", response["type"])
				assert.Contains(t, response, "data")

				responseData, ok := response["data"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "offer", responseData["type"])
				assert.Contains(t, responseData, "sdp")
			},
		},
		{
			name: "Format error response",
			message: &protocol.StandardMessage{
				Type:      protocol.MessageTypeError,
				ID:        "error-msg-1",
				Timestamp: time.Now().Unix(),
				PeerID:    "test-client",
				Error: &protocol.MessageError{
					Code:    "INVALID_MESSAGE",
					Message: "Invalid message format",
					Details: "Message validation failed",
					Type:    "validation_error",
				},
			},
			targetProtocol: protocol.ProtocolVersionGStreamer10,
			expectError:    false,
			validateFunc: func(t *testing.T, data []byte) {
				var response map[string]any
				err := json.Unmarshal(data, &response)
				require.NoError(t, err)

				assert.Equal(t, "error", response["type"])
				assert.Contains(t, response, "error")

				errorData, ok := response["error"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "INVALID_MESSAGE", errorData["code"])
				assert.Equal(t, "Invalid message format", errorData["message"])
			},
		},
		{
			name:           "Format nil message",
			message:        nil,
			targetProtocol: protocol.ProtocolVersionGStreamer10,
			expectError:    true,
			validateFunc:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := router.FormatResponse(tt.message, tt.targetProtocol)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, data)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, data)
				assert.Greater(t, len(data), 0)

				// 运行自定义验证函数
				if tt.validateFunc != nil {
					tt.validateFunc(t, data)
				}
			}
		})
	}
}

// TestMessageRouter_CreateStandardResponse 测试标准响应创建
func TestMessageRouter_CreateStandardResponse(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	// 测试用例
	tests := []struct {
		name     string
		msgType  protocol.MessageType
		peerID   string
		data     any
		validate func(t *testing.T, msg *protocol.StandardMessage)
	}{
		{
			name:    "Create pong response",
			msgType: protocol.MessageTypePong,
			peerID:  "test-client",
			data: map[string]any{
				"timestamp":   time.Now().Unix(),
				"server_time": time.Now().Unix(),
				"client_id":   "test-client",
			},
			validate: func(t *testing.T, msg *protocol.StandardMessage) {
				assert.Equal(t, protocol.MessageTypePong, msg.Type)
				assert.Equal(t, "test-client", msg.PeerID)
				assert.NotNil(t, msg.Data)

				data, ok := msg.Data.(map[string]any)
				require.True(t, ok)
				assert.Contains(t, data, "timestamp")
				assert.Contains(t, data, "server_time")
				assert.Contains(t, data, "client_id")
			},
		},
		{
			name:    "Create offer response",
			msgType: protocol.MessageTypeOffer,
			peerID:  "test-client",
			data: map[string]any{
				"type": "offer",
				"sdp":  "v=0\r\no=- 123456789 2 IN IP4 127.0.0.1\r\n...",
			},
			validate: func(t *testing.T, msg *protocol.StandardMessage) {
				assert.Equal(t, protocol.MessageTypeOffer, msg.Type)
				assert.Equal(t, "test-client", msg.PeerID)
				assert.NotNil(t, msg.Data)

				data, ok := msg.Data.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "offer", data["type"])
				assert.Contains(t, data, "sdp")
			},
		},
		{
			name:    "Create welcome response",
			msgType: protocol.MessageTypeWelcome,
			peerID:  "test-client",
			data: map[string]any{
				"client_id":    "test-client",
				"server_time":  time.Now().Unix(),
				"capabilities": []string{"webrtc", "input", "stats"},
			},
			validate: func(t *testing.T, msg *protocol.StandardMessage) {
				assert.Equal(t, protocol.MessageTypeWelcome, msg.Type)
				assert.Equal(t, "test-client", msg.PeerID)
				assert.NotNil(t, msg.Data)

				data, ok := msg.Data.(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "test-client", data["client_id"])
				assert.Contains(t, data, "server_time")
				assert.Contains(t, data, "capabilities")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := router.CreateStandardResponse(tt.msgType, tt.peerID, tt.data)

			require.NotNil(t, msg)
			assert.Equal(t, tt.msgType, msg.Type)
			assert.Equal(t, tt.peerID, msg.PeerID)
			assert.NotEmpty(t, msg.ID)
			assert.Greater(t, msg.Timestamp, int64(0))

			// 运行自定义验证函数
			if tt.validate != nil {
				tt.validate(t, msg)
			}
		})
	}
}

// TestMessageRouter_CreateErrorResponse 测试错误响应创建
func TestMessageRouter_CreateErrorResponse(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	// 测试用例
	tests := []struct {
		name    string
		code    string
		message string
		details string
	}{
		{
			name:    "Create validation error",
			code:    "INVALID_MESSAGE",
			message: "Invalid message format",
			details: "Message validation failed",
		},
		{
			name:    "Create connection error",
			code:    "CONNECTION_FAILED",
			message: "WebRTC connection failed",
			details: "Failed to establish peer connection",
		},
		{
			name:    "Create server error",
			code:    "INTERNAL_ERROR",
			message: "Internal server error",
			details: "An unexpected error occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := router.CreateErrorResponse(tt.code, tt.message, tt.details)

			require.NotNil(t, msg)
			assert.Equal(t, protocol.MessageTypeError, msg.Type)
			assert.NotEmpty(t, msg.ID)
			assert.Greater(t, msg.Timestamp, int64(0))

			require.NotNil(t, msg.Error)
			assert.Equal(t, tt.code, msg.Error.Code)
			assert.Equal(t, tt.message, msg.Error.Message)
			assert.Equal(t, tt.details, msg.Error.Details)
		})
	}
}

// TestMessageRouter_DetectProtocol 测试协议检测功能
func TestMessageRouter_DetectProtocol(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	// 测试用例
	tests := []struct {
		name             string
		messageData      []byte
		expectedProtocol protocol.ProtocolVersion
		minConfidence    float64
	}{
		{
			name: "Detect GStreamer protocol",
			messageData: []byte(`{
				"version": "1.0",
				"type": "request-offer",
				"id": "test-msg-1",
				"timestamp": 1640995200,
				"metadata": {
					"protocol": "gstreamer-1.0"
				}
			}`),
			expectedProtocol: protocol.ProtocolVersionGStreamer10,
			minConfidence:    0.8,
		},
		{
			name: "Detect Selkies protocol",
			messageData: []byte(`{
				"sdp": {
					"type": "offer",
					"sdp": "v=0..."
				}
			}`),
			expectedProtocol: protocol.ProtocolVersionSelkies,
			minConfidence:    0.7,
		},
		{
			name:             "Detect Selkies text protocol",
			messageData:      []byte("HELLO test-client"),
			expectedProtocol: protocol.ProtocolVersionSelkies,
			minConfidence:    0.9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.DetectProtocol(tt.messageData)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedProtocol, result.Protocol)
			assert.GreaterOrEqual(t, result.Confidence, tt.minConfidence)
		})
	}
}

// TestMessageRouter_GetSupportedProtocols 测试获取支持的协议列表
func TestMessageRouter_GetSupportedProtocols(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	protocols := router.GetSupportedProtocols()

	assert.NotEmpty(t, protocols)
	assert.Contains(t, protocols, protocol.ProtocolVersionGStreamer10)
	assert.Contains(t, protocols, protocol.ProtocolVersionSelkies)
}

// TestMessageRouter_GetSupportedMessageTypes 测试获取支持的消息类型
func TestMessageRouter_GetSupportedMessageTypes(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	// 测试GStreamer协议支持的消息类型
	messageTypes, err := router.GetSupportedMessageTypes(protocol.ProtocolVersionGStreamer10)
	assert.NoError(t, err)
	assert.NotEmpty(t, messageTypes)

	// 验证包含基本消息类型
	assert.Contains(t, messageTypes, protocol.MessageTypePing)
	assert.Contains(t, messageTypes, protocol.MessageTypePong)
	assert.Contains(t, messageTypes, protocol.MessageTypeRequestOffer)
	assert.Contains(t, messageTypes, protocol.MessageTypeOffer)
	assert.Contains(t, messageTypes, protocol.MessageTypeAnswer)
	assert.Contains(t, messageTypes, protocol.MessageTypeICECandidate)

	// 测试Selkies协议支持的消息类型
	selkiesMessageTypes, err := router.GetSupportedMessageTypes(protocol.ProtocolVersionSelkies)
	assert.NoError(t, err)
	assert.NotEmpty(t, selkiesMessageTypes)
}

// TestMessageRouter_IsProtocolSupported 测试协议支持检查
func TestMessageRouter_IsProtocolSupported(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	// 测试支持的协议
	assert.True(t, router.IsProtocolSupported(protocol.ProtocolVersionGStreamer10))
	assert.True(t, router.IsProtocolSupported(protocol.ProtocolVersionSelkies))

	// 测试不支持的协议
	assert.False(t, router.IsProtocolSupported(protocol.ProtocolVersion("unknown-protocol")))
}

// TestMessageRouter_GetProtocolCapabilities 测试获取协议能力
func TestMessageRouter_GetProtocolCapabilities(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	// 测试GStreamer协议能力
	capabilities, err := router.GetProtocolCapabilities(protocol.ProtocolVersionGStreamer10)
	assert.NoError(t, err)
	require.NotNil(t, capabilities)

	assert.Contains(t, capabilities, "protocol")
	assert.Contains(t, capabilities, "message_types")
	assert.Contains(t, capabilities, "features")

	assert.Equal(t, string(protocol.ProtocolVersionGStreamer10), capabilities["protocol"])

	// 测试不支持的协议
	_, err = router.GetProtocolCapabilities(protocol.ProtocolVersion("unknown-protocol"))
	assert.Error(t, err)
}

// TestMessageRouter_UpdateConfig 测试配置更新
func TestMessageRouter_UpdateConfig(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	// 获取初始配置
	initialConfig := router.GetConfig()
	assert.True(t, initialConfig.EnableLogging)
	assert.True(t, initialConfig.StrictValidation)

	// 更新配置
	newConfig := &MessageRouterConfig{
		EnableLogging:      false,
		StrictValidation:   false,
		AutoProtocolDetect: false,
		MaxRetries:         5,
	}

	router.UpdateConfig(newConfig)

	// 验证配置已更新
	updatedConfig := router.GetConfig()
	assert.False(t, updatedConfig.EnableLogging)
	assert.False(t, updatedConfig.StrictValidation)
	assert.False(t, updatedConfig.AutoProtocolDetect)
	assert.Equal(t, 5, updatedConfig.MaxRetries)

	// 测试nil配置更新
	router.UpdateConfig(nil)
	finalConfig := router.GetConfig()
	assert.Equal(t, updatedConfig, finalConfig) // 配置应该保持不变
}

// TestMessageRouter_GetStats 测试统计信息获取
func TestMessageRouter_GetStats(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	stats := router.GetStats()

	require.NotNil(t, stats)
	assert.Contains(t, stats, "config")
	assert.Contains(t, stats, "protocol_manager")

	// 验证配置统计
	configStats, ok := stats["config"].(*MessageRouterConfig)
	require.True(t, ok)
	assert.NotNil(t, configStats)
}

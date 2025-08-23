package webrtc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc/protocol"
)

// MockWebSocketConn 模拟WebSocket连接
type MockWebSocketConn struct {
	mock.Mock
	messages chan []byte
	closed   bool
}

func NewMockWebSocketConn() *MockWebSocketConn {
	return &MockWebSocketConn{
		messages: make(chan []byte, 100),
	}
}

func (m *MockWebSocketConn) ReadMessage() (int, []byte, error) {
	args := m.Called()
	return args.Int(0), args.Get(1).([]byte), args.Error(2)
}

func (m *MockWebSocketConn) WriteMessage(messageType int, data []byte) error {
	args := m.Called(messageType, data)
	if !m.closed {
		m.messages <- data
	}
	return args.Error(0)
}

func (m *MockWebSocketConn) Close() error {
	m.closed = true
	close(m.messages)
	args := m.Called()
	return args.Error(0)
}

func (m *MockWebSocketConn) SetReadDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *MockWebSocketConn) SetWriteDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *MockWebSocketConn) SetPongHandler(h func(string) error) {
	m.Called(h)
}

func (m *MockWebSocketConn) RemoteAddr() interface{} {
	args := m.Called()
	return args.Get(0)
}

func (m *MockWebSocketConn) Subprotocol() string {
	args := m.Called()
	return args.String(0)
}

// WebSocketConnInterface defines the interface that websocket.Conn implements
type WebSocketConnInterface interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(messageType int, data []byte) error
	Close() error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	SetPongHandler(h func(string) error)
	RemoteAddr() interface{}
	Subprotocol() string
}

// TestSignalingServer_HandleRequestOfferMessage 测试request-offer消息处理
func TestSignalingServer_HandleRequestOfferMessage(t *testing.T) {
	// 创建测试服务器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	server := NewSignalingServer(ctx, &SignalingEncoderConfig{Codec: "h264"}, mockMediaStream, []webrtc.ICEServer{})
	require.NotNil(t, server)

	// 创建测试客户端
	client := &SignalingClient{
		ID:       "test-client-1",
		AppName:  "test-app",
		Conn:     nil, // We don't need the websocket connection for message handling tests
		Send:     make(chan []byte, 256),
		Server:   server,
		LastSeen: time.Now(),
		State:    ClientStateConnected,
	}

	// 测试用例
	tests := []struct {
		name           string
		message        *protocol.StandardMessage
		expectError    bool
		expectedOffers int
	}{
		{
			name: "Valid request-offer message",
			message: &protocol.StandardMessage{
				Type:      protocol.MessageTypeRequestOffer,
				ID:        "test-msg-1",
				Timestamp: time.Now().Unix(),
				PeerID:    "test-client-1",
				Data: map[string]any{
					"constraints": map[string]any{
						"video": true,
						"audio": true,
					},
				},
			},
			expectError:    false,
			expectedOffers: 1,
		},
		{
			name: "Request-offer with codec preferences",
			message: &protocol.StandardMessage{
				Type:      protocol.MessageTypeRequestOffer,
				ID:        "test-msg-2",
				Timestamp: time.Now().Unix(),
				PeerID:    "test-client-1",
				Data: map[string]any{
					"constraints": map[string]any{
						"video": true,
						"audio": false,
					},
					"codec_preferences": []string{"H264", "VP8"},
				},
			},
			expectError:    false,
			expectedOffers: 1,
		},
		{
			name: "Request-offer with minimal data",
			message: &protocol.StandardMessage{
				Type:      protocol.MessageTypeRequestOffer,
				ID:        "test-msg-3",
				Timestamp: time.Now().Unix(),
				PeerID:    "test-client-1",
				Data:      nil,
			},
			expectError:    false,
			expectedOffers: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 清空发送通道
			for len(client.Send) > 0 {
				<-client.Send
			}

			// 调用处理函数
			client.handleRequestOfferMessage(tt.message)

			// 验证结果
			if tt.expectError {
				// 检查是否发送了错误消息
				select {
				case msg := <-client.Send:
					var response SignalingMessage
					err := json.Unmarshal(msg, &response)
					require.NoError(t, err)
					assert.Equal(t, "error", response.Type)
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Expected error message but none received")
				}
			} else {
				// 检查是否发送了offer消息
				offersReceived := 0
				timeout := time.After(1 * time.Second)

				for offersReceived < tt.expectedOffers {
					select {
					case msg := <-client.Send:
						var response SignalingMessage
						err := json.Unmarshal(msg, &response)
						require.NoError(t, err)

						if response.Type == "offer" {
							offersReceived++

							// 验证offer数据结构
							assert.NotNil(t, response.Data)
							offerData, ok := response.Data.(map[string]any)
							require.True(t, ok)

							assert.Contains(t, offerData, "type")
							assert.Contains(t, offerData, "sdp")
							assert.Equal(t, "offer", offerData["type"])
							assert.NotEmpty(t, offerData["sdp"])
						}
					case <-timeout:
						t.Fatalf("Expected %d offers but only received %d", tt.expectedOffers, offersReceived)
					}
				}

				assert.Equal(t, tt.expectedOffers, offersReceived)
			}
		})
	}
}

// TestSignalingServer_HandlePingMessage 测试ping消息处理
func TestSignalingServer_HandlePingMessage(t *testing.T) {
	// 创建测试服务器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockMediaStream := &MockMediaStream{}
	server := NewSignalingServer(ctx, &SignalingEncoderConfig{Codec: "h264"}, mockMediaStream, []webrtc.ICEServer{})
	require.NotNil(t, server)

	// 创建测试客户端
	client := &SignalingClient{
		ID:       "test-client-2",
		AppName:  "test-app",
		Conn:     nil, // We don't need the websocket connection for message handling tests
		Send:     make(chan []byte, 256),
		Server:   server,
		LastSeen: time.Now(),
		State:    ClientStateConnected,
	}

	// 测试用例
	tests := []struct {
		name         string
		message      *protocol.StandardMessage
		expectedPong bool
	}{
		{
			name: "Simple ping message",
			message: &protocol.StandardMessage{
				Type:      protocol.MessageTypePing,
				ID:        "ping-msg-1",
				Timestamp: time.Now().Unix(),
				PeerID:    "test-client-2",
				Data:      nil,
			},
			expectedPong: true,
		},
		{
			name: "Ping with timestamp",
			message: &protocol.StandardMessage{
				Type:      protocol.MessageTypePing,
				ID:        "ping-msg-2",
				Timestamp: time.Now().Unix(),
				PeerID:    "test-client-2",
				Data: map[string]any{
					"timestamp": time.Now().Unix(),
				},
			},
			expectedPong: true,
		},
		{
			name: "Ping with client state",
			message: &protocol.StandardMessage{
				Type:      protocol.MessageTypePing,
				ID:        "ping-msg-3",
				Timestamp: time.Now().Unix(),
				PeerID:    "test-client-2",
				Data: map[string]any{
					"timestamp":    time.Now().Unix(),
					"client_state": "active",
				},
			},
			expectedPong: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 清空发送通道
			for len(client.Send) > 0 {
				<-client.Send
			}

			// 记录处理前的时间
			beforeTime := time.Now().Unix()

			// 调用处理函数
			client.handlePingMessage(tt.message)

			// 验证结果
			if tt.expectedPong {
				select {
				case msg := <-client.Send:
					var response SignalingMessage
					err := json.Unmarshal(msg, &response)
					require.NoError(t, err)

					// 验证pong消息结构
					assert.Equal(t, "pong", response.Type)
					assert.Equal(t, client.ID, response.PeerID)
					assert.NotEmpty(t, response.MessageID)
					assert.GreaterOrEqual(t, response.Timestamp, beforeTime)

					// 验证pong数据
					assert.NotNil(t, response.Data)
					pongData, ok := response.Data.(map[string]any)
					require.True(t, ok)

					assert.Contains(t, pongData, "timestamp")
					assert.Contains(t, pongData, "server_time")
					assert.Contains(t, pongData, "client_id")
					assert.Equal(t, client.ID, pongData["client_id"])

					// 如果原始ping包含时间戳，检查是否在pong中返回
					if tt.message.Data != nil {
						if pingData, ok := tt.message.Data.(map[string]any); ok {
							if _, hasTimestamp := pingData["timestamp"]; hasTimestamp {
								assert.Contains(t, pongData, "ping_timestamp")
							}
							if _, hasClientState := pingData["client_state"]; hasClientState {
								assert.Contains(t, pongData, "client_state")
							}
						}
					}

				case <-time.After(1 * time.Second):
					t.Fatal("Expected pong message but none received")
				}
			}
		})
	}
}

// TestMessageRouter_RouteRequestOfferMessage 测试消息路由器对request-offer消息的处理
func TestMessageRouter_RouteRequestOfferMessage(t *testing.T) {
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
	}{
		{
			name: "Valid JSON request-offer message",
			messageData: []byte(`{
				"type": "request-offer",
				"id": "test-msg-1",
				"timestamp": 1640995200,
				"peer_id": "test-client",
				"data": {
					"constraints": {
						"video": true,
						"audio": true
					}
				}
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypeRequestOffer,
		},
		{
			name: "Request-offer with codec preferences",
			messageData: []byte(`{
				"type": "request-offer",
				"id": "test-msg-2",
				"timestamp": 1640995200,
				"data": {
					"constraints": {
						"video": true,
						"audio": false
					},
					"codec_preferences": ["H264", "VP8"]
				}
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypeRequestOffer,
		},
		{
			name: "Minimal request-offer message",
			messageData: []byte(`{
				"type": "request-offer",
				"id": "test-msg-3"
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypeRequestOffer,
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

				// 验证消息类型
				assert.Equal(t, tt.expectedType, result.Message.Type)

				// 验证客户端ID设置
				assert.Equal(t, tt.clientID, result.Message.PeerID)

				// 验证时间戳设置
				assert.Greater(t, result.Message.Timestamp, int64(0))

				// 验证消息ID设置
				assert.NotEmpty(t, result.Message.ID)
			}
		})
	}
}

// TestMessageRouter_RoutePingMessage 测试消息路由器对ping消息的处理
func TestMessageRouter_RoutePingMessage(t *testing.T) {
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
	}{
		{
			name: "Simple ping message",
			messageData: []byte(`{
				"type": "ping",
				"id": "ping-msg-1",
				"timestamp": 1640995200
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypePing,
		},
		{
			name: "Ping with data",
			messageData: []byte(`{
				"type": "ping",
				"id": "ping-msg-2",
				"timestamp": 1640995200,
				"data": {
					"timestamp": 1640995200,
					"client_state": "active"
				}
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypePing,
		},
		{
			name: "Minimal ping message",
			messageData: []byte(`{
				"type": "ping"
			}`),
			clientID:     "test-client",
			expectError:  false,
			expectedType: protocol.MessageTypePing,
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

				// 验证消息类型
				assert.Equal(t, tt.expectedType, result.Message.Type)

				// 验证客户端ID设置
				assert.Equal(t, tt.clientID, result.Message.PeerID)

				// 验证时间戳设置
				assert.Greater(t, result.Message.Timestamp, int64(0))

				// 验证消息ID设置
				assert.NotEmpty(t, result.Message.ID)
			}
		})
	}
}

// mockAddr 模拟网络地址
type mockAddr struct {
	addr string
}

func (m *mockAddr) Network() string {
	return "tcp"
}

func (m *mockAddr) String() string {
	return m.addr
}

// TestSignalingClient_SendStandardMessage 测试标准消息发送
func TestSignalingClient_SendStandardMessage(t *testing.T) {
	// 创建测试服务器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockMediaStream := &MockMediaStream{}
	server := NewSignalingServer(ctx, &SignalingEncoderConfig{Codec: "h264"}, mockMediaStream, []webrtc.ICEServer{})
	require.NotNil(t, server)

	// 创建测试客户端
	client := &SignalingClient{
		ID:       "test-client-3",
		AppName:  "test-app",
		Conn:     nil, // We don't need the websocket connection for message handling tests
		Send:     make(chan []byte, 256),
		Server:   server,
		LastSeen: time.Now(),
		State:    ClientStateConnected,
		Protocol: protocol.ProtocolVersionGStreamer10,
	}

	// 测试消息
	testMessage := &protocol.StandardMessage{
		Type:      protocol.MessageTypePong,
		ID:        "test-pong-1",
		Timestamp: time.Now().Unix(),
		PeerID:    client.ID,
		Data: map[string]any{
			"timestamp":   time.Now().Unix(),
			"server_time": time.Now().Unix(),
			"client_id":   client.ID,
		},
	}

	// 发送消息
	err := client.sendStandardMessage(testMessage)
	assert.NoError(t, err)

	// 验证消息被发送
	select {
	case msg := <-client.Send:
		assert.NotEmpty(t, msg)

		// 验证消息可以被解析
		var parsedMessage map[string]any
		err := json.Unmarshal(msg, &parsedMessage)
		assert.NoError(t, err)
		assert.Contains(t, parsedMessage, "type")

	case <-time.After(1 * time.Second):
		t.Fatal("Expected message to be sent but none received")
	}
}

// TestValidateSignalingMessage_IceAckRejection 测试ice-ack消息被正确拒绝
func TestValidateSignalingMessage_IceAckRejection(t *testing.T) {
	// 测试用例
	tests := []struct {
		name            string
		message         *SignalingMessage
		expectError     bool
		expectedCode    string
		expectedDetails string
	}{
		{
			name: "ice-ack message should be rejected",
			message: &SignalingMessage{
				Type:   "ice-ack",
				PeerID: "test-client",
				Data: map[string]any{
					"acknowledged": true,
				},
			},
			expectError:     true,
			expectedCode:    ErrorCodeInvalidMessageType,
			expectedDetails: "Supported types: ping, pong, request-offer, offer, answer, ice-candidate, mouse-click, mouse-move, key-press, get-stats",
		},
		{
			name: "ice-candidate message should be accepted",
			message: &SignalingMessage{
				Type:   "ice-candidate",
				PeerID: "test-client",
				Data: map[string]any{
					"candidate":     "candidate:1 1 UDP 2130706431 192.168.1.100 54400 typ host",
					"sdpMid":        "0",
					"sdpMLineIndex": 0,
				},
			},
			expectError: false,
		},
		{
			name: "ping message should be accepted",
			message: &SignalingMessage{
				Type:   "ping",
				PeerID: "test-client",
			},
			expectError: false,
		},
		{
			name: "unknown message type should be rejected",
			message: &SignalingMessage{
				Type:   "unknown-type",
				PeerID: "test-client",
			},
			expectError:     true,
			expectedCode:    ErrorCodeInvalidMessageType,
			expectedDetails: "Supported types: ping, pong, request-offer, offer, answer, ice-candidate, mouse-click, mouse-move, key-press, get-stats",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSignalingMessage(tt.message)

			if tt.expectError {
				require.NotNil(t, err, "Expected validation error but got none")
				assert.Equal(t, tt.expectedCode, err.Code)
				assert.Contains(t, err.Message, tt.message.Type)
				if tt.expectedDetails != "" {
					assert.Equal(t, tt.expectedDetails, err.Details)
					// Verify ice-ack is not in the supported types list
					assert.NotContains(t, err.Details, "ice-ack")
				}
			} else {
				assert.Nil(t, err, "Expected no validation error but got: %v", err)
			}
		})
	}
}

// TestSignalingClient_HandleStandardMessage 测试标准消息处理分发
func TestSignalingClient_HandleStandardMessage(t *testing.T) {
	// 创建测试服务器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	server := NewSignalingServer(ctx, &SignalingEncoderConfig{Codec: "h264"}, mockMediaStream, []webrtc.ICEServer{})
	require.NotNil(t, server)

	// 创建测试客户端
	client := &SignalingClient{
		ID:       "test-client-4",
		AppName:  "test-app",
		Conn:     nil, // We don't need the websocket connection for message handling tests
		Send:     make(chan []byte, 256),
		Server:   server,
		LastSeen: time.Now(),
		State:    ClientStateConnected,
	}

	// 测试用例
	tests := []struct {
		name            string
		message         *protocol.StandardMessage
		expectedHandler string
	}{
		{
			name: "Handle ping message",
			message: &protocol.StandardMessage{
				Type:      protocol.MessageTypePing,
				ID:        "ping-test-1",
				Timestamp: time.Now().Unix(),
				PeerID:    client.ID,
			},
			expectedHandler: "ping",
		},
		{
			name: "Handle request-offer message",
			message: &protocol.StandardMessage{
				Type:      protocol.MessageTypeRequestOffer,
				ID:        "offer-test-1",
				Timestamp: time.Now().Unix(),
				PeerID:    client.ID,
			},
			expectedHandler: "request-offer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 清空发送通道
			for len(client.Send) > 0 {
				<-client.Send
			}

			// 调用处理函数
			client.handleStandardMessage(tt.message, protocol.ProtocolVersionGStreamer10)

			// 验证处理结果
			select {
			case msg := <-client.Send:
				var response SignalingMessage
				err := json.Unmarshal(msg, &response)
				require.NoError(t, err)

				// 根据处理器类型验证响应
				switch tt.expectedHandler {
				case "ping":
					assert.Equal(t, "pong", response.Type)
				case "request-offer":
					assert.Equal(t, "offer", response.Type)
				}

			case <-time.After(2 * time.Second):
				t.Fatalf("Expected response for %s handler but none received", tt.expectedHandler)
			}
		})
	}
}

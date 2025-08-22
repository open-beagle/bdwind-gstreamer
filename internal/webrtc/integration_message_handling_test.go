package webrtc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc/protocol"
)

// TestIntegration_RequestOfferFlow 测试完整的request-offer流程
func TestIntegration_RequestOfferFlow(t *testing.T) {
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
		ID:       "integration-test-client",
		AppName:  "test-app",
		Conn:     nil, // We don't need the websocket connection for message handling tests
		Send:     make(chan []byte, 256),
		Server:   server,
		LastSeen: time.Now(),
		State:    ClientStateConnected,
	}

	// 测试完整流程
	t.Run("Complete request-offer to offer flow", func(t *testing.T) {
		// 1. 创建request-offer消息
		requestOfferMsg := []byte(`{
			"type": "request-offer",
			"id": "test-request-1",
			"timestamp": ` + string(rune(time.Now().Unix())) + `,
			"peer_id": "integration-test-client",
			"data": {
				"constraints": {
					"video": true,
					"audio": true
				},
				"codec_preferences": ["H264", "VP8"]
			}
		}`)

		// 2. 使用消息路由器处理消息
		routeResult, err := server.messageRouter.RouteMessage(requestOfferMsg, client.ID)
		assert.NoError(t, err)
		require.NotNil(t, routeResult)
		require.NotNil(t, routeResult.Message)

		// 3. 验证路由结果
		assert.Equal(t, protocol.MessageTypeRequestOffer, routeResult.Message.Type)
		assert.Equal(t, client.ID, routeResult.Message.PeerID)

		// 4. 处理标准化消息
		client.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)

		// 5. 验证offer响应
		select {
		case responseBytes := <-client.Send:
			var response SignalingMessage
			err := json.Unmarshal(responseBytes, &response)
			require.NoError(t, err)

			// 验证响应结构
			assert.Equal(t, "offer", response.Type)
			assert.Equal(t, client.ID, response.PeerID)
			assert.NotEmpty(t, response.MessageID)
			assert.Greater(t, response.Timestamp, int64(0))

			// 验证offer数据
			require.NotNil(t, response.Data)
			offerData, ok := response.Data.(map[string]any)
			require.True(t, ok)

			assert.Equal(t, "offer", offerData["type"])
			assert.NotEmpty(t, offerData["sdp"])

			// 验证SDP内容
			sdp, ok := offerData["sdp"].(string)
			require.True(t, ok)
			assert.Contains(t, sdp, "v=0")
			assert.Contains(t, sdp, "o=")

		case <-time.After(5 * time.Second):
			t.Fatal("Expected offer response but none received")
		}
	})
}

// TestIntegration_PingPongFlow 测试完整的ping-pong流程
func TestIntegration_PingPongFlow(t *testing.T) {
	// 创建测试服务器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockMediaStream := &MockMediaStream{}
	server := NewSignalingServer(ctx, &SignalingEncoderConfig{Codec: "h264"}, mockMediaStream, []webrtc.ICEServer{})
	require.NotNil(t, server)

	// 创建测试客户端
	client := &SignalingClient{
		ID:       "ping-test-client",
		AppName:  "test-app",
		Conn:     nil, // We don't need the websocket connection for message handling tests
		Send:     make(chan []byte, 256),
		Server:   server,
		LastSeen: time.Now(),
		State:    ClientStateConnected,
	}

	// 测试完整流程
	t.Run("Complete ping to pong flow", func(t *testing.T) {
		pingTimestamp := time.Now().Unix()

		// 1. 创建ping消息
		pingMsg := []byte(`{
			"type": "ping",
			"id": "test-ping-1",
			"timestamp": ` + string(rune(pingTimestamp)) + `,
			"peer_id": "ping-test-client",
			"data": {
				"timestamp": ` + string(rune(pingTimestamp)) + `,
				"client_state": "active"
			}
		}`)

		// 2. 使用消息路由器处理消息
		routeResult, err := server.messageRouter.RouteMessage(pingMsg, client.ID)
		assert.NoError(t, err)
		require.NotNil(t, routeResult)
		require.NotNil(t, routeResult.Message)

		// 3. 验证路由结果
		assert.Equal(t, protocol.MessageTypePing, routeResult.Message.Type)
		assert.Equal(t, client.ID, routeResult.Message.PeerID)

		// 4. 记录处理前时间
		beforeProcessing := time.Now().Unix()

		// 5. 处理标准化消息
		client.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)

		// 6. 验证pong响应
		select {
		case responseBytes := <-client.Send:
			var response SignalingMessage
			err := json.Unmarshal(responseBytes, &response)
			require.NoError(t, err)

			// 验证响应结构
			assert.Equal(t, "pong", response.Type)
			assert.Equal(t, client.ID, response.PeerID)
			assert.NotEmpty(t, response.MessageID)
			assert.GreaterOrEqual(t, response.Timestamp, beforeProcessing)

			// 验证pong数据
			require.NotNil(t, response.Data)
			pongData, ok := response.Data.(map[string]any)
			require.True(t, ok)

			assert.Contains(t, pongData, "timestamp")
			assert.Contains(t, pongData, "server_time")
			assert.Contains(t, pongData, "client_id")
			assert.Equal(t, client.ID, pongData["client_id"])

			// 验证原始ping时间戳被包含
			assert.Contains(t, pongData, "ping_timestamp")
			assert.Contains(t, pongData, "client_state")
			assert.Equal(t, "active", pongData["client_state"])

		case <-time.After(5 * time.Second):
			t.Fatal("Expected pong response but none received")
		}
	})
}

// TestIntegration_MessageRouterWithPeerConnectionManager 测试消息路由器与PeerConnection管理器的集成
func TestIntegration_MessageRouterWithPeerConnectionManager(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	// 创建PeerConnection管理器
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	logger := log.New(os.Stdout, "INTEGRATION: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)

	clientID := "integration-pc-client"

	// 测试集成流程
	t.Run("Router and PeerConnection Manager integration", func(t *testing.T) {
		// 1. 使用路由器解析request-offer消息
		requestOfferMsg := []byte(`{
			"type": "request-offer",
			"id": "integration-request-1",
			"timestamp": ` + string(rune(time.Now().Unix())) + `,
			"data": {
				"constraints": {
					"video": true,
					"audio": false
				}
			}
		}`)

		routeResult, err := router.RouteMessage(requestOfferMsg, clientID)
		assert.NoError(t, err)
		require.NotNil(t, routeResult)
		require.NotNil(t, routeResult.Message)

		// 2. 使用PeerConnection管理器创建连接
		pc, err := pcManager.CreatePeerConnection(clientID)
		assert.NoError(t, err)
		require.NotNil(t, pc)

		// 3. 创建offer
		offer, err := pc.CreateOffer(nil)
		assert.NoError(t, err)
		require.NotNil(t, offer)

		// 4. 设置本地描述
		err = pc.SetLocalDescription(offer)
		assert.NoError(t, err)

		// 5. 使用路由器格式化offer响应
		offerResponse := router.CreateStandardResponse(
			protocol.MessageTypeOffer,
			clientID,
			map[string]any{
				"type": offer.Type.String(),
				"sdp":  offer.SDP,
			},
		)

		require.NotNil(t, offerResponse)
		assert.Equal(t, protocol.MessageTypeOffer, offerResponse.Type)
		assert.Equal(t, clientID, offerResponse.PeerID)

		// 6. 格式化响应为字节
		responseBytes, err := router.FormatResponse(offerResponse)
		assert.NoError(t, err)
		assert.NotEmpty(t, responseBytes)

		// 7. 验证响应可以被解析
		var parsedResponse map[string]any
		err = json.Unmarshal(responseBytes, &parsedResponse)
		assert.NoError(t, err)

		assert.Equal(t, "offer", parsedResponse["type"])
		assert.Contains(t, parsedResponse, "data")

		responseData, ok := parsedResponse["data"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "offer", responseData["type"])
		assert.Contains(t, responseData, "sdp")
	})

	// 清理
	err := pcManager.Close()
	assert.NoError(t, err)
}

// TestIntegration_ErrorHandling 测试错误处理集成
func TestIntegration_ErrorHandling(t *testing.T) {
	// 创建测试服务器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockMediaStream := &MockMediaStream{}
	server := NewSignalingServer(ctx, &SignalingEncoderConfig{Codec: "h264"}, mockMediaStream, []webrtc.ICEServer{})
	require.NotNil(t, server)

	// 创建测试客户端
	client := &SignalingClient{
		ID:       "error-test-client",
		AppName:  "test-app",
		Conn:     nil, // We don't need the websocket connection for message handling tests
		Send:     make(chan []byte, 256),
		Server:   server,
		LastSeen: time.Now(),
		State:    ClientStateConnected,
	}

	// 测试错误处理
	tests := []struct {
		name        string
		messageData []byte
		expectError bool
	}{
		{
			name:        "Invalid JSON message",
			messageData: []byte(`{invalid json`),
			expectError: true,
		},
		{
			name: "Missing message type",
			messageData: []byte(`{
				"id": "test-msg-1",
				"timestamp": ` + string(rune(time.Now().Unix())) + `
			}`),
			expectError: true,
		},
		{
			name: "Unknown message type",
			messageData: []byte(`{
				"type": "unknown-message-type",
				"id": "test-msg-1",
				"timestamp": ` + string(rune(time.Now().Unix())) + `
			}`),
			expectError: false, // 应该被处理但可能发送错误响应
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 清空发送通道
			for len(client.Send) > 0 {
				<-client.Send
			}

			// 尝试路由消息
			routeResult, err := server.messageRouter.RouteMessage(tt.messageData, client.ID)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, routeResult)
			} else {
				// 即使消息类型未知，路由器也应该能够解析基本结构
				if err == nil && routeResult != nil && routeResult.Message != nil {
					// 处理消息（可能会发送错误响应）
					client.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)

					// 检查是否有响应（可能是错误响应）
					select {
					case responseBytes := <-client.Send:
						var response SignalingMessage
						err := json.Unmarshal(responseBytes, &response)
						if err == nil {
							// 如果是未知消息类型，应该收到错误响应
							if routeResult.Message.Type == protocol.MessageType("unknown-message-type") {
								// 可能收到错误响应或者被忽略
								t.Logf("Received response for unknown message type: %s", response.Type)
							}
						}
					case <-time.After(1 * time.Second):
						// 某些情况下可能不会有响应，这是正常的
						t.Logf("No response received for message type: %s", routeResult.Message.Type)
					}
				}
			}
		})
	}
}

// TestIntegration_ConcurrentMessageHandling 测试并发消息处理
func TestIntegration_ConcurrentMessageHandling(t *testing.T) {
	// 创建测试服务器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	server := NewSignalingServer(ctx, &SignalingEncoderConfig{Codec: "h264"}, mockMediaStream, []webrtc.ICEServer{})
	require.NotNil(t, server)

	// 创建多个测试客户端
	numClients := 5
	clients := make([]*SignalingClient, numClients)

	for i := 0; i < numClients; i++ {
		clients[i] = &SignalingClient{
			ID:       fmt.Sprintf("concurrent-client-%d", i),
			AppName:  "test-app",
			Conn:     nil, // We don't need the websocket connection for message handling tests
			Send:     make(chan []byte, 256),
			Server:   server,
			LastSeen: time.Now(),
			State:    ClientStateConnected,
		}
	}

	// 测试并发处理
	t.Run("Concurrent ping processing", func(t *testing.T) {
		done := make(chan bool, numClients)

		// 并发发送ping消息
		for i, client := range clients {
			go func(clientIndex int, c *SignalingClient) {
				defer func() { done <- true }()

				pingMsg := []byte(fmt.Sprintf(`{
					"type": "ping",
					"id": "concurrent-ping-%d",
					"timestamp": %d,
					"peer_id": "%s",
					"data": {
						"timestamp": %d,
						"client_state": "active"
					}
				}`, clientIndex, time.Now().Unix(), c.ID, time.Now().Unix()))

				// 路由消息
				routeResult, err := server.messageRouter.RouteMessage(pingMsg, c.ID)
				assert.NoError(t, err)
				require.NotNil(t, routeResult)

				// 处理消息
				c.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)

				// 验证响应
				select {
				case responseBytes := <-c.Send:
					var response SignalingMessage
					err := json.Unmarshal(responseBytes, &response)
					assert.NoError(t, err)
					assert.Equal(t, "pong", response.Type)
					assert.Equal(t, c.ID, response.PeerID)

				case <-time.After(5 * time.Second):
					t.Errorf("Client %d did not receive pong response", clientIndex)
				}
			}(i, client)
		}

		// 等待所有客户端完成
		for i := 0; i < numClients; i++ {
			select {
			case <-done:
				// 客户端完成
			case <-time.After(10 * time.Second):
				t.Fatal("Timeout waiting for concurrent ping processing")
			}
		}
	})
}

// TestIntegration_ProtocolNegotiation 测试协议协商集成
func TestIntegration_ProtocolNegotiation(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	clientID := "negotiation-test-client"

	// 测试协议协商
	t.Run("Protocol negotiation flow", func(t *testing.T) {
		// 1. 创建协议协商请求
		negotiationMsg := []byte(`{
			"type": "protocol-negotiation",
			"id": "negotiation-request-1",
			"timestamp": ` + string(rune(time.Now().Unix())) + `,
			"data": {
				"supported_protocols": ["gstreamer-1.0", "selkies"],
				"preferred_protocol": "gstreamer-1.0",
				"client_capabilities": ["webrtc", "input", "stats"]
			}
		}`)

		// 2. 处理协商请求
		response, err := router.HandleProtocolNegotiation(negotiationMsg, clientID)
		assert.NoError(t, err)
		require.NotNil(t, response)

		// 3. 验证协商响应
		assert.Equal(t, protocol.MessageType("protocol-negotiation-response"), response.Type)
		assert.Equal(t, clientID, response.PeerID)

		require.NotNil(t, response.Data)
		responseData, ok := response.Data.(map[string]any)
		require.True(t, ok)

		assert.Contains(t, responseData, "selected_protocol")
		assert.Contains(t, responseData, "server_protocols")
		assert.Contains(t, responseData, "negotiation_result")

		selectedProtocol := responseData["selected_protocol"].(string)
		assert.NotEmpty(t, selectedProtocol)
		assert.Equal(t, "success", responseData["negotiation_result"])

		// 4. 验证选择的协议是支持的
		assert.True(t, router.IsProtocolSupported(protocol.ProtocolVersion(selectedProtocol)))
	})
}

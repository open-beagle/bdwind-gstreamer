package webrtc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc/protocol"
)

// TestComprehensiveIntegration_RequestOfferEndToEnd 端到端request-offer消息测试
func TestComprehensiveIntegration_RequestOfferEndToEnd(t *testing.T) {
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
		ID:               "e2e-request-offer-client",
		AppName:          "integration-test-app",
		Conn:             nil,
		Send:             make(chan []byte, 256),
		Server:           server,
		LastSeen:         time.Now(),
		ConnectedAt:      time.Now(),
		State:            ClientStateConnected,
		MessageCount:     0,
		ErrorCount:       0,
		ProtocolDetected: true,
		Protocol:         protocol.ProtocolVersionGStreamer10,
	}

	// 测试用例
	testCases := []struct {
		name        string
		messageData []byte
		expectOffer bool
	}{
		{
			name: "Valid request-offer with video and audio",
			messageData: []byte(`{
				"type": "request-offer",
				"id": "e2e-request-1",
				"timestamp": ` + fmt.Sprintf("%d", time.Now().Unix()) + `,
				"peer_id": "e2e-request-offer-client",
				"data": {
					"constraints": {
						"video": true,
						"audio": true
					},
					"codec_preferences": ["H264", "VP8"]
				}
			}`),
			expectOffer: true,
		},
		{
			name: "Request-offer with minimal data",
			messageData: []byte(`{
				"type": "request-offer",
				"id": "e2e-request-2",
				"timestamp": ` + fmt.Sprintf("%d", time.Now().Unix()) + `,
				"peer_id": "e2e-request-offer-client"
			}`),
			expectOffer: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 清空发送通道
			for len(client.Send) > 0 {
				<-client.Send
			}

			// 使用消息路由器处理消息
			routeResult, err := server.messageRouter.RouteMessage(tc.messageData, client.ID)
			require.NoError(t, err)
			require.NotNil(t, routeResult)
			require.NotNil(t, routeResult.Message)

			// 验证消息类型
			assert.Equal(t, protocol.MessageTypeRequestOffer, routeResult.Message.Type)

			// 处理标准化消息
			client.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)

			if tc.expectOffer {
				// 验证offer响应
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

				case <-time.After(5 * time.Second):
					t.Fatal("Expected offer response but none received")
				}
			}
		})
	}
}

// TestComprehensiveIntegration_PingEndToEnd 端到端ping消息测试
func TestComprehensiveIntegration_PingEndToEnd(t *testing.T) {
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
		ID:               "e2e-ping-client",
		AppName:          "integration-test-app",
		Conn:             nil,
		Send:             make(chan []byte, 256),
		Server:           server,
		LastSeen:         time.Now(),
		ConnectedAt:      time.Now(),
		State:            ClientStateConnected,
		MessageCount:     0,
		ErrorCount:       0,
		ProtocolDetected: true,
		Protocol:         protocol.ProtocolVersionGStreamer10,
	}

	// 测试用例
	testCases := []struct {
		name        string
		messageData []byte
		expectPong  bool
	}{
		{
			name: "Simple ping message",
			messageData: []byte(`{
				"type": "ping",
				"id": "e2e-ping-1",
				"timestamp": ` + fmt.Sprintf("%d", time.Now().Unix()) + `,
				"peer_id": "e2e-ping-client"
			}`),
			expectPong: true,
		},
		{
			name: "Ping with timestamp and client state",
			messageData: []byte(`{
				"type": "ping",
				"id": "e2e-ping-2",
				"timestamp": ` + fmt.Sprintf("%d", time.Now().Unix()) + `,
				"peer_id": "e2e-ping-client",
				"data": {
					"timestamp": ` + fmt.Sprintf("%d", time.Now().Unix()) + `,
					"client_state": "active"
				}
			}`),
			expectPong: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 清空发送通道
			for len(client.Send) > 0 {
				<-client.Send
			}

			// 记录处理前时间
			beforeProcessing := time.Now().Unix()

			// 使用消息路由器处理消息
			routeResult, err := server.messageRouter.RouteMessage(tc.messageData, client.ID)
			require.NoError(t, err)
			require.NotNil(t, routeResult)
			require.NotNil(t, routeResult.Message)

			// 验证消息类型
			assert.Equal(t, protocol.MessageTypePing, routeResult.Message.Type)

			// 处理标准化消息
			client.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)

			if tc.expectPong {
				// 验证pong响应
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

					// 验证必需字段
					assert.Contains(t, pongData, "timestamp")
					assert.Contains(t, pongData, "server_time")
					assert.Contains(t, pongData, "client_id")
					assert.Equal(t, client.ID, pongData["client_id"])

				case <-time.After(5 * time.Second):
					t.Fatal("Expected pong response but none received")
				}
			}
		})
	}
}

// TestComprehensiveIntegration_ConcurrentSafety 并发安全性测试
func TestComprehensiveIntegration_ConcurrentSafety(t *testing.T) {
	// 创建测试服务器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	server := NewSignalingServer(ctx, &SignalingEncoderConfig{Codec: "h264"}, mockMediaStream, []webrtc.ICEServer{})
	require.NotNil(t, server)

	// 创建多个客户端
	numClients := 3
	numMessagesPerClient := 2

	clients := make([]*SignalingClient, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = &SignalingClient{
			ID:               fmt.Sprintf("concurrent-client-%d", i),
			AppName:          "integration-test-app",
			Conn:             nil,
			Send:             make(chan []byte, 256),
			Server:           server,
			LastSeen:         time.Now(),
			ConnectedAt:      time.Now(),
			State:            ClientStateConnected,
			MessageCount:     0,
			ErrorCount:       0,
			ProtocolDetected: true,
			Protocol:         protocol.ProtocolVersionGStreamer10,
		}
	}

	// 测试并发ping处理
	t.Run("Concurrent ping processing", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan bool, numClients*numMessagesPerClient)

		// 并发发送ping消息
		for i, client := range clients {
			wg.Add(1)
			go func(clientIndex int, c *SignalingClient) {
				defer wg.Done()

				for j := 0; j < numMessagesPerClient; j++ {
					// 清空发送通道
					for len(c.Send) > 0 {
						<-c.Send
					}

					pingMsg := []byte(fmt.Sprintf(`{
						"type": "ping",
						"id": "concurrent-ping-%d-%d",
						"timestamp": %d,
						"peer_id": "%s",
						"data": {
							"timestamp": %d,
							"client_state": "active",
							"message_index": %d
						}
					}`, clientIndex, j, time.Now().Unix(), c.ID, time.Now().Unix(), j))

					// 路由消息
					routeResult, err := server.messageRouter.RouteMessage(pingMsg, c.ID)
					if err != nil {
						results <- false
						continue
					}

					// 处理消息
					c.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)

					// 验证响应
					select {
					case responseBytes := <-c.Send:
						var response SignalingMessage
						err := json.Unmarshal(responseBytes, &response)
						if err != nil {
							results <- false
							continue
						}

						if response.Type == "pong" && response.PeerID == c.ID {
							results <- true
						} else {
							results <- false
						}

					case <-time.After(3 * time.Second):
						results <- false
					}
				}
			}(i, client)
		}

		// 等待所有goroutine完成
		wg.Wait()
		close(results)

		// 统计结果
		successCount := 0
		totalCount := 0
		for result := range results {
			totalCount++
			if result {
				successCount++
			}
		}

		// 验证成功率
		expectedTotal := numClients * numMessagesPerClient
		assert.Equal(t, expectedTotal, totalCount)
		assert.GreaterOrEqual(t, successCount, int(float64(expectedTotal)*0.80)) // 至少80%成功率
	})
}

// TestComprehensiveIntegration_ErrorHandling 错误处理测试
func TestComprehensiveIntegration_ErrorHandling(t *testing.T) {
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
		ID:               "error-scenario-client",
		AppName:          "integration-test-app",
		Conn:             nil,
		Send:             make(chan []byte, 256),
		Server:           server,
		LastSeen:         time.Now(),
		ConnectedAt:      time.Now(),
		State:            ClientStateConnected,
		MessageCount:     0,
		ErrorCount:       0,
		ProtocolDetected: true,
		Protocol:         protocol.ProtocolVersionGStreamer10,
	}

	// 错误场景测试用例
	errorScenarios := []struct {
		name        string
		messageData []byte
		expectError bool
	}{
		{
			name:        "Invalid JSON format",
			messageData: []byte(`{invalid json format`),
			expectError: true,
		},
		{
			name: "Missing message type",
			messageData: []byte(`{
				"id": "error-test-1",
				"timestamp": ` + fmt.Sprintf("%d", time.Now().Unix()) + `,
				"peer_id": "error-scenario-client"
			}`),
			expectError: true,
		},
		{
			name: "Unknown message type",
			messageData: []byte(`{
				"type": "unknown-message-type",
				"id": "error-test-2",
				"timestamp": ` + fmt.Sprintf("%d", time.Now().Unix()) + `,
				"peer_id": "error-scenario-client"
			}`),
			expectError: false, // 应该被处理但可能发送错误响应
		},
		{
			name:        "Empty message",
			messageData: []byte(`{}`),
			expectError: true,
		},
	}

	for _, scenario := range errorScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// 清空发送通道
			for len(client.Send) > 0 {
				<-client.Send
			}

			// 尝试路由消息
			routeResult, err := server.messageRouter.RouteMessage(scenario.messageData, client.ID)

			if scenario.expectError {
				// 预期错误的情况
				assert.Error(t, err)
				assert.Nil(t, routeResult)
			} else {
				// 消息应该被成功路由
				if err == nil && routeResult != nil && routeResult.Message != nil {
					// 处理消息
					client.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)

					// 检查是否有响应（可能是错误响应）
					select {
					case responseBytes := <-client.Send:
						var response SignalingMessage
						err := json.Unmarshal(responseBytes, &response)
						if err == nil {
							t.Logf("Received response for unknown message type: %s", response.Type)
						}

					case <-time.After(1 * time.Second):
						// 某些情况下可能不会有响应
						t.Logf("No response received for scenario: %s", scenario.name)
					}
				}
			}
		})
	}
}

// TestComprehensiveIntegration_MessageSequencing 消息序列测试
func TestComprehensiveIntegration_MessageSequencing(t *testing.T) {
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
		ID:               "sequencing-client",
		AppName:          "integration-test-app",
		Conn:             nil,
		Send:             make(chan []byte, 256),
		Server:           server,
		LastSeen:         time.Now(),
		ConnectedAt:      time.Now(),
		State:            ClientStateConnected,
		MessageCount:     0,
		ErrorCount:       0,
		ProtocolDetected: true,
		Protocol:         protocol.ProtocolVersionGStreamer10,
	}

	// 测试消息序列：ping -> request-offer -> ping
	t.Run("Message sequence processing", func(t *testing.T) {
		// 1. 发送初始ping
		pingMsg1 := []byte(`{
			"type": "ping",
			"id": "seq-ping-1",
			"timestamp": ` + fmt.Sprintf("%d", time.Now().Unix()) + `,
			"peer_id": "sequencing-client",
			"data": {
				"sequence": 1,
				"client_state": "initializing"
			}
		}`)

		routeResult, err := server.messageRouter.RouteMessage(pingMsg1, client.ID)
		require.NoError(t, err)
		client.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)

		// 验证pong响应
		select {
		case responseBytes := <-client.Send:
			var response SignalingMessage
			err := json.Unmarshal(responseBytes, &response)
			require.NoError(t, err)
			assert.Equal(t, "pong", response.Type)
		case <-time.After(2 * time.Second):
			t.Fatal("Expected pong response for first ping")
		}

		// 2. 发送request-offer
		requestOfferMsg := []byte(`{
			"type": "request-offer",
			"id": "seq-request-offer-1",
			"timestamp": ` + fmt.Sprintf("%d", time.Now().Unix()) + `,
			"peer_id": "sequencing-client",
			"data": {
				"sequence": 2,
				"constraints": {
					"video": true,
					"audio": true
				}
			}
		}`)

		routeResult, err = server.messageRouter.RouteMessage(requestOfferMsg, client.ID)
		require.NoError(t, err)
		client.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)

		// 验证offer响应
		select {
		case responseBytes := <-client.Send:
			var response SignalingMessage
			err := json.Unmarshal(responseBytes, &response)
			require.NoError(t, err)
			assert.Equal(t, "offer", response.Type)
		case <-time.After(5 * time.Second):
			t.Fatal("Expected offer response for request-offer")
		}

		// 3. 发送最终ping
		pingMsg2 := []byte(`{
			"type": "ping",
			"id": "seq-ping-2",
			"timestamp": ` + fmt.Sprintf("%d", time.Now().Unix()) + `,
			"peer_id": "sequencing-client",
			"data": {
				"sequence": 3,
				"client_state": "connected"
			}
		}`)

		routeResult, err = server.messageRouter.RouteMessage(pingMsg2, client.ID)
		require.NoError(t, err)
		client.handleStandardMessage(routeResult.Message, routeResult.OriginalProtocol)

		// 验证最终pong响应
		select {
		case responseBytes := <-client.Send:
			var response SignalingMessage
			err := json.Unmarshal(responseBytes, &response)
			require.NoError(t, err)
			assert.Equal(t, "pong", response.Type)

			// 验证客户端状态被正确传递
			if pongData, ok := response.Data.(map[string]any); ok {
				assert.Equal(t, "connected", pongData["client_state"])
			}

		case <-time.After(2 * time.Second):
			t.Fatal("Expected pong response for final ping")
		}
	})
}

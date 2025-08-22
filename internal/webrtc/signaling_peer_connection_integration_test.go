package webrtc

import (
	"context"
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

// createTestSignalingServer 创建用于测试的信令服务器
func createTestSignalingServer(pcManager *PeerConnectionManager) *SignalingServer {
	ctx, _ := context.WithCancel(context.Background())

	// 创建协议管理器
	protocolManager := protocol.NewProtocolManager(&protocol.ManagerConfig{
		DefaultProtocol:    protocol.ProtocolVersionGStreamer10,
		AutoDetection:      true,
		StrictValidation:   false,
		EnableLogging:      true,
		SupportedProtocols: []protocol.ProtocolVersion{protocol.ProtocolVersionGStreamer10},
	})

	// 创建消息路由器
	messageRouter := NewMessageRouter(protocolManager, nil)

	return &SignalingServer{
		clients:               make(map[string]*SignalingClient),
		apps:                  make(map[string]map[string]*SignalingClient),
		broadcast:             make(chan []byte, 256),
		register:              make(chan *SignalingClient, 256),
		unregister:            make(chan *SignalingClient, 256),
		running:               true,
		peerConnectionManager: pcManager,
		messageRouter:         messageRouter,
		ctx:                   ctx,
	}
}

// TestSignalingServer_PeerConnectionLifecycleIntegration 测试信令服务器与PeerConnection生命周期的集成
func TestSignalingServer_PeerConnectionLifecycleIntegration(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "INTEGRATION_TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)
	defer pcManager.Close()

	// 设置连接超时
	pcManager.SetConnectionTimeout(30 * time.Second)

	// 创建信令服务器
	server := createTestSignalingServer(pcManager)

	// 创建模拟客户端
	client := &SignalingClient{
		ID:               "integration-test-client",
		Server:           server,
		LastSeen:         time.Now(),
		ConnectedAt:      time.Now(),
		State:            ClientStateConnected,
		MessageCount:     0,
		ErrorCount:       0,
		ProtocolDetected: true,
		Protocol:         protocol.ProtocolVersionGStreamer10,
	}

	// 测试request-offer消息处理
	requestOfferMessage := &protocol.StandardMessage{
		Type: protocol.MessageTypeRequestOffer,
		ID:   "test-request-offer-1",
		Data: map[string]any{
			"constraints": map[string]any{
				"video": true,
				"audio": true,
			},
			"codec_preferences": []string{"H264", "VP8"},
		},
	}

	// 处理request-offer消息
	client.handleRequestOfferMessage(requestOfferMessage)

	// 验证PeerConnection被创建
	pc, exists := pcManager.GetPeerConnection(client.ID)
	assert.True(t, exists)
	require.NotNil(t, pc)

	// 验证连接健康状态
	health, err := pcManager.GetConnectionHealth(client.ID)
	assert.NoError(t, err)
	assert.True(t, health.IsHealthy)
	assert.Equal(t, webrtc.PeerConnectionStateNew, health.State)

	// 模拟连接状态变化
	pcManager.handleConnectionStateChange(client.ID, webrtc.PeerConnectionStateConnecting)
	pcManager.handleConnectionStateChange(client.ID, webrtc.PeerConnectionStateConnected)

	// 验证状态更新
	health, err = pcManager.GetConnectionHealth(client.ID)
	assert.NoError(t, err)
	assert.True(t, health.IsHealthy)
	assert.Equal(t, webrtc.PeerConnectionStateConnected, health.State)

	// 验证指标
	metrics := pcManager.GetMetrics()
	assert.Equal(t, int64(1), metrics.TotalConnections)
	assert.Equal(t, int64(1), metrics.ActiveConnections)
}

// TestSignalingServer_MultipleClientLifecycle 测试多客户端生命周期管理
func TestSignalingServer_MultipleClientLifecycle(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "MULTI_INTEGRATION_TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)
	defer pcManager.Close()

	// 设置最大连接数
	pcManager.SetMaxConnections(3)

	// 创建信令服务器
	server := createTestSignalingServer(pcManager)

	// 创建多个客户端
	clients := make([]*SignalingClient, 3)
	for i := 0; i < 3; i++ {
		clients[i] = &SignalingClient{
			ID:               fmt.Sprintf("multi-client-%d", i+1),
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

	// 为每个客户端处理request-offer
	for _, client := range clients {
		requestOfferMessage := &protocol.StandardMessage{
			Type: protocol.MessageTypeRequestOffer,
			ID:   fmt.Sprintf("request-offer-%s", client.ID),
			Data: map[string]any{
				"constraints": map[string]any{
					"video": true,
					"audio": false,
				},
			},
		}

		client.handleRequestOfferMessage(requestOfferMessage)

		// 验证PeerConnection被创建
		pc, exists := pcManager.GetPeerConnection(client.ID)
		assert.True(t, exists)
		require.NotNil(t, pc)
	}

	// 验证所有连接都存在
	allConnections := pcManager.GetAllConnections()
	assert.Len(t, allConnections, 3)

	// 验证指标
	metrics := pcManager.GetMetrics()
	assert.Equal(t, int64(3), metrics.TotalConnections)
	assert.Equal(t, int64(3), metrics.ActiveConnections)

	// 模拟一个客户端断开连接
	err := pcManager.RemovePeerConnection(clients[0].ID)
	assert.NoError(t, err)

	// 验证连接数减少
	metrics = pcManager.GetMetrics()
	assert.Equal(t, int64(3), metrics.TotalConnections)  // 总数不变
	assert.Equal(t, int64(2), metrics.ActiveConnections) // 活跃数减少

	// 模拟另一个客户端连接失败
	pcManager.handleConnectionStateChange(clients[1].ID, webrtc.PeerConnectionStateFailed)

	// 验证连接被标记为不健康
	health, err := pcManager.GetConnectionHealth(clients[1].ID)
	assert.NoError(t, err)
	assert.False(t, health.IsHealthy)
}

// TestSignalingServer_ConnectionTimeoutHandling 测试连接超时处理
func TestSignalingServer_ConnectionTimeoutHandling(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "TIMEOUT_INTEGRATION_TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)
	defer pcManager.Close()

	// 设置很短的超时时间
	pcManager.SetConnectionTimeout(1 * time.Second)

	// 创建信令服务器
	server := createTestSignalingServer(pcManager)

	// 创建客户端
	client := &SignalingClient{
		ID:               "timeout-integration-client",
		Server:           server,
		LastSeen:         time.Now(),
		ConnectedAt:      time.Now(),
		State:            ClientStateConnected,
		MessageCount:     0,
		ErrorCount:       0,
		ProtocolDetected: true,
		Protocol:         protocol.ProtocolVersionGStreamer10,
	}

	// 处理request-offer消息
	requestOfferMessage := &protocol.StandardMessage{
		Type: protocol.MessageTypeRequestOffer,
		ID:   "timeout-request-offer",
		Data: map[string]any{
			"constraints": map[string]any{
				"video": true,
				"audio": true,
			},
		},
	}

	client.handleRequestOfferMessage(requestOfferMessage)

	// 验证连接被创建
	pc, exists := pcManager.GetPeerConnection(client.ID)
	assert.True(t, exists)
	require.NotNil(t, pc)

	// 验证初始健康状态
	health, err := pcManager.GetConnectionHealth(client.ID)
	assert.NoError(t, err)
	assert.True(t, health.IsHealthy)

	// 等待超时
	time.Sleep(2 * time.Second)

	// 手动触发健康检查
	pcManager.performHealthCheck()

	// 验证连接被标记为不健康
	health, err = pcManager.GetConnectionHealth(client.ID)
	assert.NoError(t, err)
	assert.False(t, health.IsHealthy)

	// 触发清理
	pcManager.cleanupUnhealthyConnections()

	// 等待清理完成
	time.Sleep(3 * time.Second)

	// 验证重连尝试
	metrics := pcManager.GetMetrics()
	assert.Greater(t, metrics.ReconnectAttempts, int64(0))
}

// TestSignalingServer_StateChangeCallbackIntegration 测试状态变化回调集成
func TestSignalingServer_StateChangeCallbackIntegration(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "CALLBACK_INTEGRATION_TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)
	defer pcManager.Close()

	// 设置状态变化回调
	stateChanges := make([]webrtc.PeerConnectionState, 0)
	pcManager.RegisterStateChangeCallback("test-callback", func(clientID string, state webrtc.PeerConnectionState) {
		stateChanges = append(stateChanges, state)
	})

	// 创建信令服务器
	server := createTestSignalingServer(pcManager)

	// 创建客户端
	client := &SignalingClient{
		ID:               "callback-integration-client",
		Server:           server,
		LastSeen:         time.Now(),
		ConnectedAt:      time.Now(),
		State:            ClientStateConnected,
		MessageCount:     0,
		ErrorCount:       0,
		ProtocolDetected: true,
		Protocol:         protocol.ProtocolVersionGStreamer10,
	}

	// 处理request-offer消息
	requestOfferMessage := &protocol.StandardMessage{
		Type: protocol.MessageTypeRequestOffer,
		ID:   "callback-request-offer",
		Data: map[string]any{
			"constraints": map[string]any{
				"video": true,
				"audio": true,
			},
		},
	}

	client.handleRequestOfferMessage(requestOfferMessage)

	// 模拟状态变化
	pcManager.handleConnectionStateChange(client.ID, webrtc.PeerConnectionStateConnecting)
	pcManager.handleConnectionStateChange(client.ID, webrtc.PeerConnectionStateConnected)
	pcManager.handleConnectionStateChange(client.ID, webrtc.PeerConnectionStateDisconnected)

	// 等待回调执行
	time.Sleep(100 * time.Millisecond)

	// 验证回调被调用
	assert.GreaterOrEqual(t, len(stateChanges), 3)
	assert.Contains(t, stateChanges, webrtc.PeerConnectionStateConnecting)
	assert.Contains(t, stateChanges, webrtc.PeerConnectionStateConnected)
	assert.Contains(t, stateChanges, webrtc.PeerConnectionStateDisconnected)
}

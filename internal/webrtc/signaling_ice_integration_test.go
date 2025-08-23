package webrtc

import (
	"context"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalingServer_ICECandidateIntegration(t *testing.T) {
	// 创建信令服务器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	encoderConfig := &SignalingEncoderConfig{
		Codec: "h264",
	}

	mediaStream := &MockMediaStream{}
	mediaStream.On("GetVideoTrack").Return(nil)
	mediaStream.On("GetAudioTrack").Return(nil)

	iceServers := []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	}

	server := NewSignalingServer(ctx, encoderConfig, mediaStream, iceServers)
	require.NotNil(t, server)
	require.NotNil(t, server.peerConnectionManager)

	// 创建测试客户端
	clientID := "test-integration-client"
	client := &SignalingClient{
		ID:     clientID,
		Server: server,
	}

	// 创建PeerConnection
	pc, err := server.peerConnectionManager.CreatePeerConnection(clientID)
	require.NoError(t, err)
	require.NotNil(t, pc)

	// 测试ICE候选处理集成
	t.Run("ICE Candidate Processing Integration", func(t *testing.T) {
		// 创建ICE候选消息
		message := SignalingMessage{
			Type: "ice-candidate",
			Data: map[string]interface{}{
				"candidate":     "candidate:1 1 UDP 2130706431 192.168.1.100 54400 typ host",
				"sdpMid":        "0",
				"sdpMLineIndex": float64(0),
			},
		}

		// 获取初始连接信息
		initialInfo, exists := server.peerConnectionManager.connections[clientID]
		require.True(t, exists)
		initialLastActive := initialInfo.LastActive

		// 等待一小段时间确保时间戳不同
		time.Sleep(10 * time.Millisecond)

		// 处理ICE候选消息
		client.handleIceCandidate(message)

		// 验证连接状态已更新
		updatedInfo, exists := server.peerConnectionManager.connections[clientID]
		require.True(t, exists)

		// 验证LastActive时间已更新（即使处理失败）
		assert.True(t, updatedInfo.LastActive.After(initialLastActive),
			"LastActive should be updated after processing ICE candidate")

		// 验证连接仍然健康
		assert.True(t, updatedInfo.IsHealthy, "Connection should remain healthy")
	})

	// 测试多个ICE候选的批量处理
	t.Run("Multiple ICE Candidates Processing", func(t *testing.T) {
		candidates := []map[string]interface{}{
			{
				"candidate":     "candidate:2 1 UDP 2130706430 192.168.1.100 54401 typ host",
				"sdpMid":        "0",
				"sdpMLineIndex": float64(0),
			},
			{
				"candidate":     "candidate:3 1 UDP 2130706429 10.0.0.1 54402 typ host",
				"sdpMid":        "0",
				"sdpMLineIndex": float64(0),
			},
		}

		// 获取初始连接信息
		initialInfo, exists := server.peerConnectionManager.connections[clientID]
		require.True(t, exists)
		initialLastActive := initialInfo.LastActive

		// 等待一小段时间确保时间戳不同
		time.Sleep(10 * time.Millisecond)

		// 处理多个ICE候选
		for _, candidateData := range candidates {
			message := SignalingMessage{
				Type: "ice-candidate",
				Data: candidateData,
			}
			client.handleIceCandidate(message)
		}

		// 验证连接状态已更新
		updatedInfo, exists := server.peerConnectionManager.connections[clientID]
		require.True(t, exists)

		// 验证LastActive时间已更新
		assert.True(t, updatedInfo.LastActive.After(initialLastActive),
			"LastActive should be updated after processing multiple ICE candidates")
	})

	// 测试无效ICE候选的处理
	t.Run("Invalid ICE Candidate Handling", func(t *testing.T) {
		// 创建无效的ICE候选消息
		message := SignalingMessage{
			Type: "ice-candidate",
			Data: map[string]interface{}{
				"candidate": "", // 空候选字符串
			},
		}

		// 处理无效的ICE候选消息
		client.handleIceCandidate(message)

		// 验证连接仍然存在
		updatedInfo, exists := server.peerConnectionManager.connections[clientID]
		require.True(t, exists)

		// 验证连接仍然健康（处理无效候选不应影响连接健康状态）
		assert.True(t, updatedInfo.IsHealthy, "Connection should remain healthy after invalid candidate")
	})

	// 清理
	server.peerConnectionManager.Close()
}

func TestSignalingServer_ICECandidateDirectIntegration(t *testing.T) {
	// 测试PeerConnectionManager的HandleICECandidate方法是否被正确调用
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	encoderConfig := &SignalingEncoderConfig{
		Codec: "h264",
	}

	mediaStream := &MockMediaStream{}
	mediaStream.On("GetVideoTrack").Return(nil)
	mediaStream.On("GetAudioTrack").Return(nil)

	iceServers := []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	}

	server := NewSignalingServer(ctx, encoderConfig, mediaStream, iceServers)
	require.NotNil(t, server)
	require.NotNil(t, server.peerConnectionManager)

	clientID := "test-direct-client"

	// 创建PeerConnection
	pc, err := server.peerConnectionManager.CreatePeerConnection(clientID)
	require.NoError(t, err)
	require.NotNil(t, pc)

	// 直接测试PeerConnectionManager的HandleICECandidate方法
	t.Run("Direct HandleICECandidate Call", func(t *testing.T) {
		candidateData := map[string]interface{}{
			"candidate":     "candidate:1 1 UDP 2130706431 192.168.1.100 54400 typ host",
			"sdpMid":        "0",
			"sdpMLineIndex": float64(0),
		}

		// 获取初始连接信息
		initialInfo, exists := server.peerConnectionManager.connections[clientID]
		require.True(t, exists)
		initialLastActive := initialInfo.LastActive

		// 等待一小段时间确保时间戳不同
		time.Sleep(10 * time.Millisecond)

		// 直接调用HandleICECandidate方法
		err := server.peerConnectionManager.HandleICECandidate(clientID, candidateData)
		// 应该失败，因为没有设置远程描述，但这是正确的行为
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "remote description is not set")

		// 验证连接状态已更新
		updatedInfo, exists := server.peerConnectionManager.connections[clientID]
		require.True(t, exists)

		// 验证LastActive时间已更新
		assert.True(t, updatedInfo.LastActive.After(initialLastActive),
			"LastActive should be updated after HandleICECandidate call")

		// 验证连接仍然健康
		assert.True(t, updatedInfo.IsHealthy, "Connection should remain healthy")
	})

	// 清理
	server.peerConnectionManager.Close()
}

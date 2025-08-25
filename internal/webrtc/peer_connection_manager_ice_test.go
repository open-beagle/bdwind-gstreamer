package webrtc

import (
	"log"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeerConnectionManager_HandleICECandidate(t *testing.T) {
	// 创建PeerConnection管理器
	mediaStream := &MockMediaStream{}
	mediaStream.On("GetVideoTrack").Return(nil)
	mediaStream.On("GetAudioTrack").Return(nil)
	mediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	iceServers := []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	}

	pcManager := NewPeerConnectionManager(mediaStream, iceServers, log.Default())
	defer pcManager.Close()

	clientID := "test-client-1"

	// 创建PeerConnection
	pc, err := pcManager.CreatePeerConnection(clientID)
	require.NoError(t, err)
	require.NotNil(t, pc)

	// 测试ICE候选处理（在没有远程描述的情况下会失败，这是正确的行为）
	t.Run("ICE Candidate Without Remote Description", func(t *testing.T) {
		candidateData := map[string]interface{}{
			"candidate":     "candidate:1 1 UDP 2130706431 192.168.1.100 54400 typ host",
			"sdpMid":        "0",
			"sdpMLineIndex": float64(0),
		}

		err := pcManager.HandleICECandidate(clientID, candidateData)
		// 应该失败，因为没有设置远程描述
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "remote description is not set")

		// 验证连接信息仍然存在且LastActive时间已更新
		info, exists := pcManager.connections[clientID]
		require.True(t, exists)

		// 验证LastActive时间已更新（应该在最近1秒内）
		assert.WithinDuration(t, time.Now(), info.LastActive, time.Second)
	})

	// 测试无效的客户端ID
	t.Run("Invalid Client ID", func(t *testing.T) {
		candidateData := map[string]interface{}{
			"candidate": "candidate:1 1 UDP 2130706431 192.168.1.100 54400 typ host",
		}

		err := pcManager.HandleICECandidate("non-existent-client", candidateData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "peer connection not found")
	})

	// 测试无效的候选数据
	t.Run("Invalid Candidate Data", func(t *testing.T) {
		testCases := []struct {
			name          string
			candidateData map[string]interface{}
			expectedError string
		}{
			{
				name:          "Missing candidate field",
				candidateData: map[string]interface{}{},
				expectedError: "candidate field is required",
			},
			{
				name: "Empty candidate string",
				candidateData: map[string]interface{}{
					"candidate": "",
				},
				expectedError: "candidate field cannot be empty",
			},
			{
				name: "Non-string candidate",
				candidateData: map[string]interface{}{
					"candidate": 123,
				},
				expectedError: "candidate field is required",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := pcManager.HandleICECandidate(clientID, tc.candidateData)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			})
		}
	})
}

func TestPeerConnectionManager_HandleMultipleICECandidates(t *testing.T) {
	// 创建PeerConnection管理器
	mediaStream := &MockMediaStream{}
	mediaStream.On("GetVideoTrack").Return(nil)
	mediaStream.On("GetAudioTrack").Return(nil)
	mediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	iceServers := []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	}

	pcManager := NewPeerConnectionManager(mediaStream, iceServers, log.Default())
	defer pcManager.Close()

	clientID := "test-client-2"

	// 创建PeerConnection
	pc, err := pcManager.CreatePeerConnection(clientID)
	require.NoError(t, err)
	require.NotNil(t, pc)

	// 测试批量处理多个有效的ICE候选
	t.Run("Multiple Valid Candidates", func(t *testing.T) {
		candidates := []map[string]interface{}{
			{
				"candidate":     "candidate:1 1 UDP 2130706431 192.168.1.100 54400 typ host",
				"sdpMid":        "0",
				"sdpMLineIndex": float64(0),
			},
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

		err := pcManager.HandleMultipleICECandidates(clientID, candidates)
		// 应该失败，因为所有候选都无法添加（没有远程描述）
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to process any ICE candidates")

		// 验证连接信息已更新
		info, exists := pcManager.connections[clientID]
		require.True(t, exists)

		// 验证LastActive时间已更新
		assert.WithinDuration(t, time.Now(), info.LastActive, time.Second)
	})

	// 测试部分候选失败的情况
	t.Run("Partial Failure", func(t *testing.T) {
		candidates := []map[string]interface{}{
			{
				"candidate":     "candidate:4 1 UDP 2130706431 192.168.1.100 54403 typ host",
				"sdpMid":        "0",
				"sdpMLineIndex": float64(0),
			},
			{
				// 无效的候选数据
				"candidate": "",
			},
			{
				"candidate":     "candidate:5 1 UDP 2130706429 10.0.0.1 54404 typ host",
				"sdpMid":        "0",
				"sdpMLineIndex": float64(0),
			},
		}

		err := pcManager.HandleMultipleICECandidates(clientID, candidates)
		// 应该失败，因为所有候选都无法添加（没有远程描述）
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to process any ICE candidates")
	})

	// 测试所有候选都失败的情况
	t.Run("All Candidates Fail", func(t *testing.T) {
		candidates := []map[string]interface{}{
			{"candidate": ""},
			{"candidate": 123},
			{}, // 缺少candidate字段
		}

		err := pcManager.HandleMultipleICECandidates(clientID, candidates)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to process any ICE candidates")
	})

	// 测试无效的客户端ID
	t.Run("Invalid Client ID", func(t *testing.T) {
		candidates := []map[string]interface{}{
			{
				"candidate": "candidate:1 1 UDP 2130706431 192.168.1.100 54400 typ host",
			},
		}

		err := pcManager.HandleMultipleICECandidates("non-existent-client", candidates)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "peer connection not found")
	})
}

func TestPeerConnectionManager_ICECandidateConnectionStateUpdate(t *testing.T) {
	// 创建PeerConnection管理器
	mediaStream := &MockMediaStream{}
	mediaStream.On("GetVideoTrack").Return(nil)
	mediaStream.On("GetAudioTrack").Return(nil)
	mediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	iceServers := []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	}

	pcManager := NewPeerConnectionManager(mediaStream, iceServers, log.Default())
	defer pcManager.Close()

	clientID := "test-client-3"

	// 创建PeerConnection
	pc, err := pcManager.CreatePeerConnection(clientID)
	require.NoError(t, err)
	require.NotNil(t, pc)

	// 获取初始连接信息
	initialInfo, exists := pcManager.connections[clientID]
	require.True(t, exists)
	initialLastActive := initialInfo.LastActive

	// 等待一小段时间确保时间戳不同
	time.Sleep(10 * time.Millisecond)

	// 处理ICE候选
	candidateData := map[string]interface{}{
		"candidate":     "candidate:1 1 UDP 2130706431 192.168.1.100 54400 typ host",
		"sdpMid":        "0",
		"sdpMLineIndex": float64(0),
	}

	err = pcManager.HandleICECandidate(clientID, candidateData)
	// 应该失败，因为没有设置远程描述，但LastActive时间仍应更新
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "remote description is not set")

	// 验证连接状态已更新
	updatedInfo, exists := pcManager.connections[clientID]
	require.True(t, exists)

	// 验证LastActive时间已更新（即使处理失败）
	assert.True(t, updatedInfo.LastActive.After(initialLastActive),
		"LastActive should be updated after processing ICE candidate")

	// 验证连接仍然健康
	assert.True(t, updatedInfo.IsHealthy, "Connection should remain healthy after processing ICE candidate")
}

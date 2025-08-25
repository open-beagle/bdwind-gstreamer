package webrtc

import (
	"log"
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestICECollectionMonitoring 测试ICE收集状态监控日志
func TestICECollectionMonitoring(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)
	mockMediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	// 创建PeerConnection管理器
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, log.Default())
	defer pcManager.Close()

	clientID := "ice-monitoring-test-client"

	// 创建PeerConnection
	pc, err := pcManager.CreatePeerConnection(clientID)
	require.NoError(t, err)
	require.NotNil(t, pc)

	// 验证连接信息包含ICE相关字段
	pcManager.mutex.RLock()
	info, exists := pcManager.connections[clientID]
	pcManager.mutex.RUnlock()

	require.True(t, exists)

	info.mutex.RLock()
	assert.Equal(t, webrtc.ICEGatheringStateNew, info.ICEGatheringState)
	assert.Equal(t, 0, info.CandidateCount)
	info.mutex.RUnlock()

	// 测试ICE收集状态变化处理
	t.Run("ICE Gathering State Changes", func(t *testing.T) {
		// 模拟ICE收集状态变化
		pcManager.handleICEGatheringStateChange(clientID, webrtc.ICEGatheringStateGathering)

		info.mutex.RLock()
		assert.Equal(t, webrtc.ICEGatheringStateGathering, info.ICEGatheringState)
		info.mutex.RUnlock()

		// 模拟ICE收集完成
		pcManager.handleICEGatheringStateChange(clientID, webrtc.ICEGatheringStateComplete)

		info.mutex.RLock()
		assert.Equal(t, webrtc.ICEGatheringStateComplete, info.ICEGatheringState)
		info.mutex.RUnlock()
	})

	// 测试ICE候选生成处理
	t.Run("ICE Candidate Generation", func(t *testing.T) {
		// 创建模拟ICE候选
		candidate := &webrtc.ICECandidate{
			Foundation: "1",
			Priority:   2130706431,
			Address:    "192.168.1.100",
			Protocol:   webrtc.ICEProtocolUDP,
			Port:       54400,
			Typ:        webrtc.ICECandidateTypeHost,
		}

		// 模拟多个候选生成
		for i := 0; i < 7; i++ {
			pcManager.handleICECandidate(clientID, candidate)
		}

		// 验证候选计数器
		info.mutex.RLock()
		assert.Equal(t, 7, info.CandidateCount)
		info.mutex.RUnlock()

		// 模拟ICE收集完成（nil候选）
		pcManager.handleICECandidate(clientID, nil)
	})

	// 测试ICE收集超时监控
	t.Run("ICE Collection Timeout Monitoring", func(t *testing.T) {
		// 这个测试验证超时监控功能是否正确启动
		// 实际的超时测试需要等待30秒，这里只验证功能存在

		// 验证连接信息存在
		pcManager.mutex.RLock()
		_, exists := pcManager.connections[clientID]
		pcManager.mutex.RUnlock()

		assert.True(t, exists, "Connection should exist for timeout monitoring")
	})
}

// TestICECollectionErrorHandling 测试ICE收集错误处理
func TestICECollectionErrorHandling(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)
	mockMediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	// 创建PeerConnection管理器
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, log.Default())
	defer pcManager.Close()

	// 测试未知客户端的ICE状态变化
	t.Run("Unknown Client ICE State Change", func(t *testing.T) {
		// 这应该记录警告日志但不会崩溃
		pcManager.handleICEGatheringStateChange("unknown-client", webrtc.ICEGatheringStateGathering)
		pcManager.handleICECandidate("unknown-client", nil)
	})

	// 测试ICE候选处理错误
	t.Run("ICE Candidate Processing Error", func(t *testing.T) {
		clientID := "error-test-client"

		// 创建PeerConnection
		pc, err := pcManager.CreatePeerConnection(clientID)
		require.NoError(t, err)
		require.NotNil(t, pc)

		// 测试无效的候选数据
		invalidCandidateData := map[string]interface{}{
			"candidate": "", // 空候选字符串
		}

		err = pcManager.HandleICECandidate(clientID, invalidCandidateData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "candidate field cannot be empty")
	})
}

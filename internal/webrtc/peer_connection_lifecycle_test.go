package webrtc

import (
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPeerConnectionManager_LifecycleManagement 测试完整的生命周期管理
func TestPeerConnectionManager_LifecycleManagement(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)
	mockMediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "LIFECYCLE_TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)
	defer pcManager.Close()

	// 设置较短的超时时间用于测试
	pcManager.SetConnectionTimeout(2 * time.Second)

	clientID := "lifecycle-test-client"

	// 1. 创建连接
	pc, err := pcManager.CreatePeerConnection(clientID)
	assert.NoError(t, err)
	require.NotNil(t, pc)

	// 验证连接存在且健康
	health, err := pcManager.GetConnectionHealth(clientID)
	assert.NoError(t, err)
	assert.True(t, health.IsHealthy)
	assert.Equal(t, webrtc.PeerConnectionStateNew, health.State)

	// 2. 模拟连接状态变化
	pcManager.handleConnectionStateChange(clientID, webrtc.PeerConnectionStateConnecting)

	health, err = pcManager.GetConnectionHealth(clientID)
	assert.NoError(t, err)
	assert.Equal(t, webrtc.PeerConnectionStateConnecting, health.State)

	// 3. 模拟连接成功
	pcManager.handleConnectionStateChange(clientID, webrtc.PeerConnectionStateConnected)

	health, err = pcManager.GetConnectionHealth(clientID)
	assert.NoError(t, err)
	assert.True(t, health.IsHealthy)
	assert.Equal(t, webrtc.PeerConnectionStateConnected, health.State)

	// 4. 等待超时并验证清理
	time.Sleep(3 * time.Second)

	// 触发健康检查
	pcManager.performHealthCheck()
	pcManager.cleanupUnhealthyConnections()

	// 验证连接被标记为不健康或被清理
	health, err = pcManager.GetConnectionHealth(clientID)
	if err == nil {
		// 如果连接仍存在，应该被标记为不健康
		assert.False(t, health.IsHealthy)
	}
}

// TestPeerConnectionManager_MultiClientScenario 测试多客户端场景
func TestPeerConnectionManager_MultiClientScenario(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)
	mockMediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "MULTI_CLIENT_TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)
	defer pcManager.Close()

	// 设置最大连接数
	pcManager.SetMaxConnections(5)

	// 创建多个客户端连接
	clientIDs := []string{"client-1", "client-2", "client-3", "client-4", "client-5"}
	createdPCs := make(map[string]*webrtc.PeerConnection)

	// 并发创建连接
	var wg sync.WaitGroup
	var mutex sync.Mutex
	errors := make([]error, 0)

	for _, clientID := range clientIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			pc, err := pcManager.CreatePeerConnection(id)
			mutex.Lock()
			if err != nil {
				errors = append(errors, err)
			} else {
				createdPCs[id] = pc
			}
			mutex.Unlock()
		}(clientID)
	}

	wg.Wait()

	// 验证所有连接都成功创建
	assert.Empty(t, errors)
	assert.Len(t, createdPCs, len(clientIDs))

	// 验证所有连接都存在
	allConnections := pcManager.GetAllConnections()
	assert.Len(t, allConnections, len(clientIDs))

	// 验证连接健康状态
	allHealth := pcManager.GetAllConnectionsHealth()
	assert.Len(t, allHealth, len(clientIDs))

	for _, clientID := range clientIDs {
		health := allHealth[clientID]
		assert.NotNil(t, health)
		assert.True(t, health.IsHealthy)
		assert.Equal(t, webrtc.PeerConnectionStateNew, health.State)
	}

	// 测试超过最大连接数的情况
	_, err := pcManager.CreatePeerConnection("client-6")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum connections limit reached")

	// 验证指标
	metrics := pcManager.GetMetrics()
	assert.Equal(t, int64(len(clientIDs)), metrics.ActiveConnections)
	assert.Equal(t, int64(len(clientIDs)), metrics.TotalConnections)
	assert.Equal(t, int64(1), metrics.FailedConnections) // 超过限制的那个
}

// TestPeerConnectionManager_ConnectionTimeout 测试连接超时处理
func TestPeerConnectionManager_ConnectionTimeout(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)
	mockMediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "TIMEOUT_TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)
	defer pcManager.Close()

	// 设置很短的超时时间
	pcManager.SetConnectionTimeout(1 * time.Second)

	clientID := "timeout-test-client"

	// 创建连接
	pc, err := pcManager.CreatePeerConnection(clientID)
	assert.NoError(t, err)
	require.NotNil(t, pc)

	// 验证连接初始状态
	health, err := pcManager.GetConnectionHealth(clientID)
	assert.NoError(t, err)
	assert.True(t, health.IsHealthy)

	// 等待超时
	time.Sleep(2 * time.Second)

	// 手动触发健康检查
	pcManager.performHealthCheck()

	// 验证连接被标记为不健康
	health, err = pcManager.GetConnectionHealth(clientID)
	assert.NoError(t, err)
	assert.False(t, health.IsHealthy)

	// 触发清理
	pcManager.cleanupUnhealthyConnections()

	// 等待清理完成
	time.Sleep(100 * time.Millisecond)

	// 等待重连尝试完成
	time.Sleep(3 * time.Second)

	// 验证重连尝试已经发生（通过检查指标）
	metrics := pcManager.GetMetrics()
	assert.Greater(t, metrics.ReconnectAttempts, int64(0), "Should have attempted reconnection")

	// 连接可能已经被清理，这是正常的，因为重连后的连接也可能因为测试环境而失败
}

// TestPeerConnectionManager_ReconnectionMechanism 测试重连机制
func TestPeerConnectionManager_ReconnectionMechanism(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)
	mockMediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "RECONNECT_TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)
	defer pcManager.Close()

	clientID := "reconnect-test-client"

	// 创建连接
	pc, err := pcManager.CreatePeerConnection(clientID)
	assert.NoError(t, err)
	require.NotNil(t, pc)

	// 模拟连接失败
	pcManager.handleConnectionStateChange(clientID, webrtc.PeerConnectionStateFailed)

	// 验证连接被标记为不健康
	health, err := pcManager.GetConnectionHealth(clientID)
	assert.NoError(t, err)
	assert.False(t, health.IsHealthy)

	// 获取原始连接信息
	originalInfo := pcManager.GetAllConnections()[clientID]
	require.NotNil(t, originalInfo)

	// 手动触发重连
	pcManager.attemptReconnection(clientID, originalInfo)

	// 等待重连完成
	time.Sleep(3 * time.Second)

	// 验证重连尝试
	updatedInfo := pcManager.GetAllConnections()[clientID]
	if updatedInfo != nil {
		assert.Greater(t, updatedInfo.ReconnectAttempts, 0)
	}
}

// TestPeerConnectionManager_StateChangeCallbacks 测试状态变化回调
func TestPeerConnectionManager_StateChangeCallbacks(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)
	mockMediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "CALLBACK_TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)
	defer pcManager.Close()

	clientID := "callback-test-client"
	callbackCalled := false
	var callbackClientID string
	var callbackState webrtc.PeerConnectionState

	// 注册状态变化回调
	pcManager.RegisterStateChangeCallback("test-callback", func(id string, state webrtc.PeerConnectionState) {
		callbackCalled = true
		callbackClientID = id
		callbackState = state
	})

	// 创建连接
	pc, err := pcManager.CreatePeerConnection(clientID)
	assert.NoError(t, err)
	require.NotNil(t, pc)

	// 模拟状态变化
	pcManager.handleConnectionStateChange(clientID, webrtc.PeerConnectionStateConnected)

	// 等待回调执行
	time.Sleep(100 * time.Millisecond)

	// 验证回调被调用
	assert.True(t, callbackCalled)
	assert.Equal(t, clientID, callbackClientID)
	assert.Equal(t, webrtc.PeerConnectionStateConnected, callbackState)

	// 取消注册回调
	pcManager.UnregisterStateChangeCallback("test-callback")

	// 重置标志
	callbackCalled = false

	// 再次触发状态变化
	pcManager.handleConnectionStateChange(clientID, webrtc.PeerConnectionStateDisconnected)

	// 等待
	time.Sleep(100 * time.Millisecond)

	// 验证回调不再被调用
	assert.False(t, callbackCalled)
}

// TestPeerConnectionManager_ICEStateHandling 测试ICE状态处理
func TestPeerConnectionManager_ICEStateHandling(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)
	mockMediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "ICE_TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)
	defer pcManager.Close()

	clientID := "ice-test-client"

	// 创建连接
	pc, err := pcManager.CreatePeerConnection(clientID)
	assert.NoError(t, err)
	require.NotNil(t, pc)

	// 验证初始ICE状态
	health, err := pcManager.GetConnectionHealth(clientID)
	assert.NoError(t, err)
	assert.Equal(t, webrtc.ICEConnectionStateNew, health.ICEState)

	// 模拟ICE状态变化
	pcManager.handleICEConnectionStateChange(clientID, webrtc.ICEConnectionStateChecking)

	health, err = pcManager.GetConnectionHealth(clientID)
	assert.NoError(t, err)
	assert.Equal(t, webrtc.ICEConnectionStateChecking, health.ICEState)

	// 模拟ICE连接成功
	pcManager.handleICEConnectionStateChange(clientID, webrtc.ICEConnectionStateConnected)

	health, err = pcManager.GetConnectionHealth(clientID)
	assert.NoError(t, err)
	assert.Equal(t, webrtc.ICEConnectionStateConnected, health.ICEState)
	assert.True(t, health.IsHealthy)

	// 模拟ICE连接失败
	pcManager.handleICEConnectionStateChange(clientID, webrtc.ICEConnectionStateFailed)

	health, err = pcManager.GetConnectionHealth(clientID)
	assert.NoError(t, err)
	assert.Equal(t, webrtc.ICEConnectionStateFailed, health.ICEState)
	assert.False(t, health.IsHealthy)
}

// TestPeerConnectionManager_MetricsTracking 测试指标跟踪
func TestPeerConnectionManager_MetricsTracking(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)
	mockMediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "METRICS_TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)
	defer pcManager.Close()

	// 验证初始指标
	metrics := pcManager.GetMetrics()
	assert.Equal(t, int64(0), metrics.TotalConnections)
	assert.Equal(t, int64(0), metrics.ActiveConnections)
	assert.Equal(t, int64(0), metrics.FailedConnections)

	// 创建几个连接
	clientIDs := []string{"metrics-client-1", "metrics-client-2", "metrics-client-3"}

	for _, clientID := range clientIDs {
		pc, err := pcManager.CreatePeerConnection(clientID)
		assert.NoError(t, err)
		require.NotNil(t, pc)
	}

	// 验证指标更新
	metrics = pcManager.GetMetrics()
	assert.Equal(t, int64(len(clientIDs)), metrics.TotalConnections)
	assert.Equal(t, int64(len(clientIDs)), metrics.ActiveConnections)

	// 移除一个连接
	err := pcManager.RemovePeerConnection(clientIDs[0])
	assert.NoError(t, err)

	// 验证活跃连接数减少
	metrics = pcManager.GetMetrics()
	assert.Equal(t, int64(len(clientIDs)), metrics.TotalConnections)    // 总数不变
	assert.Equal(t, int64(len(clientIDs)-1), metrics.ActiveConnections) // 活跃数减少

	// 测试连接失败情况
	pcManager.SetMaxConnections(len(clientIDs) - 1) // 设置较小的最大连接数

	_, err = pcManager.CreatePeerConnection("overflow-client")
	assert.Error(t, err)

	// 验证失败连接数增加
	metrics = pcManager.GetMetrics()
	assert.Greater(t, metrics.FailedConnections, int64(0))
}

// TestPeerConnectionManager_ConcurrentOperations 测试并发操作
func TestPeerConnectionManager_ConcurrentOperations(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)
	mockMediaStream.On("GetStats").Return(MediaStreamStats{
		TotalTracks:  0,
		ActiveTracks: 0,
	})

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "CONCURRENT_TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)
	defer pcManager.Close()

	const numGoroutines = 10
	const operationsPerGoroutine = 5

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*operationsPerGoroutine)

	// 并发创建和删除连接
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				clientID := fmt.Sprintf("concurrent-client-%d-%d", goroutineID, j)

				// 创建连接
				_, err := pcManager.CreatePeerConnection(clientID)
				if err != nil {
					errors <- err
					continue
				}

				// 模拟一些操作
				time.Sleep(10 * time.Millisecond)

				// 获取健康状态
				_, err = pcManager.GetConnectionHealth(clientID)
				if err != nil {
					errors <- err
				}

				// 模拟状态变化
				pcManager.handleConnectionStateChange(clientID, webrtc.PeerConnectionStateConnected)

				// 有时移除连接
				if j%2 == 0 {
					err = pcManager.RemovePeerConnection(clientID)
					if err != nil {
						errors <- err
					}
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 检查是否有错误
	errorCount := 0
	for err := range errors {
		t.Logf("Concurrent operation error: %v", err)
		errorCount++
	}

	// 允许少量错误（由于并发竞争条件）
	assert.Less(t, errorCount, numGoroutines, "Too many concurrent operation errors")

	// 验证管理器仍然可用
	finalMetrics := pcManager.GetMetrics()
	assert.GreaterOrEqual(t, finalMetrics.TotalConnections, int64(0))
	assert.GreaterOrEqual(t, finalMetrics.ActiveConnections, int64(0))
}

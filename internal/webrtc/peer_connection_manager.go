package webrtc

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// PeerConnectionManager 管理WebRTC对等连接
type PeerConnectionManager struct {
	config               *webrtc.Configuration
	mediaStream          MediaStream
	connections          map[string]*PeerConnectionInfo
	mutex                sync.RWMutex
	logger               *log.Logger
	connectionTimeout    time.Duration
	maxConnections       int
	cleanupInterval      time.Duration
	reconnectAttempts    int
	reconnectDelay       time.Duration
	stateChangeCallbacks map[string]func(clientID string, state webrtc.PeerConnectionState)
	connectionMetrics    *ConnectionMetrics
	ctx                  context.Context
	cancel               context.CancelFunc
	cleanupTicker        *time.Ticker
}

// PeerConnectionInfo 对等连接信息
type PeerConnectionInfo struct {
	ID                string
	PC                *webrtc.PeerConnection
	CreatedAt         time.Time
	LastActive        time.Time
	State             webrtc.PeerConnectionState
	ICEState          webrtc.ICEConnectionState
	ReconnectAttempts int
	LastError         error
	IsHealthy         bool
	BytesSent         uint64
	BytesReceived     uint64
	PacketsLost       uint32
	RTT               time.Duration
	mutex             sync.RWMutex
}

// ConnectionMetrics 连接指标
type ConnectionMetrics struct {
	TotalConnections  int64
	ActiveConnections int64
	FailedConnections int64
	ReconnectAttempts int64
	CleanupOperations int64
	AverageConnTime   time.Duration
	mutex             sync.RWMutex
}

// ConnectionHealthStatus 连接健康状态
type ConnectionHealthStatus struct {
	ClientID      string
	IsHealthy     bool
	State         webrtc.PeerConnectionState
	ICEState      webrtc.ICEConnectionState
	LastActive    time.Time
	ConnectedFor  time.Duration
	BytesSent     uint64
	BytesReceived uint64
	PacketsLost   uint32
	RTT           time.Duration
	LastError     string
}

// NewPeerConnectionManager 创建对等连接管理器
func NewPeerConnectionManager(mediaStream MediaStream, iceServers []webrtc.ICEServer, logger *log.Logger) *PeerConnectionManager {
	// 如果没有提供ICE服务器，使用默认的
	if len(iceServers) == 0 {
		iceServers = []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
					"stun:stun1.l.google.com:19302",
				},
			},
		}
	}

	config := &webrtc.Configuration{
		ICEServers: iceServers,
		// 为WSL2环境优化ICE配置
		ICETransportPolicy: webrtc.ICETransportPolicyAll,
		BundlePolicy:       webrtc.BundlePolicyMaxBundle,
		RTCPMuxPolicy:      webrtc.RTCPMuxPolicyRequire,
	}

	ctx, cancel := context.WithCancel(context.Background())

	pcm := &PeerConnectionManager{
		config:               config,
		mediaStream:          mediaStream,
		connections:          make(map[string]*PeerConnectionInfo),
		logger:               logger,
		connectionTimeout:    5 * time.Minute,  // 5分钟连接超时
		maxConnections:       100,              // 最大连接数
		cleanupInterval:      30 * time.Second, // 30秒清理间隔
		reconnectAttempts:    3,                // 最大重连次数
		reconnectDelay:       2 * time.Second,  // 重连延迟
		stateChangeCallbacks: make(map[string]func(clientID string, state webrtc.PeerConnectionState)),
		connectionMetrics:    &ConnectionMetrics{},
		ctx:                  ctx,
		cancel:               cancel,
	}

	// 启动清理协程
	pcm.startCleanupRoutine()

	return pcm
}

// CreatePeerConnection 为客户端创建新的对等连接
func (pcm *PeerConnectionManager) CreatePeerConnection(clientID string) (*webrtc.PeerConnection, error) {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()

	// 检查最大连接数限制
	if len(pcm.connections) >= pcm.maxConnections {
		pcm.connectionMetrics.mutex.Lock()
		pcm.connectionMetrics.FailedConnections++
		pcm.connectionMetrics.mutex.Unlock()
		return nil, fmt.Errorf("maximum connections limit reached (%d)", pcm.maxConnections)
	}

	// 如果已存在连接，先关闭
	if existing, exists := pcm.connections[clientID]; exists {
		pcm.logger.Printf("Replacing existing connection for client %s", clientID)
		existing.PC.Close()
		delete(pcm.connections, clientID)
		pcm.connectionMetrics.mutex.Lock()
		pcm.connectionMetrics.ActiveConnections--
		pcm.connectionMetrics.mutex.Unlock()
	}

	// 创建新的PeerConnection
	pc, err := pcm.createNewPeerConnection(clientID)
	if err != nil {
		pcm.connectionMetrics.mutex.Lock()
		pcm.connectionMetrics.FailedConnections++
		pcm.connectionMetrics.mutex.Unlock()
		return nil, err
	}

	// 存储连接信息
	now := time.Now()
	pcm.connections[clientID] = &PeerConnectionInfo{
		ID:                clientID,
		PC:                pc,
		CreatedAt:         now,
		LastActive:        now,
		State:             webrtc.PeerConnectionStateNew,
		ICEState:          webrtc.ICEConnectionStateNew,
		ReconnectAttempts: 0,
		IsHealthy:         true,
		BytesSent:         0,
		BytesReceived:     0,
		PacketsLost:       0,
		RTT:               0,
	}

	// 更新指标
	pcm.connectionMetrics.mutex.Lock()
	pcm.connectionMetrics.TotalConnections++
	pcm.connectionMetrics.ActiveConnections++
	pcm.connectionMetrics.mutex.Unlock()

	pcm.logger.Printf("Created peer connection for client %s (total: %d, active: %d)",
		clientID, pcm.connectionMetrics.TotalConnections, len(pcm.connections))
	return pc, nil
}

// GetPeerConnection 获取客户端的对等连接
func (pcm *PeerConnectionManager) GetPeerConnection(clientID string) (*webrtc.PeerConnection, bool) {
	pcm.mutex.RLock()
	defer pcm.mutex.RUnlock()

	if info, exists := pcm.connections[clientID]; exists {
		return info.PC, true
	}
	return nil, false
}

// RemovePeerConnection 移除客户端的对等连接
func (pcm *PeerConnectionManager) RemovePeerConnection(clientID string) error {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()

	if info, exists := pcm.connections[clientID]; exists {
		info.PC.Close()
		delete(pcm.connections, clientID)
		pcm.logger.Printf("Removed peer connection for client %s", clientID)
		return nil
	}

	return fmt.Errorf("peer connection not found for client %s", clientID)
}

// updateConnectionState 更新连接状态（保留向后兼容性）
func (pcm *PeerConnectionManager) updateConnectionState(clientID string, state webrtc.PeerConnectionState) {
	// 这个方法现在由 handleConnectionStateChange 处理
	pcm.handleConnectionStateChange(clientID, state)
}

// GetAllConnections 获取所有连接信息
func (pcm *PeerConnectionManager) GetAllConnections() map[string]*PeerConnectionInfo {
	pcm.mutex.RLock()
	defer pcm.mutex.RUnlock()

	result := make(map[string]*PeerConnectionInfo)
	for id, info := range pcm.connections {
		result[id] = &PeerConnectionInfo{
			ID:         info.ID,
			PC:         info.PC,
			CreatedAt:  info.CreatedAt,
			LastActive: info.LastActive,
			State:      info.State,
		}
	}
	return result
}

// CleanupInactiveConnections 清理不活跃的连接
func (pcm *PeerConnectionManager) CleanupInactiveConnections(timeout time.Duration) {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()

	now := time.Now()
	for clientID, info := range pcm.connections {
		if now.Sub(info.LastActive) > timeout {
			pcm.logger.Printf("Cleaning up inactive connection for client %s", clientID)
			info.PC.Close()
			delete(pcm.connections, clientID)
		}
	}
}

// startCleanupRoutine 启动清理协程
func (pcm *PeerConnectionManager) startCleanupRoutine() {
	pcm.cleanupTicker = time.NewTicker(pcm.cleanupInterval)

	go func() {
		defer pcm.cleanupTicker.Stop()

		for {
			select {
			case <-pcm.cleanupTicker.C:
				pcm.performHealthCheck()
				pcm.cleanupUnhealthyConnections()
			case <-pcm.ctx.Done():
				pcm.logger.Printf("PeerConnection manager cleanup routine shutting down")
				return
			}
		}
	}()

	pcm.logger.Printf("PeerConnection manager cleanup routine started")
}

// performHealthCheck 执行连接健康检查
func (pcm *PeerConnectionManager) performHealthCheck() {
	pcm.mutex.RLock()
	connections := make([]*PeerConnectionInfo, 0, len(pcm.connections))
	for _, info := range pcm.connections {
		connections = append(connections, info)
	}
	pcm.mutex.RUnlock()

	now := time.Now()
	for _, info := range connections {
		info.mutex.Lock()

		// 检查连接超时
		if now.Sub(info.LastActive) > pcm.connectionTimeout {
			info.IsHealthy = false
			pcm.logger.Printf("Connection %s marked as unhealthy due to timeout (last active: %v ago)",
				info.ID, now.Sub(info.LastActive))
		}

		// 检查连接状态
		if info.State == webrtc.PeerConnectionStateFailed ||
			info.State == webrtc.PeerConnectionStateClosed ||
			info.ICEState == webrtc.ICEConnectionStateFailed ||
			info.ICEState == webrtc.ICEConnectionStateDisconnected {
			info.IsHealthy = false
			pcm.logger.Printf("Connection %s marked as unhealthy due to state (PC: %s, ICE: %s)",
				info.ID, info.State, info.ICEState)
		}

		info.mutex.Unlock()
	}
}

// cleanupUnhealthyConnections 清理不健康的连接
func (pcm *PeerConnectionManager) cleanupUnhealthyConnections() {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()

	unhealthyClients := make([]string, 0)

	for clientID, info := range pcm.connections {
		info.mutex.RLock()
		isHealthy := info.IsHealthy
		info.mutex.RUnlock()

		if !isHealthy {
			unhealthyClients = append(unhealthyClients, clientID)
		}
	}

	for _, clientID := range unhealthyClients {
		if info, exists := pcm.connections[clientID]; exists {
			pcm.logger.Printf("Cleaning up unhealthy connection for client %s", clientID)

			// 尝试重连
			if info.ReconnectAttempts < pcm.reconnectAttempts {
				pcm.attemptReconnection(clientID, info)
			} else {
				// 超过重连次数，彻底清理
				info.PC.Close()
				delete(pcm.connections, clientID)
				pcm.connectionMetrics.mutex.Lock()
				pcm.connectionMetrics.ActiveConnections--
				pcm.connectionMetrics.CleanupOperations++
				pcm.connectionMetrics.mutex.Unlock()
				pcm.logger.Printf("Permanently removed connection for client %s after %d failed reconnect attempts",
					clientID, info.ReconnectAttempts)
			}
		}
	}
}

// attemptReconnection 尝试重新连接
func (pcm *PeerConnectionManager) attemptReconnection(clientID string, info *PeerConnectionInfo) {
	info.mutex.Lock()
	info.ReconnectAttempts++
	info.mutex.Unlock()

	pcm.logger.Printf("Attempting reconnection for client %s (attempt %d/%d)",
		clientID, info.ReconnectAttempts, pcm.reconnectAttempts)

	// 延迟重连
	go func() {
		time.Sleep(pcm.reconnectDelay)

		// 创建新的PeerConnection
		newPC, err := pcm.createNewPeerConnection(clientID)
		if err != nil {
			pcm.logger.Printf("Failed to recreate PeerConnection for client %s: %v", clientID, err)
			info.mutex.Lock()
			info.LastError = err
			info.IsHealthy = false
			info.mutex.Unlock()

			pcm.connectionMetrics.mutex.Lock()
			pcm.connectionMetrics.FailedConnections++
			pcm.connectionMetrics.ReconnectAttempts++
			pcm.connectionMetrics.mutex.Unlock()
			return
		}

		pcm.mutex.Lock()
		if existingInfo, exists := pcm.connections[clientID]; exists {
			// 关闭旧连接
			existingInfo.PC.Close()

			// 更新连接信息
			existingInfo.PC = newPC
			existingInfo.LastActive = time.Now()
			existingInfo.IsHealthy = true
			existingInfo.State = webrtc.PeerConnectionStateNew
			existingInfo.ICEState = webrtc.ICEConnectionStateNew
			existingInfo.LastError = nil

			pcm.logger.Printf("Successfully reconnected client %s", clientID)

			pcm.connectionMetrics.mutex.Lock()
			pcm.connectionMetrics.ReconnectAttempts++
			pcm.connectionMetrics.mutex.Unlock()
		}
		pcm.mutex.Unlock()
	}()
}

// createNewPeerConnection 创建新的PeerConnection（内部方法）
func (pcm *PeerConnectionManager) createNewPeerConnection(clientID string) (*webrtc.PeerConnection, error) {
	pc, err := webrtc.NewPeerConnection(*pcm.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	// 添加媒体轨道
	if pcm.mediaStream != nil {
		videoTrack := pcm.mediaStream.GetVideoTrack()
		if videoTrack != nil {
			_, err = pc.AddTrack(videoTrack)
			if err != nil {
				pc.Close()
				return nil, fmt.Errorf("failed to add video track: %w", err)
			}
		}

		audioTrack := pcm.mediaStream.GetAudioTrack()
		if audioTrack != nil {
			_, err = pc.AddTrack(audioTrack)
			if err != nil {
				pc.Close()
				return nil, fmt.Errorf("failed to add audio track: %w", err)
			}
		}
	}

	// 设置事件处理器
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		pcm.handleConnectionStateChange(clientID, state)
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		pcm.handleICEConnectionStateChange(clientID, state)
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		pcm.logger.Printf("Data channel created for client %s: %s", clientID, dc.Label())
	})

	return pc, nil
}

// handleConnectionStateChange 处理连接状态变化
func (pcm *PeerConnectionManager) handleConnectionStateChange(clientID string, state webrtc.PeerConnectionState) {
	pcm.logger.Printf("Client %s connection state changed: %s", clientID, state.String())

	pcm.mutex.RLock()
	info, exists := pcm.connections[clientID]
	pcm.mutex.RUnlock()

	if exists {
		info.mutex.Lock()
		info.State = state
		info.LastActive = time.Now()

		// 根据状态更新健康状态
		switch state {
		case webrtc.PeerConnectionStateConnected:
			info.IsHealthy = true
			info.ReconnectAttempts = 0 // 重置重连计数
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			info.IsHealthy = false
		}
		info.mutex.Unlock()

		// 调用状态变化回调
		pcm.mutex.RLock()
		for _, callback := range pcm.stateChangeCallbacks {
			go callback(clientID, state)
		}
		pcm.mutex.RUnlock()

		// 如果连接失败或关闭，标记为需要清理
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			pcm.logger.Printf("Connection for client %s will be cleaned up due to state: %s", clientID, state.String())
		}
	}
}

// handleICEConnectionStateChange 处理ICE连接状态变化
func (pcm *PeerConnectionManager) handleICEConnectionStateChange(clientID string, state webrtc.ICEConnectionState) {
	pcm.logger.Printf("Client %s ICE connection state changed: %s", clientID, state.String())

	pcm.mutex.RLock()
	info, exists := pcm.connections[clientID]
	pcm.mutex.RUnlock()

	if exists {
		info.mutex.Lock()
		info.ICEState = state
		info.LastActive = time.Now()

		// 根据ICE状态更新健康状态
		switch state {
		case webrtc.ICEConnectionStateConnected, webrtc.ICEConnectionStateCompleted:
			info.IsHealthy = true
		case webrtc.ICEConnectionStateFailed, webrtc.ICEConnectionStateDisconnected:
			info.IsHealthy = false
		}
		info.mutex.Unlock()
	}
}

// RegisterStateChangeCallback 注册状态变化回调
func (pcm *PeerConnectionManager) RegisterStateChangeCallback(name string, callback func(clientID string, state webrtc.PeerConnectionState)) {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()
	pcm.stateChangeCallbacks[name] = callback
}

// UnregisterStateChangeCallback 取消注册状态变化回调
func (pcm *PeerConnectionManager) UnregisterStateChangeCallback(name string) {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()
	delete(pcm.stateChangeCallbacks, name)
}

// GetConnectionHealth 获取连接健康状态
func (pcm *PeerConnectionManager) GetConnectionHealth(clientID string) (*ConnectionHealthStatus, error) {
	pcm.mutex.RLock()
	info, exists := pcm.connections[clientID]
	pcm.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("connection not found for client %s", clientID)
	}

	info.mutex.RLock()
	defer info.mutex.RUnlock()

	status := &ConnectionHealthStatus{
		ClientID:      clientID,
		IsHealthy:     info.IsHealthy,
		State:         info.State,
		ICEState:      info.ICEState,
		LastActive:    info.LastActive,
		ConnectedFor:  time.Since(info.CreatedAt),
		BytesSent:     info.BytesSent,
		BytesReceived: info.BytesReceived,
		PacketsLost:   info.PacketsLost,
		RTT:           info.RTT,
	}

	if info.LastError != nil {
		status.LastError = info.LastError.Error()
	}

	return status, nil
}

// GetAllConnectionsHealth 获取所有连接的健康状态
func (pcm *PeerConnectionManager) GetAllConnectionsHealth() map[string]*ConnectionHealthStatus {
	pcm.mutex.RLock()
	defer pcm.mutex.RUnlock()

	result := make(map[string]*ConnectionHealthStatus)

	for clientID := range pcm.connections {
		if health, err := pcm.GetConnectionHealth(clientID); err == nil {
			result[clientID] = health
		}
	}

	return result
}

// GetMetrics 获取连接指标
func (pcm *PeerConnectionManager) GetMetrics() *ConnectionMetrics {
	pcm.connectionMetrics.mutex.RLock()
	defer pcm.connectionMetrics.mutex.RUnlock()

	// 更新当前活跃连接数
	pcm.mutex.RLock()
	activeCount := int64(len(pcm.connections))
	pcm.mutex.RUnlock()

	return &ConnectionMetrics{
		TotalConnections:  pcm.connectionMetrics.TotalConnections,
		ActiveConnections: activeCount,
		FailedConnections: pcm.connectionMetrics.FailedConnections,
		ReconnectAttempts: pcm.connectionMetrics.ReconnectAttempts,
		CleanupOperations: pcm.connectionMetrics.CleanupOperations,
		AverageConnTime:   pcm.connectionMetrics.AverageConnTime,
	}
}

// SetConnectionTimeout 设置连接超时时间
func (pcm *PeerConnectionManager) SetConnectionTimeout(timeout time.Duration) {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()
	pcm.connectionTimeout = timeout
	pcm.logger.Printf("Connection timeout set to %v", timeout)
}

// SetMaxConnections 设置最大连接数
func (pcm *PeerConnectionManager) SetMaxConnections(max int) {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()
	pcm.maxConnections = max
	pcm.logger.Printf("Maximum connections set to %d", max)
}

// Close 关闭所有连接
func (pcm *PeerConnectionManager) Close() error {
	pcm.cancel() // 停止清理协程

	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()

	for clientID, info := range pcm.connections {
		info.PC.Close()
		pcm.logger.Printf("Closed peer connection for client %s", clientID)
	}

	pcm.connections = make(map[string]*PeerConnectionInfo)
	pcm.logger.Printf("PeerConnection manager closed")
	return nil
}

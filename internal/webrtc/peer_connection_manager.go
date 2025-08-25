package webrtc

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/pion/webrtc/v4"
	"github.com/sirupsen/logrus"
)

// PeerConnectionManager 管理WebRTC对等连接
type PeerConnectionManager struct {
	config               *webrtc.Configuration
	mediaStream          MediaStream
	connections          map[string]*PeerConnectionInfo
	mutex                sync.RWMutex
	logger               *logrus.Entry
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
	ICEGatheringState webrtc.ICEGatheringState
	ReconnectAttempts int
	LastError         error
	IsHealthy         bool
	BytesSent         uint64
	BytesReceived     uint64
	PacketsLost       uint32
	RTT               time.Duration
	CandidateCount    int // ICE候选数量计数器
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

	webrtcConfig := &webrtc.Configuration{
		ICEServers: iceServers,
		// 为WSL2环境优化ICE配置
		ICETransportPolicy: webrtc.ICETransportPolicyAll,
		BundlePolicy:       webrtc.BundlePolicyMaxBundle,
		RTCPMuxPolicy:      webrtc.RTCPMuxPolicyRequire,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 获取 logrus entry 用于结构化日志记录
	logrusLogger := config.GetLoggerWithPrefix("peer-connection-manager")

	pcm := &PeerConnectionManager{
		config:               webrtcConfig,
		mediaStream:          mediaStream,
		connections:          make(map[string]*PeerConnectionInfo),
		logger:               logrusLogger,
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
		pcm.logger.Infof("Replacing existing connection for client %s", clientID)
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
		pcm.logger.Errorf("PeerConnection creation failed for client %s: %v", clientID, err)
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
		ICEGatheringState: webrtc.ICEGatheringStateNew,
		ReconnectAttempts: 0,
		IsHealthy:         true,
		BytesSent:         0,
		BytesReceived:     0,
		PacketsLost:       0,
		RTT:               0,
		CandidateCount:    0,
	}

	// 更新指标
	pcm.connectionMetrics.mutex.Lock()
	pcm.connectionMetrics.TotalConnections++
	pcm.connectionMetrics.ActiveConnections++
	pcm.connectionMetrics.mutex.Unlock()

	pcm.logger.Infof("PeerConnection management completed for client %s (total: %d, active: %d)",
		clientID, pcm.connectionMetrics.TotalConnections, len(pcm.connections))

	// 启动ICE收集超时监控
	go pcm.monitorICECollectionTimeout(clientID)

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
		pcm.logger.Infof("Removed peer connection for client %s", clientID)
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
			pcm.logger.Infof("Cleaning up inactive connection for client %s", clientID)
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
				pcm.logger.Info("PeerConnection manager cleanup routine shutting down")
				return
			}
		}
	}()

	pcm.logger.Info("PeerConnection manager cleanup routine started")
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
			pcm.logger.Warnf("Connection %s marked as unhealthy due to timeout (last active: %v ago)",
				info.ID, now.Sub(info.LastActive))
		}

		// 检查连接状态
		if info.State == webrtc.PeerConnectionStateFailed ||
			info.State == webrtc.PeerConnectionStateClosed ||
			info.ICEState == webrtc.ICEConnectionStateFailed ||
			info.ICEState == webrtc.ICEConnectionStateDisconnected {
			info.IsHealthy = false
			pcm.logger.Warnf("Connection %s marked as unhealthy due to state (PC: %s, ICE: %s)",
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
			pcm.logger.Infof("Cleaning up unhealthy connection for client %s", clientID)

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
				pcm.logger.Infof("Permanently removed connection for client %s after %d failed reconnect attempts",
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

	pcm.logger.Infof("Attempting reconnection for client %s (attempt %d/%d)",
		clientID, info.ReconnectAttempts, pcm.reconnectAttempts)

	// 延迟重连
	go func() {
		time.Sleep(pcm.reconnectDelay)

		// 创建新的PeerConnection
		newPC, err := pcm.createNewPeerConnection(clientID)
		if err != nil {
			pcm.logger.Errorf("Failed to recreate PeerConnection for client %s: %v", clientID, err)
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

			pcm.logger.Infof("Successfully reconnected client %s", clientID)

			pcm.connectionMetrics.mutex.Lock()
			pcm.connectionMetrics.ReconnectAttempts++
			pcm.connectionMetrics.mutex.Unlock()
		}
		pcm.mutex.Unlock()
	}()
}

// createNewPeerConnection 创建新的PeerConnection（内部方法）
func (pcm *PeerConnectionManager) createNewPeerConnection(clientID string) (*webrtc.PeerConnection, error) {
	pcm.logger.Infof("Creating PeerConnection for client %s", clientID)
	pcm.logger.Trace("Step 1: Creating base PeerConnection instance")

	pc, err := webrtc.NewPeerConnection(*pcm.config)
	if err != nil {
		pcm.logger.Errorf("Failed to create PeerConnection for client %s: %v", clientID, err)
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}
	pcm.logger.Debug("Base PeerConnection created successfully")

	// MediaStream 状态验证和轨道添加
	pcm.logger.Trace("Step 2: Validating MediaStream and adding tracks")

	if pcm.mediaStream == nil {
		pcm.logger.Debug("MediaStream is nil, skipping track addition")
		pcm.logger.Info("PeerConnection created successfully without media tracks")
		pcm.setupEventHandlers(pc, clientID)
		return pc, nil
	}

	pcm.logger.Debug("MediaStream validation: MediaStream is available")

	// 获取轨道统计信息用于调试
	stats := pcm.mediaStream.GetStats()
	pcm.logger.Debugf("MediaStream statistics: %+v", stats)

	tracksAdded := 0
	var trackAdditionErrors []string

	// 添加视频轨道
	pcm.logger.Trace("Step 2a: Processing video track")
	videoTrack := pcm.mediaStream.GetVideoTrack()
	if videoTrack != nil {
		pcm.logger.Debugf("Video track found - ID: %s, MimeType: %s",
			videoTrack.ID(), videoTrack.Codec().MimeType)
		pcm.logger.Trace("Adding video track to PeerConnection")

		sender, err := pc.AddTrack(videoTrack)
		if err != nil {
			errorMsg := fmt.Sprintf("failed to add video track: %v", err)
			trackAdditionErrors = append(trackAdditionErrors, errorMsg)
			pcm.logger.Errorf("Video track addition failed for client %s: %v", clientID, err)
		} else {
			tracksAdded++
			pcm.logger.Debugf("Video track added successfully, Sender available: %v", sender != nil)
			pcm.logger.Tracef("Video track Sender details: %+v", sender)
		}
	} else {
		pcm.logger.Debug("Video track not available")
	}

	// 添加音频轨道
	pcm.logger.Trace("Step 2b: Processing audio track")
	audioTrack := pcm.mediaStream.GetAudioTrack()
	if audioTrack != nil {
		pcm.logger.Debugf("Audio track found - ID: %s, MimeType: %s",
			audioTrack.ID(), audioTrack.Codec().MimeType)
		pcm.logger.Trace("Adding audio track to PeerConnection")

		sender, err := pc.AddTrack(audioTrack)
		if err != nil {
			errorMsg := fmt.Sprintf("failed to add audio track: %v", err)
			trackAdditionErrors = append(trackAdditionErrors, errorMsg)
			pcm.logger.Errorf("Audio track addition failed for client %s: %v", clientID, err)
		} else {
			tracksAdded++
			pcm.logger.Debugf("Audio track added successfully, Sender available: %v", sender != nil)
			pcm.logger.Tracef("Audio track Sender details: %+v", sender)
		}
	} else {
		pcm.logger.Debug("Audio track not available")
	}

	// 轨道添加结果摘要
	if len(trackAdditionErrors) > 0 {
		// 如果有轨道添加失败，关闭连接并返回错误
		pc.Close()
		errorSummary := fmt.Sprintf("Track addition failed: %v", trackAdditionErrors)
		pcm.logger.Errorf("PeerConnection creation failed for client %s due to track addition errors: %s", clientID, errorSummary)
		return nil, fmt.Errorf("failed to add media tracks: %s", errorSummary)
	}

	pcm.logger.Infof("PeerConnection created successfully for client %s with %d media tracks", clientID, tracksAdded)
	pcm.logger.Debugf("Track addition summary - Video: %v, Audio: %v",
		videoTrack != nil, audioTrack != nil)

	// 设置事件处理器
	pcm.logger.Trace("Step 3: Setting up event handlers")
	pcm.setupEventHandlers(pc, clientID)
	pcm.logger.Debug("Event handlers configured successfully")

	return pc, nil
}

// setupEventHandlers 设置PeerConnection事件处理器
func (pcm *PeerConnectionManager) setupEventHandlers(pc *webrtc.PeerConnection, clientID string) {
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		pcm.handleConnectionStateChange(clientID, state)
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		pcm.handleICEConnectionStateChange(clientID, state)
	})

	pc.OnICEGatheringStateChange(func(state webrtc.ICEGatheringState) {
		pcm.handleICEGatheringStateChange(clientID, state)
	})

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		pcm.handleICECandidate(clientID, candidate)
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		pcm.logger.Debugf("Data channel created for client %s: %s", clientID, dc.Label())
	})
}

// handleConnectionStateChange 处理连接状态变化
func (pcm *PeerConnectionManager) handleConnectionStateChange(clientID string, state webrtc.PeerConnectionState) {
	pcm.logger.Infof("Client %s connection state changed: %s", clientID, state.String())

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
			pcm.logger.Warnf("Connection for client %s will be cleaned up due to state: %s", clientID, state.String())
		}
	}
}

// handleICEConnectionStateChange 处理ICE连接状态变化
func (pcm *PeerConnectionManager) handleICEConnectionStateChange(clientID string, state webrtc.ICEConnectionState) {
	pcm.logger.Debugf("ICE connection state changed for client %s: %s", clientID, state.String())

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

// handleICEGatheringStateChange 处理ICE收集状态变化
func (pcm *PeerConnectionManager) handleICEGatheringStateChange(clientID string, state webrtc.ICEGatheringState) {
	pcm.mutex.RLock()
	info, exists := pcm.connections[clientID]
	pcm.mutex.RUnlock()

	if !exists {
		pcm.logger.Warnf("ICE gathering state change for unknown client %s: %s", clientID, state.String())
		return
	}

	// 更新连接状态和活跃时间
	info.mutex.Lock()
	oldState := info.ICEGatheringState
	info.ICEGatheringState = state
	info.LastActive = time.Now()
	info.mutex.Unlock()

	// 记录状态转换
	pcm.logger.Debugf("ICE gathering state transition for client %s: %s -> %s", clientID, oldState.String(), state.String())

	switch state {
	case webrtc.ICEGatheringStateNew:
		pcm.logger.Debugf("ICE gathering initialized for client %s", clientID)
	case webrtc.ICEGatheringStateGathering:
		pcm.logger.Infof("ICE collection started for client %s", clientID)
	case webrtc.ICEGatheringStateComplete:
		// 获取最终候选数量
		info.mutex.RLock()
		finalCount := info.CandidateCount
		info.mutex.RUnlock()
		pcm.logger.Infof("ICE collection completed for client %s with %d candidates", clientID, finalCount)
	default:
		pcm.logger.Debugf("ICE gathering state changed for client %s: %s", clientID, state.String())
	}
}

// handleICECandidate 处理ICE候选生成
func (pcm *PeerConnectionManager) handleICECandidate(clientID string, candidate *webrtc.ICECandidate) {
	pcm.mutex.RLock()
	info, exists := pcm.connections[clientID]
	pcm.mutex.RUnlock()

	if !exists {
		pcm.logger.Warnf("ICE candidate generated for unknown client %s", clientID)
		return
	}

	if candidate != nil {
		// 增加候选计数器并更新活跃时间
		info.mutex.Lock()
		info.CandidateCount++
		currentCount := info.CandidateCount
		info.LastActive = time.Now()
		info.mutex.Unlock()

		// 记录详细的ICE候选信息
		pcm.logger.Tracef("ICE candidate generated for client %s (#%d) - Type: %s, Protocol: %s, Address: %s, Port: %d",
			clientID, currentCount, candidate.Typ.String(), candidate.Protocol.String(), candidate.Address, candidate.Port)

		// 记录候选数量摘要（每5个候选记录一次）
		if currentCount%5 == 0 {
			pcm.logger.Infof("ICE candidate progress for client %s: %d candidates generated", clientID, currentCount)
		}
	} else {
		// ICE收集完成
		info.mutex.RLock()
		finalCount := info.CandidateCount
		info.mutex.RUnlock()

		pcm.logger.Infof("ICE collection completed for client %s - total %d candidates generated", clientID, finalCount)
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
	pcm.logger.Infof("Connection timeout set to %v", timeout)
}

// SetMaxConnections 设置最大连接数
func (pcm *PeerConnectionManager) SetMaxConnections(max int) {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()
	pcm.maxConnections = max
	pcm.logger.Infof("Maximum connections set to %d", max)
}

// HandleICECandidate 处理ICE候选
func (pcm *PeerConnectionManager) HandleICECandidate(clientID string, candidateData map[string]interface{}) error {
	startTime := time.Now()

	pcm.mutex.RLock()
	info, exists := pcm.connections[clientID]
	pcm.mutex.RUnlock()

	if !exists {
		pcm.logger.Errorf("ICE candidate processing failed: peer connection not found for client %s", clientID)
		return fmt.Errorf("peer connection not found for client %s", clientID)
	}

	// 详细验证候选数据
	candidate, ok := candidateData["candidate"].(string)
	if !ok {
		pcm.logger.Errorf("ICE candidate processing failed for client %s: candidate field is missing or not a string", clientID)
		return fmt.Errorf("invalid candidate data: candidate field is required and must be a string")
	}

	if candidate == "" {
		pcm.logger.Errorf("ICE candidate processing failed for client %s: candidate field is empty", clientID)
		return fmt.Errorf("invalid candidate data: candidate field cannot be empty")
	}

	// 获取可选的SDP信息并记录详细信息
	sdpMid, hasSdpMid := candidateData["sdpMid"].(string)
	sdpMLineIndex := uint16(0)
	hasSdpMLineIndex := false
	if idx, ok := candidateData["sdpMLineIndex"].(float64); ok {
		sdpMLineIndex = uint16(idx)
		hasSdpMLineIndex = true
	}

	// 记录详细的ICE候选信息
	pcm.logger.Debugf("Processing ICE candidate for client %s: candidate='%s', sdpMid='%s' (present: %t), sdpMLineIndex=%d (present: %t)",
		clientID, candidate, sdpMid, hasSdpMid, sdpMLineIndex, hasSdpMLineIndex)

	// 创建ICE候选
	iceCandidate := webrtc.ICECandidateInit{
		Candidate:     candidate,
		SDPMid:        &sdpMid,
		SDPMLineIndex: &sdpMLineIndex,
	}

	// 检查连接状态
	info.mutex.RLock()
	currentState := info.State
	currentICEState := info.ICEState
	info.mutex.RUnlock()

	pcm.logger.Debugf("Connection state before ICE candidate processing for client %s: PC=%s, ICE=%s",
		clientID, currentState, currentICEState)

	// 更新连接活跃时间（无论处理是否成功）
	info.mutex.Lock()
	info.LastActive = time.Now()
	info.mutex.Unlock()

	// 添加ICE候选到PeerConnection
	err := info.PC.AddICECandidate(iceCandidate)
	processingTime := time.Since(startTime)

	if err != nil {
		// 详细的错误报告
		pcm.logger.Errorf("ICE candidate processing failed for client %s after %v: %v", clientID, processingTime, err)
		pcm.logger.Errorf("Failed candidate details: candidate='%s', sdpMid='%s', sdpMLineIndex=%d", candidate, sdpMid, sdpMLineIndex)
		pcm.logger.Errorf("Connection state during failure: PC=%s, ICE=%s", currentState, currentICEState)

		// 检查是否是网络问题
		if isNetworkError(err) {
			pcm.logger.Warnf("Network connectivity issue detected for client %s: %v", clientID, err)
		}

		// 更新错误统计
		info.mutex.Lock()
		info.LastError = err
		info.mutex.Unlock()

		return fmt.Errorf("failed to add ICE candidate: %w", err)
	}

	// 成功处理的详细日志
	pcm.logger.Infof("ICE candidate processed successfully for client %s in %v", clientID, processingTime)
	pcm.logger.Debugf("Processed candidate details: candidate='%s', sdpMid='%s', sdpMLineIndex=%d", candidate, sdpMid, sdpMLineIndex)

	// 检查处理后的连接状态
	info.mutex.RLock()
	newState := info.State
	newICEState := info.ICEState
	info.mutex.RUnlock()

	if newState != currentState || newICEState != currentICEState {
		pcm.logger.Infof("Connection state changed for client %s after ICE candidate: PC=%s->%s, ICE=%s->%s",
			clientID, currentState, newState, currentICEState, newICEState)
	}

	return nil
}

// isNetworkError 检查是否是网络相关错误
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	errorStr := strings.ToLower(err.Error())
	networkKeywords := []string{
		"network", "connection", "timeout", "unreachable",
		"refused", "reset", "broken pipe", "no route",
	}

	for _, keyword := range networkKeywords {
		if strings.Contains(errorStr, keyword) {
			return true
		}
	}

	return false
}

// HandleMultipleICECandidates 批量处理多个ICE候选
func (pcm *PeerConnectionManager) HandleMultipleICECandidates(clientID string, candidates []map[string]interface{}) error {
	startTime := time.Now()

	pcm.mutex.RLock()
	info, exists := pcm.connections[clientID]
	pcm.mutex.RUnlock()

	if !exists {
		pcm.logger.Errorf("Batch ICE candidate processing failed: peer connection not found for client %s", clientID)
		return fmt.Errorf("peer connection not found for client %s", clientID)
	}

	pcm.logger.Infof("Starting batch processing of %d ICE candidates for client %s", len(candidates), clientID)

	successCount := 0
	var lastError error
	var failedCandidates []int

	for i, candidateData := range candidates {
		candidateStartTime := time.Now()
		err := pcm.HandleICECandidate(clientID, candidateData)
		candidateProcessingTime := time.Since(candidateStartTime)

		if err != nil {
			pcm.logger.Errorf("Failed to process ICE candidate %d/%d for client %s in %v: %v",
				i+1, len(candidates), clientID, candidateProcessingTime, err)
			lastError = err
			failedCandidates = append(failedCandidates, i)
		} else {
			successCount++
			pcm.logger.Debugf("Successfully processed ICE candidate %d/%d for client %s in %v",
				i+1, len(candidates), clientID, candidateProcessingTime)
		}
	}

	// 更新连接活跃时间
	info.mutex.Lock()
	info.LastActive = time.Now()
	info.mutex.Unlock()

	totalProcessingTime := time.Since(startTime)

	// 详细的批处理结果报告
	if successCount == len(candidates) {
		pcm.logger.Infof("All %d ICE candidates processed successfully for client %s in %v (avg: %v per candidate)",
			len(candidates), clientID, totalProcessingTime, totalProcessingTime/time.Duration(len(candidates)))
	} else if successCount > 0 {
		pcm.logger.Warnf("Partial success: %d/%d ICE candidates processed for client %s in %v (failed indices: %v)",
			successCount, len(candidates), clientID, totalProcessingTime, failedCandidates)
	} else {
		pcm.logger.Errorf("Complete failure: 0/%d ICE candidates processed for client %s in %v",
			len(candidates), clientID, totalProcessingTime)
	}

	// 如果有任何候选处理失败，返回最后一个错误
	if lastError != nil && successCount == 0 {
		return fmt.Errorf("failed to process any ICE candidates: %w", lastError)
	}

	return nil
}

// monitorICECollectionTimeout 监控ICE收集超时
func (pcm *PeerConnectionManager) monitorICECollectionTimeout(clientID string) {
	// ICE收集超时时间设置为30秒
	timeout := 30 * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		// 检查ICE收集是否完成
		pcm.mutex.RLock()
		info, exists := pcm.connections[clientID]
		pcm.mutex.RUnlock()

		if !exists {
			return // 连接已被清理
		}

		info.mutex.RLock()
		gatheringState := info.ICEGatheringState
		candidateCount := info.CandidateCount
		info.mutex.RUnlock()

		if gatheringState != webrtc.ICEGatheringStateComplete {
			if candidateCount == 0 {
				pcm.logger.Errorf("ICE collection failed for client %s: no candidates generated within %v", clientID, timeout)
			} else {
				pcm.logger.Warnf("ICE collection timeout for client %s: %d candidates generated but collection not completed within %v",
					clientID, candidateCount, timeout)
			}
		}

	case <-pcm.ctx.Done():
		return // 管理器正在关闭
	}
}

// Close 关闭所有连接
func (pcm *PeerConnectionManager) Close() error {
	pcm.cancel() // 停止清理协程

	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()

	for clientID, info := range pcm.connections {
		info.PC.Close()
		pcm.logger.Infof("Closed peer connection for client %s", clientID)
	}

	pcm.connections = make(map[string]*PeerConnectionInfo)
	pcm.logger.Info("PeerConnection manager closed")
	return nil
}

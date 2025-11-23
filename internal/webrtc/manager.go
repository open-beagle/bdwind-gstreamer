package webrtc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/common/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/common/events"
	webrtcEvents "github.com/open-beagle/bdwind-gstreamer/internal/webrtc/events"
)

// WebRTCManager WebRTC管理器 - 参考Selkies设计
// 专注于核心WebRTC功能：peer connection管理和视频数据发送
type WebRTCManager struct {
	config *config.WebRTCConfig
	logger *logrus.Entry

	// WebRTC核心组件
	peerConnection *webrtc.PeerConnection
	videoTrack     *webrtc.TrackLocalStaticSample

	// ICE candidate处理
	iceCandidates []webrtc.ICECandidate

	// 状态管理
	running   bool
	startTime time.Time
	mutex     sync.RWMutex

	// 上下文控制
	ctx    context.Context
	cancel context.CancelFunc

	// 事件总线
	eventBus events.EventBus

	currentSessionID string
	pcSessionID      string // 当前PeerConnection关联的会话ID

	// 会话管理
	activeSessions map[string]bool // 活跃会话集合
	sessionMutex   sync.RWMutex
	reconnectTimer *time.Timer
	idleTimer      *time.Timer

	// 统计信息
	videoFrameCount uint64
}

// NewWebRTCManager 创建WebRTC管理器
// 接受WebRTC配置并初始化基本字段
func NewWebRTCManager(cfg *config.WebRTCConfig) (*WebRTCManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("WebRTC config is required")
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid WebRTC config: %w", err)
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())

	manager := &WebRTCManager{
		config:         cfg,
		logger:         logrus.WithField("component", "webrtc"),
		iceCandidates:  make([]webrtc.ICECandidate, 0),
		activeSessions: make(map[string]bool),
		ctx:            ctx,
		cancel:         cancel,
	}

	manager.logger.Debug("WebRTCManager created successfully")
	return manager, nil
}

// NewWebRTCManagerFromSimpleConfig creates a new WebRTC manager from SimpleConfig
func NewWebRTCManagerFromSimpleConfig(cfg *config.SimpleConfig) (*WebRTCManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	// Get WebRTC config with direct access (no validation)
	webrtcConfig := cfg.GetWebRTCConfig()

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())

	// Create logger using simple config
	logger := cfg.GetLoggerWithPrefix("webrtc-minimal")

	manager := &WebRTCManager{
		config:         webrtcConfig,
		logger:         logger,
		iceCandidates:  make([]webrtc.ICECandidate, 0),
		activeSessions: make(map[string]bool),
		ctx:            ctx,
		cancel:         cancel,
	}

	manager.logger.Debug("WebRTCManager created from SimpleConfig")
	return manager, nil
}

// SetEventBus 设置事件总线
func (m *WebRTCManager) SetEventBus(bus events.EventBus) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.eventBus = bus
}

// SetCurrentSessionID 设置当前会话ID
func (m *WebRTCManager) SetCurrentSessionID(sessionID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.currentSessionID = sessionID
}

// Start 启动WebRTC管理器
func (m *WebRTCManager) Start(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		m.logger.Debug("WebRTC manager already running")
		return nil
	}

	m.logger.Info("Starting WebRTC manager...")
	m.startTime = time.Now()

	// 不在启动时创建PeerConnection，而是在收到客户端请求时创建
	// PeerConnection会在CreateOffer时按需创建

	m.running = true
	m.logger.Info("WebRTC manager started successfully (PeerConnection will be created on demand)")
	return nil
}

// createPeerConnection 创建和配置WebRTC PeerConnection
func (m *WebRTCManager) createPeerConnection() error {
	// 转换配置中的ICE服务器
	iceServers := m.convertICEServers()

	config := webrtc.Configuration{
		ICEServers: iceServers,
	}

	m.logger.Infof("🔧 Creating PeerConnection with %d ICE servers", len(iceServers))
	for i, server := range iceServers {
		m.logger.Infof("   ICE Server %d: %v", i+1, server.URLs)
	}

	var err error
	m.peerConnection, err = webrtc.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %w", err)
	}

	m.logger.Info("✅ PeerConnection created successfully")
	return nil
}

// convertICEServers 转换配置中的ICE服务器格式
func (m *WebRTCManager) convertICEServers() []webrtc.ICEServer {
	var iceServers []webrtc.ICEServer

	// 从配置中获取ICE服务器
	for _, server := range m.config.ICEServers {
		iceServer := webrtc.ICEServer{
			URLs: server.URLs,
		}

		// 如果有认证信息，添加用户名和密码
		if server.Username != "" {
			iceServer.Username = server.Username
		}
		if server.Credential != "" {
			iceServer.Credential = server.Credential
		}

		iceServers = append(iceServers, iceServer)
	}

	// 如果没有配置ICE服务器，使用默认的
	if len(iceServers) == 0 {
		iceServers = []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		}
		m.logger.Debug("No ICE servers configured, using default Google STUN server")
	} else {
		m.logger.Debugf("Using %d configured ICE servers", len(iceServers))
	}

	return iceServers
}

// recreatePeerConnection 重新创建PeerConnection
func (m *WebRTCManager) recreatePeerConnection() error {
	m.logger.Debug("Recreating PeerConnection...")

	// 清空ICE candidates
	m.mutex.Lock()
	m.iceCandidates = make([]webrtc.ICECandidate, 0)
	m.mutex.Unlock()

	// 关闭现有连接
	if m.peerConnection != nil {
		m.peerConnection.Close()
	}

	// 创建新的PeerConnection
	if err := m.createPeerConnection(); err != nil {
		return fmt.Errorf("failed to create new peer connection: %w", err)
	}

	// 重新创建视频轨道
	if err := m.createVideoTrack(); err != nil {
		return fmt.Errorf("failed to recreate video track: %w", err)
	}

	// 重新设置ICE处理
	m.setupICEHandling()

	m.logger.Debug("PeerConnection recreated successfully")
	return nil
}

// setupICEHandling 设置ICE candidate处理
func (m *WebRTCManager) setupICEHandling() {
	// 设置ICE candidate回调
	m.peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			m.logger.Debugf("New ICE candidate: %s", candidate.String())
			// 存储ICE candidate供后续使用
			m.mutex.Lock()
			m.iceCandidates = append(m.iceCandidates, *candidate)

			// 发布事件
			if m.eventBus != nil {
				// 将 ICECandidateInit 转换为 map[string]interface{}
				candidateInit := candidate.ToJSON()
				candidateStr := candidateInit.Candidate

				m.logger.Infof("📡 Publishing ICE candidate: %s", candidateStr[:min(80, len(candidateStr))])

				candidateMap := map[string]interface{}{
					"candidate": candidateInit.Candidate,
				}
				if candidateInit.SDPMid != nil {
					candidateMap["sdpMid"] = *candidateInit.SDPMid
				}
				if candidateInit.SDPMLineIndex != nil {
					candidateMap["sdpMLineIndex"] = *candidateInit.SDPMLineIndex
				}
				if candidateInit.UsernameFragment != nil {
					candidateMap["usernameFragment"] = *candidateInit.UsernameFragment
				}

				event := webrtcEvents.NewWebRTCEvent(
					webrtcEvents.EventOnICECandidate,
					m.currentSessionID,
					m.currentSessionID, // PeerID same as SessionID for now
					map[string]interface{}{
						"candidate": candidateMap,
					},
				)
				// 异步发布，不阻塞回调
				go m.eventBus.Publish(event)
			}
			m.mutex.Unlock()
		}
	})

	// 设置连接状态变化回调
	m.peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		m.logger.Infof("PeerConnection state changed: %s", state.String())
	})

	// 设置ICE连接状态变化回调
	m.peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		m.logger.Infof("ICE connection state changed: %s", state.String())
		m.handleICEConnectionStateChange(state)
	})
}

// handleICEConnectionStateChange 处理ICE连接状态变化
func (m *WebRTCManager) handleICEConnectionStateChange(state webrtc.ICEConnectionState) {
	if m.eventBus == nil {
		return
	}

	sessionID := m.currentSessionID
	if sessionID == "" {
		return
	}

	switch state {
	case webrtc.ICEConnectionStateConnected:
		m.handleICEConnected(sessionID)
	case webrtc.ICEConnectionStateDisconnected:
		m.handleICEDisconnected(sessionID)
	case webrtc.ICEConnectionStateFailed:
		m.handleICEFailed(sessionID)
	case webrtc.ICEConnectionStateClosed:
		m.handleICEClosed(sessionID)
	}
}

// handleICEConnected 处理ICE连接建立
func (m *WebRTCManager) handleICEConnected(sessionID string) {
	// 取消重连定时器
	if m.reconnectTimer != nil {
		m.reconnectTimer.Stop()
		m.reconnectTimer = nil
	}

	// 取消空闲定时器
	if m.idleTimer != nil {
		m.idleTimer.Stop()
		m.idleTimer = nil
	}

	// 发布会话就绪事件
	event := events.NewBaseEvent(
		events.EventWebRTCSessionReady,
		sessionID,
		map[string]interface{}{
			"session_id": sessionID,
			"client_id":  sessionID,
			"timestamp":  time.Now(),
		},
	)
	go m.eventBus.Publish(event)
}

// handleICEDisconnected 处理ICE断开
func (m *WebRTCManager) handleICEDisconnected(sessionID string) {
	// 发布会话暂停事件
	event := events.NewBaseEvent(
		events.EventWebRTCSessionPaused,
		sessionID,
		map[string]interface{}{
			"session_id": sessionID,
			"client_id":  sessionID,
			"reason":     "disconnected",
			"timestamp":  time.Now(),
		},
	)
	go m.eventBus.Publish(event)

	// 启动重连定时器
	keepAliveTimeout := m.config.Session.KeepAliveTimeout
	m.logger.Infof("ICE disconnected, will wait %s for reconnection", keepAliveTimeout)

	m.reconnectTimer = time.AfterFunc(keepAliveTimeout, func() {
		m.onReconnectTimeout(sessionID)
	})
}

// handleICEFailed 处理ICE失败
func (m *WebRTCManager) handleICEFailed(sessionID string) {
	// 发布会话失败事件
	event := events.NewBaseEvent(
		events.EventWebRTCSessionFailed,
		sessionID,
		map[string]interface{}{
			"session_id": sessionID,
			"client_id":  sessionID,
			"reason":     "ice_failed",
			"timestamp":  time.Now(),
		},
	)
	go m.eventBus.Publish(event)

	// 启动重连定时器（给一个较短的超时）
	m.reconnectTimer = time.AfterFunc(10*time.Second, func() {
		m.onReconnectTimeout(sessionID)
	})
}

// handleICEClosed 处理ICE关闭
func (m *WebRTCManager) handleICEClosed(sessionID string) {
	// 取消所有定时器
	if m.reconnectTimer != nil {
		m.reconnectTimer.Stop()
		m.reconnectTimer = nil
	}
	if m.idleTimer != nil {
		m.idleTimer.Stop()
		m.idleTimer = nil
	}

	// 移除会话
	m.removeSession(sessionID)
}

// onReconnectTimeout 重连超时处理
func (m *WebRTCManager) onReconnectTimeout(sessionID string) {
	m.logger.Warnf("Reconnect timeout for session %s", sessionID)

	// 发布会话超时事件
	event := events.NewBaseEvent(
		events.EventWebRTCSessionTimeout,
		sessionID,
		map[string]interface{}{
			"session_id": sessionID,
			"client_id":  sessionID,
			"duration":   m.config.Session.KeepAliveTimeout,
			"timestamp":  time.Now(),
		},
	)

	if m.eventBus != nil {
		go m.eventBus.Publish(event)
	}

	// 移除会话
	m.removeSession(sessionID)
}

// Stop 停止WebRTC
func (m *WebRTCManager) Stop(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running {
		return nil
	}

	if m.peerConnection != nil {
		m.peerConnection.Close()
	}

	m.running = false
	m.logger.Info("WebRTC stopped")
	return nil
}

// createVideoTrack 创建WebRTC视频轨道
func (m *WebRTCManager) createVideoTrack() error {
	m.logger.Debug("Creating video track...")

	// 创建H.264视频轨道
	var err error
	m.videoTrack, err = webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"bdwind-gstreamer",
	)
	if err != nil {
		return fmt.Errorf("failed to create video track: %w", err)
	}

	// 添加轨道到PeerConnection
	if _, err = m.peerConnection.AddTrack(m.videoTrack); err != nil {
		return fmt.Errorf("failed to add video track to peer connection: %w", err)
	}

	m.logger.Debug("Video track created and added to peer connection")
	return nil
}

// SendVideoData 实现 VideoDataSink 接口
// 发送视频数据 - 直接接收来自GStreamer的编码数据
func (m *WebRTCManager) SendVideoData(data []byte, timestamp time.Duration) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.logger.Debugf("SendVideoData called with %d bytes", len(data))

	// 如果WebRTC未启动，静默忽略（GStreamer可能先启动）
	// 等WebRTC启动后，后续的帧会正常发送
	if !m.running {
		return nil // 静默忽略，不返回错误
	}

	// 如果videoTrack不可用（没有客户端连接），静默忽略
	// 这样GStreamer可以继续运行，等客户端连接后再发送数据
	if m.videoTrack == nil {
		return nil // 静默忽略，不返回错误
	}

	if len(data) == 0 {
		m.logger.Debugf("Empty video data")
		return fmt.Errorf("empty video data")
	}

	// 创建WebRTC sample，使用传入的时间戳
	sample := media.Sample{
		Data:     data,
		Duration: timestamp,
	}

	// 直接发送到WebRTC轨道
	if err := m.videoTrack.WriteSample(sample); err != nil {
		m.logger.Debugf("Failed to write video sample: %v", err)
		return fmt.Errorf("failed to write video sample: %w", err)
	}

	m.logger.Debugf("Successfully wrote %d bytes to video track", len(data))
	return nil
}

// SendVideoDataWithTimestamp 发送带时间戳的视频数据
func (m *WebRTCManager) SendVideoDataWithTimestamp(data []byte, duration time.Duration) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// 如果WebRTC未启动，静默忽略（GStreamer可能先启动）
	// 等WebRTC启动后，后续的帧会正常发送
	if !m.running {
		return nil // 静默忽略，不返回错误
	}

	// 如果videoTrack不可用（没有客户端连接），静默忽略
	// 这样GStreamer可以继续运行，等客户端连接后再发送数据
	if m.videoTrack == nil {
		return nil // 静默忽略，不返回错误
	}

	if len(data) == 0 {
		return fmt.Errorf("empty video data")
	}

	// 添加计数器用于统计
	m.videoFrameCount++

	// 第一帧时打印，确认数据流开始
	if m.videoFrameCount == 1 {
		m.logger.Infof("📹 WebRTC video: first frame sent, size=%d bytes (%d KB)", len(data), len(data)/1024)
	}

	// 每300帧（约10秒）打印一次统计信息
	if m.videoFrameCount%300 == 0 {
		m.logger.Infof("📹 WebRTC video: sent %d frames, current size=%d bytes", m.videoFrameCount, len(data))
	}

	// 创建WebRTC sample with custom duration
	sample := media.Sample{
		Data:     data,
		Duration: duration,
	}

	// 直接发送到WebRTC轨道
	if err := m.videoTrack.WriteSample(sample); err != nil {
		// 只记录前10次错误，避免刷屏
		if m.videoFrameCount <= 10 {
			m.logger.Errorf("❌ Failed to write video sample (frame %d): %v", m.videoFrameCount, err)
		}
		return fmt.Errorf("failed to write video sample: %w", err)
	}

	return nil
}

// GetVideoTrack 获取视频轨道实例
func (m *WebRTCManager) GetVideoTrack() *webrtc.TrackLocalStaticSample {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.videoTrack
}

// CreateOffer 创建SDP offer
func (m *WebRTCManager) CreateOffer() (*webrtc.SessionDescription, error) {
	m.mutex.Lock()

	if !m.running {
		m.mutex.Unlock()
		return nil, fmt.Errorf("WebRTC manager not running")
	}

	// 如果PeerConnection不存在，创建新的
	if m.peerConnection == nil {
		m.logger.Info("PeerConnection not exists, creating new one for this session")
		m.mutex.Unlock()
		if err := m.recreatePeerConnection(); err != nil {
			return nil, fmt.Errorf("failed to create peer connection: %w", err)
		}
		m.mutex.Lock()
		m.pcSessionID = m.currentSessionID
	}

	// 检查当前连接状态
	signalingState := m.peerConnection.SignalingState()
	m.logger.Debugf("Current signaling state: %s", signalingState)

	// 如果已经有 local offer，返回现有的 local description
	if signalingState == webrtc.SignalingStateHaveLocalOffer {
		localDesc := m.peerConnection.LocalDescription()
		if localDesc != nil {
			m.logger.Debug("Returning existing local offer")
			m.mutex.Unlock()
			return localDesc, nil
		}
	}

	// 检查是否需要重建连接
	needsRecreate := false
	if signalingState != webrtc.SignalingStateStable {
		m.logger.Debugf("Signaling state is %s, need to recreate PeerConnection", signalingState)
		needsRecreate = true
	} else if m.pcSessionID != m.currentSessionID {
		m.logger.Infof("Session ID changed from %s to %s, need to recreate PeerConnection", m.pcSessionID, m.currentSessionID)
		needsRecreate = true
	}

	// 如果需要重建，释放锁后执行
	if needsRecreate {
		m.mutex.Unlock()
		if err := m.recreatePeerConnection(); err != nil {
			return nil, fmt.Errorf("failed to recreate peer connection: %w", err)
		}
		m.mutex.Lock()
		// 更新关联的会话ID
		m.pcSessionID = m.currentSessionID
	}

	m.logger.Debug("Creating SDP offer...")

	offer, err := m.peerConnection.CreateOffer(nil)
	if err != nil {
		m.logger.Errorf("Failed to create SDP offer: %v", err)
		m.mutex.Unlock()
		return nil, fmt.Errorf("failed to create offer: %w", err)
	}

	// 设置本地描述
	if err := m.peerConnection.SetLocalDescription(offer); err != nil {
		m.logger.Errorf("Failed to set local description: %v", err)
		m.mutex.Unlock()
		return nil, fmt.Errorf("failed to set local description: %w", err)
	}

	m.logger.Debug("SDP offer created and set as local description")
	m.mutex.Unlock()
	return &offer, nil
}

// CreateAnswer 创建SDP answer
func (m *WebRTCManager) CreateAnswer() (*webrtc.SessionDescription, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if !m.running || m.peerConnection == nil {
		return nil, fmt.Errorf("WebRTC manager not running or peer connection not initialized")
	}

	m.logger.Debug("Creating SDP answer...")

	answer, err := m.peerConnection.CreateAnswer(nil)
	if err != nil {
		m.logger.Errorf("Failed to create SDP answer: %v", err)
		return nil, fmt.Errorf("failed to create answer: %w", err)
	}

	// 设置本地描述
	if err := m.peerConnection.SetLocalDescription(answer); err != nil {
		m.logger.Errorf("Failed to set local description: %v", err)
		return nil, fmt.Errorf("failed to set local description: %w", err)
	}

	m.logger.Debug("SDP answer created and set as local description")
	return &answer, nil
}

// SetRemoteDescription 设置远程SDP描述
func (m *WebRTCManager) SetRemoteDescription(desc webrtc.SessionDescription) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if !m.running || m.peerConnection == nil {
		return fmt.Errorf("WebRTC manager not running or peer connection not initialized")
	}

	m.logger.Debugf("Setting remote description (type: %s)", desc.Type.String())

	if err := m.peerConnection.SetRemoteDescription(desc); err != nil {
		m.logger.Errorf("Failed to set remote description: %v", err)
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	m.logger.Debug("Remote description set successfully")
	return nil
}

// AddICECandidate 添加ICE candidate
func (m *WebRTCManager) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if !m.running || m.peerConnection == nil {
		return fmt.Errorf("WebRTC manager not running or peer connection not initialized")
	}

	m.logger.Debugf("Adding ICE candidate: %s", candidate.Candidate)

	if err := m.peerConnection.AddICECandidate(candidate); err != nil {
		m.logger.Errorf("Failed to add ICE candidate: %v", err)
		return fmt.Errorf("failed to add ICE candidate: %w", err)
	}

	m.logger.Debug("ICE candidate added successfully")
	return nil
}

// GetICECandidates 获取收集到的ICE candidates
func (m *WebRTCManager) GetICECandidates() []webrtc.ICECandidate {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// 返回副本以避免并发修改
	candidates := make([]webrtc.ICECandidate, len(m.iceCandidates))
	copy(candidates, m.iceCandidates)
	return candidates
}

// IsRunning 检查运行状态
func (m *WebRTCManager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.running
}

// GetPeerConnection 获取PeerConnection实例
func (m *WebRTCManager) GetPeerConnection() *webrtc.PeerConnection {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.peerConnection
}

// GetConnectionState 获取连接状态
func (m *WebRTCManager) GetConnectionState() webrtc.PeerConnectionState {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.peerConnection == nil {
		return webrtc.PeerConnectionStateClosed
	}

	return m.peerConnection.ConnectionState()
}

// GetICEConnectionState 获取ICE连接状态
func (m *WebRTCManager) GetICEConnectionState() webrtc.ICEConnectionState {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.peerConnection == nil {
		return webrtc.ICEConnectionStateClosed
	}

	return m.peerConnection.ICEConnectionState()
}

// GetStats 获取基本统计信息
func (m *WebRTCManager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]interface{}{
		"running":    m.running,
		"start_time": m.startTime,
	}

	if m.running {
		stats["uptime"] = time.Since(m.startTime).Seconds()
		stats["connection_state"] = m.GetConnectionState().String()
		stats["ice_connection_state"] = m.GetICEConnectionState().String()
		stats["ice_candidates_count"] = len(m.iceCandidates)
		stats["has_video_track"] = m.videoTrack != nil
	}

	return stats
}

// Compatibility methods for existing app integration

// StartLegacy 启动WebRTC管理器 (无context版本，向后兼容)
func (m *WebRTCManager) StartLegacy() error {
	return m.Start(context.Background())
}

// StopLegacy 停止WebRTC管理器 (无context版本，向后兼容)
func (m *WebRTCManager) StopLegacy() error {
	return m.Stop(context.Background())
}

// IsEnabled 检查是否启用 (兼容性方法)
func (m *WebRTCManager) IsEnabled() bool {
	return true // WebRTCManager 总是启用的
}

// GetContext 获取上下文 (兼容ComponentManager接口)
func (m *WebRTCManager) GetContext() context.Context {
	return m.ctx
}

// GetMediaStream 获取媒体流 (兼容性方法，返回nil直到实现)
func (m *WebRTCManager) GetMediaStream() interface{} {
	// TODO: 在任务2中实现媒体流管理
	// 返回一个具有GetStats方法的临时对象
	return &struct {
		GetStats func() map[string]interface{}
	}{
		GetStats: func() map[string]interface{} {
			return map[string]interface{}{
				"video_frames_sent": 0,
				"video_bytes_sent":  0,
				"audio_frames_sent": 0,
				"audio_bytes_sent":  0,
			}
		},
	}
}

// SetupRoutes 设置路由 (实现ComponentManager接口)
func (m *WebRTCManager) SetupRoutes(router *mux.Router) error {
	// TODO: 在任务2中实现路由设置
	// 目前为空实现以满足接口要求
	m.logger.Debug("SetupRoutes called - will be implemented in task 2")
	return nil
}

// OnClientConnected 客户端连接处理
func (m *WebRTCManager) OnClientConnected(sessionID string) {
	m.sessionMutex.Lock()
	m.activeSessions[sessionID] = true
	sessionCount := len(m.activeSessions)
	m.sessionMutex.Unlock()

	m.logger.Infof("Client connected (session=%s), active sessions: %d", sessionID, sessionCount)

	// 取消空闲定时器
	if m.idleTimer != nil {
		m.idleTimer.Stop()
		m.idleTimer = nil
	}

	// 发布会话开始事件
	if m.eventBus != nil {
		event := events.NewBaseEvent(
			events.EventWebRTCSessionStarted,
			sessionID,
			map[string]interface{}{
				"session_id": sessionID,
				"client_id":  sessionID,
				"timestamp":  time.Now(),
			},
		)
		go m.eventBus.Publish(event)
	}
}

// OnClientDisconnected 客户端断开处理
func (m *WebRTCManager) OnClientDisconnected(sessionID string) {
	m.removeSession(sessionID)
}

// removeSession 移除会话
func (m *WebRTCManager) removeSession(sessionID string) {
	m.sessionMutex.Lock()
	delete(m.activeSessions, sessionID)
	sessionCount := len(m.activeSessions)
	m.sessionMutex.Unlock()

	m.logger.Infof("Session removed (session=%s), active sessions: %d", sessionID, sessionCount)

	// 发布会话结束事件
	if m.eventBus != nil {
		event := events.NewBaseEvent(
			events.EventWebRTCSessionEnded,
			sessionID,
			map[string]interface{}{
				"session_id":      sessionID,
				"client_id":       sessionID,
				"active_sessions": sessionCount,
				"timestamp":       time.Now(),
			},
		)
		go m.eventBus.Publish(event)
	}

	// 如果无活跃会话，启动空闲定时器
	if sessionCount == 0 {
		m.startIdleTimer(sessionID)
	}
}

// startIdleTimer 启动空闲定时器
func (m *WebRTCManager) startIdleTimer(lastSessionID string) {
	// 默认5秒空闲超时
	idleTimeout := 5 * time.Second

	m.logger.Infof("No active sessions, will stop streaming after %s", idleTimeout)

	m.idleTimer = time.AfterFunc(idleTimeout, func() {
		m.sessionMutex.RLock()
		sessionCount := len(m.activeSessions)
		m.sessionMutex.RUnlock()

		// 再次检查是否有新会话
		if sessionCount == 0 {
			m.logger.Info("Idle timeout reached, publishing no active sessions event")

			if m.eventBus != nil {
				event := events.NewBaseEvent(
					events.EventWebRTCNoActiveSessions,
					lastSessionID,
					map[string]interface{}{
						"last_session_id": lastSessionID,
						"idle_duration":   idleTimeout,
						"timestamp":       time.Now(),
					},
				)
				go m.eventBus.Publish(event)
			}
		} else {
			m.logger.Debugf("New sessions connected during idle period, canceling shutdown")
		}
	})
}

// GetActiveSessionCount 获取活跃会话数
func (m *WebRTCManager) GetActiveSessionCount() int {
	m.sessionMutex.RLock()
	defer m.sessionMutex.RUnlock()
	return len(m.activeSessions)
}

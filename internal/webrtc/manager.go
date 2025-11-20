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

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
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
		config:        cfg,
		logger:        logrus.WithField("component", "webrtc"),
		iceCandidates: make([]webrtc.ICECandidate, 0),
		ctx:           ctx,
		cancel:        cancel,
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
		config:        webrtcConfig,
		logger:        logger,
		iceCandidates: make([]webrtc.ICECandidate, 0),
		ctx:           ctx,
		cancel:        cancel,
	}

	manager.logger.Debug("WebRTCManager created from SimpleConfig")
	return manager, nil
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

	// 创建并配置PeerConnection
	if err := m.createPeerConnection(); err != nil {
		return fmt.Errorf("failed to create peer connection: %w", err)
	}

	// 创建视频轨道
	if err := m.createVideoTrack(); err != nil {
		return fmt.Errorf("failed to create video track: %w", err)
	}

	// 设置ICE candidate处理
	m.setupICEHandling()

	m.running = true
	m.logger.Info("WebRTC manager started successfully")
	return nil
}

// createPeerConnection 创建和配置WebRTC PeerConnection
func (m *WebRTCManager) createPeerConnection() error {
	// 转换配置中的ICE服务器
	iceServers := m.convertICEServers()

	config := webrtc.Configuration{
		ICEServers: iceServers,
	}

	m.logger.Debugf("Creating PeerConnection with %d ICE servers", len(iceServers))

	var err error
	m.peerConnection, err = webrtc.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %w", err)
	}

	m.logger.Debug("PeerConnection created successfully")
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
	})
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

// SendVideoData 发送视频数据 - 直接接收来自GStreamer的编码数据
func (m *WebRTCManager) SendVideoData(data []byte) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.logger.Debugf("SendVideoData called with %d bytes", len(data))

	if !m.running {
		m.logger.Debugf("WebRTC manager not running")
		return fmt.Errorf("WebRTC manager not running")
	}

	if m.videoTrack == nil {
		m.logger.Debugf("Video track not available")
		return fmt.Errorf("video track not available")
	}

	if len(data) == 0 {
		m.logger.Debugf("Empty video data")
		return fmt.Errorf("empty video data")
	}

	// 创建WebRTC sample
	// 假设30fps，每帧持续时间约33.33ms
	sample := media.Sample{
		Data:     data,
		Duration: time.Millisecond * 33, // ~30fps
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

	if !m.running {
		return fmt.Errorf("WebRTC manager not running")
	}

	if m.videoTrack == nil {
		return fmt.Errorf("video track not available")
	}

	if len(data) == 0 {
		return fmt.Errorf("empty video data")
	}

	// 创建WebRTC sample with custom duration
	sample := media.Sample{
		Data:     data,
		Duration: duration,
	}

	// 直接发送到WebRTC轨道
	if err := m.videoTrack.WriteSample(sample); err != nil {
		m.logger.Debugf("Failed to write video sample with timestamp: %v", err)
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
	defer m.mutex.Unlock()

	if !m.running || m.peerConnection == nil {
		return nil, fmt.Errorf("WebRTC manager not running or peer connection not initialized")
	}

	// 检查当前连接状态
	signalingState := m.peerConnection.SignalingState()
	m.logger.Debugf("Current signaling state: %s", signalingState)

	// 如果已经有 local offer，返回现有的 local description
	if signalingState == webrtc.SignalingStateHaveLocalOffer {
		localDesc := m.peerConnection.LocalDescription()
		if localDesc != nil {
			m.logger.Debug("Returning existing local offer")
			return localDesc, nil
		}
	}

	// 如果连接状态不是 stable，需要重新创建 PeerConnection
	if signalingState != webrtc.SignalingStateStable {
		m.logger.Debugf("Signaling state is %s, recreating PeerConnection", signalingState)
		if err := m.recreatePeerConnection(); err != nil {
			return nil, fmt.Errorf("failed to recreate peer connection: %w", err)
		}
	}

	m.logger.Debug("Creating SDP offer...")

	offer, err := m.peerConnection.CreateOffer(nil)
	if err != nil {
		m.logger.Errorf("Failed to create SDP offer: %v", err)
		return nil, fmt.Errorf("failed to create offer: %w", err)
	}

	// 设置本地描述
	if err := m.peerConnection.SetLocalDescription(offer); err != nil {
		m.logger.Errorf("Failed to set local description: %v", err)
		return nil, fmt.Errorf("failed to set local description: %w", err)
	}

	m.logger.Debug("SDP offer created and set as local description")
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

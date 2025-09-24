package webrtc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/pion/webrtc/v4"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
)

// ManagerConfig WebRTC管理器配置
type ManagerConfig struct {
	Config        *config.Config
	MediaProvider gstreamer.MediaStreamProvider // Direct connection to GStreamer (replaces bridge)
}

// Manager WebRTC管理器，统一管理所有WebRTC组件
type Manager struct {
	config *config.Config
	logger *logrus.Entry // 使用 logrus entry 来实现日志管理

	// WebRTC组件
	signaling   *SignalingServer
	mediaStream MediaStream
	sdpGen      *SDPGenerator

	// GStreamer integration (replaces bridge)
	mediaProvider   gstreamer.MediaStreamProvider
	mediaSubscriber *WebRTCMediaSubscriber
	subscriberID    uint64

	// HTTP处理器
	handlers *webrtcHandlers

	// 状态管理
	running   bool
	startTime time.Time
	mutex     sync.RWMutex

	// 上下文控制
	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager 创建WebRTC管理器
func NewManager(ctx context.Context, cfg *ManagerConfig) (*Manager, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}

	if cfg == nil {
		return nil, fmt.Errorf("manager config is required")
	}

	if cfg.Config == nil {
		return nil, fmt.Errorf("config is required")
	}

	if cfg.MediaProvider == nil {
		return nil, fmt.Errorf("media provider is required")
	}

	// Use the provided context instead of creating a new one
	childCtx, cancel := context.WithCancel(ctx)

	// 获取 logrus entry 用于结构化日志记录
	logger := config.GetLoggerWithPrefix("webrtc-manager")

	logger.Trace("Creating WebRTC manager with configuration")
	logger.Debugf("WebRTC configuration: codec=%s, resolution=%dx%d@%dfps",
		cfg.Config.GStreamer.Encoding.Codec,
		cfg.Config.GStreamer.Capture.Width,
		cfg.Config.GStreamer.Capture.Height,
		cfg.Config.GStreamer.Capture.FrameRate)

	manager := &Manager{
		config:        cfg.Config,
		logger:        logger,
		ctx:           childCtx,
		cancel:        cancel,
		mediaProvider: cfg.MediaProvider,
	}

	// 初始化组件
	logger.Trace("Initializing WebRTC components")
	if err := manager.initializeComponents(); err != nil {
		logger.Errorf("Failed to initialize WebRTC components: %v", err)
		cancel()
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	logger.Debug("WebRTC manager created successfully")
	return manager, nil
}

// initializeComponents 初始化所有WebRTC组件
func (m *Manager) initializeComponents() error {
	m.logger.Trace("Starting WebRTC components initialization")

	// 1. 创建SDP生成器
	m.logger.Trace("Step 1: Creating SDP generator")
	sdpConfig := &SDPConfig{
		Codec: string(m.config.GStreamer.Encoding.Codec),
	}
	m.sdpGen = NewSDPGenerator(sdpConfig)
	m.logger.Debugf("SDP generator created with codec: %s", m.config.GStreamer.Encoding.Codec)

	// 2. 创建媒体流
	m.logger.Trace("Step 2: Creating MediaStream")
	mediaStreamConfig := &MediaStreamConfig{
		VideoEnabled:    true,
		AudioEnabled:    false, // 暂时禁用音频
		VideoTrackID:    "video",
		AudioTrackID:    "audio",
		VideoCodec:      string(m.config.GStreamer.Encoding.Codec),
		VideoWidth:      m.config.GStreamer.Capture.Width,
		VideoHeight:     m.config.GStreamer.Capture.Height,
		VideoFrameRate:  m.config.GStreamer.Capture.FrameRate,
		VideoBitrate:    2000,
		AudioCodec:      "opus",
		AudioChannels:   2,
		AudioSampleRate: 48000,
		AudioBitrate:    128,
	}

	m.logger.Tracef("MediaStream configuration: video_enabled=%v, audio_enabled=%v, codec=%s, resolution=%dx%d@%dfps, video_bitrate=%d",
		mediaStreamConfig.VideoEnabled, mediaStreamConfig.AudioEnabled,
		mediaStreamConfig.VideoCodec, mediaStreamConfig.VideoWidth,
		mediaStreamConfig.VideoHeight, mediaStreamConfig.VideoFrameRate,
		mediaStreamConfig.VideoBitrate)

	var err error
	m.mediaStream, err = NewMediaStream(mediaStreamConfig)
	if err != nil {
		m.logger.Errorf("Failed to create MediaStream: %v", err)
		return fmt.Errorf("failed to create media stream: %w", err)
	}

	// 检查轨道创建状态并记录详细信息
	videoTrack := m.mediaStream.GetVideoTrack()
	audioTrack := m.mediaStream.GetAudioTrack()

	// 视频轨道状态检查
	if mediaStreamConfig.VideoEnabled {
		if videoTrack == nil {
			m.logger.Warn("Video track creation failed despite being enabled")
		} else {
			m.logger.Debugf("Video track created successfully: ID=%s, MimeType=%s",
				videoTrack.ID(), videoTrack.Codec().MimeType)
		}
	}

	// 音频轨道状态检查
	if mediaStreamConfig.AudioEnabled {
		if audioTrack == nil {
			m.logger.Warn("Audio track creation failed despite being enabled")
		} else {
			m.logger.Debugf("Audio track created successfully: ID=%s, MimeType=%s",
				audioTrack.ID(), audioTrack.Codec().MimeType)
		}
	}

	// 获取MediaStream统计信息并记录创建结果
	stats := m.mediaStream.GetStats()
	m.logger.Debugf("MediaStream creation result: total_tracks=%d, active_tracks=%d",
		stats.TotalTracks, stats.ActiveTracks)

	// 3. Create WebRTC media subscriber (replaces GStreamer bridge)
	m.logger.Trace("Step 3: Creating WebRTC media subscriber")

	// Generate unique subscriber ID
	subscriberID := uint64(time.Now().UnixNano())
	m.mediaSubscriber = NewWebRTCMediaSubscriber(subscriberID, m.mediaStream, m.logger)

	// Subscribe to video stream from GStreamer
	m.subscriberID, err = m.mediaProvider.AddVideoSubscriber(m.mediaSubscriber)
	if err != nil {
		m.logger.Errorf("Failed to add video subscriber to GStreamer: %v", err)
		return fmt.Errorf("failed to add video subscriber: %w", err)
	}

	m.logger.Debugf("WebRTC media subscriber created and registered with GStreamer (ID: %d)", m.subscriberID)

	// 4. 创建信令服务器
	m.logger.Trace("Step 4: Creating signaling server")
	signalingConfig := &SignalingEncoderConfig{
		Codec: string(m.config.GStreamer.Encoding.Codec),
	}

	// 转换配置中的ICE服务器格式
	iceServers := m.convertICEServers()
	m.signaling = NewSignalingServer(m.ctx, signalingConfig, m.mediaStream, iceServers)
	m.logger.Debug("Signaling server created successfully")

	// 5. 创建HTTP处理器
	m.logger.Trace("Step 5: Creating WebRTC handlers")
	m.handlers = newWebRTCHandlers(m)
	m.logger.Debug("WebRTC handlers created successfully")

	m.logger.Trace("WebRTC components initialization completed")
	return nil
}

// convertICEServers 转换配置中的ICE服务器格式
func (m *Manager) convertICEServers() []webrtc.ICEServer {
	var iceServers []webrtc.ICEServer

	// 从配置中获取ICE服务器
	if m.config.WebRTC.ICEServers != nil {
		for _, server := range m.config.WebRTC.ICEServers {
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
	}

	// 如果没有配置ICE服务器，使用默认的
	if len(iceServers) == 0 {
		iceServers = []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		}
		m.logger.Debug("No ICE servers configured, using default Google STUN server")
	} else {
		m.logger.Debugf("Using %d configured ICE servers", len(iceServers))
		for i, server := range iceServers {
			m.logger.Tracef("ICE Server %d: URLs=%v, Username=%s", i+1, server.URLs, server.Username)
		}
	}

	return iceServers
}

// Start 启动WebRTC管理器和所有组件
func (m *Manager) Start(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		m.logger.Warn("WebRTC manager already running")
		return fmt.Errorf("WebRTC manager already running")
	}

	m.logger.Debug("Starting WebRTC manager")
	m.startTime = time.Now()

	// 验证组件状态
	m.logger.Trace("Verifying component availability before startup")
	if m.mediaStream == nil {
		m.logger.Error("MediaStream is nil, cannot start WebRTC manager")
		return fmt.Errorf("MediaStream not initialized")
	}
	if m.mediaProvider == nil {
		m.logger.Error("MediaProvider is nil, cannot start WebRTC manager")
		return fmt.Errorf("MediaProvider not initialized")
	}
	if m.mediaSubscriber == nil {
		m.logger.Error("Media subscriber is nil, cannot start WebRTC manager")
		return fmt.Errorf("Media subscriber not initialized")
	}
	if m.signaling == nil {
		m.logger.Error("Signaling server is nil, cannot start WebRTC manager")
		return fmt.Errorf("Signaling server not initialized")
	}

	// Activate the media subscriber (replaces bridge startup)
	m.logger.Debug("Activating WebRTC media subscriber")
	m.mediaSubscriber.SetActive(true)
	m.logger.Debug("WebRTC media subscriber activated successfully")

	// 启动媒体流监控
	m.logger.Debug("Starting MediaStream track monitoring")
	if err := m.mediaStream.StartTrackMonitoring(); err != nil {
		m.logger.Warnf("Failed to start media stream monitoring: %v", err)
	} else {
		m.logger.Debug("MediaStream track monitoring started successfully")
	}

	// 验证MediaStream状态
	stats := m.mediaStream.GetStats()
	m.logger.Debugf("MediaStream status verification: total_tracks=%d, active_tracks=%d",
		stats.TotalTracks, stats.ActiveTracks)

	// 启动信令服务器
	m.logger.Debug("Starting signaling server")
	go m.signaling.Start()
	m.logger.Debug("Signaling server started successfully")

	// 记录MediaStream创建结果摘要 (Info级别)
	m.logMediaStreamCreationResult()

	m.running = true
	uptime := time.Since(m.startTime)
	m.logger.Debugf("WebRTC manager started successfully (startup time: %v)", uptime)

	return nil
}

// Stop 停止WebRTC管理器和所有组件
func (m *Manager) Stop(ctx context.Context) error {
	m.mutex.Lock()
	if !m.running {
		m.mutex.Unlock()
		m.logger.Debug("WebRTC manager already stopped")
		return nil
	}
	m.running = false
	m.mutex.Unlock()

	m.logger.Trace("Stopping WebRTC manager...")
	stopStartTime := time.Now()

	// 停止信令服务器
	m.logger.Debug("Stopping signaling server...")
	m.signaling.Stop()
	m.logger.Debug("Signaling server stopped")

	// 停止媒体流监控
	m.logger.Debug("Stopping MediaStream track monitoring...")
	if err := m.mediaStream.StopTrackMonitoring(); err != nil {
		m.logger.Warnf("Failed to stop media stream monitoring: %v", err)
	} else {
		m.logger.Debug("MediaStream track monitoring stopped")
	}

	// Deactivate media subscriber and unsubscribe from GStreamer
	m.logger.Debug("Deactivating WebRTC media subscriber...")
	if m.mediaSubscriber != nil {
		m.mediaSubscriber.SetActive(false)
	}

	if m.mediaProvider != nil && m.subscriberID != 0 {
		if removed := m.mediaProvider.RemoveVideoSubscriber(m.subscriberID); removed {
			m.logger.Debug("Successfully unsubscribed from GStreamer video stream")
		} else {
			m.logger.Warn("Failed to unsubscribe from GStreamer video stream")
		}
	}

	// 关闭媒体流
	m.logger.Debug("Closing MediaStream...")
	if err := m.mediaStream.Close(); err != nil {
		m.logger.Warnf("Failed to close media stream: %v", err)
	} else {
		m.logger.Debug("MediaStream closed")
	}

	// 取消上下文
	m.cancel()

	stopDuration := time.Since(stopStartTime)
	totalUptime := time.Since(m.startTime)
	m.logger.Tracef("WebRTC manager stopped successfully (shutdown time: %v, total uptime: %v)",
		stopDuration, totalUptime)
	return nil
}

// IsEnabled 检查组件是否启用
// WebRTC是核心组件，始终启用
func (m *Manager) IsEnabled() bool {
	return true
}

// IsRunning 检查管理器是否运行中
func (m *Manager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.running
}

// GetSignalingServer 获取信令服务器实例
func (m *Manager) GetSignalingServer() *SignalingServer {
	return m.signaling
}

// GetMediaStream 获取媒体流实例
func (m *Manager) GetMediaStream() MediaStream {
	return m.mediaStream
}

// GetMediaProvider 获取媒体流提供者实例 (replaces GetBridge)
func (m *Manager) GetMediaProvider() gstreamer.MediaStreamProvider {
	return m.mediaProvider
}

// GetMediaSubscriber 获取媒体订阅者实例
func (m *Manager) GetMediaSubscriber() *WebRTCMediaSubscriber {
	return m.mediaSubscriber
}

// GetSDPGenerator 获取SDP生成器实例
func (m *Manager) GetSDPGenerator() *SDPGenerator {
	return m.sdpGen
}

// GetStats 获取WebRTC管理器统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]interface{}{
		"running":    m.running,
		"start_time": m.startTime,
		"uptime":     time.Since(m.startTime).Seconds(),
	}

	if m.running {
		// 获取信令服务器统计
		stats["signaling"] = map[string]interface{}{
			"client_count": m.signaling.GetClientCount(),
		}

		// 获取媒体流统计
		mediaStats := m.mediaStream.GetStats()
		stats["media_stream"] = map[string]interface{}{
			"active_tracks":        mediaStats.ActiveTracks,
			"total_tracks":         mediaStats.TotalTracks,
			"video_frames_sent":    mediaStats.VideoFramesSent,
			"video_bytes_sent":     mediaStats.VideoBytesSent,
			"audio_frames_sent":    mediaStats.AudioFramesSent,
			"audio_bytes_sent":     mediaStats.AudioBytesSent,
			"last_video_timestamp": mediaStats.LastVideoTimestamp,
			"last_audio_timestamp": mediaStats.LastAudioTimestamp,
		}

		// 获取媒体订阅者统计 (replaces bridge stats)
		if m.mediaSubscriber != nil {
			subscriberStats := m.mediaSubscriber.GetStatistics()
			stats["media_subscriber"] = subscriberStats
		}
	}

	return stats
}

// GetContext 获取组件的上下文
func (m *Manager) GetContext() context.Context {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.ctx
}

// GetConfig 获取配置信息
func (m *Manager) GetConfig() *config.Config {
	return m.config
}

// GetUptime 获取运行时间
func (m *Manager) GetUptime() time.Duration {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if !m.running {
		return 0
	}

	return time.Since(m.startTime)
}

// GetComponentStatus 获取各组件状态
func (m *Manager) GetComponentStatus() map[string]bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return map[string]bool{
		"manager":          m.running,
		"signaling":        m.signaling != nil,
		"media_stream":     m.mediaStream != nil,
		"media_provider":   m.mediaProvider != nil,
		"media_subscriber": m.mediaSubscriber != nil,
		"sdp_generator":    m.sdpGen != nil,
	}
}

// logMediaStreamCreationResult 记录MediaStream创建结果摘要 (Info级别)
func (m *Manager) logMediaStreamCreationResult() {
	if m.mediaStream == nil {
		m.logger.Info("MediaStream creation failed - no media stream available")
		return
	}

	videoTrack := m.mediaStream.GetVideoTrack()
	audioTrack := m.mediaStream.GetAudioTrack()
	stats := m.mediaStream.GetStats()

	var trackStatus []string
	if videoTrack != nil {
		trackStatus = append(trackStatus, "video")
	}
	if audioTrack != nil {
		trackStatus = append(trackStatus, "audio")
	}

	if len(trackStatus) > 0 {
		m.logger.Infof("MediaStream created successfully with %d tracks: %v", stats.TotalTracks, trackStatus)
	} else {
		m.logger.Info("MediaStream created but no tracks available")
	}
}

// SetupRoutes 设置WebRTC组件的HTTP路由
// 实现webserver.RouteSetup接口
func (m *Manager) SetupRoutes(router *mux.Router) error {
	if m.handlers == nil {
		m.logger.Error("WebRTC handlers not initialized")
		return fmt.Errorf("WebRTC handlers not initialized")
	}

	m.logger.Debug("Setting up WebRTC routes...")

	// 设置WebRTC相关的所有路由
	if err := m.handlers.setupWebRTCRoutes(router); err != nil {
		m.logger.Errorf("Failed to setup WebRTC routes: %v", err)
		return fmt.Errorf("failed to setup WebRTC routes: %w", err)
	}

	m.logger.Debug("WebRTC routes setup completed")
	return nil
}

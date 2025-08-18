package webrtc

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/pion/webrtc/v4"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// ManagerConfig WebRTC管理器配置
type ManagerConfig struct {
	Config *config.Config
	Logger *log.Logger
}

// Manager WebRTC管理器，统一管理所有WebRTC组件
type Manager struct {
	config *config.Config
	logger *log.Logger

	// WebRTC组件
	signaling   *SignalingServer
	mediaStream MediaStream
	bridge      GStreamerBridge
	sdpGen      *SDPGenerator

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

	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	// Use the provided context instead of creating a new one
	childCtx, cancel := context.WithCancel(ctx)

	manager := &Manager{
		config: cfg.Config,
		logger: logger,
		ctx:    childCtx,
		cancel: cancel,
	}

	// 初始化组件
	if err := manager.initializeComponents(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize components: %w", err)
	}

	return manager, nil
}

// initializeComponents 初始化所有WebRTC组件
func (m *Manager) initializeComponents() error {
	// 1. 创建SDP生成器
	sdpConfig := &SDPConfig{
		Codec: string(m.config.GStreamer.Encoding.Codec),
	}
	m.sdpGen = NewSDPGenerator(sdpConfig)
	m.logger.Printf("SDP generator created with codec: %s", m.config.GStreamer.Encoding.Codec)

	// 2. 创建媒体流
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

	var err error
	m.mediaStream, err = NewMediaStream(mediaStreamConfig)
	if err != nil {
		return fmt.Errorf("failed to create media stream: %w", err)
	}
	m.logger.Printf("Media stream created successfully")

	// 3. 创建GStreamer桥接器
	bridgeConfig := DefaultGStreamerBridgeConfig()
	m.bridge = NewGStreamerBridge(bridgeConfig)

	// 设置媒体流到桥接器
	if err := m.bridge.SetMediaStream(m.mediaStream); err != nil {
		return fmt.Errorf("failed to set media stream to bridge: %w", err)
	}
	m.logger.Printf("GStreamer bridge created and configured")

	// 4. 创建信令服务器
	signalingConfig := &SignalingEncoderConfig{
		Codec: string(m.config.GStreamer.Encoding.Codec),
	}

	// 转换配置中的ICE服务器格式
	iceServers := m.convertICEServers()
	m.signaling = NewSignalingServer(m.ctx, signalingConfig, m.mediaStream, iceServers)
	m.logger.Printf("Signaling server created successfully")

	// 5. 创建HTTP处理器
	m.handlers = newWebRTCHandlers(m)
	m.logger.Printf("WebRTC handlers created successfully")

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
		m.logger.Printf("No ICE servers configured, using default Google STUN server")
	} else {
		m.logger.Printf("Using %d configured ICE servers", len(iceServers))
		for i, server := range iceServers {
			m.logger.Printf("ICE Server %d: %v", i+1, server.URLs)
		}
	}

	return iceServers
}

// Start 启动WebRTC管理器和所有组件
func (m *Manager) Start(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return fmt.Errorf("WebRTC manager already running")
	}

	m.logger.Printf("Starting WebRTC manager...")
	m.startTime = time.Now()

	// 启动GStreamer桥接器
	if err := m.bridge.Start(); err != nil {
		return fmt.Errorf("failed to start GStreamer bridge: %w", err)
	}
	m.logger.Printf("GStreamer bridge started")

	// 启动媒体流监控
	if err := m.mediaStream.StartTrackMonitoring(); err != nil {
		m.logger.Printf("Warning: failed to start media stream monitoring: %v", err)
	}

	// 启动信令服务器
	go m.signaling.Start()
	m.logger.Printf("Signaling server started")

	m.running = true
	m.logger.Printf("WebRTC manager started successfully")

	return nil
}

// Stop 停止WebRTC管理器和所有组件
func (m *Manager) Stop(ctx context.Context) error {
	m.mutex.Lock()
	if !m.running {
		m.mutex.Unlock()
		return nil
	}
	m.running = false
	m.mutex.Unlock()

	m.logger.Printf("Stopping WebRTC manager...")

	// 停止信令服务器
	m.signaling.Stop()
	m.logger.Printf("Signaling server stopped")

	// 停止媒体流监控
	if err := m.mediaStream.StopTrackMonitoring(); err != nil {
		m.logger.Printf("Warning: failed to stop media stream monitoring: %v", err)
	}

	// 停止GStreamer桥接器
	if err := m.bridge.Stop(); err != nil {
		m.logger.Printf("Warning: failed to stop GStreamer bridge: %v", err)
	} else {
		m.logger.Printf("GStreamer bridge stopped")
	}

	// 关闭媒体流
	if err := m.mediaStream.Close(); err != nil {
		m.logger.Printf("Warning: failed to close media stream: %v", err)
	} else {
		m.logger.Printf("Media stream closed")
	}

	// 取消上下文
	m.cancel()

	m.logger.Printf("WebRTC manager stopped successfully")
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

// GetBridge 获取GStreamer桥接器实例
func (m *Manager) GetBridge() GStreamerBridge {
	return m.bridge
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

		// 获取桥接器统计
		bridgeStats := m.bridge.GetStats()
		stats["bridge"] = map[string]interface{}{
			"video_samples_processed": bridgeStats.VideoSamplesProcessed,
			"audio_samples_processed": bridgeStats.AudioSamplesProcessed,
			"video_samples_dropped":   bridgeStats.VideoSamplesDropped,
			"audio_samples_dropped":   bridgeStats.AudioSamplesDropped,
			"video_processing_time":   bridgeStats.VideoProcessingTime.Milliseconds(),
			"audio_processing_time":   bridgeStats.AudioProcessingTime.Milliseconds(),
			"last_video_sample":       bridgeStats.LastVideoSample,
			"last_audio_sample":       bridgeStats.LastAudioSample,
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
		"manager":       m.running,
		"signaling":     m.signaling != nil,
		"media_stream":  m.mediaStream != nil,
		"bridge":        m.bridge != nil,
		"sdp_generator": m.sdpGen != nil,
	}
}

// SetupRoutes 设置WebRTC组件的HTTP路由
// 实现webserver.RouteSetup接口
func (m *Manager) SetupRoutes(router *mux.Router) error {
	if m.handlers == nil {
		return fmt.Errorf("WebRTC handlers not initialized")
	}

	m.logger.Printf("Setting up WebRTC routes...")

	// 设置WebRTC相关的所有路由
	if err := m.handlers.setupWebRTCRoutes(router); err != nil {
		return fmt.Errorf("failed to setup WebRTC routes: %w", err)
	}

	m.logger.Printf("WebRTC routes setup completed")
	return nil
}

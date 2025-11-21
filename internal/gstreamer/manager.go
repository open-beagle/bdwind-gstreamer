package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/common/events"
	"github.com/open-beagle/bdwind-gstreamer/internal/common/interfaces"
	"github.com/open-beagle/bdwind-gstreamer/internal/common/config"
)

// StreamState GStreamer流状态
type StreamState int

const (
	// StateIdle 空闲状态：未启动，无资源占用
	StateIdle StreamState = iota
	// StateStarting 启动中：正在初始化管道
	StateStarting
	// StateStreaming 推流中：正在捕获和编码
	StateStreaming
	// StateStopping 停止中：正在清理资源
	StateStopping
)

// String 返回状态的字符串表示
func (s StreamState) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateStarting:
		return "Starting"
	case StateStreaming:
		return "Streaming"
	case StateStopping:
		return "Stopping"
	default:
		return "Unknown"
	}
}

// Manager GStreamer 管理器
// 基于 go-gst 库实现，使用 appsink 进行视频捕获
type Manager struct {
	config   *config.GStreamerConfig
	logger   *logrus.Entry
	pipeline *gst.Pipeline
	appsink  *app.Sink

	// 状态管理
	state      StreamState
	stateMutex sync.RWMutex
	running    bool
	mutex      sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc

	// 视频数据接收器（回调接口）
	videoSink interfaces.VideoDataSink

	// 统计信息
	frameCount    uint64
	bytesReceived uint64
	startTime     time.Time
	lastFrameTime time.Time
}

// NewManager 创建 GStreamer 管理器
func NewManager(cfg *config.GStreamerConfig) (*Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	logger := logrus.WithField("component", "gstreamer")

	// 初始化 GStreamer
	gst.Init(nil)

	ctx, cancel := context.WithCancel(context.Background())

	manager := &Manager{
		config: cfg,
		logger: logger,
		state:  StateIdle,
		ctx:    ctx,
		cancel: cancel,
	}

	logger.Info("GStreamer manager created successfully")
	return manager, nil
}

// Start 启动 GStreamer 管道
func (m *Manager) Start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 检查状态，避免重复启动
	if m.state != StateIdle {
		m.logger.Debugf("GStreamer already in state %s, skipping start", m.state)
		return nil
	}

	m.logger.Info("Starting go-gst GStreamer manager...")
	m.setState(StateStarting)

	// 创建管道
	if err := m.createPipeline(); err != nil {
		m.setState(StateIdle)
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	// 设置 appsink 回调
	if err := m.setupAppsink(); err != nil {
		m.setState(StateIdle)
		return fmt.Errorf("failed to setup appsink: %w", err)
	}

	// 启动管道
	if err := m.startPipeline(); err != nil {
		m.setState(StateIdle)
		return fmt.Errorf("failed to start pipeline: %w", err)
	}

	m.running = true
	m.startTime = time.Now()
	m.setState(StateStreaming)
	m.logger.Info("GoGst manager started successfully")

	return nil
}

// createPipeline 创建 GStreamer 管道
func (m *Manager) createPipeline() error {
	m.logger.Debug("Creating GStreamer pipeline...")

	// 1. 创建空管道
	pipeline, err := gst.NewPipeline("")
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}
	m.pipeline = pipeline

	// 2. 创建源元素
	src, err := gst.NewElement("ximagesrc")
	if err != nil {
		return fmt.Errorf("failed to create ximagesrc: %w", err)
	}

	// 设置 ximagesrc 属性
	src.SetProperty("display-name", m.config.Capture.DisplayID)
	src.SetProperty("show-pointer", true)
	src.SetProperty("use-damage", false)

	// 3. 创建其他处理元素
	videoscale, err := gst.NewElement("videoscale")
	if err != nil {
		return fmt.Errorf("failed to create videoscale: %w", err)
	}
	videoscale.SetProperty("method", 0)

	videoconvert, err := gst.NewElement("videoconvert")
	if err != nil {
		return fmt.Errorf("failed to create videoconvert: %w", err)
	}

	queue, err := gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("failed to create queue: %w", err)
	}
	queue.SetProperty("max-size-buffers", uint(2))
	queue.SetProperty("leaky", 2) // downstream

	encoder, err := gst.NewElement("x264enc")
	if err != nil {
		return fmt.Errorf("failed to create x264enc: %w", err)
	}

	// 设置编码器属性 - 修复黑屏问题
	encoder.SetProperty("bitrate", m.config.Encoding.Bitrate)
	encoder.SetProperty("speed-preset", "ultrafast")
	encoder.SetProperty("tune", "zerolatency")

	// 关键帧设置 - 确保定期产生I帧
	encoder.SetProperty("key-int-max", 30) // 最大关键帧间隔30帧（1秒@30fps）

	// 强制立即产生关键帧 - 修复黑屏的关键
	encoder.SetProperty("insert-vui", true) // 插入VUI信息

	// 性能优化设置
	encoder.SetProperty("cabac", false)
	encoder.SetProperty("dct8x8", false)
	encoder.SetProperty("ref", 1)
	encoder.SetProperty("bframes", 0)
	encoder.SetProperty("b-adapt", false)
	encoder.SetProperty("aud", true)         // 添加访问单元分隔符
	encoder.SetProperty("byte-stream", true) // 使用字节流格式

	parser, err := gst.NewElement("h264parse")
	if err != nil {
		return fmt.Errorf("failed to create h264parse: %w", err)
	}
	// 每秒发送一次SPS/PPS配置信息，确保客户端能正确解码
	parser.SetProperty("config-interval", 1)        // 每1秒发送一次配置
	parser.SetProperty("disable-passthrough", true) // 禁用直通模式，强制解析

	// 4. 创建 appsink (关键修正)
	appsink, err := app.NewAppSink()
	if err != nil {
		return fmt.Errorf("failed to create appsink: %w", err)
	}
	m.appsink = appsink

	// 设置appsink的caps，确保接收正确格式的H.264数据
	caps := gst.NewCapsFromString("video/x-h264,stream-format=byte-stream,alignment=au")
	appsink.SetCaps(caps)

	// 5. 添加所有元素到管道
	pipeline.AddMany(src, videoscale, videoconvert, queue, encoder, parser, appsink.Element)

	// 6. 创建 caps 并链接元素
	if err := m.linkElements(src, videoscale, videoconvert, queue, encoder, parser, appsink); err != nil {
		return fmt.Errorf("failed to link elements: %w", err)
	}

	m.logger.Debug("Pipeline created successfully")
	return nil
}

// linkElements 链接管道元素
func (m *Manager) linkElements(src, videoscale, videoconvert, queue, encoder, parser *gst.Element, appsink *app.Sink) error {
	// 简单链接元素（让 GStreamer 自动协商 caps）
	if err := src.Link(videoscale); err != nil {
		return fmt.Errorf("failed to link src to videoscale: %w", err)
	}

	if err := videoscale.Link(videoconvert); err != nil {
		return fmt.Errorf("failed to link videoscale to videoconvert: %w", err)
	}

	if err := videoconvert.Link(queue); err != nil {
		return fmt.Errorf("failed to link videoconvert to queue: %w", err)
	}

	if err := queue.Link(encoder); err != nil {
		return fmt.Errorf("failed to link queue to encoder: %w", err)
	}

	if err := encoder.Link(parser); err != nil {
		return fmt.Errorf("failed to link encoder to parser: %w", err)
	}

	if err := parser.Link(appsink.Element); err != nil {
		return fmt.Errorf("failed to link parser to appsink: %w", err)
	}

	return nil
}

// setupAppsink 设置 appsink 回调 (修正版本)
func (m *Manager) setupAppsink() error {
	m.logger.Debug("Setting up appsink callbacks...")

	// 设置 appsink 属性
	m.appsink.SetDrop(true)
	m.appsink.SetMaxBuffers(2)
	m.appsink.SetEmitSignals(false) // 使用回调而不是信号

	// 设置回调 (关键修正)
	m.appsink.SetCallbacks(&app.SinkCallbacks{
		NewSampleFunc: func(sink *app.Sink) gst.FlowReturn {
			return m.onNewSample(sink)
		},
		EOSFunc: func(sink *app.Sink) {
			m.logger.Info("Received EOS signal")
		},
	})

	m.logger.Debug("Appsink setup completed")
	return nil
}

// onNewSample 处理新的视频样本 (修正版本)
func (m *Manager) onNewSample(sink *app.Sink) gst.FlowReturn {
	// 只在第一帧时打印，避免刷屏
	if m.frameCount == 0 {
		m.logger.Info("🎬 First sample received from appsink, video pipeline is working")
	}

	// 获取样本 (关键修正)
	sample := sink.PullSample()
	if sample == nil {
		m.logger.Warn("Failed to pull sample from appsink")
		return gst.FlowEOS
	}

	// 获取缓冲区
	buffer := sample.GetBuffer()
	if buffer == nil {
		m.logger.Warn("Failed to get buffer from sample")
		return gst.FlowError
	}

	// 映射缓冲区数据 (正确的内存管理)
	mapInfo := buffer.Map(gst.MapRead)
	defer buffer.Unmap()

	if mapInfo == nil {
		m.logger.Warn("Failed to map buffer")
		return gst.FlowError
	}

	// 获取数据
	data := mapInfo.AsUint8Slice()
	if len(data) == 0 {
		m.logger.Debug("Empty buffer received")
		return gst.FlowOK
	}

	// 更新统计信息
	m.frameCount++
	m.bytesReceived += uint64(len(data))
	m.lastFrameTime = time.Now()

	// 第一帧时打印详细信息
	if m.frameCount == 1 {
		m.logger.Infof("📊 GStreamer first frame: size=%d bytes (%d KB)", len(data), len(data)/1024)
	}

	// 每300帧（约10秒）打印一次统计信息，避免刷屏
	if m.frameCount%300 == 0 {
		m.logger.Infof("📊 GStreamer stats: frame=%d, total_bytes=%d MB, current_frame_size=%d bytes",
			m.frameCount, m.bytesReceived/(1024*1024), len(data))
	}

	// 复制数据并发送到 sink
	if m.videoSink != nil {
		dataCopy := make([]byte, len(data))
		copy(dataCopy, data)

		// 获取时间戳
		duration := time.Millisecond * 33 // 默认 30fps

		if err := m.videoSink.SendVideoData(dataCopy, duration); err != nil {
			m.logger.Errorf("❌ Video sink error: %v", err)
			return gst.FlowError
		}
	} else {
		// 只在第一次警告，避免刷屏
		if m.frameCount == 1 {
			m.logger.Warn("⚠️ Video sink is nil, data not sent!")
		}
	}

	return gst.FlowOK
}

// startPipeline 启动管道
func (m *Manager) startPipeline() error {
	m.logger.Debug("Starting pipeline...")

	// 检查显示环境
	displayID := m.config.Capture.DisplayID
	m.logger.Debugf("Attempting to capture from display: %s", displayID)

	// 设置管道状态为 PLAYING
	if err := m.pipeline.SetState(gst.StatePlaying); err != nil {
		m.logger.Errorf("Failed to set pipeline to PLAYING: %v", err)

		// 检查是否是显示访问问题
		if displayID == ":99" {
			m.logger.Error("X11 display :99 access failed - ensure Xvfb is running")
			m.logger.Info("Try: Xvfb :99 -screen 0 1920x1080x24 -ac &")
		}

		return fmt.Errorf("failed to set pipeline to PLAYING state: %w", err)
	}

	// 等待状态变更完成
	ret, _ := m.pipeline.GetState(gst.StatePlaying, gst.ClockTime(5*time.Second))
	if ret == gst.StateChangeFailure {
		m.logger.Error("Pipeline failed to reach PLAYING state")

		// 获取管道错误信息
		bus := m.pipeline.GetBus()
		if bus != nil {
			msg := bus.TimedPop(gst.ClockTime(100 * time.Millisecond))
			if msg != nil {
				m.logger.Errorf("Pipeline error: %s", msg.String())
			}
		}

		return fmt.Errorf("failed to reach PLAYING state")
	}

	m.logger.Debug("Pipeline started successfully")
	return nil
}

// Stop 停止管道
func (m *Manager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.state == StateIdle {
		m.logger.Debug("GStreamer already idle, skipping stop")
		return nil
	}

	m.logger.Info("Stopping go-gst manager...")
	m.setState(StateStopping)

	// 停止管道
	if m.pipeline != nil {
		m.pipeline.SetState(gst.StateNull)
		m.pipeline.GetState(gst.StateNull, gst.ClockTime(5*time.Second))
		m.pipeline = nil
	}

	// 清理 appsink
	m.appsink = nil

	// 取消上下文
	m.cancel()

	m.running = false
	m.setState(StateIdle)
	m.logger.Info("GoGst manager stopped successfully")

	return nil
}

// SetVideoSink 设置视频数据接收器
func (m *Manager) SetVideoSink(sink interfaces.VideoDataSink) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.videoSink = sink
	m.logger.Debug("Video sink set")
}

// UpdateBitrate 动态更新编码器比特率
func (m *Manager) UpdateBitrate(bitrate int) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if !m.running || m.pipeline == nil {
		return fmt.Errorf("pipeline not running")
	}

	// 查找编码器元素 - 需要通过其他方式获取
	// 由于 go-gst 的限制，我们暂时不支持动态比特率更新
	// 可以在创建时设置正确的比特率

	// 更新配置中的比特率（下次重启时生效）
	m.config.Encoding.Bitrate = bitrate
	m.logger.Infof("Bitrate updated to %d (will take effect on next restart)", bitrate)

	// 返回提示信息，表明需要重启才能生效
	return fmt.Errorf("bitrate update requires pipeline restart in minimal implementation")
}

// IsRunning 检查管理器是否正在运行
func (m *Manager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.running
}

// GetStats 获取管道统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]interface{}{
		"running":        m.running,
		"state":          m.GetState().String(),
		"type":           "go-gst",
		"frame_count":    m.frameCount,
		"bytes_received": m.bytesReceived,
	}

	if m.running {
		stats["start_time"] = m.startTime
		stats["uptime"] = time.Since(m.startTime).Seconds()
		stats["last_frame_time"] = m.lastFrameTime

		if !m.lastFrameTime.IsZero() {
			stats["seconds_since_last_frame"] = time.Since(m.lastFrameTime).Seconds()
		}

		if m.frameCount > 0 && !m.startTime.IsZero() {
			fps := float64(m.frameCount) / time.Since(m.startTime).Seconds()
			stats["average_fps"] = fps
		}
	}

	return stats
}

// GetState 获取当前流状态
func (m *Manager) GetState() StreamState {
	m.stateMutex.RLock()
	defer m.stateMutex.RUnlock()
	return m.state
}

// setState 设置流状态（内部使用）
func (m *Manager) setState(state StreamState) {
	m.stateMutex.Lock()
	defer m.stateMutex.Unlock()

	oldState := m.state
	m.state = state

	if oldState != state {
		m.logger.Infof("GStreamer state changed: %s -> %s", oldState, state)
	}
}

// SubscribeToWebRTCEvents 订阅WebRTC事件
func (m *Manager) SubscribeToWebRTCEvents(eventBus events.EventBus) {
	// 订阅会话开始事件 - 启动推流
	eventBus.Subscribe(events.EventWebRTCSessionStarted, events.EventHandlerFunc(func(ctx context.Context, event events.Event) (*events.EventResult, error) {
		data := event.Data()
		sessionID, _ := data["session_id"].(string)

		if m.GetState() == StateIdle {
			m.logger.Infof("WebRTC session started (session=%s), starting GStreamer...", sessionID)
			if err := m.Start(); err != nil {
				m.logger.Errorf("Failed to start GStreamer: %v", err)
				return events.ErrorResult("Failed to start GStreamer", err.Error()), err
			}
			return events.SuccessResult("GStreamer started", nil), nil
		}

		m.logger.Debugf("WebRTC session started (session=%s), GStreamer already running", sessionID)
		return events.SuccessResult("GStreamer already running", nil), nil
	}))

	// 订阅会话超时事件 - 停止推流
	eventBus.Subscribe(events.EventWebRTCSessionTimeout, events.EventHandlerFunc(func(ctx context.Context, event events.Event) (*events.EventResult, error) {
		data := event.Data()
		sessionID, _ := data["session_id"].(string)

		m.logger.Warnf("WebRTC session timeout (session=%s), stopping GStreamer...", sessionID)
		if err := m.Stop(); err != nil {
			m.logger.Errorf("Failed to stop GStreamer: %v", err)
			return events.ErrorResult("Failed to stop GStreamer", err.Error()), err
		}
		return events.SuccessResult("GStreamer stopped", nil), nil
	}))

	// 订阅无活跃会话事件 - 停止推流
	eventBus.Subscribe(events.EventWebRTCNoActiveSessions, events.EventHandlerFunc(func(ctx context.Context, event events.Event) (*events.EventResult, error) {
		data := event.Data()
		idleDuration, _ := data["idle_duration"].(time.Duration)

		if m.GetState() == StateStreaming {
			m.logger.Infof("No active WebRTC sessions (idle=%s), stopping GStreamer...", idleDuration)
			if err := m.Stop(); err != nil {
				m.logger.Errorf("Failed to stop GStreamer: %v", err)
				return events.ErrorResult("Failed to stop GStreamer", err.Error()), err
			}
			return events.SuccessResult("GStreamer stopped", nil), nil
		}
		return events.SuccessResult("GStreamer already idle", nil), nil
	}))

	// 可选：订阅会话就绪事件 - 记录日志
	eventBus.Subscribe(events.EventWebRTCSessionReady, events.EventHandlerFunc(func(ctx context.Context, event events.Event) (*events.EventResult, error) {
		data := event.Data()
		sessionID, _ := data["session_id"].(string)
		m.logger.Infof("WebRTC session ready (session=%s), streaming active", sessionID)
		return events.SuccessResult("Session ready", nil), nil
	}))

	m.logger.Info("Subscribed to WebRTC lifecycle events")
}

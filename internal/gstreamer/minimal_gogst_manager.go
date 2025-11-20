package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// MinimalGoGstManager 简化的 go-gst 管理器
// 基于官方 appsink 示例的最佳实践实现
type MinimalGoGstManager struct {
	config   *config.GStreamerConfig
	logger   *logrus.Entry
	pipeline *gst.Pipeline
	appsink  *app.Sink

	// 状态管理
	running bool
	mutex   sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc

	// 回调函数
	videoCallback func(data []byte, timestamp time.Duration) error

	// 统计信息
	frameCount    uint64
	bytesReceived uint64
	startTime     time.Time
	lastFrameTime time.Time
}

// NewMinimalGoGstManager 创建简化的 go-gst 管理器
func NewMinimalGoGstManager(cfg *config.GStreamerConfig) (*MinimalGoGstManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	logger := logrus.WithField("component", "minimal-gogst")

	// 初始化 GStreamer
	gst.Init(nil)

	ctx, cancel := context.WithCancel(context.Background())

	manager := &MinimalGoGstManager{
		config: cfg,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	logger.Info("Minimal go-gst manager created successfully")
	return manager, nil
}

// Start 启动 GStreamer 管道
func (m *MinimalGoGstManager) Start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return fmt.Errorf("GStreamer manager already running")
	}

	m.logger.Info("Starting go-gst GStreamer manager...")

	// 创建管道
	if err := m.createPipeline(); err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	// 设置 appsink 回调
	if err := m.setupAppsink(); err != nil {
		return fmt.Errorf("failed to setup appsink: %w", err)
	}

	// 启动管道
	if err := m.startPipeline(); err != nil {
		return fmt.Errorf("failed to start pipeline: %w", err)
	}

	m.running = true
	m.startTime = time.Now()
	m.logger.Info("GoGst manager started successfully")

	return nil
}

// createPipeline 创建 GStreamer 管道
func (m *MinimalGoGstManager) createPipeline() error {
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

	// 设置编码器属性
	encoder.SetProperty("bitrate", m.config.Encoding.Bitrate)
	encoder.SetProperty("speed-preset", "ultrafast")
	encoder.SetProperty("tune", "zerolatency")
	encoder.SetProperty("key-int-max", 30)
	encoder.SetProperty("cabac", false)
	encoder.SetProperty("dct8x8", false)
	encoder.SetProperty("ref", 1)
	encoder.SetProperty("bframes", 0)
	encoder.SetProperty("b-adapt", false)

	parser, err := gst.NewElement("h264parse")
	if err != nil {
		return fmt.Errorf("failed to create h264parse: %w", err)
	}
	parser.SetProperty("config-interval", 1)

	// 4. 创建 appsink (关键修正)
	appsink, err := app.NewAppSink()
	if err != nil {
		return fmt.Errorf("failed to create appsink: %w", err)
	}
	m.appsink = appsink

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
func (m *MinimalGoGstManager) linkElements(src, videoscale, videoconvert, queue, encoder, parser *gst.Element, appsink *app.Sink) error {
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
func (m *MinimalGoGstManager) setupAppsink() error {
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
func (m *MinimalGoGstManager) onNewSample(sink *app.Sink) gst.FlowReturn {
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

	// 复制数据并调用回调
	if m.videoCallback != nil {
		dataCopy := make([]byte, len(data))
		copy(dataCopy, data)

		// 获取时间戳
		duration := time.Millisecond * 33 // 默认 30fps

		if err := m.videoCallback(dataCopy, duration); err != nil {
			m.logger.Errorf("❌ Video callback error: %v", err)
			return gst.FlowError
		}
	} else {
		// 只在第一次警告，避免刷屏
		if m.frameCount == 1 {
			m.logger.Error("⚠️ Video callback is nil, data not sent to WebRTC!")
		}
	}

	return gst.FlowOK
}

// startPipeline 启动管道
func (m *MinimalGoGstManager) startPipeline() error {
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
func (m *MinimalGoGstManager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running {
		return nil
	}

	m.logger.Info("Stopping go-gst manager...")

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
	m.logger.Info("GoGst manager stopped successfully")

	return nil
}

// SetVideoCallback 设置视频数据回调函数
func (m *MinimalGoGstManager) SetVideoCallback(callback func(data []byte, timestamp time.Duration) error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.videoCallback = callback
	m.logger.Debug("Video callback set")
}

// UpdateBitrate 动态更新编码器比特率
func (m *MinimalGoGstManager) UpdateBitrate(bitrate int) error {
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
func (m *MinimalGoGstManager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.running
}

// GetStats 获取管道统计信息
func (m *MinimalGoGstManager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]interface{}{
		"running":        m.running,
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

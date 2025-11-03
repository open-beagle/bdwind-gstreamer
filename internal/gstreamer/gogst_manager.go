package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// GoGstManager 使用 go-gst 库的 GStreamer 管理器
// 重写版本：使用简单的单管道 appsink 架构
type GoGstManager struct {
	config *config.GStreamerConfig
	logger *logrus.Entry

	// GStreamer 组件 - 简化架构
	pipeline *gst.Pipeline
	appsink  *app.Sink
	mainLoop *glib.MainLoop
	bus      *gst.Bus

	// 管道元素
	source       *gst.Element
	videoscale   *gst.Element
	videoconvert *gst.Element
	queue        *gst.Element
	encoder      *gst.Element
	parser       *gst.Element

	// 状态管理
	running bool
	mutex   sync.RWMutex

	// 回调函数
	videoCallback func([]byte, time.Duration) error

	// 上下文管理
	ctx    context.Context
	cancel context.CancelFunc

	// 统计信息
	frameCount    uint64
	bytesReceived uint64
	lastFrameTime time.Time
}

// NewGoGstManager 创建新的 go-gst 管理器
// 重写版本：使用简化的单管道架构
func NewGoGstManager(cfg *config.GStreamerConfig) (*GoGstManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	// 初始化 GStreamer
	gst.Init(nil)

	ctx, cancel := context.WithCancel(context.Background())
	logger := logrus.WithField("component", "gogst-manager-v2")

	manager := &GoGstManager{
		config: cfg,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	logger.Info("GoGst manager v2 created successfully (simplified architecture)")
	return manager, nil
}

// Start 启动 GStreamer 管道
// 重写版本：使用简化的启动流程
func (m *GoGstManager) Start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return fmt.Errorf("GStreamer manager already running")
	}

	m.logger.Info("Starting go-gst GStreamer manager v2 (simplified architecture)...")

	// 步骤1：创建管道
	m.logger.Debug("Step 1: Creating pipeline...")
	if err := m.createPipeline(); err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}

	// 步骤2：设置 appsink 回调
	m.logger.Debug("Step 2: Setting up appsink...")
	if err := m.setupAppsink(); err != nil {
		return fmt.Errorf("failed to setup appsink: %w", err)
	}

	// 步骤3：启动管道
	m.logger.Debug("Step 3: Starting pipeline...")
	if err := m.startPipeline(); err != nil {
		return fmt.Errorf("failed to start pipeline: %w", err)
	}

	m.running = true
	m.logger.Info("GoGst manager v2 started successfully")

	return nil
}

// createPipeline 创建 GStreamer 管道
// 重写版本：使用正确的 appsink 单管道架构
func (m *GoGstManager) createPipeline() error {
	m.logger.Info("Creating simplified GStreamer pipeline...")

	// 1. 创建空管道
	pipeline, err := gst.NewPipeline("video-pipeline")
	if err != nil {
		return fmt.Errorf("failed to create pipeline: %w", err)
	}
	m.pipeline = pipeline

	// 获取管道总线
	m.bus = pipeline.GetBus()
	if m.bus == nil {
		return fmt.Errorf("failed to get pipeline bus")
	}

	// 2. 创建源元素
	m.source, err = gst.NewElement("ximagesrc")
	if err != nil {
		return fmt.Errorf("failed to create ximagesrc: %w", err)
	}

	// 设置 ximagesrc 属性
	m.source.SetProperty("display-name", m.config.Capture.DisplayID)
	m.source.SetProperty("show-pointer", false) // 简化：不显示鼠标指针
	m.source.SetProperty("use-damage", false)

	// 3. 创建处理元素
	m.videoscale, err = gst.NewElement("videoscale")
	if err != nil {
		return fmt.Errorf("failed to create videoscale: %w", err)
	}

	m.videoconvert, err = gst.NewElement("videoconvert")
	if err != nil {
		return fmt.Errorf("failed to create videoconvert: %w", err)
	}

	m.queue, err = gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("failed to create queue: %w", err)
	}
	// 简化队列配置
	m.queue.SetProperty("max-size-buffers", uint(2))
	m.queue.SetProperty("leaky", uint(2)) // downstream

	// 4. 创建编码器
	m.encoder, err = gst.NewElement("x264enc")
	if err != nil {
		return fmt.Errorf("failed to create x264enc: %w", err)
	}

	// 使用正确的属性设置方法
	if err := m.configureEncoder(); err != nil {
		m.logger.Warnf("Failed to configure encoder: %v", err)
		// 继续执行，使用默认设置
	}

	// 5. 创建解析器
	m.parser, err = gst.NewElement("h264parse")
	if err != nil {
		return fmt.Errorf("failed to create h264parse: %w", err)
	}
	m.parser.SetProperty("config-interval", int(1))

	// 6. 创建 appsink (关键修正)
	m.appsink, err = app.NewAppSink()
	if err != nil {
		return fmt.Errorf("failed to create appsink: %w", err)
	}

	// 7. 添加所有元素到管道
	if err := pipeline.AddMany(m.source, m.videoscale, m.videoconvert, m.queue, m.encoder, m.parser, m.appsink.Element); err != nil {
		return fmt.Errorf("failed to add elements to pipeline: %w", err)
	}

	// 8. 链接元素
	if err := m.linkElements(); err != nil {
		return fmt.Errorf("failed to link elements: %w", err)
	}

	m.logger.Info("Simplified pipeline created successfully")
	return nil
}

// configureEncoder 配置编码器属性
// 使用正确的数据类型避免属性设置错误
func (m *GoGstManager) configureEncoder() error {
	m.logger.Debug("Configuring x264 encoder with correct data types...")

	// 基本属性 - 使用正确的数据类型
	if err := m.encoder.SetProperty("bitrate", uint(m.config.Encoding.Bitrate)); err != nil {
		m.logger.Warnf("Failed to set bitrate: %v", err)
	}

	if err := m.encoder.SetProperty("threads", uint(0)); err != nil { // 0 = auto
		m.logger.Warnf("Failed to set threads: %v", err)
	}

	if err := m.encoder.SetProperty("key-int-max", uint(30)); err != nil {
		m.logger.Warnf("Failed to set key-int-max: %v", err)
	}

	if err := m.encoder.SetProperty("bframes", uint(0)); err != nil {
		m.logger.Warnf("Failed to set bframes: %v", err)
	}

	// 尝试设置枚举属性 - 如果失败则使用默认值
	// speed-preset 枚举值：0=ultrafast, 1=superfast, 2=veryfast, 3=faster, 4=fast
	if err := m.encoder.SetProperty("speed-preset", uint(0)); err != nil {
		m.logger.Debugf("Failed to set speed-preset enum, trying string: %v", err)
		// 某些版本可能需要字符串
		if err := m.encoder.SetProperty("speed-preset", "ultrafast"); err != nil {
			m.logger.Warnf("Failed to set speed-preset: %v", err)
		}
	}

	// tune 枚举值：需要查询具体的枚举值
	if err := m.encoder.SetProperty("tune", uint(4)); err != nil { // 4 通常是 zerolatency
		m.logger.Debugf("Failed to set tune enum, trying string: %v", err)
		if err := m.encoder.SetProperty("tune", "zerolatency"); err != nil {
			m.logger.Warnf("Failed to set tune: %v", err)
		}
	}

	m.logger.Debug("Encoder configuration completed")
	return nil
}

// linkElements 链接管道元素
// 重写版本：使用简化的链接方式
func (m *GoGstManager) linkElements() error {
	m.logger.Debug("Linking pipeline elements...")

	// 创建 caps
	captureCaps := gst.NewCapsFromString(fmt.Sprintf(
		"video/x-raw,framerate=%d/1", m.config.Capture.FrameRate))

	scaleCaps := gst.NewCapsFromString(fmt.Sprintf(
		"video/x-raw,width=%d,height=%d,format=I420",
		m.config.Capture.Width, m.config.Capture.Height))

	h264Caps := gst.NewCapsFromString("video/x-h264,stream-format=byte-stream,alignment=au")

	// 链接元素：source → videoscale → videoconvert → queue → encoder → parser → appsink
	if err := m.source.LinkFiltered(m.videoscale, captureCaps); err != nil {
		return fmt.Errorf("failed to link source to videoscale: %w", err)
	}

	if err := m.videoscale.LinkFiltered(m.videoconvert, scaleCaps); err != nil {
		return fmt.Errorf("failed to link videoscale to videoconvert: %w", err)
	}

	if err := m.videoconvert.Link(m.queue); err != nil {
		return fmt.Errorf("failed to link videoconvert to queue: %w", err)
	}

	if err := m.queue.Link(m.encoder); err != nil {
		return fmt.Errorf("failed to link queue to encoder: %w", err)
	}

	if err := m.encoder.LinkFiltered(m.parser, h264Caps); err != nil {
		return fmt.Errorf("failed to link encoder to parser: %w", err)
	}

	if err := m.parser.Link(m.appsink.Element); err != nil {
		return fmt.Errorf("failed to link parser to appsink: %w", err)
	}

	m.logger.Debug("All elements linked successfully")
	return nil
}

// setupAppsink 设置 appsink 回调
// 重写版本：使用正确的 appsink API
func (m *GoGstManager) setupAppsink() error {
	m.logger.Info("Setting up appsink with correct callbacks...")

	// 设置 appsink 属性
	m.appsink.SetDrop(true)
	m.appsink.SetMaxBuffers(2)
	m.appsink.SetEmitSignals(false) // 重要：使用回调而不是信号

	// 设置 caps 过滤器
	caps := gst.NewCapsFromString("video/x-h264,stream-format=byte-stream,alignment=au")
	m.appsink.SetCaps(caps)

	// 设置回调 (关键修正)
	m.appsink.SetCallbacks(&app.SinkCallbacks{
		NewSampleFunc: func(sink *app.Sink) gst.FlowReturn {
			return m.onNewSample(sink)
		},
		EOSFunc: func(sink *app.Sink) {
			m.logger.Info("Appsink received EOS signal")
		},
	})

	m.logger.Info("Appsink callbacks configured successfully")
	return nil
}

// onNewSample 处理新的视频样本
// 重写版本：使用正确的 PullSample API 和安全的数据处理
func (m *GoGstManager) onNewSample(sink *app.Sink) gst.FlowReturn {
	// 更新统计信息
	m.mutex.Lock()
	m.frameCount++
	frameNum := m.frameCount
	m.lastFrameTime = time.Now()
	m.mutex.Unlock()

	// 获取样本 (关键修正)
	sample := sink.PullSample()
	if sample == nil {
		m.logger.Warn("Failed to pull sample from appsink")
		return gst.FlowEOS
	}
	defer sample.Unref() // 重要：释放样本引用

	// 获取缓冲区
	buffer := sample.GetBuffer()
	if buffer == nil {
		m.logger.Warn("Failed to get buffer from sample")
		return gst.FlowError
	}

	// 映射缓冲区数据 (修正版本)
	mapInfo := buffer.Map(gst.MapRead)
	if mapInfo == nil {
		m.logger.Warn("Failed to map buffer")
		return gst.FlowError
	}
	defer buffer.Unmap() // 重要：确保解除映射

	// 获取数据 (修正版本)
	data := mapInfo.Bytes()
	if len(data) == 0 {
		m.logger.Debug("Received empty buffer")
		return gst.FlowOK
	}

	// 更新字节统计
	m.mutex.Lock()
	m.bytesReceived += uint64(len(data))
	m.mutex.Unlock()

	// 获取时间戳
	pts := buffer.PresentationTimestamp()
	duration := buffer.Duration()

	if frameNum%30 == 1 { // 每30帧记录一次详细信息
		m.logger.Debugf("Frame #%d: %d bytes, PTS: %d, duration: %d",
			frameNum, len(data), pts, duration)
	}

	// 复制数据（避免在回调中使用原始缓冲区）
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	// 调用视频回调
	if m.videoCallback != nil {
		if err := m.videoCallback(dataCopy, time.Duration(duration)); err != nil {
			m.logger.Errorf("Video callback error for frame #%d: %v", frameNum, err)
			return gst.FlowError
		}
	}

	return gst.FlowOK
}

// startPipeline 启动管道
// 重写版本：消除死锁问题，使用安全的状态管理
func (m *GoGstManager) startPipeline() error {
	m.logger.Info("Starting simplified pipeline...")

	// 设置管道状态为 PLAYING
	if err := m.pipeline.SetState(gst.StatePlaying); err != nil {
		return fmt.Errorf("failed to set pipeline to PLAYING state: %w", err)
	}

	// 等待状态变化完成
	ret, state := m.pipeline.GetState(gst.StatePlaying, gst.ClockTimeNone)
	if ret == gst.StateChangeFailure {
		return fmt.Errorf("pipeline failed to reach PLAYING state")
	}

	m.logger.Debugf("Pipeline state: %s", state.String())

	// 启动简化的消息处理（避免死锁）
	go m.handleBusMessages()

	// 启动主循环（如果需要）
	m.mainLoop = glib.NewMainLoop(glib.MainContextDefault(), false)
	go func() {
		m.logger.Debug("Starting GLib main loop...")
		m.mainLoop.Run()
		m.logger.Debug("GLib main loop stopped")
	}()

	m.logger.Info("Pipeline started successfully")
	return nil
}

// handleBusMessages 处理总线消息
// 重写版本：简化的消息处理，避免死锁
func (m *GoGstManager) handleBusMessages() {
	m.logger.Debug("Starting simplified bus message handling...")

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Debug("Bus message handling stopped due to context cancellation")
			return
		default:
			// 使用较短的超时避免阻塞
			msg := m.bus.TimedPop(gst.ClockTime(100 * time.Millisecond))
			if msg == nil {
				continue
			}

			switch msg.Type() {
			case gst.MessageError:
				err := msg.ParseError()
				m.logger.Errorf("Pipeline error: %v", err)
				// 简化错误处理，不触发复杂的恢复机制

			case gst.MessageWarning:
				err := msg.ParseWarning()
				m.logger.Warnf("Pipeline warning: %v", err)

			case gst.MessageEOS:
				m.logger.Info("Pipeline reached end of stream")

			case gst.MessageStateChanged:
				// 记录状态变化（简化版本）
				oldState, newState := msg.ParseStateChanged()
				m.logger.Debugf("Element state changed: %s -> %s",
					oldState.String(), newState.String())
			}

			msg.Unref() // 重要：释放消息引用
		}
	}
}

// Stop 停止 GStreamer 管道
// 重写版本：确保正确的资源清理，避免内存泄漏
func (m *GoGstManager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running {
		return nil
	}

	m.logger.Info("Stopping go-gst GStreamer manager v2...")

	// 取消上下文，停止所有 goroutine
	if m.cancel != nil {
		m.cancel()
	}

	// 停止管道
	if m.pipeline != nil {
		m.logger.Debug("Setting pipeline to NULL state...")
		if err := m.pipeline.SetState(gst.StateNull); err != nil {
			m.logger.Warnf("Failed to set pipeline to NULL state: %v", err)
		}

		// 等待状态变化完成
		ret, _ := m.pipeline.GetState(gst.StateNull, gst.ClockTime(5*time.Second))
		if ret == gst.StateChangeFailure {
			m.logger.Warn("Failed to wait for NULL state")
		}

		// 清理管道引用
		m.pipeline.Unref()
		m.pipeline = nil
	}

	// 停止主循环
	if m.mainLoop != nil {
		m.mainLoop.Quit()
		m.mainLoop = nil
	}

	// 清理总线引用
	if m.bus != nil {
		m.bus.Unref()
		m.bus = nil
	}

	// 清理元素引用（GStreamer 会自动管理，但显式清理更安全）
	m.source = nil
	m.videoscale = nil
	m.videoconvert = nil
	m.queue = nil
	m.encoder = nil
	m.parser = nil
	m.appsink = nil

	m.running = false

	// 记录统计信息
	m.logger.Infof("GoGst manager v2 stopped successfully (processed %d frames, %d bytes)",
		m.frameCount, m.bytesReceived)

	return nil
}

// IsRunning 检查运行状态
func (m *GoGstManager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.running
}

// SetVideoCallback 设置视频数据回调
func (m *GoGstManager) SetVideoCallback(callback func([]byte, time.Duration) error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.videoCallback = callback
	m.logger.Debug("Video callback set")
}

// GetPipeline 获取管道实例（用于调试）
func (m *GoGstManager) GetPipeline() *gst.Pipeline {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.pipeline
}

// UpdateBitrate 动态更新比特率
func (m *GoGstManager) UpdateBitrate(bitrate int) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if !m.running || m.pipeline == nil {
		return fmt.Errorf("pipeline not running")
	}

	m.logger.Infof("Bitrate update requested to %d (go-gst implementation)", bitrate)
	// 实际实现需要根据 go-gst API 调整
	return nil
}

// GetStats 获取管道统计信息
// 重写版本：提供详细的统计信息
func (m *GoGstManager) GetStats() map[string]any {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]any{
		"running":        m.running,
		"type":           "go-gst-v2",
		"architecture":   "simplified-appsink",
		"frame_count":    m.frameCount,
		"bytes_received": m.bytesReceived,
	}

	if m.pipeline != nil {
		_, state := m.pipeline.GetState(gst.StateNull, 0)
		stats["pipeline_state"] = state.String()
	}

	if !m.lastFrameTime.IsZero() {
		stats["last_frame_time"] = m.lastFrameTime
		stats["seconds_since_last_frame"] = time.Since(m.lastFrameTime).Seconds()
	}

	// 计算平均 FPS（如果有足够的数据）
	if m.frameCount > 0 && !m.lastFrameTime.IsZero() {
		// 这里需要记录开始时间来计算准确的 FPS
		// 暂时使用简化的计算
		stats["estimated_fps"] = "calculating..."
	}

	return stats
}

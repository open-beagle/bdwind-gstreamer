package webrtc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
)

// GStreamerBridgeConfig GStreamer桥接配置
type GStreamerBridgeConfig struct {
	// 缓冲区配置
	VideoBufferSize int `yaml:"video_buffer_size"`
	AudioBufferSize int `yaml:"audio_buffer_size"`

	// 时间戳配置
	VideoClockRate uint32 `yaml:"video_clock_rate"`
	AudioClockRate uint32 `yaml:"audio_clock_rate"`

	// 性能配置
	MaxConcurrentFrames int           `yaml:"max_concurrent_frames"`
	ProcessingTimeout   time.Duration `yaml:"processing_timeout"`
}

// DefaultGStreamerBridgeConfig 返回默认桥接配置
func DefaultGStreamerBridgeConfig() *GStreamerBridgeConfig {
	return &GStreamerBridgeConfig{
		VideoBufferSize:     100,
		AudioBufferSize:     200,
		VideoClockRate:      90000, // 标准视频时钟频率
		AudioClockRate:      48000, // 标准音频时钟频率
		MaxConcurrentFrames: 10,
		ProcessingTimeout:   time.Millisecond * 100,
	}
}

// GStreamerBridge GStreamer与WebRTC的桥接器
type GStreamerBridge interface {
	// Start 启动桥接器
	Start() error

	// Stop 停止桥接器
	Stop() error

	// SetMediaStream 设置媒体流
	SetMediaStream(stream MediaStream) error

	// ProcessVideoSample 处理视频样本
	ProcessVideoSample(sample *gstreamer.Sample) error

	// ProcessAudioSample 处理音频样本
	ProcessAudioSample(sample *gstreamer.Sample) error

	// GetStats 获取桥接器统计信息
	GetStats() BridgeStats

	// ValidateProcessingStatus 验证样本处理状态
	ValidateProcessingStatus() error

	// GetHealthStatus 获取健康状态摘要
	GetHealthStatus() string

	SetEncodedSampleCallback(callback func(*gstreamer.Sample) error)
}

// BridgeStats 桥接器统计信息
type BridgeStats struct {
	VideoSamplesProcessed int64
	AudioSamplesProcessed int64
	VideoSamplesDropped   int64
	AudioSamplesDropped   int64
	VideoProcessingTime   time.Duration
	AudioProcessingTime   time.Duration
	LastVideoSample       time.Time
	LastAudioSample       time.Time

	// 新增样本处理状态验证字段
	VideoSampleRate     float64   // 视频样本处理速率 (samples/sec)
	AudioSampleRate     float64   // 音频样本处理速率 (samples/sec)
	VideoErrorCount     int64     // 视频样本处理错误计数
	AudioErrorCount     int64     // 音频样本处理错误计数
	AvgVideoProcessTime float64   // 平均视频样本处理时间 (ms)
	AvgAudioProcessTime float64   // 平均音频样本处理时间 (ms)
	HealthStatus        string    // 整体健康状态
	LastHealthCheck     time.Time // 最后健康检查时间
}

// gstreamerBridgeImpl GStreamer桥接器实现
type gstreamerBridgeImpl struct {
	config      *GStreamerBridgeConfig
	mediaStream MediaStream
	logger      *logrus.Entry // 使用 logrus entry 来实现日志管理

	// 缓冲区
	videoBuffer chan *gstreamer.Sample
	audioBuffer chan *gstreamer.Sample

	// 统计信息
	stats BridgeStats
	mutex sync.RWMutex

	// 上下文和控制
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool

	// 时间戳管理
	videoTimestamp uint32
	audioTimestamp uint32
	startTime      time.Time

	// 样本处理状态验证
	lastStatsReport     time.Time
	statsReportInterval time.Duration
	healthCheckInterval time.Duration
	lastHealthCheck     time.Time

	// 异常检测阈值
	maxProcessingTime time.Duration
	maxSampleRate     float64
	minSampleRate     float64
	maxErrorRate      float64

	encodedSampleCallback func(*gstreamer.Sample) error
}

// NewGStreamerBridge 创建新的GStreamer桥接器
func NewGStreamerBridge(cfg *GStreamerBridgeConfig) GStreamerBridge {
	if cfg == nil {
		cfg = DefaultGStreamerBridgeConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())
	logger := config.GetLoggerWithPrefix("webrtc-gstreamer-bridge")

	return &gstreamerBridgeImpl{
		config:      cfg,
		logger:      logger,
		videoBuffer: make(chan *gstreamer.Sample, cfg.VideoBufferSize),
		audioBuffer: make(chan *gstreamer.Sample, cfg.AudioBufferSize),
		ctx:         ctx,
		cancel:      cancel,
		startTime:   time.Now(),

		// 初始化样本处理状态验证参数
		statsReportInterval: time.Second * 30, // 每30秒报告统计
		healthCheckInterval: time.Second * 10, // 每10秒健康检查
		lastStatsReport:     time.Now(),
		lastHealthCheck:     time.Now(),

		// 异常检测阈值
		maxProcessingTime: time.Millisecond * 50, // 最大处理时间50ms
		maxSampleRate:     1000.0,                // 最大样本率1000/s
		minSampleRate:     1.0,                   // 最小样本率1/s
		maxErrorRate:      0.05,                  // 最大错误率5%
	}
}

// SetEncodedSampleCallback sets the callback for encoded samples.
func (gb *gstreamerBridgeImpl) SetEncodedSampleCallback(callback func(*gstreamer.Sample) error) {
	gb.mutex.Lock()
	defer gb.mutex.Unlock()
	gb.encodedSampleCallback = callback
}

// Start 启动桥接器
func (gb *gstreamerBridgeImpl) Start() error {
	gb.mutex.Lock()
	defer gb.mutex.Unlock()

	if gb.running {
		// Warn级别: 样本处理异常 - 桥接器已运行
		gb.logger.Warn("Bridge start failed: already running")
		return fmt.Errorf("bridge already running")
	}

	if gb.mediaStream == nil {
		// Error级别: 链路中断 - MediaStream未设置
		gb.logger.Error("Bridge start failed: media stream not set")
		return fmt.Errorf("media stream not set")
	}

	// Info级别: 记录样本回调链路建立成功状态
	gb.logger.Trace("Starting GStreamer bridge for sample processing")

	gb.running = true
	gb.startTime = time.Now()

	// Debug级别: 记录桥接器配置
	gb.logger.Debug("Bridge configuration:")
	gb.logger.Debugf("  Video buffer size: %d", gb.config.VideoBufferSize)
	gb.logger.Debugf("  Audio buffer size: %d", gb.config.AudioBufferSize)
	gb.logger.Debugf("  Video clock rate: %d Hz", gb.config.VideoClockRate)
	gb.logger.Debugf("  Audio clock rate: %d Hz", gb.config.AudioClockRate)
	gb.logger.Debugf("  Max concurrent frames: %d", gb.config.MaxConcurrentFrames)
	gb.logger.Debugf("  Processing timeout: %v", gb.config.ProcessingTimeout)

	// 启动视频处理协程
	gb.wg.Add(1)
	go gb.processVideoSamples()

	// 启动音频处理协程
	gb.wg.Add(1)
	go gb.processAudioSamples()

	// Info级别: 记录样本回调链路建立成功状态
	gb.logger.Debug("GStreamer bridge started successfully")
	// Debug级别: 记录处理协程状态
	gb.logger.Debug("Sample processing goroutines started")

	return nil
}

// Stop 停止桥接器
func (gb *gstreamerBridgeImpl) Stop() error {
	gb.mutex.Lock()
	if !gb.running {
		gb.mutex.Unlock()
		// Debug级别: 记录桥接器已停止
		gb.logger.Debug("Bridge stop skipped: already stopped")
		return nil
	}
	gb.running = false
	gb.mutex.Unlock()

	// Info级别: 记录样本回调链路停止开始
	gb.logger.Trace("Stopping GStreamer bridge")

	// 取消上下文
	gb.cancel()

	// 关闭缓冲区
	close(gb.videoBuffer)
	close(gb.audioBuffer)
	// Debug级别: 记录缓冲区关闭
	gb.logger.Debug("Sample processing buffers closed")

	// 等待所有协程结束
	gb.wg.Wait()
	// Debug级别: 记录协程停止
	gb.logger.Debug("All sample processing goroutines stopped")

	// Info级别: 记录最终统计信息
	gb.mutex.RLock()
	stats := gb.stats
	gb.mutex.RUnlock()

	gb.logger.Trace("Bridge stopped - Final statistics:")
	gb.logger.Tracef("  Video samples processed: %d", stats.VideoSamplesProcessed)
	gb.logger.Tracef("  Audio samples processed: %d", stats.AudioSamplesProcessed)
	gb.logger.Tracef("  Video samples dropped: %d", stats.VideoSamplesDropped)
	gb.logger.Tracef("  Audio samples dropped: %d", stats.AudioSamplesDropped)

	return nil
}

// SetMediaStream 设置媒体流
func (gb *gstreamerBridgeImpl) SetMediaStream(stream MediaStream) error {
	gb.mutex.Lock()
	defer gb.mutex.Unlock()

	gb.mediaStream = stream
	return nil
}

// ProcessVideoSample 处理视频样本
func (gb *gstreamerBridgeImpl) ProcessVideoSample(sample *gstreamer.Sample) error {
	gb.mutex.RLock()
	running := gb.running
	gb.mutex.RUnlock()

	if !running {
		// Warn级别: 样本处理异常 - 桥接器未运行
		gb.logger.Warn("Video sample processing failed: bridge not running")
		return fmt.Errorf("bridge not running")
	}

	// Trace级别: 记录每个样本的详细处理信息
	gb.logger.Tracef("Received video sample for processing: size=%d bytes, format=%s, dimensions=%dx%d",
		sample.Size(), sample.Format.Codec, sample.Format.Width, sample.Format.Height)

	select {
	case gb.videoBuffer <- sample:
		// Trace级别: 记录样本成功加入缓冲区
		gb.logger.Trace("Video sample queued for processing")
		return nil
	case <-gb.ctx.Done():
		// Warn级别: 样本处理异常 - 上下文取消
		gb.logger.Warn("Video sample processing cancelled: context done")
		return gb.ctx.Err()
	default:
		// Warn级别: 样本处理异常 - 缓冲区满
		gb.mutex.Lock()
		gb.stats.VideoSamplesDropped++
		droppedCount := gb.stats.VideoSamplesDropped
		gb.mutex.Unlock()

		gb.logger.Warnf("Video sample dropped: buffer full (total dropped: %d)", droppedCount)
		return fmt.Errorf("video buffer full, sample dropped")
	}
}

// ProcessAudioSample 处理音频样本
func (gb *gstreamerBridgeImpl) ProcessAudioSample(sample *gstreamer.Sample) error {
	gb.mutex.RLock()
	running := gb.running
	gb.mutex.RUnlock()

	if !running {
		// Warn级别: 样本处理异常 - 桥接器未运行
		gb.logger.Warn("Audio sample processing failed: bridge not running")
		return fmt.Errorf("bridge not running")
	}

	// Trace级别: 记录每个样本的详细处理信息
	gb.logger.Tracef("Received audio sample for processing: size=%d bytes, format=%s, channels=%d, sample_rate=%d",
		sample.Size(), sample.Format.Codec, sample.Format.Channels, sample.Format.SampleRate)

	select {
	case gb.audioBuffer <- sample:
		// Trace级别: 记录样本成功加入缓冲区
		gb.logger.Trace("Audio sample queued for processing")
		return nil
	case <-gb.ctx.Done():
		// Warn级别: 样本处理异常 - 上下文取消
		gb.logger.Warn("Audio sample processing cancelled: context done")
		return gb.ctx.Err()
	default:
		// Warn级别: 样本处理异常 - 缓冲区满
		gb.mutex.Lock()
		gb.stats.AudioSamplesDropped++
		droppedCount := gb.stats.AudioSamplesDropped
		gb.mutex.Unlock()

		gb.logger.Warnf("Audio sample dropped: buffer full (total dropped: %d)", droppedCount)
		return fmt.Errorf("audio buffer full, sample dropped")
	}
}

// GetStats 获取桥接器统计信息
func (gb *gstreamerBridgeImpl) GetStats() BridgeStats {
	gb.mutex.RLock()
	defer gb.mutex.RUnlock()

	// 计算实时统计数据
	stats := gb.stats
	elapsed := time.Since(gb.startTime).Seconds()

	if elapsed > 0 {
		stats.VideoSampleRate = float64(stats.VideoSamplesProcessed) / elapsed
		stats.AudioSampleRate = float64(stats.AudioSamplesProcessed) / elapsed
	}

	if stats.VideoSamplesProcessed > 0 {
		stats.AvgVideoProcessTime = float64(stats.VideoProcessingTime.Nanoseconds()) / float64(stats.VideoSamplesProcessed) / 1e6
	}

	if stats.AudioSamplesProcessed > 0 {
		stats.AvgAudioProcessTime = float64(stats.AudioProcessingTime.Nanoseconds()) / float64(stats.AudioSamplesProcessed) / 1e6
	}

	stats.HealthStatus = gb.calculateHealthStatus()
	stats.LastHealthCheck = gb.lastHealthCheck

	return stats
}

// ValidateProcessingStatus 验证样本处理状态
func (gb *gstreamerBridgeImpl) ValidateProcessingStatus() error {
	gb.mutex.RLock()
	defer gb.mutex.RUnlock()

	now := time.Now()
	elapsed := now.Sub(gb.startTime).Seconds()

	var issues []string

	// 检查样本处理速率
	if elapsed > 5.0 { // 运行超过5秒后开始检查
		videoRate := float64(gb.stats.VideoSamplesProcessed) / elapsed
		audioRate := float64(gb.stats.AudioSamplesProcessed) / elapsed

		// Debug级别: 记录样本处理统计和速率信息
		gb.logger.Debugf("Sample processing rates - Video: %.2f samples/sec, Audio: %.2f samples/sec", videoRate, audioRate)

		if videoRate > gb.maxSampleRate {
			issues = append(issues, fmt.Sprintf("video sample rate too high: %.2f/s", videoRate))
		}
		if videoRate < gb.minSampleRate && gb.stats.VideoSamplesProcessed > 0 {
			issues = append(issues, fmt.Sprintf("video sample rate too low: %.2f/s", videoRate))
		}
		if audioRate > gb.maxSampleRate {
			issues = append(issues, fmt.Sprintf("audio sample rate too high: %.2f/s", audioRate))
		}
	}

	// 检查错误率
	if gb.stats.VideoSamplesProcessed > 0 {
		videoErrorRate := float64(gb.stats.VideoErrorCount) / float64(gb.stats.VideoSamplesProcessed)
		if videoErrorRate > gb.maxErrorRate {
			issues = append(issues, fmt.Sprintf("video error rate too high: %.2f%%", videoErrorRate*100))
		}
	}

	if gb.stats.AudioSamplesProcessed > 0 {
		audioErrorRate := float64(gb.stats.AudioErrorCount) / float64(gb.stats.AudioSamplesProcessed)
		if audioErrorRate > gb.maxErrorRate {
			issues = append(issues, fmt.Sprintf("audio error rate too high: %.2f%%", audioErrorRate*100))
		}
	}

	// 检查最近活动
	if !gb.stats.LastVideoSample.IsZero() && now.Sub(gb.stats.LastVideoSample) > time.Minute {
		issues = append(issues, "no video samples received in the last minute")
	}

	if !gb.stats.LastAudioSample.IsZero() && now.Sub(gb.stats.LastAudioSample) > time.Minute {
		issues = append(issues, "no audio samples received in the last minute")
	}

	if len(issues) > 0 {
		// Info级别: 记录样本处理状态摘要和异常告警
		gb.logger.Infof("Sample processing validation failed: %d issues detected", len(issues))
		for _, issue := range issues {
			gb.logger.Warnf("Processing issue: %s", issue)
		}
		return fmt.Errorf("sample processing validation failed: %v", issues)
	}

	// Info级别: 记录样本处理状态摘要（通过）
	gb.logger.Info("Sample processing validation passed")
	return nil
}

// GetHealthStatus 获取健康状态摘要
func (gb *gstreamerBridgeImpl) GetHealthStatus() string {
	gb.mutex.RLock()
	defer gb.mutex.RUnlock()
	return gb.calculateHealthStatus()
}

// calculateHealthStatus 计算健康状态（内部方法，调用时需要持有锁）
func (gb *gstreamerBridgeImpl) calculateHealthStatus() string {
	if !gb.running {
		return "stopped"
	}

	now := time.Now()
	elapsed := now.Sub(gb.startTime).Seconds()

	// 检查基本运行状态
	if elapsed < 5.0 {
		return "starting"
	}

	// 检查样本处理活动
	hasVideoActivity := gb.stats.VideoSamplesProcessed > 0 &&
		(gb.stats.LastVideoSample.IsZero() || now.Sub(gb.stats.LastVideoSample) < time.Minute)
	hasAudioActivity := gb.stats.AudioSamplesProcessed > 0 &&
		(gb.stats.LastAudioSample.IsZero() || now.Sub(gb.stats.LastAudioSample) < time.Minute)

	// 检查错误率
	videoErrorRate := float64(0)
	audioErrorRate := float64(0)

	if gb.stats.VideoSamplesProcessed > 0 {
		videoErrorRate = float64(gb.stats.VideoErrorCount) / float64(gb.stats.VideoSamplesProcessed)
	}
	if gb.stats.AudioSamplesProcessed > 0 {
		audioErrorRate = float64(gb.stats.AudioErrorCount) / float64(gb.stats.AudioSamplesProcessed)
	}

	// 检查丢弃率
	videoDropRate := float64(0)
	audioDropRate := float64(0)

	totalVideoSamples := gb.stats.VideoSamplesProcessed + gb.stats.VideoSamplesDropped
	totalAudioSamples := gb.stats.AudioSamplesProcessed + gb.stats.AudioSamplesDropped

	if totalVideoSamples > 0 {
		videoDropRate = float64(gb.stats.VideoSamplesDropped) / float64(totalVideoSamples)
	}
	if totalAudioSamples > 0 {
		audioDropRate = float64(gb.stats.AudioSamplesDropped) / float64(totalAudioSamples)
	}

	// 判断健康状态
	if (videoErrorRate > gb.maxErrorRate) || (audioErrorRate > gb.maxErrorRate) {
		return "error"
	}

	if (videoDropRate > 0.1) || (audioDropRate > 0.1) { // 丢弃率超过10%
		return "degraded"
	}

	if !hasVideoActivity && !hasAudioActivity {
		return "idle"
	}

	return "healthy"
}

// checkAndReportStats 检查并报告统计信息（内部方法，调用时需要持有锁）
func (gb *gstreamerBridgeImpl) checkAndReportStats() {
	now := time.Now()

	// 定期统计报告
	if now.Sub(gb.lastStatsReport) >= gb.statsReportInterval {
		elapsed := now.Sub(gb.startTime).Seconds()

		// Info级别: 记录样本处理状态摘要
		gb.logger.Infof("Sample processing status report:")
		gb.logger.Infof("  Video: %d processed, %d dropped, %d errors (%.2f/s)",
			gb.stats.VideoSamplesProcessed, gb.stats.VideoSamplesDropped, gb.stats.VideoErrorCount,
			float64(gb.stats.VideoSamplesProcessed)/elapsed)
		gb.logger.Infof("  Audio: %d processed, %d dropped, %d errors (%.2f/s)",
			gb.stats.AudioSamplesProcessed, gb.stats.AudioSamplesDropped, gb.stats.AudioErrorCount,
			float64(gb.stats.AudioSamplesProcessed)/elapsed)

		// Debug级别: 记录详细的处理时间统计
		if gb.stats.VideoSamplesProcessed > 0 {
			avgVideoTime := float64(gb.stats.VideoProcessingTime.Nanoseconds()) / float64(gb.stats.VideoSamplesProcessed) / 1e6
			gb.logger.Debugf("  Video avg processing time: %.2fms", avgVideoTime)
		}
		if gb.stats.AudioSamplesProcessed > 0 {
			avgAudioTime := float64(gb.stats.AudioProcessingTime.Nanoseconds()) / float64(gb.stats.AudioSamplesProcessed) / 1e6
			gb.logger.Debugf("  Audio avg processing time: %.2fms", avgAudioTime)
		}

		gb.lastStatsReport = now
	}

	// 定期健康检查
	if now.Sub(gb.lastHealthCheck) >= gb.healthCheckInterval {
		healthStatus := gb.calculateHealthStatus()

		// Info级别: 记录健康状态变化
		if healthStatus != gb.stats.HealthStatus {
			gb.logger.Infof("Sample processing health status changed: %s -> %s", gb.stats.HealthStatus, healthStatus)
		}

		gb.stats.HealthStatus = healthStatus
		gb.lastHealthCheck = now

		// 如果状态异常，进行详细验证
		if healthStatus == "error" || healthStatus == "degraded" {
			// 临时释放锁进行验证（避免死锁）
			gb.mutex.Unlock()
			if err := gb.ValidateProcessingStatus(); err != nil {
				// Info级别: 记录异常告警
				gb.logger.Warnf("Sample processing health check failed: %v", err)
			}
			gb.mutex.Lock()
		}
	}
}

// processVideoSamples 处理视频样本的协程
func (gb *gstreamerBridgeImpl) processVideoSamples() {
	defer gb.wg.Done()

	// Debug级别: 记录视频样本处理协程启动
	gb.logger.Debug("Video sample processing goroutine started")

	for {
		select {
		case sample, ok := <-gb.videoBuffer:
			if !ok {
				// Debug级别: 记录协程正常退出
				gb.logger.Debug("Video sample processing goroutine stopped: channel closed")
				return
			}

			start := time.Now()
			err := gb.convertAndWriteVideoSample(sample)
			processingTime := time.Since(start)

			gb.mutex.Lock()
			if err == nil {
				gb.stats.VideoSamplesProcessed++
				gb.stats.LastVideoSample = time.Now()
				processedCount := gb.stats.VideoSamplesProcessed

				// Debug级别: 记录样本处理统计（每 1000 个样本）
				if processedCount%1000 == 0 {
					avgTime := float64(gb.stats.VideoProcessingTime.Nanoseconds()) / float64(processedCount) / 1e6
					elapsed := time.Since(gb.startTime).Seconds()
					rate := float64(processedCount) / elapsed
					gb.logger.Debugf("Video sample processing milestone: %d samples processed, avg_time=%.2fms, rate=%.2f/s",
						processedCount, avgTime, rate)
				}
			} else {
				gb.stats.VideoSamplesDropped++
				gb.stats.VideoErrorCount++
				// Warn级别: 样本处理异常
				gb.logger.Warnf("Video sample processing failed: %v (processing_time=%.2fms)",
					err, float64(processingTime.Nanoseconds())/1e6)
			}
			gb.stats.VideoProcessingTime += processingTime

			// 定期报告统计信息和健康检查
			gb.checkAndReportStats()
			gb.mutex.Unlock()

			// Trace级别: 记录详细的样本处理时间
			gb.logger.Tracef("Video sample processed in %.2fms", float64(processingTime.Nanoseconds())/1e6)

		case <-gb.ctx.Done():
			// Debug级别: 记录协程因上下文取消而退出
			gb.logger.Debug("Video sample processing goroutine stopped: context cancelled")
			return
		}
	}
}

// processAudioSamples 处理音频样本的协程
func (gb *gstreamerBridgeImpl) processAudioSamples() {
	defer gb.wg.Done()

	// Debug级别: 记录音频样本处理协程启动
	gb.logger.Debug("Audio sample processing goroutine started")

	for {
		select {
		case sample, ok := <-gb.audioBuffer:
			if !ok {
				// Debug级别: 记录协程正常退出
				gb.logger.Debug("Audio sample processing goroutine stopped: channel closed")
				return
			}

			start := time.Now()
			err := gb.convertAndWriteAudioSample(sample)
			processingTime := time.Since(start)

			gb.mutex.Lock()
			if err == nil {
				gb.stats.AudioSamplesProcessed++
				gb.stats.LastAudioSample = time.Now()
				processedCount := gb.stats.AudioSamplesProcessed

				// Debug级别: 记录样本处理统计（每 1000 个样本）
				if processedCount%1000 == 0 {
					avgTime := float64(gb.stats.AudioProcessingTime.Nanoseconds()) / float64(processedCount) / 1e6
					elapsed := time.Since(gb.startTime).Seconds()
					rate := float64(processedCount) / elapsed
					gb.logger.Debugf("Audio sample processing milestone: %d samples processed, avg_time=%.2fms, rate=%.2f/s",
						processedCount, avgTime, rate)
				}
			} else {
				gb.stats.AudioSamplesDropped++
				gb.stats.AudioErrorCount++
				// Warn级别: 样本处理异常
				gb.logger.Warnf("Audio sample processing failed: %v (processing_time=%.2fms)",
					err, float64(processingTime.Nanoseconds())/1e6)
			}
			gb.stats.AudioProcessingTime += processingTime

			// 定期报告统计信息和健康检查
			gb.checkAndReportStats()
			gb.mutex.Unlock()

			// Trace级别: 记录详细的样本处理时间
			gb.logger.Tracef("Audio sample processed in %.2fms", float64(processingTime.Nanoseconds())/1e6)

		case <-gb.ctx.Done():
			// Debug级别: 记录协程因上下文取消而退出
			gb.logger.Debug("Audio sample processing goroutine stopped: context cancelled")
			return
		}
	}
}

// convertAndWriteVideoSample 转换并写入视频样本
func (gb *gstreamerBridgeImpl) convertAndWriteVideoSample(gstSample *gstreamer.Sample) error {
	conversionStart := time.Now()

	// Trace级别: 记录样本转换开始
	gb.logger.Tracef("Converting video sample: size=%d bytes, format=%s, dimensions=%dx%d, timestamp=%v",
		len(gstSample.Data), gstSample.Format.Codec, gstSample.Format.Width, gstSample.Format.Height, gstSample.Timestamp)

	// 验证样本数据
	if len(gstSample.Data) == 0 {
		// Trace级别: 记录详细的样本处理错误信息
		gb.logger.Trace("Video sample conversion failed: empty data")
		return fmt.Errorf("empty video sample data")
	}

	// 转换GStreamer样本为WebRTC样本
	webrtcSample := media.Sample{
		Data:     gstSample.Data,
		Duration: gstSample.Duration,
	}

	// 计算时间戳 - 使用GStreamer样本的时间戳或计算相对时间戳
	if !gstSample.Timestamp.IsZero() {
		webrtcSample.Timestamp = gstSample.Timestamp
		// Trace级别: 记录时间戳使用原始值
		gb.logger.Tracef("Using original timestamp: %v", gstSample.Timestamp)
	} else {
		elapsed := time.Since(gb.startTime)
		gb.videoTimestamp = uint32((elapsed.Nanoseconds() * int64(gb.config.VideoClockRate)) / int64(time.Second))
		webrtcSample.Timestamp = gb.startTime.Add(elapsed)
		// Trace级别: 记录时间戳计算
		gb.logger.Tracef("Calculated timestamp: %v (elapsed=%v)", webrtcSample.Timestamp, elapsed)
	}

	conversionTime := time.Since(conversionStart)

	// Trace级别: 记录样本转换时间
	gb.logger.Tracef("Video sample conversion completed in %.2fms", float64(conversionTime.Nanoseconds())/1e6)

	// 检查转换时间是否异常
	if conversionTime > gb.maxProcessingTime {
		// Trace级别: 记录详细的样本处理时间异常
		gb.logger.Tracef("Video sample conversion time exceeded threshold: %.2fms > %.2fms",
			float64(conversionTime.Nanoseconds())/1e6, float64(gb.maxProcessingTime.Nanoseconds())/1e6)
	}

	writeStart := time.Now()

	// Trace级别: 记录写入MediaStream开始
	gb.logger.Tracef("Writing video sample to MediaStream: size=%d bytes", len(webrtcSample.Data))

	// 写入媒体流
	err := gb.mediaStream.WriteVideo(webrtcSample)

	writeTime := time.Since(writeStart)

	if err != nil {
		// Trace级别: 记录详细的样本处理错误信息
		gb.logger.Tracef("Failed to write video sample to MediaStream: %v (write_time=%.2fms)",
			err, float64(writeTime.Nanoseconds())/1e6)
	} else {
		// Trace级别: 记录成功写入和时间
		gb.logger.Tracef("Video sample written to MediaStream successfully (write_time=%.2fms)",
			float64(writeTime.Nanoseconds())/1e6)
	}

	// 检查写入时间是否异常
	if writeTime > gb.maxProcessingTime {
		// Trace级别: 记录详细的样本处理时间异常
		gb.logger.Tracef("Video sample write time exceeded threshold: %.2fms > %.2fms",
			float64(writeTime.Nanoseconds())/1e6, float64(gb.maxProcessingTime.Nanoseconds())/1e6)
	}

	return err
}

// convertAndWriteAudioSample 转换并写入音频样本
func (gb *gstreamerBridgeImpl) convertAndWriteAudioSample(gstSample *gstreamer.Sample) error {
	conversionStart := time.Now()

	// Trace级别: 记录样本转换开始
	gb.logger.Tracef("Converting audio sample: size=%d bytes, format=%s, channels=%d, sample_rate=%d, timestamp=%v",
		len(gstSample.Data), gstSample.Format.Codec, gstSample.Format.Channels, gstSample.Format.SampleRate, gstSample.Timestamp)

	// 验证样本数据
	if len(gstSample.Data) == 0 {
		// Trace级别: 记录详细的样本处理错误信息
		gb.logger.Trace("Audio sample conversion failed: empty data")
		return fmt.Errorf("empty audio sample data")
	}

	// 转换GStreamer样本为WebRTC样本
	webrtcSample := media.Sample{
		Data:     gstSample.Data,
		Duration: gstSample.Duration,
	}

	// 计算时间戳 - 使用GStreamer样本的时间戳或计算相对时间戳
	if !gstSample.Timestamp.IsZero() {
		webrtcSample.Timestamp = gstSample.Timestamp
		// Trace级别: 记录时间戳使用原始值
		gb.logger.Tracef("Using original timestamp: %v", gstSample.Timestamp)
	} else {
		elapsed := time.Since(gb.startTime)
		gb.audioTimestamp = uint32((elapsed.Nanoseconds() * int64(gb.config.AudioClockRate)) / int64(time.Second))
		webrtcSample.Timestamp = gb.startTime.Add(elapsed)
		// Trace级别: 记录时间戳计算
		gb.logger.Tracef("Calculated timestamp: %v (elapsed=%v)", webrtcSample.Timestamp, elapsed)
	}

	conversionTime := time.Since(conversionStart)

	// Trace级别: 记录样本转换时间
	gb.logger.Tracef("Audio sample conversion completed in %.2fms", float64(conversionTime.Nanoseconds())/1e6)

	// 检查转换时间是否异常
	if conversionTime > gb.maxProcessingTime {
		// Trace级别: 记录详细的样本处理时间异常
		gb.logger.Tracef("Audio sample conversion time exceeded threshold: %.2fms > %.2fms",
			float64(conversionTime.Nanoseconds())/1e6, float64(gb.maxProcessingTime.Nanoseconds())/1e6)
	}

	writeStart := time.Now()

	// Trace级别: 记录写入MediaStream开始
	gb.logger.Tracef("Writing audio sample to MediaStream: size=%d bytes", len(webrtcSample.Data))

	// 写入媒体流
	err := gb.mediaStream.WriteAudio(webrtcSample)

	writeTime := time.Since(writeStart)

	if err != nil {
		// Trace级别: 记录详细的样本处理错误信息
		gb.logger.Tracef("Failed to write audio sample to MediaStream: %v (write_time=%.2fms)",
			err, float64(writeTime.Nanoseconds())/1e6)
	} else {
		// Trace级别: 记录成功写入和时间
		gb.logger.Tracef("Audio sample written to MediaStream successfully (write_time=%.2fms)",
			float64(writeTime.Nanoseconds())/1e6)
	}

	// 检查写入时间是否异常
	if writeTime > gb.maxProcessingTime {
		// Trace级别: 记录详细的样本处理时间异常
		gb.logger.Tracef("Audio sample write time exceeded threshold: %.2fms > %.2fms",
			float64(writeTime.Nanoseconds())/1e6, float64(gb.maxProcessingTime.Nanoseconds())/1e6)
	}

	return err
}

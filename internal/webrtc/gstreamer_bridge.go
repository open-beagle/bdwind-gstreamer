package webrtc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"

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
}

// gstreamerBridgeImpl GStreamer桥接器实现
type gstreamerBridgeImpl struct {
	config      *GStreamerBridgeConfig
	mediaStream MediaStream

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

	encodedSampleCallback func(*gstreamer.Sample) error
}

// NewGStreamerBridge 创建新的GStreamer桥接器
func NewGStreamerBridge(config *GStreamerBridgeConfig) GStreamerBridge {
	if config == nil {
		config = DefaultGStreamerBridgeConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &gstreamerBridgeImpl{
		config:      config,
		videoBuffer: make(chan *gstreamer.Sample, config.VideoBufferSize),
		audioBuffer: make(chan *gstreamer.Sample, config.AudioBufferSize),
		ctx:         ctx,
		cancel:      cancel,
		startTime:   time.Now(),
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
		return fmt.Errorf("bridge already running")
	}

	if gb.mediaStream == nil {
		return fmt.Errorf("media stream not set")
	}

	gb.running = true
	gb.startTime = time.Now()

	// 启动视频处理协程
	gb.wg.Add(1)
	go gb.processVideoSamples()

	// 启动音频处理协程
	gb.wg.Add(1)
	go gb.processAudioSamples()

	return nil
}

// Stop 停止桥接器
func (gb *gstreamerBridgeImpl) Stop() error {
	gb.mutex.Lock()
	if !gb.running {
		gb.mutex.Unlock()
		return nil
	}
	gb.running = false
	gb.mutex.Unlock()

	// 取消上下文
	gb.cancel()

	// 关闭缓冲区
	close(gb.videoBuffer)
	close(gb.audioBuffer)

	// 等待所有协程结束
	gb.wg.Wait()

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
		return fmt.Errorf("bridge not running")
	}

	select {
	case gb.videoBuffer <- sample:
		return nil
	case <-gb.ctx.Done():
		return gb.ctx.Err()
	default:
		// 缓冲区满，丢弃样本
		gb.mutex.Lock()
		gb.stats.VideoSamplesDropped++
		gb.mutex.Unlock()
		return fmt.Errorf("video buffer full, sample dropped")
	}
}

// ProcessAudioSample 处理音频样本
func (gb *gstreamerBridgeImpl) ProcessAudioSample(sample *gstreamer.Sample) error {
	gb.mutex.RLock()
	running := gb.running
	gb.mutex.RUnlock()

	if !running {
		return fmt.Errorf("bridge not running")
	}

	select {
	case gb.audioBuffer <- sample:
		return nil
	case <-gb.ctx.Done():
		return gb.ctx.Err()
	default:
		// 缓冲区满，丢弃样本
		gb.mutex.Lock()
		gb.stats.AudioSamplesDropped++
		gb.mutex.Unlock()
		return fmt.Errorf("audio buffer full, sample dropped")
	}
}

// GetStats 获取桥接器统计信息
func (gb *gstreamerBridgeImpl) GetStats() BridgeStats {
	gb.mutex.RLock()
	defer gb.mutex.RUnlock()
	return gb.stats
}

// processVideoSamples 处理视频样本的协程
func (gb *gstreamerBridgeImpl) processVideoSamples() {
	defer gb.wg.Done()

	for {
		select {
		case sample, ok := <-gb.videoBuffer:
			if !ok {
				return
			}

			start := time.Now()
			err := gb.convertAndWriteVideoSample(sample)
			processingTime := time.Since(start)

			gb.mutex.Lock()
			if err == nil {
				gb.stats.VideoSamplesProcessed++
				gb.stats.LastVideoSample = time.Now()
			} else {
				gb.stats.VideoSamplesDropped++
			}
			gb.stats.VideoProcessingTime += processingTime
			gb.mutex.Unlock()

		case <-gb.ctx.Done():
			return
		}
	}
}

// processAudioSamples 处理音频样本的协程
func (gb *gstreamerBridgeImpl) processAudioSamples() {
	defer gb.wg.Done()

	for {
		select {
		case sample, ok := <-gb.audioBuffer:
			if !ok {
				return
			}

			start := time.Now()
			err := gb.convertAndWriteAudioSample(sample)
			processingTime := time.Since(start)

			gb.mutex.Lock()
			if err == nil {
				gb.stats.AudioSamplesProcessed++
				gb.stats.LastAudioSample = time.Now()
			} else {
				gb.stats.AudioSamplesDropped++
			}
			gb.stats.AudioProcessingTime += processingTime
			gb.mutex.Unlock()

		case <-gb.ctx.Done():
			return
		}
	}
}

// convertAndWriteVideoSample 转换并写入视频样本
func (gb *gstreamerBridgeImpl) convertAndWriteVideoSample(gstSample *gstreamer.Sample) error {
	fmt.Printf("Bridge: Converting video sample - size=%d bytes, format=%s, dimensions=%dx%d\n",
		len(gstSample.Data), gstSample.Format.Codec, gstSample.Format.Width, gstSample.Format.Height)

	// 转换GStreamer样本为WebRTC样本
	webrtcSample := media.Sample{
		Data:     gstSample.Data,
		Duration: gstSample.Duration,
	}

	// 计算时间戳 - 使用GStreamer样本的时间戳或计算相对时间戳
	if !gstSample.Timestamp.IsZero() {
		webrtcSample.Timestamp = gstSample.Timestamp
	} else {
		elapsed := time.Since(gb.startTime)
		gb.videoTimestamp = uint32((elapsed.Nanoseconds() * int64(gb.config.VideoClockRate)) / int64(time.Second))
		webrtcSample.Timestamp = gb.startTime.Add(elapsed)
	}

	fmt.Printf("Bridge: Writing video sample to WebRTC - size=%d bytes\n", len(webrtcSample.Data))

	// 写入媒体流
	err := gb.mediaStream.WriteVideo(webrtcSample)
	if err != nil {
		fmt.Printf("Bridge: Error writing video sample: %v\n", err)
	} else {
		fmt.Printf("Bridge: Video sample written successfully\n")
	}
	return err
}

// convertAndWriteAudioSample 转换并写入音频样本
func (gb *gstreamerBridgeImpl) convertAndWriteAudioSample(gstSample *gstreamer.Sample) error {
	// 转换GStreamer样本为WebRTC样本
	webrtcSample := media.Sample{
		Data:     gstSample.Data,
		Duration: gstSample.Duration,
	}

	// 计算时间戳 - 使用GStreamer样本的时间戳或计算相对时间戳
	if !gstSample.Timestamp.IsZero() {
		webrtcSample.Timestamp = gstSample.Timestamp
	} else {
		elapsed := time.Since(gb.startTime)
		gb.audioTimestamp = uint32((elapsed.Nanoseconds() * int64(gb.config.AudioClockRate)) / int64(time.Second))
		webrtcSample.Timestamp = gb.startTime.Add(elapsed)
	}

	// 写入媒体流
	return gb.mediaStream.WriteAudio(webrtcSample)
}

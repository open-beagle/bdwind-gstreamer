package webrtc

import (
	"fmt"
	"log"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
)

// ExampleSampleProcessingValidation demonstrates the sample processing validation functionality
func ExampleSampleProcessingValidation() {
	// 创建桥接器配置
	config := DefaultGStreamerBridgeConfig()
	config.VideoBufferSize = 50
	config.AudioBufferSize = 100

	// 创建桥接器
	bridge := NewGStreamerBridge(config)

	// 创建模拟MediaStream
	mediaStreamConfig := DefaultMediaStreamConfig()
	mediaStream, err := NewMediaStream(mediaStreamConfig)
	if err != nil {
		log.Fatalf("Failed to create MediaStream: %v", err)
	}

	// 设置MediaStream到桥接器
	err = bridge.SetMediaStream(mediaStream)
	if err != nil {
		log.Fatalf("Failed to set MediaStream: %v", err)
	}

	// 启动桥接器
	err = bridge.Start()
	if err != nil {
		log.Fatalf("Failed to start bridge: %v", err)
	}

	fmt.Println("=== Sample Processing Validation Example ===")

	// 获取初始统计信息
	stats := bridge.GetStats()
	fmt.Printf("Initial stats: Video=%d, Audio=%d, Health=%s\n",
		stats.VideoSamplesProcessed, stats.AudioSamplesProcessed, stats.HealthStatus)

	// 创建测试样本
	videoSample := &gstreamer.Sample{
		Data:      make([]byte, 1024),    // 1KB 视频数据
		Duration:  time.Millisecond * 33, // ~30fps
		Timestamp: time.Now(),
		Format: gstreamer.SampleFormat{
			MediaType: gstreamer.MediaTypeVideo,
			Codec:     "h264",
			Width:     1920,
			Height:    1080,
		},
	}

	audioSample := &gstreamer.Sample{
		Data:      make([]byte, 512), // 512B 音频数据
		Duration:  time.Millisecond * 20,
		Timestamp: time.Now(),
		Format: gstreamer.SampleFormat{
			MediaType:  gstreamer.MediaTypeAudio,
			Codec:      "opus",
			Channels:   2,
			SampleRate: 48000,
		},
	}

	// 模拟样本处理
	fmt.Println("\n=== Processing Samples ===")
	for i := 0; i < 10; i++ {
		// 处理视频样本
		err = bridge.ProcessVideoSample(videoSample)
		if err != nil {
			fmt.Printf("Video sample %d failed: %v\n", i+1, err)
		}

		// 处理音频样本
		err = bridge.ProcessAudioSample(audioSample)
		if err != nil {
			fmt.Printf("Audio sample %d failed: %v\n", i+1, err)
		}

		// 短暂延迟模拟真实处理间隔
		time.Sleep(time.Millisecond * 10)
	}

	// 等待样本处理完成
	time.Sleep(time.Millisecond * 200)

	// 获取处理后的统计信息
	stats = bridge.GetStats()
	fmt.Printf("\nAfter processing stats:\n")
	fmt.Printf("  Video samples processed: %d\n", stats.VideoSamplesProcessed)
	fmt.Printf("  Audio samples processed: %d\n", stats.AudioSamplesProcessed)
	fmt.Printf("  Video samples dropped: %d\n", stats.VideoSamplesDropped)
	fmt.Printf("  Audio samples dropped: %d\n", stats.AudioSamplesDropped)
	fmt.Printf("  Video sample rate: %.2f/s\n", stats.VideoSampleRate)
	fmt.Printf("  Audio sample rate: %.2f/s\n", stats.AudioSampleRate)
	fmt.Printf("  Avg video processing time: %.2fms\n", stats.AvgVideoProcessTime)
	fmt.Printf("  Avg audio processing time: %.2fms\n", stats.AvgAudioProcessTime)
	fmt.Printf("  Health status: %s\n", stats.HealthStatus)

	// 执行样本处理状态验证
	fmt.Println("\n=== Sample Processing Validation ===")
	err = bridge.ValidateProcessingStatus()
	if err != nil {
		fmt.Printf("Validation failed: %v\n", err)
	} else {
		fmt.Println("Validation passed: All sample processing metrics are within normal ranges")
	}

	// 获取健康状态
	healthStatus := bridge.GetHealthStatus()
	fmt.Printf("Current health status: %s\n", healthStatus)

	// 停止桥接器
	err = bridge.Stop()
	if err != nil {
		log.Fatalf("Failed to stop bridge: %v", err)
	}

	fmt.Println("\n=== Example Complete ===")
}

// ExampleSampleProcessingWithErrors demonstrates error handling and validation
func ExampleSampleProcessingWithErrors() {
	fmt.Println("\n=== Sample Processing Error Handling Example ===")

	// 创建桥接器但不设置MediaStream
	bridge := NewGStreamerBridge(nil)

	// 尝试启动桥接器（应该失败）
	err := bridge.Start()
	if err != nil {
		fmt.Printf("Expected error when starting without MediaStream: %v\n", err)
	}

	// 检查健康状态
	healthStatus := bridge.GetHealthStatus()
	fmt.Printf("Health status when stopped: %s\n", healthStatus)

	// 尝试处理样本（应该失败）
	videoSample := &gstreamer.Sample{
		Data:      make([]byte, 1024),
		Duration:  time.Millisecond * 33,
		Timestamp: time.Now(),
		Format: gstreamer.SampleFormat{
			MediaType: gstreamer.MediaTypeVideo,
			Codec:     "h264",
			Width:     1920,
			Height:    1080,
		},
	}

	err = bridge.ProcessVideoSample(videoSample)
	if err != nil {
		fmt.Printf("Expected error when processing sample on stopped bridge: %v\n", err)
	}

	// 验证处理状态
	err = bridge.ValidateProcessingStatus()
	if err != nil {
		fmt.Printf("Validation result for stopped bridge: %v\n", err)
	} else {
		fmt.Println("Validation passed for stopped bridge (no issues detected)")
	}

	fmt.Println("=== Error Handling Example Complete ===")
}

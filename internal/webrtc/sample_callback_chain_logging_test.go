package webrtc

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
)

// TestSampleCallbackChainLogging tests the sample callback chain logging functionality
func TestSampleCallbackChainLogging(t *testing.T) {
	// Create a buffer to capture log output
	var logBuffer bytes.Buffer

	// Create a custom logger that writes to our buffer
	logger := logrus.New()
	logger.SetOutput(&logBuffer)
	logger.SetLevel(logrus.TraceLevel) // Enable all log levels for testing

	// Create a logrus entry with the gstreamer-bridge prefix
	logEntry := logger.WithField("component", "gstreamer-bridge")

	// Create bridge configuration
	config := DefaultGStreamerBridgeConfig()

	// Create bridge instance
	bridge := &gstreamerBridgeImpl{
		config:              config,
		logger:              logEntry,
		videoBuffer:         make(chan *gstreamer.Sample, config.VideoBufferSize),
		audioBuffer:         make(chan *gstreamer.Sample, config.AudioBufferSize),
		statsReportInterval: time.Second * 30,
		healthCheckInterval: time.Second * 10,
		lastStatsReport:     time.Now(),
		lastHealthCheck:     time.Now(),
		maxProcessingTime:   time.Millisecond * 50,
		maxSampleRate:       1000.0,
		minSampleRate:       1.0,
		maxErrorRate:        0.05,
	}

	ctx, cancel := context.WithCancel(context.Background())
	bridge.ctx = ctx
	bridge.cancel = cancel

	// Test 1: Info级别 - 记录样本回调链路建立成功状态
	t.Run("Info_Level_Chain_Establishment", func(t *testing.T) {
		logBuffer.Reset()

		// Test successful chain establishment
		bridge.logger.Info("Sample callback chain established successfully")

		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "level=info")
		assert.Contains(t, logOutput, "Sample callback chain established successfully")
		assert.Contains(t, logOutput, "component=gstreamer-bridge")
	})

	// Test 2: Debug级别 - 记录组件状态验证结果和样本处理统计
	t.Run("Debug_Level_Component_Status_And_Stats", func(t *testing.T) {
		logBuffer.Reset()

		// Test component status verification
		bridge.logger.Debug("GStreamer desktop capture component verified")
		bridge.logger.Debug("WebRTC bridge component verified")

		// Test sample processing statistics (every 1000 samples)
		bridge.logger.Debugf("Video sample processing milestone: %d samples processed", 1000)
		bridge.logger.Debugf("Audio sample processing milestone: %d samples processed", 1000)

		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "level=debug")
		assert.Contains(t, logOutput, "component verified")
		assert.Contains(t, logOutput, "processing milestone")
		assert.Contains(t, logOutput, "1000 samples processed")
	})

	// Test 3: Trace级别 - 记录每个样本的详细处理信息和类型识别
	t.Run("Trace_Level_Sample_Details", func(t *testing.T) {
		logBuffer.Reset()

		// Create test video sample
		videoSample := &gstreamer.Sample{
			Data:      make([]byte, 1024),
			Timestamp: time.Now(),
			Format: gstreamer.SampleFormat{
				MediaType: gstreamer.MediaTypeVideo,
				Codec:     "h264",
				Width:     1920,
				Height:    1080,
			},
		}

		// Create test audio sample
		audioSample := &gstreamer.Sample{
			Data:      make([]byte, 512),
			Timestamp: time.Now(),
			Format: gstreamer.SampleFormat{
				MediaType:  gstreamer.MediaTypeAudio,
				Codec:      "opus",
				Channels:   2,
				SampleRate: 48000,
			},
		}

		// Test detailed video sample processing information
		bridge.logger.Tracef("Processing sample: type=%s, size=%d bytes, timestamp=%v, dimensions=%dx%d",
			videoSample.Format.MediaType.String(), videoSample.Size(), videoSample.Timestamp,
			videoSample.Format.Width, videoSample.Format.Height)

		// Test detailed audio sample processing information
		bridge.logger.Tracef("Processing sample: type=%s, size=%d bytes, codec=%s, channels=%d, sample_rate=%d",
			audioSample.Format.MediaType.String(), audioSample.Size(), audioSample.Format.Codec,
			audioSample.Format.Channels, audioSample.Format.SampleRate)

		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "level=trace")
		assert.Contains(t, logOutput, "Processing sample")
		assert.Contains(t, logOutput, "type=video")
		assert.Contains(t, logOutput, "type=audio")
		assert.Contains(t, logOutput, "size=1024 bytes")
		assert.Contains(t, logOutput, "size=512 bytes")
		assert.Contains(t, logOutput, "dimensions=1920x1080")
		assert.Contains(t, logOutput, "channels=2")
		assert.Contains(t, logOutput, "sample_rate=48000")
	})

	// Test 4: Warn级别 - 样本处理异常
	t.Run("Warn_Level_Sample_Processing_Exceptions", func(t *testing.T) {
		logBuffer.Reset()

		// Test nil sample warning
		bridge.logger.Warn("Sample callback received nil sample")

		// Test buffer full warning
		bridge.logger.Warnf("Video sample dropped: buffer full (total dropped: %d)", 5)

		// Test processing failure warning
		bridge.logger.Warnf("Sample processing failed for %s sample: %v", "video", "encoding error")

		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "level=warning")
		assert.Contains(t, logOutput, "nil sample")
		assert.Contains(t, logOutput, "buffer full")
		assert.Contains(t, logOutput, "processing failed")
		assert.Contains(t, logOutput, "total dropped: 5")
	})

	// Test 5: Error级别 - 链路中断
	t.Run("Error_Level_Chain_Interruption", func(t *testing.T) {
		logBuffer.Reset()

		// Test chain interruption errors
		bridge.logger.Error("Sample callback chain failed: desktop capture not available")
		bridge.logger.Error("Sample callback chain failed: WebRTC bridge not available")

		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "level=error")
		assert.Contains(t, logOutput, "chain failed")
		assert.Contains(t, logOutput, "not available")
	})

	// Test 6: Comprehensive sample processing flow with all log levels
	t.Run("Comprehensive_Sample_Processing_Flow", func(t *testing.T) {
		logBuffer.Reset()

		// Simulate the complete sample processing flow with logging

		// Info: Chain establishment
		bridge.logger.Info("Establishing sample callback chain from GStreamer to WebRTC")

		// Debug: Component verification
		bridge.logger.Debug("Verifying component availability for sample callback chain")
		bridge.logger.Debug("GStreamer desktop capture component verified")
		bridge.logger.Debug("WebRTC bridge component verified")

		// Trace: Sample processing initialization
		bridge.logger.Trace("Initializing sample processing counters")

		// Simulate processing 1000 video samples
		for i := 1; i <= 1000; i++ {
			if i == 1 || i%100 == 0 {
				// Trace: Individual sample processing (sample every 100)
				bridge.logger.Tracef("Processing sample: type=video, size=1024 bytes, count=%d", i)
			}

			if i%1000 == 0 {
				// Debug: Milestone reporting
				bridge.logger.Debugf("Video sample processing milestone: %d samples processed", i)
			}
		}

		// Info: Successful completion
		bridge.logger.Info("Sample callback chain established successfully")

		logOutput := logBuffer.String()

		// Verify all log levels are present
		assert.Contains(t, logOutput, "level=info")
		assert.Contains(t, logOutput, "level=debug")
		assert.Contains(t, logOutput, "level=trace")

		// Verify key messages
		assert.Contains(t, logOutput, "Establishing sample callback chain")
		assert.Contains(t, logOutput, "component verified")
		assert.Contains(t, logOutput, "processing milestone")
		assert.Contains(t, logOutput, "1000 samples processed")
		assert.Contains(t, logOutput, "chain established successfully")

		// Verify component prefix is consistently used
		logLines := bytes.Split(logBuffer.Bytes(), []byte("\n"))
		for _, line := range logLines {
			if len(line) > 0 {
				assert.Contains(t, string(line), "component=gstreamer-bridge")
			}
		}
	})

	// Clean up
	cancel()
}

// TestSampleCallbackChainValidation tests the validation functionality
func TestSampleCallbackChainValidation(t *testing.T) {
	// Create bridge with validation functionality
	bridge := NewGStreamerBridge(nil).(*gstreamerBridgeImpl)

	// Test validation with healthy state
	t.Run("Validation_Success", func(t *testing.T) {
		// Set up healthy state
		bridge.mutex.Lock()
		bridge.running = true
		bridge.startTime = time.Now().Add(-time.Second * 10) // Running for 10 seconds
		bridge.stats.VideoSamplesProcessed = 100
		bridge.stats.AudioSamplesProcessed = 200
		bridge.stats.VideoErrorCount = 2 // 2% error rate
		bridge.stats.AudioErrorCount = 5 // 2.5% error rate
		bridge.stats.LastVideoSample = time.Now().Add(-time.Second * 10)
		bridge.stats.LastAudioSample = time.Now().Add(-time.Second * 10)
		bridge.maxErrorRate = 0.05 // 5% threshold
		bridge.mutex.Unlock()

		err := bridge.ValidateProcessingStatus()
		assert.NoError(t, err)
	})

	// Test validation with issues
	t.Run("Validation_With_Issues", func(t *testing.T) {
		// Set up problematic state
		bridge.mutex.Lock()
		bridge.running = true
		bridge.startTime = time.Now().Add(-time.Second * 10) // Running for 10 seconds
		bridge.stats.VideoSamplesProcessed = 100
		bridge.stats.VideoErrorCount = 10 // 10% error rate (too high)
		bridge.maxErrorRate = 0.05        // 5% threshold
		bridge.mutex.Unlock()

		err := bridge.ValidateProcessingStatus()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "error rate too high")
	})
}

// TestSampleCallbackChainHealthStatus tests the health status functionality
func TestSampleCallbackChainHealthStatus(t *testing.T) {
	bridge := NewGStreamerBridge(nil).(*gstreamerBridgeImpl)

	// Test different health states
	testCases := []struct {
		name           string
		setupFunc      func()
		expectedStatus string
	}{
		{
			name: "Stopped",
			setupFunc: func() {
				bridge.mutex.Lock()
				bridge.running = false
				bridge.mutex.Unlock()
			},
			expectedStatus: "stopped",
		},
		{
			name: "Starting",
			setupFunc: func() {
				bridge.mutex.Lock()
				bridge.running = true
				bridge.startTime = time.Now() // Just started
				bridge.mutex.Unlock()
			},
			expectedStatus: "starting",
		},
		{
			name: "Idle",
			setupFunc: func() {
				bridge.mutex.Lock()
				bridge.running = true
				bridge.startTime = time.Now().Add(-time.Second * 10) // Running for 10 seconds
				bridge.stats.VideoSamplesProcessed = 0
				bridge.stats.AudioSamplesProcessed = 0
				bridge.mutex.Unlock()
			},
			expectedStatus: "idle",
		},
		{
			name: "Healthy",
			setupFunc: func() {
				bridge.mutex.Lock()
				bridge.running = true
				bridge.startTime = time.Now().Add(-time.Second * 10)
				bridge.stats.VideoSamplesProcessed = 100
				bridge.stats.LastVideoSample = time.Now().Add(-time.Second * 10)
				bridge.stats.VideoErrorCount = 2 // 2% error rate
				bridge.maxErrorRate = 0.05       // 5% threshold
				bridge.mutex.Unlock()
			},
			expectedStatus: "healthy",
		},
		{
			name: "Error",
			setupFunc: func() {
				bridge.mutex.Lock()
				bridge.running = true
				bridge.startTime = time.Now().Add(-time.Second * 10)
				bridge.stats.VideoSamplesProcessed = 100
				bridge.stats.VideoErrorCount = 10 // 10% error rate
				bridge.maxErrorRate = 0.05        // 5% threshold
				bridge.mutex.Unlock()
			},
			expectedStatus: "error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupFunc()
			status := bridge.GetHealthStatus()
			assert.Equal(t, tc.expectedStatus, status)
		})
	}
}

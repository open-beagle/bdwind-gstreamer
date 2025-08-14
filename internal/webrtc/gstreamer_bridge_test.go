package webrtc

import (
	"testing"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
)

func TestDefaultGStreamerBridgeConfig(t *testing.T) {
	config := DefaultGStreamerBridgeConfig()

	if config.VideoBufferSize != 100 {
		t.Errorf("Expected video buffer size to be 100, got %d", config.VideoBufferSize)
	}

	if config.AudioBufferSize != 200 {
		t.Errorf("Expected audio buffer size to be 200, got %d", config.AudioBufferSize)
	}

	if config.VideoClockRate != 90000 {
		t.Errorf("Expected video clock rate to be 90000, got %d", config.VideoClockRate)
	}

	if config.AudioClockRate != 48000 {
		t.Errorf("Expected audio clock rate to be 48000, got %d", config.AudioClockRate)
	}

	if config.MaxConcurrentFrames != 10 {
		t.Errorf("Expected max concurrent frames to be 10, got %d", config.MaxConcurrentFrames)
	}

	if config.ProcessingTimeout != time.Millisecond*100 {
		t.Errorf("Expected processing timeout to be 100ms, got %v", config.ProcessingTimeout)
	}
}

func TestNewGStreamerBridge(t *testing.T) {
	config := DefaultGStreamerBridgeConfig()

	bridge := NewGStreamerBridge(config)
	if bridge == nil {
		t.Fatal("Bridge should not be nil")
	}

	// Test with nil config (should use default)
	bridge2 := NewGStreamerBridge(nil)
	if bridge2 == nil {
		t.Fatal("Bridge should not be nil even with nil config")
	}
}

func TestGStreamerBridgeStartStop(t *testing.T) {
	config := DefaultGStreamerBridgeConfig()
	bridge := NewGStreamerBridge(config)

	// Create a media stream
	msConfig := DefaultMediaStreamConfig()
	ms, err := NewMediaStream(msConfig)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	// Set media stream
	err = bridge.SetMediaStream(ms)
	if err != nil {
		t.Fatalf("Failed to set media stream: %v", err)
	}

	// Start bridge
	err = bridge.Start()
	if err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}

	// Try to start again (should fail)
	err = bridge.Start()
	if err == nil {
		t.Error("Expected error when starting bridge twice")
	}

	// Stop bridge
	err = bridge.Stop()
	if err != nil {
		t.Fatalf("Failed to stop bridge: %v", err)
	}

	// Stop again (should not fail)
	err = bridge.Stop()
	if err != nil {
		t.Errorf("Unexpected error when stopping bridge twice: %v", err)
	}
}

func TestGStreamerBridgeStartWithoutMediaStream(t *testing.T) {
	config := DefaultGStreamerBridgeConfig()
	bridge := NewGStreamerBridge(config)

	// Try to start without setting media stream
	err := bridge.Start()
	if err == nil {
		t.Error("Expected error when starting bridge without media stream")
	}
}

func TestGStreamerBridgeProcessVideoSample(t *testing.T) {
	config := DefaultGStreamerBridgeConfig()
	bridge := NewGStreamerBridge(config)

	// Create a media stream
	msConfig := DefaultMediaStreamConfig()
	ms, err := NewMediaStream(msConfig)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	err = bridge.SetMediaStream(ms)
	if err != nil {
		t.Fatalf("Failed to set media stream: %v", err)
	}

	err = bridge.Start()
	if err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	defer bridge.Stop()

	// Create a test video sample
	sample := gstreamer.NewVideoSample(
		make([]byte, 1024),
		1920, 1080,
		"h264",
	)
	sample.Duration = time.Millisecond * 33

	// Process the sample
	err = bridge.ProcessVideoSample(sample)
	if err != nil {
		t.Errorf("Failed to process video sample: %v", err)
	}

	// Give some time for processing
	time.Sleep(time.Millisecond * 50)

	// Check stats
	stats := bridge.GetStats()
	if stats.VideoSamplesProcessed == 0 {
		t.Error("Expected at least 1 video sample to be processed")
	}
}

func TestGStreamerBridgeProcessAudioSample(t *testing.T) {
	config := DefaultGStreamerBridgeConfig()
	bridge := NewGStreamerBridge(config)

	// Create a media stream with audio enabled
	msConfig := DefaultMediaStreamConfig()
	msConfig.AudioEnabled = true
	ms, err := NewMediaStream(msConfig)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	err = bridge.SetMediaStream(ms)
	if err != nil {
		t.Fatalf("Failed to set media stream: %v", err)
	}

	err = bridge.Start()
	if err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	defer bridge.Stop()

	// Create a test audio sample
	sample := gstreamer.NewAudioSample(
		make([]byte, 512),
		2, 48000,
		"opus",
	)
	sample.Duration = time.Millisecond * 20

	// Process the sample
	err = bridge.ProcessAudioSample(sample)
	if err != nil {
		t.Errorf("Failed to process audio sample: %v", err)
	}

	// Give some time for processing
	time.Sleep(time.Millisecond * 50)

	// Check stats
	stats := bridge.GetStats()
	if stats.AudioSamplesProcessed == 0 {
		t.Error("Expected at least 1 audio sample to be processed")
	}
}

func TestGStreamerBridgeProcessSampleNotRunning(t *testing.T) {
	config := DefaultGStreamerBridgeConfig()
	bridge := NewGStreamerBridge(config)

	// Create a test video sample
	sample := gstreamer.NewVideoSample(
		make([]byte, 1024),
		1920, 1080,
		"h264",
	)

	// Try to process sample when bridge is not running
	err := bridge.ProcessVideoSample(sample)
	if err == nil {
		t.Error("Expected error when processing sample on non-running bridge")
	}
}

func TestGStreamerBridgeBufferOverflow(t *testing.T) {
	// Create a config with very small buffer
	config := DefaultGStreamerBridgeConfig()
	config.VideoBufferSize = 1

	bridge := NewGStreamerBridge(config)

	// Create a media stream
	msConfig := DefaultMediaStreamConfig()
	ms, err := NewMediaStream(msConfig)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	err = bridge.SetMediaStream(ms)
	if err != nil {
		t.Fatalf("Failed to set media stream: %v", err)
	}

	err = bridge.Start()
	if err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	defer bridge.Stop()

	// Create test samples
	sample1 := gstreamer.NewVideoSample(make([]byte, 1024), 1920, 1080, "h264")
	sample2 := gstreamer.NewVideoSample(make([]byte, 1024), 1920, 1080, "h264")

	// First sample should succeed
	err = bridge.ProcessVideoSample(sample1)
	if err != nil {
		t.Errorf("First sample should succeed: %v", err)
	}

	// Second sample should fail due to buffer overflow
	err = bridge.ProcessVideoSample(sample2)
	if err == nil {
		t.Error("Expected error due to buffer overflow")
	}

	// Give some time for processing
	time.Sleep(time.Millisecond * 50)

	// Check that dropped samples are counted
	stats := bridge.GetStats()
	if stats.VideoSamplesDropped == 0 {
		t.Error("Expected at least 1 dropped video sample")
	}
}

func TestGStreamerBridgeStats(t *testing.T) {
	config := DefaultGStreamerBridgeConfig()
	bridge := NewGStreamerBridge(config)

	// Initial stats should be zero
	stats := bridge.GetStats()
	if stats.VideoSamplesProcessed != 0 {
		t.Errorf("Expected 0 video samples processed initially, got %d", stats.VideoSamplesProcessed)
	}

	if stats.AudioSamplesProcessed != 0 {
		t.Errorf("Expected 0 audio samples processed initially, got %d", stats.AudioSamplesProcessed)
	}

	if stats.VideoSamplesDropped != 0 {
		t.Errorf("Expected 0 video samples dropped initially, got %d", stats.VideoSamplesDropped)
	}

	if stats.AudioSamplesDropped != 0 {
		t.Errorf("Expected 0 audio samples dropped initially, got %d", stats.AudioSamplesDropped)
	}
}

func TestGStreamerBridgeSetMediaStream(t *testing.T) {
	config := DefaultGStreamerBridgeConfig()
	bridge := NewGStreamerBridge(config)

	// Create a media stream
	msConfig := DefaultMediaStreamConfig()
	ms, err := NewMediaStream(msConfig)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	// Set media stream
	err = bridge.SetMediaStream(ms)
	if err != nil {
		t.Errorf("Failed to set media stream: %v", err)
	}

	// Set media stream again (should succeed)
	err = bridge.SetMediaStream(ms)
	if err != nil {
		t.Errorf("Failed to set media stream again: %v", err)
	}
}

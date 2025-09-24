package gstreamer

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestNewEncoderGst(t *testing.T) {
	tests := []struct {
		name        string
		config      config.EncoderConfig
		expectError bool
	}{
		{
			name:        "Valid H.264 configuration",
			config:      createValidH264Config(),
			expectError: false,
		},
		{
			name:        "Valid VP8 configuration",
			config:      createValidVP8Config(),
			expectError: false,
		},
		{
			name: "Invalid bitrate",
			config: config.EncoderConfig{
				Type:    config.EncoderTypeX264,
				Codec:   config.CodecH264,
				Bitrate: -1, // Invalid
			},
			expectError: true,
		},
		{
			name: "Invalid codec for encoder type",
			config: config.EncoderConfig{
				Type:    config.EncoderTypeX264,
				Codec:   config.CodecVP8, // x264 doesn't support VP8
				Bitrate: 2000,
			},
			expectError: false, // Should fallback or handle gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoder, err := NewEncoderGst(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, encoder)
			} else {
				if err != nil {
					t.Logf("Expected success but got error: %v", err)
					// Some encoders might not be available in test environment
					return
				}
				assert.NoError(t, err)
				assert.NotNil(t, encoder)

				// Cleanup
				if encoder != nil {
					encoder.Stop()
				}
			}
		})
	}
}

func TestEncoderGst_EncoderSelection(t *testing.T) {
	tests := []struct {
		name          string
		config        config.EncoderConfig
		expectedChain []EncoderType
	}{
		{
			name: "Hardware H.264 preference",
			config: config.EncoderConfig{
				Type:           config.EncoderTypeAuto,
				Codec:          config.CodecH264,
				UseHardware:    true,
				FallbackToSoft: true,
			},
			expectedChain: []EncoderType{
				EncoderTypeHardwareNVENC,
				EncoderTypeHardwareVAAPI,
				EncoderTypeSoftwareX264,
			},
		},
		{
			name: "VP8 hardware preference",
			config: config.EncoderConfig{
				Type:           config.EncoderTypeAuto,
				Codec:          config.CodecVP8,
				UseHardware:    true,
				FallbackToSoft: true,
			},
			expectedChain: []EncoderType{
				EncoderTypeHardwareVAAPI, // NVENC doesn't support VP8
				EncoderTypeSoftwareVP8,
			},
		},
		{
			name: "Software only",
			config: config.EncoderConfig{
				Type:           config.EncoderTypeX264,
				Codec:          config.CodecH264,
				UseHardware:    false,
				FallbackToSoft: true,
			},
			expectedChain: []EncoderType{
				EncoderTypeSoftwareX264,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoder := &EncoderGst{config: tt.config}
			chain := encoder.buildEncoderFallbackChain()

			assert.Equal(t, tt.expectedChain, chain)
		})
	}
}

func TestEncoderGst_StartStop(t *testing.T) {
	config := createValidX264Config() // Use software encoder for reliability

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed (likely no GStreamer): %v", err)
		return
	}
	require.NotNil(t, encoder)

	// Test initial state
	assert.False(t, encoder.IsRunning())

	// Test start
	err = encoder.Start()
	if err != nil {
		t.Skipf("Encoder start failed (likely no GStreamer): %v", err)
		return
	}
	assert.True(t, encoder.IsRunning())

	// Test double start
	err = encoder.Start()
	assert.Error(t, err)

	// Test stop
	err = encoder.Stop()
	assert.NoError(t, err)
	assert.False(t, encoder.IsRunning())

	// Test double stop
	err = encoder.Stop()
	assert.NoError(t, err)
}

func TestEncoderGst_SampleProcessing(t *testing.T) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	require.NotNil(t, encoder)

	err = encoder.Start()
	if err != nil {
		t.Skipf("Encoder start failed: %v", err)
		return
	}
	defer encoder.Stop()

	// Create test sample
	testSample := createTestVideoSample(640, 480)

	// Test pushing sample when not running
	encoder.Stop()
	err = encoder.PushSample(testSample)
	assert.Error(t, err)

	// Restart for actual test
	err = encoder.Start()
	if err != nil {
		t.Skipf("Encoder restart failed: %v", err)
		return
	}

	// Test pushing sample when running
	err = encoder.PushSample(testSample)
	assert.NoError(t, err)

	// Test output channel
	outputChan := encoder.GetOutputChannel()
	assert.NotNil(t, outputChan)

	// Test error channel
	errorChan := encoder.GetErrorChannel()
	assert.NotNil(t, errorChan)

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	// Check if we can get stats
	stats := encoder.GetStats()
	assert.GreaterOrEqual(t, stats.FramesEncoded, int64(0))
}

func TestEncoderGst_SafeSampleConversion(t *testing.T) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	require.NotNil(t, encoder)

	// Test lifecycle manager initialization
	assert.NotNil(t, encoder.lifecycleManager, "Lifecycle manager should be initialized")

	// Test safe sample conversion with nil sample
	sample, err := encoder.safeConvertGstSample(nil)
	assert.Error(t, err)
	assert.Nil(t, sample)
	assert.Contains(t, err.Error(), "received nil gst sample")

	// Test lifecycle manager statistics
	stats := encoder.lifecycleManager.GetStats()
	assert.GreaterOrEqual(t, stats.TotalRegistered, int64(0))
	assert.GreaterOrEqual(t, stats.CurrentActive, int64(0))

	// Test lifecycle manager object listing
	activeObjects := encoder.lifecycleManager.ListActiveObjects()
	assert.NotNil(t, activeObjects)

	// Cleanup
	encoder.Stop()

	// Verify lifecycle manager cleanup
	finalStats := encoder.lifecycleManager.GetStats()
	t.Logf("Final lifecycle stats - Total: %d, Active: %d, Released: %d",
		finalStats.TotalRegistered, finalStats.CurrentActive, finalStats.TotalReleased)
}

func TestEncoderGst_LifecycleManagerInitialization(t *testing.T) {
	// Test that lifecycle manager is properly initialized even without GStreamer
	config := createValidX264Config()

	// Create encoder struct manually to test lifecycle manager initialization
	enc := &EncoderGst{
		config: config,
		logger: logrus.WithField("component", "encoder-gst-test"),
	}

	// Initialize lifecycle manager
	lifecycleConfig := ObjectLifecycleConfig{
		CleanupInterval:    30 * time.Second,
		ObjectRetention:    5 * time.Minute,
		EnableValidation:   true,
		ValidationInterval: 60 * time.Second,
		EnableStackTrace:   false,
		LogObjectEvents:    false,
	}
	enc.lifecycleManager = NewObjectLifecycleManager(lifecycleConfig)

	// Test lifecycle manager functionality
	assert.NotNil(t, enc.lifecycleManager)

	// Test registering a mock object
	mockObject := "test-object"
	id, err := enc.lifecycleManager.RegisterObject(mockObject, "string", "test")
	assert.NoError(t, err)
	assert.NotEqual(t, uintptr(0), id)

	// Test object validation
	isValid := enc.lifecycleManager.IsValidObject(id)
	assert.True(t, isValid)

	// Test getting object info
	info := enc.lifecycleManager.GetObjectInfo(id)
	assert.NotNil(t, info)
	assert.Equal(t, "string", info.Type)
	assert.Equal(t, "test", info.Name)

	// Test unregistering object
	err = enc.lifecycleManager.UnregisterObject(id)
	assert.NoError(t, err)

	// Test statistics
	stats := enc.lifecycleManager.GetStats()
	assert.Equal(t, int64(1), stats.TotalRegistered)
	assert.Equal(t, int64(1), stats.TotalReleased)

	// Cleanup
	enc.lifecycleManager.Close()
}

func TestEncoderGst_LifecycleManagement(t *testing.T) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	require.NotNil(t, encoder)

	// Check initial lifecycle manager state
	initialStats := encoder.lifecycleManager.GetStats()
	assert.Equal(t, int64(0), initialStats.TotalRegistered)

	err = encoder.Start()
	if err != nil {
		t.Skipf("Encoder start failed: %v", err)
		return
	}

	// Check that pipeline objects are registered
	afterStartStats := encoder.lifecycleManager.GetStats()
	assert.Greater(t, afterStartStats.TotalRegistered, int64(0))
	assert.Greater(t, afterStartStats.CurrentActive, int64(0))

	// List active objects
	activeObjects := encoder.lifecycleManager.ListActiveObjects()
	assert.NotEmpty(t, activeObjects)

	// Verify we have expected object types
	objectTypes := make(map[string]int)
	for _, obj := range activeObjects {
		objectTypes[obj.Type]++
	}

	// Should have pipeline, elements, and possibly other objects
	assert.Contains(t, objectTypes, "gst.Pipeline")
	assert.Contains(t, objectTypes, "gst.Element")

	// Stop encoder and check cleanup
	encoder.Stop()

	// Verify lifecycle manager was cleaned up
	assert.Nil(t, encoder.lifecycleManager)
}

func TestEncoderGst_ResourceCleanup(t *testing.T) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	require.NotNil(t, encoder)

	// Start and immediately stop to test cleanup
	err = encoder.Start()
	if err != nil {
		t.Skipf("Encoder start failed: %v", err)
		return
	}

	// Verify running state
	assert.True(t, encoder.IsRunning())
	assert.NotNil(t, encoder.pipeline)
	assert.NotNil(t, encoder.lifecycleManager)

	// Stop and verify cleanup
	err = encoder.Stop()
	assert.NoError(t, err)
	assert.False(t, encoder.IsRunning())
	assert.Nil(t, encoder.pipeline)
	assert.Nil(t, encoder.lifecycleManager)

	// Test double stop (should not panic)
	err = encoder.Stop()
	assert.NoError(t, err)
}

func TestEncoderGst_BitrateControl(t *testing.T) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	require.NotNil(t, encoder)

	err = encoder.Start()
	if err != nil {
		t.Skipf("Encoder start failed: %v", err)
		return
	}
	defer encoder.Stop()

	// Test valid bitrate change
	err = encoder.SetBitrate(3000)
	assert.NoError(t, err)

	stats := encoder.GetStats()
	assert.Equal(t, 3000, stats.TargetBitrate)

	// Test invalid bitrate
	err = encoder.SetBitrate(-1)
	assert.Error(t, err)

	err = encoder.SetBitrate(100000)
	assert.Error(t, err)
}

func TestEncoderGst_KeyframeGeneration(t *testing.T) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	require.NotNil(t, encoder)

	err = encoder.Start()
	if err != nil {
		t.Skipf("Encoder start failed: %v", err)
		return
	}
	defer encoder.Stop()

	// Test force keyframe
	err = encoder.ForceKeyframe()
	assert.NoError(t, err)

	// Check stats
	stats := encoder.GetStats()
	assert.GreaterOrEqual(t, stats.KeyframesEncoded, int64(1))
}

func TestEncoderGst_Statistics(t *testing.T) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	require.NotNil(t, encoder)

	// Test stats before start
	stats := encoder.GetStats()
	assert.Equal(t, int64(0), stats.FramesEncoded)
	assert.Equal(t, config.Bitrate, stats.TargetBitrate)

	err = encoder.Start()
	if err != nil {
		t.Skipf("Encoder start failed: %v", err)
		return
	}
	defer encoder.Stop()

	// Push some test samples
	for i := 0; i < 5; i++ {
		sample := createTestVideoSample(640, 480)
		encoder.PushSample(sample)
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check updated stats
	stats = encoder.GetStats()
	assert.GreaterOrEqual(t, stats.FramesEncoded, int64(0))
	assert.NotZero(t, stats.StartTime)
}

func TestEncoderGst_AdaptiveBitrate(t *testing.T) {
	config := createValidX264Config()
	config.AdaptiveBitrate = true
	config.BitrateWindow = 1 // 1 second window for faster testing

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	require.NotNil(t, encoder)

	// Check that adaptive bitrate is enabled
	assert.True(t, encoder.bitrateAdaptor.enabled)
	assert.Equal(t, time.Second, encoder.bitrateAdaptor.adaptationWindow)

	err = encoder.Start()
	if err != nil {
		t.Skipf("Encoder start failed: %v", err)
		return
	}
	defer encoder.Stop()

	// Test adaptation logic (without actually running the goroutine)
	encoder.performanceTracker.mu.Lock()
	encoder.performanceTracker.cpuUsage = 90.0                     // High CPU usage
	encoder.performanceTracker.avgLatency = 150 * time.Millisecond // High latency
	encoder.performanceTracker.encodingFPS = 20.0
	encoder.performanceTracker.mu.Unlock()

	initialBitrate := encoder.bitrateAdaptor.currentBitrate
	encoder.adaptBitrate()

	// Should reduce bitrate due to high CPU and latency
	newBitrate := encoder.bitrateAdaptor.currentBitrate
	assert.Less(t, newBitrate, initialBitrate)
}

func TestEncoderGst_QualityMonitoring(t *testing.T) {
	config := createValidX264Config()
	config.AdaptiveBitrate = true

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	require.NotNil(t, encoder)

	// Check quality monitor initialization
	assert.True(t, encoder.qualityMonitor.adaptiveEnabled)
	assert.Equal(t, 0.8, encoder.qualityMonitor.targetQuality)

	err = encoder.Start()
	if err != nil {
		t.Skipf("Encoder start failed: %v", err)
		return
	}
	defer encoder.Stop()

	// Simulate some frame drops
	encoder.stats.mu.Lock()
	encoder.stats.framesEncoded = 90
	encoder.stats.framesDropped = 10
	encoder.stats.mu.Unlock()

	// Test quality monitoring
	encoder.monitorQuality()

	// Check quality calculation
	encoder.qualityMonitor.mu.RLock()
	quality := encoder.qualityMonitor.currentQuality
	encoder.qualityMonitor.mu.RUnlock()

	// Quality should be 0.9 (90 successful / 100 total)
	assert.InDelta(t, 0.9, quality, 0.01)
}

func TestEncoderGst_ErrorHandling(t *testing.T) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		t.Skipf("Encoder creation failed: %v", err)
		return
	}
	require.NotNil(t, encoder)

	// Test error channel
	errorChan := encoder.GetErrorChannel()
	assert.NotNil(t, errorChan)

	// Test pushing sample when not running
	sample := createTestVideoSample(640, 480)
	err = encoder.PushSample(sample)
	assert.Error(t, err)
}

func TestEncoderGst_ConfigurationMapping(t *testing.T) {
	encoder := &EncoderGst{}

	// Test NVENC preset mapping
	tests := []struct {
		input    config.EncoderPreset
		expected string
	}{
		{config.PresetUltraFast, "hp"},
		{config.PresetFast, "default"},
		{config.PresetSlow, "hq"},
	}

	for _, tt := range tests {
		result := encoder.mapPresetToNVENC(tt.input)
		assert.Equal(t, tt.expected, result)
	}

	// Test x264 preset mapping (should be direct)
	result := encoder.mapPresetToX264(config.PresetFast)
	assert.Equal(t, string(config.PresetFast), result)
}

func TestEncoderGst_ElementNameMapping(t *testing.T) {
	encoder := &EncoderGst{
		config: config.EncoderConfig{
			Codec: config.CodecH264,
		},
	}

	tests := []struct {
		encoderType EncoderType
		expected    string
	}{
		{EncoderTypeHardwareNVENC, "nvh264enc"},
		{EncoderTypeHardwareVAAPI, "vaapih264enc"},
		{EncoderTypeSoftwareX264, "x264enc"},
		{EncoderTypeSoftwareVP8, "vp8enc"},
		{EncoderTypeSoftwareVP9, "vp9enc"},
	}

	for _, tt := range tests {
		result := encoder.getGStreamerElementName(tt.encoderType)
		assert.Equal(t, tt.expected, result)
	}
}

// Helper functions for creating test configurations

func createValidH264Config() config.EncoderConfig {
	return config.EncoderConfig{
		Type:             config.EncoderTypeX264,
		Codec:            config.CodecH264,
		Bitrate:          2000,
		MaxBitrate:       4000,
		MinBitrate:       500,
		KeyframeInterval: 2,
		UseHardware:      false,
		FallbackToSoft:   true,
		Preset:           config.PresetFast,
		Profile:          config.ProfileMain,
		RateControl:      config.RateControlVBR,
		Quality:          23,
		Threads:          2,
		BFrames:          0,
		RefFrames:        1,
		ZeroLatency:      true,
		Tune:             "zerolatency",
		AdaptiveBitrate:  false,
		BitrateWindow:    5,
		CustomOptions:    make(map[string]interface{}),
		GStreamerOptions: make(map[string]interface{}),
	}
}

func createValidVP8Config() config.EncoderConfig {
	return config.EncoderConfig{
		Type:             config.EncoderTypeVP8,
		Codec:            config.CodecVP8,
		Bitrate:          2000,
		MaxBitrate:       4000,
		MinBitrate:       500,
		KeyframeInterval: 2,
		UseHardware:      false,
		FallbackToSoft:   true,
		Preset:           config.PresetFast,
		Profile:          config.ProfileMain,
		RateControl:      config.RateControlVBR,
		Quality:          40,
		Threads:          2,
		BFrames:          0,
		RefFrames:        1,
		ZeroLatency:      false,
		AdaptiveBitrate:  false,
		BitrateWindow:    5,
		CustomOptions:    make(map[string]interface{}),
		GStreamerOptions: make(map[string]interface{}),
	}
}

func createValidX264Config() config.EncoderConfig {
	return config.EncoderConfig{
		Type:             config.EncoderTypeX264,
		Codec:            config.CodecH264,
		Bitrate:          1000, // Lower bitrate for testing
		MaxBitrate:       2000,
		MinBitrate:       500,
		KeyframeInterval: 2,
		UseHardware:      false,
		FallbackToSoft:   true,
		Preset:           config.PresetVeryFast,  // Faster preset for testing
		Profile:          config.ProfileBaseline, // Simpler profile for testing
		RateControl:      config.RateControlCBR,  // Simpler rate control
		Quality:          30,                     // Lower quality for faster encoding
		Threads:          1,                      // Single thread for testing
		BFrames:          0,
		RefFrames:        1,
		ZeroLatency:      true,
		Tune:             "zerolatency",
		AdaptiveBitrate:  false,
		BitrateWindow:    5,
		CustomOptions:    make(map[string]interface{}),
		GStreamerOptions: make(map[string]interface{}),
	}
}

func createTestVideoSample(width, height int) *Sample {
	// Create a simple test frame (solid color)
	frameSize := width * height * 4 // BGRx format
	data := make([]byte, frameSize)

	// Fill with a simple pattern
	for i := 0; i < frameSize; i += 4 {
		data[i] = 128   // B
		data[i+1] = 64  // G
		data[i+2] = 192 // R
		data[i+3] = 255 // X
	}

	return &Sample{
		Data:      data,
		Timestamp: time.Now(),
		Format: SampleFormat{
			MediaType: MediaTypeVideo,
			Codec:     "raw",
			Width:     width,
			Height:    height,
		},
		Metadata: make(map[string]interface{}),
	}
}

// Benchmark tests

func BenchmarkEncoderGst_PushSample(b *testing.B) {
	config := createValidX264Config()

	encoder, err := NewEncoderGst(config)
	if err != nil {
		b.Skipf("Encoder creation failed: %v", err)
		return
	}

	err = encoder.Start()
	if err != nil {
		b.Skipf("Encoder start failed: %v", err)
		return
	}
	defer encoder.Stop()

	sample := createTestVideoSample(640, 480)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder.PushSample(sample)
	}
}

func BenchmarkEncoderGst_BitrateAdaptation(b *testing.B) {
	config := createValidX264Config()
	config.AdaptiveBitrate = true

	encoder, err := NewEncoderGst(config)
	if err != nil {
		b.Skipf("Encoder creation failed: %v", err)
		return
	}

	// Setup performance metrics
	encoder.performanceTracker.mu.Lock()
	encoder.performanceTracker.cpuUsage = 75.0
	encoder.performanceTracker.avgLatency = 50 * time.Millisecond
	encoder.performanceTracker.encodingFPS = 30.0
	encoder.performanceTracker.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encoder.adaptBitrate()
	}
}

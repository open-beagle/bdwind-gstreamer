package gstreamer

import (
	"testing"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestNewX264Encoder(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264
	cfg.Codec = config.CodecH264

	encoder, err := NewX264Encoder(cfg)
	if err != nil {
		t.Fatalf("Failed to create x264 encoder: %v", err)
	}

	if encoder == nil {
		t.Fatal("Expected encoder to be created")
	}

	if encoder.IsHardwareAccelerated() {
		t.Error("x264 encoder should not be hardware accelerated")
	}

	// Test supported codecs
	codecs := encoder.GetSupportedCodecs()
	if len(codecs) != 1 {
		t.Errorf("Expected x264 encoder to support exactly 1 codec, got %d", len(codecs))
	}

	if codecs[0] != config.CodecH264 {
		t.Errorf("Expected x264 encoder to support H264, got %s", codecs[0])
	}
}

func TestX264Encoder_GetElement(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264
	cfg.Codec = config.CodecH264
	cfg.Bitrate = 2000
	cfg.KeyframeInterval = 2
	cfg.RateControl = config.RateControlVBR
	cfg.Preset = config.PresetFast
	cfg.Profile = config.ProfileMain
	cfg.Threads = 4
	cfg.BFrames = 2
	cfg.RefFrames = 3
	cfg.ZeroLatency = false // Disable zero latency to allow B-frames

	encoder := &X264Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	elementName, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test element name
	if elementName != "x264enc" {
		t.Errorf("Expected element name 'x264enc', got '%s'", elementName)
	}

	// Test properties
	if properties["bitrate"] != 2000 {
		t.Errorf("Expected bitrate 2000, got %v", properties["bitrate"])
	}

	if properties["key-int-max"] != 60 { // 2 seconds * 30 fps
		t.Errorf("Expected key-int-max 60, got %v", properties["key-int-max"])
	}

	if properties["pass"] != "pass1" {
		t.Errorf("Expected pass 'pass1', got %v", properties["pass"])
	}

	if properties["speed-preset"] != "fast" {
		t.Errorf("Expected speed-preset 'fast', got %v", properties["speed-preset"])
	}

	if properties["profile"] != "main" {
		t.Errorf("Expected profile 'main', got %v", properties["profile"])
	}

	if properties["threads"] != 4 {
		t.Errorf("Expected threads 4, got %v", properties["threads"])
	}

	if properties["b-frames"] != 2 {
		t.Errorf("Expected b-frames 2, got %v", properties["b-frames"])
	}

	if properties["ref"] != 3 {
		t.Errorf("Expected ref 3, got %v", properties["ref"])
	}
}

func TestX264Encoder_GetElementZeroLatency(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264
	cfg.Codec = config.CodecH264
	cfg.ZeroLatency = true
	cfg.BFrames = 2   // Should be overridden to 0 for zero latency
	cfg.LookAhead = 8 // Should be ignored for zero latency

	encoder := &X264Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	_, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test zero latency settings
	if properties["tune"] != "zerolatency" {
		t.Errorf("Expected tune 'zerolatency', got %v", properties["tune"])
	}

	if properties["b-frames"] != 0 {
		t.Errorf("Expected b-frames to be 0 for zero latency, got %v", properties["b-frames"])
	}

	// rc-lookahead should not be set for zero latency
	if _, exists := properties["rc-lookahead"]; exists {
		t.Error("Expected rc-lookahead to not be set for zero latency")
	}

	// Check for intra-refresh in option-string
	if optionString, exists := properties["option-string"]; exists {
		if optStr, ok := optionString.(string); ok {
			if !contains(optStr, "intra-refresh=1") {
				t.Error("Expected intra-refresh=1 in option-string for zero latency")
			}
			if !contains(optStr, "no-scenecut=1") {
				t.Error("Expected no-scenecut=1 in option-string for zero latency")
			}
		}
	}
}

func TestX264Encoder_GetElementCBR(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264
	cfg.Codec = config.CodecH264
	cfg.RateControl = config.RateControlCBR
	cfg.Bitrate = 3000

	encoder := &X264Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	_, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test CBR settings
	if properties["pass"] != "cbr" {
		t.Errorf("Expected pass 'cbr', got %v", properties["pass"])
	}

	if properties["bitrate"] != 3000 {
		t.Errorf("Expected bitrate 3000, got %v", properties["bitrate"])
	}
}

func TestX264Encoder_GetElementCQP(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264
	cfg.Codec = config.CodecH264
	cfg.RateControl = config.RateControlCQP
	cfg.Quality = 20

	encoder := &X264Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	_, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test CQP settings
	if properties["pass"] != "qual" {
		t.Errorf("Expected pass 'qual', got %v", properties["pass"])
	}

	if properties["quantizer"] != 20 {
		t.Errorf("Expected quantizer 20, got %v", properties["quantizer"])
	}
}

func TestX264Encoder_GetElementWithMaxBitrate(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264
	cfg.Codec = config.CodecH264
	cfg.Bitrate = 2000
	cfg.MaxBitrate = 4000
	cfg.RateControl = config.RateControlVBR

	encoder := &X264Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	_, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test VBV settings in option-string
	if optionString, exists := properties["option-string"]; exists {
		if optStr, ok := optionString.(string); ok {
			if !contains(optStr, "vbv-maxrate=4000") {
				t.Error("Expected vbv-maxrate=4000 in option-string")
			}
			if !contains(optStr, "vbv-bufsize=4000") {
				t.Error("Expected vbv-bufsize=4000 in option-string")
			}
		}
	}
}

func TestX264Encoder_GetElementWithCustomOptions(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264
	cfg.Codec = config.CodecH264
	cfg.CustomOptions = map[string]interface{}{
		"aq-mode":     "2",
		"aq-strength": 1.0,
		"psy-rd":      "1.0:0.0",
	}

	encoder := &X264Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	_, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test custom options in option-string
	if optionString, exists := properties["option-string"]; exists {
		if optStr, ok := optionString.(string); ok {
			if !contains(optStr, "aq-mode=2") {
				t.Error("Expected aq-mode=2 in option-string")
			}
			if !contains(optStr, "aq-strength=1") {
				t.Error("Expected aq-strength=1 in option-string")
			}
			if !contains(optStr, "psy-rd=1.0:0.0") {
				t.Error("Expected psy-rd=1.0:0.0 in option-string")
			}
		}
	}
}

func TestX264Encoder_GetElementUnsupportedCodec(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264
	cfg.Codec = config.CodecVP8 // VP8 is not supported by x264

	encoder := &X264Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	_, _, err := encoder.GetElement()
	if err == nil {
		t.Error("Expected error for unsupported codec VP8")
	}
}

func TestX264Encoder_UpdateBitrate(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264

	encoder := &X264Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	// Test valid bitrate update
	err := encoder.UpdateBitrate(3000)
	if err != nil {
		t.Errorf("Failed to update bitrate: %v", err)
	}

	if encoder.config.Bitrate != 3000 {
		t.Errorf("Expected bitrate to be updated to 3000, got %d", encoder.config.Bitrate)
	}

	// Test invalid bitrate (too low)
	err = encoder.UpdateBitrate(0)
	if err == nil {
		t.Error("Expected error for invalid bitrate 0")
	}

	// Test invalid bitrate (too high)
	err = encoder.UpdateBitrate(60000)
	if err == nil {
		t.Error("Expected error for invalid bitrate 60000")
	}
}

func TestX264Encoder_ForceKeyframe(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264

	encoder := &X264Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	initialKeyframes := encoder.GetStats().KeyframesEncoded

	err := encoder.ForceKeyframe()
	if err != nil {
		t.Errorf("Failed to force keyframe: %v", err)
	}

	// Check that keyframe count increased
	newKeyframes := encoder.GetStats().KeyframesEncoded
	if newKeyframes != initialKeyframes+1 {
		t.Errorf("Expected keyframes to increase by 1, got %d -> %d", initialKeyframes, newKeyframes)
	}
}

func TestX264Encoder_Initialize(t *testing.T) {
	encoder := &X264Encoder{
		BaseEncoder: NewBaseEncoder(config.DefaultEncoderConfig()),
	}

	// Test valid initialization
	validConfig := config.DefaultEncoderConfig()
	validConfig.Type = config.EncoderTypeX264
	validConfig.Codec = config.CodecH264

	err := encoder.Initialize(validConfig)
	if err != nil {
		t.Errorf("Failed to initialize with valid config: %v", err)
	}

	// Test initialization with unsupported codec
	invalidConfig := config.DefaultEncoderConfig()
	invalidConfig.Type = config.EncoderTypeX264
	invalidConfig.Codec = config.CodecVP8

	err = encoder.Initialize(invalidConfig)
	if err == nil {
		t.Error("Expected error for unsupported codec VP8")
	}

	// Test initialization with invalid config
	invalidConfig2 := config.DefaultEncoderConfig()
	invalidConfig2.Bitrate = -1 // Invalid bitrate

	err = encoder.Initialize(invalidConfig2)
	if err == nil {
		t.Error("Expected error for invalid config")
	}
}

func TestGetOptimalX264Config(t *testing.T) {
	cfg := GetOptimalX264Config()

	// Test that we get a valid configuration
	err := config.ValidateEncoderConfig(&cfg)
	if err != nil {
		t.Errorf("Optimal x264 config should be valid, got error: %v", err)
	}

	// Test that encoder type is set to x264
	if cfg.Type != config.EncoderTypeX264 {
		t.Errorf("Expected encoder type to be %s, got %s", config.EncoderTypeX264, cfg.Type)
	}

	// Test that hardware acceleration is disabled
	if cfg.UseHardware {
		t.Error("Expected hardware acceleration to be disabled for x264")
	}

	// Test that threads are set appropriately
	if cfg.Threads <= 0 || cfg.Threads > 16 {
		t.Errorf("Expected threads to be between 1 and 16, got %d", cfg.Threads)
	}
}

func TestDetectCPUCapabilities(t *testing.T) {
	cpuInfo := detectCPUCapabilities()

	// Test that we get reasonable values
	if cpuInfo.Cores <= 0 {
		t.Errorf("Expected positive number of cores, got %d", cpuInfo.Cores)
	}

	if cpuInfo.Threads <= 0 {
		t.Errorf("Expected positive number of threads, got %d", cpuInfo.Threads)
	}

	t.Logf("Detected CPU: %s, Cores: %d, Threads: %d", cpuInfo.Model, cpuInfo.Cores, cpuInfo.Threads)
	t.Logf("CPU features: AVX=%v, AVX2=%v, SSE4.2=%v", cpuInfo.HasAVX, cpuInfo.HasAVX2, cpuInfo.HasSSE42)
}

func TestGetX264PresetForCPU(t *testing.T) {
	tests := []struct {
		name     string
		cpuInfo  CPUInfo
		expected config.EncoderPreset
	}{
		{
			name: "High-end CPU with AVX2",
			cpuInfo: CPUInfo{
				Cores:   8,
				HasAVX2: true,
			},
			expected: config.PresetMedium,
		},
		{
			name: "High-end CPU without AVX2",
			cpuInfo: CPUInfo{
				Cores:   8,
				HasAVX2: false,
			},
			expected: config.PresetFast,
		},
		{
			name: "Mid-range CPU with AVX",
			cpuInfo: CPUInfo{
				Cores:  4,
				HasAVX: true,
			},
			expected: config.PresetFast,
		},
		{
			name: "Mid-range CPU without AVX",
			cpuInfo: CPUInfo{
				Cores:  4,
				HasAVX: false,
			},
			expected: config.PresetVeryFast,
		},
		{
			name: "Low-end CPU",
			cpuInfo: CPUInfo{
				Cores: 2,
			},
			expected: config.PresetVeryFast,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetX264PresetForCPU(tt.cpuInfo)
			if result != tt.expected {
				t.Errorf("Expected preset %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetX264ThreadsForCPU(t *testing.T) {
	tests := []struct {
		name     string
		cpuInfo  CPUInfo
		expected int
	}{
		{
			name: "4 cores",
			cpuInfo: CPUInfo{
				Cores: 4,
			},
			expected: 6, // 4 * 1.5 = 6
		},
		{
			name: "8 cores",
			cpuInfo: CPUInfo{
				Cores: 8,
			},
			expected: 12, // 8 * 1.5 = 12
		},
		{
			name: "16 cores (should be capped)",
			cpuInfo: CPUInfo{
				Cores: 16,
			},
			expected: 16, // Capped at 16
		},
		{
			name: "1 core",
			cpuInfo: CPUInfo{
				Cores: 1,
			},
			expected: 1, // 1 * 1.5 = 1.5, rounded down to 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetX264ThreadsForCPU(tt.cpuInfo)
			if result != tt.expected {
				t.Errorf("Expected %d threads, got %d", tt.expected, result)
			}
		})
	}
}

func TestEstimateX264Performance(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264
	cfg.Preset = config.PresetFast

	cpuInfo := CPUInfo{
		Cores: 4,
	}

	perf := EstimateX264Performance(cfg, cpuInfo)

	// Test that we get reasonable estimates
	if perf.EstimatedFPS <= 0 {
		t.Errorf("Expected positive FPS estimate, got %f", perf.EstimatedFPS)
	}

	if perf.CPUUsage <= 0 || perf.CPUUsage > 100 {
		t.Errorf("Expected CPU usage between 0 and 100, got %f", perf.CPUUsage)
	}

	if perf.Quality == "" {
		t.Error("Expected quality estimate to be set")
	}

	t.Logf("Performance estimate: FPS=%.1f, CPU=%.1f%%, Quality=%s",
		perf.EstimatedFPS, perf.CPUUsage, perf.Quality)
}

func TestX264Encoder_Cleanup(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeX264

	encoder := &X264Encoder{
		BaseEncoder: NewBaseEncoder(cfg),
	}

	// Set a mock element
	encoder.element = "mock_element"

	err := encoder.Cleanup()
	if err != nil {
		t.Errorf("Failed to cleanup encoder: %v", err)
	}

	// Check that element was cleaned up
	if encoder.element != nil {
		t.Error("Expected element to be nil after cleanup")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

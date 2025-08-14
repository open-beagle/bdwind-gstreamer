package gstreamer

import (
	"testing"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestNewNVENCEncoder(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeNVENC
	cfg.Codec = config.CodecH264

	// Note: This test will fail if NVENC is not available
	// In a real test environment, we would mock the availability check
	encoder, err := NewNVENCEncoder(cfg)
	if err != nil {
		// Skip test if NVENC is not available
		t.Skipf("NVENC not available: %v", err)
	}

	if encoder == nil {
		t.Fatal("Expected encoder to be created")
	}

	if !encoder.IsHardwareAccelerated() {
		t.Error("NVENC encoder should be hardware accelerated")
	}

	// Test supported codecs
	codecs := encoder.GetSupportedCodecs()
	if len(codecs) == 0 {
		t.Error("NVENC encoder should support at least one codec")
	}

	found := false
	for _, codec := range codecs {
		if codec == config.CodecH264 {
			found = true
			break
		}
	}
	if !found {
		t.Error("NVENC encoder should support H264 codec")
	}
}

func TestNVENCEncoder_GetElement(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeNVENC
	cfg.Codec = config.CodecH264
	cfg.Bitrate = 2000
	cfg.MaxBitrate = 4000
	cfg.KeyframeInterval = 2
	cfg.RateControl = config.RateControlVBR
	cfg.Preset = config.PresetFast
	cfg.Profile = config.ProfileMain

	encoder := &NVENCEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		deviceID:    0,
	}

	elementName, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test element name
	if elementName != "nvh264enc" {
		t.Errorf("Expected element name 'nvh264enc', got '%s'", elementName)
	}

	// Test properties
	if properties["bitrate"] != 2000 {
		t.Errorf("Expected bitrate 2000, got %v", properties["bitrate"])
	}

	if properties["max-bitrate"] != 4000 {
		t.Errorf("Expected max-bitrate 4000, got %v", properties["max-bitrate"])
	}

	if properties["gop-size"] != 60 { // 2 seconds * 30 fps
		t.Errorf("Expected gop-size 60, got %v", properties["gop-size"])
	}

	if properties["gpu-id"] != 0 {
		t.Errorf("Expected gpu-id 0, got %v", properties["gpu-id"])
	}

	if properties["rc-mode"] != "vbr" {
		t.Errorf("Expected rc-mode 'vbr', got %v", properties["rc-mode"])
	}

	if properties["preset"] != "hp" {
		t.Errorf("Expected preset 'hp', got %v", properties["preset"])
	}

	if properties["profile"] != "main" {
		t.Errorf("Expected profile 'main', got %v", properties["profile"])
	}
}

func TestNVENCEncoder_GetElementZeroLatency(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeNVENC
	cfg.Codec = config.CodecH264
	cfg.ZeroLatency = true
	cfg.BFrames = 2   // Should be overridden to 0 for zero latency
	cfg.LookAhead = 8 // Should be overridden to 0 for zero latency

	encoder := &NVENCEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		deviceID:    0,
	}

	_, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test zero latency settings
	if properties["zerolatency"] != true {
		t.Error("Expected zerolatency to be true")
	}

	if properties["b-frames"] != 0 {
		t.Errorf("Expected b-frames to be 0 for zero latency, got %v", properties["b-frames"])
	}

	if properties["lookahead"] != 0 {
		t.Errorf("Expected lookahead to be 0 for zero latency, got %v", properties["lookahead"])
	}
}

func TestNVENCEncoder_GetElementCQP(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeNVENC
	cfg.Codec = config.CodecH264
	cfg.RateControl = config.RateControlCQP
	cfg.Quality = 20

	encoder := &NVENCEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		deviceID:    0,
	}

	_, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test CQP settings
	if properties["rc-mode"] != "cqp" {
		t.Errorf("Expected rc-mode 'cqp', got %v", properties["rc-mode"])
	}

	if properties["qp-const"] != 20 {
		t.Errorf("Expected qp-const 20, got %v", properties["qp-const"])
	}
}

func TestNVENCEncoder_GetElementUnsupportedCodec(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeNVENC
	cfg.Codec = config.CodecVP8 // VP8 is not supported by NVENC

	encoder := &NVENCEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		deviceID:    0,
	}

	_, _, err := encoder.GetElement()
	if err == nil {
		t.Error("Expected error for unsupported codec VP8")
	}
}

func TestNVENCEncoder_UpdateBitrate(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeNVENC

	encoder := &NVENCEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		deviceID:    0,
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

func TestNVENCEncoder_ForceKeyframe(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeNVENC

	encoder := &NVENCEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		deviceID:    0,
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

func TestNVENCEncoder_Initialize(t *testing.T) {
	encoder := &NVENCEncoder{
		BaseEncoder: NewBaseEncoder(config.DefaultEncoderConfig()),
		deviceID:    0,
	}

	// Test valid initialization
	validConfig := config.DefaultEncoderConfig()
	validConfig.Type = config.EncoderTypeNVENC
	validConfig.Codec = config.CodecH264

	err := encoder.Initialize(validConfig)
	if err != nil {
		t.Errorf("Failed to initialize with valid config: %v", err)
	}

	// Test initialization with unsupported codec
	invalidConfig := config.DefaultEncoderConfig()
	invalidConfig.Type = config.EncoderTypeNVENC
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

func TestGetOptimalNVENCConfig(t *testing.T) {
	cfg := GetOptimalNVENCConfig()

	// Test that we get a valid configuration
	err := config.ValidateEncoderConfig(&cfg)
	if err != nil {
		t.Errorf("Optimal NVENC config should be valid, got error: %v", err)
	}

	// Test that encoder type is set to NVENC
	if cfg.Type != config.EncoderTypeNVENC {
		t.Errorf("Expected encoder type to be %s, got %s", config.EncoderTypeNVENC, cfg.Type)
	}

	// Test that hardware acceleration is enabled
	if !cfg.UseHardware {
		t.Error("Expected hardware acceleration to be enabled for NVENC")
	}
}

func TestDetectGPUGeneration(t *testing.T) {
	tests := []struct {
		name     string
		expected int
	}{
		{"RTX 4090", 12},
		{"RTX 4080", 12},
		{"RTX 3090", 11},
		{"RTX 3080", 11},
		{"RTX 2080", 10},
		{"GTX 1660", 10},
		{"GTX 1080", 9},
		{"GTX 980", 8},
		{"GTX 970", 8},
		{"GTX 780", 7},
		{"Unknown GPU", 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectGPUGeneration(tt.name)
			if result != tt.expected {
				t.Errorf("Expected generation %d for %s, got %d", tt.expected, tt.name, result)
			}
		})
	}
}

func TestNVENCEncoder_Cleanup(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeNVENC

	encoder := &NVENCEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		deviceID:    0,
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

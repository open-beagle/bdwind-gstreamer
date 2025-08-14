package gstreamer

import (
	"fmt"
	"testing"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestNewVAAPIEncoder(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeVAAPI
	cfg.Codec = config.CodecH264

	// Note: This test will fail if VAAPI is not available
	// In a real test environment, we would mock the availability check
	encoder, err := NewVAAPIEncoder(cfg)
	if err != nil {
		// Skip test if VAAPI is not available
		t.Skipf("VAAPI not available: %v", err)
	}

	if encoder == nil {
		t.Fatal("Expected encoder to be created")
	}

	if !encoder.IsHardwareAccelerated() {
		t.Error("VAAPI encoder should be hardware accelerated")
	}

	// Test supported codecs
	codecs := encoder.GetSupportedCodecs()
	if len(codecs) == 0 {
		t.Error("VAAPI encoder should support at least one codec")
	}

	found := false
	for _, codec := range codecs {
		if codec == config.CodecH264 {
			found = true
			break
		}
	}
	if !found {
		t.Error("VAAPI encoder should support H264 codec")
	}
}

func TestVAAPIEncoder_GetElementH264(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeVAAPI
	cfg.Codec = config.CodecH264
	cfg.Bitrate = 2000
	cfg.KeyframeInterval = 2
	cfg.RateControl = config.RateControlVBR
	cfg.Profile = config.ProfileMain

	encoder := &VAAAPIEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		devicePath:  "/dev/dri/renderD128",
	}

	elementName, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test element name
	if elementName != "vaapih264enc" {
		t.Errorf("Expected element name 'vaapih264enc', got '%s'", elementName)
	}

	// Test properties
	if properties["bitrate"] != 2000 {
		t.Errorf("Expected bitrate 2000, got %v", properties["bitrate"])
	}

	if properties["keyframe-period"] != 60 { // 2 seconds * 30 fps
		t.Errorf("Expected keyframe-period 60, got %v", properties["keyframe-period"])
	}

	if properties["display"] != "/dev/dri/renderD128" {
		t.Errorf("Expected display '/dev/dri/renderD128', got %v", properties["display"])
	}

	if properties["rate-control"] != "vbr" {
		t.Errorf("Expected rate-control 'vbr', got %v", properties["rate-control"])
	}

	if properties["profile"] != "main" {
		t.Errorf("Expected profile 'main', got %v", properties["profile"])
	}
}

func TestVAAPIEncoder_GetElementVP8(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeVAAPI
	cfg.Codec = config.CodecVP8
	cfg.Bitrate = 1500

	encoder := &VAAAPIEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		devicePath:  "/dev/dri/renderD128",
	}

	elementName, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test element name
	if elementName != "vaapivp8enc" {
		t.Errorf("Expected element name 'vaapivp8enc', got '%s'", elementName)
	}

	// Test properties
	if properties["bitrate"] != 1500 {
		t.Errorf("Expected bitrate 1500, got %v", properties["bitrate"])
	}
}

func TestVAAPIEncoder_GetElementVP9(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeVAAPI
	cfg.Codec = config.CodecVP9
	cfg.Bitrate = 1000

	encoder := &VAAAPIEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		devicePath:  "/dev/dri/renderD128",
	}

	elementName, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test element name
	if elementName != "vaapivp9enc" {
		t.Errorf("Expected element name 'vaapivp9enc', got '%s'", elementName)
	}

	// Test properties
	if properties["bitrate"] != 1000 {
		t.Errorf("Expected bitrate 1000, got %v", properties["bitrate"])
	}
}

func TestVAAPIEncoder_GetElementZeroLatency(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeVAAPI
	cfg.Codec = config.CodecH264
	cfg.ZeroLatency = true
	cfg.BFrames = 2 // Should be overridden to 0 for zero latency

	encoder := &VAAAPIEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		devicePath:  "/dev/dri/renderD128",
	}

	_, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test zero latency settings
	if properties["tune"] != "low-latency" {
		t.Errorf("Expected tune 'low-latency', got %v", properties["tune"])
	}

	if properties["b-frames"] != 0 {
		t.Errorf("Expected b-frames to be 0 for zero latency, got %v", properties["b-frames"])
	}
}

func TestVAAPIEncoder_GetElementCQP(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeVAAPI
	cfg.Codec = config.CodecH264
	cfg.RateControl = config.RateControlCQP
	cfg.Quality = 25

	encoder := &VAAAPIEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		devicePath:  "/dev/dri/renderD128",
	}

	_, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test CQP settings
	if properties["rate-control"] != "cqp" {
		t.Errorf("Expected rate-control 'cqp', got %v", properties["rate-control"])
	}

	if properties["init-qp"] != 25 {
		t.Errorf("Expected init-qp 25, got %v", properties["init-qp"])
	}
}

func TestVAAPIEncoder_GetElementCBR(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeVAAPI
	cfg.Codec = config.CodecH264
	cfg.RateControl = config.RateControlCBR
	cfg.MaxBitrate = 3000
	cfg.MinBitrate = 1000

	encoder := &VAAAPIEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		devicePath:  "/dev/dri/renderD128",
	}

	_, properties, err := encoder.GetElement()
	if err != nil {
		t.Fatalf("Failed to get element: %v", err)
	}

	// Test CBR settings
	if properties["rate-control"] != "cbr" {
		t.Errorf("Expected rate-control 'cbr', got %v", properties["rate-control"])
	}

	if properties["max-bitrate"] != 3000 {
		t.Errorf("Expected max-bitrate 3000, got %v", properties["max-bitrate"])
	}

	if properties["min-bitrate"] != 1000 {
		t.Errorf("Expected min-bitrate 1000, got %v", properties["min-bitrate"])
	}
}

func TestVAAPIEncoder_UpdateBitrate(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeVAAPI

	encoder := &VAAAPIEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		devicePath:  "/dev/dri/renderD128",
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

func TestVAAPIEncoder_ForceKeyframe(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeVAAPI

	encoder := &VAAAPIEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		devicePath:  "/dev/dri/renderD128",
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

func TestVAAPIEncoder_Initialize(t *testing.T) {
	encoder := &VAAAPIEncoder{
		BaseEncoder: NewBaseEncoder(config.DefaultEncoderConfig()),
		devicePath:  "/dev/dri/renderD128",
	}

	// Test valid initialization
	validConfig := config.DefaultEncoderConfig()
	validConfig.Type = config.EncoderTypeVAAPI
	validConfig.Codec = config.CodecH264

	err := encoder.Initialize(validConfig)
	if err != nil {
		t.Errorf("Failed to initialize with valid config: %v", err)
	}

	// Test initialization with invalid config
	invalidConfig := config.DefaultEncoderConfig()
	invalidConfig.Bitrate = -1 // Invalid bitrate

	err = encoder.Initialize(invalidConfig)
	if err == nil {
		t.Error("Expected error for invalid config")
	}
}

func TestGetOptimalVAAPIConfig(t *testing.T) {
	cfg := GetOptimalVAAPIConfig()

	// Test that we get a valid configuration
	err := config.ValidateEncoderConfig(&cfg)
	if err != nil {
		t.Errorf("Optimal VAAPI config should be valid, got error: %v", err)
	}

	// Test that encoder type is set to VAAPI
	if cfg.Type != config.EncoderTypeVAAPI {
		t.Errorf("Expected encoder type to be %s, got %s", config.EncoderTypeVAAPI, cfg.Type)
	}

	// Test that hardware acceleration is enabled
	if !cfg.UseHardware {
		t.Error("Expected hardware acceleration to be enabled for VAAPI")
	}
}

func TestDetectVAAPIDevice(t *testing.T) {
	device := detectVAAPIDevice()
	// Device may or may not be available, just test that function doesn't panic
	t.Logf("Detected VAAPI device: %s", device)
}

func TestDetectVAAPIDriver(t *testing.T) {
	driver := detectVAAPIDriver()
	// Driver may or may not be detectable, just test that function doesn't panic
	t.Logf("Detected VAAPI driver: %s", driver)
}

func TestMapQualityToVAAPILevel(t *testing.T) {
	tests := []struct {
		quality  int
		expected int
	}{
		{5, 1}, // High quality
		{15, 2},
		{25, 3},
		{35, 4},
		{42, 5},
		{48, 6},
		{51, 7}, // Low quality
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("quality_%d", tt.quality), func(t *testing.T) {
			result := mapQualityToVAAPILevel(tt.quality)
			if result != tt.expected {
				t.Errorf("Expected VAAPI level %d for quality %d, got %d", tt.expected, tt.quality, result)
			}
		})
	}
}

func TestVAAPIEncoder_GetSupportedCodecs(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeVAAPI

	encoder := &VAAAPIEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		devicePath:  "/dev/dri/renderD128",
	}

	codecs := encoder.GetSupportedCodecs()

	// Should at least support H.264
	if len(codecs) == 0 {
		t.Error("VAAPI encoder should support at least one codec")
	}

	foundH264 := false
	for _, codec := range codecs {
		if codec == config.CodecH264 {
			foundH264 = true
			break
		}
	}
	if !foundH264 {
		t.Error("VAAPI encoder should support H264 codec")
	}
}

func TestVAAPIEncoder_Cleanup(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeVAAPI

	encoder := &VAAAPIEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		devicePath:  "/dev/dri/renderD128",
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

package gstreamer

import (
	"fmt"
	"strings"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// VAAAPIEncoder implements hardware-accelerated encoding using Intel/AMD VAAPI
type VAAAPIEncoder struct {
	*BaseEncoder
	devicePath string
}

// NewVAAPIEncoder creates a new VAAPI encoder
func NewVAAPIEncoder(cfg config.EncoderConfig) (*VAAAPIEncoder, error) {
	if !IsVAAPIAvailable() {
		return nil, fmt.Errorf("VAAPI is not available on this system")
	}

	encoder := &VAAAPIEncoder{
		BaseEncoder: NewBaseEncoder(cfg),
		devicePath:  detectVAAPIDevice(),
	}

	if err := encoder.Initialize(cfg); err != nil {
		return nil, fmt.Errorf("failed to initialize VAAPI encoder: %w", err)
	}

	return encoder, nil
}

// Initialize initializes the VAAPI encoder
func (e *VAAAPIEncoder) Initialize(cfg config.EncoderConfig) error {
	if err := config.ValidateEncoderConfig(&cfg); err != nil {
		return fmt.Errorf("invalid encoder config: %w", err)
	}

	// Validate codec support
	supportedCodecs := e.GetSupportedCodecs()
	codecSupported := false
	for _, codec := range supportedCodecs {
		if codec == cfg.Codec {
			codecSupported = true
			break
		}
	}
	if !codecSupported {
		return fmt.Errorf("codec %s is not supported by VAAPI encoder", cfg.Codec)
	}

	e.config = cfg
	return nil
}

// GetElement returns the GStreamer element name and properties for VAAPI
func (e *VAAAPIEncoder) GetElement() (string, map[string]interface{}, error) {
	var elementName string
	properties := make(map[string]interface{})

	// Select appropriate VAAPI element based on codec
	switch e.config.Codec {
	case config.CodecH264:
		elementName = "vaapih264enc"
	case config.CodecVP8:
		elementName = "vaapivp8enc"
	case config.CodecVP9:
		elementName = "vaapivp9enc"
	default:
		return "", nil, fmt.Errorf("unsupported codec: %s", e.config.Codec)
	}

	// Set basic properties
	properties["bitrate"] = e.config.Bitrate
	properties["keyframe-period"] = e.config.KeyframeInterval * 30 // Convert seconds to frames (assuming 30fps)

	// Set device path if available
	if e.devicePath != "" {
		properties["display"] = e.devicePath
	}

	// Set rate control mode
	switch e.config.RateControl {
	case config.RateControlCBR:
		properties["rate-control"] = "cbr"
	case config.RateControlVBR:
		properties["rate-control"] = "vbr"
	case config.RateControlCQP:
		properties["rate-control"] = "cqp"
		properties["init-qp"] = e.config.Quality
	default:
		properties["rate-control"] = "vbr"
	}

	// Set quality/quantization parameters
	if e.config.RateControl != config.RateControlCQP {
		// For VBR/CBR, use quality as a general quality parameter
		if e.config.Quality > 0 {
			properties["quality-level"] = mapQualityToVAAPILevel(e.config.Quality)
		}
	}

	// Set profile for H.264
	if e.config.Codec == config.CodecH264 {
		switch e.config.Profile {
		case config.ProfileBaseline:
			properties["profile"] = "baseline"
		case config.ProfileMain:
			properties["profile"] = "main"
		case config.ProfileHigh:
			properties["profile"] = "high"
		default:
			properties["profile"] = "main"
		}
	}

	// Performance and latency settings
	if e.config.ZeroLatency {
		properties["tune"] = "low-latency"
		properties["b-frames"] = 0
	} else {
		if e.config.BFrames >= 0 {
			properties["b-frames"] = e.config.BFrames
		}
	}

	// Reference frames
	if e.config.RefFrames > 0 {
		properties["ref-frames"] = e.config.RefFrames
	}

	// Bitrate limits
	if e.config.MaxBitrate > 0 {
		properties["max-bitrate"] = e.config.MaxBitrate
	}
	if e.config.MinBitrate > 0 {
		properties["min-bitrate"] = e.config.MinBitrate
	}

	// Custom GStreamer options
	for key, value := range e.config.GStreamerOptions {
		properties[key] = value
	}

	return elementName, properties, nil
}

// UpdateBitrate dynamically updates the encoder bitrate
func (e *VAAAPIEncoder) UpdateBitrate(bitrate int) error {
	if bitrate <= 0 || bitrate > 50000 {
		return fmt.Errorf("invalid bitrate: %d (must be between 1 and 50000 kbps)", bitrate)
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Update configuration
	e.config.Bitrate = bitrate

	// In real implementation, this would update the GStreamer element property
	// For now, we'll simulate the update
	if e.element != nil {
		// gst_element_set_property(e.element, "bitrate", bitrate)
	}

	return nil
}

// ForceKeyframe forces the encoder to generate a keyframe
func (e *VAAAPIEncoder) ForceKeyframe() error {
	// In real implementation, this would send a force-keyframe event to the encoder
	// For now, we'll simulate the keyframe generation
	if e.element != nil {
		// Send force-keyframe event
		// gst_element_send_event(e.element, gst_video_event_new_downstream_force_key_unit(...))
	}

	// Update statistics
	e.mutex.Lock()
	e.stats.KeyframesEncoded++
	e.mutex.Unlock()

	return nil
}

// IsHardwareAccelerated returns true since VAAPI is hardware-accelerated
func (e *VAAAPIEncoder) IsHardwareAccelerated() bool {
	return true
}

// GetSupportedCodecs returns the codecs supported by VAAPI
func (e *VAAAPIEncoder) GetSupportedCodecs() []config.CodecType {
	// VAAPI support varies by hardware, but these are commonly supported
	codecs := []config.CodecType{
		config.CodecH264,
	}

	// Check for VP8/VP9 support (newer Intel GPUs and some AMD GPUs)
	if supportsVP8VP9() {
		codecs = append(codecs, config.CodecVP8, config.CodecVP9)
	}

	return codecs
}

// Cleanup cleans up VAAPI encoder resources
func (e *VAAAPIEncoder) Cleanup() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	// In real implementation, this would clean up GStreamer resources
	if e.element != nil {
		// gst_object_unref(e.element)
		e.element = nil
	}

	return nil
}

// GetOptimalVAAPIConfig returns optimal VAAPI configuration based on system capabilities
func GetOptimalVAAPIConfig() config.EncoderConfig {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeVAAPI
	cfg.UseHardware = true

	// Detect GPU capabilities and adjust settings accordingly
	if gpuInfo := detectVAAPICapabilities(); gpuInfo != nil {
		// Adjust settings based on GPU capabilities
		if gpuInfo.SupportsLowLatency {
			cfg.ZeroLatency = true
			cfg.BFrames = 0
		}
		if gpuInfo.SupportsHighQuality {
			cfg.Quality = 20 // Better quality for capable hardware
		}
		if gpuInfo.MaxBitrate > 0 {
			cfg.MaxBitrate = gpuInfo.MaxBitrate
		}
	}

	return cfg
}

// VAAAPICapabilities represents VAAPI GPU capabilities
type VAAAPICapabilities struct {
	DevicePath          string
	DriverName          string
	SupportsH264        bool
	SupportsVP8         bool
	SupportsVP9         bool
	SupportsLowLatency  bool
	SupportsHighQuality bool
	MaxBitrate          int
	MaxResolution       string
}

// detectVAAPIDevice detects the VAAPI device path
func detectVAAPIDevice() string {
	// Try common VAAPI device paths
	devicePaths := []string{
		"/dev/dri/renderD128",
		"/dev/dri/renderD129",
		"/dev/dri/card0",
		"/dev/dri/card1",
	}

	for _, path := range devicePaths {
		if fileExists(path) {
			return path
		}
	}

	return ""
}

// detectVAAPICapabilities detects VAAPI capabilities (simplified implementation)
func detectVAAPICapabilities() *VAAAPICapabilities {
	devicePath := detectVAAPIDevice()
	if devicePath == "" {
		return nil
	}

	caps := &VAAAPICapabilities{
		DevicePath:          devicePath,
		SupportsH264:        true, // H.264 is widely supported
		SupportsLowLatency:  true,
		SupportsHighQuality: true,
		MaxBitrate:          10000, // 10 Mbps default
		MaxResolution:       "1920x1080",
	}

	// Try to detect driver and capabilities
	if driverInfo := detectVAAPIDriver(); driverInfo != "" {
		caps.DriverName = driverInfo

		// Intel drivers generally support VP8/VP9 on newer hardware
		if strings.Contains(strings.ToLower(driverInfo), "intel") {
			caps.SupportsVP8 = true
			caps.SupportsVP9 = true
			caps.MaxBitrate = 20000 // Intel can handle higher bitrates
		}

		// AMD drivers have varying VP8/VP9 support
		if strings.Contains(strings.ToLower(driverInfo), "amd") || strings.Contains(strings.ToLower(driverInfo), "radeon") {
			caps.SupportsVP8 = true
			caps.MaxBitrate = 15000
		}
	}

	return caps
}

// detectVAAPIDriver detects the VAAPI driver name
func detectVAAPIDriver() string {
	// Try to get driver info from vainfo if available
	if output := runCommand("vainfo"); output != "" {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, "Driver version:") {
				return strings.TrimSpace(strings.Split(line, ":")[1])
			}
		}
	}

	// Fallback to checking kernel modules
	if output := runCommand("lsmod"); output != "" {
		if strings.Contains(output, "i915") {
			return "Intel i915"
		}
		if strings.Contains(output, "amdgpu") {
			return "AMD GPU"
		}
		if strings.Contains(output, "radeon") {
			return "AMD Radeon"
		}
	}

	return "Unknown"
}

// supportsVP8VP9 checks if the system supports VP8/VP9 encoding via VAAPI
func supportsVP8VP9() bool {
	// In real implementation, this would query VAAPI capabilities
	// For now, we'll do a simple check based on driver detection
	driver := detectVAAPIDriver()
	return strings.Contains(strings.ToLower(driver), "intel") ||
		strings.Contains(strings.ToLower(driver), "amd")
}

// mapQualityToVAAPILevel maps quality value to VAAPI quality level
func mapQualityToVAAPILevel(quality int) int {
	// VAAPI quality levels are typically 1-7, with 1 being highest quality
	// Map our 0-51 quality scale to VAAPI levels
	if quality <= 10 {
		return 1 // Highest quality
	} else if quality <= 20 {
		return 2
	} else if quality <= 30 {
		return 3
	} else if quality <= 40 {
		return 4
	} else if quality <= 45 {
		return 5
	} else if quality <= 50 {
		return 6
	} else {
		return 7 // Lowest quality
	}
}

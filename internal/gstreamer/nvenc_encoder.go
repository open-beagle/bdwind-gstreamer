package gstreamer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// NVENCEncoder implements hardware-accelerated encoding using NVIDIA NVENC
type NVENCEncoder struct {
	*BaseEncoder
	deviceID int
}

// NewNVENCEncoder creates a new NVENC encoder
func NewNVENCEncoder(config config.EncoderConfig) (*NVENCEncoder, error) {
	if !IsNVENCAvailable() {
		return nil, fmt.Errorf("NVENC is not available on this system")
	}

	encoder := &NVENCEncoder{
		BaseEncoder: NewBaseEncoder(config),
		deviceID:    config.HardwareDevice,
	}

	if err := encoder.Initialize(config); err != nil {
		return nil, fmt.Errorf("failed to initialize NVENC encoder: %w", err)
	}

	return encoder, nil
}

// Initialize initializes the NVENC encoder
func (e *NVENCEncoder) Initialize(cfg config.EncoderConfig) error {
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
		return fmt.Errorf("codec %s is not supported by NVENC encoder", cfg.Codec)
	}

	e.config = cfg
	return nil
}

// GetElement returns the GStreamer element name and properties for NVENC
func (e *NVENCEncoder) GetElement() (string, map[string]interface{}, error) {
	var elementName string
	properties := make(map[string]interface{})

	// Select appropriate NVENC element based on codec
	switch e.config.Codec {
	case config.CodecH264:
		elementName = "nvh264enc"
	case config.CodecVP8:
		return "", nil, fmt.Errorf("VP8 is not supported by NVENC")
	case config.CodecVP9:
		return "", nil, fmt.Errorf("VP9 is not supported by NVENC")
	default:
		return "", nil, fmt.Errorf("unsupported codec: %s", e.config.Codec)
	}

	// Set basic properties
	properties["bitrate"] = e.config.Bitrate
	properties["max-bitrate"] = e.config.MaxBitrate
	properties["gop-size"] = e.config.KeyframeInterval * 30 // Assuming 30fps, convert seconds to frames
	properties["gpu-id"] = e.deviceID

	// Set rate control mode
	switch e.config.RateControl {
	case config.RateControlCBR:
		properties["rc-mode"] = "cbr"
	case config.RateControlVBR:
		properties["rc-mode"] = "vbr"
	case config.RateControlCQP:
		properties["rc-mode"] = "cqp"
		properties["qp-const"] = e.config.Quality
	}

	// Set preset (performance vs quality trade-off)
	switch e.config.Preset {
	case config.PresetUltraFast:
		properties["preset"] = "hp" // High Performance
	case config.PresetSuperFast, config.PresetVeryFast, config.PresetFaster, config.PresetFast:
		properties["preset"] = "hp"
	case config.PresetMedium:
		properties["preset"] = "default"
	case config.PresetSlow, config.PresetSlower, config.PresetVerySlow:
		properties["preset"] = "hq" // High Quality
	default:
		properties["preset"] = "default"
	}

	// Set profile
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

	// Zero latency settings
	if e.config.ZeroLatency {
		properties["zerolatency"] = true
		properties["b-frames"] = 0
		properties["lookahead"] = 0
	} else {
		properties["b-frames"] = e.config.BFrames
		if e.config.LookAhead > 0 {
			properties["lookahead"] = e.config.LookAhead
		}
	}

	// Advanced settings
	if e.config.RefFrames > 0 {
		properties["ref-frames"] = e.config.RefFrames
	}

	// Custom GStreamer options
	for key, value := range e.config.GStreamerOptions {
		properties[key] = value
	}

	return elementName, properties, nil
}

// UpdateBitrate dynamically updates the encoder bitrate
func (e *NVENCEncoder) UpdateBitrate(bitrate int) error {
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
func (e *NVENCEncoder) ForceKeyframe() error {
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

// IsHardwareAccelerated returns true since NVENC is hardware-accelerated
func (e *NVENCEncoder) IsHardwareAccelerated() bool {
	return true
}

// GetSupportedCodecs returns the codecs supported by NVENC
func (e *NVENCEncoder) GetSupportedCodecs() []config.CodecType {
	return []config.CodecType{
		config.CodecH264,
		// NVENC also supports HEVC but we're focusing on H264 for now
	}
}

// Cleanup cleans up NVENC encoder resources
func (e *NVENCEncoder) Cleanup() error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	// In real implementation, this would clean up GStreamer resources
	if e.element != nil {
		// gst_object_unref(e.element)
		e.element = nil
	}

	return nil
}

// GetOptimalNVENCConfig returns optimal NVENC configuration based on system capabilities
func GetOptimalNVENCConfig() config.EncoderConfig {
	cfg := config.DefaultEncoderConfig()
	cfg.Type = config.EncoderTypeNVENC
	cfg.UseHardware = true

	// Detect GPU capabilities and adjust settings accordingly
	if gpuInfo := detectNVIDIAGPU(); gpuInfo != nil {
		// Adjust settings based on GPU generation and capabilities
		if gpuInfo.Generation >= 7 { // Maxwell and newer
			cfg.Preset = config.PresetFast
			cfg.LookAhead = 8
		}
		if gpuInfo.Generation >= 8 { // Pascal and newer
			cfg.BFrames = 2
		}
		if gpuInfo.Generation >= 10 { // Turing and newer
			cfg.LookAhead = 16
		}
	}

	return cfg
}

// GPUInfo represents NVIDIA GPU information
type GPUInfo struct {
	Name       string
	Generation int
	Memory     int64
	CUDACores  int
}

// detectNVIDIAGPU detects NVIDIA GPU information (simplified implementation)
func detectNVIDIAGPU() *GPUInfo {
	// In real implementation, this would use nvidia-ml-py or similar to detect GPU info
	// For now, we'll do a simple check
	if !fileExists("/proc/driver/nvidia/version") {
		return nil
	}

	// Try to read GPU info from nvidia-smi if available
	if output := runCommand("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits"); output != "" {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		if len(lines) > 0 {
			parts := strings.Split(lines[0], ",")
			if len(parts) >= 2 {
				name := strings.TrimSpace(parts[0])
				memoryStr := strings.TrimSpace(parts[1])
				if memory, err := strconv.ParseInt(memoryStr, 10, 64); err == nil {
					return &GPUInfo{
						Name:       name,
						Generation: detectGPUGeneration(name),
						Memory:     memory * 1024 * 1024, // Convert MB to bytes
					}
				}
			}
		}
	}

	// Fallback to basic detection
	return &GPUInfo{
		Name:       "Unknown NVIDIA GPU",
		Generation: 6,                  // Assume older generation for safety
		Memory:     1024 * 1024 * 1024, // 1GB default
	}
}

// detectGPUGeneration detects GPU generation from name (simplified)
func detectGPUGeneration(name string) int {
	name = strings.ToLower(name)

	// RTX 40 series (Ada Lovelace)
	if strings.Contains(name, "rtx 40") || strings.Contains(name, "rtx 4") {
		return 12
	}
	// RTX 30 series (Ampere)
	if strings.Contains(name, "rtx 30") || strings.Contains(name, "rtx 3") {
		return 11
	}
	// RTX 20 series (Turing)
	if strings.Contains(name, "rtx 20") || strings.Contains(name, "rtx 2") || strings.Contains(name, "gtx 16") {
		return 10
	}
	// GTX 10 series (Pascal)
	if strings.Contains(name, "gtx 10") || strings.Contains(name, "titan x") {
		return 9
	}
	// GTX 900 series (Maxwell)
	if strings.Contains(name, "gtx 9") || strings.Contains(name, "gtx 8") {
		return 8
	}
	// GTX 700 series (Kepler)
	if strings.Contains(name, "gtx 7") || strings.Contains(name, "gtx 6") {
		return 7
	}

	// Default to older generation
	return 6
}

package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Rect represents a rectangular region
type Rect struct {
	X      int `yaml:"x" json:"x"`
	Y      int `yaml:"y" json:"y"`
	Width  int `yaml:"width" json:"width"`
	Height int `yaml:"height" json:"height"`
}

// DesktopCaptureConfig contains configuration for desktop capture
type DesktopCaptureConfig struct {
	DisplayID     string `yaml:"display_id" json:"display_id"`         // Display ID (e.g., ":0")
	FrameRate     int    `yaml:"frame_rate" json:"frame_rate"`         // Capture frame rate
	Width         int    `yaml:"width" json:"width"`                   // Capture width
	Height        int    `yaml:"height" json:"height"`                 // Capture height
	UseWayland    bool   `yaml:"use_wayland" json:"use_wayland"`       // Use Wayland (otherwise X11)
	CaptureRegion *Rect  `yaml:"capture_region" json:"capture_region"` // Optional capture region
	AutoDetect    bool   `yaml:"auto_detect" json:"auto_detect"`       // Auto-detect display server
	ShowPointer   bool   `yaml:"show_pointer" json:"show_pointer"`     // Show mouse pointer
	UseDamage     bool   `yaml:"use_damage" json:"use_damage"`         // Use damage events for optimization
	BufferSize    int    `yaml:"buffer_size" json:"buffer_size"`       // Buffer size for capture
	Quality       string `yaml:"quality" json:"quality"`               // Quality preset (low, medium, high)
}

// DefaultDesktopCaptureConfig returns a configuration with default values
func DefaultDesktopCaptureConfig() DesktopCaptureConfig {
	return DesktopCaptureConfig{
		DisplayID:     ":0",
		FrameRate:     30,
		Width:         1920,
		Height:        1080,
		UseWayland:    false,
		CaptureRegion: nil,
		AutoDetect:    true,
		ShowPointer:   false,
		UseDamage:     false,
		BufferSize:    10,
		Quality:       "medium",
	}
}

// ValidateDesktopCaptureConfig validates the configuration and returns an error if invalid
func ValidateDesktopCaptureConfig(config *DesktopCaptureConfig) error {
	// Validate frame rate
	if config.FrameRate <= 0 || config.FrameRate > 120 {
		return fmt.Errorf("invalid frame rate: %d (must be between 1 and 120)", config.FrameRate)
	}

	// Validate resolution
	if config.Width <= 0 || config.Width > 7680 {
		return fmt.Errorf("invalid width: %d (must be between 1 and 7680)", config.Width)
	}
	if config.Height <= 0 || config.Height > 4320 {
		return fmt.Errorf("invalid height: %d (must be between 1 and 4320)", config.Height)
	}

	// Validate capture region if specified
	if config.CaptureRegion != nil {
		if config.CaptureRegion.Width <= 0 || config.CaptureRegion.Height <= 0 {
			return fmt.Errorf("invalid capture region dimensions: %dx%d",
				config.CaptureRegion.Width, config.CaptureRegion.Height)
		}
		if config.CaptureRegion.X < 0 || config.CaptureRegion.Y < 0 {
			return fmt.Errorf("invalid capture region position: %d,%d",
				config.CaptureRegion.X, config.CaptureRegion.Y)
		}
	}

	// Validate buffer size
	if config.BufferSize <= 0 || config.BufferSize > 100 {
		return fmt.Errorf("invalid buffer size: %d (must be between 1 and 100)", config.BufferSize)
	}

	// Validate quality preset
	validQualities := []string{"low", "medium", "high"}
	validQuality := false
	for _, q := range validQualities {
		if config.Quality == q {
			validQuality = true
			break
		}
	}
	if !validQuality {
		return fmt.Errorf("invalid quality preset: %s (must be one of: %s)",
			config.Quality, strings.Join(validQualities, ", "))
	}

	return nil
}

// DetectDisplayServer detects whether the system is using X11 or Wayland
func DetectDisplayServer() (bool, error) {
	// Check if WAYLAND_DISPLAY environment variable is set
	wayland := os.Getenv("WAYLAND_DISPLAY")
	if wayland != "" {
		return true, nil
	}

	// Check if XDG_SESSION_TYPE is set to wayland
	sessionType := os.Getenv("XDG_SESSION_TYPE")
	if sessionType == "wayland" {
		return true, nil
	}

	// Default to X11
	return false, nil
}

// LoadDesktopCaptureConfigFromEnv loads configuration from environment variables
func LoadDesktopCaptureConfigFromEnv() DesktopCaptureConfig {
	config := DefaultDesktopCaptureConfig()

	// Load from environment variables with BDWIND_CAPTURE_ prefix
	if displayID := os.Getenv("BDWIND_CAPTURE_DISPLAY_ID"); displayID != "" {
		config.DisplayID = displayID
	}
	if display := os.Getenv("DISPLAY"); display != "" && config.DisplayID == ":0" {
		config.DisplayID = display
	}

	if frameRateStr := os.Getenv("BDWIND_CAPTURE_FRAME_RATE"); frameRateStr != "" {
		if frameRate, err := strconv.Atoi(frameRateStr); err == nil {
			config.FrameRate = frameRate
		}
	}

	if widthStr := os.Getenv("BDWIND_CAPTURE_WIDTH"); widthStr != "" {
		if width, err := strconv.Atoi(widthStr); err == nil {
			config.Width = width
		}
	}

	if heightStr := os.Getenv("BDWIND_CAPTURE_HEIGHT"); heightStr != "" {
		if height, err := strconv.Atoi(heightStr); err == nil {
			config.Height = height
		}
	}

	if useWaylandStr := os.Getenv("BDWIND_CAPTURE_USE_WAYLAND"); useWaylandStr != "" {
		if useWayland, err := strconv.ParseBool(useWaylandStr); err == nil {
			config.UseWayland = useWayland
		}
	}

	if autoDetectStr := os.Getenv("BDWIND_CAPTURE_AUTO_DETECT"); autoDetectStr != "" {
		if autoDetect, err := strconv.ParseBool(autoDetectStr); err == nil {
			config.AutoDetect = autoDetect
		}
	}

	if showPointerStr := os.Getenv("BDWIND_CAPTURE_SHOW_POINTER"); showPointerStr != "" {
		if showPointer, err := strconv.ParseBool(showPointerStr); err == nil {
			config.ShowPointer = showPointer
		}
	}

	if useDamageStr := os.Getenv("BDWIND_CAPTURE_USE_DAMAGE"); useDamageStr != "" {
		if useDamage, err := strconv.ParseBool(useDamageStr); err == nil {
			config.UseDamage = useDamage
		}
	}

	if bufferSizeStr := os.Getenv("BDWIND_CAPTURE_BUFFER_SIZE"); bufferSizeStr != "" {
		if bufferSize, err := strconv.Atoi(bufferSizeStr); err == nil {
			config.BufferSize = bufferSize
		}
	}

	if quality := os.Getenv("BDWIND_CAPTURE_QUALITY"); quality != "" {
		config.Quality = quality
	}

	// Load capture region from environment variables
	if regionStr := os.Getenv("BDWIND_CAPTURE_REGION"); regionStr != "" {
		parts := strings.Split(regionStr, ",")
		if len(parts) == 4 {
			if x, err1 := strconv.Atoi(parts[0]); err1 == nil {
				if y, err2 := strconv.Atoi(parts[1]); err2 == nil {
					if w, err3 := strconv.Atoi(parts[2]); err3 == nil {
						if h, err4 := strconv.Atoi(parts[3]); err4 == nil {
							config.CaptureRegion = &Rect{X: x, Y: y, Width: w, Height: h}
						}
					}
				}
			}
		}
	}

	// Auto-detect display server if enabled
	if config.AutoDetect {
		if isWayland, err := DetectDisplayServer(); err == nil {
			config.UseWayland = isWayland
		}
	}

	return config
}

// LoadDesktopCaptureConfigFromFile loads configuration from a YAML file
func LoadDesktopCaptureConfigFromFile(filename string) (DesktopCaptureConfig, error) {
	config := DefaultDesktopCaptureConfig()

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Auto-detect display server if enabled
	if config.AutoDetect {
		if isWayland, err := DetectDisplayServer(); err == nil {
			config.UseWayland = isWayland
		}
	}

	return config, nil
}

// SaveDesktopCaptureConfigToFile saves configuration to a YAML file
func SaveDesktopCaptureConfigToFile(config DesktopCaptureConfig, filename string) error {
	data, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	err = ioutil.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetAvailableDisplays returns a list of available displays
func GetAvailableDisplays() ([]string, error) {
	// In a real implementation, we would query the system for available displays
	// This is a simplified implementation

	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}

	return []string{display}, nil
}

// GetOptimalFrameRate returns the optimal frame rate for the current system
func GetOptimalFrameRate() (int, error) {
	// In a real implementation, we would determine this based on system capabilities
	// This is a simplified implementation

	// Try to get from environment variable
	framerateStr := os.Getenv("BDWIND_FRAMERATE")
	if framerateStr != "" {
		framerate, err := strconv.Atoi(framerateStr)
		if err == nil && framerate > 0 {
			return framerate, nil
		}
	}

	// Default to 30 fps
	return 30, nil
}

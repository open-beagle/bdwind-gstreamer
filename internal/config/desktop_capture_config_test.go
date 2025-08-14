package config

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDesktopCaptureConfig(t *testing.T) {
	config := DefaultDesktopCaptureConfig()

	if config.DisplayID != ":0" {
		t.Errorf("Expected DisplayID to be ':0', got %s", config.DisplayID)
	}
	if config.FrameRate != 30 {
		t.Errorf("Expected FrameRate to be 30, got %d", config.FrameRate)
	}
	if config.Width != 1920 {
		t.Errorf("Expected Width to be 1920, got %d", config.Width)
	}
	if config.Height != 1080 {
		t.Errorf("Expected Height to be 1080, got %d", config.Height)
	}
	if config.UseWayland != false {
		t.Errorf("Expected UseWayland to be false, got %t", config.UseWayland)
	}
	if config.AutoDetect != true {
		t.Errorf("Expected AutoDetect to be true, got %t", config.AutoDetect)
	}
	if config.Quality != "medium" {
		t.Errorf("Expected Quality to be 'medium', got %s", config.Quality)
	}
}

func TestValidateDesktopCaptureConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      DesktopCaptureConfig
		expectError bool
	}{
		{
			name:        "Valid default config",
			config:      DefaultDesktopCaptureConfig(),
			expectError: false,
		},
		{
			name: "Invalid frame rate - too low",
			config: DesktopCaptureConfig{
				FrameRate:  0,
				Width:      1920,
				Height:     1080,
				Quality:    "medium",
				BufferSize: 10,
			},
			expectError: true,
		},
		{
			name: "Invalid frame rate - too high",
			config: DesktopCaptureConfig{
				FrameRate:  150,
				Width:      1920,
				Height:     1080,
				Quality:    "medium",
				BufferSize: 10,
			},
			expectError: true,
		},
		{
			name: "Invalid width",
			config: DesktopCaptureConfig{
				FrameRate:  30,
				Width:      0,
				Height:     1080,
				Quality:    "medium",
				BufferSize: 10,
			},
			expectError: true,
		},
		{
			name: "Invalid height",
			config: DesktopCaptureConfig{
				FrameRate:  30,
				Width:      1920,
				Height:     0,
				Quality:    "medium",
				BufferSize: 10,
			},
			expectError: true,
		},
		{
			name: "Invalid quality",
			config: DesktopCaptureConfig{
				FrameRate:  30,
				Width:      1920,
				Height:     1080,
				Quality:    "invalid",
				BufferSize: 10,
			},
			expectError: true,
		},
		{
			name: "Invalid buffer size",
			config: DesktopCaptureConfig{
				FrameRate:  30,
				Width:      1920,
				Height:     1080,
				Quality:    "medium",
				BufferSize: 0,
			},
			expectError: true,
		},
		{
			name: "Invalid capture region",
			config: DesktopCaptureConfig{
				FrameRate:     30,
				Width:         1920,
				Height:        1080,
				Quality:       "medium",
				BufferSize:    10,
				CaptureRegion: &Rect{X: -1, Y: -1, Width: 100, Height: 100},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDesktopCaptureConfig(&tt.config)
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestLoadDesktopCaptureConfigFromEnv(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"BDWIND_CAPTURE_DISPLAY_ID",
		"BDWIND_CAPTURE_FRAME_RATE",
		"BDWIND_CAPTURE_WIDTH",
		"BDWIND_CAPTURE_HEIGHT",
		"BDWIND_CAPTURE_USE_WAYLAND",
		"BDWIND_CAPTURE_AUTO_DETECT",
		"BDWIND_CAPTURE_SHOW_POINTER",
		"BDWIND_CAPTURE_USE_DAMAGE",
		"BDWIND_CAPTURE_BUFFER_SIZE",
		"BDWIND_CAPTURE_QUALITY",
		"BDWIND_CAPTURE_REGION",
	}

	for _, envVar := range envVars {
		originalEnv[envVar] = os.Getenv(envVar)
		os.Unsetenv(envVar)
	}

	// Restore environment after test
	defer func() {
		for _, envVar := range envVars {
			if val, exists := originalEnv[envVar]; exists && val != "" {
				os.Setenv(envVar, val)
			} else {
				os.Unsetenv(envVar)
			}
		}
	}()

	// Test with environment variables set
	os.Setenv("BDWIND_CAPTURE_DISPLAY_ID", ":1")
	os.Setenv("BDWIND_CAPTURE_FRAME_RATE", "60")
	os.Setenv("BDWIND_CAPTURE_WIDTH", "2560")
	os.Setenv("BDWIND_CAPTURE_HEIGHT", "1440")
	os.Setenv("BDWIND_CAPTURE_USE_WAYLAND", "true")
	os.Setenv("BDWIND_CAPTURE_AUTO_DETECT", "false")
	os.Setenv("BDWIND_CAPTURE_SHOW_POINTER", "true")
	os.Setenv("BDWIND_CAPTURE_USE_DAMAGE", "true")
	os.Setenv("BDWIND_CAPTURE_BUFFER_SIZE", "20")
	os.Setenv("BDWIND_CAPTURE_QUALITY", "high")
	os.Setenv("BDWIND_CAPTURE_REGION", "100,200,800,600")

	config := LoadDesktopCaptureConfigFromEnv()

	if config.DisplayID != ":1" {
		t.Errorf("Expected DisplayID to be ':1', got %s", config.DisplayID)
	}
	if config.FrameRate != 60 {
		t.Errorf("Expected FrameRate to be 60, got %d", config.FrameRate)
	}
	if config.Width != 2560 {
		t.Errorf("Expected Width to be 2560, got %d", config.Width)
	}
	if config.Height != 1440 {
		t.Errorf("Expected Height to be 1440, got %d", config.Height)
	}
	if config.UseWayland != true {
		t.Errorf("Expected UseWayland to be true, got %t", config.UseWayland)
	}
	if config.AutoDetect != false {
		t.Errorf("Expected AutoDetect to be false, got %t", config.AutoDetect)
	}
	if config.ShowPointer != true {
		t.Errorf("Expected ShowPointer to be true, got %t", config.ShowPointer)
	}
	if config.UseDamage != true {
		t.Errorf("Expected UseDamage to be true, got %t", config.UseDamage)
	}
	if config.BufferSize != 20 {
		t.Errorf("Expected BufferSize to be 20, got %d", config.BufferSize)
	}
	if config.Quality != "high" {
		t.Errorf("Expected Quality to be 'high', got %s", config.Quality)
	}
	if config.CaptureRegion == nil {
		t.Error("Expected CaptureRegion to be set")
	} else {
		if config.CaptureRegion.X != 100 || config.CaptureRegion.Y != 200 ||
			config.CaptureRegion.Width != 800 || config.CaptureRegion.Height != 600 {
			t.Errorf("Expected CaptureRegion to be {100,200,800,600}, got {%d,%d,%d,%d}",
				config.CaptureRegion.X, config.CaptureRegion.Y,
				config.CaptureRegion.Width, config.CaptureRegion.Height)
		}
	}
}

func TestLoadDesktopCaptureConfigFromFile(t *testing.T) {
	// Create a temporary config file
	tempDir, err := ioutil.TempDir("", "desktop_capture_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "config.yaml")
	configContent := `
display_id: ":2"
frame_rate: 45
width: 3840
height: 2160
use_wayland: true
auto_detect: false
show_pointer: true
use_damage: true
buffer_size: 15
quality: "high"
capture_region:
  x: 50
  y: 100
  width: 1920
  height: 1080
`

	err = ioutil.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := LoadDesktopCaptureConfigFromFile(configFile)
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	if config.DisplayID != ":2" {
		t.Errorf("Expected DisplayID to be ':2', got %s", config.DisplayID)
	}
	if config.FrameRate != 45 {
		t.Errorf("Expected FrameRate to be 45, got %d", config.FrameRate)
	}
	if config.Width != 3840 {
		t.Errorf("Expected Width to be 3840, got %d", config.Width)
	}
	if config.Height != 2160 {
		t.Errorf("Expected Height to be 2160, got %d", config.Height)
	}
	if config.UseWayland != true {
		t.Errorf("Expected UseWayland to be true, got %t", config.UseWayland)
	}
	if config.AutoDetect != false {
		t.Errorf("Expected AutoDetect to be false, got %t", config.AutoDetect)
	}
	if config.ShowPointer != true {
		t.Errorf("Expected ShowPointer to be true, got %t", config.ShowPointer)
	}
	if config.UseDamage != true {
		t.Errorf("Expected UseDamage to be true, got %t", config.UseDamage)
	}
	if config.BufferSize != 15 {
		t.Errorf("Expected BufferSize to be 15, got %d", config.BufferSize)
	}
	if config.Quality != "high" {
		t.Errorf("Expected Quality to be 'high', got %s", config.Quality)
	}
	if config.CaptureRegion == nil {
		t.Error("Expected CaptureRegion to be set")
	} else {
		if config.CaptureRegion.X != 50 || config.CaptureRegion.Y != 100 ||
			config.CaptureRegion.Width != 1920 || config.CaptureRegion.Height != 1080 {
			t.Errorf("Expected CaptureRegion to be {50,100,1920,1080}, got {%d,%d,%d,%d}",
				config.CaptureRegion.X, config.CaptureRegion.Y,
				config.CaptureRegion.Width, config.CaptureRegion.Height)
		}
	}
}

func TestSaveDesktopCaptureConfigToFile(t *testing.T) {
	// Create a temporary directory
	tempDir, err := ioutil.TempDir("", "desktop_capture_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "config.yaml")

	// Create a test configuration
	config := DesktopCaptureConfig{
		DisplayID:     ":3",
		FrameRate:     75,
		Width:         2560,
		Height:        1440,
		UseWayland:    true,
		AutoDetect:    false,
		ShowPointer:   true,
		UseDamage:     true,
		BufferSize:    25,
		Quality:       "high",
		CaptureRegion: &Rect{X: 10, Y: 20, Width: 1000, Height: 800},
	}

	// Save configuration to file
	err = SaveDesktopCaptureConfigToFile(config, configFile)
	if err != nil {
		t.Fatalf("Failed to save config to file: %v", err)
	}

	// Load configuration back from file
	loadedConfig, err := LoadDesktopCaptureConfigFromFile(configFile)
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	// Compare configurations
	if loadedConfig.DisplayID != config.DisplayID {
		t.Errorf("DisplayID mismatch: expected %s, got %s", config.DisplayID, loadedConfig.DisplayID)
	}
	if loadedConfig.FrameRate != config.FrameRate {
		t.Errorf("FrameRate mismatch: expected %d, got %d", config.FrameRate, loadedConfig.FrameRate)
	}
	if loadedConfig.Width != config.Width {
		t.Errorf("Width mismatch: expected %d, got %d", config.Width, loadedConfig.Width)
	}
	if loadedConfig.Height != config.Height {
		t.Errorf("Height mismatch: expected %d, got %d", config.Height, loadedConfig.Height)
	}
	if loadedConfig.UseWayland != config.UseWayland {
		t.Errorf("UseWayland mismatch: expected %t, got %t", config.UseWayland, loadedConfig.UseWayland)
	}
	if loadedConfig.Quality != config.Quality {
		t.Errorf("Quality mismatch: expected %s, got %s", config.Quality, loadedConfig.Quality)
	}
}

func TestDetectDisplayServer(t *testing.T) {
	// Save original environment
	originalWaylandDisplay := os.Getenv("WAYLAND_DISPLAY")
	originalSessionType := os.Getenv("XDG_SESSION_TYPE")

	// Restore environment after test
	defer func() {
		if originalWaylandDisplay != "" {
			os.Setenv("WAYLAND_DISPLAY", originalWaylandDisplay)
		} else {
			os.Unsetenv("WAYLAND_DISPLAY")
		}
		if originalSessionType != "" {
			os.Setenv("XDG_SESSION_TYPE", originalSessionType)
		} else {
			os.Unsetenv("XDG_SESSION_TYPE")
		}
	}()

	// Test Wayland detection via WAYLAND_DISPLAY
	os.Setenv("WAYLAND_DISPLAY", "wayland-0")
	os.Unsetenv("XDG_SESSION_TYPE")

	isWayland, err := DetectDisplayServer()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !isWayland {
		t.Error("Expected Wayland to be detected via WAYLAND_DISPLAY")
	}

	// Test Wayland detection via XDG_SESSION_TYPE
	os.Unsetenv("WAYLAND_DISPLAY")
	os.Setenv("XDG_SESSION_TYPE", "wayland")

	isWayland, err = DetectDisplayServer()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !isWayland {
		t.Error("Expected Wayland to be detected via XDG_SESSION_TYPE")
	}

	// Test X11 detection (default)
	os.Unsetenv("WAYLAND_DISPLAY")
	os.Unsetenv("XDG_SESSION_TYPE")

	isWayland, err = DetectDisplayServer()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if isWayland {
		t.Error("Expected X11 to be detected as default")
	}
}

func TestGetAvailableDisplays(t *testing.T) {
	displays, err := GetAvailableDisplays()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(displays) == 0 {
		t.Error("Expected at least one display")
	}
}

func TestGetOptimalFrameRate(t *testing.T) {
	// Save original environment
	originalFrameRate := os.Getenv("BDWIND_FRAMERATE")

	// Restore environment after test
	defer func() {
		if originalFrameRate != "" {
			os.Setenv("BDWIND_FRAMERATE", originalFrameRate)
		} else {
			os.Unsetenv("BDWIND_FRAMERATE")
		}
	}()

	// Test with environment variable set
	os.Setenv("BDWIND_FRAMERATE", "60")

	frameRate, err := GetOptimalFrameRate()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if frameRate != 60 {
		t.Errorf("Expected frame rate to be 60, got %d", frameRate)
	}

	// Test with default value
	os.Unsetenv("BDWIND_FRAMERATE")

	frameRate, err = GetOptimalFrameRate()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if frameRate != 30 {
		t.Errorf("Expected default frame rate to be 30, got %d", frameRate)
	}
}

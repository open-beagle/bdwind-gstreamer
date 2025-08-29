package gstreamer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestNewDesktopCaptureGst(t *testing.T) {
	tests := []struct {
		name        string
		config      config.DesktopCaptureConfig
		expectError bool
	}{
		{
			name:        "Valid default config",
			config:      config.DefaultDesktopCaptureConfig(),
			expectError: false,
		},
		{
			name: "Invalid frame rate",
			config: config.DesktopCaptureConfig{
				FrameRate: 0,
				Width:     1920,
				Height:    1080,
			},
			expectError: true,
		},
		{
			name: "Invalid resolution",
			config: config.DesktopCaptureConfig{
				FrameRate: 30,
				Width:     0,
				Height:    1080,
			},
			expectError: true,
		},
		{
			name: "Valid X11 config",
			config: config.DesktopCaptureConfig{
				DisplayID:   ":0",
				FrameRate:   30,
				Width:       1920,
				Height:      1080,
				UseWayland:  false,
				ShowPointer: true,
				UseDamage:   true,
				BufferSize:  10,
				Quality:     "medium",
			},
			expectError: false,
		},
		{
			name: "Valid Wayland config",
			config: config.DesktopCaptureConfig{
				DisplayID:   "wayland-0",
				FrameRate:   60,
				Width:       2560,
				Height:      1440,
				UseWayland:  true,
				ShowPointer: false,
				BufferSize:  20,
				Quality:     "high",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc, err := NewDesktopCaptureGst(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, dc)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, dc)

				if dc != nil {
					// Verify configuration is stored correctly
					assert.Equal(t, tt.config.DisplayID, dc.config.DisplayID)
					assert.Equal(t, tt.config.FrameRate, dc.config.FrameRate)
					assert.Equal(t, tt.config.Width, dc.config.Width)
					assert.Equal(t, tt.config.Height, dc.config.Height)
					assert.Equal(t, tt.config.UseWayland, dc.config.UseWayland)

					// Verify channels are created
					assert.NotNil(t, dc.sampleChan)
					assert.NotNil(t, dc.errorChan)

					// Verify initial state
					assert.False(t, dc.IsRunning())

					// Clean up
					dc.Stop()
				}
			}
		})
	}
}

func TestDesktopCaptureGst_BuildCapsString(t *testing.T) {
	tests := []struct {
		name     string
		config   config.DesktopCaptureConfig
		expected string
	}{
		{
			name: "Low quality",
			config: config.DesktopCaptureConfig{
				Width:     1920,
				Height:    1080,
				FrameRate: 30,
				Quality:   "low",
			},
			expected: "video/x-raw,format=RGB,width=1920,height=1080,framerate=30/1,pixel-aspect-ratio=1/1",
		},
		{
			name: "Medium quality",
			config: config.DesktopCaptureConfig{
				Width:     1280,
				Height:    720,
				FrameRate: 60,
				Quality:   "medium",
			},
			expected: "video/x-raw,format=BGRx,width=1280,height=720,framerate=60/1,pixel-aspect-ratio=1/1",
		},
		{
			name: "High quality",
			config: config.DesktopCaptureConfig{
				Width:     2560,
				Height:    1440,
				FrameRate: 30,
				Quality:   "high",
			},
			expected: "video/x-raw,format=BGRA,width=2560,height=1440,framerate=30/1,pixel-aspect-ratio=1/1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc, err := NewDesktopCaptureGst(tt.config)
			require.NoError(t, err)
			require.NotNil(t, dc)

			capsStr := dc.buildCapsString()
			assert.Equal(t, tt.expected, capsStr)

			dc.Stop()
		})
	}
}

func TestDesktopCaptureGst_StateManagement(t *testing.T) {
	config := config.DefaultDesktopCaptureConfig()
	dc, err := NewDesktopCaptureGst(config)
	require.NoError(t, err)
	require.NotNil(t, dc)

	// Initial state should be not running
	assert.False(t, dc.IsRunning())

	// Test multiple stops (should not error)
	err = dc.Stop()
	assert.NoError(t, err)
	assert.False(t, dc.IsRunning())

	// Clean up
	dc.Stop()
}

func TestDesktopCaptureGst_Channels(t *testing.T) {
	config := config.DefaultDesktopCaptureConfig()
	dc, err := NewDesktopCaptureGst(config)
	require.NoError(t, err)
	require.NotNil(t, dc)

	// Test sample channel
	sampleChan := dc.GetSampleChannel()
	assert.NotNil(t, sampleChan)

	// Test error channel
	errorChan := dc.GetErrorChannel()
	assert.NotNil(t, errorChan)

	// Channels should be readable
	select {
	case <-sampleChan:
		// Should not receive anything initially
		t.Error("Should not receive samples when not running")
	case <-errorChan:
		// Should not receive errors initially
		t.Error("Should not receive errors when not running")
	case <-time.After(100 * time.Millisecond):
		// Expected - no data available
	}

	dc.Stop()
}

func TestDesktopCaptureGst_Statistics(t *testing.T) {
	config := config.DefaultDesktopCaptureConfig()
	dc, err := NewDesktopCaptureGst(config)
	require.NoError(t, err)
	require.NotNil(t, dc)

	// Get initial stats
	stats := dc.GetStats()
	assert.Equal(t, int64(0), stats.FramesCapture)
	assert.Equal(t, int64(0), stats.FramesDropped)
	assert.Equal(t, float64(0), stats.AverageFPS)

	dc.Stop()
}

func TestDesktopCaptureGst_ConfigureX11Source(t *testing.T) {
	config := config.DesktopCaptureConfig{
		DisplayID:     ":1",
		FrameRate:     30,
		Width:         1920,
		Height:        1080,
		UseWayland:    false,
		ShowPointer:   true,
		UseDamage:     true,
		BufferSize:    10,
		Quality:       "medium",
		CaptureRegion: &config.Rect{X: 100, Y: 100, Width: 800, Height: 600},
	}

	dc, err := NewDesktopCaptureGst(config)
	require.NoError(t, err)
	require.NotNil(t, dc)

	// Verify source element was created
	assert.NotNil(t, dc.source)

	dc.Stop()
}

func TestDesktopCaptureGst_ConfigureWaylandSource(t *testing.T) {
	config := config.DesktopCaptureConfig{
		DisplayID:     "wayland-0",
		FrameRate:     60,
		Width:         2560,
		Height:        1440,
		UseWayland:    true,
		ShowPointer:   false,
		BufferSize:    20,
		Quality:       "high",
		CaptureRegion: &config.Rect{X: 200, Y: 200, Width: 1600, Height: 900},
	}

	dc, err := NewDesktopCaptureGst(config)
	require.NoError(t, err)
	require.NotNil(t, dc)

	// Verify source element was created
	assert.NotNil(t, dc.source)

	dc.Stop()
}

func TestDesktopCaptureGst_ConvertGstSample(t *testing.T) {
	// This test would require a mock gst.Sample
	// For now, we'll test the error handling path
	config := config.DefaultDesktopCaptureConfig()
	dc, err := NewDesktopCaptureGst(config)
	require.NoError(t, err)
	require.NotNil(t, dc)

	// Test with nil sample (should return error)
	sample, err := dc.convertGstSample(nil)
	assert.Error(t, err)
	assert.Nil(t, sample)

	dc.Stop()
}

// Benchmark tests
func BenchmarkNewDesktopCaptureGst(b *testing.B) {
	config := config.DefaultDesktopCaptureConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dc, err := NewDesktopCaptureGst(config)
		if err != nil {
			b.Fatal(err)
		}
		dc.Stop()
	}
}

func BenchmarkBuildCapsString(b *testing.B) {
	config := config.DefaultDesktopCaptureConfig()
	dc, err := NewDesktopCaptureGst(config)
	if err != nil {
		b.Fatal(err)
	}
	defer dc.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dc.buildCapsString()
	}
}

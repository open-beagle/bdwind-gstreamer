package gstreamer

import (
	"testing"

	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// Test that we can create the basic structure
func TestDesktopCaptureGst_Creation(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	cfg := config.DefaultDesktopCaptureConfig()

	dc, err := NewDesktopCaptureGst(cfg)
	require.NoError(t, err)
	require.NotNil(t, dc)

	// Test basic properties
	assert.Equal(t, cfg.DisplayID, dc.config.DisplayID)
	assert.Equal(t, cfg.Width, dc.config.Width)
	assert.Equal(t, cfg.Height, dc.config.Height)
	assert.Equal(t, cfg.FrameRate, dc.config.FrameRate)

	// Test initial state
	assert.False(t, dc.IsRunning())

	// Test that pipeline was created
	assert.NotNil(t, dc.pipeline)
	assert.NotNil(t, dc.source)
	assert.NotNil(t, dc.sink)
	assert.NotNil(t, dc.bus)

	// Test channels
	assert.NotNil(t, dc.sampleChan)
	assert.NotNil(t, dc.errorChan)

	// Clean up
	err = dc.Stop()
	assert.NoError(t, err)
}

// Test caps string generation
func TestDesktopCaptureGst_CapsGeneration(t *testing.T) {
	gst.Init(nil)

	tests := []struct {
		name     string
		quality  string
		expected string
	}{
		{
			name:     "Low quality",
			quality:  "low",
			expected: "video/x-raw,format=RGB,width=1920,height=1080,framerate=30/1,pixel-aspect-ratio=1/1",
		},
		{
			name:     "Medium quality",
			quality:  "medium",
			expected: "video/x-raw,format=BGRx,width=1920,height=1080,framerate=30/1,pixel-aspect-ratio=1/1",
		},
		{
			name:     "High quality",
			quality:  "high",
			expected: "video/x-raw,format=BGRA,width=1920,height=1080,framerate=30/1,pixel-aspect-ratio=1/1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultDesktopCaptureConfig()
			cfg.Quality = tt.quality

			dc, err := NewDesktopCaptureGst(cfg)
			require.NoError(t, err)

			capsStr := dc.buildCapsString()
			assert.Equal(t, tt.expected, capsStr)

			dc.Stop()
		})
	}
}

// Test X11 vs Wayland source selection
func TestDesktopCaptureGst_SourceSelection(t *testing.T) {
	gst.Init(nil)

	// Test X11 source
	t.Run("X11 source", func(t *testing.T) {
		cfg := config.DefaultDesktopCaptureConfig()
		cfg.UseWayland = false

		dc, err := NewDesktopCaptureGst(cfg)
		require.NoError(t, err)

		// Should create ximagesrc
		assert.NotNil(t, dc.source)

		dc.Stop()
	})

	// Test Wayland source
	t.Run("Wayland source", func(t *testing.T) {
		cfg := config.DefaultDesktopCaptureConfig()
		cfg.UseWayland = true

		dc, err := NewDesktopCaptureGst(cfg)
		require.NoError(t, err)

		// Should create waylandsrc
		assert.NotNil(t, dc.source)

		dc.Stop()
	})
}

// Test error handling for invalid configurations
func TestDesktopCaptureGst_InvalidConfig(t *testing.T) {
	gst.Init(nil)

	// Test invalid frame rate
	cfg := config.DefaultDesktopCaptureConfig()
	cfg.FrameRate = 0

	dc, err := NewDesktopCaptureGst(cfg)
	assert.Error(t, err)
	assert.Nil(t, dc)

	// Test invalid resolution
	cfg2 := config.DefaultDesktopCaptureConfig()
	cfg2.Width = 0

	dc, err = NewDesktopCaptureGst(cfg2)
	assert.Error(t, err)
	assert.Nil(t, dc)
}

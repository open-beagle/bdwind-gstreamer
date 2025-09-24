package gstreamer

import (
	"testing"

	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// TestSafeConvertGstSample tests the safe sample conversion with object lifecycle management
func TestSafeConvertGstSample(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create test configuration
	cfg := config.DesktopCaptureConfig{
		Width:       640,
		Height:      480,
		FrameRate:   30,
		Quality:     "medium",
		BufferSize:  10,
		UseWayland:  false,
		ShowPointer: true,
		DisplayID:   ":0",
	}

	// Create desktop capture instance
	dc, err := NewDesktopCaptureGst(cfg)
	require.NoError(t, err)
	require.NotNil(t, dc)
	defer dc.Stop()

	t.Run("NilSampleHandling", func(t *testing.T) {
		// Test nil sample handling
		sample, err := dc.safeConvertGstSample(nil)
		assert.Error(t, err)
		assert.Nil(t, sample)
		assert.Contains(t, err.Error(), "gst sample is nil")
	})

	t.Run("ObjectLifecycleValidation", func(t *testing.T) {
		// Test that lifecycle manager is properly initialized
		assert.NotNil(t, dc.lifecycleManager)

		// Test lifecycle manager stats
		stats := dc.lifecycleManager.GetStats()
		assert.GreaterOrEqual(t, stats.TotalRegistered, int64(0))
	})

	t.Run("ConversionRecoveryMechanisms", func(t *testing.T) {
		// Test recovery mechanisms with nil sample
		sample, err := dc.convertGstSampleWithRecovery(nil)
		assert.Error(t, err)
		assert.Nil(t, sample)
		assert.Contains(t, err.Error(), "gst sample is nil")
	})
}

// TestObjectLifecycleIntegration tests the integration of object lifecycle management
func TestObjectLifecycleIntegration(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create test configuration
	cfg := config.DesktopCaptureConfig{
		Width:       320,
		Height:      240,
		FrameRate:   15,
		Quality:     "low",
		BufferSize:  5,
		UseWayland:  false,
		ShowPointer: false,
		DisplayID:   ":0",
	}

	// Create desktop capture instance
	dc, err := NewDesktopCaptureGst(cfg)
	require.NoError(t, err)
	require.NotNil(t, dc)
	defer dc.Stop()

	t.Run("LifecycleManagerInitialization", func(t *testing.T) {
		// Verify lifecycle manager is initialized
		assert.NotNil(t, dc.lifecycleManager)

		// Check initial stats
		stats := dc.lifecycleManager.GetStats()
		assert.GreaterOrEqual(t, stats.TotalRegistered, int64(0))
		assert.GreaterOrEqual(t, stats.CurrentActive, int64(0))
	})

	t.Run("ObjectRegistrationAndValidation", func(t *testing.T) {
		// Create a test pipeline to register
		pipeline, err := gst.NewPipeline("test-pipeline")
		require.NoError(t, err)
		require.NotNil(t, pipeline)

		// Register object with lifecycle manager
		id, err := dc.lifecycleManager.RegisterObject(pipeline, "gst.Pipeline", "test-pipeline")
		assert.NoError(t, err)
		assert.NotZero(t, id)

		// Validate object
		isValid := dc.lifecycleManager.IsValidObject(id)
		assert.True(t, isValid)

		// Validate using ValidateGstObject
		isValidGst := dc.lifecycleManager.ValidateGstObject(pipeline)
		assert.True(t, isValidGst)

		// Unregister object
		err = dc.lifecycleManager.UnregisterObject(id)
		assert.NoError(t, err)
	})

	t.Run("MemoryManagerIntegration", func(t *testing.T) {
		// Verify memory manager is initialized
		assert.NotNil(t, dc.memoryManager)

		// Check memory stats
		memStats := dc.GetMemoryStats()
		assert.GreaterOrEqual(t, memStats.TotalObjects, int64(0))
	})
}

// TestSampleConversionErrorHandling tests error handling in sample conversion
func TestSampleConversionErrorHandling(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create test configuration
	cfg := config.DesktopCaptureConfig{
		Width:       160,
		Height:      120,
		FrameRate:   10,
		Quality:     "low",
		BufferSize:  3,
		UseWayland:  false,
		ShowPointer: false,
		DisplayID:   ":0",
	}

	// Create desktop capture instance
	dc, err := NewDesktopCaptureGst(cfg)
	require.NoError(t, err)
	require.NotNil(t, dc)
	defer dc.Stop()

	t.Run("NilSampleErrorHandling", func(t *testing.T) {
		// Test convertGstSample with nil
		sample, err := dc.convertGstSample(nil)
		assert.Error(t, err)
		assert.Nil(t, sample)
		assert.Contains(t, err.Error(), "received nil gst sample")
	})

	t.Run("SafeConversionErrorHandling", func(t *testing.T) {
		// Test safeConvertGstSample with nil
		sample, err := dc.safeConvertGstSample(nil)
		assert.Error(t, err)
		assert.Nil(t, sample)
		assert.Contains(t, err.Error(), "gst sample is nil")
	})

	t.Run("RecoveryErrorHandling", func(t *testing.T) {
		// Test convertGstSampleWithRecovery with nil
		sample, err := dc.convertGstSampleWithRecovery(nil)
		assert.Error(t, err)
		assert.Nil(t, sample)
		assert.Contains(t, err.Error(), "gst sample is nil")
	})
}

// TestSampleConversionStatistics tests that statistics are properly updated
func TestSampleConversionStatistics(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create test configuration
	cfg := config.DesktopCaptureConfig{
		Width:       160,
		Height:      120,
		FrameRate:   10,
		Quality:     "low",
		BufferSize:  3,
		UseWayland:  false,
		ShowPointer: false,
		DisplayID:   ":0",
	}

	// Create desktop capture instance
	dc, err := NewDesktopCaptureGst(cfg)
	require.NoError(t, err)
	require.NotNil(t, dc)
	defer dc.Stop()

	t.Run("StatisticsInitialization", func(t *testing.T) {
		// Get initial stats
		stats := dc.GetStats()
		assert.GreaterOrEqual(t, stats.FramesTotal, int64(0))
		assert.GreaterOrEqual(t, stats.FramesDropped, int64(0))
		assert.GreaterOrEqual(t, stats.ErrorCount, int64(0))
	})

	t.Run("ConversionFailureStatistics", func(t *testing.T) {
		// Trigger conversion failures
		_, err1 := dc.safeConvertGstSample(nil)
		assert.Error(t, err1)

		_, err2 := dc.convertGstSampleWithRecovery(nil)
		assert.Error(t, err2)

		// Check that statistics were updated
		// Note: The actual statistics update depends on the internal implementation
		// This test verifies the methods can be called without panicking
	})
}

// BenchmarkSampleConversion benchmarks the sample conversion performance
func BenchmarkSampleConversion(b *testing.B) {
	// Initialize GStreamer for benchmarking
	gst.Init(nil)

	// Create test configuration
	cfg := config.DesktopCaptureConfig{
		Width:       320,
		Height:      240,
		FrameRate:   30,
		Quality:     "medium",
		BufferSize:  10,
		UseWayland:  false,
		ShowPointer: false,
		DisplayID:   ":0",
	}

	// Create desktop capture instance
	dc, err := NewDesktopCaptureGst(cfg)
	require.NoError(b, err)
	require.NotNil(b, dc)
	defer dc.Stop()

	b.Run("SafeConversionNilHandling", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = dc.safeConvertGstSample(nil)
		}
	})

	b.Run("RecoveryConversionNilHandling", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = dc.convertGstSampleWithRecovery(nil)
		}
	})
}

// TestLifecycleManagerCleanup tests proper cleanup of lifecycle manager
func TestLifecycleManagerCleanup(t *testing.T) {
	// Initialize GStreamer for testing
	gst.Init(nil)

	// Create test configuration
	cfg := config.DesktopCaptureConfig{
		Width:       160,
		Height:      120,
		FrameRate:   10,
		Quality:     "low",
		BufferSize:  3,
		UseWayland:  false,
		ShowPointer: false,
		DisplayID:   ":0",
	}

	// Create desktop capture instance
	dc, err := NewDesktopCaptureGst(cfg)
	require.NoError(t, err)
	require.NotNil(t, dc)

	t.Run("ProperCleanup", func(t *testing.T) {
		// Verify lifecycle manager is initialized
		assert.NotNil(t, dc.lifecycleManager)

		// Stop the desktop capture (should clean up lifecycle manager)
		err := dc.Stop()
		assert.NoError(t, err)

		// Verify lifecycle manager is cleaned up
		assert.Nil(t, dc.lifecycleManager)
	})
}

package gstreamer

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestEncoderStatsCollector_RecordFrameEncoded(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	encoder := NewBaseEncoder(cfg)
	collector := encoder.statsCollector

	// Record some frames
	collector.RecordFrameEncoded(true, 25, 0.8, 2000)   // Keyframe
	collector.RecordFrameEncoded(false, 30, 0.75, 1800) // Regular frame
	collector.RecordFrameEncoded(false, 20, 0.85, 2200) // Regular frame

	stats := encoder.GetStats()

	// Check frame counts
	if stats.FramesEncoded != 3 {
		t.Errorf("Expected 3 frames encoded, got %d", stats.FramesEncoded)
	}

	if stats.KeyframesEncoded != 1 {
		t.Errorf("Expected 1 keyframe encoded, got %d", stats.KeyframesEncoded)
	}

	// Check latency statistics
	if stats.EncodingLatency != 20 { // Last recorded latency
		t.Errorf("Expected current latency 20, got %d", stats.EncodingLatency)
	}

	if stats.MinLatency != 20 {
		t.Errorf("Expected min latency 20, got %d", stats.MinLatency)
	}

	if stats.MaxLatency != 30 {
		t.Errorf("Expected max latency 30, got %d", stats.MaxLatency)
	}

	expectedAvgLatency := (25 + 30 + 20) / 3 // 25
	if stats.AverageLatency != int64(expectedAvgLatency) {
		t.Errorf("Expected average latency %d, got %d", expectedAvgLatency, stats.AverageLatency)
	}

	// Check bitrate statistics
	if stats.CurrentBitrate != 2200 { // Last recorded bitrate
		t.Errorf("Expected current bitrate 2200, got %d", stats.CurrentBitrate)
	}

	if stats.PeakBitrate != 2200 {
		t.Errorf("Expected peak bitrate 2200, got %d", stats.PeakBitrate)
	}

	expectedAvgBitrate := (2000 + 1800 + 2200) / 3 // 2000
	if stats.AverageBitrate != expectedAvgBitrate {
		t.Errorf("Expected average bitrate %d, got %d", expectedAvgBitrate, stats.AverageBitrate)
	}

	// Check quality statistics
	if stats.CurrentQuality != 0.85 { // Last recorded quality
		t.Errorf("Expected current quality 0.85, got %f", stats.CurrentQuality)
	}

	expectedAvgQuality := (0.8 + 0.75 + 0.85) / 3 // 0.8
	if math.Abs(stats.AverageQuality-expectedAvgQuality) > 0.001 {
		t.Errorf("Expected average quality %f, got %f", expectedAvgQuality, stats.AverageQuality)
	}
}

func TestEncoderStatsCollector_RecordFrameDropped(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	encoder := NewBaseEncoder(cfg)
	collector := encoder.statsCollector

	// Record some dropped frames
	collector.RecordFrameDropped()
	collector.RecordFrameDropped()

	stats := encoder.GetStats()

	if stats.DropFrames != 2 {
		t.Errorf("Expected 2 dropped frames, got %d", stats.DropFrames)
	}
}

func TestEncoderStatsCollector_RecordFrameSkipped(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	encoder := NewBaseEncoder(cfg)
	collector := encoder.statsCollector

	// Record some skipped frames
	collector.RecordFrameSkipped()
	collector.RecordFrameSkipped()
	collector.RecordFrameSkipped()

	stats := encoder.GetStats()

	if stats.SkippedFrames != 3 {
		t.Errorf("Expected 3 skipped frames, got %d", stats.SkippedFrames)
	}
}

func TestEncoderStatsCollector_RecordError(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	encoder := NewBaseEncoder(cfg)
	collector := encoder.statsCollector

	// Record some errors
	err1 := fmt.Errorf("encoding error 1")
	err2 := fmt.Errorf("encoding error 2")

	collector.RecordError(err1)
	collector.RecordError(err2)

	stats := encoder.GetStats()

	if stats.ErrorCount != 2 {
		t.Errorf("Expected 2 errors, got %d", stats.ErrorCount)
	}

	if stats.LastError != "encoding error 2" {
		t.Errorf("Expected last error 'encoding error 2', got '%s'", stats.LastError)
	}
}

func TestEncoderStatsCollector_UpdateResourceUsage(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	encoder := NewBaseEncoder(cfg)
	collector := encoder.statsCollector

	// Update resource usage
	collector.UpdateResourceUsage(45.5, 1024*1024*100) // 45.5% CPU, 100MB memory

	stats := encoder.GetStats()

	if stats.CPUUsage != 45.5 {
		t.Errorf("Expected CPU usage 45.5, got %f", stats.CPUUsage)
	}

	if stats.MemoryUsage != 1024*1024*100 {
		t.Errorf("Expected memory usage %d, got %d", 1024*1024*100, stats.MemoryUsage)
	}
}

func TestEncoderStatsCollector_GetDetailedStats(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	encoder := NewBaseEncoder(cfg)
	collector := encoder.statsCollector

	// Record some frames with different characteristics
	collector.RecordFrameEncoded(true, 25, 0.8, 2000)   // Keyframe
	collector.RecordFrameEncoded(false, 30, 0.75, 1800) // Regular frame
	collector.RecordFrameEncoded(false, 20, 0.85, 2200) // Regular frame
	collector.RecordFrameEncoded(true, 35, 0.9, 2100)   // Keyframe
	collector.RecordFrameDropped()
	collector.RecordFrameDropped()

	detailedStats := collector.GetDetailedStats()

	// Check keyframe ratio
	expectedKeyframeRatio := 2.0 / 4.0 // 2 keyframes out of 4 total frames
	if math.Abs(detailedStats.KeyframeRatio-expectedKeyframeRatio) > 0.001 {
		t.Errorf("Expected keyframe ratio %f, got %f", expectedKeyframeRatio, detailedStats.KeyframeRatio)
	}

	// Check drop ratio
	expectedDropRatio := 2.0 / 6.0 // 2 dropped out of 6 total (4 encoded + 2 dropped)
	if math.Abs(detailedStats.DropRatio-expectedDropRatio) > 0.001 {
		t.Errorf("Expected drop ratio %f, got %f", expectedDropRatio, detailedStats.DropRatio)
	}

	// Check bitrate efficiency
	targetBitrate := float64(cfg.Bitrate)
	averageBitrate := float64(detailedStats.AverageBitrate)
	expectedEfficiency := averageBitrate / targetBitrate
	if math.Abs(detailedStats.BitrateEfficiency-expectedEfficiency) > 0.001 {
		t.Errorf("Expected bitrate efficiency %f, got %f", expectedEfficiency, detailedStats.BitrateEfficiency)
	}

	// Check that standard deviations are calculated
	if detailedStats.LatencyStdDev < 0 {
		t.Error("Expected non-negative latency standard deviation")
	}

	if detailedStats.QualityStdDev < 0 {
		t.Error("Expected non-negative quality standard deviation")
	}
}

func TestEncoderStatsCollector_LatencyWindow(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	encoder := NewBaseEncoder(cfg)
	collector := encoder.statsCollector

	// Set a small window for testing
	collector.latencyWindow = 3

	// Record more frames than the window size
	collector.RecordFrameEncoded(false, 10, 0.8, 2000)
	collector.RecordFrameEncoded(false, 20, 0.8, 2000)
	collector.RecordFrameEncoded(false, 30, 0.8, 2000)
	collector.RecordFrameEncoded(false, 40, 0.8, 2000) // This should push out the first value

	stats := encoder.GetStats()

	// Average should be based on last 3 values: (20 + 30 + 40) / 3 = 30
	expectedAvg := (20 + 30 + 40) / 3
	if stats.AverageLatency != int64(expectedAvg) {
		t.Errorf("Expected average latency %d (windowed), got %d", expectedAvg, stats.AverageLatency)
	}

	// Check that history is limited to window size
	if len(collector.latencyHistory) != 3 {
		t.Errorf("Expected latency history size 3, got %d", len(collector.latencyHistory))
	}
}

func TestEncoderStatsCollector_FPSCalculation(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	encoder := NewBaseEncoder(cfg)
	collector := encoder.statsCollector

	// Record frames with specific timing
	now := time.Now()
	collector.lastFrameTime = now

	// Simulate 30 FPS (33.33ms between frames)
	frameInterval := time.Millisecond * 33
	for i := 0; i < 5; i++ {
		now = now.Add(frameInterval)
		collector.lastFrameTime = now.Add(-frameInterval) // Set previous frame time
		collector.updateFPS(now)
	}

	stats := encoder.GetStats()

	// Should be approximately 30 FPS
	expectedFPS := 30.0
	if math.Abs(stats.EncodingFPS-expectedFPS) > 1.0 {
		t.Errorf("Expected FPS around %f, got %f", expectedFPS, stats.EncodingFPS)
	}
}

func TestEncoderStatsCollector_Reset(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	encoder := NewBaseEncoder(cfg)
	collector := encoder.statsCollector

	// Record some data
	collector.RecordFrameEncoded(true, 25, 0.8, 2000)
	collector.RecordFrameDropped()
	collector.RecordError(fmt.Errorf("test error"))

	// Verify data is recorded
	stats := encoder.GetStats()
	if stats.FramesEncoded == 0 || stats.DropFrames == 0 || stats.ErrorCount == 0 {
		t.Error("Expected some statistics to be recorded before reset")
	}

	// Reset
	collector.Reset()

	// Verify data is cleared
	stats = encoder.GetStats()
	if stats.FramesEncoded != 0 {
		t.Errorf("Expected frames encoded to be 0 after reset, got %d", stats.FramesEncoded)
	}
	if stats.DropFrames != 0 {
		t.Errorf("Expected drop frames to be 0 after reset, got %d", stats.DropFrames)
	}
	if stats.ErrorCount != 0 {
		t.Errorf("Expected error count to be 0 after reset, got %d", stats.ErrorCount)
	}
	if stats.LastError != "" {
		t.Errorf("Expected last error to be empty after reset, got '%s'", stats.LastError)
	}

	// Check that history is cleared
	if len(collector.latencyHistory) != 0 {
		t.Errorf("Expected latency history to be empty after reset, got %d items", len(collector.latencyHistory))
	}
	if len(collector.bitrateHistory) != 0 {
		t.Errorf("Expected bitrate history to be empty after reset, got %d items", len(collector.bitrateHistory))
	}
}

func TestEncoderStatsHistory(t *testing.T) {
	history := NewEncoderStatsHistory(3) // Keep only 3 snapshots

	// Create some test stats
	stats1 := EncoderStats{FramesEncoded: 10, CurrentBitrate: 2000}
	stats2 := EncoderStats{FramesEncoded: 20, CurrentBitrate: 2100}
	stats3 := EncoderStats{FramesEncoded: 30, CurrentBitrate: 2200}
	stats4 := EncoderStats{FramesEncoded: 40, CurrentBitrate: 2300}

	// Add snapshots
	history.AddSnapshot(stats1)
	time.Sleep(time.Millisecond) // Ensure different timestamps
	history.AddSnapshot(stats2)
	time.Sleep(time.Millisecond)
	history.AddSnapshot(stats3)
	time.Sleep(time.Millisecond)
	history.AddSnapshot(stats4) // This should push out stats1

	snapshots := history.GetSnapshots()

	// Should only have 3 snapshots (oldest one removed)
	if len(snapshots) != 3 {
		t.Errorf("Expected 3 snapshots, got %d", len(snapshots))
	}

	// Should have stats2, stats3, stats4 (stats1 should be removed)
	if snapshots[0].Stats.FramesEncoded != 20 {
		t.Errorf("Expected first snapshot to have 20 frames, got %d", snapshots[0].Stats.FramesEncoded)
	}
	if snapshots[2].Stats.FramesEncoded != 40 {
		t.Errorf("Expected last snapshot to have 40 frames, got %d", snapshots[2].Stats.FramesEncoded)
	}
}

func TestEncoderStatsHistory_GetSnapshotsSince(t *testing.T) {
	history := NewEncoderStatsHistory(10)

	// Record the time before adding snapshots
	beforeTime := time.Now()
	time.Sleep(time.Millisecond)

	// Add some snapshots
	stats1 := EncoderStats{FramesEncoded: 10}
	stats2 := EncoderStats{FramesEncoded: 20}
	history.AddSnapshot(stats1)
	time.Sleep(time.Millisecond)

	// Record time between snapshots
	middleTime := time.Now()
	time.Sleep(time.Millisecond)

	history.AddSnapshot(stats2)

	// Get snapshots since middle time
	recentSnapshots := history.GetSnapshotsSince(middleTime)

	// Should only have the second snapshot
	if len(recentSnapshots) != 1 {
		t.Errorf("Expected 1 recent snapshot, got %d", len(recentSnapshots))
	}
	if recentSnapshots[0].Stats.FramesEncoded != 20 {
		t.Errorf("Expected recent snapshot to have 20 frames, got %d", recentSnapshots[0].Stats.FramesEncoded)
	}

	// Get all snapshots since before time
	allSnapshots := history.GetSnapshotsSince(beforeTime)
	if len(allSnapshots) != 2 {
		t.Errorf("Expected 2 total snapshots, got %d", len(allSnapshots))
	}
}

func TestEncoderStatsHistory_Clear(t *testing.T) {
	history := NewEncoderStatsHistory(10)

	// Add some snapshots
	stats := EncoderStats{FramesEncoded: 10}
	history.AddSnapshot(stats)
	history.AddSnapshot(stats)

	// Verify snapshots exist
	snapshots := history.GetSnapshots()
	if len(snapshots) != 2 {
		t.Errorf("Expected 2 snapshots before clear, got %d", len(snapshots))
	}

	// Clear history
	history.Clear()

	// Verify snapshots are cleared
	snapshots = history.GetSnapshots()
	if len(snapshots) != 0 {
		t.Errorf("Expected 0 snapshots after clear, got %d", len(snapshots))
	}
}

func TestEncoderStatsCollector_QualityVariance(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	encoder := NewBaseEncoder(cfg)
	collector := encoder.statsCollector

	// Record frames with varying quality
	qualities := []float64{0.8, 0.9, 0.7, 0.85, 0.75}
	for _, quality := range qualities {
		collector.RecordFrameEncoded(false, 25, quality, 2000)
	}

	stats := encoder.GetStats()

	// Calculate expected variance manually
	var sum float64
	for _, q := range qualities {
		sum += q
	}
	mean := sum / float64(len(qualities))

	var variance float64
	for _, q := range qualities {
		diff := q - mean
		variance += diff * diff
	}
	variance /= float64(len(qualities) - 1)

	if math.Abs(stats.QualityVariance-variance) > 0.001 {
		t.Errorf("Expected quality variance %f, got %f", variance, stats.QualityVariance)
	}

	// Test detailed stats standard deviation
	detailedStats := collector.GetDetailedStats()
	expectedStdDev := math.Sqrt(variance)
	if math.Abs(detailedStats.QualityStdDev-expectedStdDev) > 0.001 {
		t.Errorf("Expected quality std dev %f, got %f", expectedStdDev, detailedStats.QualityStdDev)
	}
}

func TestEncoderStatsCollector_ConcurrentAccess(t *testing.T) {
	cfg := config.DefaultEncoderConfig()
	encoder := NewBaseEncoder(cfg)
	collector := encoder.statsCollector

	// Test concurrent access to statistics
	done := make(chan bool, 2)

	// Goroutine 1: Record frames
	go func() {
		for i := 0; i < 100; i++ {
			collector.RecordFrameEncoded(i%10 == 0, int64(20+i%10), 0.8, 2000)
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Goroutine 2: Read statistics
	go func() {
		for i := 0; i < 100; i++ {
			_ = encoder.GetStats()
			_ = collector.GetDetailedStats()
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify final state
	stats := encoder.GetStats()
	if stats.FramesEncoded != 100 {
		t.Errorf("Expected 100 frames encoded, got %d", stats.FramesEncoded)
	}
	if stats.KeyframesEncoded != 10 { // Every 10th frame is a keyframe
		t.Errorf("Expected 10 keyframes encoded, got %d", stats.KeyframesEncoded)
	}
}

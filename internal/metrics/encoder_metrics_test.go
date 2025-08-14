package metrics

import (
	"testing"
	"time"
)

func TestNewEncoderMetrics(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false // Don't start server for this test

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	if em == nil {
		t.Fatal("Encoder metrics is nil")
	}
}

func TestEncoderMetrics_RecordFrameEncoded(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test recording frame encoding
	encoderType := "nvenc"
	codec := "h264"
	hardwareAccel := "true"

	em.RecordFrameEncoded(encoderType, codec, hardwareAccel, false)
	em.RecordFrameEncoded(encoderType, codec, hardwareAccel, true) // keyframe
	em.RecordFrameEncoded(encoderType, codec, hardwareAccel, false)
}

func TestEncoderMetrics_RecordFrameDropped(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test recording dropped frames
	encoderType := "x264"
	codec := "h264"
	hardwareAccel := "false"

	em.RecordFrameDropped(encoderType, codec, hardwareAccel)
	em.RecordFrameDropped(encoderType, codec, hardwareAccel)
}

func TestEncoderMetrics_RecordFrameSkipped(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test recording skipped frames
	encoderType := "vaapi"
	codec := "h264"
	hardwareAccel := "true"

	em.RecordFrameSkipped(encoderType, codec, hardwareAccel)
}

func TestEncoderMetrics_UpdateEncodingFPS(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test updating encoding FPS
	encoderType := "nvenc"
	codec := "h264"
	hardwareAccel := "true"
	fps := 29.97

	em.UpdateEncodingFPS(encoderType, codec, hardwareAccel, fps)
}

func TestEncoderMetrics_RecordEncodingLatency(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test recording encoding latency
	encoderType := "nvenc"
	codec := "h264"
	hardwareAccel := "true"

	em.RecordEncodingLatency(encoderType, codec, hardwareAccel, 15.5)
	em.RecordEncodingLatency(encoderType, codec, hardwareAccel, 22.3)
	em.RecordEncodingLatency(encoderType, codec, hardwareAccel, 8.7)
}

func TestEncoderMetrics_UpdateLatencyStats(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test updating latency stats
	encoderType := "x264"
	codec := "h264"
	hardwareAccel := "false"
	minLatency := 5.2
	maxLatency := 45.8
	avgLatency := 18.3

	em.UpdateLatencyStats(encoderType, codec, hardwareAccel, minLatency, maxLatency, avgLatency)
}

func TestEncoderMetrics_UpdateBitrateStats(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test updating bitrate stats
	encoderType := "nvenc"
	codec := "h264"
	hardwareAccel := "true"
	current := 2500
	average := 2400
	target := 2000
	peak := 3000
	efficiency := 1.2

	em.UpdateBitrateStats(encoderType, codec, hardwareAccel, current, average, target, peak, efficiency)
}

func TestEncoderMetrics_UpdateQualityStats(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test updating quality stats
	encoderType := "vaapi"
	codec := "h264"
	hardwareAccel := "true"
	current := 0.85
	average := 0.82
	variance := 0.05

	em.UpdateQualityStats(encoderType, codec, hardwareAccel, current, average, variance)
}

func TestEncoderMetrics_UpdateRatios(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test updating ratios
	encoderType := "x264"
	codec := "h264"
	hardwareAccel := "false"
	keyframeRatio := 0.033 // ~1 keyframe per 30 frames
	dropRatio := 0.001     // 0.1% drop rate

	em.UpdateRatios(encoderType, codec, hardwareAccel, keyframeRatio, dropRatio)
}

func TestEncoderMetrics_UpdateResourceUsage(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test updating resource usage
	encoderType := "nvenc"
	codec := "h264"
	hardwareAccel := "true"
	cpuUsage := 15.5
	memoryUsage := int64(256 * 1024 * 1024) // 256MB

	em.UpdateResourceUsage(encoderType, codec, hardwareAccel, cpuUsage, memoryUsage)
}

func TestEncoderMetrics_SetEncoderActive(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test setting encoder active
	encoderType := "vaapi"
	codec := "h264"
	hardwareAccel := "true"

	em.SetEncoderActive(encoderType, codec, hardwareAccel, true)
	em.SetEncoderActive(encoderType, codec, hardwareAccel, false)
}

func TestEncoderMetrics_UpdateUptime(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test updating uptime
	encoderType := "x264"
	codec := "h264"
	hardwareAccel := "false"
	startTime := time.Now().Add(-5 * time.Minute)

	em.UpdateUptime(encoderType, codec, hardwareAccel, startTime)
}

func TestEncoderMetrics_RecordError(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test recording errors
	encoderType := "nvenc"
	codec := "h264"
	hardwareAccel := "true"

	em.RecordError(encoderType, codec, hardwareAccel, "encoding_timeout")
	em.RecordError(encoderType, codec, hardwareAccel, "hardware_error")
}

func TestEncoderMetrics_RecordRestart(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test recording restarts
	encoderType := "vaapi"
	codec := "h264"
	hardwareAccel := "true"

	em.RecordRestart(encoderType, codec, hardwareAccel)
}

func TestEncoderStatsCollector(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Create stats collector
	encoderType := "nvenc"
	codec := "h264"
	hardwareAccel := true
	collector := NewEncoderStatsCollector(em, encoderType, codec, hardwareAccel)

	if collector == nil {
		t.Fatal("Stats collector is nil")
	}

	// Test collector methods
	collector.Start()
	collector.RecordFrameEncoded(false)
	collector.RecordFrameEncoded(true) // keyframe
	collector.RecordFrameDropped()
	collector.RecordFrameSkipped()
	collector.UpdateEncodingFPS(30.0)
	collector.RecordEncodingLatency(15.2)
	collector.UpdateLatencyStats(5.0, 50.0, 18.5)
	collector.UpdateBitrateStats(2500, 2400, 2000, 3000, 1.2)
	collector.UpdateQualityStats(0.85, 0.82, 0.05)
	collector.UpdateRatios(0.033, 0.001)
	collector.UpdateResourceUsage(15.5, 256*1024*1024)
	collector.RecordError("test_error")
	collector.RecordRestart()
	collector.Stop()
}

func TestEncoderStatsCollector_Concurrent(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Create stats collector
	encoderType := "x264"
	codec := "h264"
	hardwareAccel := false
	collector := NewEncoderStatsCollector(em, encoderType, codec, hardwareAccel)

	collector.Start()

	// Test concurrent access
	done := make(chan bool, 4)

	// Goroutine 1: Record frames
	go func() {
		for i := 0; i < 100; i++ {
			collector.RecordFrameEncoded(i%30 == 0) // keyframe every 30 frames
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 2: Record latency
	go func() {
		for i := 0; i < 100; i++ {
			collector.RecordEncodingLatency(float64(10 + i%20))
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 3: Update FPS and bitrate
	go func() {
		for i := 0; i < 100; i++ {
			collector.UpdateEncodingFPS(float64(25 + i%10))
			collector.UpdateBitrateStats(2000+i%500, 2000, 2000, 2500, 1.0)
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 4: Update quality and resource usage
	go func() {
		for i := 0; i < 100; i++ {
			collector.UpdateQualityStats(0.8+float64(i%20)/100, 0.82, 0.05)
			collector.UpdateResourceUsage(float64(10+i%20), int64(200*1024*1024+i*1024*1024))
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 4; i++ {
		<-done
	}

	collector.Stop()
}

func TestEncoderStatsCollector_HardwareAcceleration(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	em, err := NewEncoderMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create encoder metrics: %v", err)
	}

	// Test hardware accelerated encoder
	hwCollector := NewEncoderStatsCollector(em, "nvenc", "h264", true)
	if hwCollector.hardwareAccel != "true" {
		t.Errorf("Expected hardware acceleration to be 'true', got '%s'", hwCollector.hardwareAccel)
	}

	// Test software encoder
	swCollector := NewEncoderStatsCollector(em, "x264", "h264", false)
	if swCollector.hardwareAccel != "false" {
		t.Errorf("Expected hardware acceleration to be 'false', got '%s'", swCollector.hardwareAccel)
	}
}

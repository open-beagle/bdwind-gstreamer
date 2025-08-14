package metrics

import (
	"testing"
	"time"
)

func TestNewDesktopCaptureMetrics(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false // Don't start server for this test

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	dcm, err := NewDesktopCaptureMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create desktop capture metrics: %v", err)
	}

	if dcm == nil {
		t.Fatal("Desktop capture metrics is nil")
	}
}

func TestDesktopCaptureMetrics_RecordFrameCapture(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	dcm, err := NewDesktopCaptureMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create desktop capture metrics: %v", err)
	}

	// Test recording frame capture
	display := ":0"
	sourceType := "x11"

	dcm.RecordFrameCapture(display, sourceType)
	dcm.RecordFrameCapture(display, sourceType)
	dcm.RecordFrameCapture(display, sourceType)

	// No direct way to verify counter values without exposing internal state
	// In a real scenario, you would check the Prometheus metrics endpoint
}

func TestDesktopCaptureMetrics_RecordFrameDropped(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	dcm, err := NewDesktopCaptureMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create desktop capture metrics: %v", err)
	}

	// Test recording dropped frames
	display := ":0"
	sourceType := "x11"

	dcm.RecordFrameDropped(display, sourceType)
	dcm.RecordFrameDropped(display, sourceType)
}

func TestDesktopCaptureMetrics_UpdateFPS(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	dcm, err := NewDesktopCaptureMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create desktop capture metrics: %v", err)
	}

	// Test updating FPS
	display := ":0"
	sourceType := "x11"
	currentFPS := 30.5
	averageFPS := 29.8

	dcm.UpdateFPS(display, sourceType, currentFPS, averageFPS)
}

func TestDesktopCaptureMetrics_RecordLatency(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	dcm, err := NewDesktopCaptureMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create desktop capture metrics: %v", err)
	}

	// Test recording latency
	display := ":0"
	sourceType := "x11"

	dcm.RecordLatency(display, sourceType, 15.5)
	dcm.RecordLatency(display, sourceType, 22.3)
	dcm.RecordLatency(display, sourceType, 8.7)
}

func TestDesktopCaptureMetrics_UpdateLatencyStats(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	dcm, err := NewDesktopCaptureMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create desktop capture metrics: %v", err)
	}

	// Test updating latency stats
	display := ":0"
	sourceType := "x11"
	minLatency := 5.2
	maxLatency := 45.8
	avgLatency := 18.3

	dcm.UpdateLatencyStats(display, sourceType, minLatency, maxLatency, avgLatency)
}

func TestDesktopCaptureMetrics_SetCaptureActive(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	dcm, err := NewDesktopCaptureMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create desktop capture metrics: %v", err)
	}

	// Test setting capture active
	display := ":0"
	sourceType := "x11"

	dcm.SetCaptureActive(display, sourceType, true)
	dcm.SetCaptureActive(display, sourceType, false)
}

func TestDesktopCaptureMetrics_UpdateUptime(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	dcm, err := NewDesktopCaptureMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create desktop capture metrics: %v", err)
	}

	// Test updating uptime
	display := ":0"
	sourceType := "x11"
	startTime := time.Now().Add(-5 * time.Minute)

	dcm.UpdateUptime(display, sourceType, startTime)
}

func TestDesktopCaptureMetrics_RecordError(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	dcm, err := NewDesktopCaptureMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create desktop capture metrics: %v", err)
	}

	// Test recording errors
	display := ":0"
	sourceType := "x11"

	dcm.RecordError(display, sourceType, "pipeline_error")
	dcm.RecordError(display, sourceType, "capture_timeout")
}

func TestDesktopCaptureMetrics_RecordRestart(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	dcm, err := NewDesktopCaptureMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create desktop capture metrics: %v", err)
	}

	// Test recording restarts
	display := ":0"
	sourceType := "x11"

	dcm.RecordRestart(display, sourceType)
}

func TestCaptureStatsCollector(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	dcm, err := NewDesktopCaptureMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create desktop capture metrics: %v", err)
	}

	// Create stats collector
	display := ":0"
	sourceType := "x11"
	collector := NewCaptureStatsCollector(dcm, display, sourceType)

	if collector == nil {
		t.Fatal("Stats collector is nil")
	}

	// Test collector methods
	collector.Start()
	collector.RecordFrame()
	collector.RecordDroppedFrame()
	collector.UpdateFPS(30.0, 29.5)
	collector.RecordLatency(15.2)
	collector.UpdateLatencyStats(5.0, 50.0, 18.5)
	collector.RecordError("test_error")
	collector.RecordRestart()
	collector.Stop()
}

func TestCaptureStatsCollector_Concurrent(t *testing.T) {
	config := DefaultMetricsConfig()
	config.Enabled = false

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	dcm, err := NewDesktopCaptureMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create desktop capture metrics: %v", err)
	}

	// Create stats collector
	display := ":0"
	sourceType := "x11"
	collector := NewCaptureStatsCollector(dcm, display, sourceType)

	collector.Start()

	// Test concurrent access
	done := make(chan bool, 3)

	// Goroutine 1: Record frames
	go func() {
		for i := 0; i < 100; i++ {
			collector.RecordFrame()
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 2: Record latency
	go func() {
		for i := 0; i < 100; i++ {
			collector.RecordLatency(float64(10 + i%20))
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 3: Update FPS
	go func() {
		for i := 0; i < 100; i++ {
			collector.UpdateFPS(float64(25+i%10), float64(28+i%5))
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		<-done
	}

	collector.Stop()
}

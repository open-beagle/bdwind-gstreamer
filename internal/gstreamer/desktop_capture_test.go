package gstreamer

import (
	"strings"
	"testing"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestNewDesktopCapture(t *testing.T) {
	// Test with valid configuration
	validConfig := config.DefaultDesktopCaptureConfig()
	capture, err := NewDesktopCapture(validConfig)
	if err != nil {
		t.Errorf("Expected no error with valid config, got: %v", err)
	}
	if capture == nil {
		t.Error("Expected capture instance, got nil")
	}

	// Test with invalid configuration
	invalidConfig := config.DesktopCaptureConfig{
		FrameRate:  0, // Invalid frame rate
		Width:      1920,
		Height:     1080,
		Quality:    "medium",
		BufferSize: 10,
	}
	capture, err = NewDesktopCapture(invalidConfig)
	if err == nil {
		t.Error("Expected error with invalid config, got none")
	}
	if capture != nil {
		t.Error("Expected nil capture instance with invalid config")
	}
}

func TestNewDesktopCaptureFromEnv(t *testing.T) {
	// This test will use default environment values
	capture, err := NewDesktopCaptureFromEnv()
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if capture == nil {
		t.Error("Expected capture instance, got nil")
	}
}

func TestDesktopCaptureGetSource(t *testing.T) {
	// Test X11 source
	x11Config := config.DefaultDesktopCaptureConfig()
	x11Config.UseWayland = false
	x11Config.ShowPointer = true
	x11Config.UseDamage = true
	x11Config.CaptureRegion = &config.Rect{X: 100, Y: 200, Width: 800, Height: 600}

	capture, err := NewDesktopCapture(x11Config)
	if err != nil {
		t.Fatalf("Failed to create desktop capture: %v", err)
	}

	sourceType, props := capture.GetSource()
	if sourceType != "ximagesrc" {
		t.Errorf("Expected source type 'ximagesrc', got %s", sourceType)
	}

	if props["display"] != x11Config.DisplayID {
		t.Errorf("Expected display %s, got %v", x11Config.DisplayID, props["display"])
	}
	if props["show-pointer"] != true {
		t.Errorf("Expected show-pointer to be true, got %v", props["show-pointer"])
	}
	if props["use-damage"] != true {
		t.Errorf("Expected use-damage to be true, got %v", props["use-damage"])
	}
	if props["startx"] != 100 {
		t.Errorf("Expected startx to be 100, got %v", props["startx"])
	}
	if props["starty"] != 200 {
		t.Errorf("Expected starty to be 200, got %v", props["starty"])
	}
	if props["endx"] != 899 { // startx + width - 1
		t.Errorf("Expected endx to be 899, got %v", props["endx"])
	}
	if props["endy"] != 799 { // starty + height - 1
		t.Errorf("Expected endy to be 799, got %v", props["endy"])
	}

	// Test Wayland source
	waylandConfig := config.DefaultDesktopCaptureConfig()
	waylandConfig.UseWayland = true

	capture, err = NewDesktopCapture(waylandConfig)
	if err != nil {
		t.Fatalf("Failed to create desktop capture: %v", err)
	}

	sourceType, props = capture.GetSource()
	if sourceType != "waylandsrc" {
		t.Errorf("Expected source type 'waylandsrc', got %s", sourceType)
	}
}

func TestDesktopCaptureGetStats(t *testing.T) {
	config := config.DefaultDesktopCaptureConfig()
	capture, err := NewDesktopCapture(config)
	if err != nil {
		t.Fatalf("Failed to create desktop capture: %v", err)
	}

	stats := capture.GetStats()
	if stats.FramesCapture != 0 {
		t.Errorf("Expected FramesCapture to be 0, got %d", stats.FramesCapture)
	}
	if stats.CurrentFPS != 0 {
		t.Errorf("Expected CurrentFPS to be 0, got %f", stats.CurrentFPS)
	}
	if stats.AverageFPS != 0 {
		t.Errorf("Expected AverageFPS to be 0, got %f", stats.AverageFPS)
	}
	if stats.FramesDropped != 0 {
		t.Errorf("Expected FramesDropped to be 0, got %d", stats.FramesDropped)
	}
	if stats.CaptureLatency != 0 {
		t.Errorf("Expected CaptureLatency to be 0, got %d", stats.CaptureLatency)
	}
}

func TestDesktopCaptureStatsCollection(t *testing.T) {
	cfg := config.DefaultDesktopCaptureConfig()
	capture, err := NewDesktopCapture(cfg)
	if err != nil {
		t.Fatalf("Failed to create desktop capture: %v", err)
	}

	dc := capture.(*desktopCapture)

	// Start capture to initialize statistics
	err = dc.Start()
	if err != nil {
		// Start may fail without actual GStreamer, but we can still test stats
		t.Logf("Start failed (expected without GStreamer): %v", err)
	}

	// Simulate some frame captures
	dc.simulateFrameCapture(10) // 10ms latency
	dc.simulateFrameCapture(15) // 15ms latency
	dc.simulateFrameCapture(12) // 12ms latency

	// Simulate some dropped frames
	dc.simulateDroppedFrame()
	dc.simulateDroppedFrame()

	// Get statistics
	stats := dc.GetStats()

	// Check frame counts
	if stats.FramesCapture != 3 {
		t.Errorf("Expected FramesCapture to be 3, got %d", stats.FramesCapture)
	}
	if stats.FramesDropped != 2 {
		t.Errorf("Expected FramesDropped to be 2, got %d", stats.FramesDropped)
	}

	// Check latency statistics
	if stats.MinLatency != 10 {
		t.Errorf("Expected MinLatency to be 10, got %d", stats.MinLatency)
	}
	if stats.MaxLatency != 15 {
		t.Errorf("Expected MaxLatency to be 15, got %d", stats.MaxLatency)
	}
	if stats.AvgLatency != 12 { // (10+15+12)/3 = 12.33, truncated to 12
		t.Errorf("Expected AvgLatency to be around 12, got %d", stats.AvgLatency)
	}

	// Check that timestamps are set
	if stats.StartTime == 0 {
		t.Error("Expected StartTime to be set")
	}
	if stats.LastFrameTime == 0 {
		t.Error("Expected LastFrameTime to be set")
	}
}

func TestDesktopCaptureUpdateStats(t *testing.T) {
	cfg := config.DefaultDesktopCaptureConfig()
	capture, err := NewDesktopCapture(cfg)
	if err != nil {
		t.Fatalf("Failed to create desktop capture: %v", err)
	}

	dc := capture.(*desktopCapture)

	// Start capture to initialize statistics
	err = dc.Start()
	if err != nil {
		// Start may fail without actual GStreamer, but we can still test stats
		t.Logf("Start failed (expected without GStreamer): %v", err)
	}

	// Update stats as if from GStreamer
	dc.UpdateStats(100, 5, 20) // 100 frames captured, 5 dropped, 20ms avg latency

	stats := dc.GetStats()

	if stats.FramesCapture != 100 {
		t.Errorf("Expected FramesCapture to be 100, got %d", stats.FramesCapture)
	}
	if stats.FramesDropped != 5 {
		t.Errorf("Expected FramesDropped to be 5, got %d", stats.FramesDropped)
	}

	// Update again with more frames
	dc.UpdateStats(150, 8, 18) // 50 more frames, 3 more dropped

	stats = dc.GetStats()

	if stats.FramesCapture != 150 {
		t.Errorf("Expected FramesCapture to be 150, got %d", stats.FramesCapture)
	}
	if stats.FramesDropped != 8 {
		t.Errorf("Expected FramesDropped to be 8, got %d", stats.FramesDropped)
	}
}

func TestStatsCollectorFrameRate(t *testing.T) {
	collector := newStatsCollector()

	// Record frames with small delays to simulate real capture
	for i := 0; i < 10; i++ {
		collector.recordFrame()
		time.Sleep(10 * time.Millisecond) // 10ms between frames = ~100 FPS theoretical
	}

	stats := collector.getStats()

	// Should have recorded 10 frames
	if stats.FramesCapture != 10 {
		t.Errorf("Expected FramesCapture to be 10, got %d", stats.FramesCapture)
	}

	// Current FPS should be calculated based on sliding window
	if stats.CurrentFPS <= 0 {
		t.Errorf("Expected positive CurrentFPS, got %f", stats.CurrentFPS)
	}

	// Average FPS should be calculated based on total time
	if stats.AverageFPS <= 0 {
		t.Errorf("Expected positive AverageFPS, got %f", stats.AverageFPS)
	}
}

func TestStatsCollectorLatency(t *testing.T) {
	collector := newStatsCollector()

	// Record various latencies
	latencies := []int64{10, 15, 20, 12, 18, 25, 8, 30, 14, 16}
	for _, latency := range latencies {
		collector.recordLatency(latency)
	}

	stats := collector.getStats()

	// Check min/max latency
	if stats.MinLatency != 8 {
		t.Errorf("Expected MinLatency to be 8, got %d", stats.MinLatency)
	}
	if stats.MaxLatency != 30 {
		t.Errorf("Expected MaxLatency to be 30, got %d", stats.MaxLatency)
	}

	// Check average latency (sum = 168, count = 10, avg = 16.8)
	expectedAvg := int64(168 / 10) // 16
	if stats.AvgLatency != expectedAvg {
		t.Errorf("Expected AvgLatency to be %d, got %d", expectedAvg, stats.AvgLatency)
	}
}

func TestStatsCollectorDroppedFrames(t *testing.T) {
	collector := newStatsCollector()

	// Record some frames and dropped frames
	collector.recordFrame()
	collector.recordFrame()
	collector.recordDroppedFrame()
	collector.recordFrame()
	collector.recordDroppedFrame()
	collector.recordDroppedFrame()

	stats := collector.getStats()

	if stats.FramesCapture != 3 {
		t.Errorf("Expected FramesCapture to be 3, got %d", stats.FramesCapture)
	}
	if stats.FramesDropped != 3 {
		t.Errorf("Expected FramesDropped to be 3, got %d", stats.FramesDropped)
	}
}

func TestStatsCollectorReset(t *testing.T) {
	collector := newStatsCollector()

	// Record some data
	collector.recordFrame()
	collector.recordDroppedFrame()
	collector.recordLatency(20)

	// Verify data is recorded
	stats := collector.getStats()
	if stats.FramesCapture == 0 || stats.FramesDropped == 0 {
		t.Error("Expected some data before reset")
	}

	// Reset collector
	collector.reset()

	// Verify data is cleared
	stats = collector.getStats()
	if stats.FramesCapture != 0 {
		t.Errorf("Expected FramesCapture to be 0 after reset, got %d", stats.FramesCapture)
	}
	if stats.FramesDropped != 0 {
		t.Errorf("Expected FramesDropped to be 0 after reset, got %d", stats.FramesDropped)
	}
	if stats.CurrentFPS != 0 {
		t.Errorf("Expected CurrentFPS to be 0 after reset, got %f", stats.CurrentFPS)
	}
	if stats.AverageFPS != 0 {
		t.Errorf("Expected AverageFPS to be 0 after reset, got %f", stats.AverageFPS)
	}
}

func TestStatsCollectorConcurrency(t *testing.T) {
	collector := newStatsCollector()

	// Test concurrent access to statistics collector
	done := make(chan bool)

	// Goroutine 1: Record frames
	go func() {
		for i := 0; i < 100; i++ {
			collector.recordFrame()
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 2: Record dropped frames
	go func() {
		for i := 0; i < 50; i++ {
			collector.recordDroppedFrame()
			time.Sleep(2 * time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 3: Record latencies
	go func() {
		for i := 0; i < 75; i++ {
			collector.recordLatency(int64(10 + i%20))
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 4: Read stats
	go func() {
		for i := 0; i < 20; i++ {
			_ = collector.getStats()
			time.Sleep(5 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 4; i++ {
		<-done
	}

	// Verify final stats
	stats := collector.getStats()
	if stats.FramesCapture != 100 {
		t.Errorf("Expected FramesCapture to be 100, got %d", stats.FramesCapture)
	}
	if stats.FramesDropped != 50 {
		t.Errorf("Expected FramesDropped to be 50, got %d", stats.FramesDropped)
	}
}

func TestGetX11Displays(t *testing.T) {
	displays, err := GetX11Displays()
	if err != nil {
		t.Fatalf("Failed to get X11 displays: %v", err)
	}

	if len(displays) == 0 {
		t.Error("Expected at least one display, got none")
	}

	// Check first display has valid properties
	display := displays[0]
	if display.DisplayName == "" {
		t.Error("Expected display name to be non-empty")
	}
	if display.Width <= 0 {
		t.Errorf("Expected positive width, got %d", display.Width)
	}
	if display.Height <= 0 {
		t.Errorf("Expected positive height, got %d", display.Height)
	}
	if display.RefreshRate <= 0 {
		t.Errorf("Expected positive refresh rate, got %f", display.RefreshRate)
	}
}

func TestValidateX11Display(t *testing.T) {
	// Test with default display
	err := ValidateX11Display(":0")
	if err != nil {
		t.Errorf("Expected no error for default display, got: %v", err)
	}

	// Test with invalid display
	err = ValidateX11Display(":999")
	if err == nil {
		t.Error("Expected error for invalid display, got none")
	}
}

func TestGetOptimalX11CaptureSettings(t *testing.T) {
	cfg, err := GetOptimalX11CaptureSettings(":0")
	if err != nil {
		t.Fatalf("Failed to get optimal settings: %v", err)
	}

	if cfg.UseWayland {
		t.Error("Expected UseWayland to be false for X11")
	}
	if cfg.DisplayID != ":0" {
		t.Errorf("Expected DisplayID to be ':0', got %s", cfg.DisplayID)
	}
	if cfg.Width <= 0 {
		t.Errorf("Expected positive width, got %d", cfg.Width)
	}
	if cfg.Height <= 0 {
		t.Errorf("Expected positive height, got %d", cfg.Height)
	}
	if cfg.FrameRate <= 0 {
		t.Errorf("Expected positive frame rate, got %d", cfg.FrameRate)
	}
}

func TestCreateX11CaptureRegion(t *testing.T) {
	region := CreateX11CaptureRegion(100, 200, 800, 600)
	if region == nil {
		t.Fatal("Expected region to be non-nil")
	}

	if region.X != 100 {
		t.Errorf("Expected X to be 100, got %d", region.X)
	}
	if region.Y != 200 {
		t.Errorf("Expected Y to be 200, got %d", region.Y)
	}
	if region.Width != 800 {
		t.Errorf("Expected Width to be 800, got %d", region.Width)
	}
	if region.Height != 600 {
		t.Errorf("Expected Height to be 600, got %d", region.Height)
	}
}

func TestCreateX11WindowCaptureRegion(t *testing.T) {
	region, err := CreateX11WindowCaptureRegion("test-window")
	if err != nil {
		t.Fatalf("Failed to create window capture region: %v", err)
	}

	if region == nil {
		t.Fatal("Expected region to be non-nil")
	}

	if region.Width <= 0 {
		t.Errorf("Expected positive width, got %d", region.Width)
	}
	if region.Height <= 0 {
		t.Errorf("Expected positive height, got %d", region.Height)
	}
}

func TestX11CaptureConfiguration(t *testing.T) {
	// Test X11 capture with region
	cfg := config.DefaultDesktopCaptureConfig()
	cfg.UseWayland = false
	cfg.DisplayID = ":0"
	cfg.CaptureRegion = CreateX11CaptureRegion(100, 100, 800, 600)
	cfg.ShowPointer = true
	cfg.UseDamage = true

	capture, err := NewDesktopCapture(cfg)
	if err != nil {
		t.Fatalf("Failed to create X11 capture: %v", err)
	}

	sourceType, props := capture.GetSource()
	if sourceType != "ximagesrc" {
		t.Errorf("Expected source type 'ximagesrc', got %s", sourceType)
	}

	// Verify X11-specific properties
	if props["display"] != ":0" {
		t.Errorf("Expected display ':0', got %v", props["display"])
	}
	if props["show-pointer"] != true {
		t.Errorf("Expected show-pointer to be true, got %v", props["show-pointer"])
	}
	if props["use-damage"] != true {
		t.Errorf("Expected use-damage to be true, got %v", props["use-damage"])
	}

	// Verify region properties
	if props["startx"] != 100 {
		t.Errorf("Expected startx to be 100, got %v", props["startx"])
	}
	if props["starty"] != 100 {
		t.Errorf("Expected starty to be 100, got %v", props["starty"])
	}
	if props["endx"] != 899 { // startx + width - 1
		t.Errorf("Expected endx to be 899, got %v", props["endx"])
	}
	if props["endy"] != 699 { // starty + height - 1
		t.Errorf("Expected endy to be 699, got %v", props["endy"])
	}
}

func TestX11MultiMonitorSupport(t *testing.T) {
	// Test with screen number in display ID
	cfg := config.DefaultDesktopCaptureConfig()
	cfg.UseWayland = false
	cfg.DisplayID = ":0.1" // Screen 1

	capture, err := NewDesktopCapture(cfg)
	if err != nil {
		t.Fatalf("Failed to create X11 capture with screen number: %v", err)
	}

	sourceType, props := capture.GetSource()
	if sourceType != "ximagesrc" {
		t.Errorf("Expected source type 'ximagesrc', got %s", sourceType)
	}

	if props["display"] != ":0.1" {
		t.Errorf("Expected display ':0.1', got %v", props["display"])
	}
}

// Wayland-specific tests

func TestGetWaylandDisplays(t *testing.T) {
	displays, err := GetWaylandDisplays()
	if err != nil {
		t.Fatalf("Failed to get Wayland displays: %v", err)
	}

	if len(displays) == 0 {
		t.Error("Expected at least one display, got none")
	}

	// Check first display has valid properties
	display := displays[0]
	if display.DisplayName == "" {
		t.Error("Expected display name to be non-empty")
	}
	if display.OutputName == "" {
		t.Error("Expected output name to be non-empty")
	}
	if display.Width <= 0 {
		t.Errorf("Expected positive width, got %d", display.Width)
	}
	if display.Height <= 0 {
		t.Errorf("Expected positive height, got %d", display.Height)
	}
	if display.RefreshRate <= 0 {
		t.Errorf("Expected positive refresh rate, got %f", display.RefreshRate)
	}
	if display.Scale <= 0 {
		t.Errorf("Expected positive scale, got %f", display.Scale)
	}
}

func TestValidateWaylandDisplay(t *testing.T) {
	// Test with default display
	err := ValidateWaylandDisplay("wayland-0")
	if err != nil {
		t.Errorf("Expected no error for default display, got: %v", err)
	}

	// Test with invalid display
	err = ValidateWaylandDisplay("wayland-999")
	if err == nil {
		t.Error("Expected error for invalid display, got none")
	}
}

func TestGetOptimalWaylandCaptureSettings(t *testing.T) {
	cfg, err := GetOptimalWaylandCaptureSettings("wayland-0")
	if err != nil {
		t.Fatalf("Failed to get optimal settings: %v", err)
	}

	if !cfg.UseWayland {
		t.Error("Expected UseWayland to be true for Wayland")
	}
	if cfg.DisplayID != "wayland-0" {
		t.Errorf("Expected DisplayID to be 'wayland-0', got %s", cfg.DisplayID)
	}
	if cfg.Width <= 0 {
		t.Errorf("Expected positive width, got %d", cfg.Width)
	}
	if cfg.Height <= 0 {
		t.Errorf("Expected positive height, got %d", cfg.Height)
	}
	if cfg.FrameRate <= 0 {
		t.Errorf("Expected positive frame rate, got %d", cfg.FrameRate)
	}
}

func TestCreateWaylandCaptureRegion(t *testing.T) {
	region := CreateWaylandCaptureRegion(100, 200, 800, 600)
	if region == nil {
		t.Fatal("Expected region to be non-nil")
	}

	if region.X != 100 {
		t.Errorf("Expected X to be 100, got %d", region.X)
	}
	if region.Y != 200 {
		t.Errorf("Expected Y to be 200, got %d", region.Y)
	}
	if region.Width != 800 {
		t.Errorf("Expected Width to be 800, got %d", region.Width)
	}
	if region.Height != 600 {
		t.Errorf("Expected Height to be 600, got %d", region.Height)
	}
}

func TestCreateWaylandWindowCaptureRegion(t *testing.T) {
	region, err := CreateWaylandWindowCaptureRegion("test-window")
	if err != nil {
		t.Fatalf("Failed to create window capture region: %v", err)
	}

	if region == nil {
		t.Fatal("Expected region to be non-nil")
	}

	if region.Width <= 0 {
		t.Errorf("Expected positive width, got %d", region.Width)
	}
	if region.Height <= 0 {
		t.Errorf("Expected positive height, got %d", region.Height)
	}
}

func TestWaylandCaptureConfiguration(t *testing.T) {
	// Test Wayland capture with region
	cfg := config.DefaultDesktopCaptureConfig()
	cfg.UseWayland = true
	cfg.DisplayID = "wayland-0"
	cfg.CaptureRegion = CreateWaylandCaptureRegion(100, 100, 800, 600)
	cfg.ShowPointer = true

	capture, err := NewDesktopCapture(cfg)
	if err != nil {
		t.Fatalf("Failed to create Wayland capture: %v", err)
	}

	sourceType, props := capture.GetSource()
	if sourceType != "waylandsrc" {
		t.Errorf("Expected source type 'waylandsrc', got %s", sourceType)
	}

	// Verify Wayland-specific properties
	if props["display"] != "wayland-0" {
		t.Errorf("Expected display 'wayland-0', got %v", props["display"])
	}
	if props["show-pointer"] != true {
		t.Errorf("Expected show-pointer to be true, got %v", props["show-pointer"])
	}

	// Verify region properties (Wayland uses crop- prefix)
	if props["crop-x"] != 100 {
		t.Errorf("Expected crop-x to be 100, got %v", props["crop-x"])
	}
	if props["crop-y"] != 100 {
		t.Errorf("Expected crop-y to be 100, got %v", props["crop-y"])
	}
	if props["crop-width"] != 800 {
		t.Errorf("Expected crop-width to be 800, got %v", props["crop-width"])
	}
	if props["crop-height"] != 600 {
		t.Errorf("Expected crop-height to be 600, got %v", props["crop-height"])
	}
}

func TestWaylandMultiMonitorSupport(t *testing.T) {
	// Test with output number in display ID
	cfg := config.DefaultDesktopCaptureConfig()
	cfg.UseWayland = true
	cfg.DisplayID = "wayland-1" // Output 1

	capture, err := NewDesktopCapture(cfg)
	if err != nil {
		t.Fatalf("Failed to create Wayland capture with output number: %v", err)
	}

	sourceType, props := capture.GetSource()
	if sourceType != "waylandsrc" {
		t.Errorf("Expected source type 'waylandsrc', got %s", sourceType)
	}

	if props["display"] != "wayland-1" {
		t.Errorf("Expected display 'wayland-1', got %v", props["display"])
	}

	// Check if output property is set (may not be supported by all compositors)
	if output, exists := props["output"]; exists && output != "1" {
		t.Errorf("Expected output to be '1', got %v", output)
	}
}

func TestDetectWaylandCompositor(t *testing.T) {
	compositor, err := DetectWaylandCompositor()
	if err != nil {
		t.Fatalf("Failed to detect Wayland compositor: %v", err)
	}

	// Should return a string (even if "unknown")
	if compositor == "" {
		t.Error("Expected non-empty compositor name")
	}
}

func TestGetWaylandCompositorCapabilities(t *testing.T) {
	capabilities, err := GetWaylandCompositorCapabilities()
	if err != nil {
		t.Fatalf("Failed to get Wayland compositor capabilities: %v", err)
	}

	// Should return a map with at least some capabilities
	if len(capabilities) == 0 {
		t.Error("Expected at least some capabilities")
	}

	// Check for required capabilities
	requiredCapabilities := []string{
		"screen-capture",
		"pointer-capture",
	}

	for _, cap := range requiredCapabilities {
		if _, exists := capabilities[cap]; !exists {
			t.Errorf("Expected capability '%s' to be present", cap)
		}
	}
}

func TestWaylandCapsString(t *testing.T) {
	cfg := config.DefaultDesktopCaptureConfig()
	cfg.UseWayland = true
	cfg.Quality = "high"
	cfg.Width = 1920
	cfg.Height = 1080
	cfg.FrameRate = 60

	capture, err := NewDesktopCapture(cfg)
	if err != nil {
		t.Fatalf("Failed to create Wayland capture: %v", err)
	}

	dc := capture.(*desktopCapture)
	capsStr, err := dc.buildWaylandCapsString()
	if err != nil {
		t.Fatalf("Failed to build Wayland caps string: %v", err)
	}

	// Check that caps string contains expected elements
	expectedElements := []string{
		"video/x-raw",
		"format=BGRA", // High quality format
		"width=1920",
		"height=1080",
		"framerate=60/1",
		"pixel-aspect-ratio=1/1",
	}

	for _, element := range expectedElements {
		if !strings.Contains(capsStr, element) {
			t.Errorf("Expected caps string to contain '%s', got: %s", element, capsStr)
		}
	}
}

func TestWaylandDisplayDetection(t *testing.T) {
	// Test that Wayland display detection works
	displays, err := GetWaylandDisplays()
	if err != nil {
		t.Fatalf("Failed to get Wayland displays: %v", err)
	}

	// Should have at least one display
	if len(displays) == 0 {
		t.Error("Expected at least one Wayland display")
	}

	// First display should have reasonable defaults
	display := displays[0]
	if display.DisplayName == "" {
		t.Error("Expected display name to be set")
	}
	if display.OutputName == "" {
		t.Error("Expected output name to be set")
	}
	if display.Width <= 0 || display.Height <= 0 {
		t.Errorf("Expected positive dimensions, got %dx%d", display.Width, display.Height)
	}
	if display.RefreshRate <= 0 {
		t.Errorf("Expected positive refresh rate, got %f", display.RefreshRate)
	}
	if display.Scale <= 0 {
		t.Errorf("Expected positive scale, got %f", display.Scale)
	}
}

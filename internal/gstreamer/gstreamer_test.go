package gstreamer

import (
	"testing"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

func TestVersion(t *testing.T) {
	version := Version()
	if version == "" {
		t.Error("Expected non-empty GStreamer version")
	}
	t.Logf("GStreamer version: %s", version)
}

func TestNewPipeline(t *testing.T) {
	pipeline := NewPipeline("test-pipeline")
	if pipeline == nil {
		t.Error("Expected non-nil pipeline")
	}

	err := pipeline.Create()
	if err != nil {
		t.Errorf("Failed to create pipeline: %v", err)
	}

	err = pipeline.Stop()
	if err != nil {
		t.Errorf("Failed to stop pipeline: %v", err)
	}
}

func TestPipelineFromStringGstreamer(t *testing.T) {
	pipeline := NewPipeline("test-pipeline")
	if pipeline == nil {
		t.Error("Expected non-nil pipeline")
	}

	// Create a simple videotestsrc pipeline
	err := pipeline.CreateFromString("videotestsrc ! fakesink")
	if err != nil {
		t.Errorf("Failed to create pipeline from string: %v", err)
	}

	// Set to playing state
	err = pipeline.SetState(StatePlaying)
	if err != nil {
		t.Errorf("Failed to set pipeline to playing state: %v", err)
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Get current state
	state, err := pipeline.GetState()
	if err != nil {
		t.Errorf("Failed to get pipeline state: %v", err)
	}

	if state != StatePlaying {
		t.Errorf("Expected pipeline state to be playing, got %v", state)
	}

	// Stop the pipeline
	err = pipeline.Stop()
	if err != nil {
		t.Errorf("Failed to stop pipeline: %v", err)
	}
}

func TestElementLinking(t *testing.T) {
	pipeline := NewPipeline("test-pipeline")
	if pipeline == nil {
		t.Error("Expected non-nil pipeline")
	}

	err := pipeline.Create()
	if err != nil {
		t.Errorf("Failed to create pipeline: %v", err)
	}

	// Create elements
	src, err := pipeline.GetElement("videotestsrc")
	if err != nil {
		t.Errorf("Failed to create videotestsrc element: %v", err)
	}

	sink, err := pipeline.GetElement("fakesink")
	if err != nil {
		t.Errorf("Failed to create fakesink element: %v", err)
	}

	// Link elements
	err = src.Link(sink)
	if err != nil {
		t.Errorf("Failed to link elements: %v", err)
	}

	// Clean up
	err = pipeline.Stop()
	if err != nil {
		t.Errorf("Failed to stop pipeline: %v", err)
	}
}

func TestCaps(t *testing.T) {
	// Create caps from string
	capsStr := "video/x-raw,width=1280,height=720,framerate=30/1"
	caps, err := NewCapsFromString(capsStr)
	if err != nil {
		t.Errorf("Failed to create caps from string: %v", err)
	}

	// Convert caps back to string
	resultStr := caps.ToString()
	t.Logf("Caps string: %s", resultStr)

	// Check if caps are any or empty
	if caps.IsAny() {
		t.Error("Expected caps not to be any")
	}

	if caps.IsEmpty() {
		t.Error("Expected caps not to be empty")
	}
}

func TestPluginAvailability(t *testing.T) {
	// Check if core plugin is available
	if !IsPluginAvailable("coreelements") {
		t.Error("Expected coreelements plugin to be available")
	}

	// Check if videotestsrc element is available
	if !IsElementFactoryAvailable("videotestsrc") {
		t.Error("Expected videotestsrc element to be available")
	}
}

func TestDesktopCaptureIntegration(t *testing.T) {
	// Skip this test if running in CI environment
	if testing.Short() {
		t.Skip("Skipping desktop capture test in short mode")
	}

	cfg := config.DesktopCaptureConfig{
		FrameRate:  30,
		Width:      1280,
		Height:     720,
		Quality:    "medium",
		BufferSize: 10,
	}

	capture, err := NewDesktopCapture(cfg)
	if err != nil {
		t.Errorf("Failed to create desktop capture: %v", err)
	}
	if capture == nil {
		t.Error("Expected non-nil desktop capture")
	}

	sourceType, props := capture.GetSource()
	if sourceType == "" {
		t.Error("Expected non-empty source type")
	}

	t.Logf("Source type: %s", sourceType)
	t.Logf("Source properties: %v", props)
}

func TestDetectDisplayServerIntegration(t *testing.T) {
	isWayland, err := config.DetectDisplayServer()
	if err != nil {
		t.Errorf("Failed to detect display server: %v", err)
	}

	t.Logf("Using Wayland: %v", isWayland)
}

func TestGetAvailableDisplaysIntegration(t *testing.T) {
	displays, err := config.GetAvailableDisplays()
	if err != nil {
		t.Errorf("Failed to get available displays: %v", err)
	}

	if len(displays) == 0 {
		t.Error("Expected at least one display")
	}

	t.Logf("Available displays: %v", displays)
}

func TestGetOptimalFrameRateIntegration(t *testing.T) {
	framerate, err := config.GetOptimalFrameRate()
	if err != nil {
		t.Errorf("Failed to get optimal frame rate: %v", err)
	}

	if framerate <= 0 {
		t.Errorf("Expected positive frame rate, got %d", framerate)
	}

	t.Logf("Optimal frame rate: %d", framerate)
}

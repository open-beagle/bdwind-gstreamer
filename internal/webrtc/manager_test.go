package webrtc

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// TestManagerLoggingEnhancement tests the enhanced logging functionality
func TestManagerLoggingEnhancement(t *testing.T) {
	// Create a test hook to capture log entries
	hook := test.NewGlobal()
	logrus.SetLevel(logrus.TraceLevel) // Set to trace level to capture all logs

	// Create a test config
	cfg := &config.Config{
		GStreamer: &config.GStreamerConfig{
			Encoding: config.EncoderConfig{
				Codec: config.CodecH264,
			},
			Capture: config.DesktopCaptureConfig{
				Width:     1920,
				Height:    1080,
				FrameRate: 30,
			},
		},
		WebRTC: &config.WebRTCConfig{
			ICEServers: []config.ICEServerConfig{
				{
					URLs: []string{"stun:stun.l.google.com:19302"},
				},
			},
		},
	}

	// Create manager config
	managerConfig := &ManagerConfig{
		Config: cfg,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	// Create manager
	manager, err := NewManager(ctx, managerConfig)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Verify that logger entry was created
	if manager.logger == nil {
		t.Error("Expected logger entry to be initialized")
	}

	// Check that initialization logs were captured
	entries := hook.AllEntries()
	if len(entries) == 0 {
		t.Error("Expected log entries during manager initialization")
	}

	// Verify log levels are used correctly
	hasTrace := false
	hasDebug := false
	hasInfo := false

	for _, entry := range entries {
		switch entry.Level {
		case logrus.TraceLevel:
			hasTrace = true
		case logrus.DebugLevel:
			hasDebug = true
		case logrus.InfoLevel:
			hasInfo = true
		}

		// Verify component field is set
		if component, ok := entry.Data["component"]; !ok || component != "webrtc-manager" {
			t.Errorf("Expected component field to be 'webrtc-manager', got: %v", component)
		}
	}

	// Verify different log levels were used during initialization
	if !hasTrace {
		t.Error("Expected trace level logs during initialization")
	}
	if !hasDebug {
		t.Error("Expected debug level logs during initialization")
	}

	// Use hasInfo to avoid unused variable error
	_ = hasInfo

	// Test start method logging
	hook.Reset()

	// Start the manager (this will likely fail due to missing dependencies, but we want to test logging)
	err = manager.Start(ctx)

	// Check that start logs were captured regardless of success/failure
	startEntries := hook.AllEntries()
	if len(startEntries) == 0 {
		t.Error("Expected log entries during manager start")
	}

	// Verify Info level log for start attempt
	hasStartInfo := false
	for _, entry := range startEntries {
		if entry.Level == logrus.InfoLevel && entry.Message == "Starting WebRTC manager..." {
			hasStartInfo = true
			break
		}
	}
	if !hasStartInfo {
		t.Error("Expected Info level log for WebRTC manager start")
	}

	// Clean up
	_ = manager.Stop(ctx)
}

// TestManagerComponentStatusLogging tests component status logging
func TestManagerComponentStatusLogging(t *testing.T) {
	// Create a test hook to capture log entries
	hook := test.NewGlobal()
	logrus.SetLevel(logrus.DebugLevel)

	// Create a minimal config
	cfg := &config.Config{
		GStreamer: &config.GStreamerConfig{
			Encoding: config.EncoderConfig{
				Codec: config.CodecH264,
			},
			Capture: config.DesktopCaptureConfig{
				Width:     1920,
				Height:    1080,
				FrameRate: 30,
			},
		},
		WebRTC: &config.WebRTCConfig{},
	}

	managerConfig := &ManagerConfig{
		Config: cfg,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	manager, err := NewManager(ctx, managerConfig)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Test component status
	status := manager.GetComponentStatus()

	// Verify expected components exist
	expectedComponents := []string{"manager", "signaling", "media_stream", "bridge", "sdp_generator"}
	for _, component := range expectedComponents {
		if _, exists := status[component]; !exists {
			t.Errorf("Expected component '%s' in status", component)
		}
	}

	// Verify MediaStream was created and logged
	entries := hook.AllEntries()
	hasMediaStreamLog := false
	for _, entry := range entries {
		if entry.Level == logrus.DebugLevel &&
			entry.Message != "" &&
			(entry.Message == "MediaStream created with 1 total tracks, 1 active tracks" ||
				entry.Message == "MediaStream created with 0 total tracks, 0 active tracks") {
			hasMediaStreamLog = true
			break
		}
	}
	if !hasMediaStreamLog {
		t.Error("Expected debug log about MediaStream creation with track count")
	}
}

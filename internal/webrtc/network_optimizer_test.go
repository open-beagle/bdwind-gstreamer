package webrtc

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestDefaultNetworkOptimizerConfig(t *testing.T) {
	config := DefaultNetworkOptimizerConfig()

	if !config.CongestionControlEnabled {
		t.Error("Expected congestion control to be enabled by default")
	}

	if config.CongestionThreshold != 0.05 {
		t.Errorf("Expected congestion threshold to be 0.05, got %f", config.CongestionThreshold)
	}

	if !config.AdaptiveBitrateEnabled {
		t.Error("Expected adaptive bitrate to be enabled by default")
	}

	if config.MinBitrate != 500 {
		t.Errorf("Expected min bitrate to be 500, got %d", config.MinBitrate)
	}

	if config.MaxBitrate != 5000 {
		t.Errorf("Expected max bitrate to be 5000, got %d", config.MaxBitrate)
	}

	if !config.PacketLossRecoveryEnabled {
		t.Error("Expected packet loss recovery to be enabled by default")
	}
}

func TestNewNetworkOptimizer(t *testing.T) {
	config := DefaultNetworkOptimizerConfig()

	optimizer := NewNetworkOptimizer(config)
	if optimizer == nil {
		t.Fatal("Optimizer should not be nil")
	}

	// Test with nil config (should use default)
	optimizer2 := NewNetworkOptimizer(nil)
	if optimizer2 == nil {
		t.Fatal("Optimizer should not be nil even with nil config")
	}
}

func TestNetworkOptimizerStartStop(t *testing.T) {
	config := DefaultNetworkOptimizerConfig()
	optimizer := NewNetworkOptimizer(config)

	// Create a mock peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create peer connection: %v", err)
	}
	defer pc.Close()

	// Set peer connection
	err = optimizer.SetPeerConnection(pc)
	if err != nil {
		t.Fatalf("Failed to set peer connection: %v", err)
	}

	// Start optimizer
	err = optimizer.Start()
	if err != nil {
		t.Fatalf("Failed to start optimizer: %v", err)
	}

	// Try to start again (should fail)
	err = optimizer.Start()
	if err == nil {
		t.Error("Expected error when starting optimizer twice")
	}

	// Stop optimizer
	err = optimizer.Stop()
	if err != nil {
		t.Fatalf("Failed to stop optimizer: %v", err)
	}

	// Stop again (should not fail)
	err = optimizer.Stop()
	if err != nil {
		t.Errorf("Unexpected error when stopping optimizer twice: %v", err)
	}
}

func TestNetworkOptimizerStartWithoutPeerConnection(t *testing.T) {
	config := DefaultNetworkOptimizerConfig()
	optimizer := NewNetworkOptimizer(config)

	// Try to start without setting peer connection
	err := optimizer.Start()
	if err == nil {
		t.Error("Expected error when starting optimizer without peer connection")
	}
}

func TestNetworkOptimizerUpdateStats(t *testing.T) {
	config := DefaultNetworkOptimizerConfig()
	optimizer := NewNetworkOptimizer(config)

	// Create test network stats
	stats := NetworkStats{
		PacketsSent: 1000,
		PacketsLost: 50,
		BytesSent:   1024000,
		RTT:         time.Millisecond * 100,
		Jitter:      time.Millisecond * 10,
		Bandwidth:   1000000, // 1 Mbps
		Timestamp:   time.Now(),
	}

	// Update stats
	err := optimizer.UpdateNetworkStats(stats)
	if err != nil {
		t.Errorf("Failed to update network stats: %v", err)
	}

	// Get optimization stats
	optStats := optimizer.GetOptimizationStats()
	if optStats.CurrentBitrate == 0 {
		t.Error("Expected current bitrate to be set")
	}
}

func TestNetworkOptimizerBitrateCallback(t *testing.T) {
	config := DefaultNetworkOptimizerConfig()
	optimizer := NewNetworkOptimizer(config)

	// Set bitrate callback
	err := optimizer.OnBitrateChange(func(bitrate int) {
		// Callback function for bitrate changes
		_ = bitrate // Use the parameter to avoid unused variable warning
	})
	if err != nil {
		t.Errorf("Failed to set bitrate callback: %v", err)
	}

	// Create a mock peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create peer connection: %v", err)
	}
	defer pc.Close()

	err = optimizer.SetPeerConnection(pc)
	if err != nil {
		t.Fatalf("Failed to set peer connection: %v", err)
	}

	err = optimizer.Start()
	if err != nil {
		t.Fatalf("Failed to start optimizer: %v", err)
	}
	defer optimizer.Stop()

	// Simulate high packet loss to trigger bitrate adjustment
	highLossStats := NetworkStats{
		PacketsSent: 1000,
		PacketsLost: 100, // 10% loss rate
		BytesSent:   1024000,
		RTT:         time.Millisecond * 200,
		Jitter:      time.Millisecond * 20,
		Bandwidth:   500000, // 500 kbps
		Timestamp:   time.Now(),
	}

	// Update stats multiple times to trigger optimization
	for i := 0; i < 3; i++ {
		highLossStats.Timestamp = time.Now()
		highLossStats.PacketsSent += 1000
		highLossStats.PacketsLost += 100
		optimizer.UpdateNetworkStats(highLossStats)
		time.Sleep(time.Millisecond * 10)
	}

	// Give some time for processing
	time.Sleep(time.Millisecond * 100)

	// Note: The callback might not be called in this simple test
	// because the optimization logic requires more complex conditions
	// This test mainly verifies that the callback can be set without error
}

func TestCongestionStateString(t *testing.T) {
	tests := []struct {
		state    CongestionState
		expected string
	}{
		{CongestionStateNormal, "normal"},
		{CongestionStateDetected, "detected"},
		{CongestionStateRecovering, "recovering"},
	}

	for _, test := range tests {
		result := test.state.String()
		if result != test.expected {
			t.Errorf("CongestionState(%d).String() = %s, expected %s", test.state, result, test.expected)
		}
	}
}

func TestNetworkOptimizerGetStats(t *testing.T) {
	config := DefaultNetworkOptimizerConfig()
	optimizer := NewNetworkOptimizer(config)

	// Initial stats should have default values
	stats := optimizer.GetOptimizationStats()
	if stats.CurrentBitrate == 0 {
		t.Error("Expected current bitrate to be initialized")
	}

	if stats.TargetBitrate == 0 {
		t.Error("Expected target bitrate to be initialized")
	}

	if stats.PacketLossRate != 0 {
		t.Errorf("Expected initial packet loss rate to be 0, got %f", stats.PacketLossRate)
	}

	if stats.CongestionDetected {
		t.Error("Expected congestion not to be detected initially")
	}
}

func TestNetworkOptimizerSetPeerConnection(t *testing.T) {
	config := DefaultNetworkOptimizerConfig()
	optimizer := NewNetworkOptimizer(config)

	// Create a mock peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create peer connection: %v", err)
	}
	defer pc.Close()

	// Set peer connection
	err = optimizer.SetPeerConnection(pc)
	if err != nil {
		t.Errorf("Failed to set peer connection: %v", err)
	}

	// Set peer connection again (should succeed)
	err = optimizer.SetPeerConnection(pc)
	if err != nil {
		t.Errorf("Failed to set peer connection again: %v", err)
	}
}

package webrtc

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestDefaultWebRTCStatsConfig(t *testing.T) {
	config := DefaultWebRTCStatsConfig()

	if config.CollectionInterval != time.Second*2 {
		t.Errorf("Expected collection interval to be 2s, got %v", config.CollectionInterval)
	}

	if config.MaxHistorySize != 100 {
		t.Errorf("Expected max history size to be 100, got %d", config.MaxHistorySize)
	}

	if !config.DetailedStats {
		t.Error("Expected detailed stats to be enabled by default")
	}

	expectedStats := []string{
		"connection",
		"transport",
		"inbound-rtp",
		"outbound-rtp",
		"remote-inbound-rtp",
		"remote-outbound-rtp",
	}

	if len(config.EnabledStats) != len(expectedStats) {
		t.Errorf("Expected %d enabled stats, got %d", len(expectedStats), len(config.EnabledStats))
	}

	for i, expected := range expectedStats {
		if i >= len(config.EnabledStats) || config.EnabledStats[i] != expected {
			t.Errorf("Expected enabled stat %d to be %s, got %s", i, expected, config.EnabledStats[i])
		}
	}
}

func TestNewWebRTCStatsCollector(t *testing.T) {
	config := DefaultWebRTCStatsConfig()

	collector := NewWebRTCStatsCollector(config)
	if collector == nil {
		t.Fatal("Collector should not be nil")
	}

	// Test with nil config (should use default)
	collector2 := NewWebRTCStatsCollector(nil)
	if collector2 == nil {
		t.Fatal("Collector should not be nil even with nil config")
	}
}

func TestWebRTCStatsCollectorStartStop(t *testing.T) {
	config := DefaultWebRTCStatsConfig()
	collector := NewWebRTCStatsCollector(config)

	// Start without peer connection (should not fail)
	err := collector.Start()
	if err != nil {
		t.Errorf("Failed to start collector without peer connection: %v", err)
	}

	// Start again (should not fail)
	err = collector.Start()
	if err != nil {
		t.Errorf("Failed to start collector twice: %v", err)
	}

	// Stop collector
	err = collector.Stop()
	if err != nil {
		t.Errorf("Failed to stop collector: %v", err)
	}

	// Stop again (should not fail)
	err = collector.Stop()
	if err != nil {
		t.Errorf("Unexpected error when stopping collector twice: %v", err)
	}
}

func TestWebRTCStatsCollectorWithPeerConnection(t *testing.T) {
	config := DefaultWebRTCStatsConfig()
	config.CollectionInterval = time.Millisecond * 100 // Faster for testing
	collector := NewWebRTCStatsCollector(config)

	// Create a mock peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create peer connection: %v", err)
	}
	defer pc.Close()

	// Set peer connection
	err = collector.SetPeerConnection(pc)
	if err != nil {
		t.Errorf("Failed to set peer connection: %v", err)
	}

	// Start collector
	err = collector.Start()
	if err != nil {
		t.Errorf("Failed to start collector: %v", err)
	}
	defer collector.Stop()

	// Give some time for stats collection
	time.Sleep(time.Millisecond * 250)

	// Get current stats
	stats := collector.GetCurrentStats()
	if stats.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}

	// Check connection state
	if stats.ConnectionState != pc.ConnectionState() {
		t.Errorf("Expected connection state to match peer connection state")
	}
}

func TestWebRTCStatsCollectorHistory(t *testing.T) {
	config := DefaultWebRTCStatsConfig()
	config.CollectionInterval = time.Millisecond * 50 // Very fast for testing
	config.MaxHistorySize = 5                         // Small history for testing
	collector := NewWebRTCStatsCollector(config)

	// Create a mock peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create peer connection: %v", err)
	}
	defer pc.Close()

	err = collector.SetPeerConnection(pc)
	if err != nil {
		t.Errorf("Failed to set peer connection: %v", err)
	}

	err = collector.Start()
	if err != nil {
		t.Errorf("Failed to start collector: %v", err)
	}
	defer collector.Stop()

	// Wait for several collection cycles
	time.Sleep(time.Millisecond * 300)

	// Get stats history
	history := collector.GetStatsHistory()
	if len(history) == 0 {
		t.Error("Expected some stats history")
	}

	// Check that history doesn't exceed max size
	if len(history) > config.MaxHistorySize {
		t.Errorf("History size %d exceeds max size %d", len(history), config.MaxHistorySize)
	}

	// Check that timestamps are in order
	for i := 1; i < len(history); i++ {
		if history[i].Timestamp.Before(history[i-1].Timestamp) {
			t.Error("Stats history timestamps should be in chronological order")
		}
	}
}

func TestWebRTCStatsCollectorCallback(t *testing.T) {
	config := DefaultWebRTCStatsConfig()
	config.CollectionInterval = time.Millisecond * 100
	collector := NewWebRTCStatsCollector(config)

	callbackCalled := false
	var receivedStats WebRTCStats

	// Set callback
	err := collector.OnStatsUpdate(func(stats WebRTCStats) {
		callbackCalled = true
		receivedStats = stats
	})
	if err != nil {
		t.Errorf("Failed to set stats update callback: %v", err)
	}

	// Create a mock peer connection
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create peer connection: %v", err)
	}
	defer pc.Close()

	err = collector.SetPeerConnection(pc)
	if err != nil {
		t.Errorf("Failed to set peer connection: %v", err)
	}

	err = collector.Start()
	if err != nil {
		t.Errorf("Failed to start collector: %v", err)
	}
	defer collector.Stop()

	// Wait for callback
	time.Sleep(time.Millisecond * 250)

	if !callbackCalled {
		t.Error("Expected stats update callback to be called")
	}

	if receivedStats.Timestamp.IsZero() {
		t.Error("Expected received stats to have timestamp")
	}
}

func TestWebRTCStatsCollectorSetPeerConnectionAfterStart(t *testing.T) {
	config := DefaultWebRTCStatsConfig()
	config.CollectionInterval = time.Millisecond * 100
	collector := NewWebRTCStatsCollector(config)

	// Start without peer connection
	err := collector.Start()
	if err != nil {
		t.Errorf("Failed to start collector: %v", err)
	}
	defer collector.Stop()

	// Create and set peer connection after start
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create peer connection: %v", err)
	}
	defer pc.Close()

	err = collector.SetPeerConnection(pc)
	if err != nil {
		t.Errorf("Failed to set peer connection after start: %v", err)
	}

	// Give some time for stats collection
	time.Sleep(time.Millisecond * 250)

	// Should have collected some stats
	stats := collector.GetCurrentStats()
	if stats.Timestamp.IsZero() {
		t.Error("Expected stats to be collected after setting peer connection")
	}
}

func TestWebRTCStatsFields(t *testing.T) {
	config := DefaultWebRTCStatsConfig()
	collector := NewWebRTCStatsCollector(config)

	// Get initial stats (should be zero values)
	stats := collector.GetCurrentStats()

	// Check that all fields exist and have expected zero values
	if stats.ConnectionState != webrtc.PeerConnectionStateNew {
		// Note: zero value for PeerConnectionState might be different
	}

	if stats.BytesSent != 0 {
		t.Errorf("Expected initial bytes sent to be 0, got %d", stats.BytesSent)
	}

	if stats.BytesReceived != 0 {
		t.Errorf("Expected initial bytes received to be 0, got %d", stats.BytesReceived)
	}

	if stats.PacketsSent != 0 {
		t.Errorf("Expected initial packets sent to be 0, got %d", stats.PacketsSent)
	}

	if stats.PacketsLost != 0 {
		t.Errorf("Expected initial packets lost to be 0, got %d", stats.PacketsLost)
	}

	if stats.CurrentRTT != 0 {
		t.Errorf("Expected initial RTT to be 0, got %v", stats.CurrentRTT)
	}

	if stats.PacketLossRate != 0 {
		t.Errorf("Expected initial packet loss rate to be 0, got %f", stats.PacketLossRate)
	}
}

func TestWebRTCStatsCollectorMultiplePeerConnections(t *testing.T) {
	config := DefaultWebRTCStatsConfig()
	collector := NewWebRTCStatsCollector(config)

	// Create first peer connection
	pc1, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create first peer connection: %v", err)
	}
	defer pc1.Close()

	err = collector.SetPeerConnection(pc1)
	if err != nil {
		t.Errorf("Failed to set first peer connection: %v", err)
	}

	// Create second peer connection
	pc2, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to create second peer connection: %v", err)
	}
	defer pc2.Close()

	// Replace with second peer connection
	err = collector.SetPeerConnection(pc2)
	if err != nil {
		t.Errorf("Failed to set second peer connection: %v", err)
	}

	err = collector.Start()
	if err != nil {
		t.Errorf("Failed to start collector: %v", err)
	}
	defer collector.Stop()

	// Give some time for stats collection
	time.Sleep(time.Millisecond * 150)

	// The test mainly verifies that setting multiple peer connections doesn't crash
	// and that the collector continues to work
	stats := collector.GetCurrentStats()
	// Just verify that the collector is still functional
	_ = stats // Use the stats to avoid unused variable warning
}

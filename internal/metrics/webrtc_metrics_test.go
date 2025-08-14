package metrics

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestNewWebRTCMetrics(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	if webrtcMetrics == nil {
		t.Fatal("WebRTC metrics should not be nil")
	}

	if webrtcMetrics.metrics != metrics {
		t.Error("WebRTC metrics should reference the provided metrics instance")
	}
}

func TestWebRTCMetrics_UpdateConnectionState(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"

	// Test different connection states
	states := []webrtc.PeerConnectionState{
		webrtc.PeerConnectionStateNew,
		webrtc.PeerConnectionStateConnecting,
		webrtc.PeerConnectionStateConnected,
		webrtc.PeerConnectionStateDisconnected,
		webrtc.PeerConnectionStateFailed,
		webrtc.PeerConnectionStateClosed,
	}

	for _, state := range states {
		webrtcMetrics.UpdateConnectionState(peerID, sessionID, state)
		// 在实际应用中，这里应该验证指标值是否正确设置
		// 由于我们使用的是接口，这里只验证方法调用不会panic
	}
}

func TestWebRTCMetrics_UpdateICEConnectionState(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"

	// Test different ICE connection states
	states := []webrtc.ICEConnectionState{
		webrtc.ICEConnectionStateNew,
		webrtc.ICEConnectionStateChecking,
		webrtc.ICEConnectionStateConnected,
		webrtc.ICEConnectionStateCompleted,
		webrtc.ICEConnectionStateFailed,
		webrtc.ICEConnectionStateDisconnected,
		webrtc.ICEConnectionStateClosed,
	}

	for _, state := range states {
		webrtcMetrics.UpdateICEConnectionState(peerID, sessionID, state)
	}
}

func TestWebRTCMetrics_UpdateSignalingState(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"

	// Test different signaling states
	states := []webrtc.SignalingState{
		webrtc.SignalingStateStable,
		webrtc.SignalingStateHaveLocalOffer,
		webrtc.SignalingStateHaveRemoteOffer,
		webrtc.SignalingStateHaveLocalPranswer,
		webrtc.SignalingStateHaveRemotePranswer,
		webrtc.SignalingStateClosed,
	}

	for _, state := range states {
		webrtcMetrics.UpdateSignalingState(peerID, sessionID, state)
	}
}

func TestWebRTCMetrics_RecordTransmissionStats(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"
	mediaType := "video"

	// Test recording transmission stats
	webrtcMetrics.RecordBytesSent(peerID, sessionID, mediaType, 1024)
	webrtcMetrics.RecordBytesReceived(peerID, sessionID, mediaType, 2048)
	webrtcMetrics.RecordPacketsSent(peerID, sessionID, mediaType, 10)
	webrtcMetrics.RecordPacketsLost(peerID, sessionID, mediaType, 1)
}

func TestWebRTCMetrics_UpdateRTTStats(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"

	current := 50 * time.Millisecond
	average := 45 * time.Millisecond
	min := 30 * time.Millisecond
	max := 80 * time.Millisecond

	webrtcMetrics.UpdateRTTStats(peerID, sessionID, current, average, min, max)
}

func TestWebRTCMetrics_UpdateJitterStats(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"
	mediaType := "video"

	current := 5 * time.Millisecond
	average := 4 * time.Millisecond

	webrtcMetrics.UpdateJitterStats(peerID, sessionID, mediaType, current, average)
}

func TestWebRTCMetrics_UpdateBitrateStats(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"
	videoBitrate := int64(2000000) // 2 Mbps
	audioBitrate := int64(128000)  // 128 kbps
	videoCodec := "H264"
	audioCodec := "OPUS"

	webrtcMetrics.UpdateBitrateStats(peerID, sessionID, videoBitrate, audioBitrate, videoCodec, audioCodec)
}

func TestWebRTCMetrics_UpdateFrameRateStats(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"
	videoFPS := 30.0
	audioFPS := 50.0 // Audio frames per second
	videoCodec := "H264"
	audioCodec := "OPUS"

	webrtcMetrics.UpdateFrameRateStats(peerID, sessionID, videoFPS, audioFPS, videoCodec, audioCodec)
}

func TestWebRTCMetrics_RecordFramesDropped(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"

	// Test video frames dropped
	webrtcMetrics.RecordFramesDropped(peerID, sessionID, "video", "H264", 5)

	// Test audio frames dropped
	webrtcMetrics.RecordFramesDropped(peerID, sessionID, "audio", "OPUS", 2)
}

func TestWebRTCMetrics_UpdatePacketLossRate(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"
	mediaType := "video"
	lossRate := 0.05 // 5% packet loss

	webrtcMetrics.UpdatePacketLossRate(peerID, sessionID, mediaType, lossRate)
}

func TestWebRTCMetrics_UpdateBandwidthStats(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"
	available := int64(5000000) // 5 Mbps available
	used := int64(2500000)      // 2.5 Mbps used

	webrtcMetrics.UpdateBandwidthStats(peerID, sessionID, available, used)
}

func TestWebRTCMetrics_UpdateCodecInfo(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"
	videoCodec := "H264"
	audioCodec := "OPUS"
	videoProfile := "baseline"
	videoLevel := "3.1"
	audioSampleRate := "48000"
	audioChannels := "2"

	webrtcMetrics.UpdateCodecInfo(peerID, sessionID, videoCodec, audioCodec, videoProfile, videoLevel, audioSampleRate, audioChannels)
}

func TestWebRTCMetrics_UpdateConnectionQuality(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"
	quality := 0.85 // 85% quality

	webrtcMetrics.UpdateConnectionQuality(peerID, sessionID, quality)
}

func TestWebRTCMetrics_UpdateConnectionUptime(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"
	startTime := time.Now().Add(-5 * time.Minute) // Started 5 minutes ago

	webrtcMetrics.UpdateConnectionUptime(peerID, sessionID, startTime)
}

func TestWebRTCMetrics_RecordEvents(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"

	// Test recording various events
	webrtcMetrics.RecordReconnect(peerID, sessionID)
	webrtcMetrics.RecordConnectionError(peerID, sessionID, "ice_failed")
	webrtcMetrics.RecordMediaError(peerID, sessionID, "video", "encoding_error")
}

func TestNewWebRTCStatsCollector(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	peerID := "peer-123"
	sessionID := "session-456"

	collector := NewWebRTCStatsCollector(webrtcMetrics, peerID, sessionID)
	if collector == nil {
		t.Fatal("WebRTC stats collector should not be nil")
	}

	if collector.peerID != peerID {
		t.Errorf("Expected peer ID %s, got %s", peerID, collector.peerID)
	}

	if collector.sessionID != sessionID {
		t.Errorf("Expected session ID %s, got %s", sessionID, collector.sessionID)
	}

	if collector.metrics != webrtcMetrics {
		t.Error("Collector should reference the provided WebRTC metrics instance")
	}
}

func TestWebRTCStatsCollector_StartStop(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	collector := NewWebRTCStatsCollector(webrtcMetrics, "peer-123", "session-456")

	// Test start and stop
	collector.Start()
	collector.Stop()
}

func TestWebRTCStatsCollector_UpdateStates(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	collector := NewWebRTCStatsCollector(webrtcMetrics, "peer-123", "session-456")

	// Test updating various states
	collector.UpdateConnectionState(webrtc.PeerConnectionStateConnected)
	collector.UpdateICEConnectionState(webrtc.ICEConnectionStateConnected)
	collector.UpdateSignalingState(webrtc.SignalingStateStable)
}

func TestWebRTCStatsCollector_RecordTransmission(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	collector := NewWebRTCStatsCollector(webrtcMetrics, "peer-123", "session-456")

	// Test recording transmission stats
	collector.RecordBytesSent("video", 1024)
	collector.RecordBytesReceived("video", 2048)
	collector.RecordPacketsSent("video", 10)
	collector.RecordPacketsLost("video", 1)
}

func TestWebRTCStatsCollector_UpdateStats(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	collector := NewWebRTCStatsCollector(webrtcMetrics, "peer-123", "session-456")

	// Test updating various stats
	collector.UpdateRTTStats(50*time.Millisecond, 45*time.Millisecond, 30*time.Millisecond, 80*time.Millisecond)
	collector.UpdateJitterStats("video", 5*time.Millisecond, 4*time.Millisecond)
	collector.UpdateBitrateStats(2000000, 128000, "H264", "OPUS")
	collector.UpdateFrameRateStats(30.0, 50.0, "H264", "OPUS")
	collector.UpdatePacketLossRate("video", 0.05)
	collector.UpdateBandwidthStats(5000000, 2500000)
	collector.UpdateCodecInfo("H264", "OPUS", "baseline", "3.1", "48000", "2")
	collector.UpdateConnectionQuality(0.85)
}

func TestWebRTCStatsCollector_RecordEvents(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	collector := NewWebRTCStatsCollector(webrtcMetrics, "peer-123", "session-456")

	// Test recording events
	collector.RecordFramesDropped("video", "H264", 5)
	collector.RecordReconnect()
	collector.RecordConnectionError("ice_failed")
	collector.RecordMediaError("video", "encoding_error")
}

func TestWebRTCStatsCollector_Concurrent(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	webrtcMetrics, err := NewWebRTCMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create WebRTC metrics: %v", err)
	}

	collector := NewWebRTCStatsCollector(webrtcMetrics, "peer-123", "session-456")

	// Test concurrent access
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			collector.UpdateConnectionState(webrtc.PeerConnectionStateConnected)
			collector.RecordBytesSent("video", int64(1024*id))
			collector.UpdateRTTStats(time.Duration(50+id)*time.Millisecond, 45*time.Millisecond, 30*time.Millisecond, 80*time.Millisecond)
			collector.RecordFramesDropped("video", "H264", int64(id))
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

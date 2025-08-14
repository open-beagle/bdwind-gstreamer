package webrtc

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
)

func TestDefaultMediaStreamConfig(t *testing.T) {
	config := DefaultMediaStreamConfig()

	if !config.VideoEnabled {
		t.Error("Expected video to be enabled by default")
	}

	if config.AudioEnabled {
		t.Error("Expected audio to be disabled by default")
	}

	if config.VideoTrackID != "video" {
		t.Errorf("Expected video track ID to be 'video', got %s", config.VideoTrackID)
	}

	if config.VideoCodec != "h264" {
		t.Errorf("Expected video codec to be 'h264', got %s", config.VideoCodec)
	}

	if config.VideoWidth != 1920 {
		t.Errorf("Expected video width to be 1920, got %d", config.VideoWidth)
	}

	if config.VideoHeight != 1080 {
		t.Errorf("Expected video height to be 1080, got %d", config.VideoHeight)
	}

	if config.VideoFrameRate != 30 {
		t.Errorf("Expected video frame rate to be 30, got %d", config.VideoFrameRate)
	}
}

func TestNewMediaStream(t *testing.T) {
	config := DefaultMediaStreamConfig()

	ms, err := NewMediaStream(config)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	if ms == nil {
		t.Fatal("Media stream is nil")
	}

	// Test video track creation
	videoTrack := ms.GetVideoTrack()
	if videoTrack == nil {
		t.Error("Video track should not be nil when video is enabled")
	}

	// Test audio track creation (should be nil since audio is disabled)
	audioTrack := ms.GetAudioTrack()
	if audioTrack != nil {
		t.Error("Audio track should be nil when audio is disabled")
	}
}

func TestNewMediaStreamWithAudio(t *testing.T) {
	config := DefaultMediaStreamConfig()
	config.AudioEnabled = true

	ms, err := NewMediaStream(config)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	// Test audio track creation
	audioTrack := ms.GetAudioTrack()
	if audioTrack == nil {
		t.Error("Audio track should not be nil when audio is enabled")
	}
}

func TestMediaStreamWriteVideo(t *testing.T) {
	config := DefaultMediaStreamConfig()

	ms, err := NewMediaStream(config)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	// Create a test video sample
	sampleData := make([]byte, 1024)
	for i := range sampleData {
		sampleData[i] = byte(i % 256)
	}

	sample := media.Sample{
		Data:      sampleData,
		Duration:  time.Millisecond * 33, // ~30fps
		Timestamp: time.Now(),
	}

	err = ms.WriteVideo(sample)
	if err != nil {
		t.Errorf("Failed to write video sample: %v", err)
	}

	// Check stats
	stats := ms.GetStats()
	if stats.VideoFramesSent != 1 {
		t.Errorf("Expected 1 video frame sent, got %d", stats.VideoFramesSent)
	}

	if stats.VideoBytesSent != int64(len(sampleData)) {
		t.Errorf("Expected %d video bytes sent, got %d", len(sampleData), stats.VideoBytesSent)
	}
}

func TestMediaStreamWriteAudio(t *testing.T) {
	config := DefaultMediaStreamConfig()
	config.AudioEnabled = true

	ms, err := NewMediaStream(config)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	// Create a test audio sample
	sampleData := make([]byte, 512)
	for i := range sampleData {
		sampleData[i] = byte(i % 256)
	}

	sample := media.Sample{
		Data:      sampleData,
		Duration:  time.Millisecond * 20,
		Timestamp: time.Now(),
	}

	err = ms.WriteAudio(sample)
	if err != nil {
		t.Errorf("Failed to write audio sample: %v", err)
	}

	// Check stats
	stats := ms.GetStats()
	if stats.AudioFramesSent != 1 {
		t.Errorf("Expected 1 audio frame sent, got %d", stats.AudioFramesSent)
	}

	if stats.AudioBytesSent != int64(len(sampleData)) {
		t.Errorf("Expected %d audio bytes sent, got %d", len(sampleData), stats.AudioBytesSent)
	}
}

func TestMediaStreamWriteVideoWithoutTrack(t *testing.T) {
	config := DefaultMediaStreamConfig()
	config.VideoEnabled = false

	ms, err := NewMediaStream(config)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	sample := media.Sample{
		Data:      []byte{1, 2, 3, 4},
		Duration:  time.Millisecond * 33,
		Timestamp: time.Now(),
	}

	err = ms.WriteVideo(sample)
	if err == nil {
		t.Error("Expected error when writing video sample without video track")
	}
}

func TestMediaStreamWriteAudioWithoutTrack(t *testing.T) {
	config := DefaultMediaStreamConfig()
	config.AudioEnabled = false

	ms, err := NewMediaStream(config)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	sample := media.Sample{
		Data:      []byte{1, 2, 3, 4},
		Duration:  time.Millisecond * 20,
		Timestamp: time.Now(),
	}

	err = ms.WriteAudio(sample)
	if err == nil {
		t.Error("Expected error when writing audio sample without audio track")
	}
}

func TestMediaStreamClose(t *testing.T) {
	config := DefaultMediaStreamConfig()

	ms, err := NewMediaStream(config)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	err = ms.Close()
	if err != nil {
		t.Errorf("Failed to close media stream: %v", err)
	}

	// After closing, tracks should be nil
	if ms.GetVideoTrack() != nil {
		t.Error("Video track should be nil after closing")
	}

	if ms.GetAudioTrack() != nil {
		t.Error("Audio track should be nil after closing")
	}
}

func TestGetVideoMimeType(t *testing.T) {
	tests := []struct {
		codec    string
		expected string
	}{
		{"h264", "video/H264"},
		{"vp8", "video/VP8"},
		{"vp9", "video/VP9"},
		{"unknown", "video/H264"}, // default
	}

	for _, test := range tests {
		result := getVideoMimeType(test.codec)
		if result != test.expected {
			t.Errorf("getVideoMimeType(%s) = %s, expected %s", test.codec, result, test.expected)
		}
	}
}

func TestGetAudioMimeType(t *testing.T) {
	tests := []struct {
		codec    string
		expected string
	}{
		{"opus", "audio/opus"},
		{"pcmu", "audio/PCMU"},
		{"pcma", "audio/PCMA"},
		{"unknown", "audio/opus"}, // default
	}

	for _, test := range tests {
		result := getAudioMimeType(test.codec)
		if result != test.expected {
			t.Errorf("getAudioMimeType(%s) = %s, expected %s", test.codec, result, test.expected)
		}
	}
}

func TestMediaStreamStats(t *testing.T) {
	config := DefaultMediaStreamConfig()
	config.AudioEnabled = true

	ms, err := NewMediaStream(config)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	// Initial stats should be zero
	stats := ms.GetStats()
	if stats.VideoFramesSent != 0 {
		t.Errorf("Expected 0 video frames sent initially, got %d", stats.VideoFramesSent)
	}

	if stats.AudioFramesSent != 0 {
		t.Errorf("Expected 0 audio frames sent initially, got %d", stats.AudioFramesSent)
	}

	if stats.TotalTracks != 2 {
		t.Errorf("Expected 2 total tracks, got %d", stats.TotalTracks)
	}

	if stats.ActiveTracks != 2 {
		t.Errorf("Expected 2 active tracks, got %d", stats.ActiveTracks)
	}

	// Write some samples
	videoSample := media.Sample{
		Data:      make([]byte, 100),
		Duration:  time.Millisecond * 33,
		Timestamp: time.Now(),
	}

	audioSample := media.Sample{
		Data:      make([]byte, 50),
		Duration:  time.Millisecond * 20,
		Timestamp: time.Now(),
	}

	ms.WriteVideo(videoSample)
	ms.WriteAudio(audioSample)

	// Check updated stats
	stats = ms.GetStats()
	if stats.VideoFramesSent != 1 {
		t.Errorf("Expected 1 video frame sent, got %d", stats.VideoFramesSent)
	}

	if stats.AudioFramesSent != 1 {
		t.Errorf("Expected 1 audio frame sent, got %d", stats.AudioFramesSent)
	}

	if stats.VideoBytesSent != 100 {
		t.Errorf("Expected 100 video bytes sent, got %d", stats.VideoBytesSent)
	}

	if stats.AudioBytesSent != 50 {
		t.Errorf("Expected 50 audio bytes sent, got %d", stats.AudioBytesSent)
	}
}

func TestTrackManagement(t *testing.T) {
	config := DefaultMediaStreamConfig()
	config.AudioEnabled = true

	ms, err := NewMediaStream(config)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	// Test GetAllTracks
	tracks := ms.GetAllTracks()
	if len(tracks) != 2 {
		t.Errorf("Expected 2 tracks, got %d", len(tracks))
	}

	// Test GetTrackState
	videoState, err := ms.GetTrackState(config.VideoTrackID)
	if err != nil {
		t.Errorf("Failed to get video track state: %v", err)
	}
	if videoState != TrackStateActive {
		t.Errorf("Expected video track to be active, got %s", videoState.String())
	}

	audioState, err := ms.GetTrackState(config.AudioTrackID)
	if err != nil {
		t.Errorf("Failed to get audio track state: %v", err)
	}
	if audioState != TrackStateActive {
		t.Errorf("Expected audio track to be active, got %s", audioState.String())
	}

	// Test RemoveTrack
	err = ms.RemoveTrack(config.AudioTrackID)
	if err != nil {
		t.Errorf("Failed to remove audio track: %v", err)
	}

	// Check that track is removed
	_, err = ms.GetTrackState(config.AudioTrackID)
	if err == nil {
		t.Error("Expected error when getting state of removed track")
	}

	// Check updated track count
	tracks = ms.GetAllTracks()
	if len(tracks) != 1 {
		t.Errorf("Expected 1 track after removal, got %d", len(tracks))
	}
}

func TestTrackMonitoring(t *testing.T) {
	config := DefaultMediaStreamConfig()
	ms, err := NewMediaStream(config)
	if err != nil {
		t.Fatalf("Failed to create media stream: %v", err)
	}

	// Start monitoring
	err = ms.StartTrackMonitoring()
	if err != nil {
		t.Errorf("Failed to start track monitoring: %v", err)
	}

	// Try to start again (should fail)
	err = ms.StartTrackMonitoring()
	if err == nil {
		t.Error("Expected error when starting monitoring twice")
	}

	// Stop monitoring
	err = ms.StopTrackMonitoring()
	if err != nil {
		t.Errorf("Failed to stop track monitoring: %v", err)
	}

	// Stop again (should not fail)
	err = ms.StopTrackMonitoring()
	if err != nil {
		t.Errorf("Unexpected error when stopping monitoring twice: %v", err)
	}
}

func TestTrackStateString(t *testing.T) {
	tests := []struct {
		state    TrackState
		expected string
	}{
		{TrackStateActive, "active"},
		{TrackStateInactive, "inactive"},
		{TrackStateMuted, "muted"},
		{TrackStateEnded, "ended"},
		{TrackStateUnknown, "unknown"},
	}

	for _, test := range tests {
		result := test.state.String()
		if result != test.expected {
			t.Errorf("TrackState(%d).String() = %s, expected %s", test.state, result, test.expected)
		}
	}
}

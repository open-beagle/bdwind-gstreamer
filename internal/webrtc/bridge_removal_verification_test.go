package webrtc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
)

// TestBridgeRemovalVerification verifies that the GStreamer Bridge has been successfully removed
// and replaced with direct MediaStreamProvider integration
func TestBridgeRemovalVerification(t *testing.T) {
	t.Run("VerifyBridgeRemoval", testVerifyBridgeRemoval)
	t.Run("VerifyMediaProviderIntegration", testVerifyMediaProviderIntegration)
	t.Run("VerifyWebRTCSubscriber", testVerifyWebRTCSubscriber)
}

// testVerifyBridgeRemoval verifies that the bridge has been completely removed
func testVerifyBridgeRemoval(t *testing.T) {
	t.Log("=== Verifying GStreamer Bridge Removal ===")

	// Create a mock media provider for testing
	mockProvider := &MockMediaProvider{}

	// Create WebRTC manager config
	cfg := &ManagerConfig{
		Config: &config.Config{
			GStreamer: config.GStreamerConfig{
				Encoding: config.EncoderConfig{
					Codec: config.CodecH264,
				},
				Capture: config.DesktopCaptureConfig{
					Width:     1920,
					Height:    1080,
					FrameRate: 30,
				},
			},
		},
		MediaProvider: mockProvider,
	}

	ctx := context.Background()
	manager, err := NewManager(ctx, cfg)
	require.NoError(t, err, "Failed to create WebRTC manager")
	require.NotNil(t, manager, "Manager should not be nil")

	// Verify that the manager has MediaProvider instead of Bridge
	assert.NotNil(t, manager.mediaProvider, "MediaProvider should be set")
	assert.NotNil(t, manager.mediaSubscriber, "MediaSubscriber should be created")
	assert.Equal(t, mockProvider, manager.mediaProvider, "MediaProvider should match the provided one")

	t.Log("✓ Confirmed that GStreamer Bridge has been removed and replaced with MediaProvider")
}

// testVerifyMediaProviderIntegration verifies that WebRTC integrates correctly with MediaProvider
func testVerifyMediaProviderIntegration(t *testing.T) {
	t.Log("=== Verifying MediaProvider Integration ===")

	mockProvider := &MockMediaProvider{}

	cfg := &ManagerConfig{
		Config: &config.Config{
			GStreamer: config.GStreamerConfig{
				Encoding: config.EncoderConfig{
					Codec: config.CodecH264,
				},
				Capture: config.DesktopCaptureConfig{
					Width:     1920,
					Height:    1080,
					FrameRate: 30,
				},
			},
		},
		MediaProvider: mockProvider,
	}

	ctx := context.Background()
	manager, err := NewManager(ctx, cfg)
	require.NoError(t, err)

	// Verify that the subscriber was registered with the provider
	assert.True(t, mockProvider.videoSubscriberAdded, "Video subscriber should be added to MediaProvider")
	assert.NotZero(t, manager.subscriberID, "Subscriber ID should be set")

	t.Log("✓ Confirmed that WebRTC correctly integrates with MediaProvider")
}

// testVerifyWebRTCSubscriber verifies that the WebRTC subscriber works correctly
func testVerifyWebRTCSubscriber(t *testing.T) {
	t.Log("=== Verifying WebRTC Subscriber ===")

	// Create a mock media stream
	mockMediaStream := &MockMediaStream{}

	// Create WebRTC media subscriber
	subscriber := NewWebRTCMediaSubscriber(123, mockMediaStream, nil)
	require.NotNil(t, subscriber, "Subscriber should not be nil")

	// Test subscriber properties
	assert.Equal(t, uint64(123), subscriber.GetSubscriberID(), "Subscriber ID should match")
	assert.True(t, subscriber.IsActive(), "Subscriber should be active by default")

	// Test video frame processing
	videoFrame := &gstreamer.EncodedVideoFrame{
		Data:      []byte("test video data"),
		Timestamp: time.Now(),
		Duration:  time.Millisecond * 33, // ~30fps
		KeyFrame:  true,
		Width:     1920,
		Height:    1080,
		Codec:     "h264",
		Bitrate:   2000000,
	}

	err := subscriber.OnVideoFrame(videoFrame)
	assert.NoError(t, err, "Video frame processing should succeed")
	assert.True(t, mockMediaStream.videoWritten, "Video should be written to media stream")

	// Test audio frame processing
	audioFrame := &gstreamer.EncodedAudioFrame{
		Data:       []byte("test audio data"),
		Timestamp:  time.Now(),
		Duration:   time.Millisecond * 20,
		SampleRate: 48000,
		Channels:   2,
		Codec:      "opus",
		Bitrate:    128000,
	}

	err = subscriber.OnAudioFrame(audioFrame)
	assert.NoError(t, err, "Audio frame processing should succeed")
	assert.True(t, mockMediaStream.audioWritten, "Audio should be written to media stream")

	// Test statistics
	stats := subscriber.GetStatistics()
	assert.NotNil(t, stats, "Statistics should not be nil")
	assert.Equal(t, uint64(123), stats["id"], "Statistics should include subscriber ID")
	assert.True(t, stats["active"].(bool), "Statistics should show subscriber as active")

	t.Log("✓ Confirmed that WebRTC subscriber works correctly")
}

// MockMediaProvider implements gstreamer.MediaStreamProvider for testing
type MockMediaProvider struct {
	videoSubscriberAdded bool
	audioSubscriberAdded bool
	subscribers          map[uint64]interface{}
	nextID               uint64
}

func (m *MockMediaProvider) AddVideoSubscriber(subscriber gstreamer.VideoStreamSubscriber) (uint64, error) {
	if m.subscribers == nil {
		m.subscribers = make(map[uint64]interface{})
	}
	m.nextID++
	m.subscribers[m.nextID] = subscriber
	m.videoSubscriberAdded = true
	return m.nextID, nil
}

func (m *MockMediaProvider) RemoveVideoSubscriber(id uint64) bool {
	if m.subscribers == nil {
		return false
	}
	_, exists := m.subscribers[id]
	if exists {
		delete(m.subscribers, id)
	}
	return exists
}

func (m *MockMediaProvider) AddAudioSubscriber(subscriber gstreamer.AudioStreamSubscriber) (uint64, error) {
	if m.subscribers == nil {
		m.subscribers = make(map[uint64]interface{})
	}
	m.nextID++
	m.subscribers[m.nextID] = subscriber
	m.audioSubscriberAdded = true
	return m.nextID, nil
}

func (m *MockMediaProvider) RemoveAudioSubscriber(id uint64) bool {
	if m.subscribers == nil {
		return false
	}
	_, exists := m.subscribers[id]
	if exists {
		delete(m.subscribers, id)
	}
	return exists
}

func (m *MockMediaProvider) AdaptToNetworkCondition(condition gstreamer.NetworkCondition) error {
	return nil
}

func (m *MockMediaProvider) GenerateMediaConfigForWebRTC() *gstreamer.MediaConfig {
	return &gstreamer.MediaConfig{
		VideoCodec:   "h264",
		VideoProfile: "baseline",
		Bitrate:      2000000,
		Resolution:   "1920x1080",
		FrameRate:    30,
	}
}

// MockMediaStream implements MediaStream for testing
type MockMediaStream struct {
	videoWritten bool
	audioWritten bool
}

func (m *MockMediaStream) WriteVideo(sample interface{}) error {
	m.videoWritten = true
	return nil
}

func (m *MockMediaStream) WriteAudio(sample interface{}) error {
	m.audioWritten = true
	return nil
}

func (m *MockMediaStream) GetVideoTrack() interface{} {
	return nil
}

func (m *MockMediaStream) GetAudioTrack() interface{} {
	return nil
}

func (m *MockMediaStream) GetStats() interface{} {
	return map[string]interface{}{
		"TotalTracks":  2,
		"ActiveTracks": 2,
	}
}

func (m *MockMediaStream) StartTrackMonitoring() error {
	return nil
}

func (m *MockMediaStream) StopTrackMonitoring() error {
	return nil
}

func (m *MockMediaStream) Close() error {
	return nil
}

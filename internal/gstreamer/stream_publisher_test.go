package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock video subscriber for testing
type mockVideoSubscriber struct {
	id             uint64
	active         bool
	receivedFrames []*EncodedVideoFrame
	receivedErrors []error
	mutex          sync.RWMutex
	onFrameDelay   time.Duration
	shouldError    bool
}

func newMockVideoSubscriber(id uint64) *mockVideoSubscriber {
	return &mockVideoSubscriber{
		id:             id,
		active:         true,
		receivedFrames: make([]*EncodedVideoFrame, 0),
		receivedErrors: make([]error, 0),
	}
}

func (m *mockVideoSubscriber) OnVideoFrame(frame *EncodedVideoFrame) error {
	if m.onFrameDelay > 0 {
		time.Sleep(m.onFrameDelay)
	}

	if m.shouldError {
		return fmt.Errorf("mock error")
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.receivedFrames = append(m.receivedFrames, frame)
	return nil
}

func (m *mockVideoSubscriber) OnVideoError(err error) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.receivedErrors = append(m.receivedErrors, err)
	return nil
}

func (m *mockVideoSubscriber) GetSubscriberID() uint64 {
	return m.id
}

func (m *mockVideoSubscriber) IsActive() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.active
}

func (m *mockVideoSubscriber) SetActive(active bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.active = active
}

func (m *mockVideoSubscriber) SetDelay(delay time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.onFrameDelay = delay
}

func (m *mockVideoSubscriber) SetShouldError(shouldError bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.shouldError = shouldError
}

func (m *mockVideoSubscriber) GetReceivedFrames() []*EncodedVideoFrame {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	frames := make([]*EncodedVideoFrame, len(m.receivedFrames))
	copy(frames, m.receivedFrames)
	return frames
}

func (m *mockVideoSubscriber) GetReceivedErrors() []error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	errors := make([]error, len(m.receivedErrors))
	copy(errors, m.receivedErrors)
	return errors
}

// Mock audio subscriber for testing
type mockAudioSubscriber struct {
	id             uint64
	active         bool
	receivedFrames []*EncodedAudioFrame
	receivedErrors []error
	mutex          sync.RWMutex
	onFrameDelay   time.Duration
	shouldError    bool
}

func newMockAudioSubscriber(id uint64) *mockAudioSubscriber {
	return &mockAudioSubscriber{
		id:             id,
		active:         true,
		receivedFrames: make([]*EncodedAudioFrame, 0),
		receivedErrors: make([]error, 0),
	}
}

func (m *mockAudioSubscriber) OnAudioFrame(frame *EncodedAudioFrame) error {
	if m.onFrameDelay > 0 {
		time.Sleep(m.onFrameDelay)
	}

	if m.shouldError {
		return fmt.Errorf("mock error")
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.receivedFrames = append(m.receivedFrames, frame)
	return nil
}

func (m *mockAudioSubscriber) OnAudioError(err error) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.receivedErrors = append(m.receivedErrors, err)
	return nil
}

func (m *mockAudioSubscriber) GetSubscriberID() uint64 {
	return m.id
}

func (m *mockAudioSubscriber) IsActive() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.active
}

func (m *mockAudioSubscriber) SetActive(active bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.active = active
}

func (m *mockAudioSubscriber) SetDelay(delay time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.onFrameDelay = delay
}

func (m *mockAudioSubscriber) SetShouldError(shouldError bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.shouldError = shouldError
}

func (m *mockAudioSubscriber) GetReceivedFrames() []*EncodedAudioFrame {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	frames := make([]*EncodedAudioFrame, len(m.receivedFrames))
	copy(frames, m.receivedFrames)
	return frames
}

func (m *mockAudioSubscriber) GetReceivedErrors() []error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	errors := make([]error, len(m.receivedErrors))
	copy(errors, m.receivedErrors)
	return errors
}

// Mock media subscriber for testing
type mockMediaSubscriber struct {
	*mockVideoSubscriber
	*mockAudioSubscriber
	streamStarted bool
	streamStopped bool
	mutex         sync.RWMutex
}

func newMockMediaSubscriber(id uint64) *mockMediaSubscriber {
	return &mockMediaSubscriber{
		mockVideoSubscriber: newMockVideoSubscriber(id),
		mockAudioSubscriber: newMockAudioSubscriber(id),
	}
}

func (m *mockMediaSubscriber) OnStreamStart() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.streamStarted = true
	return nil
}

func (m *mockMediaSubscriber) OnStreamStop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.streamStopped = true
	return nil
}

func (m *mockMediaSubscriber) IsStreamStarted() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.streamStarted
}

func (m *mockMediaSubscriber) IsStreamStopped() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.streamStopped
}

// Test helper functions
func createTestStreamPublisher(t *testing.T) *StreamPublisher {
	ctx := context.Background()
	config := DefaultStreamPublisherConfig()
	config.PublishTimeout = 100 * time.Millisecond // Shorter timeout for tests
	config.MetricsInterval = 50 * time.Millisecond // Faster metrics for tests

	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.DebugLevel)

	return NewStreamPublisher(ctx, config, logger)
}

func createTestVideoFrame() *EncodedVideoFrame {
	return &EncodedVideoFrame{
		Data:        []byte("test video data"),
		Timestamp:   time.Now(),
		Duration:    33 * time.Millisecond,
		KeyFrame:    true,
		Width:       1920,
		Height:      1080,
		Codec:       "h264",
		Bitrate:     2000000,
		FrameNumber: 1,
		PTS:         1000,
		DTS:         1000,
	}
}

func createTestAudioFrame() *EncodedAudioFrame {
	return &EncodedAudioFrame{
		Data:        []byte("test audio data"),
		Timestamp:   time.Now(),
		Duration:    20 * time.Millisecond,
		SampleRate:  48000,
		Channels:    2,
		Codec:       "opus",
		Bitrate:     128000,
		FrameNumber: 1,
		PTS:         1000,
	}
}

// Test StreamPublisher creation and lifecycle
func TestStreamPublisher_NewStreamPublisher(t *testing.T) {
	sp := createTestStreamPublisher(t)
	assert.NotNil(t, sp)
	assert.False(t, sp.IsRunning())
	assert.Equal(t, 0, sp.GetSubscriberCount())
}

func TestStreamPublisher_StartStop(t *testing.T) {
	sp := createTestStreamPublisher(t)

	// Test start
	err := sp.Start()
	require.NoError(t, err)
	assert.True(t, sp.IsRunning())

	// Test double start
	err = sp.Start()
	assert.Error(t, err)

	// Test stop
	err = sp.Stop()
	require.NoError(t, err)
	assert.False(t, sp.IsRunning())

	// Test double stop
	err = sp.Stop()
	assert.Error(t, err)
}

// Test video subscriber management
func TestStreamPublisher_VideoSubscribers(t *testing.T) {
	sp := createTestStreamPublisher(t)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Test adding video subscriber
	sub1 := newMockVideoSubscriber(1)
	id1, err := sp.AddVideoSubscriber(sub1)
	require.NoError(t, err)
	assert.Greater(t, id1, uint64(0))
	assert.Equal(t, 1, sp.GetSubscriberCount())

	// Test adding another video subscriber
	sub2 := newMockVideoSubscriber(2)
	id2, err := sp.AddVideoSubscriber(sub2)
	require.NoError(t, err)
	assert.Greater(t, id2, uint64(0))
	assert.NotEqual(t, id1, id2)
	assert.Equal(t, 2, sp.GetSubscriberCount())

	// Test adding nil subscriber
	_, err = sp.AddVideoSubscriber(nil)
	assert.Error(t, err)

	// Test removing video subscriber
	removed := sp.RemoveVideoSubscriber(id1)
	assert.True(t, removed)
	assert.Equal(t, 1, sp.GetSubscriberCount())

	// Test removing non-existent subscriber
	removed = sp.RemoveVideoSubscriber(999)
	assert.False(t, removed)
	assert.Equal(t, 1, sp.GetSubscriberCount())
}

// Test audio subscriber management
func TestStreamPublisher_AudioSubscribers(t *testing.T) {
	sp := createTestStreamPublisher(t)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Test adding audio subscriber
	sub1 := newMockAudioSubscriber(1)
	id1, err := sp.AddAudioSubscriber(sub1)
	require.NoError(t, err)
	assert.Greater(t, id1, uint64(0))
	assert.Equal(t, 1, sp.GetSubscriberCount())

	// Test adding another audio subscriber
	sub2 := newMockAudioSubscriber(2)
	id2, err := sp.AddAudioSubscriber(sub2)
	require.NoError(t, err)
	assert.Greater(t, id2, uint64(0))
	assert.NotEqual(t, id1, id2)
	assert.Equal(t, 2, sp.GetSubscriberCount())

	// Test removing audio subscriber
	removed := sp.RemoveAudioSubscriber(id1)
	assert.True(t, removed)
	assert.Equal(t, 1, sp.GetSubscriberCount())
}

// Test media subscriber management
func TestStreamPublisher_MediaSubscribers(t *testing.T) {
	sp := createTestStreamPublisher(t)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Test adding media subscriber
	sub := newMockMediaSubscriber(1)
	id, err := sp.AddMediaSubscriber(sub)
	require.NoError(t, err)
	assert.Greater(t, id, uint64(0))
	assert.Equal(t, 1, sp.GetSubscriberCount())

	// Check that OnStreamStart was called
	time.Sleep(10 * time.Millisecond)
	assert.True(t, sub.IsStreamStarted())

	// Test removing media subscriber
	removed := sp.RemoveMediaSubscriber(id)
	assert.True(t, removed)
	assert.Equal(t, 0, sp.GetSubscriberCount())

	// Check that OnStreamStop was called
	time.Sleep(10 * time.Millisecond)
	assert.True(t, sub.IsStreamStopped())
}

// Test video frame publishing
func TestStreamPublisher_PublishVideoFrame(t *testing.T) {
	sp := createTestStreamPublisher(t)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Add video subscriber
	sub := newMockVideoSubscriber(1)
	_, err := sp.AddVideoSubscriber(sub)
	require.NoError(t, err)

	// Publish video frame
	frame := createTestVideoFrame()
	err = sp.PublishVideoFrame(frame)
	require.NoError(t, err)

	// Wait for frame to be processed
	time.Sleep(50 * time.Millisecond)

	// Check that subscriber received the frame
	receivedFrames := sub.GetReceivedFrames()
	assert.Len(t, receivedFrames, 1)
	assert.Equal(t, frame.Data, receivedFrames[0].Data)

	// Check statistics
	stats := sp.GetStatistics()
	assert.Equal(t, uint64(1), stats.VideoFramesPublished)
	assert.Equal(t, 1, stats.VideoSubscribers)
}

// Test audio frame publishing
func TestStreamPublisher_PublishAudioFrame(t *testing.T) {
	sp := createTestStreamPublisher(t)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Add audio subscriber
	sub := newMockAudioSubscriber(1)
	_, err := sp.AddAudioSubscriber(sub)
	require.NoError(t, err)

	// Publish audio frame
	frame := createTestAudioFrame()
	err = sp.PublishAudioFrame(frame)
	require.NoError(t, err)

	// Wait for frame to be processed
	time.Sleep(50 * time.Millisecond)

	// Check that subscriber received the frame
	receivedFrames := sub.GetReceivedFrames()
	assert.Len(t, receivedFrames, 1)
	assert.Equal(t, frame.Data, receivedFrames[0].Data)

	// Check statistics
	stats := sp.GetStatistics()
	assert.Equal(t, uint64(1), stats.AudioFramesPublished)
	assert.Equal(t, 1, stats.AudioSubscribers)
}

// Test media subscriber receiving both video and audio
func TestStreamPublisher_MediaSubscriberReceivesBoth(t *testing.T) {
	sp := createTestStreamPublisher(t)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Add media subscriber
	sub := newMockMediaSubscriber(1)
	_, err := sp.AddMediaSubscriber(sub)
	require.NoError(t, err)

	// Publish video frame
	videoFrame := createTestVideoFrame()
	err = sp.PublishVideoFrame(videoFrame)
	require.NoError(t, err)

	// Publish audio frame
	audioFrame := createTestAudioFrame()
	err = sp.PublishAudioFrame(audioFrame)
	require.NoError(t, err)

	// Wait for frames to be processed
	time.Sleep(50 * time.Millisecond)

	// Check that subscriber received both frames
	receivedVideoFrames := sub.mockVideoSubscriber.GetReceivedFrames()
	receivedAudioFrames := sub.mockAudioSubscriber.GetReceivedFrames()

	assert.Len(t, receivedVideoFrames, 1)
	assert.Len(t, receivedAudioFrames, 1)
	assert.Equal(t, videoFrame.Data, receivedVideoFrames[0].Data)
	assert.Equal(t, audioFrame.Data, receivedAudioFrames[0].Data)
}

// Test publishing with nil frames
func TestStreamPublisher_PublishNilFrames(t *testing.T) {
	sp := createTestStreamPublisher(t)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Test publishing nil video frame
	err := sp.PublishVideoFrame(nil)
	assert.Error(t, err)

	// Test publishing nil audio frame
	err = sp.PublishAudioFrame(nil)
	assert.Error(t, err)
}

// Test publishing when not running
func TestStreamPublisher_PublishWhenNotRunning(t *testing.T) {
	sp := createTestStreamPublisher(t)

	// Test publishing video frame when not running
	frame := createTestVideoFrame()
	err := sp.PublishVideoFrame(frame)
	assert.Error(t, err)

	// Test publishing audio frame when not running
	audioFrame := createTestAudioFrame()
	err = sp.PublishAudioFrame(audioFrame)
	assert.Error(t, err)
}

// Test subscriber error handling
func TestStreamPublisher_SubscriberErrorHandling(t *testing.T) {
	sp := createTestStreamPublisher(t)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Add video subscriber that will error
	sub := newMockVideoSubscriber(1)
	sub.SetShouldError(true)
	_, err := sp.AddVideoSubscriber(sub)
	require.NoError(t, err)

	// Publish video frame
	frame := createTestVideoFrame()
	err = sp.PublishVideoFrame(frame)
	require.NoError(t, err)

	// Wait for frame to be processed and error to be handled
	time.Sleep(50 * time.Millisecond)

	// Check that subscriber received error notification
	receivedErrors := sub.GetReceivedErrors()
	assert.Len(t, receivedErrors, 1)

	// Check statistics
	stats := sp.GetStatistics()
	assert.Greater(t, stats.PublishErrors, uint64(0))
}

// Test subscriber timeout handling
func TestStreamPublisher_SubscriberTimeout(t *testing.T) {
	sp := createTestStreamPublisher(t)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Add video subscriber with long delay (longer than publish timeout)
	sub := newMockVideoSubscriber(1)
	sub.SetDelay(200 * time.Millisecond) // Longer than 100ms timeout
	_, err := sp.AddVideoSubscriber(sub)
	require.NoError(t, err)

	// Publish video frame
	frame := createTestVideoFrame()
	err = sp.PublishVideoFrame(frame)
	require.NoError(t, err)

	// Wait for timeout to occur
	time.Sleep(150 * time.Millisecond)

	// Check that subscriber received timeout error notification
	receivedErrors := sub.GetReceivedErrors()
	assert.Len(t, receivedErrors, 1)
	assert.Contains(t, receivedErrors[0].Error(), "timeout")
}

// Test inactive subscriber handling
func TestStreamPublisher_InactiveSubscriber(t *testing.T) {
	sp := createTestStreamPublisher(t)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Add video subscriber and mark as inactive
	sub := newMockVideoSubscriber(1)
	sub.SetActive(false)
	_, err := sp.AddVideoSubscriber(sub)
	require.NoError(t, err)

	// Publish video frame
	frame := createTestVideoFrame()
	err = sp.PublishVideoFrame(frame)
	require.NoError(t, err)

	// Wait for frame processing
	time.Sleep(50 * time.Millisecond)

	// Check that inactive subscriber didn't receive the frame
	receivedFrames := sub.GetReceivedFrames()
	assert.Len(t, receivedFrames, 0)
}

// Test statistics and status
func TestStreamPublisher_DetailedStatistics(t *testing.T) {
	sp := createTestStreamPublisher(t)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Add subscribers
	videoSub := newMockVideoSubscriber(1)
	audioSub := newMockAudioSubscriber(2)
	mediaSub := newMockMediaSubscriber(3)

	_, err := sp.AddVideoSubscriber(videoSub)
	require.NoError(t, err)
	_, err = sp.AddAudioSubscriber(audioSub)
	require.NoError(t, err)
	_, err = sp.AddMediaSubscriber(mediaSub)
	require.NoError(t, err)

	// Publish frames
	videoFrame := createTestVideoFrame()
	audioFrame := createTestAudioFrame()

	err = sp.PublishVideoFrame(videoFrame)
	require.NoError(t, err)
	err = sp.PublishAudioFrame(audioFrame)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Check statistics
	stats := sp.GetStatistics()
	assert.Equal(t, uint64(1), stats.VideoFramesPublished)
	assert.Equal(t, uint64(1), stats.AudioFramesPublished)
	assert.Equal(t, 1, stats.VideoSubscribers)
	assert.Equal(t, 1, stats.AudioSubscribers)
	assert.Equal(t, 3, stats.TotalSubscribers)
	assert.Equal(t, 3, stats.ActiveSubscribers)

	// Check status
	status := sp.GetStatus()
	assert.True(t, status["running"].(bool))
	assert.Equal(t, 3, status["total_subscribers"].(int))
}

// Test configuration
func TestStreamPublisher_DetailedConfiguration(t *testing.T) {
	config := DefaultStreamPublisherConfig()
	assert.NotNil(t, config)
	assert.Equal(t, 100, config.VideoBufferSize)
	assert.Equal(t, 100, config.AudioBufferSize)
	assert.Equal(t, 10, config.MaxSubscribers)
	assert.Equal(t, 1*time.Second, config.PublishTimeout)
	assert.True(t, config.EnableMetrics)
	assert.Equal(t, 30*time.Second, config.MetricsInterval)
}

// Test frame structures
func TestStreamPublisher_FrameStructures(t *testing.T) {
	videoFrame := &EncodedVideoFrame{
		Data:        []byte("test data"),
		Timestamp:   time.Now(),
		Duration:    33 * time.Millisecond,
		KeyFrame:    true,
		Width:       1920,
		Height:      1080,
		Codec:       "h264",
		Bitrate:     2000000,
		FrameNumber: 1,
		PTS:         1000,
		DTS:         1000,
	}

	assert.NotNil(t, videoFrame)
	assert.Equal(t, []byte("test data"), videoFrame.Data)
	assert.True(t, videoFrame.KeyFrame)
	assert.Equal(t, 1920, videoFrame.Width)
	assert.Equal(t, 1080, videoFrame.Height)
	assert.Equal(t, "h264", videoFrame.Codec)

	audioFrame := &EncodedAudioFrame{
		Data:        []byte("test audio data"),
		Timestamp:   time.Now(),
		Duration:    20 * time.Millisecond,
		SampleRate:  48000,
		Channels:    2,
		Codec:       "opus",
		Bitrate:     128000,
		FrameNumber: 1,
		PTS:         1000,
	}

	assert.NotNil(t, audioFrame)
	assert.Equal(t, []byte("test audio data"), audioFrame.Data)
	assert.Equal(t, 48000, audioFrame.SampleRate)
	assert.Equal(t, 2, audioFrame.Channels)
	assert.Equal(t, "opus", audioFrame.Codec)
}

// Test legacy sample compatibility
func TestStreamPublisher_LegacySampleCompatibility(t *testing.T) {
	sp := createTestStreamPublisher(t)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Add video subscriber
	videoSub := newMockVideoSubscriber(1)
	_, err := sp.AddVideoSubscriber(videoSub)
	require.NoError(t, err)

	// Add audio subscriber
	audioSub := newMockAudioSubscriber(2)
	_, err = sp.AddAudioSubscriber(audioSub)
	require.NoError(t, err)

	// Publish video sample using legacy method
	videoSample := &Sample{
		Data:      []byte("legacy video data"),
		Timestamp: time.Now(),
		Format: SampleFormat{
			Width:  1920,
			Height: 1080,
			Codec:  "h264",
		},
		Metadata: map[string]interface{}{
			"key_frame": true,
			"bitrate":   2000000,
			"duration":  33 * time.Millisecond,
		},
	}

	err = sp.PublishVideoSample(videoSample)
	require.NoError(t, err)

	// Publish audio sample using legacy method
	audioSample := &Sample{
		Data:      []byte("legacy audio data"),
		Timestamp: time.Now(),
		Format: SampleFormat{
			SampleRate: 48000,
			Channels:   2,
			Codec:      "opus",
		},
		Metadata: map[string]interface{}{
			"bitrate":  128000,
			"duration": 20 * time.Millisecond,
		},
	}

	err = sp.PublishAudioSample(audioSample)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Check that subscribers received the frames
	receivedVideoFrames := videoSub.GetReceivedFrames()
	receivedAudioFrames := audioSub.GetReceivedFrames()

	assert.Len(t, receivedVideoFrames, 1)
	assert.Len(t, receivedAudioFrames, 1)

	assert.Equal(t, videoSample.Data, receivedVideoFrames[0].Data)
	assert.Equal(t, audioSample.Data, receivedAudioFrames[0].Data)
	assert.True(t, receivedVideoFrames[0].KeyFrame)
	assert.Equal(t, 2000000, receivedVideoFrames[0].Bitrate)
}

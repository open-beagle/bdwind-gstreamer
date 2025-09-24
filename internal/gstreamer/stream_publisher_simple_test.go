package gstreamer

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Simple test that doesn't depend on conflicting types
func TestStreamPublisher_Basic(t *testing.T) {
	ctx := context.Background()
	config := DefaultStreamPublisherConfig()
	logger := logrus.NewEntry(logrus.New())

	sp := NewStreamPublisher(ctx, config, logger)
	assert.NotNil(t, sp)
	assert.False(t, sp.IsRunning())

	// Test start
	err := sp.Start()
	require.NoError(t, err)
	assert.True(t, sp.IsRunning())

	// Test stop
	err = sp.Stop()
	require.NoError(t, err)
	assert.False(t, sp.IsRunning())
}

func TestStreamPublisher_VideoFrameStructure(t *testing.T) {
	frame := &EncodedVideoFrame{
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

	assert.NotNil(t, frame)
	assert.Equal(t, []byte("test data"), frame.Data)
	assert.True(t, frame.KeyFrame)
	assert.Equal(t, 1920, frame.Width)
	assert.Equal(t, 1080, frame.Height)
	assert.Equal(t, "h264", frame.Codec)
}

func TestStreamPublisher_AudioFrameStructure(t *testing.T) {
	frame := &EncodedAudioFrame{
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

	assert.NotNil(t, frame)
	assert.Equal(t, []byte("test audio data"), frame.Data)
	assert.Equal(t, 48000, frame.SampleRate)
	assert.Equal(t, 2, frame.Channels)
	assert.Equal(t, "opus", frame.Codec)
}

func TestStreamPublisher_Configuration(t *testing.T) {
	config := DefaultStreamPublisherConfig()
	assert.NotNil(t, config)
	assert.Equal(t, 100, config.VideoBufferSize)
	assert.Equal(t, 100, config.AudioBufferSize)
	assert.Equal(t, 10, config.MaxSubscribers)
	assert.Equal(t, 1*time.Second, config.PublishTimeout)
	assert.True(t, config.EnableMetrics)
	assert.Equal(t, 30*time.Second, config.MetricsInterval)
}

func TestStreamPublisher_PublishingWhenNotRunning(t *testing.T) {
	ctx := context.Background()
	config := DefaultStreamPublisherConfig()
	logger := logrus.NewEntry(logrus.New())

	sp := NewStreamPublisher(ctx, config, logger)

	// Test publishing video frame when not running
	frame := &EncodedVideoFrame{
		Data:      []byte("test data"),
		Timestamp: time.Now(),
		Codec:     "h264",
	}
	err := sp.PublishVideoFrame(frame)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")

	// Test publishing audio frame when not running
	audioFrame := &EncodedAudioFrame{
		Data:      []byte("test audio data"),
		Timestamp: time.Now(),
		Codec:     "opus",
	}
	err = sp.PublishAudioFrame(audioFrame)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestStreamPublisher_NilFrameHandling(t *testing.T) {
	ctx := context.Background()
	config := DefaultStreamPublisherConfig()
	logger := logrus.NewEntry(logrus.New())

	sp := NewStreamPublisher(ctx, config, logger)
	require.NoError(t, sp.Start())
	defer sp.Stop()

	// Test publishing nil video frame
	err := sp.PublishVideoFrame(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")

	// Test publishing nil audio frame
	err = sp.PublishAudioFrame(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestStreamPublisher_SubscriberCount(t *testing.T) {
	ctx := context.Background()
	config := DefaultStreamPublisherConfig()
	logger := logrus.NewEntry(logrus.New())

	sp := NewStreamPublisher(ctx, config, logger)
	assert.Equal(t, 0, sp.GetSubscriberCount())

	// The count should remain 0 since we haven't added any subscribers
	// This tests the basic functionality without needing mock subscribers
}

func TestStreamPublisher_Statistics(t *testing.T) {
	ctx := context.Background()
	config := DefaultStreamPublisherConfig()
	logger := logrus.NewEntry(logrus.New())

	sp := NewStreamPublisher(ctx, config, logger)

	stats := sp.GetStatistics()
	assert.Equal(t, uint64(0), stats.VideoFramesPublished)
	assert.Equal(t, uint64(0), stats.AudioFramesPublished)
	assert.Equal(t, 0, stats.TotalSubscribers)
	assert.Equal(t, 0, stats.ActiveSubscribers)
}

func TestStreamPublisher_Status(t *testing.T) {
	ctx := context.Background()
	config := DefaultStreamPublisherConfig()
	logger := logrus.NewEntry(logrus.New())

	sp := NewStreamPublisher(ctx, config, logger)

	status := sp.GetStatus()
	assert.False(t, status["running"].(bool))
	assert.Equal(t, 0, status["total_subscribers"].(int))
	assert.Equal(t, 0, status["active_subscribers"].(int))

	// Start the publisher
	require.NoError(t, sp.Start())
	defer sp.Stop()

	status = sp.GetStatus()
	assert.True(t, status["running"].(bool))
}

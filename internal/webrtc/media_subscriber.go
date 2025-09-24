package webrtc

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
)

// WebRTCMediaSubscriber implements the VideoStreamSubscriber and AudioStreamSubscriber interfaces
// This replaces the GStreamer Bridge by directly subscribing to the GStreamer stream publisher
type WebRTCMediaSubscriber struct {
	id          uint64
	mediaStream MediaStream
	logger      *logrus.Entry

	// Statistics
	videoFramesReceived uint64
	audioFramesReceived uint64
	videoErrors         uint64
	audioErrors         uint64
	lastVideoFrame      time.Time
	lastAudioFrame      time.Time

	// State management
	active bool
	mutex  sync.RWMutex

	// Configuration
	videoClockRate uint32
	audioClockRate uint32
}

// NewWebRTCMediaSubscriber creates a new WebRTC media subscriber
func NewWebRTCMediaSubscriber(id uint64, mediaStream MediaStream, logger *logrus.Entry) *WebRTCMediaSubscriber {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}

	return &WebRTCMediaSubscriber{
		id:             id,
		mediaStream:    mediaStream,
		logger:         logger.WithField("subscriber_id", id),
		active:         true,
		videoClockRate: 90000, // Standard video clock rate
		audioClockRate: 48000, // Standard audio clock rate
	}
}

// GetSubscriberID returns the unique subscriber ID
func (w *WebRTCMediaSubscriber) GetSubscriberID() uint64 {
	return w.id
}

// IsActive returns whether the subscriber is active
func (w *WebRTCMediaSubscriber) IsActive() bool {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	return w.active
}

// SetActive sets the active state of the subscriber
func (w *WebRTCMediaSubscriber) SetActive(active bool) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.active = active
}

// OnVideoFrame processes a video frame from the GStreamer stream publisher
// This method replaces the bridge's video sample processing
func (w *WebRTCMediaSubscriber) OnVideoFrame(frame *gstreamer.EncodedVideoFrame) error {
	if !w.IsActive() {
		return fmt.Errorf("subscriber %d is not active", w.id)
	}

	if frame == nil {
		w.logger.Warn("Received nil video frame")
		return fmt.Errorf("received nil video frame")
	}

	w.logger.Tracef("Processing video frame: size=%d bytes, codec=%s, dimensions=%dx%d, keyframe=%v",
		len(frame.Data), frame.Codec, frame.Width, frame.Height, frame.KeyFrame)

	// Convert GStreamer frame to WebRTC media sample
	webrtcSample := media.Sample{
		Data:      frame.Data,
		Duration:  frame.Duration,
		Timestamp: frame.Timestamp,
	}

	// Write to WebRTC media stream (no format conversion needed - GStreamer already provides WebRTC-compatible format)
	if err := w.mediaStream.WriteVideo(webrtcSample); err != nil {
		atomic.AddUint64(&w.videoErrors, 1)
		w.logger.Warnf("Failed to write video frame to WebRTC stream: %v", err)
		return fmt.Errorf("failed to write video frame: %w", err)
	}

	// Update statistics
	atomic.AddUint64(&w.videoFramesReceived, 1)
	w.mutex.Lock()
	w.lastVideoFrame = time.Now()
	w.mutex.Unlock()

	w.logger.Tracef("Video frame processed successfully")
	return nil
}

// OnAudioFrame processes an audio frame from the GStreamer stream publisher
// This method replaces the bridge's audio sample processing
func (w *WebRTCMediaSubscriber) OnAudioFrame(frame *gstreamer.EncodedAudioFrame) error {
	if !w.IsActive() {
		return fmt.Errorf("subscriber %d is not active", w.id)
	}

	if frame == nil {
		w.logger.Warn("Received nil audio frame")
		return fmt.Errorf("received nil audio frame")
	}

	w.logger.Tracef("Processing audio frame: size=%d bytes, codec=%s, channels=%d, sample_rate=%d",
		len(frame.Data), frame.Codec, frame.Channels, frame.SampleRate)

	// Convert GStreamer frame to WebRTC media sample
	webrtcSample := media.Sample{
		Data:      frame.Data,
		Duration:  frame.Duration,
		Timestamp: frame.Timestamp,
	}

	// Write to WebRTC media stream (no format conversion needed - GStreamer already provides WebRTC-compatible format)
	if err := w.mediaStream.WriteAudio(webrtcSample); err != nil {
		atomic.AddUint64(&w.audioErrors, 1)
		w.logger.Warnf("Failed to write audio frame to WebRTC stream: %v", err)
		return fmt.Errorf("failed to write audio frame: %w", err)
	}

	// Update statistics
	atomic.AddUint64(&w.audioFramesReceived, 1)
	w.mutex.Lock()
	w.lastAudioFrame = time.Now()
	w.mutex.Unlock()

	w.logger.Tracef("Audio frame processed successfully")
	return nil
}

// OnVideoError handles video processing errors
func (w *WebRTCMediaSubscriber) OnVideoError(err error) error {
	atomic.AddUint64(&w.videoErrors, 1)
	w.logger.Warnf("Video processing error: %v", err)

	// For now, just log the error. In the future, we might implement error recovery
	return nil
}

// OnAudioError handles audio processing errors
func (w *WebRTCMediaSubscriber) OnAudioError(err error) error {
	atomic.AddUint64(&w.audioErrors, 1)
	w.logger.Warnf("Audio processing error: %v", err)

	// For now, just log the error. In the future, we might implement error recovery
	return nil
}

// GetStatistics returns subscriber statistics
func (w *WebRTCMediaSubscriber) GetStatistics() map[string]interface{} {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	return map[string]interface{}{
		"id":                    w.id,
		"active":                w.active,
		"video_frames_received": atomic.LoadUint64(&w.videoFramesReceived),
		"audio_frames_received": atomic.LoadUint64(&w.audioFramesReceived),
		"video_errors":          atomic.LoadUint64(&w.videoErrors),
		"audio_errors":          atomic.LoadUint64(&w.audioErrors),
		"last_video_frame":      w.lastVideoFrame,
		"last_audio_frame":      w.lastAudioFrame,
	}
}

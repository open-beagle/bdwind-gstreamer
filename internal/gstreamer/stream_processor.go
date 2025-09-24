package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// StreamProcessor handles the publish-subscribe mechanism for media streams
// This replaces the complex callback chains with clear channel communication
type StreamProcessor struct {
	// Context and lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *logrus.Entry

	// State management
	mu        sync.RWMutex
	isRunning bool

	// Stream channels for type-safe transmission
	videoStreamChan chan *EncodedVideoStream
	audioStreamChan chan *EncodedAudioStream
	errorChan       chan error

	// Subscriber management with dynamic subscription support
	subscribers   []MediaStreamSubscriber
	subscribersMu sync.RWMutex
	nextSubID     uint64
	subscriberIDs map[MediaStreamSubscriber]uint64

	// Stream processing statistics
	stats *StreamProcessorStats

	// Configuration
	config StreamProcessorConfig
}

// StreamProcessorConfig configuration for the stream processor
type StreamProcessorConfig struct {
	// Channel buffer sizes
	VideoChannelSize int
	AudioChannelSize int
	ErrorChannelSize int

	// Processing timeouts
	PublishTimeout    time.Duration
	SubscriberTimeout time.Duration

	// Error handling
	MaxRetries          int
	RetryDelay          time.Duration
	DropOnChannelFull   bool
	LogDroppedFrames    bool
	MaxSubscriberErrors int
	RemoveFailedSubs    bool

	// Performance monitoring
	EnableMetrics    bool
	MetricsInterval  time.Duration
	LogStatsInterval time.Duration
}

// StreamProcessorStats tracks stream processing statistics
type StreamProcessorStats struct {
	mu sync.RWMutex

	// Stream counts
	VideoFramesProcessed int64
	AudioFramesProcessed int64
	TotalFramesDropped   int64
	VideoFramesDropped   int64
	AudioFramesDropped   int64

	// Subscriber stats
	ActiveSubscribers  int
	TotalSubscribers   int64
	RemovedSubscribers int64
	SubscriberErrors   int64

	// Performance metrics
	AvgVideoProcessTime time.Duration
	AvgAudioProcessTime time.Duration
	LastProcessTime     time.Time
	StartTime           time.Time

	// Error tracking
	PublishErrors    int64
	SubscriberFails  int64
	ChannelFullCount int64
}

// DefaultStreamProcessorConfig returns default configuration
func DefaultStreamProcessorConfig() StreamProcessorConfig {
	return StreamProcessorConfig{
		VideoChannelSize:    100,
		AudioChannelSize:    100,
		ErrorChannelSize:    50,
		PublishTimeout:      100 * time.Millisecond,
		SubscriberTimeout:   50 * time.Millisecond,
		MaxRetries:          3,
		RetryDelay:          10 * time.Millisecond,
		DropOnChannelFull:   true,
		LogDroppedFrames:    true,
		MaxSubscriberErrors: 10,
		RemoveFailedSubs:    true,
		EnableMetrics:       true,
		MetricsInterval:     30 * time.Second,
		LogStatsInterval:    60 * time.Second,
	}
}

// NewStreamProcessor creates a new stream processor
func NewStreamProcessor(ctx context.Context, config StreamProcessorConfig, logger *logrus.Entry) *StreamProcessor {
	processorCtx, cancel := context.WithCancel(ctx)

	sp := &StreamProcessor{
		ctx:             processorCtx,
		cancel:          cancel,
		logger:          logger.WithField("component", "stream-processor"),
		config:          config,
		subscribers:     make([]MediaStreamSubscriber, 0),
		subscriberIDs:   make(map[MediaStreamSubscriber]uint64),
		videoStreamChan: make(chan *EncodedVideoStream, config.VideoChannelSize),
		audioStreamChan: make(chan *EncodedAudioStream, config.AudioChannelSize),
		errorChan:       make(chan error, config.ErrorChannelSize),
		stats: &StreamProcessorStats{
			StartTime: time.Now(),
		},
	}

	return sp
}

// Start starts the stream processor
func (sp *StreamProcessor) Start() error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if sp.isRunning {
		return fmt.Errorf("stream processor already running")
	}

	sp.isRunning = true

	// Start video stream processing goroutine
	sp.wg.Add(1)
	go sp.processVideoStreams()

	// Start audio stream processing goroutine
	sp.wg.Add(1)
	go sp.processAudioStreams()

	// Start error handling goroutine
	sp.wg.Add(1)
	go sp.processErrors()

	// Start metrics collection if enabled
	if sp.config.EnableMetrics {
		sp.wg.Add(1)
		go sp.collectMetrics()
	}

	// Start statistics logging if enabled
	if sp.config.LogStatsInterval > 0 {
		sp.wg.Add(1)
		go sp.logStatistics()
	}

	sp.logger.Info("Stream processor started successfully")
	return nil
}

// Stop stops the stream processor
func (sp *StreamProcessor) Stop() error {
	sp.mu.Lock()
	if !sp.isRunning {
		sp.mu.Unlock()
		return nil
	}
	sp.isRunning = false
	sp.mu.Unlock()

	sp.logger.Info("Stopping stream processor...")

	// Cancel context to stop all goroutines
	sp.cancel()

	// Wait for all goroutines to finish
	sp.wg.Wait()

	// Close channels
	close(sp.videoStreamChan)
	close(sp.audioStreamChan)
	close(sp.errorChan)

	// Clear subscribers
	sp.subscribersMu.Lock()
	sp.subscribers = nil
	sp.subscriberIDs = nil
	sp.subscribersMu.Unlock()

	sp.logger.Info("Stream processor stopped successfully")
	return nil
}

// IsRunning returns whether the processor is running
func (sp *StreamProcessor) IsRunning() bool {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return sp.isRunning
}

// AddSubscriber adds a new media stream subscriber with dynamic subscription support
func (sp *StreamProcessor) AddSubscriber(subscriber MediaStreamSubscriber) uint64 {
	sp.subscribersMu.Lock()
	defer sp.subscribersMu.Unlock()

	// Assign unique ID
	sp.nextSubID++
	subID := sp.nextSubID

	// Add to subscribers list
	sp.subscribers = append(sp.subscribers, subscriber)
	sp.subscriberIDs[subscriber] = subID

	// Update statistics
	sp.stats.mu.Lock()
	sp.stats.ActiveSubscribers = len(sp.subscribers)
	sp.stats.TotalSubscribers++
	sp.stats.mu.Unlock()

	sp.logger.Infof("Added subscriber (ID: %d), total active: %d", subID, len(sp.subscribers))
	return subID
}

// RemoveSubscriber removes a media stream subscriber
func (sp *StreamProcessor) RemoveSubscriber(subscriber MediaStreamSubscriber) bool {
	sp.subscribersMu.Lock()
	defer sp.subscribersMu.Unlock()

	// Find subscriber
	subID, exists := sp.subscriberIDs[subscriber]
	if !exists {
		return false
	}

	// Remove from subscribers list
	for i, sub := range sp.subscribers {
		if sub == subscriber {
			sp.subscribers = append(sp.subscribers[:i], sp.subscribers[i+1:]...)
			break
		}
	}

	// Remove from ID map
	delete(sp.subscriberIDs, subscriber)

	// Update statistics
	sp.stats.mu.Lock()
	sp.stats.ActiveSubscribers = len(sp.subscribers)
	sp.stats.RemovedSubscribers++
	sp.stats.mu.Unlock()

	sp.logger.Infof("Removed subscriber (ID: %d), remaining active: %d", subID, len(sp.subscribers))
	return true
}

// GetSubscriberCount returns the current number of active subscribers
func (sp *StreamProcessor) GetSubscriberCount() int {
	sp.subscribersMu.RLock()
	defer sp.subscribersMu.RUnlock()
	return len(sp.subscribers)
}

// PublishVideoStream publishes a video stream to all subscribers (non-blocking)
func (sp *StreamProcessor) PublishVideoStream(stream *EncodedVideoStream) error {
	if stream == nil {
		return fmt.Errorf("video stream is nil")
	}

	select {
	case sp.videoStreamChan <- stream:
		return nil
	case <-time.After(sp.config.PublishTimeout):
		if sp.config.DropOnChannelFull {
			sp.stats.mu.Lock()
			sp.stats.VideoFramesDropped++
			sp.stats.TotalFramesDropped++
			sp.stats.ChannelFullCount++
			sp.stats.mu.Unlock()

			if sp.config.LogDroppedFrames {
				sp.logger.Warn("Dropped video frame due to full channel")
			}
			return nil // Don't return error for dropped frames
		}
		return fmt.Errorf("video stream channel full")
	case <-sp.ctx.Done():
		return fmt.Errorf("stream processor stopped")
	}
}

// PublishAudioStream publishes an audio stream to all subscribers (non-blocking)
func (sp *StreamProcessor) PublishAudioStream(stream *EncodedAudioStream) error {
	if stream == nil {
		return fmt.Errorf("audio stream is nil")
	}

	select {
	case sp.audioStreamChan <- stream:
		return nil
	case <-time.After(sp.config.PublishTimeout):
		if sp.config.DropOnChannelFull {
			sp.stats.mu.Lock()
			sp.stats.AudioFramesDropped++
			sp.stats.TotalFramesDropped++
			sp.stats.ChannelFullCount++
			sp.stats.mu.Unlock()

			if sp.config.LogDroppedFrames {
				sp.logger.Warn("Dropped audio frame due to full channel")
			}
			return nil // Don't return error for dropped frames
		}
		return fmt.Errorf("audio stream channel full")
	case <-sp.ctx.Done():
		return fmt.Errorf("stream processor stopped")
	}
}

// processVideoStreams processes video streams from the channel
func (sp *StreamProcessor) processVideoStreams() {
	defer sp.wg.Done()

	sp.logger.Debug("Video stream processing started")

	for {
		select {
		case videoStream := <-sp.videoStreamChan:
			if videoStream != nil {
				startTime := time.Now()
				sp.publishVideoToSubscribers(videoStream)

				// Update processing time statistics
				processingTime := time.Since(startTime)
				sp.updateVideoProcessingStats(processingTime)
			}

		case <-sp.ctx.Done():
			sp.logger.Debug("Video stream processing stopped")
			return
		}
	}
}

// processAudioStreams processes audio streams from the channel
func (sp *StreamProcessor) processAudioStreams() {
	defer sp.wg.Done()

	sp.logger.Debug("Audio stream processing started")

	for {
		select {
		case audioStream := <-sp.audioStreamChan:
			if audioStream != nil {
				startTime := time.Now()
				sp.publishAudioToSubscribers(audioStream)

				// Update processing time statistics
				processingTime := time.Since(startTime)
				sp.updateAudioProcessingStats(processingTime)
			}

		case <-sp.ctx.Done():
			sp.logger.Debug("Audio stream processing stopped")
			return
		}
	}
}

// processErrors processes errors from the error channel
func (sp *StreamProcessor) processErrors() {
	defer sp.wg.Done()

	sp.logger.Debug("Error processing started")

	for {
		select {
		case err := <-sp.errorChan:
			if err != nil {
				sp.logger.Errorf("Stream processing error: %v", err)
				sp.stats.mu.Lock()
				sp.stats.PublishErrors++
				sp.stats.mu.Unlock()
			}

		case <-sp.ctx.Done():
			sp.logger.Debug("Error processing stopped")
			return
		}
	}
}

// publishVideoToSubscribers publishes video stream to all subscribers with error handling
func (sp *StreamProcessor) publishVideoToSubscribers(stream *EncodedVideoStream) {
	sp.subscribersMu.RLock()
	subscribers := make([]MediaStreamSubscriber, len(sp.subscribers))
	copy(subscribers, sp.subscribers)
	sp.subscribersMu.RUnlock()

	if len(subscribers) == 0 {
		return
	}

	// Track failed subscribers for potential removal
	var failedSubscribers []MediaStreamSubscriber

	// Publish to all subscribers with timeout
	for _, subscriber := range subscribers {
		done := make(chan error, 1)

		go func(sub MediaStreamSubscriber) {
			done <- sub.OnVideoStream(stream)
		}(subscriber)

		select {
		case err := <-done:
			if err != nil {
				sp.logger.Warnf("Subscriber failed to process video stream: %v", err)
				sp.stats.mu.Lock()
				sp.stats.SubscriberErrors++
				sp.stats.mu.Unlock()

				// Notify subscriber of error
				subscriber.OnStreamError(err)

				if sp.config.RemoveFailedSubs {
					failedSubscribers = append(failedSubscribers, subscriber)
				}
			}

		case <-time.After(sp.config.SubscriberTimeout):
			sp.logger.Warn("Subscriber timed out processing video stream")
			sp.stats.mu.Lock()
			sp.stats.SubscriberFails++
			sp.stats.mu.Unlock()

			if sp.config.RemoveFailedSubs {
				failedSubscribers = append(failedSubscribers, subscriber)
			}

		case <-sp.ctx.Done():
			return
		}
	}

	// Remove failed subscribers if configured
	for _, failedSub := range failedSubscribers {
		sp.RemoveSubscriber(failedSub)
	}

	// Update statistics
	sp.stats.mu.Lock()
	sp.stats.VideoFramesProcessed++
	sp.stats.LastProcessTime = time.Now()
	sp.stats.mu.Unlock()
}

// publishAudioToSubscribers publishes audio stream to all subscribers with error handling
func (sp *StreamProcessor) publishAudioToSubscribers(stream *EncodedAudioStream) {
	sp.subscribersMu.RLock()
	subscribers := make([]MediaStreamSubscriber, len(sp.subscribers))
	copy(subscribers, sp.subscribers)
	sp.subscribersMu.RUnlock()

	if len(subscribers) == 0 {
		return
	}

	// Track failed subscribers for potential removal
	var failedSubscribers []MediaStreamSubscriber

	// Publish to all subscribers with timeout
	for _, subscriber := range subscribers {
		done := make(chan error, 1)

		go func(sub MediaStreamSubscriber) {
			done <- sub.OnAudioStream(stream)
		}(subscriber)

		select {
		case err := <-done:
			if err != nil {
				sp.logger.Warnf("Subscriber failed to process audio stream: %v", err)
				sp.stats.mu.Lock()
				sp.stats.SubscriberErrors++
				sp.stats.mu.Unlock()

				// Notify subscriber of error
				subscriber.OnStreamError(err)

				if sp.config.RemoveFailedSubs {
					failedSubscribers = append(failedSubscribers, subscriber)
				}
			}

		case <-time.After(sp.config.SubscriberTimeout):
			sp.logger.Warn("Subscriber timed out processing audio stream")
			sp.stats.mu.Lock()
			sp.stats.SubscriberFails++
			sp.stats.mu.Unlock()

			if sp.config.RemoveFailedSubs {
				failedSubscribers = append(failedSubscribers, subscriber)
			}

		case <-sp.ctx.Done():
			return
		}
	}

	// Remove failed subscribers if configured
	for _, failedSub := range failedSubscribers {
		sp.RemoveSubscriber(failedSub)
	}

	// Update statistics
	sp.stats.mu.Lock()
	sp.stats.AudioFramesProcessed++
	sp.stats.LastProcessTime = time.Now()
	sp.stats.mu.Unlock()
}

// updateVideoProcessingStats updates video processing time statistics
func (sp *StreamProcessor) updateVideoProcessingStats(processingTime time.Duration) {
	sp.stats.mu.Lock()
	defer sp.stats.mu.Unlock()

	// Simple moving average for processing time
	if sp.stats.AvgVideoProcessTime == 0 {
		sp.stats.AvgVideoProcessTime = processingTime
	} else {
		sp.stats.AvgVideoProcessTime = (sp.stats.AvgVideoProcessTime + processingTime) / 2
	}
}

// updateAudioProcessingStats updates audio processing time statistics
func (sp *StreamProcessor) updateAudioProcessingStats(processingTime time.Duration) {
	sp.stats.mu.Lock()
	defer sp.stats.mu.Unlock()

	// Simple moving average for processing time
	if sp.stats.AvgAudioProcessTime == 0 {
		sp.stats.AvgAudioProcessTime = processingTime
	} else {
		sp.stats.AvgAudioProcessTime = (sp.stats.AvgAudioProcessTime + processingTime) / 2
	}
}

// collectMetrics collects and logs performance metrics
func (sp *StreamProcessor) collectMetrics() {
	defer sp.wg.Done()

	ticker := time.NewTicker(sp.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sp.logMetrics()

		case <-sp.ctx.Done():
			sp.logger.Debug("Metrics collection stopped")
			return
		}
	}
}

// logStatistics logs processing statistics
func (sp *StreamProcessor) logStatistics() {
	defer sp.wg.Done()

	ticker := time.NewTicker(sp.config.LogStatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sp.logStats()

		case <-sp.ctx.Done():
			sp.logger.Debug("Statistics logging stopped")
			return
		}
	}
}

// logMetrics logs current performance metrics
func (sp *StreamProcessor) logMetrics() {
	sp.stats.mu.RLock()
	stats := *sp.stats // Copy stats
	sp.stats.mu.RUnlock()

	uptime := time.Since(stats.StartTime)

	sp.logger.WithFields(logrus.Fields{
		"uptime_seconds":         uptime.Seconds(),
		"video_frames_processed": stats.VideoFramesProcessed,
		"audio_frames_processed": stats.AudioFramesProcessed,
		"frames_dropped":         stats.TotalFramesDropped,
		"active_subscribers":     stats.ActiveSubscribers,
		"subscriber_errors":      stats.SubscriberErrors,
		"avg_video_process_ms":   stats.AvgVideoProcessTime.Milliseconds(),
		"avg_audio_process_ms":   stats.AvgAudioProcessTime.Milliseconds(),
	}).Info("Stream processor metrics")
}

// logStats logs detailed processing statistics
func (sp *StreamProcessor) logStats() {
	sp.stats.mu.RLock()
	stats := *sp.stats // Copy stats
	sp.stats.mu.RUnlock()

	sp.subscribersMu.RLock()
	activeSubscribers := len(sp.subscribers)
	sp.subscribersMu.RUnlock()

	uptime := time.Since(stats.StartTime)
	videoFPS := float64(stats.VideoFramesProcessed) / uptime.Seconds()
	audioFPS := float64(stats.AudioFramesProcessed) / uptime.Seconds()

	sp.logger.Infof("Stream Processor Statistics:")
	sp.logger.Infof("  Uptime: %v", uptime)
	sp.logger.Infof("  Video: %d frames (%.2f fps), %d dropped",
		stats.VideoFramesProcessed, videoFPS, stats.VideoFramesDropped)
	sp.logger.Infof("  Audio: %d frames (%.2f fps), %d dropped",
		stats.AudioFramesProcessed, audioFPS, stats.AudioFramesDropped)
	sp.logger.Infof("  Subscribers: %d active, %d total, %d removed",
		activeSubscribers, stats.TotalSubscribers, stats.RemovedSubscribers)
	sp.logger.Infof("  Errors: %d publish, %d subscriber, %d timeouts",
		stats.PublishErrors, stats.SubscriberErrors, stats.SubscriberFails)
	sp.logger.Infof("  Performance: %.2fms avg video, %.2fms avg audio",
		float64(stats.AvgVideoProcessTime.Microseconds())/1000.0,
		float64(stats.AvgAudioProcessTime.Microseconds())/1000.0)
}

// GetStats returns current stream processor statistics
func (sp *StreamProcessor) GetStats() StreamProcessorStats {
	sp.stats.mu.RLock()
	defer sp.stats.mu.RUnlock()

	sp.subscribersMu.RLock()
	activeSubscribers := len(sp.subscribers)
	sp.subscribersMu.RUnlock()

	stats := *sp.stats // Copy stats
	stats.ActiveSubscribers = activeSubscribers
	return stats
}

// PublishError publishes an error to the error channel
func (sp *StreamProcessor) PublishError(err error) {
	if err == nil {
		return
	}

	select {
	case sp.errorChan <- err:
		// Error published successfully
	default:
		// Error channel full, log directly
		sp.logger.Errorf("Error channel full, logging directly: %v", err)
	}
}

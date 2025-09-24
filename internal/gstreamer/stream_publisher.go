package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// StreamPublisherConfig configuration for the stream publisher
type StreamPublisherConfig struct {
	VideoBufferSize     int           `json:"video_buffer_size"`
	AudioBufferSize     int           `json:"audio_buffer_size"`
	MaxSubscribers      int           `json:"max_subscribers"`
	PublishTimeout      time.Duration `json:"publish_timeout"`
	EnableMetrics       bool          `json:"enable_metrics"`
	MetricsInterval     time.Duration `json:"metrics_interval"`
	LogProcessingErrors bool          `json:"log_processing_errors"`
}

// DefaultStreamPublisherConfig returns default configuration
func DefaultStreamPublisherConfig() *StreamPublisherConfig {
	return &StreamPublisherConfig{
		VideoBufferSize:     100,
		AudioBufferSize:     100,
		MaxSubscribers:      10,
		PublishTimeout:      1 * time.Second,
		EnableMetrics:       true,
		LogProcessingErrors: false,
		MetricsInterval:     30 * time.Second,
	}
}

// EncodedVideoFrame represents an encoded video frame with metadata
type EncodedVideoFrame struct {
	Data        []byte        `json:"data"`
	Timestamp   time.Time     `json:"timestamp"`
	Duration    time.Duration `json:"duration"`
	KeyFrame    bool          `json:"key_frame"`
	Width       int           `json:"width"`
	Height      int           `json:"height"`
	Codec       string        `json:"codec"`
	Bitrate     int           `json:"bitrate"`
	FrameNumber uint64        `json:"frame_number"`
	PTS         uint64        `json:"pts"`
	DTS         uint64        `json:"dts"`
}

// EncodedAudioFrame represents an encoded audio frame with metadata
type EncodedAudioFrame struct {
	Data        []byte        `json:"data"`
	Timestamp   time.Time     `json:"timestamp"`
	Duration    time.Duration `json:"duration"`
	SampleRate  int           `json:"sample_rate"`
	Channels    int           `json:"channels"`
	Codec       string        `json:"codec"`
	Bitrate     int           `json:"bitrate"`
	FrameNumber uint64        `json:"frame_number"`
	PTS         uint64        `json:"pts"`
}

// VideoStreamSubscriber interface for video stream subscribers
type VideoStreamSubscriber interface {
	OnVideoFrame(frame *EncodedVideoFrame) error
	OnVideoError(err error) error
	GetSubscriberID() uint64
	IsActive() bool
}

// AudioStreamSubscriber interface for audio stream subscribers
type AudioStreamSubscriber interface {
	OnAudioFrame(frame *EncodedAudioFrame) error
	OnAudioError(err error) error
	GetSubscriberID() uint64
	IsActive() bool
}

// CombinedMediaStreamSubscriber interface for combined media stream subscribers
type CombinedMediaStreamSubscriber interface {
	VideoStreamSubscriber
	AudioStreamSubscriber
	OnStreamStart() error
	OnStreamStop() error
}

// SubscriberInfo holds information about a subscriber
type SubscriberInfo struct {
	ID           uint64    `json:"id"`
	Type         string    `json:"type"`
	RegisteredAt time.Time `json:"registered_at"`
	LastActivity time.Time `json:"last_activity"`
	FramesSent   uint64    `json:"frames_sent"`
	ErrorCount   uint64    `json:"error_count"`
	Active       bool      `json:"active"`
}

// StreamStatistics holds statistics about the stream publisher
type StreamStatistics struct {
	VideoFramesPublished  uint64            `json:"video_frames_published"`
	AudioFramesPublished  uint64            `json:"audio_frames_published"`
	VideoFramesDropped    uint64            `json:"video_frames_dropped"`
	AudioFramesDropped    uint64            `json:"audio_frames_dropped"`
	VideoSubscribers      int               `json:"video_subscribers"`
	AudioSubscribers      int               `json:"audio_subscribers"`
	TotalSubscribers      int               `json:"total_subscribers"`
	ActiveSubscribers     int               `json:"active_subscribers"`
	PublishErrors         uint64            `json:"publish_errors"`
	LastVideoFrame        time.Time         `json:"last_video_frame"`
	LastAudioFrame        time.Time         `json:"last_audio_frame"`
	AverageVideoFrameSize int64             `json:"average_video_frame_size"`
	AverageAudioFrameSize int64             `json:"average_audio_frame_size"`
	Subscribers           []*SubscriberInfo `json:"subscribers"`
}

// StreamPublisher manages media stream publishing using publish-subscribe pattern
// This component replaces complex callback chains with a simple pub-sub model
type StreamPublisher struct {
	config *StreamPublisherConfig
	logger *logrus.Entry

	// Video stream management
	videoChannel     chan *EncodedVideoFrame
	videoSubscribers map[uint64]VideoStreamSubscriber
	videoMutex       sync.RWMutex

	// Audio stream management
	audioChannel     chan *EncodedAudioFrame
	audioSubscribers map[uint64]AudioStreamSubscriber
	audioMutex       sync.RWMutex

	// Media stream management (for combined video+audio subscribers)
	mediaSubscribers map[uint64]MediaStreamSubscriber
	mediaMutex       sync.RWMutex

	// Subscriber management
	nextSubscriberID uint64
	subscriberInfo   map[uint64]*SubscriberInfo
	infoMutex        sync.RWMutex

	// Statistics
	stats      StreamStatistics
	statsMutex sync.RWMutex

	// Lifecycle management
	ctx      context.Context
	cancel   context.CancelFunc
	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewStreamPublisher creates a new stream publisher instance
func NewStreamPublisher(ctx context.Context, config *StreamPublisherConfig, logger *logrus.Entry) *StreamPublisher {
	if config == nil {
		config = DefaultStreamPublisherConfig()
	}
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}

	childCtx, cancel := context.WithCancel(ctx)

	sp := &StreamPublisher{
		config:           config,
		logger:           logger,
		ctx:              childCtx,
		cancel:           cancel,
		videoChannel:     make(chan *EncodedVideoFrame, config.VideoBufferSize),
		audioChannel:     make(chan *EncodedAudioFrame, config.AudioBufferSize),
		videoSubscribers: make(map[uint64]VideoStreamSubscriber),
		audioSubscribers: make(map[uint64]AudioStreamSubscriber),
		mediaSubscribers: make(map[uint64]MediaStreamSubscriber),
		subscriberInfo:   make(map[uint64]*SubscriberInfo),
		stopChan:         make(chan struct{}),
		stats: StreamStatistics{
			Subscribers: make([]*SubscriberInfo, 0),
		},
	}

	sp.logger.Debug("Stream publisher created successfully")
	return sp
}

// Start starts the stream publisher and its background processing loops
func (sp *StreamPublisher) Start() error {
	if sp.running {
		return fmt.Errorf("stream publisher is already running")
	}

	sp.logger.Debug("Starting stream publisher")

	// Start video publishing goroutine
	sp.wg.Add(1)
	go sp.videoPublishLoop()

	// Start audio publishing goroutine
	sp.wg.Add(1)
	go sp.audioPublishLoop()

	// Start metrics collection if enabled
	if sp.config.EnableMetrics {
		sp.wg.Add(1)
		go sp.metricsLoop()
	}

	// Start subscriber monitoring
	sp.wg.Add(1)
	go sp.subscriberMonitorLoop()

	sp.running = true
	sp.logger.Info("Stream publisher started successfully")
	return nil
}

// Stop stops the stream publisher and waits for all goroutines to finish
func (sp *StreamPublisher) Stop() error {
	if !sp.running {
		return fmt.Errorf("stream publisher is not running")
	}

	sp.logger.Debug("Stopping stream publisher")

	// Cancel context and stop background tasks
	sp.cancel()
	close(sp.stopChan)

	// Close channels to signal shutdown
	close(sp.videoChannel)
	close(sp.audioChannel)

	// Wait for all goroutines to finish
	sp.wg.Wait()

	sp.running = false
	sp.logger.Info("Stream publisher stopped successfully")
	return nil
}

// IsRunning returns whether the stream publisher is currently running
func (sp *StreamPublisher) IsRunning() bool {
	return sp.running
}

// PublishVideoFrame publishes a video frame to all video subscribers
// This method implements asynchronous publishing to avoid blocking media processing
func (sp *StreamPublisher) PublishVideoFrame(frame *EncodedVideoFrame) error {
	if frame == nil {
		return fmt.Errorf("video frame is nil")
	}

	if !sp.running {
		return fmt.Errorf("stream publisher not running")
	}

	select {
	case sp.videoChannel <- frame:
		atomic.AddUint64(&sp.stats.VideoFramesPublished, 1)
		sp.statsMutex.Lock()
		sp.stats.LastVideoFrame = time.Now()

		// Update average frame size
		if sp.stats.VideoFramesPublished > 0 {
			totalSize := sp.stats.AverageVideoFrameSize*int64(sp.stats.VideoFramesPublished-1) + int64(len(frame.Data))
			sp.stats.AverageVideoFrameSize = totalSize / int64(sp.stats.VideoFramesPublished)
		} else {
			sp.stats.AverageVideoFrameSize = int64(len(frame.Data))
		}
		sp.statsMutex.Unlock()
		return nil

	case <-time.After(sp.config.PublishTimeout):
		atomic.AddUint64(&sp.stats.VideoFramesDropped, 1)
		return fmt.Errorf("video frame publish timeout")
	case <-sp.ctx.Done():
		return fmt.Errorf("stream publisher shutting down")
	}
}

// PublishAudioFrame publishes an audio frame to all audio subscribers
// This method implements asynchronous publishing to avoid blocking media processing
func (sp *StreamPublisher) PublishAudioFrame(frame *EncodedAudioFrame) error {
	if frame == nil {
		return fmt.Errorf("audio frame is nil")
	}

	if !sp.running {
		return fmt.Errorf("stream publisher not running")
	}

	select {
	case sp.audioChannel <- frame:
		atomic.AddUint64(&sp.stats.AudioFramesPublished, 1)
		sp.statsMutex.Lock()
		sp.stats.LastAudioFrame = time.Now()

		// Update average frame size
		if sp.stats.AudioFramesPublished > 0 {
			totalSize := sp.stats.AverageAudioFrameSize*int64(sp.stats.AudioFramesPublished-1) + int64(len(frame.Data))
			sp.stats.AverageAudioFrameSize = totalSize / int64(sp.stats.AudioFramesPublished)
		} else {
			sp.stats.AverageAudioFrameSize = int64(len(frame.Data))
		}
		sp.statsMutex.Unlock()
		return nil

	case <-time.After(sp.config.PublishTimeout):
		atomic.AddUint64(&sp.stats.AudioFramesDropped, 1)
		return fmt.Errorf("audio frame publish timeout")
	case <-sp.ctx.Done():
		return fmt.Errorf("stream publisher shutting down")
	}
}

// AddVideoSubscriber adds a video stream subscriber and returns its unique ID
func (sp *StreamPublisher) AddVideoSubscriber(subscriber VideoStreamSubscriber) (uint64, error) {
	if subscriber == nil {
		return 0, fmt.Errorf("subscriber is nil")
	}

	sp.videoMutex.Lock()
	defer sp.videoMutex.Unlock()

	if len(sp.videoSubscribers) >= sp.config.MaxSubscribers {
		return 0, fmt.Errorf("maximum number of subscribers reached")
	}

	// Generate unique ID
	atomic.AddUint64(&sp.nextSubscriberID, 1)
	id := sp.nextSubscriberID

	// Add subscriber
	sp.videoSubscribers[id] = subscriber

	// Create subscriber info
	info := &SubscriberInfo{
		ID:           id,
		Type:         "video",
		RegisteredAt: time.Now(),
		LastActivity: time.Now(),
		Active:       true,
	}

	sp.infoMutex.Lock()
	sp.subscriberInfo[id] = info
	sp.infoMutex.Unlock()

	// Update statistics
	sp.updateSubscriberStats()

	sp.logger.Infof("Added video subscriber with ID %d", id)
	return id, nil
}

// RemoveVideoSubscriber removes a video stream subscriber by ID
func (sp *StreamPublisher) RemoveVideoSubscriber(id uint64) bool {
	sp.videoMutex.Lock()
	defer sp.videoMutex.Unlock()

	if _, exists := sp.videoSubscribers[id]; !exists {
		return false
	}

	delete(sp.videoSubscribers, id)

	// Remove subscriber info
	sp.infoMutex.Lock()
	delete(sp.subscriberInfo, id)
	sp.infoMutex.Unlock()

	// Update statistics
	sp.updateSubscriberStats()

	sp.logger.Infof("Removed video subscriber with ID %d", id)
	return true
}

// AddAudioSubscriber adds an audio stream subscriber and returns its unique ID
func (sp *StreamPublisher) AddAudioSubscriber(subscriber AudioStreamSubscriber) (uint64, error) {
	if subscriber == nil {
		return 0, fmt.Errorf("subscriber is nil")
	}

	sp.audioMutex.Lock()
	defer sp.audioMutex.Unlock()

	if len(sp.audioSubscribers) >= sp.config.MaxSubscribers {
		return 0, fmt.Errorf("maximum number of subscribers reached")
	}

	// Generate unique ID
	atomic.AddUint64(&sp.nextSubscriberID, 1)
	id := sp.nextSubscriberID

	// Add subscriber
	sp.audioSubscribers[id] = subscriber

	// Create subscriber info
	info := &SubscriberInfo{
		ID:           id,
		Type:         "audio",
		RegisteredAt: time.Now(),
		LastActivity: time.Now(),
		Active:       true,
	}

	sp.infoMutex.Lock()
	sp.subscriberInfo[id] = info
	sp.infoMutex.Unlock()

	// Update statistics
	sp.updateSubscriberStats()

	sp.logger.Infof("Added audio subscriber with ID %d", id)
	return id, nil
}

// RemoveAudioSubscriber removes an audio stream subscriber by ID
func (sp *StreamPublisher) RemoveAudioSubscriber(id uint64) bool {
	sp.audioMutex.Lock()
	defer sp.audioMutex.Unlock()

	if _, exists := sp.audioSubscribers[id]; !exists {
		return false
	}

	delete(sp.audioSubscribers, id)

	// Remove subscriber info
	sp.infoMutex.Lock()
	delete(sp.subscriberInfo, id)
	sp.infoMutex.Unlock()

	// Update statistics
	sp.updateSubscriberStats()

	sp.logger.Infof("Removed audio subscriber with ID %d", id)
	return true
}

// AddMediaSubscriber adds a media stream subscriber and returns its unique ID
func (sp *StreamPublisher) AddMediaSubscriber(subscriber MediaStreamSubscriber) (uint64, error) {
	if subscriber == nil {
		return 0, fmt.Errorf("subscriber is nil")
	}

	sp.mediaMutex.Lock()
	defer sp.mediaMutex.Unlock()

	if len(sp.mediaSubscribers) >= sp.config.MaxSubscribers {
		return 0, fmt.Errorf("maximum number of subscribers reached")
	}

	// Generate unique ID
	atomic.AddUint64(&sp.nextSubscriberID, 1)
	id := sp.nextSubscriberID

	// Add subscriber
	sp.mediaSubscribers[id] = subscriber

	// Create subscriber info
	info := &SubscriberInfo{
		ID:           id,
		Type:         "media",
		RegisteredAt: time.Now(),
		LastActivity: time.Now(),
		Active:       true,
	}

	sp.infoMutex.Lock()
	sp.subscriberInfo[id] = info
	sp.infoMutex.Unlock()

	// Update statistics
	sp.updateSubscriberStats()

	// Notify subscriber of stream start
	if err := subscriber.OnStreamStart(); err != nil {
		sp.logger.Warnf("Error notifying subscriber %d of stream start: %v", id, err)
	}

	sp.logger.Infof("Added media subscriber with ID %d", id)
	return id, nil
}

// RemoveMediaSubscriber removes a media stream subscriber by ID
func (sp *StreamPublisher) RemoveMediaSubscriber(id uint64) bool {
	sp.mediaMutex.Lock()
	defer sp.mediaMutex.Unlock()

	subscriber, exists := sp.mediaSubscribers[id]
	if !exists {
		return false
	}

	// Notify subscriber of stream stop
	if err := subscriber.OnStreamStop(); err != nil {
		sp.logger.Warnf("Error notifying subscriber %d of stream stop: %v", id, err)
	}

	delete(sp.mediaSubscribers, id)

	// Remove subscriber info
	sp.infoMutex.Lock()
	delete(sp.subscriberInfo, id)
	sp.infoMutex.Unlock()

	// Update statistics
	sp.updateSubscriberStats()

	sp.logger.Infof("Removed media subscriber with ID %d", id)
	return true
}

// GetSubscriberCount returns the total number of subscribers
func (sp *StreamPublisher) GetSubscriberCount() int {
	sp.videoMutex.RLock()
	videoCount := len(sp.videoSubscribers)
	sp.videoMutex.RUnlock()

	sp.audioMutex.RLock()
	audioCount := len(sp.audioSubscribers)
	sp.audioMutex.RUnlock()

	sp.mediaMutex.RLock()
	mediaCount := len(sp.mediaSubscribers)
	sp.mediaMutex.RUnlock()

	return videoCount + audioCount + mediaCount
}

// GetStatistics returns current stream publisher statistics
func (sp *StreamPublisher) GetStatistics() StreamStatistics {
	sp.statsMutex.RLock()
	defer sp.statsMutex.RUnlock()

	// Update subscriber information
	sp.updateSubscriberStatsLocked()

	return sp.stats
}

// GetStatus returns current status information as a map
func (sp *StreamPublisher) GetStatus() map[string]interface{} {
	stats := sp.GetStatistics()

	return map[string]interface{}{
		"running":                sp.running,
		"video_buffer_usage":     len(sp.videoChannel),
		"audio_buffer_usage":     len(sp.audioChannel),
		"total_subscribers":      stats.TotalSubscribers,
		"active_subscribers":     stats.ActiveSubscribers,
		"video_subscribers":      stats.VideoSubscribers,
		"audio_subscribers":      stats.AudioSubscribers,
		"video_frames_published": stats.VideoFramesPublished,
		"audio_frames_published": stats.AudioFramesPublished,
		"video_frames_dropped":   stats.VideoFramesDropped,
		"audio_frames_dropped":   stats.AudioFramesDropped,
		"publish_errors":         stats.PublishErrors,
		"last_video_frame":       stats.LastVideoFrame,
		"last_audio_frame":       stats.LastAudioFrame,
	}
}

// Background processing loops

// videoPublishLoop processes video frames and distributes them to subscribers
func (sp *StreamPublisher) videoPublishLoop() {
	defer sp.wg.Done()

	sp.logger.Debug("Video publish loop started")

	for {
		select {
		case frame, ok := <-sp.videoChannel:
			if !ok {
				sp.logger.Debug("Video channel closed, stopping video publish loop")
				return
			}
			sp.distributeVideoFrame(frame)

		case <-sp.ctx.Done():
			sp.logger.Debug("Context cancelled, stopping video publish loop")
			return
		}
	}
}

// audioPublishLoop processes audio frames and distributes them to subscribers
func (sp *StreamPublisher) audioPublishLoop() {
	defer sp.wg.Done()

	sp.logger.Debug("Audio publish loop started")

	for {
		select {
		case frame, ok := <-sp.audioChannel:
			if !ok {
				sp.logger.Debug("Audio channel closed, stopping audio publish loop")
				return
			}
			sp.distributeAudioFrame(frame)

		case <-sp.ctx.Done():
			sp.logger.Debug("Context cancelled, stopping audio publish loop")
			return
		}
	}
}

// distributeVideoFrame distributes a video frame to all video and media subscribers
func (sp *StreamPublisher) distributeVideoFrame(frame *EncodedVideoFrame) {
	// Distribute to video subscribers
	sp.videoMutex.RLock()
	videoSubs := make(map[uint64]VideoStreamSubscriber)
	for id, sub := range sp.videoSubscribers {
		videoSubs[id] = sub
	}
	sp.videoMutex.RUnlock()

	for id, subscriber := range videoSubs {
		go sp.sendVideoFrameToSubscriber(id, subscriber, frame)
	}

	// Distribute to media subscribers
	sp.mediaMutex.RLock()
	mediaSubs := make(map[uint64]MediaStreamSubscriber)
	for id, sub := range sp.mediaSubscribers {
		mediaSubs[id] = sub
	}
	sp.mediaMutex.RUnlock()

	for id, subscriber := range mediaSubs {
		go sp.sendVideoFrameToMediaSubscriber(id, subscriber, frame)
	}
}

// distributeAudioFrame distributes an audio frame to all audio and media subscribers
func (sp *StreamPublisher) distributeAudioFrame(frame *EncodedAudioFrame) {
	// Distribute to audio subscribers
	sp.audioMutex.RLock()
	audioSubs := make(map[uint64]AudioStreamSubscriber)
	for id, sub := range sp.audioSubscribers {
		audioSubs[id] = sub
	}
	sp.audioMutex.RUnlock()

	for id, subscriber := range audioSubs {
		go sp.sendAudioFrameToSubscriber(id, subscriber, frame)
	}

	// Distribute to media subscribers
	sp.mediaMutex.RLock()
	mediaSubs := make(map[uint64]MediaStreamSubscriber)
	for id, sub := range sp.mediaSubscribers {
		mediaSubs[id] = sub
	}
	sp.mediaMutex.RUnlock()

	for id, subscriber := range mediaSubs {
		go sp.sendAudioFrameToMediaSubscriber(id, subscriber, frame)
	}
}

// sendVideoFrameToSubscriber sends a video frame to a specific video subscriber
func (sp *StreamPublisher) sendVideoFrameToSubscriber(id uint64, subscriber VideoStreamSubscriber, frame *EncodedVideoFrame) {
	defer func() {
		if r := recover(); r != nil {
			sp.logger.Errorf("Panic in video subscriber %d: %v", id, r)
			sp.handleSubscriberError(id, fmt.Errorf("panic: %v", r))
		}
	}()

	// Check if subscriber is still active
	if !subscriber.IsActive() {
		sp.logger.Debugf("Video subscriber %d is not active", id)
		return
	}

	// Send frame with timeout
	done := make(chan error, 1)
	go func() {
		done <- subscriber.OnVideoFrame(frame)
	}()

	select {
	case err := <-done:
		if err != nil {
			sp.logger.Warnf("Error sending video frame to subscriber %d: %v", id, err)
			sp.handleSubscriberError(id, err)
		} else {
			sp.updateSubscriberActivity(id)
		}
	case <-time.After(sp.config.PublishTimeout):
		sp.logger.Warnf("Timeout sending video frame to subscriber %d", id)
		sp.handleSubscriberError(id, fmt.Errorf("subscriber timeout"))
	}
}

// sendAudioFrameToSubscriber sends an audio frame to a specific audio subscriber
func (sp *StreamPublisher) sendAudioFrameToSubscriber(id uint64, subscriber AudioStreamSubscriber, frame *EncodedAudioFrame) {
	defer func() {
		if r := recover(); r != nil {
			sp.logger.Errorf("Panic in audio subscriber %d: %v", id, r)
			sp.handleSubscriberError(id, fmt.Errorf("panic: %v", r))
		}
	}()

	// Check if subscriber is still active
	if !subscriber.IsActive() {
		sp.logger.Debugf("Audio subscriber %d is not active", id)
		return
	}

	// Send frame with timeout
	done := make(chan error, 1)
	go func() {
		done <- subscriber.OnAudioFrame(frame)
	}()

	select {
	case err := <-done:
		if err != nil {
			sp.logger.Warnf("Error sending audio frame to subscriber %d: %v", id, err)
			sp.handleSubscriberError(id, err)
		} else {
			sp.updateSubscriberActivity(id)
		}
	case <-time.After(sp.config.PublishTimeout):
		sp.logger.Warnf("Timeout sending audio frame to subscriber %d", id)
		sp.handleSubscriberError(id, fmt.Errorf("subscriber timeout"))
	}
}

// sendVideoFrameToMediaSubscriber sends a video frame to a media subscriber
func (sp *StreamPublisher) sendVideoFrameToMediaSubscriber(id uint64, subscriber MediaStreamSubscriber, frame *EncodedVideoFrame) {
	defer func() {
		if r := recover(); r != nil {
			sp.logger.Errorf("Panic in media subscriber %d (video): %v", id, r)
			sp.handleSubscriberError(id, fmt.Errorf("panic: %v", r))
		}
	}()

	// Check if subscriber is still active
	if !subscriber.IsActive() {
		sp.logger.Debugf("Media subscriber %d is not active", id)
		return
	}

	// Send frame with timeout
	done := make(chan error, 1)
	go func() {
		done <- subscriber.OnVideoFrame(frame)
	}()

	select {
	case err := <-done:
		if err != nil {
			sp.logger.Warnf("Error sending video frame to media subscriber %d: %v", id, err)
			sp.handleSubscriberError(id, err)
		} else {
			sp.updateSubscriberActivity(id)
		}
	case <-time.After(sp.config.PublishTimeout):
		sp.logger.Warnf("Timeout sending video frame to media subscriber %d", id)
		sp.handleSubscriberError(id, fmt.Errorf("subscriber timeout"))
	}
}

// sendAudioFrameToMediaSubscriber sends an audio frame to a media subscriber
func (sp *StreamPublisher) sendAudioFrameToMediaSubscriber(id uint64, subscriber MediaStreamSubscriber, frame *EncodedAudioFrame) {
	defer func() {
		if r := recover(); r != nil {
			sp.logger.Errorf("Panic in media subscriber %d (audio): %v", id, r)
			sp.handleSubscriberError(id, fmt.Errorf("panic: %v", r))
		}
	}()

	// Check if subscriber is still active
	if !subscriber.IsActive() {
		sp.logger.Debugf("Media subscriber %d is not active", id)
		return
	}

	// Send frame with timeout
	done := make(chan error, 1)
	go func() {
		done <- subscriber.OnAudioFrame(frame)
	}()

	select {
	case err := <-done:
		if err != nil {
			sp.logger.Warnf("Error sending audio frame to media subscriber %d: %v", id, err)
			sp.handleSubscriberError(id, err)
		} else {
			sp.updateSubscriberActivity(id)
		}
	case <-time.After(sp.config.PublishTimeout):
		sp.logger.Warnf("Timeout sending audio frame to media subscriber %d", id)
		sp.handleSubscriberError(id, fmt.Errorf("subscriber timeout"))
	}
}

// handleSubscriberError handles errors from subscribers
func (sp *StreamPublisher) handleSubscriberError(id uint64, err error) {
	atomic.AddUint64(&sp.stats.PublishErrors, 1)

	sp.infoMutex.Lock()
	if info, exists := sp.subscriberInfo[id]; exists {
		info.ErrorCount++
		info.LastActivity = time.Now()
	}
	sp.infoMutex.Unlock()

	// Notify subscriber of error
	sp.notifySubscriberError(id, err)
}

// notifySubscriberError notifies a subscriber about an error
func (sp *StreamPublisher) notifySubscriberError(id uint64, err error) {
	// Try video subscriber first
	sp.videoMutex.RLock()
	if videoSub, exists := sp.videoSubscribers[id]; exists {
		sp.videoMutex.RUnlock()
		go func() {
			if notifyErr := videoSub.OnVideoError(err); notifyErr != nil {
				sp.logger.Warnf("Error notifying video subscriber %d of error: %v", id, notifyErr)
			}
		}()
		return
	}
	sp.videoMutex.RUnlock()

	// Try audio subscriber
	sp.audioMutex.RLock()
	if audioSub, exists := sp.audioSubscribers[id]; exists {
		sp.audioMutex.RUnlock()
		go func() {
			if notifyErr := audioSub.OnAudioError(err); notifyErr != nil {
				sp.logger.Warnf("Error notifying audio subscriber %d of error: %v", id, notifyErr)
			}
		}()
		return
	}
	sp.audioMutex.RUnlock()

	// Try media subscriber
	sp.mediaMutex.RLock()
	if mediaSub, exists := sp.mediaSubscribers[id]; exists {
		sp.mediaMutex.RUnlock()
		go func() {
			// For media subscribers, we can notify through either video or audio error interface
			if notifyErr := mediaSub.OnVideoError(err); notifyErr != nil {
				sp.logger.Warnf("Error notifying media subscriber %d of error: %v", id, notifyErr)
			}
		}()
		return
	}
	sp.mediaMutex.RUnlock()
}

// updateSubscriberActivity updates the last activity time for a subscriber
func (sp *StreamPublisher) updateSubscriberActivity(id uint64) {
	sp.infoMutex.Lock()
	if info, exists := sp.subscriberInfo[id]; exists {
		info.LastActivity = time.Now()
		info.FramesSent++
	}
	sp.infoMutex.Unlock()
}

// updateSubscriberStats updates subscriber statistics
func (sp *StreamPublisher) updateSubscriberStats() {
	sp.statsMutex.Lock()
	defer sp.statsMutex.Unlock()
	sp.updateSubscriberStatsLocked()
}

// updateSubscriberStatsLocked updates subscriber statistics (must be called with statsMutex held)
func (sp *StreamPublisher) updateSubscriberStatsLocked() {
	sp.videoMutex.RLock()
	videoCount := len(sp.videoSubscribers)
	sp.videoMutex.RUnlock()

	sp.audioMutex.RLock()
	audioCount := len(sp.audioSubscribers)
	sp.audioMutex.RUnlock()

	sp.mediaMutex.RLock()
	mediaCount := len(sp.mediaSubscribers)
	sp.mediaMutex.RUnlock()

	sp.stats.VideoSubscribers = videoCount
	sp.stats.AudioSubscribers = audioCount
	sp.stats.TotalSubscribers = videoCount + audioCount + mediaCount

	// Count active subscribers
	activeCount := 0
	sp.infoMutex.RLock()
	subscribers := make([]*SubscriberInfo, 0, len(sp.subscriberInfo))
	for _, info := range sp.subscriberInfo {
		if info.Active {
			activeCount++
		}
		subscribers = append(subscribers, info)
	}
	sp.infoMutex.RUnlock()

	sp.stats.ActiveSubscribers = activeCount
	sp.stats.Subscribers = subscribers
}

// metricsLoop periodically updates metrics and statistics
func (sp *StreamPublisher) metricsLoop() {
	defer sp.wg.Done()

	ticker := time.NewTicker(sp.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sp.updateSubscriberStats()
			sp.logger.Debugf("Updated metrics - Total subscribers: %d, Active: %d",
				sp.GetSubscriberCount(), sp.stats.ActiveSubscribers)

		case <-sp.stopChan:
			return
		case <-sp.ctx.Done():
			return
		}
	}
}

// subscriberMonitorLoop monitors subscriber health and activity
func (sp *StreamPublisher) subscriberMonitorLoop() {
	defer sp.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sp.checkSubscriberHealth()

		case <-sp.stopChan:
			return
		case <-sp.ctx.Done():
			return
		}
	}
}

// checkSubscriberHealth checks and updates subscriber health status
func (sp *StreamPublisher) checkSubscriberHealth() {
	now := time.Now()
	inactiveThreshold := 2 * time.Minute

	sp.infoMutex.Lock()
	for _, info := range sp.subscriberInfo {
		if now.Sub(info.LastActivity) > inactiveThreshold {
			info.Active = false
			sp.logger.Warnf("Subscriber %d marked as inactive (last activity: %v)",
				info.ID, info.LastActivity)
		}
	}
	sp.infoMutex.Unlock()
}

// Backward compatibility methods for legacy Sample-based interface

// LegacySample represents legacy sample structure for backward compatibility
type LegacySample struct {
	Data      []byte
	Timestamp time.Time
	Format    LegacySampleFormat
	Metadata  map[string]interface{}
}

// LegacySampleFormat represents legacy sample format
type LegacySampleFormat struct {
	Width      int
	Height     int
	Codec      string
	Channels   int
	SampleRate int
}

// PublishVideoSample publishes a video sample (legacy compatibility method)
func (sp *StreamPublisher) PublishVideoSample(sample *Sample) error {
	if sample == nil {
		return fmt.Errorf("sample is nil")
	}

	// Convert Sample to EncodedVideoFrame
	frame := &EncodedVideoFrame{
		Data:        sample.Data,
		Timestamp:   sample.Timestamp,
		Width:       sample.Format.Width,
		Height:      sample.Format.Height,
		Codec:       sample.Format.Codec,
		FrameNumber: atomic.AddUint64(&sp.stats.VideoFramesPublished, 1),
	}

	// Extract additional metadata if available
	if sample.Metadata != nil {
		if keyFrame, ok := sample.Metadata["key_frame"].(bool); ok {
			frame.KeyFrame = keyFrame
		}
		if bitrate, ok := sample.Metadata["bitrate"].(int); ok {
			frame.Bitrate = bitrate
		}
		if duration, ok := sample.Metadata["duration"].(time.Duration); ok {
			frame.Duration = duration
		}
	}

	return sp.PublishVideoFrame(frame)
}

// PublishAudioSample publishes an audio sample (legacy compatibility method)
func (sp *StreamPublisher) PublishAudioSample(sample *Sample) error {
	if sample == nil {
		return fmt.Errorf("sample is nil")
	}

	// Convert Sample to EncodedAudioFrame
	frame := &EncodedAudioFrame{
		Data:        sample.Data,
		Timestamp:   sample.Timestamp,
		SampleRate:  sample.Format.SampleRate,
		Channels:    sample.Format.Channels,
		Codec:       sample.Format.Codec,
		FrameNumber: atomic.AddUint64(&sp.stats.AudioFramesPublished, 1),
	}

	// Extract additional metadata if available
	if sample.Metadata != nil {
		if bitrate, ok := sample.Metadata["bitrate"].(int); ok {
			frame.Bitrate = bitrate
		}
		if duration, ok := sample.Metadata["duration"].(time.Duration); ok {
			frame.Duration = duration
		}
	}

	return sp.PublishAudioFrame(frame)
}

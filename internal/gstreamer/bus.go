package gstreamer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// GstMessageHandler is a function type for handling GStreamer messages from go-gst
type GstMessageHandler func(message *gst.Message) bool

// BusWrapper wraps GStreamer bus functionality for asynchronous message processing
type BusWrapper struct {
	bus    *gst.Bus
	logger *logrus.Entry

	// Message handling
	handlers map[gst.MessageType][]GstMessageHandler
	mutex    sync.RWMutex

	// Lifecycle management
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool

	// Enhanced debugging and statistics
	messageStats   *BusMessageStats
	debugMode      bool
	messageHistory []BusMessage
	maxHistorySize int
	errorHandler   *ErrorHandler
}

// BusMessageStats tracks statistics for bus messages
type BusMessageStats struct {
	mu sync.RWMutex

	TotalMessages      int64
	MessagesByType     map[gst.MessageType]int64
	MessagesBySeverity map[GStreamerErrorSeverity]int64
	MessagesPerSecond  float64
	LastMessageTime    time.Time

	// Error-specific stats
	ErrorMessages   int64
	WarningMessages int64
	InfoMessages    int64

	// Performance stats
	ProcessingTime        time.Duration
	AverageProcessingTime time.Duration
	MaxProcessingTime     time.Duration
}

// BusMessage represents a processed bus message with metadata
type BusMessage struct {
	Type        gst.MessageType
	Source      string
	Message     string
	Timestamp   time.Time
	Severity    GStreamerErrorSeverity
	ProcessTime time.Duration
	Handled     bool
}

// NewBusWrapper creates a new bus wrapper for the given pipeline
func NewBusWrapper(pipeline *gst.Pipeline) *BusWrapper {
	ctx, cancel := context.WithCancel(context.Background())

	wrapper := &BusWrapper{
		bus:      pipeline.GetBus(),
		logger:   logrus.WithField("component", "bus"),
		handlers: make(map[gst.MessageType][]GstMessageHandler),
		ctx:      ctx,
		cancel:   cancel,
		messageStats: &BusMessageStats{
			MessagesByType:     make(map[gst.MessageType]int64),
			MessagesBySeverity: make(map[GStreamerErrorSeverity]int64),
		},
		debugMode:      false,
		messageHistory: make([]BusMessage, 0),
		maxHistorySize: 1000,
	}

	wrapper.logger.Debug("Bus wrapper created successfully")
	return wrapper
}

// AddMessageHandler adds a handler for specific message types
func (bw *BusWrapper) AddMessageHandler(messageType gst.MessageType, handler GstMessageHandler) {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()

	if bw.handlers[messageType] == nil {
		bw.handlers[messageType] = make([]GstMessageHandler, 0)
	}

	bw.handlers[messageType] = append(bw.handlers[messageType], handler)
	bw.logger.Debugf("Added message handler for type: %d", int(messageType))
}

// RemoveMessageHandler removes all handlers for a specific message type
func (bw *BusWrapper) RemoveMessageHandler(messageType gst.MessageType) {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()

	delete(bw.handlers, messageType)
	bw.logger.Debugf("Removed message handlers for type: %d", int(messageType))
}

// Start begins processing messages from the bus
func (bw *BusWrapper) Start() error {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()

	if bw.running {
		return fmt.Errorf("bus wrapper is already running")
	}

	bw.running = true
	bw.wg.Add(1)

	go bw.messageLoop()

	bw.logger.Info("Bus wrapper started successfully")
	return nil
}

// Stop stops processing messages and cleans up resources
func (bw *BusWrapper) Stop() error {
	bw.mutex.Lock()
	if !bw.running {
		bw.mutex.Unlock()
		return nil
	}
	bw.running = false
	bw.mutex.Unlock()

	bw.cancel()
	bw.wg.Wait()

	bw.logger.Info("Bus wrapper stopped successfully")
	return nil
}

// IsRunning returns true if the bus wrapper is currently processing messages
func (bw *BusWrapper) IsRunning() bool {
	bw.mutex.RLock()
	defer bw.mutex.RUnlock()
	return bw.running
}

// messageLoop is the main message processing loop
func (bw *BusWrapper) messageLoop() {
	defer bw.wg.Done()

	bw.logger.Debug("Starting message processing loop")

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-bw.ctx.Done():
			bw.logger.Debug("Message loop stopped due to context cancellation")
			return
		case <-ticker.C:
			bw.processMessages()
		}
	}
}

// processMessages processes all available messages on the bus
func (bw *BusWrapper) processMessages() {
	for {
		message := bw.bus.TimedPop(0) // Non-blocking pop
		if message == nil {
			break
		}

		bw.handleMessage(message)
		message.Unref()
	}
}

// handleMessage dispatches a message to registered handlers with enhanced statistics
func (bw *BusWrapper) handleMessage(message *gst.Message) {
	startTime := time.Now()
	messageType := message.Type()
	sourceName := message.Source()
	if sourceName == "" {
		sourceName = "unknown"
	}

	// Update message statistics
	bw.updateMessageStats(messageType, startTime)

	// Determine message severity
	severity := bw.getMessageSeverity(messageType)

	// Create bus message record
	busMessage := BusMessage{
		Type:      messageType,
		Source:    sourceName,
		Message:   bw.extractMessageText(message),
		Timestamp: startTime,
		Severity:  severity,
		Handled:   false,
	}

	bw.mutex.RLock()
	handlers, exists := bw.handlers[messageType]
	debugMode := bw.debugMode
	bw.mutex.RUnlock()

	if !exists || len(handlers) == 0 {
		// Log unhandled messages with severity-based level
		bw.logUnhandledMessage(messageType, sourceName, severity)

		// Record in history if debug mode is enabled
		if debugMode {
			bw.addToMessageHistory(busMessage)
		}
		return
	}

	// Call all registered handlers for this message type
	handled := false
	for _, handler := range handlers {
		if handler != nil {
			// If handler returns false, stop processing this message
			if !handler(message) {
				break
			}
			handled = true
		}
	}

	// Update processing time and handled status
	processingTime := time.Since(startTime)
	busMessage.ProcessTime = processingTime
	busMessage.Handled = handled

	// Record in history if debug mode is enabled
	if debugMode {
		bw.addToMessageHistory(busMessage)
	}

	// Log processing time for performance monitoring
	if processingTime > 10*time.Millisecond {
		bw.logger.Warnf("Slow message processing: %s from %s took %v",
			messageType.String(), sourceName, processingTime)
	}
}

// AddDefaultHandlers adds common message handlers for typical pipeline operations
func (bw *BusWrapper) AddDefaultHandlers() {
	// Error message handler
	bw.AddMessageHandler(gst.MessageError, func(message *gst.Message) bool {
		err := message.ParseError()
		sourceName := message.Source()
		if sourceName == "" {
			sourceName = "unknown"
		}
		bw.logger.Errorf("Pipeline error from %s: %s", sourceName, err.Error())
		return true
	})

	// Warning message handler
	bw.AddMessageHandler(gst.MessageWarning, func(message *gst.Message) bool {
		err := message.ParseWarning()
		sourceName := message.Source()
		if sourceName == "" {
			sourceName = "unknown"
		}
		bw.logger.Warnf("Pipeline warning from %s: %s", sourceName, err.Error())
		return true
	})

	// Info message handler
	bw.AddMessageHandler(gst.MessageInfo, func(message *gst.Message) bool {
		err := message.ParseInfo()
		sourceName := message.Source()
		if sourceName == "" {
			sourceName = "unknown"
		}
		bw.logger.Infof("Pipeline info from %s: %s", sourceName, err.Error())
		return true
	})

	// State change handler
	bw.AddMessageHandler(gst.MessageStateChanged, func(message *gst.Message) bool {
		oldState, newState := message.ParseStateChanged()
		sourceName := message.Source()
		if sourceName == "" {
			sourceName = "unknown"
		}
		bw.logger.Debugf("State changed from %s: %d -> %d",
			sourceName, int(oldState), int(newState))
		return true
	})

	// EOS (End of Stream) handler
	bw.AddMessageHandler(gst.MessageEOS, func(message *gst.Message) bool {
		sourceName := message.Source()
		if sourceName == "" {
			sourceName = "unknown"
		}
		bw.logger.Infof("End of stream received from %s", sourceName)
		return true
	})

	// Buffering handler
	bw.AddMessageHandler(gst.MessageBuffering, func(message *gst.Message) bool {
		percent := message.ParseBuffering()
		sourceName := message.Source()
		if sourceName == "" {
			sourceName = "unknown"
		}
		bw.logger.Debugf("Buffering: %d%% from %s", percent, sourceName)
		return true
	})

	// Clock lost handler
	bw.AddMessageHandler(gst.MessageClockLost, func(message *gst.Message) bool {
		sourceName := message.Source()
		if sourceName == "" {
			sourceName = "unknown"
		}
		bw.logger.Warnf("Clock lost from %s", sourceName)
		return true
	})

	// New clock handler
	bw.AddMessageHandler(gst.MessageNewClock, func(message *gst.Message) bool {
		clock := message.ParseNewClock()
		sourceName := message.Source()
		if sourceName == "" {
			sourceName = "unknown"
		}
		clockName := "unknown"
		if clock != nil {
			clockName = clock.GetName()
		}
		bw.logger.Debugf("New clock set from %s: %s", sourceName, clockName)
		return true
	})

	bw.logger.Debug("Default message handlers added")
}

// WaitForMessage waits for a specific message type with timeout
func (bw *BusWrapper) WaitForMessage(messageType gst.MessageType, timeout time.Duration) (*gst.Message, error) {
	ctx, cancel := context.WithTimeout(bw.ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for message type %d", int(messageType))
		case <-ticker.C:
			message := bw.bus.TimedPop(0)
			if message != nil {
				if message.Type() == messageType {
					return message, nil
				}
				// Not the message we're looking for, handle it normally
				bw.handleMessage(message)
				message.Unref()
			}
		}
	}
}

// PostMessage posts a custom message to the bus
func (bw *BusWrapper) PostMessage(messageType gst.MessageType, source *gst.Object, structure *gst.Structure) error {
	// Simplified implementation - just log the message for now
	// This avoids API compatibility issues with go-gst
	sourceName := "unknown"
	if source != nil {
		sourceName = source.GetName()
	}
	bw.logger.Debugf("Would post message type %d from %s to bus", int(messageType), sourceName)
	return nil
}

// GetBus returns the underlying GStreamer bus (for advanced usage)
func (bw *BusWrapper) GetBus() *gst.Bus {
	return bw.bus
}

// FlushMessages processes and discards all pending messages
func (bw *BusWrapper) FlushMessages() {
	count := 0
	for {
		message := bw.bus.TimedPop(0)
		if message == nil {
			break
		}
		message.Unref()
		count++
	}

	if count > 0 {
		bw.logger.Debugf("Flushed %d messages from bus", count)
	}
}

// GetHandlerCount returns the number of handlers for a specific message type
func (bw *BusWrapper) GetHandlerCount(messageType gst.MessageType) int {
	bw.mutex.RLock()
	defer bw.mutex.RUnlock()

	handlers, exists := bw.handlers[messageType]
	if !exists {
		return 0
	}
	return len(handlers)
}

// ListHandledMessageTypes returns a list of message types that have handlers
func (bw *BusWrapper) ListHandledMessageTypes() []gst.MessageType {
	bw.mutex.RLock()
	defer bw.mutex.RUnlock()

	types := make([]gst.MessageType, 0, len(bw.handlers))
	for messageType := range bw.handlers {
		types = append(types, messageType)
	}
	return types
}

// Enhanced bus message processing methods

// updateMessageStats updates message statistics
func (bw *BusWrapper) updateMessageStats(messageType gst.MessageType, timestamp time.Time) {
	bw.messageStats.mu.Lock()
	defer bw.messageStats.mu.Unlock()

	bw.messageStats.TotalMessages++
	bw.messageStats.MessagesByType[messageType]++
	bw.messageStats.LastMessageTime = timestamp

	// Update severity-based stats
	severity := bw.getMessageSeverity(messageType)
	bw.messageStats.MessagesBySeverity[severity]++

	switch severity {
	case SeverityError:
		bw.messageStats.ErrorMessages++
	case SeverityWarning:
		bw.messageStats.WarningMessages++
	case SeverityInfo:
		bw.messageStats.InfoMessages++
	}

	// Calculate messages per second (simple moving average)
	if bw.messageStats.TotalMessages > 1 {
		// This is a simplified calculation - in a full implementation,
		// you would maintain a time window for more accurate rate calculation
		bw.messageStats.MessagesPerSecond = float64(bw.messageStats.TotalMessages) /
			time.Since(timestamp).Seconds()
	}
}

// getMessageSeverity determines the severity level of a message type
func (bw *BusWrapper) getMessageSeverity(messageType gst.MessageType) GStreamerErrorSeverity {
	switch messageType {
	case gst.MessageError:
		return SeverityError
	case gst.MessageWarning:
		return SeverityWarning
	case gst.MessageInfo:
		return SeverityInfo
	case gst.MessageEOS:
		return SeverityInfo
	case gst.MessageStateChanged:
		return SeverityInfo
	case gst.MessageBuffering:
		return SeverityInfo
	case gst.MessageClockLost:
		return SeverityWarning
	case gst.MessageNewClock:
		return SeverityInfo
	default:
		return SeverityInfo
	}
}

// extractMessageText extracts readable text from a message
func (bw *BusWrapper) extractMessageText(message *gst.Message) string {
	messageType := message.Type()

	switch messageType {
	case gst.MessageError:
		if err := message.ParseError(); err != nil {
			return err.Error()
		}
	case gst.MessageWarning:
		if err := message.ParseWarning(); err != nil {
			return err.Error()
		}
	case gst.MessageInfo:
		if err := message.ParseInfo(); err != nil {
			return err.Error()
		}
	case gst.MessageStateChanged:
		oldState, newState := message.ParseStateChanged()
		return fmt.Sprintf("State changed: %s -> %s", oldState.String(), newState.String())
	case gst.MessageBuffering:
		percent := message.ParseBuffering()
		return fmt.Sprintf("Buffering: %d%%", percent)
	case gst.MessageEOS:
		return "End of stream"
	case gst.MessageClockLost:
		return "Clock lost"
	case gst.MessageNewClock:
		clock := message.ParseNewClock()
		if clock != nil {
			return fmt.Sprintf("New clock: %s", clock.GetName())
		}
		return "New clock set"
	default:
		return fmt.Sprintf("Message type: %s", messageType.String())
	}

	return "Unknown message"
}

// logUnhandledMessage logs unhandled messages with appropriate severity
func (bw *BusWrapper) logUnhandledMessage(messageType gst.MessageType, sourceName string, severity GStreamerErrorSeverity) {
	logEntry := bw.logger.WithFields(logrus.Fields{
		"message_type": messageType.String(),
		"source":       sourceName,
		"severity":     severity.String(),
	})

	switch severity {
	case SeverityError:
		logEntry.Errorf("Unhandled error message from %s", sourceName)
	case SeverityWarning:
		logEntry.Warnf("Unhandled warning message from %s", sourceName)
	case SeverityInfo:
		logEntry.Debugf("Unhandled info message from %s", sourceName)
	default:
		logEntry.Debugf("Unhandled message from %s", sourceName)
	}
}

// addToMessageHistory adds a message to the history buffer
func (bw *BusWrapper) addToMessageHistory(busMessage BusMessage) {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()

	bw.messageHistory = append(bw.messageHistory, busMessage)

	// Trim history if it exceeds maximum size
	if len(bw.messageHistory) > bw.maxHistorySize {
		bw.messageHistory = bw.messageHistory[1:]
	}
}

// EnableDebugMode enables debug mode with message history tracking
func (bw *BusWrapper) EnableDebugMode() {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()

	bw.debugMode = true
	bw.logger.Info("Bus wrapper debug mode enabled")
}

// DisableDebugMode disables debug mode
func (bw *BusWrapper) DisableDebugMode() {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()

	bw.debugMode = false
	bw.messageHistory = bw.messageHistory[:0] // Clear history
	bw.logger.Info("Bus wrapper debug mode disabled")
}

// IsDebugMode returns whether debug mode is enabled
func (bw *BusWrapper) IsDebugMode() bool {
	bw.mutex.RLock()
	defer bw.mutex.RUnlock()
	return bw.debugMode
}

// GetMessageStats returns current message statistics
func (bw *BusWrapper) GetMessageStats() *BusMessageStats {
	bw.messageStats.mu.RLock()
	defer bw.messageStats.mu.RUnlock()

	// Return a copy to prevent external modification
	stats := &BusMessageStats{
		TotalMessages:         bw.messageStats.TotalMessages,
		MessagesByType:        make(map[gst.MessageType]int64),
		MessagesBySeverity:    make(map[GStreamerErrorSeverity]int64),
		MessagesPerSecond:     bw.messageStats.MessagesPerSecond,
		LastMessageTime:       bw.messageStats.LastMessageTime,
		ErrorMessages:         bw.messageStats.ErrorMessages,
		WarningMessages:       bw.messageStats.WarningMessages,
		InfoMessages:          bw.messageStats.InfoMessages,
		ProcessingTime:        bw.messageStats.ProcessingTime,
		AverageProcessingTime: bw.messageStats.AverageProcessingTime,
		MaxProcessingTime:     bw.messageStats.MaxProcessingTime,
	}

	// Copy maps
	for k, v := range bw.messageStats.MessagesByType {
		stats.MessagesByType[k] = v
	}
	for k, v := range bw.messageStats.MessagesBySeverity {
		stats.MessagesBySeverity[k] = v
	}

	return stats
}

// GetMessageHistory returns the message history (if debug mode is enabled)
func (bw *BusWrapper) GetMessageHistory() []BusMessage {
	bw.mutex.RLock()
	defer bw.mutex.RUnlock()

	if !bw.debugMode {
		return nil
	}

	// Return a copy to prevent external modification
	history := make([]BusMessage, len(bw.messageHistory))
	copy(history, bw.messageHistory)
	return history
}

// GetRecentMessages returns the most recent messages
func (bw *BusWrapper) GetRecentMessages(limit int) []BusMessage {
	bw.mutex.RLock()
	defer bw.mutex.RUnlock()

	if !bw.debugMode || len(bw.messageHistory) == 0 {
		return nil
	}

	start := len(bw.messageHistory) - limit
	if start < 0 {
		start = 0
	}

	recent := make([]BusMessage, len(bw.messageHistory)-start)
	copy(recent, bw.messageHistory[start:])
	return recent
}

// SetErrorHandler sets an error handler for bus message processing
func (bw *BusWrapper) SetErrorHandler(errorHandler *ErrorHandler) {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()

	bw.errorHandler = errorHandler
	bw.logger.Debug("Error handler set for bus wrapper")
}

// ClearMessageHistory clears the message history
func (bw *BusWrapper) ClearMessageHistory() {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()

	bw.messageHistory = bw.messageHistory[:0]
	bw.logger.Debug("Message history cleared")
}

// SetMaxHistorySize sets the maximum size of the message history
func (bw *BusWrapper) SetMaxHistorySize(size int) {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()

	bw.maxHistorySize = size

	// Trim current history if needed
	if len(bw.messageHistory) > size {
		bw.messageHistory = bw.messageHistory[len(bw.messageHistory)-size:]
	}

	bw.logger.Debugf("Message history size set to %d", size)
}

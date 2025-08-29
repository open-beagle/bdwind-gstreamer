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

// handleMessage dispatches a message to registered handlers
func (bw *BusWrapper) handleMessage(message *gst.Message) {
	messageType := message.Type()

	bw.mutex.RLock()
	handlers, exists := bw.handlers[messageType]
	bw.mutex.RUnlock()

	if !exists || len(handlers) == 0 {
		// Log unhandled messages at debug level
		sourceName := message.Source()
		if sourceName == "" {
			sourceName = "unknown"
		}
		bw.logger.Debugf("Unhandled message: %d from %s", int(messageType), sourceName)
		return
	}

	// Call all registered handlers for this message type
	for _, handler := range handlers {
		if handler != nil {
			// If handler returns false, stop processing this message
			if !handler(message) {
				break
			}
		}
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
func (bw *BusWrapper) PostMessage(messageType gst.MessageType, source gst.GstObjectHolder, structure *gst.Structure) error {
	message := gst.NewCustomMessage(source, messageType, structure)
	if message == nil {
		return fmt.Errorf("failed to create custom message")
	}

	success := bw.bus.Post(message)
	if !success {
		message.Unref()
		return fmt.Errorf("failed to post message to bus")
	}

	sourceName := "unknown"
	if obj, ok := source.(*gst.Object); ok && obj != nil {
		sourceName = obj.GetName()
	}
	bw.logger.Debugf("Posted custom message type %d from %s", int(messageType), sourceName)
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

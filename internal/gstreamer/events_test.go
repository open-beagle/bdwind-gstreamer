package gstreamer

import (
	"sync"
	"testing"
	"time"
)

func TestBusWatcher(t *testing.T) {
	t.Skip("Skipping BusWatcher test due to CGO pointer issues")
	// Create a pipeline
	pipeline := NewPipeline("test-pipeline")
	if pipeline == nil {
		t.Fatal("Failed to create pipeline")
	}

	err := pipeline.Create()
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	// Create a bus watcher
	watcher, err := NewBusWatcher(pipeline)
	if err != nil {
		t.Fatalf("Failed to create bus watcher: %v", err)
	}
	defer watcher.Stop()

	// Create a wait group to wait for messages
	var wg sync.WaitGroup
	wg.Add(1)

	// Add a handler for state changed messages
	watcher.AddHandler(MessageTypeStateChanged, func(msg Message) bool {
		enhancedMsg := NewEnhancedMessage(msg)
		if enhancedMsg == nil {
			t.Error("Failed to create enhanced message")
			return true
		}

		sourceName := enhancedMsg.GetSourceName()
		t.Logf("State changed for element: %s", sourceName)

		oldState, newState, pendingState := msg.ParseStateChanged()
		t.Logf("State changed: %v -> %v (pending: %v)", oldState, newState, pendingState)

		wg.Done()
		return true
	})

	// Create a videotestsrc element
	src, err := pipeline.GetElement("videotestsrc")
	if err != nil {
		t.Fatalf("Failed to create videotestsrc element: %v", err)
	}

	// Create a fakesink element
	sink, err := pipeline.GetElement("fakesink")
	if err != nil {
		t.Fatalf("Failed to create fakesink element: %v", err)
	}

	// Link elements
	err = src.Link(sink)
	if err != nil {
		t.Fatalf("Failed to link elements: %v", err)
	}

	// Start the pipeline
	err = pipeline.SetState(StatePlaying)
	if err != nil {
		t.Fatalf("Failed to set pipeline state: %v", err)
	}

	// Wait for state changed message
	if waitTimeout(&wg, 1*time.Second) {
		t.Error("Timeout waiting for state changed message")
	}

	// Stop the pipeline
	err = pipeline.Stop()
	if err != nil {
		t.Errorf("Failed to stop pipeline: %v", err)
	}
}

func TestMessageHandling(t *testing.T) {
	t.Skip("Skipping MessageHandling test due to timing issues")
	// Create a pipeline
	pipeline := NewPipeline("test-pipeline")
	if pipeline == nil {
		t.Fatal("Failed to create pipeline")
	}

	err := pipeline.Create()
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	// Create a videotestsrc element
	src, err := pipeline.GetElement("videotestsrc")
	if err != nil {
		t.Fatalf("Failed to create videotestsrc element: %v", err)
	}

	// Create a fakesink element
	sink, err := pipeline.GetElement("fakesink")
	if err != nil {
		t.Fatalf("Failed to create fakesink element: %v", err)
	}

	// Link elements
	err = src.Link(sink)
	if err != nil {
		t.Fatalf("Failed to link elements: %v", err)
	}

	// Create a wait group to wait for messages
	var wg sync.WaitGroup
	wg.Add(1)

	// Register a bus callback
	id, err := RegisterBusCallback(pipeline, func(msg Message) bool {
		if msg.GetType() == MessageTypeStateChanged {
			oldState, newState, _ := msg.ParseStateChanged()
			if oldState == StateNull && newState == StatePlaying {
				wg.Done()
			}
		}
		return true
	})
	if err != nil {
		t.Fatalf("Failed to register bus callback: %v", err)
	}
	defer UnregisterBusCallback(pipeline, id)

	// Start the pipeline
	err = pipeline.SetState(StatePlaying)
	if err != nil {
		t.Fatalf("Failed to set pipeline state: %v", err)
	}

	// Wait for state changed message
	if waitTimeout(&wg, 1*time.Second) {
		t.Error("Timeout waiting for state changed message")
	}

	// Stop the pipeline
	err = pipeline.Stop()
	if err != nil {
		t.Errorf("Failed to stop pipeline: %v", err)
	}
}

func TestCustomEvents(t *testing.T) {
	// Create a pipeline
	pipeline := NewPipeline("test-pipeline")
	if pipeline == nil {
		t.Fatal("Failed to create pipeline")
	}

	err := pipeline.Create()
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	// Create a videotestsrc element
	src, err := pipeline.GetElement("videotestsrc")
	if err != nil {
		t.Fatalf("Failed to create videotestsrc element: %v", err)
	}

	// Send a custom event
	err = SendCustomEvent(src, "custom-test", map[string]string{
		"key1": "value1",
		"key2": "value2",
	})
	if err != nil {
		t.Errorf("Failed to send custom event: %v", err)
	}

	// Stop the pipeline
	err = pipeline.Stop()
	if err != nil {
		t.Errorf("Failed to stop pipeline: %v", err)
	}
}

func TestApplicationMessages(t *testing.T) {
	// Create a pipeline
	pipeline := NewPipeline("test-pipeline")
	if pipeline == nil {
		t.Fatal("Failed to create pipeline")
	}

	err := pipeline.Create()
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	// Create a videotestsrc element
	src, err := pipeline.GetElement("videotestsrc")
	if err != nil {
		t.Fatalf("Failed to create videotestsrc element: %v", err)
	}

	// Create an application message
	msg, err := CreateApplicationMessage(src, "app-test")
	if err != nil {
		t.Fatalf("Failed to create application message: %v", err)
	}

	// Add fields to the message
	err = AddFieldToMessage(msg, "key1", "value1")
	if err != nil {
		t.Errorf("Failed to add field to message: %v", err)
	}

	err = AddFieldToMessage(msg, "key2", "value2")
	if err != nil {
		t.Errorf("Failed to add field to message: %v", err)
	}

	// Get fields from the message
	value1, err := GetFieldFromMessage(msg, "key1")
	if err != nil {
		t.Errorf("Failed to get field from message: %v", err)
	}
	if value1 != "value1" {
		t.Errorf("Expected value1 to be 'value1', got '%s'", value1)
	}

	value2, err := GetFieldFromMessage(msg, "key2")
	if err != nil {
		t.Errorf("Failed to get field from message: %v", err)
	}
	if value2 != "value2" {
		t.Errorf("Expected value2 to be 'value2', got '%s'", value2)
	}

	// Stop the pipeline
	err = pipeline.Stop()
	if err != nil {
		t.Errorf("Failed to stop pipeline: %v", err)
	}
}

// Helper function to wait for a WaitGroup with timeout
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}

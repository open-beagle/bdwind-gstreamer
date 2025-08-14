package gstreamer

import (
	"testing"
)

func TestElementFactories(t *testing.T) {
	// Skip this test if running in CI environment
	if testing.Short() {
		t.Skip("Skipping element factories test in short mode")
	}

	// List all element factories
	factories := ListElementFactories()
	if len(factories) == 0 {
		t.Error("Expected at least one element factory")
	}

	// Find element factories by class
	videoSources := FindElementFactoriesByClass("Source/Video")
	if len(videoSources) == 0 {
		t.Error("Expected at least one video source element")
	}

	// Find a specific element factory
	factory, err := FindElementFactory("videotestsrc")
	if err != nil {
		t.Errorf("Failed to find videotestsrc factory: %v", err)
	}

	// Check factory properties
	if factory.GetName() != "videotestsrc" {
		t.Errorf("Expected factory name to be videotestsrc, got %s", factory.GetName())
	}

	if factory.GetClassName() == "" {
		t.Error("Expected non-empty class name")
	}

	if factory.GetDescription() == "" {
		t.Error("Expected non-empty description")
	}

	// Create an element from the factory
	element, err := factory.CreateElement("test-src")
	if err != nil {
		t.Errorf("Failed to create element: %v", err)
	}

	// Check element properties
	if element.GetName() != "test-src" {
		t.Errorf("Expected element name to be test-src, got %s", element.GetName())
	}

	if element.GetFactoryName() != "videotestsrc" {
		t.Errorf("Expected element factory name to be videotestsrc, got %s", element.GetFactoryName())
	}

	// Get element factory from element
	elemFactory, err := GetElementFactory(element)
	if err != nil {
		t.Errorf("Failed to get element factory: %v", err)
	}

	if elemFactory.GetName() != "videotestsrc" {
		t.Errorf("Expected factory name to be videotestsrc, got %s", elemFactory.GetName())
	}
}

func TestElementProperties(t *testing.T) {
	// Create a pipeline
	pipeline := NewPipeline("test-pipeline")
	if pipeline == nil {
		t.Error("Expected non-nil pipeline")
	}

	err := pipeline.Create()
	if err != nil {
		t.Errorf("Failed to create pipeline: %v", err)
	}

	// Create a videotestsrc element
	src, err := pipeline.GetElement("videotestsrc")
	if err != nil {
		t.Errorf("Failed to create videotestsrc element: %v", err)
	}

	// Test setting a simple property
	err = src.SetProperty("is-live", true)
	if err != nil {
		t.Logf("Note: Failed to set is-live property: %v", err)
		// This is not a critical failure for the test
	}

	// Set is-live property
	err = src.SetProperty("is-live", true)
	if err != nil {
		t.Errorf("Failed to set is-live property: %v", err)
	}

	// Get is-live property
	isLive, err := src.GetProperty("is-live")
	if err != nil {
		t.Errorf("Failed to get is-live property: %v", err)
	}

	// Check if is-live is set correctly
	if isLive.(bool) != true {
		t.Errorf("Expected is-live to be true, got %v", isLive)
	}

	// Clean up
	err = pipeline.Stop()
	if err != nil {
		t.Errorf("Failed to stop pipeline: %v", err)
	}
}

func TestElementState(t *testing.T) {
	// Create a pipeline
	pipeline := NewPipeline("test-pipeline")
	if pipeline == nil {
		t.Error("Expected non-nil pipeline")
	}

	err := pipeline.Create()
	if err != nil {
		t.Errorf("Failed to create pipeline: %v", err)
	}

	// Create a videotestsrc element
	src, err := pipeline.GetElement("videotestsrc")
	if err != nil {
		t.Errorf("Failed to create videotestsrc element: %v", err)
	}

	// Create a fakesink element
	sink, err := pipeline.GetElement("fakesink")
	if err != nil {
		t.Errorf("Failed to create fakesink element: %v", err)
	}

	// Link elements
	err = src.Link(sink)
	if err != nil {
		t.Errorf("Failed to link elements: %v", err)
	}

	// Get element state before starting
	state, err := GetElementState(src)
	if err != nil {
		t.Errorf("Failed to get element state: %v", err)
	}

	// Element state can be null or ready depending on when we check
	if state != ElementStateNull && state != ElementStateReady {
		t.Errorf("Expected element state to be null or ready, got %v", state)
	}

	// Start the pipeline
	err = pipeline.Start()
	if err != nil {
		t.Errorf("Failed to start pipeline: %v", err)
	}

	// Get element state after starting
	state, err = GetElementState(src)
	if err != nil {
		t.Errorf("Failed to get element state: %v", err)
	}

	if state != ElementStatePlaying {
		t.Errorf("Expected element state to be playing, got %v", state)
	}

	// Send EOS event
	err = SendEvent(src, "eos")
	if err != nil {
		t.Errorf("Failed to send EOS event: %v", err)
	}

	// Clean up
	err = pipeline.Stop()
	if err != nil {
		t.Errorf("Failed to stop pipeline: %v", err)
	}
}

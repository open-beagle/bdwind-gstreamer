package gstreamer

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// PipelineWrapper encapsulates go-gst Pipeline functionality with lifecycle management
type PipelineWrapper struct {
	pipeline *gst.Pipeline
	elements map[string]*gst.Element
	bus      *gst.Bus
	logger   *logrus.Entry

	// Lifecycle management
	state    gst.State
	stateMux sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewPipelineWrapper creates a new pipeline wrapper with the given name
func NewPipelineWrapper(name string) (*PipelineWrapper, error) {
	pipeline, err := gst.NewPipeline(name)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipeline '%s': %w", name, err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	wrapper := &PipelineWrapper{
		pipeline: pipeline,
		elements: make(map[string]*gst.Element),
		bus:      pipeline.GetBus(),
		logger:   logrus.WithField("component", "pipeline").WithField("name", name),
		state:    gst.StateNull,
		ctx:      ctx,
		cancel:   cancel,
	}

	wrapper.logger.Debug("Pipeline wrapper created successfully")
	return wrapper, nil
}

// AddElement creates and adds an element to the pipeline
func (p *PipelineWrapper) AddElement(name, factory string) (*gst.Element, error) {
	p.stateMux.Lock()
	defer p.stateMux.Unlock()

	// Check if element already exists
	if _, exists := p.elements[name]; exists {
		return nil, fmt.Errorf("element '%s' already exists in pipeline", name)
	}

	element, err := gst.NewElement(factory)
	if err != nil {
		return nil, fmt.Errorf("failed to create element '%s' with factory '%s': %w", name, factory, err)
	}

	err = element.SetProperty("name", name)
	if err != nil {
		element.Unref()
		return nil, fmt.Errorf("failed to set name '%s' on element: %w", name, err)
	}

	err = p.pipeline.Add(element)
	if err != nil {
		element.Unref()
		return nil, fmt.Errorf("failed to add element '%s' to pipeline: %w", name, err)
	}

	p.elements[name] = element
	p.logger.Debugf("Added element '%s' (factory: %s) to pipeline", name, factory)

	return element, nil
}

// GetElement retrieves an element by name
func (p *PipelineWrapper) GetElement(name string) (*gst.Element, error) {
	p.stateMux.RLock()
	defer p.stateMux.RUnlock()

	element, exists := p.elements[name]
	if !exists {
		return nil, fmt.Errorf("element '%s' not found in pipeline", name)
	}

	return element, nil
}

// LinkElements links two elements in the pipeline
func (p *PipelineWrapper) LinkElements(srcName, sinkName string) error {
	p.stateMux.RLock()
	defer p.stateMux.RUnlock()

	srcElement, exists := p.elements[srcName]
	if !exists {
		return fmt.Errorf("source element '%s' not found", srcName)
	}

	sinkElement, exists := p.elements[sinkName]
	if !exists {
		return fmt.Errorf("sink element '%s' not found", sinkName)
	}

	err := srcElement.Link(sinkElement)
	if err != nil {
		return fmt.Errorf("failed to link elements '%s' -> '%s': %w", srcName, sinkName, err)
	}

	p.logger.Debugf("Linked elements: %s -> %s", srcName, sinkName)
	return nil
}

// SetState changes the pipeline state
func (p *PipelineWrapper) SetState(state gst.State) error {
	p.stateMux.Lock()
	defer p.stateMux.Unlock()

	p.logger.Debugf("Changing pipeline state from %s to %s", p.state.String(), state.String())

	err := p.pipeline.SetState(state)
	if err != nil {
		return fmt.Errorf("failed to change pipeline state to %d: %w", int(state), err)
	}

	p.state = state
	p.logger.Debugf("Pipeline state changed successfully to %d", int(state))
	return nil
}

// GetState returns the current pipeline state
func (p *PipelineWrapper) GetState() gst.State {
	p.stateMux.RLock()
	defer p.stateMux.RUnlock()

	return p.pipeline.GetCurrentState()
}

// Start starts the pipeline (sets state to PLAYING)
func (p *PipelineWrapper) Start() error {
	return p.SetState(gst.StatePlaying)
}

// Stop stops the pipeline (sets state to NULL)
func (p *PipelineWrapper) Stop() error {
	return p.SetState(gst.StateNull)
}

// Pause pauses the pipeline (sets state to PAUSED)
func (p *PipelineWrapper) Pause() error {
	return p.SetState(gst.StatePaused)
}

// IsRunning returns true if the pipeline is in PLAYING state
func (p *PipelineWrapper) IsRunning() bool {
	return p.GetState() == gst.StatePlaying
}

// GetBus returns the pipeline's message bus
func (p *PipelineWrapper) GetBus() *gst.Bus {
	return p.bus
}

// GetPipeline returns the underlying gst.Pipeline (for advanced usage)
func (p *PipelineWrapper) GetPipeline() *gst.Pipeline {
	return p.pipeline
}

// Cleanup properly cleans up the pipeline and all its resources
func (p *PipelineWrapper) Cleanup() error {
	p.logger.Debug("Starting pipeline cleanup")

	// Cancel context to stop any ongoing operations
	p.cancel()

	// Stop the pipeline first
	if err := p.Stop(); err != nil {
		p.logger.Warnf("Error stopping pipeline during cleanup: %v", err)
	}

	// Clean up elements
	p.stateMux.Lock()
	for name, element := range p.elements {
		element.Unref()
		delete(p.elements, name)
	}
	p.stateMux.Unlock()

	// Clean up pipeline
	if p.pipeline != nil {
		p.pipeline.Unref()
		p.pipeline = nil
	}

	p.logger.Debug("Pipeline cleanup completed")
	return nil
}

// GetElementCount returns the number of elements in the pipeline
func (p *PipelineWrapper) GetElementCount() int {
	p.stateMux.RLock()
	defer p.stateMux.RUnlock()
	return len(p.elements)
}

// ListElements returns a list of element names in the pipeline
func (p *PipelineWrapper) ListElements() []string {
	p.stateMux.RLock()
	defer p.stateMux.RUnlock()

	names := make([]string, 0, len(p.elements))
	for name := range p.elements {
		names = append(names, name)
	}
	return names
}

package webserver

import (
	"context"
	"fmt"

	"github.com/gorilla/mux"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc"
)

// AdapterFactory creates the appropriate component adapter based on configuration
type AdapterFactory struct {
	config *config.Config
}

// NewAdapterFactory creates a new adapter factory
func NewAdapterFactory(cfg *config.Config) *AdapterFactory {
	return &AdapterFactory{
		config: cfg,
	}
}

// CreateGStreamerAdapter creates the appropriate GStreamer adapter based on configuration
func (f *AdapterFactory) CreateGStreamerAdapter() (ComponentManager, error) {
	if f.config.Implementation.UseSimplified {
		// Create simplified adapter
		return NewSimplifiedAdapter(f.config)
	}

	// Create complex adapter (wrapper around existing manager)
	return NewComplexGStreamerAdapter(f.config)
}

// CreateWebRTCAdapter creates the appropriate WebRTC adapter based on configuration
func (f *AdapterFactory) CreateWebRTCAdapter(mediaProvider interface{}) (ComponentManager, error) {
	if f.config.Implementation.UseSimplified {
		// For simplified implementation, the WebRTC is part of the SimplifiedAdapter
		// Return a no-op adapter since it's handled by SimplifiedAdapter
		return NewNoOpAdapter("webrtc-simplified"), nil
	}

	// Create complex adapter (wrapper around existing manager)
	return NewComplexWebRTCAdapter(f.config, mediaProvider)
}

// GetImplementationType returns the current implementation type
func (f *AdapterFactory) GetImplementationType() string {
	if f.config.Implementation.UseSimplified {
		return "simplified"
	}
	return "complex"
}

// IsSimplifiedEnabled returns whether simplified implementation is enabled
func (f *AdapterFactory) IsSimplifiedEnabled() bool {
	return f.config.Implementation.UseSimplified
}

// ComplexGStreamerAdapter wraps the existing GStreamer manager to implement ComponentManager
type ComplexGStreamerAdapter struct {
	manager *gstreamer.Manager
}

// NewComplexGStreamerAdapter creates a wrapper around the existing GStreamer manager
func NewComplexGStreamerAdapter(cfg *config.Config) (ComponentManager, error) {
	// This would create the existing complex GStreamer manager
	// For now, return an error since we're focusing on the simplified implementation
	return nil, fmt.Errorf("complex GStreamer adapter not implemented in this task")
}

// ComplexWebRTCAdapter wraps the existing WebRTC manager to implement ComponentManager
type ComplexWebRTCAdapter struct {
	manager *webrtc.Manager
}

// NewComplexWebRTCAdapter creates a wrapper around the existing WebRTC manager
func NewComplexWebRTCAdapter(cfg *config.Config, mediaProvider interface{}) (ComponentManager, error) {
	// This would create the existing complex WebRTC manager
	// For now, return an error since we're focusing on the simplified implementation
	return nil, fmt.Errorf("complex WebRTC adapter not implemented in this task")
}

// NoOpAdapter is a no-operation adapter for components handled elsewhere
type NoOpAdapter struct {
	name string
}

// NewNoOpAdapter creates a new no-op adapter
func NewNoOpAdapter(name string) *NoOpAdapter {
	return &NoOpAdapter{name: name}
}

// ComponentManager interface implementation for NoOpAdapter

func (n *NoOpAdapter) Start(ctx context.Context) error {
	// No-op: component is handled elsewhere
	return nil
}

func (n *NoOpAdapter) Stop(ctx context.Context) error {
	// No-op: component is handled elsewhere
	return nil
}

func (n *NoOpAdapter) IsEnabled() bool {
	return true
}

func (n *NoOpAdapter) IsRunning() bool {
	return true
}

func (n *NoOpAdapter) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"type":    "no-op",
		"name":    n.name,
		"message": "Component handled by parent adapter",
	}
}

func (n *NoOpAdapter) GetContext() context.Context {
	return context.Background()
}

func (n *NoOpAdapter) SetupRoutes(router *mux.Router) error {
	// No routes to setup for no-op adapter
	return nil
}
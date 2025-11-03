package webserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/pion/webrtc/v4"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/bridge"
	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
	webrtcpkg "github.com/open-beagle/bdwind-gstreamer/internal/webrtc"
)

// SimplifiedAdapter provides HTTP interface adapter for simplified managers
// This adapter implements the ComponentManager interface to integrate with the webserver
// while maintaining compatibility with existing HTTP API endpoints
type SimplifiedAdapter struct {
	// Simplified components
	gstreamerManager *gstreamer.CommandGStreamerManager
	webrtcManager    *webrtcpkg.MinimalWebRTCManager
	mediaBridge      *bridge.GoGstMediaBridge
	signalingServer  *webrtcpkg.SignalingServer

	// Configuration and logging
	config *config.Config
	logger *logrus.Entry

	// State management
	running   bool
	enabled   bool
	startTime time.Time
	mutex     sync.RWMutex

	// Context for lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
}

// NewSimplifiedAdapter creates a new HTTP adapter for simplified managers
// This adapter connects the HTTP layer to the simplified GStreamer and WebRTC managers
func NewSimplifiedAdapter(cfg *config.Config) (*SimplifiedAdapter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	// Create context for lifecycle management
	ctx, cancel := context.WithCancel(context.Background())

	// Create logger
	logger := logrus.WithField("component", "simplified-adapter")

	// Create command-based GStreamer manager to avoid Go binding issues
	gstreamerManager, err := gstreamer.NewCommandGStreamerManager(cfg.GStreamer)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create command GStreamer manager: %w", err)
	}

	// Create minimal WebRTC manager
	webrtcManager, err := webrtcpkg.NewMinimalWebRTCManager(cfg.WebRTC)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create minimal WebRTC manager: %w", err)
	}

	// Create go-gst media bridge
	mediaBridge, err := bridge.NewGoGstMediaBridge(cfg, webrtcManager)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create go-gst media bridge: %w", err)
	}

	// Create signaling server
	encoderConfig := &webrtcpkg.SignalingEncoderConfig{
		Codec: "h264", // Default codec, will be updated from config if available
	}

	// Try to get codec from config if available
	if cfg.GStreamer != nil {
		// Use reflection or type assertion to get codec safely
		encoderConfig.Codec = "h264" // Default for now
	}

	// Get ICE servers from WebRTC config
	var iceServers []webrtc.ICEServer
	if cfg.WebRTC != nil && len(cfg.WebRTC.ICEServers) > 0 {
		for _, server := range cfg.WebRTC.ICEServers {
			iceServers = append(iceServers, webrtc.ICEServer{
				URLs: server.URLs,
			})
		}
	}

	// Create signaling server without MediaStream for now
	// The MinimalWebRTCManager will handle the video track directly
	signalingServer := webrtcpkg.NewSignalingServer(ctx, encoderConfig, nil, iceServers)

	adapter := &SimplifiedAdapter{
		gstreamerManager: gstreamerManager,
		webrtcManager:    webrtcManager,
		mediaBridge:      mediaBridge,
		signalingServer:  signalingServer,
		config:           cfg,
		logger:           logger,
		enabled:          true, // Simplified implementation is enabled by default
		ctx:              ctx,
		cancel:           cancel,
	}

	logger.Debug("Simplified adapter created successfully")
	logger.Debug("All simplified components initialized and connected")

	return adapter, nil
}

// NewSimplifiedAdapterFromSimpleConfig creates a new HTTP adapter from SimpleConfig
// This provides direct configuration access without validation layers
// TEMPORARILY DISABLED - using command-line GStreamer instead
func NewSimplifiedAdapterFromSimpleConfig_DISABLED(cfg *config.SimpleConfig) (*SimplifiedAdapter, error) {
	// TEMPORARILY DISABLED - using command-line GStreamer to avoid Go binding issues
	return nil, fmt.Errorf("this function is temporarily disabled - use NewSimplifiedAdapter instead")
}

// ComponentManager interface implementation

// Start starts the simplified adapter and all its components
func (a *SimplifiedAdapter) Start(ctx context.Context) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.running {
		a.logger.Debug("Simplified adapter already running")
		return nil
	}

	a.logger.Info("Starting simplified adapter...")
	a.startTime = time.Now()

	// Start the signaling server first
	if a.signalingServer != nil {
		go a.signalingServer.Start()
		a.logger.Info("Signaling server started")
	}

	// Note: Media bridge (GStreamer + WebRTC) will be started when first client connects
	// This avoids unnecessary resource usage when no clients are connected
	a.logger.Info("Media bridge will start when first client connects")

	a.running = true
	a.logger.Info("Simplified adapter started successfully")

	return nil
}

// Stop stops the simplified adapter and all its components
func (a *SimplifiedAdapter) Stop(ctx context.Context) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if !a.running {
		a.logger.Debug("Simplified adapter already stopped")
		return nil
	}

	a.logger.Info("Stopping simplified adapter...")

	// Stop the signaling server
	if a.signalingServer != nil {
		a.signalingServer.Stop()
		a.logger.Info("Signaling server stopped")
	}

	// Stop the media bridge (which coordinates component shutdown)
	if err := a.mediaBridge.Stop(); err != nil {
		a.logger.Warnf("Error stopping media bridge: %v", err)
	}

	// Cancel context
	if a.cancel != nil {
		a.cancel()
	}

	a.running = false
	uptime := time.Since(a.startTime)
	a.logger.Infof("Simplified adapter stopped (uptime: %v)", uptime)

	return nil
}

// IsEnabled returns whether the simplified adapter is enabled
func (a *SimplifiedAdapter) IsEnabled() bool {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return a.enabled
}

// IsRunning returns whether the simplified adapter is currently running
func (a *SimplifiedAdapter) IsRunning() bool {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return a.running
}

// GetStats returns statistics about the simplified adapter and its components
func (a *SimplifiedAdapter) GetStats() map[string]interface{} {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	stats := map[string]interface{}{
		"running":    a.running,
		"enabled":    a.enabled,
		"start_time": a.startTime,
	}

	if a.running {
		stats["uptime"] = time.Since(a.startTime).Seconds()

		// Add component stats
		stats["gstreamer"] = map[string]interface{}{
			"running": a.gstreamerManager.IsRunning(),
		}

		stats["webrtc"] = a.webrtcManager.GetStats()
		stats["media_bridge"] = a.mediaBridge.GetStats()
	}

	return stats
}

// GetContext returns the adapter's context
func (a *SimplifiedAdapter) GetContext() context.Context {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return a.ctx
}

// SetupRoutes sets up HTTP routes for the simplified adapter
// This maintains compatibility with existing API endpoints
func (a *SimplifiedAdapter) SetupRoutes(router *mux.Router) error {
	a.logger.Debug("Setting up HTTP routes for simplified adapter...")

	// Create subrouters for different components
	gstreamerAPI := router.PathPrefix("/api/gstreamer").Subrouter()
	webrtcAPI := router.PathPrefix("/api/webrtc").Subrouter()
	bridgeAPI := router.PathPrefix("/api/bridge").Subrouter()

	// Signaling WebSocket route
	router.HandleFunc("/api/signaling", a.handleSignalingWebSocket).Methods("GET")

	// GStreamer routes
	gstreamerAPI.HandleFunc("/status", a.handleGStreamerStatus).Methods("GET")
	gstreamerAPI.HandleFunc("/start", a.handleGStreamerStart).Methods("POST")
	gstreamerAPI.HandleFunc("/stop", a.handleGStreamerStop).Methods("POST")
	gstreamerAPI.HandleFunc("/stats", a.handleGStreamerStats).Methods("GET")

	// WebRTC routes
	webrtcAPI.HandleFunc("/status", a.handleWebRTCStatus).Methods("GET")
	webrtcAPI.HandleFunc("/start", a.handleWebRTCStart).Methods("POST")
	webrtcAPI.HandleFunc("/stop", a.handleWebRTCStop).Methods("POST")
	webrtcAPI.HandleFunc("/stats", a.handleWebRTCStats).Methods("GET")
	webrtcAPI.HandleFunc("/offer", a.handleWebRTCOffer).Methods("POST")
	webrtcAPI.HandleFunc("/answer", a.handleWebRTCAnswer).Methods("POST")
	webrtcAPI.HandleFunc("/ice-candidate", a.handleWebRTCICECandidate).Methods("POST")

	// Media bridge routes
	bridgeAPI.HandleFunc("/status", a.handleBridgeStatus).Methods("GET")
	bridgeAPI.HandleFunc("/start", a.handleBridgeStart).Methods("POST")
	bridgeAPI.HandleFunc("/stop", a.handleBridgeStop).Methods("POST")
	bridgeAPI.HandleFunc("/restart", a.handleBridgeRestart).Methods("POST")
	bridgeAPI.HandleFunc("/stats", a.handleBridgeStats).Methods("GET")

	// Implementation switch routes (for toggling between old and new implementations)
	router.HandleFunc("/api/implementation/status", a.handleImplementationStatus).Methods("GET")
	router.HandleFunc("/api/implementation/switch", a.handleImplementationSwitch).Methods("POST")

	a.logger.Debug("HTTP routes setup completed for simplified adapter")
	return nil
}

// HTTP handlers for GStreamer endpoints

func (a *SimplifiedAdapter) handleGStreamerStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"running": a.gstreamerManager.IsRunning(),
		"type":    "simplified",
	}
	a.writeJSON(w, status)
}

func (a *SimplifiedAdapter) handleGStreamerStart(w http.ResponseWriter, r *http.Request) {
	if err := a.gstreamerManager.Start(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start GStreamer: %v", err), http.StatusInternalServerError)
		return
	}

	a.writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "GStreamer started successfully",
	})
}

func (a *SimplifiedAdapter) handleGStreamerStop(w http.ResponseWriter, r *http.Request) {
	if err := a.gstreamerManager.Stop(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop GStreamer: %v", err), http.StatusInternalServerError)
		return
	}

	a.writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "GStreamer stopped successfully",
	})
}

func (a *SimplifiedAdapter) handleGStreamerStats(w http.ResponseWriter, r *http.Request) {
	stats := map[string]interface{}{
		"running": a.gstreamerManager.IsRunning(),
		"type":    "simplified",
	}
	a.writeJSON(w, stats)
}

// HTTP handlers for WebRTC endpoints

func (a *SimplifiedAdapter) handleWebRTCStatus(w http.ResponseWriter, r *http.Request) {
	status := a.webrtcManager.GetStats()
	status["type"] = "minimal"
	a.writeJSON(w, status)
}

func (a *SimplifiedAdapter) handleWebRTCStart(w http.ResponseWriter, r *http.Request) {
	if err := a.webrtcManager.Start(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start WebRTC: %v", err), http.StatusInternalServerError)
		return
	}

	a.writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "WebRTC started successfully",
	})
}

func (a *SimplifiedAdapter) handleWebRTCStop(w http.ResponseWriter, r *http.Request) {
	if err := a.webrtcManager.Stop(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop WebRTC: %v", err), http.StatusInternalServerError)
		return
	}

	a.writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "WebRTC stopped successfully",
	})
}

func (a *SimplifiedAdapter) handleWebRTCStats(w http.ResponseWriter, r *http.Request) {
	stats := a.webrtcManager.GetStats()
	a.writeJSON(w, stats)
}

func (a *SimplifiedAdapter) handleWebRTCOffer(w http.ResponseWriter, r *http.Request) {
	offer, err := a.webrtcManager.CreateOffer()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create offer: %v", err), http.StatusInternalServerError)
		return
	}

	a.writeJSON(w, map[string]interface{}{
		"type": offer.Type.String(),
		"sdp":  offer.SDP,
	})
}

func (a *SimplifiedAdapter) handleWebRTCAnswer(w http.ResponseWriter, r *http.Request) {
	answer, err := a.webrtcManager.CreateAnswer()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create answer: %v", err), http.StatusInternalServerError)
		return
	}

	a.writeJSON(w, map[string]interface{}{
		"type": answer.Type.String(),
		"sdp":  answer.SDP,
	})
}

func (a *SimplifiedAdapter) handleWebRTCICECandidate(w http.ResponseWriter, r *http.Request) {
	candidates := a.webrtcManager.GetICECandidates()

	a.writeJSON(w, map[string]interface{}{
		"candidates": candidates,
		"count":      len(candidates),
	})
}

// HTTP handlers for Media Bridge endpoints

func (a *SimplifiedAdapter) handleBridgeStatus(w http.ResponseWriter, r *http.Request) {
	status := a.mediaBridge.GetStats()
	status["type"] = "simple"
	a.writeJSON(w, status)
}

func (a *SimplifiedAdapter) handleBridgeStart(w http.ResponseWriter, r *http.Request) {
	if err := a.mediaBridge.Start(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start media bridge: %v", err), http.StatusInternalServerError)
		return
	}

	a.writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Media bridge started successfully",
	})
}

func (a *SimplifiedAdapter) handleBridgeStop(w http.ResponseWriter, r *http.Request) {
	if err := a.mediaBridge.Stop(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop media bridge: %v", err), http.StatusInternalServerError)
		return
	}

	a.writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Media bridge stopped successfully",
	})
}

func (a *SimplifiedAdapter) handleBridgeRestart(w http.ResponseWriter, r *http.Request) {
	if err := a.mediaBridge.Restart(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to restart media bridge: %v", err), http.StatusInternalServerError)
		return
	}

	a.writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Media bridge restarted successfully",
	})
}

func (a *SimplifiedAdapter) handleBridgeStats(w http.ResponseWriter, r *http.Request) {
	stats := a.mediaBridge.GetStats()
	a.writeJSON(w, stats)
}

// HTTP handlers for implementation switching

func (a *SimplifiedAdapter) handleImplementationStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"current_implementation":    "simplified",
		"available_implementations": []string{"simplified", "complex"},
		"simplified_enabled":        a.enabled,
		"simplified_running":        a.running,
	}
	a.writeJSON(w, status)
}

func (a *SimplifiedAdapter) handleImplementationSwitch(w http.ResponseWriter, r *http.Request) {
	// For now, just return information about the switch capability
	// Actual implementation switching would require application-level coordination
	response := map[string]interface{}{
		"status":  "info",
		"message": "Implementation switching requires application restart",
		"current": "simplified",
		"note":    "Use configuration to select implementation at startup",
	}
	a.writeJSON(w, response)
}

// Utility methods

func (a *SimplifiedAdapter) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		a.logger.Errorf("Failed to encode JSON response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// GetGStreamerManager returns the command GStreamer manager
func (a *SimplifiedAdapter) GetGStreamerManager() *gstreamer.CommandGStreamerManager {
	return a.gstreamerManager
}

// GetWebRTCManager returns the minimal WebRTC manager
func (a *SimplifiedAdapter) GetWebRTCManager() *webrtcpkg.MinimalWebRTCManager {
	return a.webrtcManager
}

// GetMediaBridge returns the go-gst media bridge
func (a *SimplifiedAdapter) GetMediaBridge() *bridge.GoGstMediaBridge {
	return a.mediaBridge
}

// SetEnabled allows enabling/disabling the simplified adapter
func (a *SimplifiedAdapter) SetEnabled(enabled bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.enabled = enabled
	a.logger.Debugf("Simplified adapter enabled status changed to: %v", enabled)
}

// handleSignalingWebSocket handles WebSocket connections for signaling
func (a *SimplifiedAdapter) handleSignalingWebSocket(w http.ResponseWriter, r *http.Request) {
	a.logger.Info("WebSocket signaling connection request received")

	if a.signalingServer == nil {
		a.logger.Error("Signaling server not available")
		http.Error(w, "Signaling server not available", http.StatusServiceUnavailable)
		return
	}

	// Start media bridge if not already running (lazy initialization)
	if !a.mediaBridge.IsRunning() {
		a.logger.Info("Starting media bridge for first client connection...")
		if err := a.mediaBridge.Start(); err != nil {
			a.logger.Errorf("Failed to start media bridge: %v", err)
			http.Error(w, "Failed to start media capture", http.StatusInternalServerError)
			return
		}
		a.logger.Info("Media bridge started successfully")
	}

	// Set the WebRTC manager in the signaling server for direct access
	a.signalingServer.SetWebRTCManager(a.webrtcManager)

	// Delegate to the signaling server's WebSocket handler
	a.signalingServer.HandleWebSocket(w, r)
}

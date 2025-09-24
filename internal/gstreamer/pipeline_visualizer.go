package gstreamer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/sirupsen/logrus"
)

// PipelineVisualizer provides pipeline visualization and debugging capabilities
type PipelineVisualizer struct {
	logger *logrus.Entry
	mu     sync.RWMutex

	// Configuration
	config *VisualizerConfig

	// Pipeline tracking
	pipelines map[string]*PipelineInfo

	// Export settings
	outputDir     string
	enableExport  bool
	exportFormats []ExportFormat
}

// VisualizerConfig configures the pipeline visualizer
type VisualizerConfig struct {
	OutputDirectory     string
	EnableAutoExport    bool
	ExportOnError       bool
	ExportOnStateChange bool
	MaxExportFiles      int
	ExportFormats       []ExportFormat
	IncludeTimestamp    bool
	IncludeMetadata     bool
}

// PipelineInfo contains information about a tracked pipeline
type PipelineInfo struct {
	Name        string
	Pipeline    *gst.Pipeline
	Elements    map[string]*ElementInfo
	Links       []LinkInfo
	State       gst.State
	CreatedAt   time.Time
	LastUpdated time.Time
	ErrorCount  int64
	Metadata    map[string]interface{}
}

// ElementInfo contains information about a pipeline element
type ElementInfo struct {
	Name       string
	Factory    string
	Element    *gst.Element
	Properties map[string]interface{}
	Pads       []PadInfo
	State      gst.State
	Position   Position
	Metadata   map[string]interface{}
}

// LinkInfo contains information about element links
type LinkInfo struct {
	SourceElement string
	SourcePad     string
	SinkElement   string
	SinkPad       string
	Caps          string
	Active        bool
}

// PadInfo contains information about element pads
type PadInfo struct {
	Name      string
	Direction gst.PadDirection
	Template  string
	Caps      string
	Linked    bool
	Peer      string
}

// Position represents element position in visualization
type Position struct {
	X, Y int
}

// ExportFormat defines the export format for pipeline visualization
type ExportFormat int

const (
	ExportDOT ExportFormat = iota
	ExportSVG
	ExportPNG
	ExportJSON
	ExportText
)

// String returns the string representation of ExportFormat
func (ef ExportFormat) String() string {
	switch ef {
	case ExportDOT:
		return "DOT"
	case ExportSVG:
		return "SVG"
	case ExportPNG:
		return "PNG"
	case ExportJSON:
		return "JSON"
	case ExportText:
		return "Text"
	default:
		return "Unknown"
	}
}

// FileExtension returns the file extension for the export format
func (ef ExportFormat) FileExtension() string {
	switch ef {
	case ExportDOT:
		return ".dot"
	case ExportSVG:
		return ".svg"
	case ExportPNG:
		return ".png"
	case ExportJSON:
		return ".json"
	case ExportText:
		return ".txt"
	default:
		return ".unknown"
	}
}

// NewPipelineVisualizer creates a new pipeline visualizer
func NewPipelineVisualizer(logger *logrus.Entry, config *VisualizerConfig) *PipelineVisualizer {
	if config == nil {
		config = DefaultVisualizerConfig()
	}

	if logger == nil {
		logger = logrus.WithField("component", "pipeline-visualizer")
	}

	visualizer := &PipelineVisualizer{
		logger:        logger,
		config:        config,
		pipelines:     make(map[string]*PipelineInfo),
		outputDir:     config.OutputDirectory,
		enableExport:  config.EnableAutoExport,
		exportFormats: config.ExportFormats,
	}

	// Create output directory if it doesn't exist
	if config.OutputDirectory != "" {
		if err := os.MkdirAll(config.OutputDirectory, 0755); err != nil {
			logger.Warnf("Failed to create visualization output directory: %v", err)
		}
	}

	return visualizer
}

// DefaultVisualizerConfig returns default visualizer configuration
func DefaultVisualizerConfig() *VisualizerConfig {
	return &VisualizerConfig{
		OutputDirectory:     "./pipeline_debug",
		EnableAutoExport:    true,
		ExportOnError:       true,
		ExportOnStateChange: false,
		MaxExportFiles:      100,
		ExportFormats:       []ExportFormat{ExportDOT, ExportJSON, ExportText},
		IncludeTimestamp:    true,
		IncludeMetadata:     true,
	}
}

// RegisterPipeline registers a pipeline for visualization
func (pv *PipelineVisualizer) RegisterPipeline(name string, pipeline *gst.Pipeline) error {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	if pipeline == nil {
		return fmt.Errorf("pipeline is nil")
	}

	pipelineInfo := &PipelineInfo{
		Name:        name,
		Pipeline:    pipeline,
		Elements:    make(map[string]*ElementInfo),
		Links:       make([]LinkInfo, 0),
		State:       pipeline.GetCurrentState(),
		CreatedAt:   time.Now(),
		LastUpdated: time.Now(),
		Metadata:    make(map[string]interface{}),
	}

	// Analyze pipeline structure
	if err := pv.analyzePipelineStructure(pipelineInfo); err != nil {
		pv.logger.Warnf("Failed to analyze pipeline structure for %s: %v", name, err)
	}

	pv.pipelines[name] = pipelineInfo
	pv.logger.Debugf("Registered pipeline for visualization: %s", name)

	// Auto-export if enabled
	if pv.enableExport {
		go pv.exportPipeline(name, "registration")
	}

	return nil
}

// UnregisterPipeline unregisters a pipeline from visualization
func (pv *PipelineVisualizer) UnregisterPipeline(name string) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	delete(pv.pipelines, name)
	pv.logger.Debugf("Unregistered pipeline from visualization: %s", name)
}

// UpdatePipelineState updates the state of a registered pipeline
func (pv *PipelineVisualizer) UpdatePipelineState(name string, newState gst.State) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	pipelineInfo, exists := pv.pipelines[name]
	if !exists {
		pv.logger.Warnf("Pipeline %s not registered for visualization", name)
		return
	}

	oldState := pipelineInfo.State
	pipelineInfo.State = newState
	pipelineInfo.LastUpdated = time.Now()

	pv.logger.Debugf("Pipeline %s state changed: %s -> %s", name, oldState.String(), newState.String())

	// Export on state change if enabled
	if pv.config.ExportOnStateChange && pv.enableExport {
		go pv.exportPipeline(name, fmt.Sprintf("state_change_%s_to_%s", oldState.String(), newState.String()))
	}
}

// RecordError records an error for a pipeline
func (pv *PipelineVisualizer) RecordError(pipelineName string, errorType GStreamerErrorType, message string) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	pipelineInfo, exists := pv.pipelines[pipelineName]
	if !exists {
		pv.logger.Warnf("Pipeline %s not registered for visualization", pipelineName)
		return
	}

	pipelineInfo.ErrorCount++
	pipelineInfo.LastUpdated = time.Now()

	// Add error to metadata
	if pipelineInfo.Metadata["errors"] == nil {
		pipelineInfo.Metadata["errors"] = make([]map[string]interface{}, 0)
	}

	errorInfo := map[string]interface{}{
		"type":      errorType.String(),
		"message":   message,
		"timestamp": time.Now(),
	}

	errors := pipelineInfo.Metadata["errors"].([]map[string]interface{})
	pipelineInfo.Metadata["errors"] = append(errors, errorInfo)

	pv.logger.Debugf("Recorded error for pipeline %s: %s - %s", pipelineName, errorType.String(), message)

	// Export on error if enabled
	if pv.config.ExportOnError && pv.enableExport {
		go pv.exportPipeline(pipelineName, fmt.Sprintf("error_%s", errorType.String()))
	}
}

// analyzePipelineStructure analyzes the structure of a pipeline
func (pv *PipelineVisualizer) analyzePipelineStructure(pipelineInfo *PipelineInfo) error {
	// Note: This is a simplified implementation
	// In a full implementation, you would iterate through all elements in the pipeline
	// and analyze their connections, properties, etc.

	// For now, we'll create a placeholder structure
	pv.logger.Debugf("Analyzing pipeline structure for %s", pipelineInfo.Name)

	// This would typically involve:
	// 1. Iterating through all elements in the pipeline
	// 2. Getting element properties and pad information
	// 3. Analyzing element links and data flow
	// 4. Calculating element positions for visualization

	return nil
}

// exportPipeline exports a pipeline visualization
func (pv *PipelineVisualizer) exportPipeline(name, reason string) {
	pv.mu.RLock()
	pipelineInfo, exists := pv.pipelines[name]
	pv.mu.RUnlock()

	if !exists {
		pv.logger.Warnf("Pipeline %s not found for export", name)
		return
	}

	timestamp := ""
	if pv.config.IncludeTimestamp {
		timestamp = fmt.Sprintf("_%s", time.Now().Format("20060102_150405"))
	}

	baseFilename := fmt.Sprintf("%s_%s%s", name, reason, timestamp)

	for _, format := range pv.exportFormats {
		filename := baseFilename + format.FileExtension()
		filepath := filepath.Join(pv.outputDir, filename)

		if err := pv.exportToFormat(pipelineInfo, format, filepath); err != nil {
			pv.logger.Errorf("Failed to export pipeline %s to %s: %v", name, format.String(), err)
		} else {
			pv.logger.Debugf("Exported pipeline %s to %s: %s", name, format.String(), filepath)
		}
	}

	// Clean up old export files if needed
	pv.cleanupOldExports()
}

// exportToFormat exports pipeline to a specific format
func (pv *PipelineVisualizer) exportToFormat(pipelineInfo *PipelineInfo, format ExportFormat, filepath string) error {
	switch format {
	case ExportDOT:
		return pv.exportToDOT(pipelineInfo, filepath)
	case ExportJSON:
		return pv.exportToJSON(pipelineInfo, filepath)
	case ExportText:
		return pv.exportToText(pipelineInfo, filepath)
	case ExportSVG:
		return pv.exportToSVG(pipelineInfo, filepath)
	case ExportPNG:
		return pv.exportToPNG(pipelineInfo, filepath)
	default:
		return fmt.Errorf("unsupported export format: %s", format.String())
	}
}

// exportToDOT exports pipeline to DOT format (Graphviz)
func (pv *PipelineVisualizer) exportToDOT(pipelineInfo *PipelineInfo, filepath string) error {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("digraph \"%s\" {\n", pipelineInfo.Name))
	builder.WriteString("  rankdir=LR;\n")
	builder.WriteString("  node [shape=box];\n\n")

	// Add metadata as comment
	if pv.config.IncludeMetadata {
		builder.WriteString(fmt.Sprintf("  // Pipeline: %s\n", pipelineInfo.Name))
		builder.WriteString(fmt.Sprintf("  // State: %s\n", pipelineInfo.State.String()))
		builder.WriteString(fmt.Sprintf("  // Created: %s\n", pipelineInfo.CreatedAt.Format(time.RFC3339)))
		builder.WriteString(fmt.Sprintf("  // Last Updated: %s\n", pipelineInfo.LastUpdated.Format(time.RFC3339)))
		builder.WriteString(fmt.Sprintf("  // Error Count: %d\n\n", pipelineInfo.ErrorCount))
	}

	// Add elements
	for _, element := range pipelineInfo.Elements {
		label := fmt.Sprintf("%s\\n(%s)", element.Name, element.Factory)
		color := pv.getElementColor(element)
		builder.WriteString(fmt.Sprintf("  \"%s\" [label=\"%s\", color=\"%s\"];\n",
			element.Name, label, color))
	}

	builder.WriteString("\n")

	// Add links
	for _, link := range pipelineInfo.Links {
		style := "solid"
		if !link.Active {
			style = "dashed"
		}
		builder.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\" [style=\"%s\"];\n",
			link.SourceElement, link.SinkElement, style))
	}

	builder.WriteString("}\n")

	return os.WriteFile(filepath, []byte(builder.String()), 0644)
}

// exportToJSON exports pipeline to JSON format
func (pv *PipelineVisualizer) exportToJSON(pipelineInfo *PipelineInfo, filepath string) error {
	// Create a simplified JSON representation
	data := map[string]interface{}{
		"name":         pipelineInfo.Name,
		"state":        pipelineInfo.State.String(),
		"created_at":   pipelineInfo.CreatedAt,
		"last_updated": pipelineInfo.LastUpdated,
		"error_count":  pipelineInfo.ErrorCount,
		"elements":     make([]map[string]interface{}, 0),
		"links":        make([]map[string]interface{}, 0),
	}

	if pv.config.IncludeMetadata {
		data["metadata"] = pipelineInfo.Metadata
	}

	// Add elements
	for _, element := range pipelineInfo.Elements {
		elementData := map[string]interface{}{
			"name":    element.Name,
			"factory": element.Factory,
			"state":   element.State.String(),
		}
		if pv.config.IncludeMetadata {
			elementData["properties"] = element.Properties
			elementData["metadata"] = element.Metadata
		}
		data["elements"] = append(data["elements"].([]map[string]interface{}), elementData)
	}

	// Add links
	for _, link := range pipelineInfo.Links {
		linkData := map[string]interface{}{
			"source_element": link.SourceElement,
			"source_pad":     link.SourcePad,
			"sink_element":   link.SinkElement,
			"sink_pad":       link.SinkPad,
			"active":         link.Active,
		}
		if link.Caps != "" {
			linkData["caps"] = link.Caps
		}
		data["links"] = append(data["links"].([]map[string]interface{}), linkData)
	}

	// Convert to JSON and write to file
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return os.WriteFile(filepath, jsonData, 0644)
}

// exportToText exports pipeline to human-readable text format
func (pv *PipelineVisualizer) exportToText(pipelineInfo *PipelineInfo, filepath string) error {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Pipeline Visualization: %s\n", pipelineInfo.Name))
	builder.WriteString(strings.Repeat("=", 50) + "\n\n")

	// Pipeline information
	builder.WriteString("Pipeline Information:\n")
	builder.WriteString(fmt.Sprintf("  Name: %s\n", pipelineInfo.Name))
	builder.WriteString(fmt.Sprintf("  State: %s\n", pipelineInfo.State.String()))
	builder.WriteString(fmt.Sprintf("  Created: %s\n", pipelineInfo.CreatedAt.Format(time.RFC3339)))
	builder.WriteString(fmt.Sprintf("  Last Updated: %s\n", pipelineInfo.LastUpdated.Format(time.RFC3339)))
	builder.WriteString(fmt.Sprintf("  Error Count: %d\n", pipelineInfo.ErrorCount))
	builder.WriteString(fmt.Sprintf("  Element Count: %d\n", len(pipelineInfo.Elements)))
	builder.WriteString(fmt.Sprintf("  Link Count: %d\n\n", len(pipelineInfo.Links)))

	// Elements
	if len(pipelineInfo.Elements) > 0 {
		builder.WriteString("Elements:\n")
		for _, element := range pipelineInfo.Elements {
			builder.WriteString(fmt.Sprintf("  - %s (%s) [%s]\n",
				element.Name, element.Factory, element.State.String()))
		}
		builder.WriteString("\n")
	}

	// Links
	if len(pipelineInfo.Links) > 0 {
		builder.WriteString("Links:\n")
		for _, link := range pipelineInfo.Links {
			status := "active"
			if !link.Active {
				status = "inactive"
			}
			builder.WriteString(fmt.Sprintf("  %s -> %s (%s)\n",
				link.SourceElement, link.SinkElement, status))
		}
		builder.WriteString("\n")
	}

	// Metadata
	if pv.config.IncludeMetadata && len(pipelineInfo.Metadata) > 0 {
		builder.WriteString("Metadata:\n")
		for key, value := range pipelineInfo.Metadata {
			builder.WriteString(fmt.Sprintf("  %s: %v\n", key, value))
		}
		builder.WriteString("\n")
	}

	return os.WriteFile(filepath, []byte(builder.String()), 0644)
}

// exportToSVG exports pipeline to SVG format (placeholder)
func (pv *PipelineVisualizer) exportToSVG(pipelineInfo *PipelineInfo, filepath string) error {
	// This would require a more complex implementation with SVG generation
	// For now, we'll create a simple SVG placeholder
	svg := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg width="800" height="600" xmlns="http://www.w3.org/2000/svg">
  <text x="10" y="30" font-family="Arial" font-size="16">Pipeline: %s</text>
  <text x="10" y="60" font-family="Arial" font-size="12">State: %s</text>
  <text x="10" y="80" font-family="Arial" font-size="12">Elements: %d</text>
  <text x="10" y="100" font-family="Arial" font-size="12">Links: %d</text>
  <text x="10" y="120" font-family="Arial" font-size="12">Errors: %d</text>
</svg>`, pipelineInfo.Name, pipelineInfo.State.String(),
		len(pipelineInfo.Elements), len(pipelineInfo.Links), pipelineInfo.ErrorCount)

	return os.WriteFile(filepath, []byte(svg), 0644)
}

// exportToPNG exports pipeline to PNG format (placeholder)
func (pv *PipelineVisualizer) exportToPNG(pipelineInfo *PipelineInfo, filepath string) error {
	// This would require image generation capabilities
	// For now, we'll return an error indicating it's not implemented
	return fmt.Errorf("PNG export not implemented - requires image generation library")
}

// getElementColor returns a color for an element based on its state
func (pv *PipelineVisualizer) getElementColor(element *ElementInfo) string {
	switch element.State {
	case gst.StatePlaying:
		return "green"
	case gst.StatePaused:
		return "yellow"
	case gst.StateReady:
		return "blue"
	case gst.StateNull:
		return "gray"
	default:
		return "red"
	}
}

// cleanupOldExports removes old export files if the limit is exceeded
func (pv *PipelineVisualizer) cleanupOldExports() {
	if pv.config.MaxExportFiles <= 0 {
		return
	}

	// This is a simplified cleanup - in a full implementation,
	// you would scan the output directory and remove old files
	pv.logger.Debug("Cleaning up old export files (placeholder)")
}

// ExportPipeline manually exports a pipeline visualization
func (pv *PipelineVisualizer) ExportPipeline(name, reason string) error {
	pv.mu.RLock()
	_, exists := pv.pipelines[name]
	pv.mu.RUnlock()

	if !exists {
		return fmt.Errorf("pipeline %s not registered", name)
	}

	go pv.exportPipeline(name, reason)
	return nil
}

// ExportAllPipelines exports all registered pipelines
func (pv *PipelineVisualizer) ExportAllPipelines(reason string) error {
	pv.mu.RLock()
	names := make([]string, 0, len(pv.pipelines))
	for name := range pv.pipelines {
		names = append(names, name)
	}
	pv.mu.RUnlock()

	for _, name := range names {
		go pv.exportPipeline(name, reason)
	}

	return nil
}

// GetPipelineInfo returns information about a registered pipeline
func (pv *PipelineVisualizer) GetPipelineInfo(name string) (*PipelineInfo, error) {
	pv.mu.RLock()
	defer pv.mu.RUnlock()

	pipelineInfo, exists := pv.pipelines[name]
	if !exists {
		return nil, fmt.Errorf("pipeline %s not registered", name)
	}

	// Return a copy to prevent external modification
	return &PipelineInfo{
		Name:        pipelineInfo.Name,
		State:       pipelineInfo.State,
		CreatedAt:   pipelineInfo.CreatedAt,
		LastUpdated: pipelineInfo.LastUpdated,
		ErrorCount:  pipelineInfo.ErrorCount,
		// Note: Elements, Links, and Metadata would need deep copying in a full implementation
	}, nil
}

// ListPipelines returns a list of registered pipeline names
func (pv *PipelineVisualizer) ListPipelines() []string {
	pv.mu.RLock()
	defer pv.mu.RUnlock()

	names := make([]string, 0, len(pv.pipelines))
	for name := range pv.pipelines {
		names = append(names, name)
	}

	return names
}

// SetExportEnabled enables or disables automatic export
func (pv *PipelineVisualizer) SetExportEnabled(enabled bool) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	pv.enableExport = enabled
	pv.logger.Debugf("Pipeline visualization export %s", map[bool]string{true: "enabled", false: "disabled"}[enabled])
}

// IsExportEnabled returns whether automatic export is enabled
func (pv *PipelineVisualizer) IsExportEnabled() bool {
	pv.mu.RLock()
	defer pv.mu.RUnlock()

	return pv.enableExport
}

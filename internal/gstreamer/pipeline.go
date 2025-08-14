package gstreamer

/*
#cgo pkg-config: gstreamer-1.0
#include <gst/gst.h>
#include <stdlib.h>

// Helper function to parse pipeline description with error handling
static GstElement* parse_launch_with_error(const gchar *pipeline_description, GError **error) {
    return gst_parse_launch(pipeline_description, error);
}

// Helper function to dump pipeline to dot file
static void pipeline_dump_dot_file(GstElement *pipeline, const gchar *filename) {
    GST_DEBUG_BIN_TO_DOT_FILE(GST_BIN(pipeline), GST_DEBUG_GRAPH_SHOW_ALL, filename);
}

// Helper function to get pipeline position
static gboolean pipeline_query_position(GstElement *pipeline, gint64 *position) {
    return gst_element_query_position(pipeline, GST_FORMAT_TIME, position);
}

// Helper function to get pipeline duration
static gboolean pipeline_query_duration(GstElement *pipeline, gint64 *duration) {
    return gst_element_query_duration(pipeline, GST_FORMAT_TIME, duration);
}

// Helper function to seek in pipeline
static gboolean pipeline_seek(GstElement *pipeline, gdouble rate, gint64 start, gint64 stop) {
    return gst_element_seek(pipeline,
                           rate,
                           GST_FORMAT_TIME,
                           GST_SEEK_FLAG_FLUSH | GST_SEEK_FLAG_ACCURATE,
                           GST_SEEK_TYPE_SET, start,
                           GST_SEEK_TYPE_SET, stop);
}
*/
import "C"
import (
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"
)

// PipelineManager manages GStreamer pipelines
type PipelineManager struct {
	pipelines map[string]Pipeline
	mu        sync.RWMutex
}

// NewPipelineManager creates a new pipeline manager
func NewPipelineManager() *PipelineManager {
	return &PipelineManager{
		pipelines: make(map[string]Pipeline),
	}
}

// CreatePipeline creates a new pipeline with the given name
func (pm *PipelineManager) CreatePipeline(name string) (Pipeline, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.pipelines[name]; exists {
		return nil, fmt.Errorf("pipeline with name '%s' already exists", name)
	}

	pipeline := NewPipeline(name)
	err := pipeline.Create()
	if err != nil {
		return nil, err
	}

	pm.pipelines[name] = pipeline
	return pipeline, nil
}

// CreatePipelineFromString creates a new pipeline from a string description
func (pm *PipelineManager) CreatePipelineFromString(name, pipelineStr string) (Pipeline, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.pipelines[name]; exists {
		return nil, fmt.Errorf("pipeline with name '%s' already exists", name)
	}

	pipeline := NewPipeline(name)
	err := pipeline.CreateFromString(pipelineStr)
	if err != nil {
		return nil, err
	}

	pm.pipelines[name] = pipeline
	return pipeline, nil
}

// GetPipeline gets a pipeline by name
func (pm *PipelineManager) GetPipeline(name string) (Pipeline, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	pipeline, exists := pm.pipelines[name]
	if !exists {
		return nil, fmt.Errorf("pipeline with name '%s' not found", name)
	}

	return pipeline, nil
}

// DestroyPipeline destroys a pipeline by name
func (pm *PipelineManager) DestroyPipeline(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pipeline, exists := pm.pipelines[name]
	if !exists {
		return fmt.Errorf("pipeline with name '%s' not found", name)
	}

	err := pipeline.Stop()
	if err != nil {
		return err
	}

	delete(pm.pipelines, name)
	return nil
}

// DestroyAllPipelines destroys all pipelines
func (pm *PipelineManager) DestroyAllPipelines() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var lastErr error
	for name, pipeline := range pm.pipelines {
		err := pipeline.Stop()
		if err != nil {
			lastErr = err
		}
		delete(pm.pipelines, name)
	}

	return lastErr
}

// PipelineExists checks if a pipeline with the given name exists
func (pm *PipelineManager) PipelineExists(name string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	_, exists := pm.pipelines[name]
	return exists
}

// GetPipelineCount returns the number of pipelines
func (pm *PipelineManager) GetPipelineCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return len(pm.pipelines)
}

// GetPipelineNames returns the names of all pipelines
func (pm *PipelineManager) GetPipelineNames() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	names := make([]string, 0, len(pm.pipelines))
	for name := range pm.pipelines {
		names = append(names, name)
	}

	return names
}

// Enhanced pipeline implementation with additional features
type enhancedPipeline struct {
	*pipeline
	name string
}

// NewEnhancedPipeline creates a new enhanced pipeline
func NewEnhancedPipeline(name string) Pipeline {
	return &enhancedPipeline{
		pipeline: &pipeline{
			elements: make(map[string]*C.GstElement),
		},
		name: name,
	}
}

// DumpDotFile dumps the pipeline to a dot file for visualization
func DumpDotFile(p Pipeline, filename string) error {
	ep, ok := p.(*enhancedPipeline)
	if !ok {
		return errors.New("not an enhanced pipeline")
	}

	if ep.gstPipeline == nil {
		return ErrGStreamerNotInitialized
	}

	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))

	C.pipeline_dump_dot_file(ep.gstPipeline, cFilename)
	return nil
}

// GetPosition gets the current position of the pipeline in nanoseconds
func GetPosition(p Pipeline) (time.Duration, error) {
	ep, ok := p.(*enhancedPipeline)
	if !ok {
		return 0, errors.New("not an enhanced pipeline")
	}

	if ep.gstPipeline == nil {
		return 0, ErrGStreamerNotInitialized
	}

	var position C.gint64
	ret := C.pipeline_query_position(ep.gstPipeline, &position)
	if ret == 0 {
		return 0, errors.New("failed to query position")
	}

	return time.Duration(position), nil
}

// GetDuration gets the duration of the pipeline in nanoseconds
func GetDuration(p Pipeline) (time.Duration, error) {
	ep, ok := p.(*enhancedPipeline)
	if !ok {
		return 0, errors.New("not an enhanced pipeline")
	}

	if ep.gstPipeline == nil {
		return 0, ErrGStreamerNotInitialized
	}

	var duration C.gint64
	ret := C.pipeline_query_duration(ep.gstPipeline, &duration)
	if ret == 0 {
		return 0, errors.New("failed to query duration")
	}

	return time.Duration(duration), nil
}

// Seek seeks to a specific position in the pipeline
func Seek(p Pipeline, position time.Duration) error {
	ep, ok := p.(*enhancedPipeline)
	if !ok {
		return errors.New("not an enhanced pipeline")
	}

	if ep.gstPipeline == nil {
		return ErrGStreamerNotInitialized
	}

	ret := C.pipeline_seek(ep.gstPipeline, 1.0, C.gint64(position), -1)
	if ret == 0 {
		return errors.New("failed to seek")
	}

	return nil
}

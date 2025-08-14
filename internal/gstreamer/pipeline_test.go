package gstreamer

import (
	"fmt"
	"testing"
	"time"
)

func TestPipelineManager(t *testing.T) {
	// Create a pipeline manager
	pm := NewPipelineManager()
	if pm == nil {
		t.Fatal("Failed to create pipeline manager")
	}

	// Create a pipeline
	pipeline, err := pm.CreatePipeline("test-pipeline")
	if err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	// Check if pipeline exists
	if !pm.PipelineExists("test-pipeline") {
		t.Error("Expected pipeline to exist")
	}

	// Get pipeline count
	count := pm.GetPipelineCount()
	if count != 1 {
		t.Errorf("Expected pipeline count to be 1, got %d", count)
	}

	// Get pipeline names
	names := pm.GetPipelineNames()
	if len(names) != 1 || names[0] != "test-pipeline" {
		t.Errorf("Expected pipeline names to be [test-pipeline], got %v", names)
	}

	// Get pipeline by name
	p, err := pm.GetPipeline("test-pipeline")
	if err != nil {
		t.Errorf("Failed to get pipeline: %v", err)
	}
	if p != pipeline {
		t.Error("Expected to get the same pipeline")
	}

	// Destroy pipeline
	err = pm.DestroyPipeline("test-pipeline")
	if err != nil {
		t.Errorf("Failed to destroy pipeline: %v", err)
	}

	// Check if pipeline exists after destruction
	if pm.PipelineExists("test-pipeline") {
		t.Error("Expected pipeline to not exist after destruction")
	}
}

func TestPipelineFromString(t *testing.T) {
	// Create a pipeline manager
	pm := NewPipelineManager()
	if pm == nil {
		t.Fatal("Failed to create pipeline manager")
	}

	// Create a pipeline from string
	pipeline, err := pm.CreatePipelineFromString("test-pipeline", "videotestsrc ! fakesink")
	if err != nil {
		t.Fatalf("Failed to create pipeline from string: %v", err)
	}

	// Start the pipeline
	err = pipeline.Start()
	if err != nil {
		t.Errorf("Failed to start pipeline: %v", err)
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Stop the pipeline
	err = pipeline.Stop()
	if err != nil {
		t.Errorf("Failed to stop pipeline: %v", err)
	}

	// Destroy pipeline
	err = pm.DestroyPipeline("test-pipeline")
	if err != nil {
		t.Logf("Note: Failed to destroy pipeline: %v", err)
		// This might be expected if the pipeline is already cleaned up
	}
}

func TestMultiplePipelines(t *testing.T) {
	// Create a pipeline manager
	pm := NewPipelineManager()
	if pm == nil {
		t.Fatal("Failed to create pipeline manager")
	}

	// Create multiple pipelines
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("test-pipeline-%d", i)
		_, err := pm.CreatePipeline(name)
		if err != nil {
			t.Fatalf("Failed to create pipeline %s: %v", name, err)
		}
	}

	// Check pipeline count
	count := pm.GetPipelineCount()
	if count != 3 {
		t.Errorf("Expected pipeline count to be 3, got %d", count)
	}

	// Destroy all pipelines
	err := pm.DestroyAllPipelines()
	if err != nil {
		t.Errorf("Failed to destroy all pipelines: %v", err)
	}

	// Check pipeline count after destruction
	count = pm.GetPipelineCount()
	if count != 0 {
		t.Errorf("Expected pipeline count to be 0 after destruction, got %d", count)
	}
}

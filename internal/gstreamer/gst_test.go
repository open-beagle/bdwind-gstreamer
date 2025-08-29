package gstreamer

import (
	"testing"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGstInitialization(t *testing.T) {
	// Initialize GStreamer
	gst.Init(nil)

	// Test basic GStreamer functionality
	t.Logf("GStreamer version: %d.%d.%d", gst.VersionMajor, gst.VersionMinor, gst.VersionMicro)

	// Verify we have a valid version
	assert.Greater(t, int(gst.VersionMajor), 0)
}

func TestPipelineWrapperCreation(t *testing.T) {
	gst.Init(nil)

	// Test pipeline creation
	pipeline, err := NewPipelineWrapper("test-pipeline")
	require.NoError(t, err)
	require.NotNil(t, pipeline)

	// Test initial state
	assert.Equal(t, gst.StateNull, pipeline.GetState())
	assert.False(t, pipeline.IsRunning())
	assert.Equal(t, 0, pipeline.GetElementCount())

	// Cleanup
	err = pipeline.Cleanup()
	assert.NoError(t, err)
}

func TestElementFactoryCreation(t *testing.T) {
	gst.Init(nil)

	factory := NewElementFactory()
	require.NotNil(t, factory)

	// Test element availability check
	assert.True(t, factory.IsElementAvailable("videotestsrc"))
	assert.True(t, factory.IsElementAvailable("fakesink"))
	assert.False(t, factory.IsElementAvailable("nonexistent-element"))

	// Test element creation
	element, err := factory.CreateElement("videotestsrc")
	require.NoError(t, err)
	require.NotNil(t, element)

	// Test element with name
	namedElement, err := factory.CreateElementWithName("fakesink", "test-sink")
	require.NoError(t, err)
	require.NotNil(t, namedElement)
	assert.Equal(t, "test-sink", namedElement.GetName())

	// Cleanup
	element.Unref()
	namedElement.Unref()
}

func TestPipelineWithElements(t *testing.T) {
	gst.Init(nil)

	pipeline, err := NewPipelineWrapper("test-pipeline")
	require.NoError(t, err)
	defer pipeline.Cleanup()

	// Add elements
	src, err := pipeline.AddElement("src", "videotestsrc")
	require.NoError(t, err)
	require.NotNil(t, src)

	sink, err := pipeline.AddElement("sink", "fakesink")
	require.NoError(t, err)
	require.NotNil(t, sink)

	// Test element count
	assert.Equal(t, 2, pipeline.GetElementCount())

	// Test element retrieval
	retrievedSrc, err := pipeline.GetElement("src")
	require.NoError(t, err)
	assert.Equal(t, src, retrievedSrc)

	// Test linking
	err = pipeline.LinkElements("src", "sink")
	require.NoError(t, err)

	// Test state changes
	err = pipeline.SetState(gst.StateReady)
	assert.NoError(t, err)

	err = pipeline.SetState(gst.StateNull)
	assert.NoError(t, err)
}

func TestBusWrapperCreation(t *testing.T) {
	gst.Init(nil)

	pipeline, err := NewPipelineWrapper("test-pipeline")
	require.NoError(t, err)
	defer pipeline.Cleanup()

	busWrapper := NewBusWrapper(pipeline.GetPipeline())
	require.NotNil(t, busWrapper)

	// Test initial state
	assert.False(t, busWrapper.IsRunning())
	assert.Equal(t, 0, busWrapper.GetHandlerCount(gst.MessageError))

	// Add default handlers
	busWrapper.AddDefaultHandlers()
	assert.Greater(t, busWrapper.GetHandlerCount(gst.MessageError), 0)

	// Test start/stop
	err = busWrapper.Start()
	assert.NoError(t, err)
	assert.True(t, busWrapper.IsRunning())

	err = busWrapper.Stop()
	assert.NoError(t, err)
	assert.False(t, busWrapper.IsRunning())
}

func TestElementPropertyConfiguration(t *testing.T) {
	gst.Init(nil)

	factory := NewElementFactory()
	element, err := factory.CreateElement("videotestsrc")
	require.NoError(t, err)
	defer element.Unref()

	// Test property setting
	properties := map[string]interface{}{
		"pattern": 0, // SMPTE pattern
		"is-live": true,
	}

	err = factory.ConfigureElement(element, properties)
	assert.NoError(t, err)

	// Test individual property setting
	err = factory.SetElementProperty(element, "num-buffers", 100)
	assert.NoError(t, err)
}

func TestVideoSourceCreation(t *testing.T) {
	gst.Init(nil)

	factory := NewElementFactory()

	// Test video test source (should always be available)
	testSrc, err := factory.CreateVideoSource(VideoSourceTest, map[string]interface{}{
		"pattern": 0,
	})
	require.NoError(t, err)
	require.NotNil(t, testSrc)
	defer testSrc.Unref()

	// Verify it's the correct element type
	assert.Equal(t, "videotestsrc", testSrc.GetFactory().GetName())
}

func TestAppSinkCreation(t *testing.T) {
	gst.Init(nil)

	factory := NewElementFactory()

	// Test appsink creation with default config
	appSink, err := factory.CreateAppSink(nil)
	require.NoError(t, err)
	require.NotNil(t, appSink)
	defer appSink.Unref()

	// Test appsink creation with custom config
	customAppSink, err := factory.CreateAppSink(map[string]interface{}{
		"max-buffers": 5,
		"sync":        true,
	})
	require.NoError(t, err)
	require.NotNil(t, customAppSink)
	defer customAppSink.Unref()
}

func TestSimplePipelineExecution(t *testing.T) {
	gst.Init(nil)

	pipeline, err := NewPipelineWrapper("test-execution")
	require.NoError(t, err)
	defer pipeline.Cleanup()

	// Create a simple test pipeline: videotestsrc ! fakesink
	_, err = pipeline.AddElement("src", "videotestsrc")
	require.NoError(t, err)

	_, err = pipeline.AddElement("sink", "fakesink")
	require.NoError(t, err)

	err = pipeline.LinkElements("src", "sink")
	require.NoError(t, err)

	// Configure source to generate only a few buffers
	src, _ := pipeline.GetElement("src")
	factory := NewElementFactory()
	err = factory.SetElementProperty(src, "num-buffers", 100)
	require.NoError(t, err)

	// Start bus wrapper
	busWrapper := NewBusWrapper(pipeline.GetPipeline())
	busWrapper.AddDefaultHandlers()
	err = busWrapper.Start()
	require.NoError(t, err)
	defer busWrapper.Stop()

	// Start pipeline
	err = pipeline.Start()
	require.NoError(t, err)

	// Wait a bit for pipeline to process
	time.Sleep(100 * time.Millisecond)

	// Stop pipeline
	err = pipeline.Stop()
	assert.NoError(t, err)
}

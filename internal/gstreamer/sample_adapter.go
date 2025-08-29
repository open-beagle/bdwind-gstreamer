package gstreamer

import (
	"fmt"
	"time"

	"github.com/go-gst/go-gst/gst"
)

// SampleAdapter provides conversion between go-gst samples and internal Sample format
type SampleAdapter struct {
	// Pool for reusing byte slices to reduce allocations
	bufferPool chan []byte
	poolSize   int
}

// NewSampleAdapter creates a new sample adapter with buffer pooling
func NewSampleAdapter(poolSize int) *SampleAdapter {
	adapter := &SampleAdapter{
		bufferPool: make(chan []byte, poolSize),
		poolSize:   poolSize,
	}

	// Pre-populate the pool with some buffers
	for i := 0; i < poolSize/2; i++ {
		adapter.bufferPool <- make([]byte, 0, 1024*1024) // 1MB initial capacity
	}

	return adapter
}

// ConvertGstSample converts a go-gst sample to internal Sample format
// This is the main conversion function that should be used by all components
func (sa *SampleAdapter) ConvertGstSample(gstSample *gst.Sample) (*Sample, error) {
	if gstSample == nil {
		return nil, fmt.Errorf("gst sample is nil")
	}

	// Get buffer from sample
	buffer := gstSample.GetBuffer()
	if buffer == nil {
		return nil, fmt.Errorf("no buffer in gst sample")
	}

	// Map buffer for reading
	mapInfo := buffer.Map(gst.MapRead)
	if mapInfo == nil {
		return nil, fmt.Errorf("failed to map gst buffer")
	}
	defer buffer.Unmap()

	// Get data with optimized copying
	data, err := sa.CopyBufferData(mapInfo.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to copy buffer data: %w", err)
	}

	// Get caps for format information
	caps := gstSample.GetCaps()
	if caps == nil {
		return nil, fmt.Errorf("no caps in gst sample")
	}

	// Parse format information
	format, err := sa.parseMediaFormat(caps)
	if err != nil {
		return nil, fmt.Errorf("failed to parse media format: %w", err)
	}

	// Extract timestamp and duration from buffer
	timestamp := sa.extractTimestamp(buffer)
	duration := sa.extractDuration(buffer)

	// Create internal sample
	sample := &Sample{
		Data:      data,
		Timestamp: timestamp,
		Duration:  duration,
		Format:    format,
		Metadata:  sa.extractMetadata(gstSample, buffer),
	}

	return sample, nil
}

// ConvertToGstSample converts an internal Sample to go-gst sample format
// Note: This function is currently not implemented as go-gst v1.4.0 doesn't provide
// a direct NewSample constructor. Samples are typically obtained from GStreamer elements.
// This function is kept as a placeholder for future implementation if needed.
func (sa *SampleAdapter) ConvertToGstSample(sample *Sample) (*gst.Sample, error) {
	if sample == nil {
		return nil, fmt.Errorf("internal sample is nil")
	}

	// TODO: Implement when go-gst provides a sample constructor
	// For now, this conversion is not needed as samples are obtained from appsink elements
	return nil, fmt.Errorf("ConvertToGstSample not implemented - samples are obtained from GStreamer elements")
}

// CopyBufferData efficiently copies buffer data using the buffer pool
func (sa *SampleAdapter) CopyBufferData(srcData []byte) ([]byte, error) {
	if len(srcData) == 0 {
		return nil, fmt.Errorf("source data is empty")
	}

	var buffer []byte

	// Try to get a buffer from the pool
	select {
	case buffer = <-sa.bufferPool:
		// Ensure buffer has enough capacity
		if cap(buffer) < len(srcData) {
			buffer = make([]byte, len(srcData))
		} else {
			buffer = buffer[:len(srcData)]
		}
	default:
		// Pool is empty, create new buffer
		buffer = make([]byte, len(srcData))
	}

	// Copy data
	copy(buffer, srcData)

	return buffer, nil
}

// returnBufferToPool returns a buffer to the pool for reuse
func (sa *SampleAdapter) returnBufferToPool(buffer []byte) {
	if buffer == nil {
		return
	}

	// Reset buffer length but keep capacity
	buffer = buffer[:0]

	// Try to return to pool
	select {
	case sa.bufferPool <- buffer:
		// Successfully returned to pool
	default:
		// Pool is full, let GC handle it
	}
}

// parseMediaFormat extracts media format information from GStreamer caps
func (sa *SampleAdapter) parseMediaFormat(caps *gst.Caps) (SampleFormat, error) {
	if caps.GetSize() == 0 {
		return SampleFormat{}, fmt.Errorf("caps structure is empty")
	}

	structure := caps.GetStructureAt(0)
	if structure == nil {
		return SampleFormat{}, fmt.Errorf("failed to get caps structure")
	}

	format := SampleFormat{
		Parameters: make(map[string]interface{}),
	}

	// Get structure name to determine media type
	structName := structure.Name()
	switch {
	case structName == "video/x-raw" || structName == "video/x-h264" || structName == "video/x-vp8" || structName == "video/x-vp9":
		format.MediaType = MediaTypeVideo
		sa.parseVideoFormat(structure, &format)
	case structName == "audio/x-raw" || structName == "audio/mpeg" || structName == "audio/x-opus":
		format.MediaType = MediaTypeAudio
		sa.parseAudioFormat(structure, &format)
	default:
		format.MediaType = MediaTypeUnknown
	}

	// Extract codec information from structure name
	format.Codec = sa.ExtractCodecFromStructure(structName)

	return format, nil
}

// parseVideoFormat extracts video-specific format information
func (sa *SampleAdapter) parseVideoFormat(structure *gst.Structure, format *SampleFormat) {
	// Extract width
	if width, err := structure.GetValue("width"); err == nil {
		if w, ok := width.(int); ok {
			format.Width = w
		}
	}

	// Extract height
	if height, err := structure.GetValue("height"); err == nil {
		if h, ok := height.(int); ok {
			format.Height = h
		}
	}

	// Extract framerate
	if framerate, err := structure.GetValue("framerate"); err == nil {
		format.Parameters["framerate"] = framerate
	}

	// Extract pixel format
	if pixelFormat, err := structure.GetValue("format"); err == nil {
		format.Parameters["pixel_format"] = pixelFormat
	}

	// Extract colorimetry
	if colorimetry, err := structure.GetValue("colorimetry"); err == nil {
		format.Parameters["colorimetry"] = colorimetry
	}
}

// parseAudioFormat extracts audio-specific format information
func (sa *SampleAdapter) parseAudioFormat(structure *gst.Structure, format *SampleFormat) {
	// Extract channels
	if channels, err := structure.GetValue("channels"); err == nil {
		if c, ok := channels.(int); ok {
			format.Channels = c
		}
	}

	// Extract sample rate
	if rate, err := structure.GetValue("rate"); err == nil {
		if r, ok := rate.(int); ok {
			format.SampleRate = r
		}
	}

	// Extract audio format
	if audioFormat, err := structure.GetValue("format"); err == nil {
		format.Parameters["audio_format"] = audioFormat
	}

	// Extract layout
	if layout, err := structure.GetValue("layout"); err == nil {
		format.Parameters["layout"] = layout
	}
}

// ExtractCodecFromStructure determines codec from GStreamer structure name
func (sa *SampleAdapter) ExtractCodecFromStructure(structName string) string {
	switch structName {
	case "video/x-h264":
		return "h264"
	case "video/x-h265":
		return "h265"
	case "video/x-vp8":
		return "vp8"
	case "video/x-vp9":
		return "vp9"
	case "video/x-raw":
		return "raw"
	case "audio/x-opus":
		return "opus"
	case "audio/mpeg":
		return "mp3"
	case "audio/x-raw":
		return "raw"
	default:
		return "unknown"
	}
}

// extractTimestamp extracts timestamp from GStreamer buffer
func (sa *SampleAdapter) extractTimestamp(buffer *gst.Buffer) time.Time {
	if buffer == nil {
		return time.Now()
	}

	pts := buffer.PresentationTimestamp()
	if pts == gst.ClockTimeNone {
		return time.Now()
	}

	// Convert GStreamer timestamp to Go time
	// GStreamer uses nanoseconds since some epoch
	return time.Unix(0, int64(pts))
}

// extractDuration extracts duration from GStreamer buffer
func (sa *SampleAdapter) extractDuration(buffer *gst.Buffer) time.Duration {
	if buffer == nil {
		return 0
	}

	duration := buffer.Duration()
	if duration == gst.ClockTimeNone {
		return 0
	}

	return time.Duration(duration)
}

// extractMetadata extracts metadata from GStreamer sample and buffer
func (sa *SampleAdapter) extractMetadata(gstSample *gst.Sample, buffer *gst.Buffer) map[string]interface{} {
	metadata := make(map[string]interface{})

	if buffer != nil {
		// Check for keyframe flag in buffer flags
		flags := buffer.GetFlags()
		if flags&gst.BufferFlagDeltaUnit == 0 {
			// If delta unit flag is not set, this is likely a keyframe
			metadata["key_frame"] = true
		} else {
			metadata["key_frame"] = false
		}

		// Extract buffer size
		metadata["buffer_size"] = buffer.GetSize()

		// Extract buffer offset (use -1 as invalid offset indicator)
		if offset := buffer.Offset(); offset != -1 {
			metadata["offset"] = offset
		}

		// Extract buffer offset end (use -1 as invalid offset indicator)
		if offsetEnd := buffer.OffsetEnd(); offsetEnd != -1 {
			metadata["offset_end"] = offsetEnd
		}
	}

	return metadata
}

// createCapsFromFormat creates GStreamer caps from internal format
// Note: This function is kept for potential future use
func (sa *SampleAdapter) createCapsFromFormat(format SampleFormat) (*gst.Caps, error) {
	var capsString string

	switch format.MediaType {
	case MediaTypeVideo:
		capsString = sa.createVideoCapsString(format)
	case MediaTypeAudio:
		capsString = sa.createAudioCapsString(format)
	default:
		return nil, fmt.Errorf("unsupported media type: %v", format.MediaType)
	}

	caps := gst.NewCapsFromString(capsString)
	if caps == nil {
		return nil, fmt.Errorf("failed to create caps from string '%s'", capsString)
	}

	return caps, nil
}

// createVideoCapsString creates a video caps string from format
func (sa *SampleAdapter) createVideoCapsString(format SampleFormat) string {
	switch format.Codec {
	case "h264":
		return fmt.Sprintf("video/x-h264,width=%d,height=%d", format.Width, format.Height)
	case "h265":
		return fmt.Sprintf("video/x-h265,width=%d,height=%d", format.Width, format.Height)
	case "vp8":
		return fmt.Sprintf("video/x-vp8,width=%d,height=%d", format.Width, format.Height)
	case "vp9":
		return fmt.Sprintf("video/x-vp9,width=%d,height=%d", format.Width, format.Height)
	case "raw":
		return fmt.Sprintf("video/x-raw,format=RGB,width=%d,height=%d", format.Width, format.Height)
	default:
		return fmt.Sprintf("video/x-raw,width=%d,height=%d", format.Width, format.Height)
	}
}

// createAudioCapsString creates an audio caps string from format
func (sa *SampleAdapter) createAudioCapsString(format SampleFormat) string {
	switch format.Codec {
	case "opus":
		return fmt.Sprintf("audio/x-opus,channels=%d,rate=%d", format.Channels, format.SampleRate)
	case "mp3":
		return fmt.Sprintf("audio/mpeg,mpegversion=1,layer=3,channels=%d,rate=%d", format.Channels, format.SampleRate)
	case "raw":
		return fmt.Sprintf("audio/x-raw,format=S16LE,channels=%d,rate=%d", format.Channels, format.SampleRate)
	default:
		return fmt.Sprintf("audio/x-raw,channels=%d,rate=%d", format.Channels, format.SampleRate)
	}
}

// ValidateFormat validates that a media format is supported and complete
func (sa *SampleAdapter) ValidateFormat(format SampleFormat) error {
	switch format.MediaType {
	case MediaTypeVideo:
		if format.Width <= 0 {
			return fmt.Errorf("invalid video width: %d", format.Width)
		}
		if format.Height <= 0 {
			return fmt.Errorf("invalid video height: %d", format.Height)
		}
		if format.Codec == "" {
			return fmt.Errorf("video codec not specified")
		}

	case MediaTypeAudio:
		if format.Channels <= 0 {
			return fmt.Errorf("invalid audio channels: %d", format.Channels)
		}
		if format.SampleRate <= 0 {
			return fmt.Errorf("invalid audio sample rate: %d", format.SampleRate)
		}
		if format.Codec == "" {
			return fmt.Errorf("audio codec not specified")
		}

	case MediaTypeUnknown:
		return fmt.Errorf("unknown media type")

	default:
		return fmt.Errorf("unsupported media type: %v", format.MediaType)
	}

	return nil
}

// Close cleans up the adapter resources
func (sa *SampleAdapter) Close() {
	// Drain the buffer pool
	for {
		select {
		case <-sa.bufferPool:
			// Continue draining
		default:
			// Pool is empty
			close(sa.bufferPool)
			return
		}
	}
}

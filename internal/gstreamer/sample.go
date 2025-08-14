package gstreamer

import (
	"time"
)

// Sample represents a GStreamer sample containing media data
type Sample struct {
	// Data contains the raw media data
	Data []byte

	// Duration represents the duration of this sample
	Duration time.Duration

	// Timestamp represents the presentation timestamp
	Timestamp time.Time

	// Format contains format information
	Format SampleFormat

	// Metadata contains additional sample metadata
	Metadata map[string]interface{}
}

// SampleFormat describes the format of a media sample
type SampleFormat struct {
	// MediaType indicates the type of media (video, audio)
	MediaType MediaType

	// Codec specifies the codec used
	Codec string

	// Width and Height for video samples
	Width  int
	Height int

	// Channels and SampleRate for audio samples
	Channels   int
	SampleRate int

	// Additional format parameters
	Parameters map[string]interface{}
}

// MediaType represents the type of media
type MediaType int

const (
	MediaTypeUnknown MediaType = iota
	MediaTypeVideo
	MediaTypeAudio
)

// String returns the string representation of MediaType
func (mt MediaType) String() string {
	switch mt {
	case MediaTypeVideo:
		return "video"
	case MediaTypeAudio:
		return "audio"
	default:
		return "unknown"
	}
}

// NewVideoSample creates a new video sample
func NewVideoSample(data []byte, width, height int, codec string) *Sample {
	return &Sample{
		Data:      data,
		Timestamp: time.Now(),
		Format: SampleFormat{
			MediaType: MediaTypeVideo,
			Codec:     codec,
			Width:     width,
			Height:    height,
		},
		Metadata: make(map[string]interface{}),
	}
}

// NewAudioSample creates a new audio sample
func NewAudioSample(data []byte, channels, sampleRate int, codec string) *Sample {
	return &Sample{
		Data:      data,
		Timestamp: time.Now(),
		Format: SampleFormat{
			MediaType:  MediaTypeAudio,
			Codec:      codec,
			Channels:   channels,
			SampleRate: sampleRate,
		},
		Metadata: make(map[string]interface{}),
	}
}

// IsVideo returns true if this is a video sample
func (s *Sample) IsVideo() bool {
	return s.Format.MediaType == MediaTypeVideo
}

// IsAudio returns true if this is an audio sample
func (s *Sample) IsAudio() bool {
	return s.Format.MediaType == MediaTypeAudio
}

// Size returns the size of the sample data in bytes
func (s *Sample) Size() int {
	return len(s.Data)
}

// Clone creates a deep copy of the sample
func (s *Sample) Clone() *Sample {
	// Copy data
	data := make([]byte, len(s.Data))
	copy(data, s.Data)

	// Copy metadata
	metadata := make(map[string]interface{})
	for k, v := range s.Metadata {
		metadata[k] = v
	}

	// Copy parameters
	parameters := make(map[string]interface{})
	for k, v := range s.Format.Parameters {
		parameters[k] = v
	}

	return &Sample{
		Data:      data,
		Duration:  s.Duration,
		Timestamp: s.Timestamp,
		Format: SampleFormat{
			MediaType:  s.Format.MediaType,
			Codec:      s.Format.Codec,
			Width:      s.Format.Width,
			Height:     s.Format.Height,
			Channels:   s.Format.Channels,
			SampleRate: s.Format.SampleRate,
			Parameters: parameters,
		},
		Metadata: metadata,
	}
}

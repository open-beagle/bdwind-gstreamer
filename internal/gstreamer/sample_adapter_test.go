package gstreamer

import (
	"testing"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSampleAdapter(t *testing.T) {
	adapter := NewSampleAdapter(10)
	require.NotNil(t, adapter)
	assert.Equal(t, 10, adapter.poolSize)
	assert.NotNil(t, adapter.bufferPool)

	// Check that some buffers are pre-populated
	select {
	case buffer := <-adapter.bufferPool:
		assert.NotNil(t, buffer)
		assert.Equal(t, 0, len(buffer))
		assert.True(t, cap(buffer) > 0)
	default:
		t.Error("Expected pre-populated buffers in pool")
	}

	adapter.Close()
}

func TestSampleAdapter_ConvertGstSample_NilInput(t *testing.T) {
	adapter := NewSampleAdapter(5)
	defer adapter.Close()

	sample, err := adapter.ConvertGstSample(nil)
	assert.Error(t, err)
	assert.Nil(t, sample)
	assert.Contains(t, err.Error(), "gst sample is nil")
}

func TestSampleAdapter_ValidateFormat(t *testing.T) {
	adapter := NewSampleAdapter(5)
	defer adapter.Close()

	tests := []struct {
		name    string
		format  SampleFormat
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid video format",
			format: SampleFormat{
				MediaType: MediaTypeVideo,
				Codec:     "h264",
				Width:     1920,
				Height:    1080,
			},
			wantErr: false,
		},
		{
			name: "valid audio format",
			format: SampleFormat{
				MediaType:  MediaTypeAudio,
				Codec:      "opus",
				Channels:   2,
				SampleRate: 48000,
			},
			wantErr: false,
		},
		{
			name: "invalid video width",
			format: SampleFormat{
				MediaType: MediaTypeVideo,
				Codec:     "h264",
				Width:     0,
				Height:    1080,
			},
			wantErr: true,
			errMsg:  "invalid video width",
		},
		{
			name: "invalid video height",
			format: SampleFormat{
				MediaType: MediaTypeVideo,
				Codec:     "h264",
				Width:     1920,
				Height:    0,
			},
			wantErr: true,
			errMsg:  "invalid video height",
		},
		{
			name: "missing video codec",
			format: SampleFormat{
				MediaType: MediaTypeVideo,
				Width:     1920,
				Height:    1080,
			},
			wantErr: true,
			errMsg:  "video codec not specified",
		},
		{
			name: "invalid audio channels",
			format: SampleFormat{
				MediaType:  MediaTypeAudio,
				Codec:      "opus",
				Channels:   0,
				SampleRate: 48000,
			},
			wantErr: true,
			errMsg:  "invalid audio channels",
		},
		{
			name: "invalid audio sample rate",
			format: SampleFormat{
				MediaType:  MediaTypeAudio,
				Codec:      "opus",
				Channels:   2,
				SampleRate: 0,
			},
			wantErr: true,
			errMsg:  "invalid audio sample rate",
		},
		{
			name: "missing audio codec",
			format: SampleFormat{
				MediaType:  MediaTypeAudio,
				Channels:   2,
				SampleRate: 48000,
			},
			wantErr: true,
			errMsg:  "audio codec not specified",
		},
		{
			name: "unknown media type",
			format: SampleFormat{
				MediaType: MediaTypeUnknown,
			},
			wantErr: true,
			errMsg:  "unknown media type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.ValidateFormat(tt.format)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSampleAdapter_ExtractCodecFromStructure(t *testing.T) {
	adapter := NewSampleAdapter(5)
	defer adapter.Close()

	tests := []struct {
		structName string
		expected   string
	}{
		{"video/x-h264", "h264"},
		{"video/x-h265", "h265"},
		{"video/x-vp8", "vp8"},
		{"video/x-vp9", "vp9"},
		{"video/x-raw", "raw"},
		{"audio/x-opus", "opus"},
		{"audio/mpeg", "mp3"},
		{"audio/x-raw", "raw"},
		{"unknown/format", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.structName, func(t *testing.T) {
			result := adapter.ExtractCodecFromStructure(tt.structName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSampleAdapter_CreateVideoCapsString(t *testing.T) {
	adapter := NewSampleAdapter(5)
	defer adapter.Close()

	tests := []struct {
		name     string
		format   SampleFormat
		expected string
	}{
		{
			name: "h264 video",
			format: SampleFormat{
				Codec:  "h264",
				Width:  1920,
				Height: 1080,
			},
			expected: "video/x-h264,width=1920,height=1080",
		},
		{
			name: "vp8 video",
			format: SampleFormat{
				Codec:  "vp8",
				Width:  1280,
				Height: 720,
			},
			expected: "video/x-vp8,width=1280,height=720",
		},
		{
			name: "raw video",
			format: SampleFormat{
				Codec:  "raw",
				Width:  640,
				Height: 480,
			},
			expected: "video/x-raw,format=RGB,width=640,height=480",
		},
		{
			name: "unknown codec",
			format: SampleFormat{
				Codec:  "unknown",
				Width:  800,
				Height: 600,
			},
			expected: "video/x-raw,width=800,height=600",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.createVideoCapsString(tt.format)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSampleAdapter_CreateAudioCapsString(t *testing.T) {
	adapter := NewSampleAdapter(5)
	defer adapter.Close()

	tests := []struct {
		name     string
		format   SampleFormat
		expected string
	}{
		{
			name: "opus audio",
			format: SampleFormat{
				Codec:      "opus",
				Channels:   2,
				SampleRate: 48000,
			},
			expected: "audio/x-opus,channels=2,rate=48000",
		},
		{
			name: "mp3 audio",
			format: SampleFormat{
				Codec:      "mp3",
				Channels:   2,
				SampleRate: 44100,
			},
			expected: "audio/mpeg,mpegversion=1,layer=3,channels=2,rate=44100",
		},
		{
			name: "raw audio",
			format: SampleFormat{
				Codec:      "raw",
				Channels:   1,
				SampleRate: 16000,
			},
			expected: "audio/x-raw,format=S16LE,channels=1,rate=16000",
		},
		{
			name: "unknown codec",
			format: SampleFormat{
				Codec:      "unknown",
				Channels:   2,
				SampleRate: 22050,
			},
			expected: "audio/x-raw,channels=2,rate=22050",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.createAudioCapsString(tt.format)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSampleAdapter_CopyBufferData(t *testing.T) {
	adapter := NewSampleAdapter(5)
	defer adapter.Close()

	t.Run("empty data", func(t *testing.T) {
		data, err := adapter.CopyBufferData([]byte{})
		assert.Error(t, err)
		assert.Nil(t, data)
		assert.Contains(t, err.Error(), "source data is empty")
	})

	t.Run("normal data", func(t *testing.T) {
		srcData := []byte{1, 2, 3, 4, 5}
		data, err := adapter.CopyBufferData(srcData)
		assert.NoError(t, err)
		assert.Equal(t, srcData, data)
		assert.NotSame(t, &srcData[0], &data[0]) // Ensure it's a copy

		// Return buffer to pool for reuse
		adapter.returnBufferToPool(data)
	})

	t.Run("large data", func(t *testing.T) {
		srcData := make([]byte, 2*1024*1024) // 2MB
		for i := range srcData {
			srcData[i] = byte(i % 256)
		}

		data, err := adapter.CopyBufferData(srcData)
		assert.NoError(t, err)
		assert.Equal(t, srcData, data)

		adapter.returnBufferToPool(data)
	})
}

func TestSampleAdapter_ReturnBufferToPool(t *testing.T) {
	adapter := NewSampleAdapter(2) // Small pool for testing
	defer adapter.Close()

	// Fill the pool
	buffer1 := make([]byte, 100)
	buffer2 := make([]byte, 200)

	adapter.returnBufferToPool(buffer1)
	adapter.returnBufferToPool(buffer2)

	// Pool should be full now, verify we can get buffers back
	select {
	case buf := <-adapter.bufferPool:
		assert.NotNil(t, buf)
		assert.Equal(t, 0, len(buf)) // Should be reset to length 0
	default:
		t.Error("Expected buffer in pool")
	}

	// Try to return another buffer when pool is full
	buffer3 := make([]byte, 300)
	adapter.returnBufferToPool(buffer3) // Should not block

	// Test with nil buffer
	adapter.returnBufferToPool(nil) // Should not panic
}

func TestSampleAdapter_ExtractTimestamp(t *testing.T) {
	adapter := NewSampleAdapter(5)
	defer adapter.Close()

	// Initialize GStreamer for testing
	gst.Init(nil)

	// Note: gst.NewBuffer() is not available in go-gst v1.4.0
	// Buffers are typically obtained from GStreamer elements
	// We'll test with nil buffer to verify error handling
	t.Run("nil buffer", func(t *testing.T) {
		// Test with nil buffer - should return current time
		timestamp := adapter.extractTimestamp(nil)
		now := time.Now()
		assert.WithinDuration(t, now, timestamp, time.Second)
	})
}

func TestSampleAdapter_ExtractDuration(t *testing.T) {
	adapter := NewSampleAdapter(5)
	defer adapter.Close()

	// Initialize GStreamer for testing
	gst.Init(nil)

	// Note: gst.NewBuffer() is not available in go-gst v1.4.0
	// Buffers are typically obtained from GStreamer elements
	// We'll test with nil buffer to verify error handling
	t.Run("nil buffer", func(t *testing.T) {
		// Test with nil buffer - should return 0 duration
		duration := adapter.extractDuration(nil)
		assert.Equal(t, time.Duration(0), duration)
	})
}

func TestSampleAdapter_ExtractMetadata(t *testing.T) {
	adapter := NewSampleAdapter(5)
	defer adapter.Close()

	// Initialize GStreamer for testing
	gst.Init(nil)

	// Note: gst.NewBuffer() is not available in go-gst v1.4.0
	// We'll test the metadata extraction with nil sample and buffer to test error handling
	sample := (*gst.Sample)(nil)
	buffer := (*gst.Buffer)(nil)

	metadata := adapter.extractMetadata(sample, buffer)

	// Check that basic metadata is extracted (even with nil buffer)
	assert.NotNil(t, metadata)
	// With nil buffer, we should still get some default metadata
	assert.IsType(t, map[string]interface{}{}, metadata)
}

func TestSampleAdapter_ConvertToGstSample(t *testing.T) {
	adapter := NewSampleAdapter(5)
	defer adapter.Close()

	t.Run("nil sample", func(t *testing.T) {
		gstSample, err := adapter.ConvertToGstSample(nil)
		assert.Error(t, err)
		assert.Nil(t, gstSample)
		assert.Contains(t, err.Error(), "internal sample is nil")
	})

	t.Run("not implemented", func(t *testing.T) {
		sample := &Sample{
			Data:      []byte{1, 2, 3},
			Timestamp: time.Now(),
			Format: SampleFormat{
				MediaType: MediaTypeVideo,
				Codec:     "h264",
				Width:     640,
				Height:    480,
			},
			Metadata: make(map[string]interface{}),
		}

		gstSample, err := adapter.ConvertToGstSample(sample)
		assert.Error(t, err)
		assert.Nil(t, gstSample)
		assert.Contains(t, err.Error(), "not implemented")
	})
}

func TestSampleAdapter_CreateCapsFromFormat(t *testing.T) {
	adapter := NewSampleAdapter(5)
	defer adapter.Close()

	// Initialize GStreamer for testing
	gst.Init(nil)

	t.Run("video format", func(t *testing.T) {
		format := SampleFormat{
			MediaType: MediaTypeVideo,
			Codec:     "h264",
			Width:     1920,
			Height:    1080,
		}

		caps, err := adapter.createCapsFromFormat(format)
		assert.NoError(t, err)
		assert.NotNil(t, caps)
		assert.True(t, caps.GetSize() > 0)
	})

	t.Run("audio format", func(t *testing.T) {
		format := SampleFormat{
			MediaType:  MediaTypeAudio,
			Codec:      "opus",
			Channels:   2,
			SampleRate: 48000,
		}

		caps, err := adapter.createCapsFromFormat(format)
		assert.NoError(t, err)
		assert.NotNil(t, caps)
		assert.True(t, caps.GetSize() > 0)
	})

	t.Run("unsupported media type", func(t *testing.T) {
		format := SampleFormat{
			MediaType: MediaTypeUnknown,
		}

		caps, err := adapter.createCapsFromFormat(format)
		assert.Error(t, err)
		assert.Nil(t, caps)
		assert.Contains(t, err.Error(), "unsupported media type")
	})
}

func TestSampleAdapter_Close(t *testing.T) {
	adapter := NewSampleAdapter(5)

	// Add some buffers to the pool
	buffer1 := make([]byte, 100)
	buffer2 := make([]byte, 200)
	adapter.returnBufferToPool(buffer1)
	adapter.returnBufferToPool(buffer2)

	// Close should not panic and should clean up resources
	assert.NotPanics(t, func() {
		adapter.Close()
	})

	// After close, the channel should be closed
	_, ok := <-adapter.bufferPool
	assert.False(t, ok, "Buffer pool channel should be closed")
}

// Benchmark tests for performance optimization
func BenchmarkSampleAdapter_CopyBufferData(b *testing.B) {
	adapter := NewSampleAdapter(10)
	defer adapter.Close()

	data := make([]byte, 1920*1080*3) // Simulate RGB frame data
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		copied, err := adapter.CopyBufferData(data)
		if err != nil {
			b.Fatal(err)
		}
		adapter.returnBufferToPool(copied)
	}
}

func BenchmarkSampleAdapter_ConvertGstSample(b *testing.B) {
	adapter := NewSampleAdapter(10)
	defer adapter.Close()

	// Initialize GStreamer
	gst.Init(nil)

	// Note: We can't create a gst.Sample directly in go-gst v1.4.0
	// This benchmark would need to be run with actual samples from GStreamer elements
	// For now, we'll skip this benchmark
	b.Skip("Benchmark requires actual gst.Sample from GStreamer elements")
}

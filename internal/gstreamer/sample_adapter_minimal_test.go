package gstreamer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSampleAdapter_Basic tests basic functionality without GStreamer dependencies
func TestSampleAdapter_Basic(t *testing.T) {
	adapter := NewSampleAdapter(10)
	require.NotNil(t, adapter)
	assert.Equal(t, 10, adapter.poolSize)
	assert.NotNil(t, adapter.bufferPool)

	// Test that some buffers are pre-populated
	select {
	case buffer := <-adapter.bufferPool:
		assert.NotNil(t, buffer)
		assert.Equal(t, 0, len(buffer))
		assert.True(t, cap(buffer) > 0)
		// Return buffer to pool
		adapter.returnBufferToPool(buffer)
	default:
		t.Error("Expected pre-populated buffers in pool")
	}

	adapter.Close()
}

func TestSampleAdapter_ValidateFormat_Basic(t *testing.T) {
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

func TestSampleAdapter_ExtractCodecFromStructure_Basic(t *testing.T) {
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

func TestSampleAdapter_CopyBufferData_Basic(t *testing.T) {
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

func TestSampleAdapter_ConvertGstSample_NilInput_Basic(t *testing.T) {
	adapter := NewSampleAdapter(5)
	defer adapter.Close()

	sample, err := adapter.ConvertGstSample(nil)
	assert.Error(t, err)
	assert.Nil(t, sample)
	assert.Contains(t, err.Error(), "gst sample is nil")
}

func TestSampleAdapter_ConvertToGstSample_Basic(t *testing.T) {
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
			Data: []byte{1, 2, 3},
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

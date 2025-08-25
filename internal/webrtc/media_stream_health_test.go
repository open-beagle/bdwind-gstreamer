package webrtc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaStreamHealthCheck(t *testing.T) {
	tests := []struct {
		name           string
		config         *MediaStreamConfig
		expectError    bool
		expectedIssues []string
	}{
		{
			name: "healthy video-only stream",
			config: &MediaStreamConfig{
				VideoEnabled: true,
				AudioEnabled: false,
				VideoTrackID: "video",
				VideoCodec:   "h264",
			},
			expectError: false,
		},
		{
			name: "healthy audio-only stream",
			config: &MediaStreamConfig{
				VideoEnabled: false,
				AudioEnabled: true,
				AudioTrackID: "audio",
				AudioCodec:   "opus",
			},
			expectError: false,
		},
		{
			name: "healthy video and audio stream",
			config: &MediaStreamConfig{
				VideoEnabled: true,
				AudioEnabled: true,
				VideoTrackID: "video",
				AudioTrackID: "audio",
				VideoCodec:   "h264",
				AudioCodec:   "opus",
			},
			expectError: false,
		},
		{
			name: "disabled streams",
			config: &MediaStreamConfig{
				VideoEnabled: false,
				AudioEnabled: false,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建媒体流
			ms, err := NewMediaStream(tt.config)
			require.NoError(t, err)
			defer ms.Close()

			// 如果有轨道，更新最后活动时间以避免不活跃警告
			if tt.config.VideoEnabled {
				impl := ms.(*mediaStreamImpl)
				impl.mutex.Lock()
				if trackInfo, exists := impl.trackInfo[tt.config.VideoTrackID]; exists {
					trackInfo.LastActive = time.Now()
				}
				impl.mutex.Unlock()
			}

			if tt.config.AudioEnabled {
				impl := ms.(*mediaStreamImpl)
				impl.mutex.Lock()
				if trackInfo, exists := impl.trackInfo[tt.config.AudioTrackID]; exists {
					trackInfo.LastActive = time.Now()
				}
				impl.mutex.Unlock()
			}

			// 执行健康检查
			err = ms.HealthCheck()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMediaStreamHealthCheckInactiveTrack(t *testing.T) {
	config := &MediaStreamConfig{
		VideoEnabled: true,
		AudioEnabled: false,
		VideoTrackID: "video",
		VideoCodec:   "h264",
	}

	// 创建媒体流
	ms, err := NewMediaStream(config)
	require.NoError(t, err)
	defer ms.Close()

	// 设置轨道为很久之前活跃（模拟不活跃状态）
	impl := ms.(*mediaStreamImpl)
	impl.mutex.Lock()
	if trackInfo, exists := impl.trackInfo[config.VideoTrackID]; exists {
		trackInfo.LastActive = time.Now().Add(-time.Hour) // 1小时前
	}
	impl.mutex.Unlock()

	// 执行健康检查，应该检测到不活跃问题
	err = ms.HealthCheck()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "inactive")
}

func TestMediaStreamHealthCheckMimeTypeMismatch(t *testing.T) {
	config := &MediaStreamConfig{
		VideoEnabled: true,
		AudioEnabled: false,
		VideoTrackID: "video",
		VideoCodec:   "h264",
	}

	// 创建媒体流
	ms, err := NewMediaStream(config)
	require.NoError(t, err)
	defer ms.Close()

	// 修改配置中的编码器类型以创建不匹配
	impl := ms.(*mediaStreamImpl)
	impl.mutex.Lock()
	impl.config.VideoCodec = "vp8" // 改为不同的编码器
	if trackInfo, exists := impl.trackInfo[config.VideoTrackID]; exists {
		trackInfo.LastActive = time.Now()
	}
	impl.mutex.Unlock()

	// 执行健康检查，应该检测到MIME类型不匹配
	err = ms.HealthCheck()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MIME type mismatch")
}

func TestMediaStreamHealthCheckTrackStateMismatch(t *testing.T) {
	config := &MediaStreamConfig{
		VideoEnabled: true,
		AudioEnabled: false,
		VideoTrackID: "video",
		VideoCodec:   "h264",
	}

	// 创建媒体流
	ms, err := NewMediaStream(config)
	require.NoError(t, err)
	defer ms.Close()

	// 设置轨道状态为非活跃
	impl := ms.(*mediaStreamImpl)
	impl.mutex.Lock()
	if trackInfo, exists := impl.trackInfo[config.VideoTrackID]; exists {
		trackInfo.State = TrackStateInactive
		trackInfo.LastActive = time.Now()
	}
	impl.trackStates[config.VideoTrackID] = TrackStateInactive
	impl.mutex.Unlock()

	// 执行健康检查，应该检测到轨道状态问题
	err = ms.HealthCheck()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not active")
}

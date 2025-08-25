package webrtc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGStreamerBridge_SampleProcessingValidation(t *testing.T) {
	// 创建桥接器
	config := DefaultGStreamerBridgeConfig()
	bridge := NewGStreamerBridge(config).(*gstreamerBridgeImpl)

	// 测试初始状态
	stats := bridge.GetStats()
	assert.Equal(t, int64(0), stats.VideoSamplesProcessed)
	assert.Equal(t, int64(0), stats.AudioSamplesProcessed)
	assert.Equal(t, int64(0), stats.VideoErrorCount)
	assert.Equal(t, int64(0), stats.AudioErrorCount)
	assert.Equal(t, float64(0), stats.VideoSampleRate)
	assert.Equal(t, float64(0), stats.AudioSampleRate)

	// 测试健康状态
	healthStatus := bridge.GetHealthStatus()
	assert.Equal(t, "stopped", healthStatus)

	// 测试验证方法（桥接器未启动时）
	err := bridge.ValidateProcessingStatus()
	assert.NoError(t, err) // 未启动时不应该有验证错误
}

func TestGStreamerBridge_HealthStatusCalculation(t *testing.T) {
	bridge := NewGStreamerBridge(nil).(*gstreamerBridgeImpl)

	// 测试停止状态
	assert.Equal(t, "stopped", bridge.GetHealthStatus())

	// 模拟启动状态
	bridge.mutex.Lock()
	bridge.running = true
	bridge.startTime = time.Now()
	bridge.mutex.Unlock()

	// 测试启动状态（运行时间不足5秒）
	assert.Equal(t, "starting", bridge.GetHealthStatus())

	// 模拟运行超过5秒
	bridge.mutex.Lock()
	bridge.startTime = time.Now().Add(-time.Second * 10)
	bridge.mutex.Unlock()

	// 测试空闲状态（无样本活动）
	assert.Equal(t, "idle", bridge.GetHealthStatus())

	// 模拟有视频活动
	bridge.mutex.Lock()
	bridge.stats.VideoSamplesProcessed = 100
	bridge.stats.LastVideoSample = time.Now()
	bridge.mutex.Unlock()

	// 测试健康状态
	assert.Equal(t, "healthy", bridge.GetHealthStatus())

	// 模拟高错误率
	bridge.mutex.Lock()
	bridge.stats.VideoErrorCount = 10 // 10% 错误率
	bridge.maxErrorRate = 0.05        // 5% 阈值
	bridge.mutex.Unlock()

	// 测试错误状态
	assert.Equal(t, "error", bridge.GetHealthStatus())
}

func TestGStreamerBridge_StatsCalculation(t *testing.T) {
	bridge := NewGStreamerBridge(nil).(*gstreamerBridgeImpl)

	// 设置一些模拟数据
	bridge.mutex.Lock()
	bridge.startTime = time.Now().Add(-time.Second * 10) // 运行10秒
	bridge.stats.VideoSamplesProcessed = 100
	bridge.stats.AudioSamplesProcessed = 200
	bridge.stats.VideoProcessingTime = time.Millisecond * 500  // 总处理时间500ms
	bridge.stats.AudioProcessingTime = time.Millisecond * 1000 // 总处理时间1000ms
	bridge.mutex.Unlock()

	// 获取统计信息
	stats := bridge.GetStats()

	// 验证计算的统计数据
	assert.True(t, stats.VideoSampleRate > 0, "Video sample rate should be calculated")
	assert.True(t, stats.AudioSampleRate > 0, "Audio sample rate should be calculated")
	assert.True(t, stats.AvgVideoProcessTime > 0, "Average video processing time should be calculated")
	assert.True(t, stats.AvgAudioProcessTime > 0, "Average audio processing time should be calculated")
	assert.NotEmpty(t, stats.HealthStatus, "Health status should be set")

	// 验证具体数值
	expectedVideoRate := float64(100) / 10.0 // 100 samples in 10 seconds = 10/s
	assert.InDelta(t, expectedVideoRate, stats.VideoSampleRate, 1.0)

	expectedAudioRate := float64(200) / 10.0 // 200 samples in 10 seconds = 20/s
	assert.InDelta(t, expectedAudioRate, stats.AudioSampleRate, 1.0)

	expectedVideoAvgTime := float64(500) / float64(100) // 500ms / 100 samples = 5ms avg
	assert.InDelta(t, expectedVideoAvgTime, stats.AvgVideoProcessTime, 0.1)

	expectedAudioAvgTime := float64(1000) / float64(200) // 1000ms / 200 samples = 5ms avg
	assert.InDelta(t, expectedAudioAvgTime, stats.AvgAudioProcessTime, 0.1)
}

func TestGStreamerBridge_ValidationWithIssues(t *testing.T) {
	bridge := NewGStreamerBridge(nil).(*gstreamerBridgeImpl)

	// 模拟运行状态和一些问题
	bridge.mutex.Lock()
	bridge.running = true
	bridge.startTime = time.Now().Add(-time.Second * 10) // 运行10秒
	bridge.stats.VideoSamplesProcessed = 100
	bridge.stats.VideoErrorCount = 10 // 10% 错误率
	bridge.maxErrorRate = 0.05        // 5% 阈值
	bridge.mutex.Unlock()

	// 验证应该失败
	err := bridge.ValidateProcessingStatus()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error rate too high")
}

func TestGStreamerBridge_ValidationSuccess(t *testing.T) {
	bridge := NewGStreamerBridge(nil).(*gstreamerBridgeImpl)

	// 模拟健康的运行状态
	bridge.mutex.Lock()
	bridge.running = true
	bridge.startTime = time.Now().Add(-time.Second * 10) // 运行10秒
	bridge.stats.VideoSamplesProcessed = 100
	bridge.stats.AudioSamplesProcessed = 200
	bridge.stats.VideoErrorCount = 2 // 2% 错误率
	bridge.stats.AudioErrorCount = 5 // 2.5% 错误率
	bridge.stats.LastVideoSample = time.Now().Add(-time.Second * 10)
	bridge.stats.LastAudioSample = time.Now().Add(-time.Second * 10)
	bridge.maxErrorRate = 0.05 // 5% 阈值
	bridge.mutex.Unlock()

	// 验证应该成功
	err := bridge.ValidateProcessingStatus()
	assert.NoError(t, err)
}

func TestGStreamerBridge_ConfigDefaults(t *testing.T) {
	// 测试默认配置
	config := DefaultGStreamerBridgeConfig()
	assert.Equal(t, 100, config.VideoBufferSize)
	assert.Equal(t, 200, config.AudioBufferSize)
	assert.Equal(t, uint32(90000), config.VideoClockRate)
	assert.Equal(t, uint32(48000), config.AudioClockRate)
	assert.Equal(t, 10, config.MaxConcurrentFrames)
	assert.Equal(t, time.Millisecond*100, config.ProcessingTimeout)

	// 测试使用默认配置创建桥接器
	bridge := NewGStreamerBridge(nil).(*gstreamerBridgeImpl)
	assert.NotNil(t, bridge)
	assert.Equal(t, config.VideoBufferSize, bridge.config.VideoBufferSize)
	assert.Equal(t, config.AudioBufferSize, bridge.config.AudioBufferSize)
	assert.Equal(t, time.Millisecond*50, bridge.maxProcessingTime)
	assert.Equal(t, float64(1000.0), bridge.maxSampleRate)
	assert.Equal(t, float64(1.0), bridge.minSampleRate)
	assert.Equal(t, float64(0.05), bridge.maxErrorRate)
}

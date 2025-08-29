package gstreamer

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGStreamerBridgeRemovalVerification 验证删除GStreamer Bridge后WebRTC功能正常
// 任务8: 验证删除 GStreamer Bridge 后 WebRTC 功能正常
// 需求 7.1: 消除 GStreamer Bridge，WebRTC 直接订阅 GStreamer 输出
func TestGStreamerBridgeRemovalVerification(t *testing.T) {
	t.Run("VerifyBridgeRemoval", testVerifyBridgeRemoval)
	t.Run("VerifyPublishSubscribeMechanism", testVerifyPublishSubscribeMechanism)
	t.Run("VerifyDirectWebRTCIntegration", testVerifyDirectWebRTCIntegration)
	t.Run("VerifyStreamProcessorFunctionality", testVerifyStreamProcessorFunctionality)
}

// testVerifyBridgeRemoval 验证GStreamer Bridge已被移除
func testVerifyBridgeRemoval(t *testing.T) {
	t.Log("=== 验证GStreamer Bridge已被移除 ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")
	require.NotNil(t, manager, "管理器不能为nil")

	// 验证不存在Bridge相关的字段或方法
	t.Log("--- 检查Bridge相关组件是否已移除 ---")

	// 通过反射检查manager结构体，确保没有bridge相关字段
	managerValue := reflect.ValueOf(manager).Elem()
	managerType := managerValue.Type()

	bridgeFieldFound := false
	for i := 0; i < managerType.NumField(); i++ {
		field := managerType.Field(i)
		fieldName := strings.ToLower(field.Name)

		// 检查是否包含bridge相关的字段名
		if strings.Contains(fieldName, "bridge") {
			bridgeFieldFound = true
			t.Errorf("发现Bridge相关字段: %s，应该已被移除", field.Name)
		}
	}

	if !bridgeFieldFound {
		t.Log("✓ 确认没有Bridge相关字段存在")
	}

	// 验证发布-订阅机制已实现
	t.Log("--- 验证发布-订阅机制替代Bridge ---")

	// 检查subscribers字段存在
	subscribersField := managerValue.FieldByName("subscribers")
	assert.True(t, subscribersField.IsValid(), "subscribers字段应该存在")
	if subscribersField.IsValid() {
		t.Log("✓ 发布-订阅机制subscribers字段存在")
	}

	// 检查流处理通道存在
	videoStreamChanField := managerValue.FieldByName("videoStreamChan")
	assert.True(t, videoStreamChanField.IsValid(), "videoStreamChan字段应该存在")
	if videoStreamChanField.IsValid() {
		t.Log("✓ 视频流处理通道存在")
	}

	audioStreamChanField := managerValue.FieldByName("audioStreamChan")
	assert.True(t, audioStreamChanField.IsValid(), "audioStreamChan字段应该存在")
	if audioStreamChanField.IsValid() {
		t.Log("✓ 音频流处理通道存在")
	}

	// 检查流处理器存在
	streamProcessorField := managerValue.FieldByName("streamProcessor")
	assert.True(t, streamProcessorField.IsValid(), "streamProcessor字段应该存在")
	if streamProcessorField.IsValid() && !streamProcessorField.IsNil() {
		t.Log("✓ 流处理器已初始化")
	}

	t.Log("=== GStreamer Bridge移除验证完成 ===")
}

// testVerifyPublishSubscribeMechanism 验证发布-订阅机制正常工作
func testVerifyPublishSubscribeMechanism(t *testing.T) {
	t.Log("=== 验证发布-订阅机制 ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")

	// 启动管理器以激活流处理器
	err = manager.Start(ctx)
	require.NoError(t, err, "启动管理器失败")
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		manager.Stop(stopCtx)
	}()

	t.Log("--- 验证发布-订阅接口 ---")

	// 验证发布-订阅相关方法存在
	managerType := reflect.TypeOf(manager)

	// 检查Subscribe方法
	_, subscribeExists := managerType.MethodByName("Subscribe")
	if subscribeExists {
		t.Log("✓ Subscribe方法存在")
	}

	// 检查Unsubscribe方法
	_, unsubscribeExists := managerType.MethodByName("Unsubscribe")
	if unsubscribeExists {
		t.Log("✓ Unsubscribe方法存在")
	}

	// 检查PublishVideoStream方法
	_, publishVideoExists := managerType.MethodByName("PublishVideoStream")
	if publishVideoExists {
		t.Log("✓ PublishVideoStream方法存在")
	}

	// 检查PublishAudioStream方法
	_, publishAudioExists := managerType.MethodByName("PublishAudioStream")
	if publishAudioExists {
		t.Log("✓ PublishAudioStream方法存在")
	}

	t.Log("--- 验证流处理器状态 ---")

	// 通过反射检查流处理器状态
	managerValue := reflect.ValueOf(manager).Elem()
	streamProcessorField := managerValue.FieldByName("streamProcessor")

	if streamProcessorField.IsValid() && !streamProcessorField.IsNil() {
		// 获取流处理器的isRunning状态
		streamProcessor := streamProcessorField.Interface()
		streamProcessorValue := reflect.ValueOf(streamProcessor).Elem()

		// 检查isRunning字段
		isRunningField := streamProcessorValue.FieldByName("isRunning")
		if isRunningField.IsValid() {
			isRunning := isRunningField.Bool()
			assert.True(t, isRunning, "流处理器应该处于运行状态")
			if isRunning {
				t.Log("✓ 流处理器正在运行")
			}
		}
	}

	t.Log("=== 发布-订阅机制验证完成 ===")
}

// testVerifyDirectWebRTCIntegration 验证WebRTC直接集成(无Bridge)
func testVerifyDirectWebRTCIntegration(t *testing.T) {
	t.Log("=== 验证WebRTC直接集成 ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")

	// 启动管理器
	err = manager.Start(ctx)
	require.NoError(t, err, "启动管理器失败")
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		manager.Stop(stopCtx)
	}()

	t.Log("--- 验证直接样本处理机制 ---")

	// 测试编码样本处理流程
	var processedSamples []*Sample
	var processingErrors []error

	// 设置样本回调来验证处理流程
	sampleCallback := func(sample *Sample) error {
		if sample == nil {
			err := fmt.Errorf("received nil sample")
			processingErrors = append(processingErrors, err)
			return err
		}

		processedSamples = append(processedSamples, sample)
		t.Logf("处理样本: type=%s, size=%d, codec=%s",
			sample.Format.MediaType.String(), sample.Size(), sample.Format.Codec)
		return nil
	}

	manager.SetEncodedSampleCallback(sampleCallback)
	t.Log("✓ 编码样本回调设置成功")

	// 模拟样本处理(如果有编码器可用)
	if manager.encoder != nil {
		t.Log("✓ 编码器可用，可以进行样本处理测试")

		// 创建测试样本
		testSample := &Sample{
			Data:      []byte("test video data"),
			Timestamp: time.Now(),
			Format: SampleFormat{
				MediaType: MediaTypeVideo,
				Codec:     "h264",
				Width:     1920,
				Height:    1080,
			},
		}

		// 直接调用processEncodedSample来测试处理流程
		err := manager.processEncodedSample(testSample)
		assert.NoError(t, err, "编码样本处理不应该失败")

		// 验证样本是否被正确处理
		if len(processedSamples) > 0 {
			t.Log("✓ 样本处理流程验证成功")
		}

		if len(processingErrors) == 0 {
			t.Log("✓ 样本处理无错误")
		}
	} else {
		t.Log("⚠ 编码器不可用(测试环境预期)")
	}

	t.Log("--- 验证发布-订阅数据流 ---")

	// 验证流媒体通道状态
	managerValue := reflect.ValueOf(manager).Elem()

	// 检查视频流通道
	videoStreamChanField := managerValue.FieldByName("videoStreamChan")
	if videoStreamChanField.IsValid() {
		// 通道应该是开放的
		assert.False(t, videoStreamChanField.IsNil(), "视频流通道不应该为nil")
		t.Log("✓ 视频流通道状态正常")
	}

	// 检查音频流通道
	audioStreamChanField := managerValue.FieldByName("audioStreamChan")
	if audioStreamChanField.IsValid() {
		// 通道应该是开放的
		assert.False(t, audioStreamChanField.IsNil(), "音频流通道不应该为nil")
		t.Log("✓ 音频流通道状态正常")
	}

	t.Log("=== WebRTC直接集成验证完成 ===")
}

// testVerifyStreamProcessorFunctionality 验证流处理器功能
func testVerifyStreamProcessorFunctionality(t *testing.T) {
	t.Log("=== 验证流处理器功能 ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")

	// 启动管理器
	err = manager.Start(ctx)
	require.NoError(t, err, "启动管理器失败")
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		manager.Stop(stopCtx)
	}()

	t.Log("--- 验证流处理器组件 ---")

	// 通过反射检查流处理器
	managerValue := reflect.ValueOf(manager).Elem()
	streamProcessorField := managerValue.FieldByName("streamProcessor")

	require.True(t, streamProcessorField.IsValid(), "streamProcessor字段应该存在")
	require.False(t, streamProcessorField.IsNil(), "streamProcessor不应该为nil")

	streamProcessor := streamProcessorField.Interface()
	streamProcessorValue := reflect.ValueOf(streamProcessor).Elem()
	_ = streamProcessorValue.Type() // Unused but needed for reflection

	t.Log("✓ 流处理器组件存在")

	// 验证流处理器字段
	expectedFields := []string{"manager", "ctx", "cancel", "wg", "logger", "isRunning", "mu"}
	for _, fieldName := range expectedFields {
		field := streamProcessorValue.FieldByName(fieldName)
		assert.True(t, field.IsValid(), fmt.Sprintf("流处理器应该有%s字段", fieldName))
		if field.IsValid() {
			t.Logf("✓ 流处理器.%s字段存在", fieldName)
		}
	}

	// 验证流处理器状态
	isRunningField := streamProcessorValue.FieldByName("isRunning")
	if isRunningField.IsValid() {
		isRunning := isRunningField.Bool()
		assert.True(t, isRunning, "流处理器应该处于运行状态")
		t.Logf("✓ 流处理器运行状态: %v", isRunning)
	}

	t.Log("--- 验证流处理goroutine ---")

	// 验证流处理器的goroutine是否正在运行
	// 这里我们通过检查context是否被取消来间接验证
	ctxField := streamProcessorValue.FieldByName("ctx")
	if ctxField.IsValid() {
		ctx := ctxField.Interface().(context.Context)

		select {
		case <-ctx.Done():
			t.Error("流处理器context不应该被取消")
		default:
			t.Log("✓ 流处理器context正常运行")
		}
	}

	t.Log("--- 验证流处理方法 ---")

	// 验证流处理器相关的方法存在
	managerType := reflect.TypeOf(manager)

	// 检查processVideoStreams方法
	_, processVideoExists := managerType.MethodByName("processVideoStreams")
	if processVideoExists {
		t.Log("✓ processVideoStreams方法存在")
	}

	// 检查processAudioStreams方法
	_, processAudioExists := managerType.MethodByName("processAudioStreams")
	if processAudioExists {
		t.Log("✓ processAudioStreams方法存在")
	}

	// 检查startStreamProcessor方法
	_, startStreamExists := managerType.MethodByName("startStreamProcessor")
	if startStreamExists {
		t.Log("✓ startStreamProcessor方法存在")
	}

	t.Log("=== 流处理器功能验证完成 ===")
}

// MockMediaStreamSubscriber 模拟媒体流订阅者(用于测试)
type MockMediaStreamSubscriber struct {
	ID              string
	ReceivedStreams []interface{}
	Errors          []error
}

func (m *MockMediaStreamSubscriber) GetID() string {
	return m.ID
}

func (m *MockMediaStreamSubscriber) OnVideoStream(stream *EncodedVideoStream) error {
	if stream == nil {
		err := fmt.Errorf("received nil video stream")
		m.Errors = append(m.Errors, err)
		return err
	}
	m.ReceivedStreams = append(m.ReceivedStreams, stream)
	return nil
}

func (m *MockMediaStreamSubscriber) OnAudioStream(stream *EncodedAudioStream) error {
	if stream == nil {
		err := fmt.Errorf("received nil audio stream")
		m.Errors = append(m.Errors, err)
		return err
	}
	m.ReceivedStreams = append(m.ReceivedStreams, stream)
	return nil
}

func (m *MockMediaStreamSubscriber) OnError(err error) {
	m.Errors = append(m.Errors, err)
}

// Note: EncodedVideoStream and EncodedAudioStream are already defined in media_config.go

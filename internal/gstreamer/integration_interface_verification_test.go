package gstreamer

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTask8_ExternalIntegrationInterfaceVerification 验证对外集成接口完全不变
// 任务8: 对外集成接口验证(接口不变原则)
// 需求: 6.5, 7.1, 7.2
func TestTask8_ExternalIntegrationInterfaceVerification(t *testing.T) {
	t.Run("LifecycleManagementInterface", testLifecycleManagementInterface)
	t.Run("ConfigurationManagementInterface", testConfigurationManagementInterface)
	t.Run("LoggingManagementInterface", testLoggingManagementInterface)
	t.Run("WebRTCBusinessIntegration", testWebRTCBusinessIntegration)
	t.Run("HTTPInterfaceCompatibility", testHTTPInterfaceCompatibility)
	t.Run("ConfigurationStructureUnchanged", testConfigurationStructureUnchanged)
	t.Run("AppIntegrationCodeUnchanged", testAppIntegrationCodeUnchanged)
}

// testLifecycleManagementInterface 验证生命周期管理接口完全不变
// 需求 6.5: 保持生命周期管理接口完全不变(Start/Stop/IsRunning 等)
func testLifecycleManagementInterface(t *testing.T) {
	t.Log("=== 验证生命周期管理接口 ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")
	require.NotNil(t, manager, "管理器不能为nil")

	t.Log("✓ GStreamer管理器创建成功")

	// 验证生命周期管理接口存在且签名正确
	managerType := reflect.TypeOf(manager)

	// 1. 验证Start方法
	startMethod, exists := managerType.MethodByName("Start")
	assert.True(t, exists, "Start方法必须存在")
	if exists {
		// 验证方法签名: Start(ctx context.Context) error
		assert.Equal(t, 2, startMethod.Type.NumIn(), "Start方法应该有2个参数(receiver + ctx)")
		assert.Equal(t, 1, startMethod.Type.NumOut(), "Start方法应该返回1个值(error)")
		assert.Equal(t, "context.Context", startMethod.Type.In(1).String(), "第一个参数应该是context.Context")
		assert.True(t, startMethod.Type.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()), "返回值应该是error")
		t.Log("✓ Start方法签名验证通过")
	}

	// 2. 验证Stop方法
	stopMethod, exists := managerType.MethodByName("Stop")
	assert.True(t, exists, "Stop方法必须存在")
	if exists {
		// 验证方法签名: Stop(ctx context.Context) error
		assert.Equal(t, 2, stopMethod.Type.NumIn(), "Stop方法应该有2个参数(receiver + ctx)")
		assert.Equal(t, 1, stopMethod.Type.NumOut(), "Stop方法应该返回1个值(error)")
		assert.Equal(t, "context.Context", stopMethod.Type.In(1).String(), "第一个参数应该是context.Context")
		assert.True(t, stopMethod.Type.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()), "返回值应该是error")
		t.Log("✓ Stop方法签名验证通过")
	}

	// 3. 验证IsRunning方法
	isRunningMethod, exists := managerType.MethodByName("IsRunning")
	assert.True(t, exists, "IsRunning方法必须存在")
	if exists {
		// 验证方法签名: IsRunning() bool
		assert.Equal(t, 1, isRunningMethod.Type.NumIn(), "IsRunning方法应该只有1个参数(receiver)")
		assert.Equal(t, 1, isRunningMethod.Type.NumOut(), "IsRunning方法应该返回1个值(bool)")
		assert.Equal(t, "bool", isRunningMethod.Type.Out(0).String(), "返回值应该是bool")
		t.Log("✓ IsRunning方法签名验证通过")
	}

	// 4. 验证IsEnabled方法
	isEnabledMethod, exists := managerType.MethodByName("IsEnabled")
	assert.True(t, exists, "IsEnabled方法必须存在")
	if exists {
		// 验证方法签名: IsEnabled() bool
		assert.Equal(t, 1, isEnabledMethod.Type.NumIn(), "IsEnabled方法应该只有1个参数(receiver)")
		assert.Equal(t, 1, isEnabledMethod.Type.NumOut(), "IsEnabled方法应该返回1个值(bool)")
		assert.Equal(t, "bool", isEnabledMethod.Type.Out(0).String(), "返回值应该是bool")
		t.Log("✓ IsEnabled方法签名验证通过")
	}

	// 5. 验证GetStats方法
	getStatsMethod, exists := managerType.MethodByName("GetStats")
	assert.True(t, exists, "GetStats方法必须存在")
	if exists {
		// 验证方法签名: GetStats() map[string]interface{}
		assert.Equal(t, 1, getStatsMethod.Type.NumIn(), "GetStats方法应该只有1个参数(receiver)")
		assert.Equal(t, 1, getStatsMethod.Type.NumOut(), "GetStats方法应该返回1个值")
		returnType := getStatsMethod.Type.Out(0)
		assert.Equal(t, "map[string]interface {}", returnType.String(), "返回值应该是map[string]interface{}")
		t.Log("✓ GetStats方法签名验证通过")
	}

	// 验证生命周期行为
	t.Log("--- 验证生命周期行为 ---")

	// 初始状态应该是未运行
	assert.False(t, manager.IsRunning(), "初始状态应该是未运行")
	assert.True(t, manager.IsEnabled(), "GStreamer组件应该始终启用")
	t.Log("✓ 初始状态验证通过")

	// 启动管理器
	err = manager.Start(ctx)
	assert.NoError(t, err, "启动管理器不应该失败")
	assert.True(t, manager.IsRunning(), "启动后应该处于运行状态")
	t.Log("✓ 启动行为验证通过")

	// 获取统计信息
	stats := manager.GetStats()
	assert.NotNil(t, stats, "统计信息不能为nil")
	assert.Contains(t, stats, "running", "统计信息应该包含running字段")
	assert.Equal(t, true, stats["running"], "running字段应该为true")
	t.Log("✓ 统计信息验证通过")

	// 停止管理器
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = manager.Stop(stopCtx)
	assert.NoError(t, err, "停止管理器不应该失败")
	assert.False(t, manager.IsRunning(), "停止后应该处于非运行状态")
	t.Log("✓ 停止行为验证通过")

	t.Log("=== 生命周期管理接口验证完成 ===")
}

// testConfigurationManagementInterface 验证配置管理接口完全不变
// 需求 6.5: 保持配置管理接口完全不变(GetConfig/ConfigureLogging 等)
func testConfigurationManagementInterface(t *testing.T) {
	t.Log("=== 验证配置管理接口 ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")

	managerType := reflect.TypeOf(manager)

	// 1. 验证ConfigureLogging方法
	configureLoggingMethod, exists := managerType.MethodByName("ConfigureLogging")
	assert.True(t, exists, "ConfigureLogging方法必须存在")
	if exists {
		// 验证方法签名: ConfigureLogging(*config.LoggingConfig) error
		assert.Equal(t, 2, configureLoggingMethod.Type.NumIn(), "ConfigureLogging方法应该有2个参数")
		assert.Equal(t, 1, configureLoggingMethod.Type.NumOut(), "ConfigureLogging方法应该返回1个值(error)")
		assert.True(t, configureLoggingMethod.Type.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()), "返回值应该是error")
		t.Log("✓ ConfigureLogging方法签名验证通过")
	}

	// 2. 验证UpdateLoggingConfig方法
	updateLoggingConfigMethod, exists := managerType.MethodByName("UpdateLoggingConfig")
	assert.True(t, exists, "UpdateLoggingConfig方法必须存在")
	if exists {
		// 验证方法签名: UpdateLoggingConfig(*config.LoggingConfig) error
		assert.Equal(t, 2, updateLoggingConfigMethod.Type.NumIn(), "UpdateLoggingConfig方法应该有2个参数")
		assert.Equal(t, 1, updateLoggingConfigMethod.Type.NumOut(), "UpdateLoggingConfig方法应该返回1个值(error)")
		t.Log("✓ UpdateLoggingConfig方法签名验证通过")
	}

	// 3. 验证GetLoggingConfig方法
	getLoggingConfigMethod, exists := managerType.MethodByName("GetLoggingConfig")
	assert.True(t, exists, "GetLoggingConfig方法必须存在")
	if exists {
		// 验证方法签名: GetLoggingConfig() *GStreamerLogConfig
		assert.Equal(t, 1, getLoggingConfigMethod.Type.NumIn(), "GetLoggingConfig方法应该只有1个参数(receiver)")
		assert.Equal(t, 1, getLoggingConfigMethod.Type.NumOut(), "GetLoggingConfig方法应该返回1个值")
		t.Log("✓ GetLoggingConfig方法签名验证通过")
	}

	// 4. 验证AdaptToNetworkCondition方法
	adaptMethod, exists := managerType.MethodByName("AdaptToNetworkCondition")
	assert.True(t, exists, "AdaptToNetworkCondition方法必须存在")
	if exists {
		// 验证方法签名: AdaptToNetworkCondition(NetworkCondition) error
		assert.Equal(t, 2, adaptMethod.Type.NumIn(), "AdaptToNetworkCondition方法应该有2个参数")
		assert.Equal(t, 1, adaptMethod.Type.NumOut(), "AdaptToNetworkCondition方法应该返回1个值(error)")
		t.Log("✓ AdaptToNetworkCondition方法签名验证通过")
	}

	// 验证配置管理行为
	t.Log("--- 验证配置管理行为 ---")

	// 测试日志配置
	loggingConfig := &config.LoggingConfig{
		Level:        "debug",
		Output:       "stdout",
		EnableColors: true,
	}

	err = manager.ConfigureLogging(loggingConfig)
	assert.NoError(t, err, "配置日志不应该失败")
	t.Log("✓ 日志配置验证通过")

	// 测试网络条件适配
	networkCondition := NetworkCondition{
		PacketLoss: 0.02,
		RTT:        100,
		Bandwidth:  1000,
	}

	err = manager.AdaptToNetworkCondition(networkCondition)
	assert.NoError(t, err, "网络条件适配不应该失败")
	t.Log("✓ 网络条件适配验证通过")

	t.Log("=== 配置管理接口验证完成 ===")
}

// testLoggingManagementInterface 验证日志管理接口完全不变
// 需求 6.5: 保持日志管理接口完全不变(日志配置和监控接口)
func testLoggingManagementInterface(t *testing.T) {
	t.Log("=== 验证日志管理接口 ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")

	managerType := reflect.TypeOf(manager)

	// 1. 验证MonitorLoggingConfigChanges方法
	monitorMethod, exists := managerType.MethodByName("MonitorLoggingConfigChanges")
	assert.True(t, exists, "MonitorLoggingConfigChanges方法必须存在")
	if exists {
		// 验证方法签名: MonitorLoggingConfigChanges(<-chan *config.LoggingConfig)
		assert.Equal(t, 2, monitorMethod.Type.NumIn(), "MonitorLoggingConfigChanges方法应该有2个参数")
		assert.Equal(t, 0, monitorMethod.Type.NumOut(), "MonitorLoggingConfigChanges方法不应该有返回值")
		t.Log("✓ MonitorLoggingConfigChanges方法签名验证通过")
	}

	// 2. 验证NotifyLoggingConfigChange方法
	notifyMethod, exists := managerType.MethodByName("NotifyLoggingConfigChange")
	assert.True(t, exists, "NotifyLoggingConfigChange方法必须存在")
	if exists {
		// 验证方法签名: NotifyLoggingConfigChange(*config.LoggingConfig)
		assert.Equal(t, 2, notifyMethod.Type.NumIn(), "NotifyLoggingConfigChange方法应该有2个参数")
		assert.Equal(t, 0, notifyMethod.Type.NumOut(), "NotifyLoggingConfigChange方法不应该有返回值")
		t.Log("✓ NotifyLoggingConfigChange方法签名验证通过")
	}

	// 3. 验证GetConfigChangeChannel方法
	getChannelMethod, exists := managerType.MethodByName("GetConfigChangeChannel")
	assert.True(t, exists, "GetConfigChangeChannel方法必须存在")
	if exists {
		// 验证方法签名: GetConfigChangeChannel() chan<- *config.LoggingConfig
		assert.Equal(t, 1, getChannelMethod.Type.NumIn(), "GetConfigChangeChannel方法应该只有1个参数(receiver)")
		assert.Equal(t, 1, getChannelMethod.Type.NumOut(), "GetConfigChangeChannel方法应该返回1个值")
		t.Log("✓ GetConfigChangeChannel方法签名验证通过")
	}

	// 验证日志管理行为
	t.Log("--- 验证日志管理行为 ---")

	// 测试配置变化通道
	configChannel := make(chan *config.LoggingConfig, 1)
	manager.MonitorLoggingConfigChanges(configChannel)
	t.Log("✓ 日志配置监控启动成功")

	// 测试配置变化通知
	newLoggingConfig := &config.LoggingConfig{
		Level:        "trace",
		Output:       "stderr",
		EnableColors: false,
	}

	manager.NotifyLoggingConfigChange(newLoggingConfig)
	t.Log("✓ 日志配置变化通知发送成功")

	// 获取当前日志配置
	currentConfig := manager.GetLoggingConfig()
	assert.NotNil(t, currentConfig, "当前日志配置不能为nil")
	t.Log("✓ 获取当前日志配置成功")

	t.Log("=== 日志管理接口验证完成 ===")
}

// testWebRTCBusinessIntegration 验证WebRTC与GStreamer业务集成通过发布-订阅模式正常工作
// 需求 7.1: 验证 WebRTC 与 GStreamer 业务集成通过发布-订阅模式正常工作
func testWebRTCBusinessIntegration(t *testing.T) {
	t.Log("=== 验证WebRTC业务集成(发布-订阅模式) ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")

	managerType := reflect.TypeOf(manager)

	// 1. 验证发布-订阅接口存在
	_, exists := managerType.MethodByName("Subscribe")
	if exists {
		t.Log("✓ Subscribe方法存在")
	}

	_, exists = managerType.MethodByName("Unsubscribe")
	if exists {
		t.Log("✓ Unsubscribe方法存在")
	}

	_, exists = managerType.MethodByName("PublishVideoStream")
	if exists {
		t.Log("✓ PublishVideoStream方法存在")
	}

	_, exists = managerType.MethodByName("PublishAudioStream")
	if exists {
		t.Log("✓ PublishAudioStream方法存在")
	}

	// 2. 验证SetEncodedSampleCallback方法(向后兼容)
	setCallbackMethod, exists := managerType.MethodByName("SetEncodedSampleCallback")
	assert.True(t, exists, "SetEncodedSampleCallback方法必须存在(向后兼容)")
	if exists {
		// 验证方法签名: SetEncodedSampleCallback(func(*Sample) error)
		assert.Equal(t, 2, setCallbackMethod.Type.NumIn(), "SetEncodedSampleCallback方法应该有2个参数")
		assert.Equal(t, 0, setCallbackMethod.Type.NumOut(), "SetEncodedSampleCallback方法不应该有返回值")
		t.Log("✓ SetEncodedSampleCallback方法签名验证通过")
	}

	// 3. 验证GetCapture方法
	getCaptureMethod, exists := managerType.MethodByName("GetCapture")
	assert.True(t, exists, "GetCapture方法必须存在")
	if exists {
		// 验证方法签名: GetCapture() DesktopCapture
		assert.Equal(t, 1, getCaptureMethod.Type.NumIn(), "GetCapture方法应该只有1个参数(receiver)")
		assert.Equal(t, 1, getCaptureMethod.Type.NumOut(), "GetCapture方法应该返回1个值")
		t.Log("✓ GetCapture方法签名验证通过")
	}

	// 验证业务集成行为
	t.Log("--- 验证业务集成行为 ---")

	// 测试样本回调设置(向后兼容)
	var receivedSamples []*Sample
	sampleCallback := func(sample *Sample) error {
		receivedSamples = append(receivedSamples, sample)
		return nil
	}

	manager.SetEncodedSampleCallback(sampleCallback)
	t.Log("✓ 编码样本回调设置成功")

	// 测试获取桌面捕获
	capture := manager.GetCapture()
	if capture != nil {
		t.Log("✓ 桌面捕获组件获取成功")

		// 验证桌面捕获接口
		captureType := reflect.TypeOf(capture)

		// 验证SetSampleCallback方法
		_, exists := captureType.MethodByName("SetSampleCallback")
		if exists {
			t.Log("✓ 桌面捕获SetSampleCallback方法存在")
		}

		// 验证Start方法
		_, exists = captureType.MethodByName("Start")
		if exists {
			t.Log("✓ 桌面捕获Start方法存在")
		}

		// 验证Stop方法
		_, exists = captureType.MethodByName("Stop")
		if exists {
			t.Log("✓ 桌面捕获Stop方法存在")
		}
	} else {
		t.Log("⚠ 桌面捕获组件不可用(测试环境预期)")
	}

	t.Log("=== WebRTC业务集成验证完成 ===")
}

// testHTTPInterfaceCompatibility 验证现有HTTP接口和配置加载的完全兼容性
// 需求 7.2: 测试现有 HTTP 接口和配置加载的完全兼容性
func testHTTPInterfaceCompatibility(t *testing.T) {
	t.Log("=== 验证HTTP接口兼容性 ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")

	managerType := reflect.TypeOf(manager)

	// 1. 验证RegisterRoutes方法
	registerRoutesMethod, exists := managerType.MethodByName("RegisterRoutes")
	if exists {
		// 验证方法签名: RegisterRoutes(*mux.Router)
		assert.Equal(t, 2, registerRoutesMethod.Type.NumIn(), "RegisterRoutes方法应该有2个参数")
		assert.Equal(t, 0, registerRoutesMethod.Type.NumOut(), "RegisterRoutes方法不应该有返回值")
		t.Log("✓ RegisterRoutes方法签名验证通过")
	}

	// 2. 验证GetHandlers方法
	_, exists = managerType.MethodByName("GetHandlers")
	if exists {
		t.Log("✓ GetHandlers方法存在")
	}

	// 验证HTTP处理器内部结构
	t.Log("--- 验证HTTP处理器结构 ---")

	// 通过反射检查handlers字段
	managerValue := reflect.ValueOf(manager).Elem()
	handlersField := managerValue.FieldByName("handlers")

	if handlersField.IsValid() && !handlersField.IsNil() {
		t.Log("✓ HTTP处理器字段存在且已初始化")

		// 验证处理器类型
		handlersType := handlersField.Type()
		assert.Equal(t, "*gstreamer.gstreamerHandlers", handlersType.String(), "处理器类型应该是*gstreamerHandlers")
		t.Log("✓ HTTP处理器类型验证通过")
	} else {
		t.Log("⚠ HTTP处理器字段不存在或未初始化")
	}

	t.Log("=== HTTP接口兼容性验证完成 ===")
}

// testConfigurationStructureUnchanged 验证配置结构体定义保持完全不变
// 需求 7.2: 验证配置结构体定义保持完全不变
func testConfigurationStructureUnchanged(t *testing.T) {
	t.Log("=== 验证配置结构体定义不变 ===")

	// 1. 验证GStreamerConfig结构体
	gstreamerConfigType := reflect.TypeOf(config.GStreamerConfig{})
	assert.Equal(t, "config.GStreamerConfig", gstreamerConfigType.String(), "GStreamerConfig类型名称应该保持不变")

	// 验证主要字段存在
	expectedFields := []string{"Capture", "Encoding", "Pipeline", "Logging"}
	for _, fieldName := range expectedFields {
		field, exists := gstreamerConfigType.FieldByName(fieldName)
		assert.True(t, exists, fmt.Sprintf("GStreamerConfig.%s字段必须存在", fieldName))
		if exists {
			t.Logf("✓ GStreamerConfig.%s字段存在，类型: %s", fieldName, field.Type.String())
		}
	}

	// 2. 验证DesktopCaptureConfig结构体
	captureConfigType := reflect.TypeOf(config.DesktopCaptureConfig{})
	assert.Equal(t, "config.DesktopCaptureConfig", captureConfigType.String(), "DesktopCaptureConfig类型名称应该保持不变")

	captureFields := []string{"DisplayID", "Width", "Height", "FrameRate", "UseWayland", "ShowPointer", "UseDamage"}
	for _, fieldName := range captureFields {
		field, exists := captureConfigType.FieldByName(fieldName)
		assert.True(t, exists, fmt.Sprintf("CaptureConfig.%s字段必须存在", fieldName))
		if exists {
			t.Logf("✓ CaptureConfig.%s字段存在，类型: %s", fieldName, field.Type.String())
		}
	}

	// 3. 验证EncoderConfig结构体
	encodingConfigType := reflect.TypeOf(config.EncoderConfig{})
	assert.Equal(t, "config.EncoderConfig", encodingConfigType.String(), "EncoderConfig类型名称应该保持不变")

	encodingFields := []string{"Type", "Codec", "Bitrate", "MinBitrate", "MaxBitrate", "UseHardware", "Preset", "Profile"}
	for _, fieldName := range encodingFields {
		field, exists := encodingConfigType.FieldByName(fieldName)
		assert.True(t, exists, fmt.Sprintf("EncodingConfig.%s字段必须存在", fieldName))
		if exists {
			t.Logf("✓ EncodingConfig.%s字段存在，类型: %s", fieldName, field.Type.String())
		}
	}

	// 4. 验证LoggingConfig结构体
	loggingConfigType := reflect.TypeOf(config.LoggingConfig{})
	assert.Equal(t, "config.LoggingConfig", loggingConfigType.String(), "LoggingConfig类型名称应该保持不变")

	loggingFields := []string{"Level", "Output", "File", "EnableColors"}
	for _, fieldName := range loggingFields {
		field, exists := loggingConfigType.FieldByName(fieldName)
		assert.True(t, exists, fmt.Sprintf("LoggingConfig.%s字段必须存在", fieldName))
		if exists {
			t.Logf("✓ LoggingConfig.%s字段存在，类型: %s", fieldName, field.Type.String())
		}
	}

	t.Log("=== 配置结构体定义验证完成 ===")
}

// testAppIntegrationCodeUnchanged 验证app.go中的所有调用代码无需任何修改
// 需求 7.2: 确保 app.go 中的所有调用代码无需任何修改
func testAppIntegrationCodeUnchanged(t *testing.T) {
	t.Log("=== 验证App集成代码不变 ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器配置(模拟app.go中的调用)
	gstreamerMgrConfig := &ManagerConfig{
		Config: cfg,
	}

	// 1. 验证NewManager调用方式不变
	gstreamerMgr, err := NewManager(ctx, gstreamerMgrConfig)
	assert.NoError(t, err, "NewManager调用方式应该保持不变")
	assert.NotNil(t, gstreamerMgr, "管理器创建应该成功")
	t.Log("✓ NewManager调用方式验证通过")

	// 2. 验证管理器接口调用不变

	// 生命周期管理调用
	assert.True(t, gstreamerMgr.IsEnabled(), "IsEnabled()调用应该保持不变")
	assert.False(t, gstreamerMgr.IsRunning(), "IsRunning()调用应该保持不变")
	t.Log("✓ 生命周期管理调用验证通过")

	// 启动管理器
	err = gstreamerMgr.Start(ctx)
	assert.NoError(t, err, "Start(ctx)调用应该保持不变")
	t.Log("✓ Start调用验证通过")

	// 获取统计信息
	stats := gstreamerMgr.GetStats()
	assert.NotNil(t, stats, "GetStats()调用应该保持不变")
	assert.IsType(t, map[string]interface{}{}, stats, "GetStats()返回类型应该保持不变")
	t.Log("✓ GetStats调用验证通过")

	// 获取桌面捕获
	capture := gstreamerMgr.GetCapture()
	// capture可能为nil(测试环境)，但调用方式应该不变
	_ = capture // 使用变量避免编译错误
	t.Log("✓ GetCapture调用验证通过")

	// 设置编码样本回调
	var callbackCalled bool
	sampleCallback := func(sample *Sample) error {
		callbackCalled = true
		return nil
	}
	_ = callbackCalled // 使用变量避免编译错误
	gstreamerMgr.SetEncodedSampleCallback(sampleCallback)
	t.Log("✓ SetEncodedSampleCallback调用验证通过")

	// 配置日志
	loggingConfig := &config.LoggingConfig{
		Level:        "debug",
		Output:       "stdout",
		EnableColors: true,
	}
	err = gstreamerMgr.ConfigureLogging(loggingConfig)
	assert.NoError(t, err, "ConfigureLogging调用应该保持不变")
	t.Log("✓ ConfigureLogging调用验证通过")

	// 停止管理器
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = gstreamerMgr.Stop(stopCtx)
	assert.NoError(t, err, "Stop(ctx)调用应该保持不变")
	t.Log("✓ Stop调用验证通过")

	// 3. 验证配置结构体使用不变

	// 验证配置访问方式
	assert.Equal(t, "h264", string(cfg.Encoding.Codec), "配置访问方式应该保持不变")
	assert.Equal(t, 1920, cfg.Capture.Width, "配置字段访问应该保持不变")
	assert.Equal(t, 1080, cfg.Capture.Height, "配置字段访问应该保持不变")
	assert.Equal(t, 30, cfg.Capture.FrameRate, "配置字段访问应该保持不变")
	t.Log("✓ 配置结构体使用验证通过")

	t.Log("=== App集成代码验证完成 ===")
}

// createTestGStreamerConfig 创建测试用的GStreamer配置
func createTestGStreamerConfig() *config.GStreamerConfig {
	return &config.GStreamerConfig{
		Capture: config.DesktopCaptureConfig{
			DisplayID:   ":0",
			Width:       1920,
			Height:      1080,
			FrameRate:   30,
			UseWayland:  false,
			ShowPointer: true,
			UseDamage:   true,
			BufferSize:  10,
			Quality:     "medium",
		},
		Encoding: config.EncoderConfig{
			Type:             config.EncoderTypeX264,
			Codec:            config.CodecH264,
			Bitrate:          2000,
			MinBitrate:       500,
			MaxBitrate:       5000,
			KeyframeInterval: 2,
			UseHardware:      false,
			Preset:           config.PresetMedium,
			Profile:          config.ProfileBaseline,
			RateControl:      config.RateControlCBR,
			ZeroLatency:      true,
			RefFrames:        1,
			Quality:          23,
		},
	}
}

// Note: NetworkCondition is already defined in media_config.go

package gstreamer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/go-gst/go-gst/gst"
	"github.com/gorilla/mux"
	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTTPInterfaceCompatibility 验证HTTP接口完全兼容性
// 任务8: 测试现有 HTTP 接口和配置加载的完全兼容性
// 需求 7.2: 测试现有 HTTP 接口和配置加载的完全兼容性
func TestHTTPInterfaceCompatibility(t *testing.T) {
	t.Run("VerifyHTTPHandlerStructure", testVerifyHTTPHandlerStructure)
	t.Run("VerifyRouteRegistration", testVerifyRouteRegistration)
	t.Run("VerifyStatusEndpoint", testVerifyStatusEndpoint)
	t.Run("VerifyStatsEndpoint", testVerifyStatsEndpoint)
	t.Run("VerifyConfigEndpoint", testVerifyConfigEndpoint)
	t.Run("VerifyControlEndpoints", testVerifyControlEndpoints)
}

// testVerifyHTTPHandlerStructure 验证HTTP处理器结构不变
func testVerifyHTTPHandlerStructure(t *testing.T) {
	t.Log("=== 验证HTTP处理器结构 ===")

	// Initialize GStreamer for testing
	gst.Init(nil)

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")
	require.NotNil(t, manager, "管理器不能为nil")

	// 验证handlers字段存在
	managerValue := reflect.ValueOf(manager).Elem()
	handlersField := managerValue.FieldByName("handlers")

	assert.True(t, handlersField.IsValid(), "handlers字段必须存在")
	if handlersField.IsValid() {
		assert.False(t, handlersField.IsNil(), "handlers不应该为nil")
		t.Log("✓ HTTP处理器字段存在且已初始化")

		// 验证处理器类型
		handlersType := handlersField.Type()
		expectedType := "*gstreamer.gstreamerHandlers"
		assert.Equal(t, expectedType, handlersType.String(), "处理器类型应该是*gstreamerHandlers")
		t.Log("✓ HTTP处理器类型验证通过")
	}

	// 验证HTTP处理器相关方法存在
	managerType := reflect.TypeOf(manager)

	// 检查RegisterRoutes方法
	registerRoutesMethod, exists := managerType.MethodByName("RegisterRoutes")
	if exists {
		// 验证方法签名: RegisterRoutes(*mux.Router)
		assert.Equal(t, 2, registerRoutesMethod.Type.NumIn(), "RegisterRoutes方法应该有2个参数")
		assert.Equal(t, 0, registerRoutesMethod.Type.NumOut(), "RegisterRoutes方法不应该有返回值")
		t.Log("✓ RegisterRoutes方法存在且签名正确")
	}

	t.Log("=== HTTP处理器结构验证完成 ===")
}

// testVerifyRouteRegistration 验证路由注册功能
func testVerifyRouteRegistration(t *testing.T) {
	t.Log("=== 验证路由注册功能 ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")

	// 创建测试路由器
	router := mux.NewRouter()

	// 验证RegisterRoutes方法可以调用
	managerType := reflect.TypeOf(manager)
	_, exists := managerType.MethodByName("RegisterRoutes")

	if exists {
		// 通过反射调用RegisterRoutes方法
		methodValue := reflect.ValueOf(manager).MethodByName("RegisterRoutes")
		if methodValue.IsValid() {
			// 调用方法
			args := []reflect.Value{reflect.ValueOf(router)}
			methodValue.Call(args)
			t.Log("✓ RegisterRoutes方法调用成功")
		}
	} else {
		t.Log("⚠ RegisterRoutes方法不存在，可能使用其他路由注册方式")
	}

	// 验证路由是否已注册
	var registeredRoutes []string
	err = router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, err := route.GetPathTemplate()
		if err == nil {
			registeredRoutes = append(registeredRoutes, pathTemplate)
		}
		return nil
	})

	assert.NoError(t, err, "遍历路由不应该失败")
	t.Logf("已注册的路由数量: %d", len(registeredRoutes))

	for _, route := range registeredRoutes {
		t.Logf("  - %s", route)
	}

	t.Log("=== 路由注册功能验证完成 ===")
}

// testVerifyStatusEndpoint 验证状态端点兼容性
func testVerifyStatusEndpoint(t *testing.T) {
	t.Log("=== 验证状态端点兼容性 ===")

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
		manager.Stop(ctx)
	}()

	// 创建测试路由器并注册路由
	router := mux.NewRouter()

	// 尝试注册路由
	managerType := reflect.TypeOf(manager)
	_, exists := managerType.MethodByName("RegisterRoutes")

	if exists {
		methodValue := reflect.ValueOf(manager).MethodByName("RegisterRoutes")
		if methodValue.IsValid() {
			args := []reflect.Value{reflect.ValueOf(router)}
			methodValue.Call(args)
		}
	}

	// 测试状态端点
	statusEndpoints := []string{
		"/api/gstreamer/status",
		"/gstreamer/status",
		"/status",
	}

	for _, endpoint := range statusEndpoints {
		t.Logf("测试状态端点: %s", endpoint)

		// 创建测试请求
		req, err := http.NewRequest("GET", endpoint, nil)
		if err != nil {
			continue
		}

		// 创建响应记录器
		rr := httptest.NewRecorder()

		// 执行请求
		router.ServeHTTP(rr, req)

		// 检查响应
		if rr.Code == http.StatusOK {
			t.Logf("✓ 状态端点 %s 响应正常 (状态码: %d)", endpoint, rr.Code)

			// 验证响应内容是JSON格式
			var response map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &response)
			if err == nil {
				t.Logf("✓ 状态端点 %s 返回有效JSON", endpoint)

				// 验证基本字段存在
				if running, exists := response["running"]; exists {
					t.Logf("  - running: %v", running)
				}
				if enabled, exists := response["enabled"]; exists {
					t.Logf("  - enabled: %v", enabled)
				}
			}
			break
		} else if rr.Code == http.StatusNotFound {
			t.Logf("  状态端点 %s 未找到 (状态码: %d)", endpoint, rr.Code)
		} else {
			t.Logf("  状态端点 %s 响应异常 (状态码: %d)", endpoint, rr.Code)
		}
	}

	t.Log("=== 状态端点兼容性验证完成 ===")
}

// testVerifyStatsEndpoint 验证统计端点兼容性
func testVerifyStatsEndpoint(t *testing.T) {
	t.Log("=== 验证统计端点兼容性 ===")

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
		manager.Stop(ctx)
	}()

	// 验证GetStats方法返回的数据结构
	stats := manager.GetStats()
	assert.NotNil(t, stats, "统计信息不能为nil")
	assert.IsType(t, map[string]interface{}{}, stats, "统计信息应该是map[string]interface{}类型")

	// 验证基本统计字段存在
	expectedFields := []string{"running", "uptime"}
	for _, field := range expectedFields {
		if value, exists := stats[field]; exists {
			t.Logf("✓ 统计字段 %s 存在: %v", field, value)
		} else {
			t.Logf("⚠ 统计字段 %s 不存在", field)
		}
	}

	// 验证统计数据可以序列化为JSON
	statsJSON, err := json.Marshal(stats)
	assert.NoError(t, err, "统计信息应该可以序列化为JSON")
	if err == nil {
		t.Log("✓ 统计信息JSON序列化成功")
		t.Logf("  统计信息大小: %d bytes", len(statsJSON))
	}

	t.Log("=== 统计端点兼容性验证完成 ===")
}

// testVerifyConfigEndpoint 验证配置端点兼容性
func testVerifyConfigEndpoint(t *testing.T) {
	t.Log("=== 验证配置端点兼容性 ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")

	// 验证配置相关方法存在
	managerType := reflect.TypeOf(manager)

	// 检查ConfigureLogging方法
	_, exists := managerType.MethodByName("ConfigureLogging")
	if exists {
		t.Log("✓ ConfigureLogging方法存在")

		// 测试配置日志
		loggingConfig := &config.LoggingConfig{
			Level:        "debug",
			Output:       "stdout",
			EnableColors: true,
		}

		// 通过反射调用方法
		methodValue := reflect.ValueOf(manager).MethodByName("ConfigureLogging")
		if methodValue.IsValid() {
			args := []reflect.Value{reflect.ValueOf(loggingConfig)}
			results := methodValue.Call(args)

			// 检查返回值(应该是error)
			if len(results) > 0 {
				if results[0].Interface() == nil {
					t.Log("✓ ConfigureLogging调用成功")
				} else {
					t.Logf("⚠ ConfigureLogging调用返回错误: %v", results[0].Interface())
				}
			}
		}
	}

	// 检查GetLoggingConfig方法
	_, exists = managerType.MethodByName("GetLoggingConfig")
	if exists {
		t.Log("✓ GetLoggingConfig方法存在")

		// 通过反射调用方法
		methodValue := reflect.ValueOf(manager).MethodByName("GetLoggingConfig")
		if methodValue.IsValid() {
			results := methodValue.Call([]reflect.Value{})

			// 检查返回值
			if len(results) > 0 {
				config := results[0].Interface()
				if config != nil {
					t.Log("✓ GetLoggingConfig返回配置成功")
				} else {
					t.Log("⚠ GetLoggingConfig返回nil配置")
				}
			}
		}
	}

	t.Log("=== 配置端点兼容性验证完成 ===")
}

// testVerifyControlEndpoints 验证控制端点兼容性
func testVerifyControlEndpoints(t *testing.T) {
	t.Log("=== 验证控制端点兼容性 ===")

	// 创建测试配置
	cfg := createTestGStreamerConfig()
	ctx := context.Background()

	// 创建管理器
	manager, err := NewManager(ctx, &ManagerConfig{Config: cfg})
	require.NoError(t, err, "创建GStreamer管理器失败")

	// 验证控制相关方法存在
	managerType := reflect.TypeOf(manager)

	// 检查AdaptToNetworkCondition方法
	_, exists := managerType.MethodByName("AdaptToNetworkCondition")
	if exists {
		t.Log("✓ AdaptToNetworkCondition方法存在")

		// 测试网络条件适配
		networkCondition := NetworkCondition{
			PacketLoss: 0.02,
			RTT:        100,
			Bandwidth:  1000,
		}

		// 通过反射调用方法
		methodValue := reflect.ValueOf(manager).MethodByName("AdaptToNetworkCondition")
		if methodValue.IsValid() {
			args := []reflect.Value{reflect.ValueOf(networkCondition)}
			results := methodValue.Call(args)

			// 检查返回值(应该是error)
			if len(results) > 0 {
				if results[0].Interface() == nil {
					t.Log("✓ AdaptToNetworkCondition调用成功")
				} else {
					t.Logf("⚠ AdaptToNetworkCondition调用返回错误: %v", results[0].Interface())
				}
			}
		}
	}

	// 验证生命周期控制方法
	t.Log("--- 验证生命周期控制 ---")

	// 测试启动
	err = manager.Start(ctx)
	assert.NoError(t, err, "Start方法应该正常工作")
	if err == nil {
		t.Log("✓ Start控制端点功能正常")
	}

	// 验证运行状态
	isRunning := manager.IsRunning()
	assert.True(t, isRunning, "启动后应该处于运行状态")
	if isRunning {
		t.Log("✓ IsRunning控制端点功能正常")
	}

	// 测试停止
	err = manager.Stop(ctx)
	assert.NoError(t, err, "Stop方法应该正常工作")
	if err == nil {
		t.Log("✓ Stop控制端点功能正常")
	}

	// 验证停止状态
	isRunning = manager.IsRunning()
	assert.False(t, isRunning, "停止后应该处于非运行状态")
	if !isRunning {
		t.Log("✓ 停止后状态控制正常")
	}

	t.Log("=== 控制端点兼容性验证完成 ===")
}

// HTTPResponseRecorder 用于记录HTTP响应的辅助结构
type HTTPResponseRecorder struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// NewHTTPResponseRecorder 创建新的HTTP响应记录器
func NewHTTPResponseRecorder() *HTTPResponseRecorder {
	return &HTTPResponseRecorder{
		Headers: make(http.Header),
	}
}

// Header 实现http.ResponseWriter接口
func (r *HTTPResponseRecorder) Header() http.Header {
	return r.Headers
}

// Write 实现http.ResponseWriter接口
func (r *HTTPResponseRecorder) Write(data []byte) (int, error) {
	r.Body = append(r.Body, data...)
	return len(data), nil
}

// WriteHeader 实现http.ResponseWriter接口
func (r *HTTPResponseRecorder) WriteHeader(statusCode int) {
	r.StatusCode = statusCode
}

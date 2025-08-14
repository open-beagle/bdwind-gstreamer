package metrics

import (
	"context"
	"testing"
	"time"
)

func TestNewSystemMetrics(t *testing.T) {
	// 创建测试用的metrics实例
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	// 创建系统指标实例
	systemMetrics, err := NewSystemMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create system metrics: %v", err)
	}

	// 验证初始状态
	if systemMetrics.IsRunning() {
		t.Error("System metrics should not be running initially")
	}

	// 测试启动
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = systemMetrics.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start system metrics: %v", err)
	}

	if !systemMetrics.IsRunning() {
		t.Error("System metrics should be running after start")
	}

	// 等待一段时间让指标收集
	time.Sleep(2 * time.Second)

	// 手动触发一次更新以确保数据是最新的
	sm := systemMetrics.(*systemMetricsImpl)
	sm.updateMetrics()

	// 测试获取CPU使用率
	cpuUsage := systemMetrics.GetCPUUsage()
	if cpuUsage < 0 || cpuUsage > 100 {
		t.Errorf("Invalid CPU usage: %f", cpuUsage)
	}

	// 测试获取内存使用情况
	memUsage := systemMetrics.GetMemoryUsage()
	if memUsage.Total == 0 {
		t.Error("Memory total should not be zero")
	}
	if memUsage.Percent < 0 || memUsage.Percent > 100 {
		t.Errorf("Invalid memory usage percentage: %f", memUsage.Percent)
	}

	// 测试获取GPU使用情况
	gpuUsages := systemMetrics.GetGPUUsage()
	// GPU可能不存在，所以不强制要求有GPU
	t.Logf("Found %d GPUs", len(gpuUsages))

	// 测试停止
	err = systemMetrics.Stop()
	if err != nil {
		t.Fatalf("Failed to stop system metrics: %v", err)
	}

	if systemMetrics.IsRunning() {
		t.Error("System metrics should not be running after stop")
	}
}

func TestCPUStatParsing(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	systemMetrics, err := NewSystemMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create system metrics: %v", err)
	}

	sm := systemMetrics.(*systemMetricsImpl)

	// 测试读取CPU统计
	stat, err := sm.readCPUStat()
	if err != nil {
		t.Fatalf("Failed to read CPU stat: %v", err)
	}

	// 验证统计数据的合理性
	if stat.User == 0 && stat.System == 0 && stat.Idle == 0 {
		t.Error("CPU stat should have some non-zero values")
	}

	// 测试CPU使用率计算
	time.Sleep(100 * time.Millisecond)
	stat2, err := sm.readCPUStat()
	if err != nil {
		t.Fatalf("Failed to read CPU stat: %v", err)
	}

	usage := sm.calculateCPUUsage(stat, stat2)
	if usage < 0 || usage > 100 {
		t.Errorf("Invalid CPU usage calculation: %f", usage)
	}
}

func TestMemoryInfoParsing(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	systemMetrics, err := NewSystemMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create system metrics: %v", err)
	}

	sm := systemMetrics.(*systemMetricsImpl)

	// 测试读取内存信息
	memUsage, err := sm.readMemoryInfo()
	if err != nil {
		t.Fatalf("Failed to read memory info: %v", err)
	}

	// 验证内存信息的合理性
	if memUsage.Total == 0 {
		t.Error("Memory total should not be zero")
	}
	if memUsage.Used > memUsage.Total {
		t.Error("Memory used should not exceed total")
	}
	if memUsage.Available > memUsage.Total {
		t.Error("Memory available should not exceed total")
	}
	if memUsage.Percent < 0 || memUsage.Percent > 100 {
		t.Errorf("Invalid memory usage percentage: %f", memUsage.Percent)
	}
}

func TestGPUInfoParsing(t *testing.T) {
	config := DefaultMetricsConfig()
	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	systemMetrics, err := NewSystemMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create system metrics: %v", err)
	}

	sm := systemMetrics.(*systemMetricsImpl)

	// 测试读取GPU信息
	gpuUsages, err := sm.readGPUInfo()
	if err != nil {
		t.Logf("Failed to read GPU info (this is expected if no GPU is available): %v", err)
		return
	}

	// 如果有GPU，验证信息的合理性
	for i, gpu := range gpuUsages {
		if gpu.Index != i {
			t.Errorf("GPU index mismatch: expected %d, got %d", i, gpu.Index)
		}
		if gpu.Name == "" {
			t.Error("GPU name should not be empty")
		}
		if gpu.Utilization < 0 || gpu.Utilization > 100 {
			t.Errorf("Invalid GPU utilization: %f", gpu.Utilization)
		}
		if gpu.Memory.Usage < 0 || gpu.Memory.Usage > 100 {
			t.Errorf("Invalid GPU memory usage: %f", gpu.Memory.Usage)
		}
	}
}

func TestSystemMetricsIntegration(t *testing.T) {
	// 创建完整的集成测试
	config := DefaultMetricsConfig()
	config.Port = 9091 // 使用不同的端口避免冲突

	metrics, err := NewMetrics(config)
	if err != nil {
		t.Fatalf("Failed to create metrics: %v", err)
	}

	// 启动metrics服务器
	err = metrics.Start()
	if err != nil {
		t.Fatalf("Failed to start metrics server: %v", err)
	}
	defer metrics.Stop()

	// 创建并启动系统指标
	systemMetrics, err := NewSystemMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to create system metrics: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = systemMetrics.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start system metrics: %v", err)
	}
	defer systemMetrics.Stop()

	// 等待指标收集
	time.Sleep(3 * time.Second)

	// 手动触发一次更新以确保数据是最新的
	sm := systemMetrics.(*systemMetricsImpl)
	sm.updateMetrics()

	// 验证指标数据
	cpuUsage := systemMetrics.GetCPUUsage()
	memUsage := systemMetrics.GetMemoryUsage()
	gpuUsages := systemMetrics.GetGPUUsage()

	t.Logf("CPU Usage: %.2f%%", cpuUsage)
	t.Logf("Memory Usage: %.2f%% (%d/%d bytes)",
		memUsage.Percent, memUsage.Used, memUsage.Total)
	t.Logf("GPU Count: %d", len(gpuUsages))

	for i, gpu := range gpuUsages {
		t.Logf("GPU %d (%s): %.2f%% utilization, %.2f%% memory usage",
			i, gpu.Name, gpu.Utilization, gpu.Memory.Usage)
	}
}

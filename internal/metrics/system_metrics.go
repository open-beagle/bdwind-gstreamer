package metrics

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SystemMetrics 系统资源指标接口
type SystemMetrics interface {
	// Start 启动系统指标收集
	Start(ctx context.Context) error

	// Stop 停止系统指标收集
	Stop() error

	// GetCPUUsage 获取CPU使用率
	GetCPUUsage() float64

	// GetMemoryUsage 获取内存使用情况
	GetMemoryUsage() MemoryUsage

	// GetGPUUsage 获取GPU使用情况
	GetGPUUsage() []GPUUsage

	// IsRunning 检查是否正在运行
	IsRunning() bool
}

// MemoryUsage 内存使用情况
type MemoryUsage struct {
	Total     uint64  // 总内存 (bytes)
	Used      uint64  // 已使用内存 (bytes)
	Available uint64  // 可用内存 (bytes)
	Percent   float64 // 使用百分比
}

// GPUUsage GPU使用情况
type GPUUsage struct {
	Index       int     // GPU索引
	Name        string  // GPU名称
	Utilization float64 // GPU利用率百分比
	Memory      struct {
		Total uint64  // 总显存 (MB)
		Used  uint64  // 已使用显存 (MB)
		Free  uint64  // 空闲显存 (MB)
		Usage float64 // 显存使用百分比
	}
	Temperature int // 温度 (摄氏度)
	PowerUsage  int // 功耗 (瓦特)
}

// CPUStat CPU统计信息
type CPUStat struct {
	User      uint64
	Nice      uint64
	System    uint64
	Idle      uint64
	IOWait    uint64
	IRQ       uint64
	SoftIRQ   uint64
	Steal     uint64
	Guest     uint64
	GuestNice uint64
}

// systemMetricsImpl SystemMetrics接口的实现
type systemMetricsImpl struct {
	metrics Metrics
	running bool
	cancel  context.CancelFunc
	mu      sync.RWMutex

	// CPU指标
	cpuUsageGauge Gauge
	cpuCoresGauge Gauge

	// 内存指标
	memoryTotalGauge     Gauge
	memoryUsedGauge      Gauge
	memoryAvailableGauge Gauge
	memoryUsageGauge     Gauge

	// GPU指标
	gpuUtilizationGauge Gauge
	gpuMemoryTotalGauge Gauge
	gpuMemoryUsedGauge  Gauge
	gpuMemoryFreeGauge  Gauge
	gpuMemoryUsageGauge Gauge
	gpuTemperatureGauge Gauge
	gpuPowerUsageGauge  Gauge

	// 缓存数据
	lastCPUStat CPUStat
	cpuUsage    float64
	memUsage    MemoryUsage
	gpuUsages   []GPUUsage

	// 更新间隔
	updateInterval time.Duration
}

// NewSystemMetrics 创建新的系统指标实例
func NewSystemMetrics(metrics Metrics) (SystemMetrics, error) {
	sm := &systemMetricsImpl{
		metrics:        metrics,
		updateInterval: 5 * time.Second, // 默认5秒更新一次
	}

	// 注册CPU指标
	cpuUsageGauge, err := metrics.RegisterGauge(
		"system_cpu_usage_percent",
		"Current CPU usage percentage",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register CPU usage gauge: %w", err)
	}
	sm.cpuUsageGauge = cpuUsageGauge

	cpuCoresGauge, err := metrics.RegisterGauge(
		"system_cpu_cores_total",
		"Total number of CPU cores",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register CPU cores gauge: %w", err)
	}
	sm.cpuCoresGauge = cpuCoresGauge

	// 注册内存指标
	memoryTotalGauge, err := metrics.RegisterGauge(
		"system_memory_total_bytes",
		"Total system memory in bytes",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register memory total gauge: %w", err)
	}
	sm.memoryTotalGauge = memoryTotalGauge

	memoryUsedGauge, err := metrics.RegisterGauge(
		"system_memory_used_bytes",
		"Used system memory in bytes",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register memory used gauge: %w", err)
	}
	sm.memoryUsedGauge = memoryUsedGauge

	memoryAvailableGauge, err := metrics.RegisterGauge(
		"system_memory_available_bytes",
		"Available system memory in bytes",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register memory available gauge: %w", err)
	}
	sm.memoryAvailableGauge = memoryAvailableGauge

	memoryUsageGauge, err := metrics.RegisterGauge(
		"system_memory_usage_percent",
		"Memory usage percentage",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register memory usage gauge: %w", err)
	}
	sm.memoryUsageGauge = memoryUsageGauge

	// 注册GPU指标
	gpuUtilizationGauge, err := metrics.RegisterGauge(
		"system_gpu_utilization_percent",
		"GPU utilization percentage",
		[]string{"gpu_index", "gpu_name"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register GPU utilization gauge: %w", err)
	}
	sm.gpuUtilizationGauge = gpuUtilizationGauge

	gpuMemoryTotalGauge, err := metrics.RegisterGauge(
		"system_gpu_memory_total_mb",
		"Total GPU memory in MB",
		[]string{"gpu_index", "gpu_name"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register GPU memory total gauge: %w", err)
	}
	sm.gpuMemoryTotalGauge = gpuMemoryTotalGauge

	gpuMemoryUsedGauge, err := metrics.RegisterGauge(
		"system_gpu_memory_used_mb",
		"Used GPU memory in MB",
		[]string{"gpu_index", "gpu_name"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register GPU memory used gauge: %w", err)
	}
	sm.gpuMemoryUsedGauge = gpuMemoryUsedGauge

	gpuMemoryFreeGauge, err := metrics.RegisterGauge(
		"system_gpu_memory_free_mb",
		"Free GPU memory in MB",
		[]string{"gpu_index", "gpu_name"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register GPU memory free gauge: %w", err)
	}
	sm.gpuMemoryFreeGauge = gpuMemoryFreeGauge

	gpuMemoryUsageGauge, err := metrics.RegisterGauge(
		"system_gpu_memory_usage_percent",
		"GPU memory usage percentage",
		[]string{"gpu_index", "gpu_name"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register GPU memory usage gauge: %w", err)
	}
	sm.gpuMemoryUsageGauge = gpuMemoryUsageGauge

	gpuTemperatureGauge, err := metrics.RegisterGauge(
		"system_gpu_temperature_celsius",
		"GPU temperature in Celsius",
		[]string{"gpu_index", "gpu_name"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register GPU temperature gauge: %w", err)
	}
	sm.gpuTemperatureGauge = gpuTemperatureGauge

	gpuPowerUsageGauge, err := metrics.RegisterGauge(
		"system_gpu_power_usage_watts",
		"GPU power usage in watts",
		[]string{"gpu_index", "gpu_name"},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register GPU power usage gauge: %w", err)
	}
	sm.gpuPowerUsageGauge = gpuPowerUsageGauge

	// 设置CPU核心数
	sm.cpuCoresGauge.Set(float64(runtime.NumCPU()))

	return sm, nil
}

// Start 启动系统指标收集
func (sm *systemMetricsImpl) Start(ctx context.Context) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.running {
		return fmt.Errorf("system metrics already running")
	}

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(ctx)
	sm.cancel = cancel

	// 初始化CPU统计
	if err := sm.initCPUStat(); err != nil {
		return fmt.Errorf("failed to initialize CPU stats: %w", err)
	}

	sm.running = true

	// 启动指标收集协程
	go sm.collectMetrics(ctx)

	return nil
}

// Stop 停止系统指标收集
func (sm *systemMetricsImpl) Stop() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.running {
		return fmt.Errorf("system metrics not running")
	}

	if sm.cancel != nil {
		sm.cancel()
	}

	sm.running = false
	return nil
}

// GetCPUUsage 获取CPU使用率
func (sm *systemMetricsImpl) GetCPUUsage() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.cpuUsage
}

// GetMemoryUsage 获取内存使用情况
func (sm *systemMetricsImpl) GetMemoryUsage() MemoryUsage {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.memUsage
}

// GetGPUUsage 获取GPU使用情况
func (sm *systemMetricsImpl) GetGPUUsage() []GPUUsage {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// 返回副本以避免并发问题
	result := make([]GPUUsage, len(sm.gpuUsages))
	copy(result, sm.gpuUsages)
	return result
}

// IsRunning 检查是否正在运行
func (sm *systemMetricsImpl) IsRunning() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.running
}

// collectMetrics 收集指标的主循环
func (sm *systemMetricsImpl) collectMetrics(ctx context.Context) {
	ticker := time.NewTicker(sm.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sm.updateMetrics()
		}
	}
}

// updateMetrics 更新所有指标
func (sm *systemMetricsImpl) updateMetrics() {
	// 更新CPU指标
	sm.updateCPUMetrics()

	// 更新内存指标
	sm.updateMemoryMetrics()

	// 更新GPU指标
	sm.updateGPUMetrics()
}

// initCPUStat 初始化CPU统计
func (sm *systemMetricsImpl) initCPUStat() error {
	stat, err := sm.readCPUStat()
	if err != nil {
		return err
	}
	sm.lastCPUStat = stat
	return nil
}

// readCPUStat 读取CPU统计信息
func (sm *systemMetricsImpl) readCPUStat() (CPUStat, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return CPUStat{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return CPUStat{}, fmt.Errorf("failed to read /proc/stat")
	}

	line := scanner.Text()
	fields := strings.Fields(line)
	if len(fields) < 11 || fields[0] != "cpu" {
		return CPUStat{}, fmt.Errorf("invalid /proc/stat format")
	}

	var stat CPUStat
	var err2 error

	if stat.User, err2 = strconv.ParseUint(fields[1], 10, 64); err2 != nil {
		return CPUStat{}, err2
	}
	if stat.Nice, err2 = strconv.ParseUint(fields[2], 10, 64); err2 != nil {
		return CPUStat{}, err2
	}
	if stat.System, err2 = strconv.ParseUint(fields[3], 10, 64); err2 != nil {
		return CPUStat{}, err2
	}
	if stat.Idle, err2 = strconv.ParseUint(fields[4], 10, 64); err2 != nil {
		return CPUStat{}, err2
	}
	if stat.IOWait, err2 = strconv.ParseUint(fields[5], 10, 64); err2 != nil {
		return CPUStat{}, err2
	}
	if stat.IRQ, err2 = strconv.ParseUint(fields[6], 10, 64); err2 != nil {
		return CPUStat{}, err2
	}
	if stat.SoftIRQ, err2 = strconv.ParseUint(fields[7], 10, 64); err2 != nil {
		return CPUStat{}, err2
	}
	if stat.Steal, err2 = strconv.ParseUint(fields[8], 10, 64); err2 != nil {
		return CPUStat{}, err2
	}
	if len(fields) > 9 {
		if stat.Guest, err2 = strconv.ParseUint(fields[9], 10, 64); err2 != nil {
			return CPUStat{}, err2
		}
	}
	if len(fields) > 10 {
		if stat.GuestNice, err2 = strconv.ParseUint(fields[10], 10, 64); err2 != nil {
			return CPUStat{}, err2
		}
	}

	return stat, nil
}

// calculateCPUUsage 计算CPU使用率
func (sm *systemMetricsImpl) calculateCPUUsage(prev, curr CPUStat) float64 {
	prevIdle := prev.Idle + prev.IOWait
	currIdle := curr.Idle + curr.IOWait

	prevNonIdle := prev.User + prev.Nice + prev.System + prev.IRQ + prev.SoftIRQ + prev.Steal
	currNonIdle := curr.User + curr.Nice + curr.System + curr.IRQ + curr.SoftIRQ + curr.Steal

	prevTotal := prevIdle + prevNonIdle
	currTotal := currIdle + currNonIdle

	totalDiff := currTotal - prevTotal
	idleDiff := currIdle - prevIdle

	if totalDiff == 0 {
		return 0.0
	}

	return float64(totalDiff-idleDiff) / float64(totalDiff) * 100.0
}

// updateCPUMetrics 更新CPU指标
func (sm *systemMetricsImpl) updateCPUMetrics() {
	stat, err := sm.readCPUStat()
	if err != nil {
		return
	}

	usage := sm.calculateCPUUsage(sm.lastCPUStat, stat)

	sm.mu.Lock()
	sm.cpuUsage = usage
	sm.lastCPUStat = stat
	sm.mu.Unlock()

	// 更新Prometheus指标
	sm.cpuUsageGauge.Set(usage)
}

// readMemoryInfo 读取内存信息
func (sm *systemMetricsImpl) readMemoryInfo() (MemoryUsage, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return MemoryUsage{}, err
	}
	defer file.Close()

	var memTotal, memFree, memAvailable, buffers, cached uint64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		// Convert from KB to bytes
		value *= 1024

		switch key {
		case "MemTotal":
			memTotal = value
		case "MemFree":
			memFree = value
		case "MemAvailable":
			memAvailable = value
		case "Buffers":
			buffers = value
		case "Cached":
			cached = value
		}
	}

	if memAvailable == 0 {
		// If MemAvailable is not available, calculate it
		memAvailable = memFree + buffers + cached
	}

	used := memTotal - memAvailable
	percent := 0.0
	if memTotal > 0 {
		percent = float64(used) / float64(memTotal) * 100.0
	}

	return MemoryUsage{
		Total:     memTotal,
		Used:      used,
		Available: memAvailable,
		Percent:   percent,
	}, nil
}

// updateMemoryMetrics 更新内存指标
func (sm *systemMetricsImpl) updateMemoryMetrics() {
	memUsage, err := sm.readMemoryInfo()
	if err != nil {
		return
	}

	sm.mu.Lock()
	sm.memUsage = memUsage
	sm.mu.Unlock()

	// 更新Prometheus指标
	sm.memoryTotalGauge.Set(float64(memUsage.Total))
	sm.memoryUsedGauge.Set(float64(memUsage.Used))
	sm.memoryAvailableGauge.Set(float64(memUsage.Available))
	sm.memoryUsageGauge.Set(memUsage.Percent)
}

// readGPUInfo 读取GPU信息
func (sm *systemMetricsImpl) readGPUInfo() ([]GPUUsage, error) {
	var gpuUsages []GPUUsage

	// 尝试使用nvidia-smi获取NVIDIA GPU信息
	nvidiaSMI, err := exec.LookPath("nvidia-smi")
	if err == nil {
		if gpus, err := sm.readNVIDIAGPUInfo(nvidiaSMI); err == nil {
			gpuUsages = append(gpuUsages, gpus...)
		}
	}

	// 尝试使用其他方法获取AMD GPU信息
	// 这里可以添加AMD GPU的支持

	return gpuUsages, nil
}

// readNVIDIAGPUInfo 读取NVIDIA GPU信息
func (sm *systemMetricsImpl) readNVIDIAGPUInfo(nvidiaSMI string) ([]GPUUsage, error) {
	// 使用nvidia-smi查询GPU信息
	cmd := exec.Command(nvidiaSMI,
		"--query-gpu=index,name,utilization.gpu,memory.total,memory.used,memory.free,temperature.gpu,power.draw",
		"--format=csv,noheader,nounits")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var gpuUsages []GPUUsage
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Split(line, ", ")
		if len(fields) < 8 {
			continue
		}

		index, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}

		name := strings.TrimSpace(fields[1])

		utilization, err := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		if err != nil {
			utilization = 0
		}

		memTotal, err := strconv.ParseUint(strings.TrimSpace(fields[3]), 10, 64)
		if err != nil {
			memTotal = 0
		}

		memUsed, err := strconv.ParseUint(strings.TrimSpace(fields[4]), 10, 64)
		if err != nil {
			memUsed = 0
		}

		memFree, err := strconv.ParseUint(strings.TrimSpace(fields[5]), 10, 64)
		if err != nil {
			memFree = 0
		}

		temperature, err := strconv.Atoi(strings.TrimSpace(fields[6]))
		if err != nil {
			temperature = 0
		}

		powerUsage, err := strconv.ParseFloat(strings.TrimSpace(fields[7]), 64)
		if err != nil {
			powerUsage = 0
		}

		memUsagePercent := 0.0
		if memTotal > 0 {
			memUsagePercent = float64(memUsed) / float64(memTotal) * 100.0
		}

		gpu := GPUUsage{
			Index:       index,
			Name:        name,
			Utilization: utilization,
			Temperature: temperature,
			PowerUsage:  int(powerUsage),
		}
		gpu.Memory.Total = memTotal
		gpu.Memory.Used = memUsed
		gpu.Memory.Free = memFree
		gpu.Memory.Usage = memUsagePercent

		gpuUsages = append(gpuUsages, gpu)
	}

	return gpuUsages, nil
}

// updateGPUMetrics 更新GPU指标
func (sm *systemMetricsImpl) updateGPUMetrics() {
	gpuUsages, err := sm.readGPUInfo()
	if err != nil {
		return
	}

	sm.mu.Lock()
	sm.gpuUsages = gpuUsages
	sm.mu.Unlock()

	// 更新Prometheus指标
	for _, gpu := range gpuUsages {
		labels := []string{strconv.Itoa(gpu.Index), gpu.Name}

		sm.gpuUtilizationGauge.Set(gpu.Utilization, labels...)
		sm.gpuMemoryTotalGauge.Set(float64(gpu.Memory.Total), labels...)
		sm.gpuMemoryUsedGauge.Set(float64(gpu.Memory.Used), labels...)
		sm.gpuMemoryFreeGauge.Set(float64(gpu.Memory.Free), labels...)
		sm.gpuMemoryUsageGauge.Set(gpu.Memory.Usage, labels...)
		sm.gpuTemperatureGauge.Set(float64(gpu.Temperature), labels...)
		sm.gpuPowerUsageGauge.Set(float64(gpu.PowerUsage), labels...)
	}
}

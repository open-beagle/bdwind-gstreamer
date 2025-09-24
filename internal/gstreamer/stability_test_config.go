package gstreamer

import (
	"fmt"
	"time"
)

// MemoryStabilityConfig 内存稳定性测试配置
type MemoryStabilityConfig struct {
	TestDuration       time.Duration // 测试持续时间
	CheckInterval      time.Duration // 检查间隔
	AlertThresholdMB   int64         // 警告阈值 (MB)
	LeakThresholdMB    int64         // 泄漏阈值 (MB)
	GoroutineThreshold int           // 协程阈值
	AllocationRate     int           // 分配速率 (次/秒)
	AllocationSizeMB   int           // 分配大小 (MB)
	CleanupInterval    time.Duration // 清理间隔
	GCInterval         time.Duration // GC间隔
	ReportInterval     time.Duration // 报告间隔
}

// StabilityTestConfig 稳定性测试配置
type StabilityTestConfig struct {
	Duration             time.Duration // 测试持续时间
	MemoryCheckInterval  time.Duration // 内存检查间隔
	LoadTestInterval     time.Duration // 负载测试间隔
	ErrorInjectionRate   float64       // 错误注入率
	MaxMemoryGrowthMB    int64         // 最大内存增长 (MB)
	MaxGoroutines        int           // 最大协程数
	HighLoadDuration     time.Duration // 高负载持续时间
	RecoveryTestInterval time.Duration // 恢复测试间隔
}

// StabilityTestScenario 稳定性测试场景
type StabilityTestScenario struct {
	Name        string
	Description string
	Config      *StabilityTestConfig
	Enabled     bool
}

// PredefinedStabilityScenarios 预定义的稳定性测试场景
var PredefinedStabilityScenarios = map[string]*StabilityTestScenario{
	"quick": {
		Name:        "Quick Stability Test",
		Description: "快速稳定性测试，适用于CI/CD环境",
		Enabled:     true,
		Config: &StabilityTestConfig{
			Duration:             30 * time.Minute,
			MemoryCheckInterval:  1 * time.Minute,
			LoadTestInterval:     5 * time.Minute,
			ErrorInjectionRate:   0.02, // 2% 错误率
			MaxMemoryGrowthMB:    50,   // 50MB 限制
			MaxGoroutines:        500,  // 500个协程限制
			HighLoadDuration:     2 * time.Minute,
			RecoveryTestInterval: 10 * time.Minute,
		},
	},

	"standard": {
		Name:        "Standard Stability Test",
		Description: "标准稳定性测试，2小时中等强度测试",
		Enabled:     true,
		Config: &StabilityTestConfig{
			Duration:             2 * time.Hour,
			MemoryCheckInterval:  2 * time.Minute,
			LoadTestInterval:     15 * time.Minute,
			ErrorInjectionRate:   0.01, // 1% 错误率
			MaxMemoryGrowthMB:    100,  // 100MB 限制
			MaxGoroutines:        1000, // 1000个协程限制
			HighLoadDuration:     5 * time.Minute,
			RecoveryTestInterval: 30 * time.Minute,
		},
	},

	"extended": {
		Name:        "Extended Stability Test",
		Description: "扩展稳定性测试，8小时长时间运行",
		Enabled:     true,
		Config: &StabilityTestConfig{
			Duration:             8 * time.Hour,
			MemoryCheckInterval:  3 * time.Minute,
			LoadTestInterval:     20 * time.Minute,
			ErrorInjectionRate:   0.005, // 0.5% 错误率
			MaxMemoryGrowthMB:    150,   // 150MB 限制
			MaxGoroutines:        1500,  // 1500个协程限制
			HighLoadDuration:     8 * time.Minute,
			RecoveryTestInterval: 1 * time.Hour,
		},
	},

	"marathon": {
		Name:        "Marathon Stability Test",
		Description: "马拉松稳定性测试，24小时以上超长时间运行",
		Enabled:     true,
		Config: &StabilityTestConfig{
			Duration:             24 * time.Hour,
			MemoryCheckInterval:  5 * time.Minute,
			LoadTestInterval:     30 * time.Minute,
			ErrorInjectionRate:   0.01, // 1% 错误率
			MaxMemoryGrowthMB:    200,  // 200MB 限制
			MaxGoroutines:        2000, // 2000个协程限制
			HighLoadDuration:     10 * time.Minute,
			RecoveryTestInterval: 2 * time.Hour,
		},
	},

	"stress": {
		Name:        "Stress Stability Test",
		Description: "压力稳定性测试，高负载高错误率",
		Enabled:     true,
		Config: &StabilityTestConfig{
			Duration:             4 * time.Hour,
			MemoryCheckInterval:  30 * time.Second,
			LoadTestInterval:     2 * time.Minute,
			ErrorInjectionRate:   0.05, // 5% 错误率
			MaxMemoryGrowthMB:    300,  // 300MB 限制
			MaxGoroutines:        3000, // 3000个协程限制
			HighLoadDuration:     15 * time.Minute,
			RecoveryTestInterval: 15 * time.Minute,
		},
	},

	"memory_intensive": {
		Name:        "Memory Intensive Test",
		Description: "内存密集型测试，专注于内存稳定性",
		Enabled:     true,
		Config: &StabilityTestConfig{
			Duration:             6 * time.Hour,
			MemoryCheckInterval:  15 * time.Second,
			LoadTestInterval:     10 * time.Minute,
			ErrorInjectionRate:   0.02, // 2% 错误率
			MaxMemoryGrowthMB:    100,  // 严格的100MB限制
			MaxGoroutines:        1000, // 1000个协程限制
			HighLoadDuration:     5 * time.Minute,
			RecoveryTestInterval: 30 * time.Minute,
		},
	},

	"low_resource": {
		Name:        "Low Resource Test",
		Description: "低资源环境测试，模拟资源受限环境",
		Enabled:     true,
		Config: &StabilityTestConfig{
			Duration:             3 * time.Hour,
			MemoryCheckInterval:  1 * time.Minute,
			LoadTestInterval:     10 * time.Minute,
			ErrorInjectionRate:   0.01, // 1% 错误率
			MaxMemoryGrowthMB:    30,   // 严格的30MB限制
			MaxGoroutines:        200,  // 200个协程限制
			HighLoadDuration:     3 * time.Minute,
			RecoveryTestInterval: 20 * time.Minute,
		},
	},

	"high_concurrency": {
		Name:        "High Concurrency Test",
		Description: "高并发测试，测试大量并发连接",
		Enabled:     true,
		Config: &StabilityTestConfig{
			Duration:             4 * time.Hour,
			MemoryCheckInterval:  2 * time.Minute,
			LoadTestInterval:     5 * time.Minute,
			ErrorInjectionRate:   0.03, // 3% 错误率
			MaxMemoryGrowthMB:    250,  // 250MB 限制
			MaxGoroutines:        5000, // 5000个协程限制
			HighLoadDuration:     12 * time.Minute,
			RecoveryTestInterval: 25 * time.Minute,
		},
	},
}

// MemoryStabilityScenarios 内存稳定性测试场景
var MemoryStabilityScenarios = map[string]*MemoryStabilityConfig{
	"quick_memory": {
		TestDuration:       30 * time.Minute,
		CheckInterval:      15 * time.Second,
		AlertThresholdMB:   50,
		LeakThresholdMB:    100,
		GoroutineThreshold: 500,
		AllocationRate:     20, // 20次/秒
		AllocationSizeMB:   1,  // 1MB每次
		CleanupInterval:    2 * time.Minute,
		GCInterval:         30 * time.Second,
		ReportInterval:     5 * time.Minute,
	},

	"standard_memory": {
		TestDuration:       2 * time.Hour,
		CheckInterval:      30 * time.Second,
		AlertThresholdMB:   100,
		LeakThresholdMB:    200,
		GoroutineThreshold: 1000,
		AllocationRate:     50, // 50次/秒
		AllocationSizeMB:   1,  // 1MB每次
		CleanupInterval:    5 * time.Minute,
		GCInterval:         1 * time.Minute,
		ReportInterval:     10 * time.Minute,
	},

	"intensive_memory": {
		TestDuration:       6 * time.Hour,
		CheckInterval:      15 * time.Second,
		AlertThresholdMB:   150,
		LeakThresholdMB:    300,
		GoroutineThreshold: 1500,
		AllocationRate:     100, // 100次/秒
		AllocationSizeMB:   2,   // 2MB每次
		CleanupInterval:    3 * time.Minute,
		GCInterval:         45 * time.Second,
		ReportInterval:     8 * time.Minute,
	},

	"marathon_memory": {
		TestDuration:       24 * time.Hour,
		CheckInterval:      30 * time.Second,
		AlertThresholdMB:   200,
		LeakThresholdMB:    500,
		GoroutineThreshold: 2000,
		AllocationRate:     75, // 75次/秒
		AllocationSizeMB:   1,  // 1MB每次
		CleanupInterval:    5 * time.Minute,
		GCInterval:         1 * time.Minute,
		ReportInterval:     15 * time.Minute,
	},

	"stress_memory": {
		TestDuration:       4 * time.Hour,
		CheckInterval:      10 * time.Second,
		AlertThresholdMB:   300,
		LeakThresholdMB:    600,
		GoroutineThreshold: 3000,
		AllocationRate:     200, // 200次/秒，高压力
		AllocationSizeMB:   3,   // 3MB每次
		CleanupInterval:    2 * time.Minute,
		GCInterval:         30 * time.Second,
		ReportInterval:     5 * time.Minute,
	},
}

// GetStabilityScenario 获取稳定性测试场景
func GetStabilityScenario(name string) (*StabilityTestScenario, bool) {
	scenario, exists := PredefinedStabilityScenarios[name]
	return scenario, exists
}

// GetMemoryStabilityConfig 获取内存稳定性测试配置
func GetMemoryStabilityConfig(name string) (*MemoryStabilityConfig, bool) {
	config, exists := MemoryStabilityScenarios[name]
	return config, exists
}

// ListAvailableScenarios 列出可用的测试场景
func ListAvailableScenarios() []string {
	scenarios := make([]string, 0, len(PredefinedStabilityScenarios))
	for name := range PredefinedStabilityScenarios {
		scenarios = append(scenarios, name)
	}
	return scenarios
}

// ListAvailableMemoryScenarios 列出可用的内存测试场景
func ListAvailableMemoryScenarios() []string {
	scenarios := make([]string, 0, len(MemoryStabilityScenarios))
	for name := range MemoryStabilityScenarios {
		scenarios = append(scenarios, name)
	}
	return scenarios
}

// ValidateScenarioConfig 验证场景配置
func ValidateScenarioConfig(config *StabilityTestConfig) error {
	if config.Duration <= 0 {
		return fmt.Errorf("test duration must be positive")
	}

	if config.MemoryCheckInterval <= 0 {
		return fmt.Errorf("memory check interval must be positive")
	}

	if config.LoadTestInterval <= 0 {
		return fmt.Errorf("load test interval must be positive")
	}

	if config.ErrorInjectionRate < 0 || config.ErrorInjectionRate > 1 {
		return fmt.Errorf("error injection rate must be between 0 and 1")
	}

	if config.MaxMemoryGrowthMB <= 0 {
		return fmt.Errorf("max memory growth must be positive")
	}

	if config.MaxGoroutines <= 0 {
		return fmt.Errorf("max goroutines must be positive")
	}

	return nil
}

// ValidateMemoryConfig 验证内存配置
func ValidateMemoryConfig(config *MemoryStabilityConfig) error {
	if config.TestDuration <= 0 {
		return fmt.Errorf("test duration must be positive")
	}

	if config.CheckInterval <= 0 {
		return fmt.Errorf("check interval must be positive")
	}

	if config.AlertThresholdMB <= 0 {
		return fmt.Errorf("alert threshold must be positive")
	}

	if config.LeakThresholdMB <= 0 {
		return fmt.Errorf("leak threshold must be positive")
	}

	if config.AllocationRate <= 0 {
		return fmt.Errorf("allocation rate must be positive")
	}

	if config.AllocationSizeMB <= 0 {
		return fmt.Errorf("allocation size must be positive")
	}

	return nil
}

// CreateCustomScenario 创建自定义场景
func CreateCustomScenario(name, description string, config *StabilityTestConfig) (*StabilityTestScenario, error) {
	if err := ValidateScenarioConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %v", err)
	}

	return &StabilityTestScenario{
		Name:        name,
		Description: description,
		Config:      config,
		Enabled:     true,
	}, nil
}

// CreateCustomMemoryConfig 创建自定义内存配置
func CreateCustomMemoryConfig(config *MemoryStabilityConfig) (*MemoryStabilityConfig, error) {
	if err := ValidateMemoryConfig(config); err != nil {
		return nil, fmt.Errorf("invalid memory config: %v", err)
	}

	return config, nil
}

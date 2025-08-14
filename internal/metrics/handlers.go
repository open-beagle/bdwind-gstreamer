package metrics

import (
	"encoding/json"
	"net/http"
	"time"
)

// SystemHandler 系统监控处理器
// 处理系统监控相关的HTTP请求
type SystemHandler struct {
	systemMetrics SystemMetrics
}

// NewSystemHandler 创建系统监控处理器
func NewSystemHandler(systemMetrics SystemMetrics) *SystemHandler {
	return &SystemHandler{
		systemMetrics: systemMetrics,
	}
}

// handleStats 处理系统统计请求
// GET /api/system/stats
func (h *SystemHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	var stats interface{}

	if h.systemMetrics != nil && h.systemMetrics.IsRunning() {
		stats = map[string]interface{}{
			"cpu_usage":    h.systemMetrics.GetCPUUsage(),
			"memory_usage": h.systemMetrics.GetMemoryUsage(),
			"gpu_usage":    h.systemMetrics.GetGPUUsage(),
			"timestamp":    time.Now().Unix(),
		}
	} else {
		stats = map[string]interface{}{
			"error":     "metrics not available",
			"message":   "System metrics are not initialized or not running",
			"timestamp": time.Now().Unix(),
		}
	}

	h.writeJSON(w, stats)
}

// handleLogs 处理系统日志请求
// GET /api/system/logs
func (h *SystemHandler) handleLogs(w http.ResponseWriter, r *http.Request) {
	// 这里返回模拟的日志数据
	// 在实际实现中，可以从日志文件或日志系统中读取
	logs := []map[string]interface{}{
		{
			"timestamp": time.Now().Unix(),
			"level":     "info",
			"message":   "Metrics manager started successfully",
			"component": "metrics",
		},
		{
			"timestamp": time.Now().Unix() - 60,
			"level":     "debug",
			"message":   "System metrics collection started",
			"component": "system_metrics",
		},
		{
			"timestamp": time.Now().Unix() - 120,
			"level":     "info",
			"message":   "Prometheus metrics server started",
			"component": "prometheus",
		},
	}

	h.writeJSON(w, logs)
}

// handleProcesses 处理系统进程请求
// GET /api/system/processes
func (h *SystemHandler) handleProcesses(w http.ResponseWriter, r *http.Request) {
	// 这里返回模拟的进程数据
	// 在实际实现中，可以从 /proc 文件系统读取进程信息
	processes := []map[string]interface{}{
		{
			"pid":       12345,
			"name":      "bdwind-gstreamer",
			"cpu":       15.5,
			"memory":    256,
			"status":    "running",
			"command":   "./bdwind-gstreamer",
			"user":      "root",
			"timestamp": time.Now().Unix(),
		},
		{
			"pid":       12346,
			"name":      "gstreamer-pipeline",
			"cpu":       25.2,
			"memory":    512,
			"status":    "running",
			"command":   "gst-launch-1.0",
			"user":      "root",
			"timestamp": time.Now().Unix(),
		},
	}

	h.writeJSON(w, processes)
}

// handleResources 处理系统资源请求
// GET /api/system/resources
func (h *SystemHandler) handleResources(w http.ResponseWriter, r *http.Request) {
	var resources map[string]interface{}

	if h.systemMetrics != nil && h.systemMetrics.IsRunning() {
		cpuUsage := h.systemMetrics.GetCPUUsage()
		memUsage := h.systemMetrics.GetMemoryUsage()
		gpuUsages := h.systemMetrics.GetGPUUsage()

		// 构建GPU信息
		var gpuInfo []map[string]interface{}
		for _, gpu := range gpuUsages {
			gpuInfo = append(gpuInfo, map[string]interface{}{
				"index":        gpu.Index,
				"name":         gpu.Name,
				"utilization":  gpu.Utilization,
				"memory_total": gpu.Memory.Total,
				"memory_used":  gpu.Memory.Used,
				"memory_free":  gpu.Memory.Free,
				"memory_usage": gpu.Memory.Usage,
				"temperature":  gpu.Temperature,
				"power_usage":  gpu.PowerUsage,
			})
		}

		resources = map[string]interface{}{
			"cpu": map[string]interface{}{
				"usage":     cpuUsage,
				"cores":     8,                         // 这里可以从 runtime.NumCPU() 获取
				"load":      [3]float64{1.2, 1.5, 1.8}, // 模拟负载平均值
				"timestamp": time.Now().Unix(),
			},
			"memory": map[string]interface{}{
				"total":     memUsage.Total,
				"used":      memUsage.Used,
				"available": memUsage.Available,
				"percent":   memUsage.Percent,
				"timestamp": time.Now().Unix(),
			},
			"gpu": gpuInfo,
			"disk": map[string]interface{}{
				"total":     1000000000, // 模拟磁盘信息
				"used":      500000000,
				"available": 500000000,
				"percent":   50.0,
				"timestamp": time.Now().Unix(),
			},
			"timestamp": time.Now().Unix(),
		}
	} else {
		resources = map[string]interface{}{
			"error":     "metrics not available",
			"message":   "System metrics are not initialized or not running",
			"timestamp": time.Now().Unix(),
		}
	}

	h.writeJSON(w, resources)
}

// handleInfo 处理系统信息请求
// GET /api/system/info
func (h *SystemHandler) handleInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"system":     "linux",
		"arch":       "amd64",
		"version":    "1.0.0",
		"go_version": "go1.21",
		"build_time": "2024-01-01T00:00:00Z",
		"git_commit": "abc123def456",
		"uptime":     time.Now().Unix() - 1640995200, // 模拟启动时间
		"timestamp":  time.Now().Unix(),
	}

	h.writeJSON(w, info)
}

// writeJSON 写入JSON响应
func (h *SystemHandler) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError)
		return
	}
}

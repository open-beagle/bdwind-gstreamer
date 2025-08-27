package gstreamer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// gstreamerHandlers GStreamer组件的HTTP处理器
type gstreamerHandlers struct {
	manager *Manager
	logger  *logrus.Entry
}

// newGStreamerHandlers 创建GStreamer处理器实例
func newGStreamerHandlers(manager *Manager) *gstreamerHandlers {
	return &gstreamerHandlers{
		manager: manager,
		logger:  config.GetLoggerWithPrefix("gstreamer-handlers"),
	}
}

// setupGStreamerRoutes 设置GStreamer相关的所有路由
func (h *gstreamerHandlers) setupGStreamerRoutes(router *mux.Router) error {
	// 桌面捕获API路由
	captureAPI := router.PathPrefix("/api/capture").Subrouter()
	captureAPI.HandleFunc("/start", h.handleCaptureStart).Methods("POST")
	captureAPI.HandleFunc("/stop", h.handleCaptureStop).Methods("POST")
	captureAPI.HandleFunc("/restart", h.handleCaptureRestart).Methods("POST")
	captureAPI.HandleFunc("/status", h.handleCaptureStatus).Methods("GET")
	captureAPI.HandleFunc("/stats", h.handleCaptureStats).Methods("GET")
	captureAPI.HandleFunc("/config", h.handleCaptureConfig).Methods("GET", "POST", "PUT")

	// 显示器API路由
	displayAPI := router.PathPrefix("/api/display").Subrouter()
	displayAPI.HandleFunc("/displays", h.handleDisplays).Methods("GET")
	displayAPI.HandleFunc("/displays/{id}", h.handleDisplayInfo).Methods("GET")
	displayAPI.HandleFunc("/displays/{id}/screenshot", h.handleDisplayScreenshot).Methods("GET")

	return nil
}

// 桌面捕获相关处理器

// handleCaptureStart 处理开始捕获请求
func (h *gstreamerHandlers) handleCaptureStart(w http.ResponseWriter, r *http.Request) {
	if !h.manager.IsRunning() {
		http.Error(w, "GStreamer manager not running", http.StatusServiceUnavailable)
		return
	}

	capture := h.manager.GetCapture()
	if capture == nil {
		h.writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": "Desktop capture not initialized",
		})
		return
	}

	if err := capture.Start(); err != nil {
		h.writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("Failed to start capture: %v", err),
		})
		return
	}

	h.writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Desktop capture started successfully",
	})
}

// handleCaptureStop 处理停止捕获请求
func (h *gstreamerHandlers) handleCaptureStop(w http.ResponseWriter, r *http.Request) {
	if !h.manager.IsRunning() {
		http.Error(w, "GStreamer manager not running", http.StatusServiceUnavailable)
		return
	}

	capture := h.manager.GetCapture()
	if capture == nil {
		h.writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": "Desktop capture not initialized",
		})
		return
	}

	if err := capture.Stop(); err != nil {
		h.writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("Failed to stop capture: %v", err),
		})
		return
	}

	h.writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Desktop capture stopped successfully",
	})
}

// handleCaptureRestart 处理重启捕获请求
func (h *gstreamerHandlers) handleCaptureRestart(w http.ResponseWriter, r *http.Request) {
	if !h.manager.IsRunning() {
		http.Error(w, "GStreamer manager not running", http.StatusServiceUnavailable)
		return
	}

	capture := h.manager.GetCapture()
	if capture == nil {
		h.writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": "Desktop capture not initialized",
		})
		return
	}

	// 停止捕获
	if err := capture.Stop(); err != nil {
		// 记录错误但继续重启
		h.logger.Infof("Warning: failed to stop capture during restart: %v", err)
	}

	// 重新启动捕获
	if err := capture.Start(); err != nil {
		h.writeJSON(w, map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("Failed to restart capture: %v", err),
		})
		return
	}

	h.writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Desktop capture restarted successfully",
	})
}

// handleCaptureStatus 处理捕获状态请求
func (h *gstreamerHandlers) handleCaptureStatus(w http.ResponseWriter, r *http.Request) {
	capture := h.manager.GetCapture()

	status := map[string]interface{}{
		"manager_running": h.manager.IsRunning(),
		"initialized":     capture != nil,
		"running":         false,
	}

	if capture != nil {
		// TODO: 实现实际的状态检查
		// 这里应该检查capture的实际运行状态
		status["running"] = true
	}

	h.writeJSON(w, status)
}

// handleCaptureStats 处理捕获统计请求
func (h *gstreamerHandlers) handleCaptureStats(w http.ResponseWriter, r *http.Request) {
	if !h.manager.IsRunning() {
		http.Error(w, "GStreamer manager not running", http.StatusServiceUnavailable)
		return
	}

	// 获取管理器统计信息
	managerStats := h.manager.GetStats()

	// 构造捕获统计信息
	stats := map[string]interface{}{
		"frames_captured": 0,
		"frames_dropped":  0,
		"current_fps":     0.0,
		"average_fps":     0.0,
		"timestamp":       time.Now().Unix(),
		"manager_stats":   managerStats,
	}

	capture := h.manager.GetCapture()
	if capture != nil {
		// TODO: 实现实际的统计获取
		// 这里应该从capture实例获取真实的统计数据
		stats["frames_captured"] = 1000
		stats["current_fps"] = 30.0
		stats["average_fps"] = 29.5
	}

	h.writeJSON(w, stats)
}

// handleCaptureConfig 处理捕获配置请求
func (h *gstreamerHandlers) handleCaptureConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		config := h.manager.GetConfig()
		h.writeJSON(w, config)

	case "POST", "PUT":
		var newConfig config.DesktopCaptureConfig
		if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// TODO: 验证配置并应用更改
		// 这里应该验证新配置的有效性，并可能需要重启捕获

		h.writeJSON(w, map[string]interface{}{
			"status":  "updated",
			"message": "Capture configuration updated successfully",
			"config":  newConfig,
		})
	}
}

// 显示器相关处理器

// handleDisplays 处理显示器列表请求
func (h *gstreamerHandlers) handleDisplays(w http.ResponseWriter, r *http.Request) {
	// TODO: 实现实际的显示器检测
	// 这里应该调用系统API来获取可用的显示器列表
	displays := []map[string]interface{}{
		{
			"id":           ":0",
			"name":         "Primary Display",
			"width":        1920,
			"height":       1080,
			"refresh_rate": 60.0,
			"type":         "X11",
			"active":       true,
		},
	}

	h.writeJSON(w, displays)
}

// handleDisplayInfo 处理单个显示器信息请求
func (h *gstreamerHandlers) handleDisplayInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	displayID := vars["id"]

	// TODO: 实现实际的显示器信息获取
	// 这里应该根据displayID获取具体的显示器信息
	info := map[string]interface{}{
		"id":           displayID,
		"name":         "Primary Display",
		"width":        1920,
		"height":       1080,
		"refresh_rate": 60.0,
		"type":         "X11",
		"active":       true,
		"available":    true,
	}

	h.writeJSON(w, info)
}

// handleDisplayScreenshot 处理显示器截图请求
func (h *gstreamerHandlers) handleDisplayScreenshot(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	displayID := vars["id"]

	// TODO: 实现实际的截图功能
	// 这里应该使用GStreamer或其他方式来捕获显示器截图
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("Screenshot not implemented for display: " + displayID))
}

// writeJSON 写入JSON响应
func (h *gstreamerHandlers) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Infof("Failed to encode JSON: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

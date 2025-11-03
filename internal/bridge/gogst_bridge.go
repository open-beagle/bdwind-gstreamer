package bridge

import (
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"
	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc"
)

// GoGstMediaBridge go-gst 和 WebRTC 之间的媒体桥接器
type GoGstMediaBridge struct {
	config    *config.Config
	logger    *logrus.Entry
	gstreamer *gstreamer.MinimalGoGstManager
	webrtc    *webrtc.MinimalWebRTCManager

	// 状态管理
	running   bool
	mutex     sync.RWMutex
	startTime time.Time

	// 统计信息
	frameCount    uint64
	bytesReceived uint64
	lastFrameTime time.Time
}

// NewGoGstMediaBridge 创建 go-gst 媒体桥接器
func NewGoGstMediaBridge(cfg *config.Config, webrtcMgr *webrtc.MinimalWebRTCManager) (*GoGstMediaBridge, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if webrtcMgr == nil {
		return nil, fmt.Errorf("webrtc manager cannot be nil")
	}

	logger := logrus.WithField("component", "gogst-bridge")

	// 创建 GStreamer 管理器
	gstreamerMgr, err := gstreamer.NewMinimalGoGstManager(cfg.GStreamer)
	if err != nil {
		return nil, fmt.Errorf("failed to create GStreamer manager: %w", err)
	}

	bridge := &GoGstMediaBridge{
		config:    cfg,
		logger:    logger,
		gstreamer: gstreamerMgr,
		webrtc:    webrtcMgr,
	}

	logger.Info("GoGst media bridge created successfully")
	return bridge, nil
}

// Start 启动媒体桥接器
func (b *GoGstMediaBridge) Start() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.running {
		return fmt.Errorf("media bridge already running")
	}

	b.logger.Info("Starting go-gst media bridge...")

	// 设置视频数据回调
	b.gstreamer.SetVideoCallback(b.onVideoFrame)

	// 启动 GStreamer
	if err := b.gstreamer.Start(); err != nil {
		return fmt.Errorf("failed to start GStreamer: %w", err)
	}

	// 启动 WebRTC
	if err := b.webrtc.Start(); err != nil {
		b.gstreamer.Stop() // 回滚
		return fmt.Errorf("failed to start WebRTC: %w", err)
	}

	b.running = true
	b.startTime = time.Now()
	b.logger.Info("GoGst media bridge started successfully")

	return nil
}

// onVideoFrame 处理来自 GStreamer 的视频帧数据
func (b *GoGstMediaBridge) onVideoFrame(data []byte, timestamp time.Duration) error {
	b.logger.Debug("Processing video frame from GStreamer")

	// 更新统计信息
	b.frameCount++
	b.bytesReceived += uint64(len(data))
	b.lastFrameTime = time.Now()

	// 处理 H.264 数据
	processedData, err := b.processH264Data(data)
	if err != nil {
		b.logger.Errorf("Failed to process H.264 data: %v", err)
		return err
	}

	// 发送到 WebRTC
	if err := b.webrtc.SendVideoDataWithTimestamp(processedData, timestamp); err != nil {
		b.logger.Errorf("Failed to send video data to WebRTC: %v", err)
		return err
	}

	return nil
}

// processH264Data 处理 H.264 数据格式
func (b *GoGstMediaBridge) processH264Data(data []byte) ([]byte, error) {
	// 验证数据格式
	if len(data) < 4 {
		return nil, fmt.Errorf("invalid H.264 data: too short")
	}

	// 检查 NAL 单元起始码
	if data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x00 && data[3] == 0x01 {
		// 数据已经是正确的格式
		return data, nil
	}

	// 如果需要，可以在这里添加格式转换逻辑
	// 目前直接返回原始数据
	return data, nil
}

// Stop 停止媒体桥接器
func (b *GoGstMediaBridge) Stop() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if !b.running {
		return nil
	}

	b.logger.Info("Stopping go-gst media bridge...")

	// 停止 WebRTC
	if err := b.webrtc.Stop(); err != nil {
		b.logger.Errorf("Failed to stop WebRTC: %v", err)
	}

	// 停止 GStreamer
	if err := b.gstreamer.Stop(); err != nil {
		b.logger.Errorf("Failed to stop GStreamer: %v", err)
	}

	b.running = false
	b.logger.Info("GoGst media bridge stopped successfully")

	return nil
}

// Restart 重启媒体桥接器
func (b *GoGstMediaBridge) Restart() error {
	b.logger.Info("Restarting go-gst media bridge...")

	if err := b.Stop(); err != nil {
		return fmt.Errorf("failed to stop bridge: %w", err)
	}

	// 等待一小段时间确保资源清理完成
	time.Sleep(100 * time.Millisecond)

	if err := b.Start(); err != nil {
		return fmt.Errorf("failed to start bridge: %w", err)
	}

	b.logger.Info("GoGst media bridge restarted successfully")
	return nil
}

// IsRunning 检查桥接器是否正在运行
func (b *GoGstMediaBridge) IsRunning() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.running
}

// GetStats 获取桥接器统计信息
func (b *GoGstMediaBridge) GetStats() map[string]interface{} {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	stats := map[string]interface{}{
		"running":           b.running,
		"type":              "go-gst",
		"gstreamer_running": b.gstreamer.IsRunning(),
		"webrtc_running":    b.webrtc.IsRunning(),
		"frame_count":       b.frameCount,
		"bytes_received":    b.bytesReceived,
	}

	if b.running {
		stats["start_time"] = b.startTime
		stats["uptime"] = time.Since(b.startTime).Seconds()
		stats["last_frame_time"] = b.lastFrameTime

		if !b.lastFrameTime.IsZero() {
			stats["seconds_since_last_frame"] = time.Since(b.lastFrameTime).Seconds()
		}

		if b.frameCount > 0 && !b.startTime.IsZero() {
			fps := float64(b.frameCount) / time.Since(b.startTime).Seconds()
			stats["average_fps"] = fps
		}
	}

	return stats
}

// UpdateBitrate 动态更新比特率
func (b *GoGstMediaBridge) UpdateBitrate(bitrate int) error {
	return b.gstreamer.UpdateBitrate(bitrate)
}

// GetGStreamerManager 获取 GStreamer 管理器
func (b *GoGstMediaBridge) GetGStreamerManager() *gstreamer.MinimalGoGstManager {
	return b.gstreamer
}

// GetWebRTCManager 获取 WebRTC 管理器
func (b *GoGstMediaBridge) GetWebRTCManager() *webrtc.MinimalWebRTCManager {
	return b.webrtc
}

package factory

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/bridge"
	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc"
)

// CreateMediaBridge 直接创建 go-gst 媒体桥接器
func CreateMediaBridge(cfg *config.Config, webrtcMgr *webrtc.WebRTCManager) (*bridge.GoGstMediaBridge, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if cfg.GStreamer == nil {
		return nil, fmt.Errorf("GStreamer config cannot be nil")
	}

	logger := logrus.WithField("component", "factory")
	logger.Info("Creating go-gst media bridge...")

	mediaBridge, err := bridge.NewGoGstMediaBridge(cfg, webrtcMgr)
	if err != nil {
		return nil, fmt.Errorf("failed to create go-gst bridge: %w", err)
	}

	logger.Info("Go-gst media bridge created successfully")
	return mediaBridge, nil
}

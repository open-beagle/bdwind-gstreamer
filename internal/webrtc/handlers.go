package webrtc

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// webrtcHandlers WebRTC组件的HTTP处理器
type webrtcHandlers struct {
	manager *Manager
}

// newWebRTCHandlers 创建WebRTC处理器实例
func newWebRTCHandlers(manager *Manager) *webrtcHandlers {
	return &webrtcHandlers{
		manager: manager,
	}
}

// setupWebRTCRoutes 设置WebRTC相关的所有路由
func (h *webrtcHandlers) setupWebRTCRoutes(router *mux.Router) error {
	// WebRTC API路由
	webrtcAPI := router.PathPrefix("/api/webrtc").Subrouter()
	webrtcAPI.HandleFunc("/peers", h.handlePeers).Methods("GET")
	webrtcAPI.HandleFunc("/peers/{id}", h.handlePeerInfo).Methods("GET")
	webrtcAPI.HandleFunc("/peers/{id}/disconnect", h.handlePeerDisconnect).Methods("POST")
	webrtcAPI.HandleFunc("/stats", h.handleStats).Methods("GET")
	webrtcAPI.HandleFunc("/config", h.handleConfig).Methods("GET", "POST")

	// WebSocket信令路由
	router.HandleFunc("/ws", h.handleWebSocket).Methods("GET")
	router.HandleFunc("/signaling/{app}", h.handleSignaling).Methods("GET")

	return nil
}

// WebSocket处理器
func (h *webrtcHandlers) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // 允许跨域，实际应用中应该更严格
		},
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
		HandshakeTimeout: 10 * time.Second,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	log.Printf("WebSocket connection established from %s", r.RemoteAddr)

	// 不要在这里关闭连接，让信令服务器管理连接生命周期
	// 处理WebSocket连接
	h.manager.signaling.HandleWebSocketConnection(conn)
}

func (h *webrtcHandlers) handleSignaling(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	appName := vars["app"]

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // 允许跨域，实际应用中应该更严格
		},
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
		HandshakeTimeout: 10 * time.Second,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed for app %s: %v", appName, err)
		return
	}

	log.Printf("WebSocket connection established for app %s from %s", appName, r.RemoteAddr)

	// 不要在这里关闭连接，让信令服务器管理连接生命周期
	// 处理特定应用的信令连接
	h.manager.signaling.HandleAppConnection(conn, appName)
}

// REST API处理器

// handlePeers 处理WebRTC对等连接列表请求
func (h *webrtcHandlers) handlePeers(w http.ResponseWriter, r *http.Request) {
	if !h.manager.IsRunning() {
		http.Error(w, "WebRTC manager not running", http.StatusServiceUnavailable)
		return
	}

	// 从信令服务器获取实际的客户端信息
	clientCount := h.manager.signaling.GetClientCount()

	// 构造对等连接列表（这里是示例数据，实际应该从信令服务器获取）
	peers := []map[string]interface{}{
		{
			"id":           "peer-1",
			"state":        "connected",
			"created_at":   time.Now().Unix() - 300,
			"remote_ip":    "192.168.1.100",
			"client_count": clientCount,
		},
	}

	h.writeJSON(w, peers)
}

// handlePeerInfo 处理单个WebRTC对等连接信息请求
func (h *webrtcHandlers) handlePeerInfo(w http.ResponseWriter, r *http.Request) {
	if !h.manager.IsRunning() {
		http.Error(w, "WebRTC manager not running", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	peerID := vars["id"]

	// 获取媒体流统计信息
	mediaStats := h.manager.mediaStream.GetStats()
	bridgeStats := h.manager.bridge.GetStats()

	info := map[string]interface{}{
		"id":         peerID,
		"state":      "connected",
		"created_at": time.Now().Unix() - 300,
		"remote_ip":  "192.168.1.100",
		"stats": map[string]interface{}{
			"video_frames_sent":    mediaStats.VideoFramesSent,
			"video_bytes_sent":     mediaStats.VideoBytesSent,
			"audio_frames_sent":    mediaStats.AudioFramesSent,
			"audio_bytes_sent":     mediaStats.AudioBytesSent,
			"bridge_video_samples": bridgeStats.VideoSamplesProcessed,
			"bridge_audio_samples": bridgeStats.AudioSamplesProcessed,
		},
	}

	h.writeJSON(w, info)
}

// handlePeerDisconnect 处理WebRTC对等连接断开请求
func (h *webrtcHandlers) handlePeerDisconnect(w http.ResponseWriter, r *http.Request) {
	if !h.manager.IsRunning() {
		http.Error(w, "WebRTC manager not running", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	peerID := vars["id"]

	// TODO: 实现实际的断开逻辑
	// 这里应该调用信令服务器的方法来断开特定的客户端连接

	h.writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Peer %s disconnected", peerID),
	})
}

// handleStats 处理WebRTC统计请求
func (h *webrtcHandlers) handleStats(w http.ResponseWriter, r *http.Request) {
	if !h.manager.IsRunning() {
		http.Error(w, "WebRTC manager not running", http.StatusServiceUnavailable)
		return
	}

	// 获取WebRTC管理器的完整统计信息
	stats := h.manager.GetStats()

	// 添加时间戳
	stats["timestamp"] = time.Now().Unix()

	h.writeJSON(w, stats)
}

// handleConfig 处理WebRTC配置请求
func (h *webrtcHandlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		if !h.manager.IsRunning() {
			http.Error(w, "WebRTC manager not running", http.StatusServiceUnavailable)
			return
		}

		// 转换ICE服务器配置为前端格式
		iceServers := make([]map[string]interface{}, 0)
		for _, server := range h.manager.config.WebRTC.ICEServers {
			iceServer := map[string]interface{}{
				"urls": server.URLs,
			}
			if server.Username != "" {
				iceServer["username"] = server.Username
			}
			if server.Credential != "" {
				iceServer["credential"] = server.Credential
			}
			iceServers = append(iceServers, iceServer)
		}

		config := map[string]interface{}{
			"video_enabled": true,
			"audio_enabled": false,
			"video_codec":   h.manager.config.GStreamer.Encoding.Codec,
			"audio_codec":   "opus",
			"video_width":   h.manager.config.GStreamer.Capture.Width,
			"video_height":  h.manager.config.GStreamer.Capture.Height,
			"video_fps":     h.manager.config.GStreamer.Capture.FrameRate,
			"ice_servers":   iceServers,
		}
		h.writeJSON(w, config)

	case "POST":
		var newConfig map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// TODO: 实现配置更新逻辑
		// 这里应该验证配置并更新WebRTC管理器的设置

		h.writeJSON(w, map[string]interface{}{
			"status":  "updated",
			"message": "WebRTC configuration updated successfully",
			"config":  newConfig,
		})
	}
}

// writeJSON 写入JSON响应
func (h *webrtcHandlers) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

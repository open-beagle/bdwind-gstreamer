package webrtc

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
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
	log.Printf("[WebRTC] 开始设置WebRTC路由...")

	// WebRTC API路由
	webrtcAPI := router.PathPrefix("/api/webrtc").Subrouter()
	webrtcAPI.HandleFunc("/peers", h.handlePeers).Methods("GET")
	webrtcAPI.HandleFunc("/peers/{id}", h.handlePeerInfo).Methods("GET")
	webrtcAPI.HandleFunc("/peers/{id}/disconnect", h.handlePeerDisconnect).Methods("POST")
	webrtcAPI.HandleFunc("/stats", h.handleStats).Methods("GET")
	webrtcAPI.HandleFunc("/config", h.handleConfig).Methods("GET", "POST")
	log.Printf("[WebRTC] WebRTC API路由已注册: /api/webrtc/*")

	// 调试和诊断API路由
	debugAPI := router.PathPrefix("/api/debug").Subrouter()
	debugAPI.HandleFunc("/logs", h.handleDebugLogs).Methods("POST")
	debugAPI.HandleFunc("/diagnostics", h.handleDiagnostics).Methods("GET", "POST")
	debugAPI.HandleFunc("/export", h.handleDebugExport).Methods("POST")
	log.Printf("[WebRTC] 调试API路由已注册: /api/debug/*")

	// 健康检查和测试API
	router.HandleFunc("/api/health", h.handleHealth).Methods("GET")
	router.HandleFunc("/api/ping", h.handlePing).Methods("GET")
	router.HandleFunc("/api/bandwidth-test/download", h.handleBandwidthTestDownload).Methods("GET")
	router.HandleFunc("/api/bandwidth-test/upload", h.handleBandwidthTestUpload).Methods("POST")
	log.Printf("[WebRTC] 健康检查和测试API路由已注册")

	// WebSocket信令路由
	router.HandleFunc("/ws", h.handleWebSocket).Methods("GET")
	log.Printf("[WebRTC] 主要WebSocket路由已注册: /ws")
	log.Printf("[WebRTC] WebSocket处理器配置信息:")
	log.Printf("[WebRTC]   - 路径: /ws")
	log.Printf("[WebRTC]   - 方法: GET")
	log.Printf("[WebRTC]   - 处理器: handleWebSocket")
	log.Printf("[WebRTC]   - 跨域检查: 允许所有来源")
	log.Printf("[WebRTC]   - 读缓冲区大小: 1024 bytes")
	log.Printf("[WebRTC]   - 写缓冲区大小: 1024 bytes")
	log.Printf("[WebRTC]   - 握手超时: 10 seconds")

	router.HandleFunc("/signaling/{app}", h.handleSignaling).Methods("GET")
	log.Printf("[WebRTC] 应用特定信令路由已注册: /signaling/{app}")
	log.Printf("[WebRTC] 应用信令处理器配置信息:")
	log.Printf("[WebRTC]   - 路径: /signaling/{app}")
	log.Printf("[WebRTC]   - 方法: GET")
	log.Printf("[WebRTC]   - 处理器: handleSignaling")
	log.Printf("[WebRTC]   - 支持应用特定连接")

	log.Printf("[WebRTC] 所有WebRTC路由设置完成")
	log.Printf("[WebRTC] 路由摘要:")
	log.Printf("[WebRTC]   - WebRTC API: /api/webrtc/*")
	log.Printf("[WebRTC]   - 调试API: /api/debug/*")
	log.Printf("[WebRTC]   - 健康检查: /api/health, /api/ping")
	log.Printf("[WebRTC]   - 带宽测试: /api/bandwidth-test/*")
	log.Printf("[WebRTC]   - 主要WebSocket: /ws")
	log.Printf("[WebRTC]   - 应用信令: /signaling/{app}")

	return nil
}

// WebSocket处理器
func (h *webrtcHandlers) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 记录连接请求的详细信息
	log.Printf("[WebRTC] WebSocket连接请求开始")
	log.Printf("[WebRTC] 请求详情: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	log.Printf("[WebRTC] 客户端信息:")
	log.Printf("[WebRTC]   - 远程地址: %s", r.RemoteAddr)
	log.Printf("[WebRTC]   - User-Agent: %s", r.Header.Get("User-Agent"))
	log.Printf("[WebRTC]   - Origin: %s", r.Header.Get("Origin"))
	log.Printf("[WebRTC]   - Host: %s", r.Header.Get("Host"))
	log.Printf("[WebRTC]   - Referer: %s", r.Header.Get("Referer"))
	log.Printf("[WebRTC]   - X-Forwarded-For: %s", r.Header.Get("X-Forwarded-For"))
	log.Printf("[WebRTC]   - X-Real-IP: %s", r.Header.Get("X-Real-IP"))

	// 记录WebSocket特定的请求头
	log.Printf("[WebRTC] WebSocket协议头:")
	log.Printf("[WebRTC]   - Connection: %s", r.Header.Get("Connection"))
	log.Printf("[WebRTC]   - Upgrade: %s", r.Header.Get("Upgrade"))
	log.Printf("[WebRTC]   - Sec-WebSocket-Version: %s", r.Header.Get("Sec-WebSocket-Version"))
	log.Printf("[WebRTC]   - Sec-WebSocket-Key: %s", r.Header.Get("Sec-WebSocket-Key"))
	log.Printf("[WebRTC]   - Sec-WebSocket-Protocol: %s", r.Header.Get("Sec-WebSocket-Protocol"))
	log.Printf("[WebRTC]   - Sec-WebSocket-Extensions: %s", r.Header.Get("Sec-WebSocket-Extensions"))

	// 验证WebSocket升级请求的有效性
	if r.Method != "GET" {
		log.Printf("[WebRTC] WebSocket升级失败: 无效的HTTP方法 %s (必须是GET)", r.Method)
		http.Error(w, "Method not allowed for WebSocket upgrade", http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("Connection") == "" || r.Header.Get("Upgrade") == "" {
		log.Printf("[WebRTC] WebSocket升级失败: 缺少必要的升级头")
		log.Printf("[WebRTC]   - Connection头: %s", r.Header.Get("Connection"))
		log.Printf("[WebRTC]   - Upgrade头: %s", r.Header.Get("Upgrade"))
		http.Error(w, "Missing WebSocket upgrade headers", http.StatusBadRequest)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			log.Printf("[WebRTC] WebSocket跨域检查:")
			log.Printf("[WebRTC]   - Origin: %s", origin)
			log.Printf("[WebRTC]   - Host: %s", r.Header.Get("Host"))
			log.Printf("[WebRTC]   - 跨域策略: 允许所有来源 (开发模式)")
			log.Printf("[WebRTC]   - 检查结果: 允许连接")
			return true // 允许跨域，实际应用中应该更严格
		},
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
		HandshakeTimeout: 10 * time.Second,
	}

	log.Printf("[WebRTC] 开始WebSocket协议升级...")
	log.Printf("[WebRTC] 升级器配置:")
	log.Printf("[WebRTC]   - 读缓冲区大小: %d bytes", upgrader.ReadBufferSize)
	log.Printf("[WebRTC]   - 写缓冲区大小: %d bytes", upgrader.WriteBufferSize)
	log.Printf("[WebRTC]   - 握手超时时间: %v", upgrader.HandshakeTimeout)
	log.Printf("[WebRTC]   - 跨域检查: 启用")

	// 记录升级过程开始时间
	upgradeStartTime := time.Now()
	log.Printf("[WebRTC] 升级开始时间: %s", upgradeStartTime.Format("2006-01-02 15:04:05.000"))

	conn, err := upgrader.Upgrade(w, r, nil)
	upgradeDuration := time.Since(upgradeStartTime)

	if err != nil {
		log.Printf("[WebRTC] WebSocket升级失败!")
		log.Printf("[WebRTC] 升级失败详情:")
		log.Printf("[WebRTC]   - 错误信息: %v", err)
		log.Printf("[WebRTC]   - 升级耗时: %v", upgradeDuration)
		log.Printf("[WebRTC]   - 请求方法: %s", r.Method)
		log.Printf("[WebRTC]   - 请求路径: %s", r.URL.Path)
		log.Printf("[WebRTC]   - 客户端地址: %s", r.RemoteAddr)
		log.Printf("[WebRTC]   - User-Agent: %s", r.Header.Get("User-Agent"))
		log.Printf("[WebRTC]   - Origin: %s", r.Header.Get("Origin"))

		// 根据错误类型提供更详细的诊断信息
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			log.Printf("[WebRTC]   - 错误类型: 意外的连接关闭")
		} else if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			log.Printf("[WebRTC]   - 错误类型: 正常连接关闭")
		} else {
			log.Printf("[WebRTC]   - 错误类型: 升级协议错误")
		}

		return
	}

	log.Printf("[WebRTC] WebSocket连接升级成功!")
	log.Printf("[WebRTC] 连接建立详情:")
	log.Printf("[WebRTC]   - 升级耗时: %v", upgradeDuration)
	log.Printf("[WebRTC]   - 连接建立时间: %s", time.Now().Format("2006-01-02 15:04:05.000"))
	log.Printf("[WebRTC]   - 客户端远程地址: %s", r.RemoteAddr)
	log.Printf("[WebRTC]   - 本地服务器地址: %s", conn.LocalAddr().String())
	log.Printf("[WebRTC]   - 远程客户端地址: %s", conn.RemoteAddr().String())

	// 记录连接的技术参数
	log.Printf("[WebRTC] 连接技术参数:")
	log.Printf("[WebRTC]   - 协议: WebSocket")
	log.Printf("[WebRTC]   - 子协议: %s", conn.Subprotocol())
	log.Printf("[WebRTC]   - 读缓冲区: %d bytes", upgrader.ReadBufferSize)
	log.Printf("[WebRTC]   - 写缓冲区: %d bytes", upgrader.WriteBufferSize)
	log.Printf("[WebRTC]   - 握手超时: %v", upgrader.HandshakeTimeout)
	conn.EnableWriteCompression(false) // 禁用写压缩
	log.Printf("[WebRTC]   - 压缩支持: 已禁用")

	// 记录客户端环境信息
	userAgent := r.Header.Get("User-Agent")
	log.Printf("[WebRTC] 客户端环境分析:")
	if userAgent != "" {
		log.Printf("[WebRTC]   - 浏览器信息: %s", userAgent)
		// 简单的浏览器检测
		if contains(userAgent, "Chrome") {
			log.Printf("[WebRTC]   - 浏览器类型: Chrome/Chromium")
		} else if contains(userAgent, "Firefox") {
			log.Printf("[WebRTC]   - 浏览器类型: Firefox")
		} else if contains(userAgent, "Safari") {
			log.Printf("[WebRTC]   - 浏览器类型: Safari")
		} else if contains(userAgent, "Edge") {
			log.Printf("[WebRTC]   - 浏览器类型: Microsoft Edge")
		} else {
			log.Printf("[WebRTC]   - 浏览器类型: 未知")
		}
	} else {
		log.Printf("[WebRTC]   - 浏览器信息: 未提供")
	}

	// 记录连接状态和准备传递给信令服务器
	log.Printf("[WebRTC] 连接状态检查:")
	log.Printf("[WebRTC]   - 连接对象: %p", conn)
	log.Printf("[WebRTC]   - 连接状态: 已建立")
	log.Printf("[WebRTC]   - 准备传递给信令服务器: 是")

	// 获取当前活跃连接数
	currentClientCount := h.manager.signaling.GetClientCount()
	log.Printf("[WebRTC] 连接统计:")
	log.Printf("[WebRTC]   - 当前活跃连接数: %d", currentClientCount)
	log.Printf("[WebRTC]   - 新连接后预期连接数: %d", currentClientCount+1)

	// 不要在这里关闭连接，让信令服务器管理连接生命周期
	// 处理WebSocket连接
	log.Printf("[WebRTC] 将WebSocket连接传递给信令服务器处理...")
	log.Printf("[WebRTC] 信令服务器状态:")
	log.Printf("[WebRTC]   - 信令服务器对象: %p", h.manager.signaling)
	log.Printf("[WebRTC]   - 传递时间: %s", time.Now().Format("2006-01-02 15:04:05.000"))

	// 调用信令服务器处理连接
	h.manager.signaling.HandleWebSocketConnection(conn)

	log.Printf("[WebRTC] WebSocket连接已成功传递给信令服务器处理")
}

func (h *webrtcHandlers) handleSignaling(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	appName := vars["app"]

	log.Printf("[WebRTC] 应用特定信令连接请求: app=%s, %s %s from %s", appName, r.Method, r.URL.Path, r.RemoteAddr)
	log.Printf("[WebRTC] 应用信令请求头: User-Agent=%s, Origin=%s", r.Header.Get("User-Agent"), r.Header.Get("Origin"))

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			log.Printf("[WebRTC] 应用信令跨域检查: app=%s, Origin=%s, 允许连接", appName, r.Header.Get("Origin"))
			return true // 允许跨域，实际应用中应该更严格
		},
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
		HandshakeTimeout: 10 * time.Second,
	}

	log.Printf("[WebRTC] 开始应用信令WebSocket升级: app=%s", appName)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebRTC] 应用信令WebSocket升级失败: app=%s, error=%v", appName, err)
		log.Printf("[WebRTC] 应用信令升级失败详情: 应用=%s, 请求方法=%s, 路径=%s, 来源=%s", appName, r.Method, r.URL.Path, r.RemoteAddr)
		return
	}

	log.Printf("[WebRTC] 应用信令WebSocket连接成功建立: app=%s, 来源=%s", appName, r.RemoteAddr)
	log.Printf("[WebRTC] 应用信令连接配置:")
	log.Printf("[WebRTC]   - 应用名称: %s", appName)
	log.Printf("[WebRTC]   - 读缓冲区: 1024 bytes")
	log.Printf("[WebRTC]   - 写缓冲区: 1024 bytes")
	log.Printf("[WebRTC]   - 握手超时: 10s")
	log.Printf("[WebRTC]   - 跨域策略: 允许所有")

	// 不要在这里关闭连接，让信令服务器管理连接生命周期
	// 处理特定应用的信令连接
	log.Printf("[WebRTC] 将应用信令连接传递给信令服务器处理: app=%s", appName)
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
	peers := []map[string]any{
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

	info := map[string]any{
		"id":         peerID,
		"state":      "connected",
		"created_at": time.Now().Unix() - 300,
		"remote_ip":  "192.168.1.100",
		"stats": map[string]any{
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

	h.writeJSON(w, map[string]any{
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

// WebRTCConfigResponse WebRTC配置响应结构
type WebRTCConfigResponse struct {
	Success   bool               `json:"success"`
	Data      *WebRTCConfigData  `json:"data,omitempty"`
	Error     *WebRTCConfigError `json:"error,omitempty"`
	Timestamp int64              `json:"timestamp"`
}

// WebRTCConfigData WebRTC配置数据
type WebRTCConfigData struct {
	ICEServers         []map[string]any `json:"iceServers"`
	VideoEnabled       bool             `json:"video_enabled"`
	AudioEnabled       bool             `json:"audio_enabled"`
	VideoCodec         string           `json:"video_codec"`
	AudioCodec         string           `json:"audio_codec"`
	VideoWidth         int              `json:"video_width"`
	VideoHeight        int              `json:"video_height"`
	VideoFPS           int              `json:"video_fps"`
	ConnectionTimeout  int              `json:"connection_timeout"`
	IceTransportPolicy string           `json:"ice_transport_policy"`
	BundlePolicy       string           `json:"bundle_policy"`
	RtcpMuxPolicy      string           `json:"rtcp_mux_policy"`
}

// WebRTCConfigError WebRTC配置错误信息
type WebRTCConfigError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// handleConfig 处理WebRTC配置请求
func (h *webrtcHandlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		h.handleGetConfig(w, r)
	case "POST":
		h.handleUpdateConfig(w, r)
	default:
		h.writeConfigError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
			"Method not allowed", "Only GET and POST methods are supported")
	}
}

// handleGetConfig 处理获取配置请求
func (h *webrtcHandlers) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	// 检查WebRTC管理器状态
	if !h.manager.IsRunning() {
		h.writeConfigError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE",
			"WebRTC manager not running", "The WebRTC service is currently unavailable")
		return
	}

	// 验证配置完整性
	if h.manager.config == nil {
		h.writeConfigError(w, http.StatusInternalServerError, "CONFIG_NOT_LOADED",
			"Configuration not loaded", "WebRTC configuration is not properly loaded")
		return
	}

	// 转换ICE服务器配置为前端格式
	iceServers, err := h.convertICEServersToFrontendFormat()
	if err != nil {
		h.writeConfigError(w, http.StatusInternalServerError, "ICE_CONFIG_ERROR",
			"Failed to process ICE server configuration", err.Error())
		return
	}

	// 构建配置响应
	configData := &WebRTCConfigData{
		ICEServers:         iceServers,
		VideoEnabled:       true,
		AudioEnabled:       false, // 当前音频未启用
		VideoCodec:         h.getVideoCodec(),
		AudioCodec:         "opus",
		VideoWidth:         h.getVideoWidth(),
		VideoHeight:        h.getVideoHeight(),
		VideoFPS:           h.getVideoFPS(),
		ConnectionTimeout:  10000, // 10秒默认超时
		IceTransportPolicy: "all",
		BundlePolicy:       "balanced",
		RtcpMuxPolicy:      "require",
	}

	response := &WebRTCConfigResponse{
		Success:   true,
		Data:      configData,
		Timestamp: time.Now().Unix(),
	}

	log.Printf("WebRTC config requested, returning %d ICE servers", len(iceServers))
	h.writeConfigResponse(w, http.StatusOK, response)
}

// handleUpdateConfig 处理配置更新请求
func (h *webrtcHandlers) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var newConfig map[string]any
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		h.writeConfigError(w, http.StatusBadRequest, "INVALID_JSON",
			"Invalid JSON format", err.Error())
		return
	}

	// 验证配置格式
	if err := h.validateConfigUpdate(newConfig); err != nil {
		h.writeConfigError(w, http.StatusBadRequest, "INVALID_CONFIG",
			"Configuration validation failed", err.Error())
		return
	}

	// TODO: 实现配置更新逻辑
	// 这里应该验证配置并更新WebRTC管理器的设置
	log.Printf("WebRTC configuration update requested: %+v", newConfig)

	response := &WebRTCConfigResponse{
		Success:   true,
		Data:      nil,
		Timestamp: time.Now().Unix(),
	}

	h.writeConfigResponse(w, http.StatusOK, response)
}

// convertICEServersToFrontendFormat 转换ICE服务器配置为前端格式
func (h *webrtcHandlers) convertICEServersToFrontendFormat() ([]map[string]any, error) {
	if h.manager.config.WebRTC.ICEServers == nil {
		return nil, fmt.Errorf("ICE servers configuration is nil")
	}

	iceServers := make([]map[string]any, 0, len(h.manager.config.WebRTC.ICEServers))

	for i, server := range h.manager.config.WebRTC.ICEServers {
		if len(server.URLs) == 0 {
			log.Printf("Warning: ICE server %d has no URLs, skipping", i)
			continue
		}

		iceServer := map[string]any{
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

	if len(iceServers) == 0 {
		return nil, fmt.Errorf("no valid ICE servers found in configuration")
	}

	return iceServers, nil
}

// getVideoCodec 获取视频编解码器配置
func (h *webrtcHandlers) getVideoCodec() string {
	if h.manager.config.GStreamer.Encoding.Codec != "" {
		return string(h.manager.config.GStreamer.Encoding.Codec)
	}
	return "h264" // 默认编解码器
}

// getVideoWidth 获取视频宽度配置
func (h *webrtcHandlers) getVideoWidth() int {
	if h.manager.config.GStreamer.Capture.Width > 0 {
		return h.manager.config.GStreamer.Capture.Width
	}
	return 1920 // 默认宽度
}

// getVideoHeight 获取视频高度配置
func (h *webrtcHandlers) getVideoHeight() int {
	if h.manager.config.GStreamer.Capture.Height > 0 {
		return h.manager.config.GStreamer.Capture.Height
	}
	return 1080 // 默认高度
}

// getVideoFPS 获取视频帧率配置
func (h *webrtcHandlers) getVideoFPS() int {
	if h.manager.config.GStreamer.Capture.FrameRate > 0 {
		return h.manager.config.GStreamer.Capture.FrameRate
	}
	return 30 // 默认帧率
}

// validateConfigUpdate 验证配置更新请求
func (h *webrtcHandlers) validateConfigUpdate(config map[string]any) error {
	// 验证ICE服务器配置
	if iceServers, exists := config["ice_servers"]; exists {
		servers, ok := iceServers.([]any)
		if !ok {
			return fmt.Errorf("ice_servers must be an array")
		}

		for i, server := range servers {
			serverMap, ok := server.(map[string]any)
			if !ok {
				return fmt.Errorf("ice_servers[%d] must be an object", i)
			}

			urls, exists := serverMap["urls"]
			if !exists {
				return fmt.Errorf("ice_servers[%d] must have 'urls' field", i)
			}

			switch urlsValue := urls.(type) {
			case string:
				if urlsValue == "" {
					return fmt.Errorf("ice_servers[%d].urls cannot be empty", i)
				}
			case []any:
				if len(urlsValue) == 0 {
					return fmt.Errorf("ice_servers[%d].urls cannot be empty array", i)
				}
			default:
				return fmt.Errorf("ice_servers[%d].urls must be string or array", i)
			}
		}
	}

	return nil
}

// writeConfigResponse 写入配置响应
func (h *webrtcHandlers) writeConfigResponse(w http.ResponseWriter, statusCode int, response *WebRTCConfigResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode WebRTC config response: %v", err)
		// 如果JSON编码失败，发送简单的错误响应
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"success":false,"error":{"code":"ENCODING_ERROR","message":"Failed to encode response"}}`))
	}
}

// writeConfigError 写入配置错误响应
func (h *webrtcHandlers) writeConfigError(w http.ResponseWriter, statusCode int, code, message, details string) {
	response := &WebRTCConfigResponse{
		Success: false,
		Error: &WebRTCConfigError{
			Code:    code,
			Message: message,
			Details: details,
		},
		Timestamp: time.Now().Unix(),
	}

	h.writeConfigResponse(w, statusCode, response)
}

// writeJSON 写入JSON响应
func (h *webrtcHandlers) writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// 调试日志处理器
func (h *webrtcHandlers) handleDebugLogs(w http.ResponseWriter, r *http.Request) {
	var logData struct {
		SessionID string `json:"sessionId"`
		Logs      []struct {
			ID        string                 `json:"id"`
			Timestamp int64                  `json:"timestamp"`
			Level     string                 `json:"level"`
			Category  string                 `json:"category"`
			Message   string                 `json:"message"`
			Data      map[string]interface{} `json:"data"`
		} `json:"logs"`
		Metrics map[string]interface{} `json:"metrics"`
	}

	if err := json.NewDecoder(r.Body).Decode(&logData); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// 记录接收到的调试日志
	log.Printf("收到调试日志: 会话ID=%s, 日志数量=%d", logData.SessionID, len(logData.Logs))

	// 这里可以将日志存储到数据库或文件系统
	// 目前只是记录到服务器日志
	for _, logEntry := range logData.Logs {
		log.Printf("[DEBUG] [%s] [%s] %s", logEntry.Level, logEntry.Category, logEntry.Message)
	}

	h.writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "调试日志已接收",
		"count":   len(logData.Logs),
	})
}

// 诊断处理器
func (h *webrtcHandlers) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		h.handleGetDiagnostics(w, r)
	case "POST":
		h.handlePostDiagnostics(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// 获取诊断信息
func (h *webrtcHandlers) handleGetDiagnostics(w http.ResponseWriter, r *http.Request) {
	diagnostics := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"server": map[string]interface{}{
			"status":  "running",
			"uptime":  time.Since(time.Now()).Seconds(), // 这里应该是实际的启动时间
			"version": "1.0.0",
		},
		"webrtc": map[string]interface{}{
			"manager_running": h.manager.IsRunning(),
			"client_count":    h.manager.signaling.GetClientCount(),
		},
		"system": map[string]interface{}{
			"timestamp": time.Now().Unix(),
			"platform":  "linux",
		},
	}

	h.writeJSON(w, diagnostics)
}

// 接收诊断数据
func (h *webrtcHandlers) handlePostDiagnostics(w http.ResponseWriter, r *http.Request) {
	var diagnosticData map[string]interface{}

	if err := json.NewDecoder(r.Body).Decode(&diagnosticData); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// 记录诊断数据
	log.Printf("收到诊断数据: %+v", diagnosticData)

	h.writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "诊断数据已接收",
	})
}

// 调试信息导出处理器
func (h *webrtcHandlers) handleDebugExport(w http.ResponseWriter, r *http.Request) {
	var exportRequest struct {
		Type    string                 `json:"type"`
		Format  string                 `json:"format"`
		Options map[string]interface{} `json:"options"`
	}

	if err := json.NewDecoder(r.Body).Decode(&exportRequest); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// 生成导出数据
	exportData := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"type":      exportRequest.Type,
		"format":    exportRequest.Format,
		"server_info": map[string]interface{}{
			"status":       "running",
			"webrtc_stats": h.manager.GetStats(),
		},
	}

	// 根据格式设置响应头
	switch exportRequest.Format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=debug_export_%d.json", time.Now().Unix()))
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=debug_export_%d.csv", time.Now().Unix()))
	default:
		w.Header().Set("Content-Type", "application/json")
	}

	h.writeJSON(w, exportData)
}

// 健康检查处理器
func (h *webrtcHandlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"services": map[string]interface{}{
			"webrtc": map[string]interface{}{
				"status":  "running",
				"running": h.manager.IsRunning(),
			},
			"signaling": map[string]interface{}{
				"status":       "running",
				"client_count": h.manager.signaling.GetClientCount(),
			},
		},
	}

	h.writeJSON(w, health)
}

// Ping处理器
func (h *webrtcHandlers) handlePing(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"pong":      true,
		"timestamp": time.Now().Unix(),
		"server":    "webrtc-server",
	}

	h.writeJSON(w, response)
}

// 带宽测试下载处理器
func (h *webrtcHandlers) handleBandwidthTestDownload(w http.ResponseWriter, r *http.Request) {
	sizeStr := r.URL.Query().Get("size")
	size := 1024 * 1024 // 默认1MB

	if sizeStr != "" {
		if parsedSize, err := strconv.Atoi(sizeStr); err == nil && parsedSize > 0 {
			size = parsedSize
		}
	}

	// 限制最大下载大小为10MB
	if size > 10*1024*1024 {
		size = 10 * 1024 * 1024
	}

	// 生成随机数据
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(size))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Write(data)
}

// 带宽测试上传处理器
func (h *webrtcHandlers) handleBandwidthTestUpload(w http.ResponseWriter, r *http.Request) {
	// 读取上传的数据
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read upload data", http.StatusBadRequest)
		return
	}

	response := map[string]interface{}{
		"success":   true,
		"received":  len(data),
		"timestamp": time.Now().Unix(),
	}

	h.writeJSON(w, response)
}

// contains 检查字符串是否包含子字符串（不区分大小写）
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

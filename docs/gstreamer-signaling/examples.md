# GStreamer WebRTC 信令协议示例代码

## 概述

本文档提供了 GStreamer WebRTC 信令协议的完整示例代码，包括客户端和服务端的实现示例、测试用例和最佳实践。

## 客户端实现示例

### JavaScript 客户端完整实现

```javascript
/**
 * GStreamer WebRTC 信令客户端
 * 支持标准协议和 Selkies 兼容模式
 */
class GStreamerSignalingClient {
  constructor(options = {}) {
    // 基础配置
    this.serverUrl = options.serverUrl || this._generateServerUrl();
    this.peerId = options.peerId || this._generatePeerId();
    this.protocol = options.protocol || 'gstreamer-1.0';
    
    // 事件系统
    this.eventBus = options.eventBus || new EventTarget();
    this.logger = options.logger || console;
    
    // 连接配置
    this.config = {
      maxRetries: options.maxRetries || 3,
      retryDelay: options.retryDelay || 3000,
      connectionTimeout: options.connectionTimeout || 10000,
      heartbeatInterval: options.heartbeatInterval || 30000,
      ...options.config
    };
    
    // 内部状态
    this._ws = null;
    this._state = 'disconnected';
    this._retryCount = 0;
    this._messageId = 0;
    this._heartbeatTimer = null;
    this._connectionTimer = null;
    this._pendingMessages = new Map();
    
    // 协议检测
    this._protocolMode = 'standard';
    this._selkiesMode = false;
    
    // 事件回调
    this.onopen = null;
    this.onclose = null;
    this.onerror = null;
    this.onmessage = null;
    this.onsdp = null;
    this.onice = null;
    this.onstatus = null;
  }
  
  /**
   * 连接到信令服务器
   */
  async connect() {
    if (this._state === 'connected' || this._state === 'connecting') {
      throw new Error('Already connected or connecting');
    }
    
    this._state = 'connecting';
    this._emit('status', 'Connecting to signaling server...');
    
    try {
      // 创建 WebSocket 连接
      this._ws = new WebSocket(this.serverUrl);
      this._setupWebSocketHandlers();
      
      // 设置连接超时
      this._connectionTimer = setTimeout(() => {
        if (this._ws && this._ws.readyState === WebSocket.CONNECTING) {
          this._ws.close();
          throw new Error('Connection timeout');
        }
      }, this.config.connectionTimeout);
      
      // 等待连接建立
      await this._waitForConnection();
      
      // 发送 HELLO 消息
      await this._sendHello();
      
      // 连接成功
      this._state = 'connected';
      this._retryCount = 0;
      this._startHeartbeat();
      
      this._emit('status', 'Connected to signaling server');
      if (this.onopen) this.onopen();
      
    } catch (error) {
      this._state = 'disconnected';
      this._clearTimers();
      throw error;
    }
  }
  
  /**
   * 断开连接
   */
  disconnect() {
    this._state = 'disconnecting';
    this._clearTimers();
    
    if (this._ws) {
      this._ws.close(1000, 'Client disconnect');
      this._ws = null;
    }
    
    this._state = 'disconnected';
    this._retryCount = 0;
    this._pendingMessages.clear();
    
    this._emit('status', 'Disconnected from signaling server');
    if (this.onclose) this.onclose({ wasClean: true });
  }
  
  /**
   * 发送标准格式消息
   */
  sendMessage(type, data = null, options = {}) {
    if (this._state !== 'connected') {
      throw new Error('Not connected to signaling server');
    }
    
    const message = {
      version: '1.0',
      type: type,
      id: this._generateMessageId(),
      timestamp: Date.now(),
      peer_id: this.peerId,
      data: data
    };
    
    // 处理消息确认
    if (options.expectAck) {
      this._pendingMessages.set(message.id, {
        message,
        timestamp: Date.now(),
        resolve: options.resolve,
        reject: options.reject,
        timeout: setTimeout(() => {
          this._pendingMessages.delete(message.id);
          if (options.reject) {
            options.reject(new Error('Message acknowledgment timeout'));
          }
        }, options.ackTimeout || 5000)
      });
    }
    
    this._ws.send(JSON.stringify(message));
    this.logger.debug('Message sent:', message);
    
    return message.id;
  }
  
  /**
   * 发送 SDP 消息
   */
  sendSDP(sdp) {
    if (this._selkiesMode) {
      // Selkies 格式
      const message = { sdp: sdp };
      this._ws.send(JSON.stringify(message));
    } else {
      // 标准格式
      this.sendMessage('answer', { sdp: sdp });
    }
  }
  
  /**
   * 发送 ICE 候选
   */
  sendICE(candidate) {
    if (this._selkiesMode) {
      // Selkies 格式
      const message = { ice: candidate };
      this._ws.send(JSON.stringify(message));
    } else {
      // 标准格式
      this.sendMessage('ice-candidate', { candidate: candidate });
    }
  }
  
  /**
   * 发送输入事件
   */
  sendInputEvent(eventType, eventData) {
    this.sendMessage(eventType, eventData);
  }
  
  /**
   * 请求统计信息
   */
  async requestStats(statsTypes = ['webrtc', 'system']) {
    return new Promise((resolve, reject) => {
      this.sendMessage('get-stats', { stats_type: statsTypes }, {
        expectAck: true,
        resolve: resolve,
        reject: reject
      });
    });
  }
  
  /**
   * 协议协商
   */
  async negotiateProtocol(supportedProtocols = ['gstreamer-1.0', 'selkies']) {
    return new Promise((resolve, reject) => {
      this.sendMessage('protocol-negotiation', {
        supported_protocols: supportedProtocols,
        preferred_protocol: supportedProtocols[0],
        client_capabilities: this._getClientCapabilities()
      }, {
        expectAck: true,
        resolve: resolve,
        reject: reject
      });
    });
  }
  
  /**
   * 获取连接状态
   */
  getConnectionState() {
    return {
      connectionState: this._state,
      readyState: this._ws ? this._ws.readyState : WebSocket.CLOSED,
      peerId: this.peerId,
      retryCount: this._retryCount,
      serverUrl: this.serverUrl,
      protocol: this._protocolMode,
      isConnected: this._state === 'connected',
      canRetry: this._retryCount < this.config.maxRetries
    };
  }
  
  // 私有方法
  
  _setupWebSocketHandlers() {
    this._ws.onopen = (event) => {
      this._clearConnectionTimer();
      this.logger.info('WebSocket connection opened');
    };
    
    this._ws.onclose = (event) => {
      this._handleConnectionClose(event);
    };
    
    this._ws.onerror = (event) => {
      this._handleConnectionError(event);
    };
    
    this._ws.onmessage = (event) => {
      this._handleMessage(event);
    };
  }
  
  _handleMessage(event) {
    try {
      const data = event.data;
      
      // 处理 Selkies 文本协议
      if (typeof data === 'string' && !data.startsWith('{')) {
        this._handleSelkiesTextMessage(data);
        return;
      }
      
      // 解析 JSON 消息
      const message = JSON.parse(data);
      
      // 检测协议模式
      this._detectProtocolMode(message);
      
      // 处理消息确认
      this._handleMessageAck(message);
      
      // 路由消息到处理器
      this._routeMessage(message);
      
      // 触发通用消息回调
      if (this.onmessage) {
        this.onmessage(message);
      }
      
    } catch (error) {
      this.logger.error('Failed to handle message:', error);
      this._emit('error', new Error('Message parsing failed: ' + error.message));
    }
  }
  
  _routeMessage(message) {
    const messageType = message.type || this._detectMessageType(message);
    
    switch (messageType) {
      case 'welcome':
        this._handleWelcome(message);
        break;
      case 'offer':
        this._handleOffer(message);
        break;
      case 'ice-candidate':
        this._handleIceCandidate(message);
        break;
      case 'stats':
        this._handleStats(message);
        break;
      case 'error':
        this._handleError(message);
        break;
      case 'pong':
        this._handlePong(message);
        break;
      case 'protocol-selected':
        this._handleProtocolSelected(message);
        break;
      default:
        // 处理 Selkies JSON 消息
        if (message.sdp) {
          this._handleSelkiesSDP(message);
        } else if (message.ice) {
          this._handleSelkiesICE(message);
        } else {
          this.logger.warn('Unknown message type:', messageType, message);
        }
    }
  }
  
  _handleWelcome(message) {
    this._emit('status', 'Registered with server');
    
    if (message.data && message.data.protocol) {
      this._protocolMode = message.data.protocol;
      this._selkiesMode = message.data.protocol === 'selkies';
    }
    
    this._emit('welcome', message.data);
  }
  
  _handleOffer(message) {
    if (message.data && message.data.sdp) {
      const sdp = new RTCSessionDescription(message.data.sdp);
      if (this.onsdp) this.onsdp(sdp);
      this._emit('sdp', sdp);
    }
  }
  
  _handleIceCandidate(message) {
    if (message.data && message.data.candidate) {
      const candidate = new RTCIceCandidate(message.data.candidate);
      if (this.onice) this.onice(candidate);
      this._emit('ice-candidate', candidate);
    }
  }
  
  _handleStats(message) {
    if (message.data) {
      this._emit('stats', message.data);
    }
  }
  
  _handleError(message) {
    const error = new Error(message.error ? message.error.message : 'Server error');
    error.code = message.error ? message.error.code : 'UNKNOWN_ERROR';
    error.details = message.error ? message.error.details : null;
    
    this.logger.error('Server error:', error);
    if (this.onerror) this.onerror(error);
    this._emit('error', error);
  }
  
  _handlePong(message) {
    const now = Date.now();
    const pingTime = message.data ? message.data.timestamp : null;
    
    if (pingTime) {
      const latency = now - pingTime;
      this._emit('latency', { latency });
    }
  }
  
  _handleProtocolSelected(message) {
    if (message.data && message.data.selected_protocol) {
      this._protocolMode = message.data.selected_protocol;
      this._selkiesMode = message.data.selected_protocol === 'selkies';
      this._emit('protocol-selected', message.data);
    }
  }
  
  _handleSelkiesTextMessage(data) {
    if (data === 'HELLO') {
      this._selkiesMode = true;
      this._protocolMode = 'selkies';
      this._emit('status', 'Registered with server (Selkies mode)');
    } else if (data.startsWith('ERROR')) {
      const error = new Error('Server error: ' + data);
      if (this.onerror) this.onerror(error);
      this._emit('error', error);
    }
  }
  
  _handleSelkiesSDP(message) {
    if (message.sdp) {
      const sdp = new RTCSessionDescription(message.sdp);
      if (this.onsdp) this.onsdp(sdp);
      this._emit('sdp', sdp);
    }
  }
  
  _handleSelkiesICE(message) {
    if (message.ice) {
      const candidate = new RTCIceCandidate(message.ice);
      if (this.onice) this.onice(candidate);
      this._emit('ice-candidate', candidate);
    }
  }
  
  _sendHello() {
    if (this._selkiesMode || this._protocolMode === 'selkies') {
      // Selkies HELLO 格式
      const meta = {
        res: `${window.screen.width}x${window.screen.height}`,
        scale: window.devicePixelRatio
      };
      const helloMessage = `HELLO ${this.peerId} ${btoa(JSON.stringify(meta))}`;
      this._ws.send(helloMessage);
    } else {
      // 标准 HELLO 格式
      this.sendMessage('hello', {
        client_info: this._getClientInfo(),
        capabilities: this._getClientCapabilities(),
        supported_protocols: ['gstreamer-1.0', 'selkies'],
        preferred_protocol: this.protocol
      });
    }
  }
  
  _startHeartbeat() {
    this._stopHeartbeat();
    
    this._heartbeatTimer = setInterval(() => {
      if (this._state === 'connected') {
        this._sendPing();
      } else {
        this._stopHeartbeat();
      }
    }, this.config.heartbeatInterval);
  }
  
  _stopHeartbeat() {
    if (this._heartbeatTimer) {
      clearInterval(this._heartbeatTimer);
      this._heartbeatTimer = null;
    }
  }
  
  _sendPing() {
    try {
      this.sendMessage('ping', {
        timestamp: Date.now(),
        client_state: this._state
      });
    } catch (error) {
      this.logger.error('Failed to send ping:', error);
    }
  }
  
  _waitForConnection() {
    return new Promise((resolve, reject) => {
      if (this._ws.readyState === WebSocket.OPEN) {
        resolve();
        return;
      }
      
      const onOpen = () => {
        this._ws.removeEventListener('open', onOpen);
        this._ws.removeEventListener('error', onError);
        resolve();
      };
      
      const onError = (error) => {
        this._ws.removeEventListener('open', onOpen);
        this._ws.removeEventListener('error', onError);
        reject(error);
      };
      
      this._ws.addEventListener('open', onOpen);
      this._ws.addEventListener('error', onError);
    });
  }
  
  _handleConnectionClose(event) {
    const wasConnected = this._state === 'connected';
    this._state = 'disconnected';
    this._clearTimers();
    
    const closeInfo = {
      code: event.code,
      reason: event.reason,
      wasClean: event.wasClean,
      wasConnected: wasConnected
    };
    
    this.logger.info('WebSocket connection closed:', closeInfo);
    
    // 非正常关闭且之前已连接，尝试重连
    if (!event.wasClean && wasConnected && this._state !== 'disconnecting') {
      this._scheduleReconnect();
    }
    
    if (this.onclose) this.onclose(closeInfo);
    this._emit('close', closeInfo);
  }
  
  _handleConnectionError(event) {
    const error = {
      type: 'connection_error',
      message: 'WebSocket connection failed',
      event: event,
      recoverable: this._retryCount < this.config.maxRetries
    };
    
    this.logger.error('Connection error:', error);
    
    if (this.onerror) this.onerror(error);
    this._emit('error', error);
    
    // 自动重连
    if (error.recoverable && this._state !== 'disconnecting') {
      this._scheduleReconnect();
    }
  }
  
  _scheduleReconnect() {
    if (this._retryCount >= this.config.maxRetries) {
      const error = new Error('Max retry attempts exceeded');
      error.type = 'max_retries_exceeded';
      if (this.onerror) this.onerror(error);
      this._emit('error', error);
      return;
    }
    
    const delay = Math.min(
      this.config.retryDelay * Math.pow(2, this._retryCount),
      30000
    );
    
    this._retryCount++;
    
    this.logger.info(`Scheduling reconnect in ${delay}ms (attempt ${this._retryCount}/${this.config.maxRetries})`);
    this._emit('status', `Reconnecting in ${delay/1000}s... (${this._retryCount}/${this.config.maxRetries})`);
    
    setTimeout(() => {
      if (this._state === 'disconnected') {
        this.connect().catch(error => {
          this.logger.error('Reconnect failed:', error);
          if (this.onerror) this.onerror(error);
          this._emit('error', error);
        });
      }
    }, delay);
  }
  
  _clearTimers() {
    this._stopHeartbeat();
    
    if (this._connectionTimer) {
      clearTimeout(this._connectionTimer);
      this._connectionTimer = null;
    }
  }
  
  _generateServerUrl() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${window.location.host}/ws`;
  }
  
  _generatePeerId() {
    return Math.floor(Math.random() * 1000000);
  }
  
  _generateMessageId() {
    return `msg_${Date.now()}_${++this._messageId}`;
  }
  
  _getClientInfo() {
    return {
      user_agent: navigator.userAgent,
      screen_resolution: `${window.screen.width}x${window.screen.height}`,
      device_pixel_ratio: window.devicePixelRatio,
      language: navigator.language,
      platform: navigator.platform
    };
  }
  
  _getClientCapabilities() {
    return [
      'webrtc',
      'datachannel',
      'video',
      'audio',
      'input-events',
      'statistics'
    ];
  }
  
  _detectProtocolMode(message) {
    // 检测是否为 Selkies 协议
    if (message.sdp || message.ice) {
      this._selkiesMode = true;
      this._protocolMode = 'selkies';
    } else if (message.version) {
      this._selkiesMode = false;
      this._protocolMode = 'standard';
    }
  }
  
  _detectMessageType(message) {
    if (message.sdp) return 'sdp';
    if (message.ice) return 'ice';
    if (message.error) return 'error';
    return 'unknown';
  }
  
  _handleMessageAck(message) {
    // 处理消息确认
    if (message.id && this._pendingMessages.has(message.id)) {
      const pending = this._pendingMessages.get(message.id);
      this._pendingMessages.delete(message.id);
      
      if (pending.timeout) {
        clearTimeout(pending.timeout);
      }
      
      if (pending.resolve) {
        pending.resolve(message);
      }
    }
  }
  
  _emit(eventType, data) {
    const event = new CustomEvent(eventType, { detail: data });
    this.eventBus.dispatchEvent(event);
    
    // 兼容回调方式
    const callbackName = `on${eventType}`;
    if (typeof this[callbackName] === 'function') {
      this[callbackName](data);
    }
  }
}

// 使用示例
const signalingClient = new GStreamerSignalingClient({
  serverUrl: 'ws://localhost:8080/ws',
  peerId: 1,
  protocol: 'gstreamer-1.0'
});

// 事件监听
signalingClient.eventBus.addEventListener('status', (event) => {
  console.log('Status:', event.detail);
});

signalingClient.eventBus.addEventListener('sdp', (event) => {
  console.log('Received SDP:', event.detail);
});

signalingClient.eventBus.addEventListener('ice-candidate', (event) => {
  console.log('Received ICE candidate:', event.detail);
});

// 连接
signalingClient.connect().then(() => {
  console.log('Connected successfully');
}).catch(error => {
  console.error('Connection failed:', error);
});
```

## 服务端实现示例

### Golang 服务端完整实现

```go
package signaling

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "sync"
    "time"
    
    "github.com/gorilla/websocket"
)

// SignalingServer 信令服务器实现
type SignalingServer struct {
    // 客户端管理
    clients    map[string]*SignalingClient
    apps       map[string]map[string]*SignalingClient
    mutex      sync.RWMutex
    
    // 通道管理
    register   chan *SignalingClient
    unregister chan *SignalingClient
    broadcast  chan []byte
    
    // WebSocket 升级器
    upgrader   websocket.Upgrader
    
    // 协议适配器
    adapters   map[string]ProtocolAdapter
    
    // 配置和状态
    config     *SignalingConfig
    ctx        context.Context
    cancel     context.CancelFunc
    running    bool
    logger     *log.Logger
}

// SignalingConfig 服务器配置
type SignalingConfig struct {
    Host              string        `json:"host"`
    Port              int           `json:"port"`
    MaxConnections    int           `json:"max_connections"`
    ConnectionTimeout time.Duration `json:"connection_timeout"`
    HeartbeatInterval time.Duration `json:"heartbeat_interval"`
    CleanupInterval   time.Duration `json:"cleanup_interval"`
    MaxMessageSize    int           `json:"max_message_size"`
    SupportedProtocols []string     `json:"supported_protocols"`
    DefaultProtocol   string        `json:"default_protocol"`
}

// NewSignalingServer 创建信令服务器
func NewSignalingServer(ctx context.Context, config *SignalingConfig) *SignalingServer {
    childCtx, cancel := context.WithCancel(ctx)
    
    server := &SignalingServer{
        clients:    make(map[string]*SignalingClient),
        apps:       make(map[string]map[string]*SignalingClient),
        register:   make(chan *SignalingClient),
        unregister: make(chan *SignalingClient),
        broadcast:  make(chan []byte),
        adapters:   make(map[string]ProtocolAdapter),
        config:     config,
        ctx:        childCtx,
        cancel:     cancel,
        logger:     log.Default(),
        upgrader: websocket.Upgrader{
            CheckOrigin: func(r *http.Request) bool {
                return true // 允许跨域
            },
            ReadBufferSize:  1024,
            WriteBufferSize: 1024,
        },
    }
    
    // 注册协议适配器
    server.registerAdapters()
    
    return server
}

// Start 启动信令服务器
func (s *SignalingServer) Start() error {
    s.logger.Printf("Starting signaling server on %s:%d", s.config.Host, s.config.Port)
    s.running = true
    
    // 启动主循环
    go s.run()
    
    // 启动清理协程
    go s.cleanupRoutine()
    
    // 设置 HTTP 路由
    http.HandleFunc("/ws", s.HandleWebSocket)
    http.HandleFunc("/ws/", s.HandleAppWebSocket)
    http.HandleFunc("/api/signaling/status", s.HandleStatus)
    http.HandleFunc("/api/signaling/clients", s.HandleClients)
    
    // 启动 HTTP 服务器
    addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
    return http.ListenAndServe(addr, nil)
}

// Stop 停止信令服务器
func (s *SignalingServer) Stop() {
    s.logger.Println("Stopping signaling server...")
    s.running = false
    s.cancel()
    
    s.mutex.Lock()
    defer s.mutex.Unlock()
    
    // 关闭所有客户端连接
    for _, client := range s.clients {
        close(client.Send)
    }
    
    s.clients = make(map[string]*SignalingClient)
    s.apps = make(map[string]map[string]*SignalingClient)
    
    s.logger.Println("Signaling server stopped")
}

// HandleWebSocket 处理 WebSocket 连接
func (s *SignalingServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := s.upgrader.Upgrade(w, r, nil)
    if err != nil {
        s.logger.Printf("WebSocket upgrade failed: %v", err)
        return
    }
    
    s.handleConnection(conn, r, "default")
}

// HandleAppWebSocket 处理应用特定的 WebSocket 连接
func (s *SignalingServer) HandleAppWebSocket(w http.ResponseWriter, r *http.Request) {
    appName := r.URL.Path[4:] // 移除 "/ws/" 前缀
    if appName == "" {
        appName = "default"
    }
    
    conn, err := s.upgrader.Upgrade(w, r, nil)
    if err != nil {
        s.logger.Printf("WebSocket upgrade failed for app %s: %v", appName, err)
        return
    }
    
    s.handleConnection(conn, r, appName)
}

// handleConnection 处理新的 WebSocket 连接
func (s *SignalingServer) handleConnection(conn *websocket.Conn, r *http.Request, appName string) {
    client := &SignalingClient{
        ID:           s.generateClientID(),
        AppName:      appName,
        Conn:         conn,
        Send:         make(chan []byte, 256),
        Server:       s,
        LastSeen:     time.Now(),
        ConnectedAt:  time.Now(),
        RemoteAddr:   conn.RemoteAddr().String(),
        UserAgent:    r.Header.Get("User-Agent"),
        State:        ClientStateConnecting,
        Protocol:     s.config.DefaultProtocol,
        messageHandlers: s.createMessageHandlers(),
    }
    
    // 检测客户端协议
    s.detectClientProtocol(client, r)
    
    s.logger.Printf("New client connected: %s (app: %s, protocol: %s)", 
        client.ID, appName, client.Protocol)
    
    // 启动客户端处理协程
    go client.readPump()
    go client.writePump()
    
    // 注册客户端
    s.register <- client
    
    // 发送欢迎消息
    s.sendWelcomeMessage(client)
}

// run 主事件循环
func (s *SignalingServer) run() {
    for s.running {
        select {
        case client := <-s.register:
            s.registerClient(client)
            
        case client := <-s.unregister:
            s.unregisterClient(client)
            
        case message := <-s.broadcast:
            s.broadcastMessage(message)
            
        case <-s.ctx.Done():
            return
        }
    }
}

// registerClient 注册客户端
func (s *SignalingServer) registerClient(client *SignalingClient) {
    s.mutex.Lock()
    defer s.mutex.Unlock()
    
    // 检查连接数限制
    if len(s.clients) >= s.config.MaxConnections {
        s.logger.Printf("Max connections reached, rejecting client %s", client.ID)
        client.sendError(&SignalingError{
            Code:    "SERVER_UNAVAILABLE",
            Message: "Server at maximum capacity",
        })
        client.Conn.Close()
        return
    }
    
    // 注册客户端
    s.clients[client.ID] = client
    
    // 按应用分组
    if s.apps[client.AppName] == nil {
        s.apps[client.AppName] = make(map[string]*SignalingClient)
    }
    s.apps[client.AppName][client.ID] = client
    
    client.setState(ClientStateConnected)
    
    s.logger.Printf("Client registered: %s (total: %d)", client.ID, len(s.clients))
}

// unregisterClient 注销客户端
func (s *SignalingServer) unregisterClient(client *SignalingClient) {
    s.mutex.Lock()
    defer s.mutex.Unlock()
    
    if _, exists := s.clients[client.ID]; exists {
        delete(s.clients, client.ID)
        
        // 从应用分组中移除
        if appClients, exists := s.apps[client.AppName]; exists {
            delete(appClients, client.ID)
            if len(appClients) == 0 {
                delete(s.apps, client.AppName)
            }
        }
        
        close(client.Send)
        
        duration := time.Since(client.ConnectedAt)
        s.logger.Printf("Client unregistered: %s (duration: %v, messages: %d)", 
            client.ID, duration, client.MessageCount)
    }
}

// sendWelcomeMessage 发送欢迎消息
func (s *SignalingServer) sendWelcomeMessage(client *SignalingClient) {
    welcome := SignalingMessage{
        Version:   "1.0",
        Type:      "welcome",
        ID:        s.generateMessageID(),
        Timestamp: time.Now().Unix(),
        PeerID:    client.ID,
        Data: map[string]interface{}{
            "client_id":    client.ID,
            "app_name":     client.AppName,
            "server_time":  time.Now().Unix(),
            "protocol":     client.Protocol,
            "capabilities": []string{"webrtc", "input", "stats"},
            "session_config": map[string]interface{}{
                "heartbeat_interval": s.config.HeartbeatInterval.Milliseconds(),
                "max_message_size":   s.config.MaxMessageSize,
            },
        },
    }
    
    if err := client.sendMessage(welcome); err != nil {
        s.logger.Printf("Failed to send welcome message to %s: %v", client.ID, err)
    }
}

// createMessageHandlers 创建消息处理器映射
func (s *SignalingServer) createMessageHandlers() map[string]MessageHandler {
    return map[string]MessageHandler{
        "ping":                s.handlePing,
        "hello":               s.handleHello,
        "request-offer":       s.handleRequestOffer,
        "answer":              s.handleAnswer,
        "ice-candidate":       s.handleIceCandidate,
        "mouse-click":         s.handleMouseClick,
        "mouse-move":          s.handleMouseMove,
        "key-press":           s.handleKeyPress,
        "get-stats":           s.handleGetStats,
        "protocol-negotiation": s.handleProtocolNegotiation,
    }
}

// 消息处理器实现

func (s *SignalingServer) handlePing(client *SignalingClient, message *SignalingMessage) error {
    response := SignalingMessage{
        Version:   "1.0",
        Type:      "pong",
        ID:        s.generateMessageID(),
        Timestamp: time.Now().Unix(),
        PeerID:    client.ID,
        Data: map[string]interface{}{
            "timestamp":     time.Now().Unix(),
            "client_state":  client.State,
            "message_count": client.MessageCount,
        },
    }
    
    return client.sendMessage(response)
}

func (s *SignalingServer) handleHello(client *SignalingClient, message *SignalingMessage) error {
    s.logger.Printf("Hello received from client %s", client.ID)
    
    // 处理客户端信息
    if data, ok := message.Data.(map[string]interface{}); ok {
        if clientInfo, exists := data["client_info"]; exists {
            s.logger.Printf("Client info: %+v", clientInfo)
        }
        
        if capabilities, exists := data["capabilities"]; exists {
            s.logger.Printf("Client capabilities: %+v", capabilities)
        }
    }
    
    return nil
}

func (s *SignalingServer) handleRequestOffer(client *SignalingClient, message *SignalingMessage) error {
    s.logger.Printf("Offer requested by client %s", client.ID)
    
    // 这里应该创建 WebRTC offer
    // 示例中返回模拟的 offer
    offer := SignalingMessage{
        Version:   "1.0",
        Type:      "offer",
        ID:        s.generateMessageID(),
        Timestamp: time.Now().Unix(),
        PeerID:    client.ID,
        Data: map[string]interface{}{
            "sdp": map[string]interface{}{
                "type": "offer",
                "sdp":  "v=0\r\no=- 123456789 2 IN IP4 127.0.0.1\r\n...", // 模拟 SDP
            },
        },
    }
    
    return client.sendMessage(offer)
}

func (s *SignalingServer) handleAnswer(client *SignalingClient, message *SignalingMessage) error {
    s.logger.Printf("Answer received from client %s", client.ID)
    
    // 处理 SDP answer
    if data, ok := message.Data.(map[string]interface{}); ok {
        if sdp, exists := data["sdp"]; exists {
            s.logger.Printf("SDP answer: %+v", sdp)
            // 这里应该处理 SDP answer
        }
    }
    
    // 发送确认
    ack := SignalingMessage{
        Version:   "1.0",
        Type:      "answer-ack",
        ID:        s.generateMessageID(),
        Timestamp: time.Now().Unix(),
        PeerID:    client.ID,
    }
    
    return client.sendMessage(ack)
}

func (s *SignalingServer) handleIceCandidate(client *SignalingClient, message *SignalingMessage) error {
    s.logger.Printf("ICE candidate received from client %s", client.ID)
    
    // 处理 ICE candidate
    if data, ok := message.Data.(map[string]interface{}); ok {
        if candidate, exists := data["candidate"]; exists {
            s.logger.Printf("ICE candidate: %+v", candidate)
            // 这里应该处理 ICE candidate
        }
    }
    
    return nil
}

func (s *SignalingServer) handleMouseClick(client *SignalingClient, message *SignalingMessage) error {
    s.logger.Printf("Mouse click from client %s: %+v", client.ID, message.Data)
    // 处理鼠标点击事件
    return nil
}

func (s *SignalingServer) handleMouseMove(client *SignalingClient, message *SignalingMessage) error {
    // 处理鼠标移动事件（通常不记录日志以避免过多输出）
    return nil
}

func (s *SignalingServer) handleKeyPress(client *SignalingClient, message *SignalingMessage) error {
    s.logger.Printf("Key press from client %s: %+v", client.ID, message.Data)
    // 处理键盘事件
    return nil
}

func (s *SignalingServer) handleGetStats(client *SignalingClient, message *SignalingMessage) error {
    s.logger.Printf("Stats requested by client %s", client.ID)
    
    // 生成模拟统计数据
    stats := SignalingMessage{
        Version:   "1.0",
        Type:      "stats",
        ID:        s.generateMessageID(),
        Timestamp: time.Now().Unix(),
        PeerID:    client.ID,
        Data: map[string]interface{}{
            "webrtc": map[string]interface{}{
                "bytes_sent":     1048576,
                "bytes_received": 2097152,
                "packets_sent":   1024,
                "packets_received": 2048,
                "rtt":            0.05,
            },
            "system": map[string]interface{}{
                "cpu_usage":    25.5,
                "memory_usage": 512000000,
                "fps":          60,
            },
        },
    }
    
    return client.sendMessage(stats)
}

func (s *SignalingServer) handleProtocolNegotiation(client *SignalingClient, message *SignalingMessage) error {
    s.logger.Printf("Protocol negotiation from client %s", client.ID)
    
    // 选择协议
    selectedProtocol := s.config.DefaultProtocol
    if data, ok := message.Data.(map[string]interface{}); ok {
        if supportedProtocols, exists := data["supported_protocols"]; exists {
            if protocols, ok := supportedProtocols.([]interface{}); ok {
                for _, protocol := range protocols {
                    if protocolStr, ok := protocol.(string); ok {
                        if s.isProtocolSupported(protocolStr) {
                            selectedProtocol = protocolStr
                            break
                        }
                    }
                }
            }
        }
    }
    
    client.Protocol = selectedProtocol
    
    response := SignalingMessage{
        Version:   "1.0",
        Type:      "protocol-selected",
        ID:        s.generateMessageID(),
        Timestamp: time.Now().Unix(),
        PeerID:    client.ID,
        Data: map[string]interface{}{
            "selected_protocol":   selectedProtocol,
            "protocol_version":    "1.0",
            "server_capabilities": []string{"webrtc", "input", "stats"},
        },
    }
    
    return client.sendMessage(response)
}

// 辅助方法

func (s *SignalingServer) generateClientID() string {
    return fmt.Sprintf("client_%d_%d", time.Now().UnixNano(), 
        time.Now().Nanosecond()%10000)
}

func (s *SignalingServer) generateMessageID() string {
    return fmt.Sprintf("msg_%d_%d", time.Now().UnixNano(), 
        time.Now().Nanosecond()%1000)
}

func (s *SignalingServer) detectClientProtocol(client *SignalingClient, r *http.Request) {
    // 从请求头检测协议
    if protocol := r.Header.Get("X-Signaling-Protocol"); protocol != "" {
        if s.isProtocolSupported(protocol) {
            client.Protocol = protocol
        }
    }
    
    // 从 User-Agent 检测 selkies 客户端
    if userAgent := r.Header.Get("User-Agent"); userAgent != "" {
        if contains(userAgent, "selkies") {
            client.Protocol = "selkies"
            client.IsSelkies = true
        }
    }
}

func (s *SignalingServer) isProtocolSupported(protocol string) bool {
    for _, supported := range s.config.SupportedProtocols {
        if supported == protocol {
            return true
        }
    }
    return false
}

func (s *SignalingServer) registerAdapters() {
    // 注册标准协议适配器
    s.adapters["gstreamer-1.0"] = NewGStreamerProtocolAdapter()
    
    // 注册 Selkies 兼容适配器
    s.adapters["selkies"] = NewSelkiesProtocolAdapter()
}

func (s *SignalingServer) cleanupRoutine() {
    ticker := time.NewTicker(s.config.CleanupInterval)
    defer ticker.Stop()
    
    for s.running {
        select {
        case <-ticker.C:
            s.cleanupExpiredClients()
        case <-s.ctx.Done():
            return
        }
    }
}

func (s *SignalingServer) cleanupExpiredClients() {
    s.mutex.Lock()
    defer s.mutex.Unlock()
    
    now := time.Now()
    expiredClients := make([]*SignalingClient, 0)
    
    for _, client := range s.clients {
        if now.Sub(client.LastSeen) > s.config.ConnectionTimeout {
            expiredClients = append(expiredClients, client)
        }
    }
    
    for _, client := range expiredClients {
        s.logger.Printf("Cleaning up expired client: %s", client.ID)
        s.unregisterClient(client)
    }
}

// 辅助函数
func contains(s, substr string) bool {
    return len(s) >= len(substr) && s[len(s)-len(substr):] == substr
}

// 使用示例
func main() {
    config := &SignalingConfig{
        Host:              "localhost",
        Port:              8080,
        MaxConnections:    100,
        ConnectionTimeout: 5 * time.Minute,
        HeartbeatInterval: 30 * time.Second,
        CleanupInterval:   2 * time.Minute,
        MaxMessageSize:    65536,
        SupportedProtocols: []string{"gstreamer-1.0", "selkies"},
        DefaultProtocol:   "gstreamer-1.0",
    }
    
    ctx := context.Background()
    server := NewSignalingServer(ctx, config)
    
    if err := server.Start(); err != nil {
        log.Fatalf("Failed to start server: %v", err)
    }
}
```

## WebRTC 集成示例

### 完整的 WebRTC 客户端集成

```javascript
/**
 * WebRTC 管理器，集成信令客户端
 */
class WebRTCManager {
  constructor(signalingClient, videoElement) {
    this.signaling = signalingClient;
    this.videoElement = videoElement;
    this.peerConnection = null;
    this.dataChannel = null;
    
    // 绑定信令事件
    this.setupSignalingHandlers();
    
    // WebRTC 配置
    this.rtcConfig = {
      iceServers: [
        { urls: 'stun:stun.l.google.com:19302' }
      ]
    };
  }
  
  setupSignalingHandlers() {
    this.signaling.onsdp = this.handleRemoteSDP.bind(this);
    this.signaling.onice = this.handleRemoteICE.bind(this);
    
    this.signaling.eventBus.addEventListener('welcome', (event) => {
      // 服务器欢迎消息，可以开始 WebRTC 协商
      this.requestOffer();
    });
  }
  
  async requestOffer() {
    try {
      // 创建 PeerConnection
      await this.createPeerConnection();
      
      // 请求服务器发送 offer
      this.signaling.sendMessage('request-offer', {
        constraints: {
          video: true,
          audio: false,
          data_channel: true
        }
      });
      
    } catch (error) {
      console.error('Failed to request offer:', error);
    }
  }
  
  async createPeerConnection() {
    this.peerConnection = new RTCPeerConnection(this.rtcConfig);
    
    // 处理 ICE 候选
    this.peerConnection.onicecandidate = (event) => {
      if (event.candidate) {
        this.signaling.sendICE(event.candidate);
      }
    };
    
    // 处理远程媒体流
    this.peerConnection.ontrack = (event) => {
      console.log('Remote track received:', event.track.kind);
      if (event.streams && event.streams[0]) {
        this.videoElement.srcObject = event.streams[0];
        this.playVideo();
      }
    };
    
    // 处理连接状态变化
    this.peerConnection.onconnectionstatechange = () => {
      console.log('Connection state:', this.peerConnection.connectionState);
    };
    
    // 处理数据通道
    this.peerConnection.ondatachannel = (event) => {
      this.dataChannel = event.channel;
      this.setupDataChannel();
    };
  }
  
  async handleRemoteSDP(sdp) {
    try {
      console.log('Received remote SDP:', sdp.type);
      
      await this.peerConnection.setRemoteDescription(sdp);
      
      if (sdp.type === 'offer') {
        // 创建 answer
        const answer = await this.peerConnection.createAnswer();
        await this.peerConnection.setLocalDescription(answer);
        
        // 发送 answer
        this.signaling.sendSDP(answer);
      }
      
    } catch (error) {
      console.error('Failed to handle remote SDP:', error);
    }
  }
  
  async handleRemoteICE(candidate) {
    try {
      await this.peerConnection.addIceCandidate(candidate);
      console.log('Added ICE candidate');
    } catch (error) {
      console.error('Failed to add ICE candidate:', error);
    }
  }
  
  setupDataChannel() {
    this.dataChannel.onopen = () => {
      console.log('Data channel opened');
    };
    
    this.dataChannel.onmessage = (event) => {
      console.log('Data channel message:', event.data);
      this.handleDataChannelMessage(event.data);
    };
    
    this.dataChannel.onclose = () => {
      console.log('Data channel closed');
    };
  }
  
  handleDataChannelMessage(data) {
    try {
      const message = JSON.parse(data);
      
      switch (message.type) {
        case 'cursor':
          this.handleCursorUpdate(message.data);
          break;
        case 'system_stats':
          this.handleSystemStats(message.data);
          break;
        default:
          console.log('Unknown data channel message:', message);
      }
    } catch (error) {
      console.error('Failed to parse data channel message:', error);
    }
  }
  
  handleCursorUpdate(data) {
    // 处理光标更新
    console.log('Cursor update:', data);
  }
  
  handleSystemStats(data) {
    // 处理系统统计信息
    console.log('System stats:', data);
  }
  
  async playVideo() {
    try {
      await this.videoElement.play();
      console.log('Video playback started');
    } catch (error) {
      console.error('Failed to play video:', error);
    }
  }
  
  // 输入事件处理
  
  sendMouseClick(x, y, button = 'left') {
    this.signaling.sendInputEvent('mouse-click', {
      x: x,
      y: y,
      button: button,
      action: 'down'
    });
  }
  
  sendMouseMove(x, y) {
    this.signaling.sendInputEvent('mouse-move', {
      x: x,
      y: y
    });
  }
  
  sendKeyPress(key, code, action = 'down') {
    this.signaling.sendInputEvent('key-press', {
      key: key,
      code: code,
      action: action
    });
  }
  
  // 统计信息
  
  async getStats() {
    try {
      const stats = await this.signaling.requestStats(['webrtc', 'system']);
      return stats;
    } catch (error) {
      console.error('Failed to get stats:', error);
      return null;
    }
  }
  
  async getConnectionStats() {
    if (!this.peerConnection) return null;
    
    try {
      const stats = await this.peerConnection.getStats();
      const result = {};
      
      stats.forEach((report) => {
        result[report.id] = report;
      });
      
      return result;
    } catch (error) {
      console.error('Failed to get connection stats:', error);
      return null;
    }
  }
  
  // 清理资源
  
  close() {
    if (this.dataChannel) {
      this.dataChannel.close();
      this.dataChannel = null;
    }
    
    if (this.peerConnection) {
      this.peerConnection.close();
      this.peerConnection = null;
    }
    
    if (this.videoElement) {
      this.videoElement.srcObject = null;
    }
  }
}

// 使用示例
async function initializeWebRTC() {
  // 创建视频元素
  const videoElement = document.getElementById('remoteVideo');
  
  // 创建信令客户端
  const signalingClient = new GStreamerSignalingClient({
    serverUrl: 'ws://localhost:8080/ws',
    peerId: Math.floor(Math.random() * 1000000)
  });
  
  // 创建 WebRTC 管理器
  const webrtcManager = new WebRTCManager(signalingClient, videoElement);
  
  // 设置输入事件处理
  videoElement.addEventListener('click', (event) => {
    const rect = videoElement.getBoundingClientRect();
    const x = event.clientX - rect.left;
    const y = event.clientY - rect.top;
    webrtcManager.sendMouseClick(x, y, 'left');
  });
  
  videoElement.addEventListener('mousemove', (event) => {
    const rect = videoElement.getBoundingClientRect();
    const x = event.clientX - rect.left;
    const y = event.clientY - rect.top;
    webrtcManager.sendMouseMove(x, y);
  });
  
  document.addEventListener('keydown', (event) => {
    webrtcManager.sendKeyPress(event.key, event.code, 'down');
  });
  
  document.addEventListener('keyup', (event) => {
    webrtcManager.sendKeyPress(event.key, event.code, 'up');
  });
  
  // 连接到信令服务器
  try {
    await signalingClient.connect();
    console.log('Connected to signaling server');
  } catch (error) {
    console.error('Failed to connect:', error);
  }
  
  return { signalingClient, webrtcManager };
}

// 页面加载完成后初始化
document.addEventListener('DOMContentLoaded', () => {
  initializeWebRTC().then(({ signalingClient, webrtcManager }) => {
    console.log('WebRTC initialized successfully');
    
    // 定期获取统计信息
    setInterval(async () => {
      const stats = await webrtcManager.getStats();
      if (stats) {
        console.log('Stats:', stats);
      }
    }, 5000);
  }).catch(error => {
    console.error('Failed to initialize WebRTC:', error);
  });
});
```

## 测试用例示例

### 单元测试

```javascript
// Jest 测试用例
describe('GStreamerSignalingClient', () => {
  let client;
  let mockWebSocket;
  
  beforeEach(() => {
    // Mock WebSocket
    mockWebSocket = {
      send: jest.fn(),
      close: jest.fn(),
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      readyState: WebSocket.CONNECTING
    };
    
    global.WebSocket = jest.fn(() => mockWebSocket);
    
    client = new GStreamerSignalingClient({
      serverUrl: 'ws://localhost:8080/ws',
      peerId: 1
    });
  });
  
  afterEach(() => {
    if (client) {
      client.disconnect();
    }
  });
  
  test('should create client with default configuration', () => {
    expect(client.peerId).toBe(1);
    expect(client.serverUrl).toBe('ws://localhost:8080/ws');
    expect(client._state).toBe('disconnected');
  });
  
  test('should connect to server', async () => {
    const connectPromise = client.connect();
    
    // 模拟连接打开
    mockWebSocket.readyState = WebSocket.OPEN;
    const openHandler = mockWebSocket.addEventListener.mock.calls
      .find(call => call[0] === 'open')[1];
    openHandler();
    
    await connectPromise;
    
    expect(client._state).toBe('connected');
    expect(mockWebSocket.send).toHaveBeenCalled();
  });
  
  test('should send standard message', () => {
    client._state = 'connected';
    client._ws = mockWebSocket;
    
    const messageId = client.sendMessage('ping', { test: 'data' });
    
    expect(mockWebSocket.send).toHaveBeenCalledWith(
      expect.stringContaining('"type":"ping"')
    );
    expect(messageId).toMatch(/^msg_\d+_\d+$/);
  });
  
  test('should handle SDP message', () => {
    const mockSDP = {
      type: 'offer',
      sdp: 'v=0\r\no=- 123456789 2 IN IP4 127.0.0.1\r\n'
    };
    
    const onsdpSpy = jest.fn();
    client.onsdp = onsdpSpy;
    
    client._handleOffer({
      type: 'offer',
      data: { sdp: mockSDP }
    });
    
    expect(onsdpSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'offer',
        sdp: mockSDP.sdp
      })
    );
  });
  
  test('should handle connection error and retry', () => {
    const onerrorSpy = jest.fn();
    client.onerror = onerrorSpy;
    
    client._handleConnectionError(new Error('Connection failed'));
    
    expect(onerrorSpy).toHaveBeenCalled();
    expect(client._retryCount).toBe(1);
  });
  
  test('should detect Selkies protocol', () => {
    const selkiesMessage = { sdp: { type: 'offer', sdp: 'test' } };
    
    client._detectProtocolMode(selkiesMessage);
    
    expect(client._selkiesMode).toBe(true);
    expect(client._protocolMode).toBe('selkies');
  });
});

// 集成测试
describe('SignalingClient Integration', () => {
  let server;
  let client;
  
  beforeAll(async () => {
    // 启动测试服务器
    server = new TestSignalingServer(8081);
    await server.start();
  });
  
  afterAll(async () => {
    if (server) {
      await server.stop();
    }
  });
  
  beforeEach(() => {
    client = new GStreamerSignalingClient({
      serverUrl: 'ws://localhost:8081/ws',
      peerId: Math.floor(Math.random() * 1000000)
    });
  });
  
  afterEach(() => {
    if (client) {
      client.disconnect();
    }
  });
  
  test('should establish connection with real server', async () => {
    const connectPromise = client.connect();
    
    await expect(connectPromise).resolves.toBeUndefined();
    expect(client.getConnectionState().isConnected).toBe(true);
  });
  
  test('should exchange messages with server', async () => {
    await client.connect();
    
    const response = await client.requestStats(['webrtc']);
    
    expect(response).toBeDefined();
    expect(response.data).toBeDefined();
  });
  
  test('should handle server disconnect gracefully', async () => {
    await client.connect();
    
    const disconnectPromise = new Promise(resolve => {
      client.onclose = resolve;
    });
    
    // 服务器主动断开连接
    server.disconnectClient(client.peerId);
    
    await disconnectPromise;
    expect(client.getConnectionState().isConnected).toBe(false);
  });
});
```

### 性能测试

```javascript
// 性能测试用例
describe('Performance Tests', () => {
  test('should handle high message throughput', async () => {
    const client = new GStreamerSignalingClient({
      serverUrl: 'ws://localhost:8081/ws'
    });
    
    await client.connect();
    
    const messageCount = 1000;
    const startTime = Date.now();
    
    const promises = [];
    for (let i = 0; i < messageCount; i++) {
      promises.push(
        client.sendMessage('ping', { sequence: i })
      );
    }
    
    await Promise.all(promises);
    
    const endTime = Date.now();
    const duration = endTime - startTime;
    const messagesPerSecond = messageCount / (duration / 1000);
    
    console.log(`Throughput: ${messagesPerSecond.toFixed(2)} messages/second`);
    expect(messagesPerSecond).toBeGreaterThan(100);
  });
  
  test('should handle concurrent connections', async () => {
    const clientCount = 50;
    const clients = [];
    
    // 创建多个客户端
    for (let i = 0; i < clientCount; i++) {
      clients.push(new GStreamerSignalingClient({
        serverUrl: 'ws://localhost:8081/ws',
        peerId: i + 1
      }));
    }
    
    const startTime = Date.now();
    
    // 并发连接
    const connectPromises = clients.map(client => client.connect());
    await Promise.all(connectPromises);
    
    const connectTime = Date.now() - startTime;
    
    // 发送消息
    const messagePromises = clients.map(client => 
      client.sendMessage('ping', { test: 'concurrent' })
    );
    await Promise.all(messagePromises);
    
    const totalTime = Date.now() - startTime;
    
    console.log(`Connected ${clientCount} clients in ${connectTime}ms`);
    console.log(`Total test time: ${totalTime}ms`);
    
    // 清理
    await Promise.all(clients.map(client => client.disconnect()));
    
    expect(connectTime).toBeLessThan(5000); // 5秒内完成连接
  });
});
```

这些示例代码提供了完整的 GStreamer WebRTC 信令协议实现，包括客户端、服务端、WebRTC 集成和测试用例，可以作为实际项目开发的参考和起点。
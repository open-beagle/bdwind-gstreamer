# GStreamer WebRTC 信令服务端规范

## 概述

本文档详细说明了 Golang 服务端 GStreamer WebRTC 信令服务器的实现要求，包括必需的 API 端点、业务逻辑处理流程、连接管理机制和资源管理。

## 核心架构

### SignalingServer 结构

```go
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
    protocolAdapters map[string]ProtocolAdapter

    // WebRTC 管理
    peerConnectionManager *PeerConnectionManager
    sdpGenerator         *SDPGenerator

    // 配置和状态
    config  *SignalingConfig
    ctx     context.Context
    cancel  context.CancelFunc
    running bool
}

type SignalingConfig struct {
    // 服务器配置
    Host string `yaml:"host" json:"host"`
    Port int    `yaml:"port" json:"port"`

    // WebSocket 配置
    ReadBufferSize  int           `yaml:"read_buffer_size" json:"read_buffer_size"`
    WriteBufferSize int           `yaml:"write_buffer_size" json:"write_buffer_size"`
    CheckOrigin     bool          `yaml:"check_origin" json:"check_origin"`

    // 连接管理
    MaxConnections    int           `yaml:"max_connections" json:"max_connections"`
    ConnectionTimeout time.Duration `yaml:"connection_timeout" json:"connection_timeout"`
    HeartbeatInterval time.Duration `yaml:"heartbeat_interval" json:"heartbeat_interval"`
    CleanupInterval   time.Duration `yaml:"cleanup_interval" json:"cleanup_interval"`

    // 消息配置
    MaxMessageSize int `yaml:"max_message_size" json:"max_message_size"`
    MaxDataSize    int `yaml:"max_data_size" json:"max_data_size"`

    // WebRTC 配置
    ICEServers []webrtc.ICEServer `yaml:"ice_servers" json:"ice_servers"`

    // 协议配置
    SupportedProtocols []string `yaml:"supported_protocols" json:"supported_protocols"`
    DefaultProtocol    string   `yaml:"default_protocol" json:"default_protocol"`
}
```

## 必需实现的 API 端点

### 1. WebSocket 连接端点

#### `/api/signaling` - 通用信令连接

```go
func (s *SignalingServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
    // 升级 HTTP 连接为 WebSocket
    conn, err := s.upgrader.Upgrade(w, r, nil)
    if err != nil {
        s.logger.Error("WebSocket upgrade failed", "error", err)
        return
    }

    // 创建客户端实例
    client := s.createClient(conn, r)

    // 启动客户端处理协程
    go client.readPump()
    go client.writePump()

    // 注册客户端
    s.register <- client

    // 发送欢迎消息
    s.sendWelcomeMessage(client)
}
```

#### `/api/signaling/{app}` - 应用特定信令连接

```go
func (s *SignalingServer) HandleAppWebSocket(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    appName := vars["app"]

    if !s.isValidAppName(appName) {
        http.Error(w, "Invalid app name", http.StatusBadRequest)
        return
    }

    conn, err := s.upgrader.Upgrade(w, r, nil)
    if err != nil {
        s.logger.Error("WebSocket upgrade failed", "error", err, "app", appName)
        return
    }

    client := s.createAppClient(conn, r, appName)

    go client.readPump()
    go client.writePump()

    s.register <- client
    s.sendWelcomeMessage(client)
}
```

### 2. REST API 端点

#### `GET /api/signaling/status` - 服务器状态

```go
func (s *SignalingServer) HandleStatus(w http.ResponseWriter, r *http.Request) {
    status := SignalingStatus{
        Running:        s.running,
        ClientCount:    s.GetClientCount(),
        AppCount:       len(s.apps),
        SupportedProtocols: s.config.SupportedProtocols,
        Uptime:         time.Since(s.startTime),
        Version:        s.version,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(status)
}
```

#### `GET /api/signaling/clients` - 客户端列表

```go
func (s *SignalingServer) HandleClients(w http.ResponseWriter, r *http.Request) {
    s.mutex.RLock()
    defer s.mutex.RUnlock()

    clients := make([]ClientInfo, 0, len(s.clients))
    for _, client := range s.clients {
        clients = append(clients, ClientInfo{
            ID:           client.ID,
            AppName:      client.AppName,
            RemoteAddr:   client.RemoteAddr,
            ConnectedAt:  client.ConnectedAt,
            LastSeen:     client.LastSeen,
            State:        client.State,
            MessageCount: client.MessageCount,
            ErrorCount:   client.ErrorCount,
        })
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(clients)
}
```

#### `POST /api/signaling/broadcast` - 广播消息

```go
func (s *SignalingServer) HandleBroadcast(w http.ResponseWriter, r *http.Request) {
    var req BroadcastRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    message := SignalingMessage{
        Type:      req.Type,
        Data:      req.Data,
        MessageID: generateMessageID(),
        Timestamp: time.Now().Unix(),
    }

    if req.AppName != "" {
        s.BroadcastToApp(req.AppName, message)
    } else {
        s.BroadcastToAll(message)
    }

    w.WriteHeader(http.StatusOK)
}
```

## 业务逻辑处理流程

### 1. 客户端连接处理

```go
func (s *SignalingServer) createClient(conn *websocket.Conn, r *http.Request) *SignalingClient {
    client := &SignalingClient{
        ID:           generateClientID(),
        AppName:      "default",
        Conn:         conn,
        Send:         make(chan []byte, 256),
        Server:       s,
        LastSeen:     time.Now(),
        ConnectedAt:  time.Now(),
        RemoteAddr:   conn.RemoteAddr().String(),
        UserAgent:    r.Header.Get("User-Agent"),
        State:        ClientStateConnecting,
        MessageCount: 0,
        ErrorCount:   0,
        Protocol:     s.config.DefaultProtocol,
    }

    // 检测客户端协议偏好
    s.detectClientProtocol(client, r)

    return client
}

func (s *SignalingServer) detectClientProtocol(client *SignalingClient, r *http.Request) {
    // 从请求头检测协议偏好
    if protocol := r.Header.Get("X-Signaling-Protocol"); protocol != "" {
        if s.isSupportedProtocol(protocol) {
            client.Protocol = protocol
        }
    }

    // 从 User-Agent 检测 selkies 客户端
    if strings.Contains(client.UserAgent, "selkies") {
        client.Protocol = "selkies"
        client.IsSelkies = true
    }
}
```

### 2. 消息处理流程

```go
func (c *SignalingClient) handleMessage(messageBytes []byte) {
    // 更新最后活跃时间
    c.LastSeen = time.Now()
    c.incrementMessageCount()

    // 尝试处理 selkies 协议消息
    messageText := string(messageBytes)
    if c.handleSelkiesMessage(messageText) {
        return
    }

    // 解析标准 JSON 消息
    var message SignalingMessage
    if err := json.Unmarshal(messageBytes, &message); err != nil {
        c.sendError(&SignalingError{
            Code:    ErrorCodeMessageParsingFailed,
            Message: "Failed to parse JSON message",
            Details: err.Error(),
        })
        return
    }

    // 验证消息
    if err := c.validateMessage(&message); err != nil {
        c.sendError(err)
        return
    }

    // 路由消息到处理器
    c.routeMessage(&message)
}

func (c *SignalingClient) routeMessage(message *SignalingMessage) {
    handler, exists := c.messageHandlers[message.Type]
    if !exists {
        c.sendError(&SignalingError{
            Code:    ErrorCodeInvalidMessageType,
            Message: fmt.Sprintf("Unknown message type: %s", message.Type),
        })
        return
    }

    if err := handler(message); err != nil {
        c.sendError(&SignalingError{
            Code:    ErrorCodeInternalError,
            Message: "Message processing failed",
            Details: err.Error(),
        })
    }
}
```

### 3. WebRTC 协商处理

```go
func (c *SignalingClient) handleOfferRequest(message *SignalingMessage) error {
    // 创建 WebRTC offer
    offer, err := c.Server.peerConnectionManager.CreateOffer(c.ID)
    if err != nil {
        return fmt.Errorf("failed to create offer: %w", err)
    }

    // 发送 offer 给客户端
    response := SignalingMessage{
        Type:      "offer",
        PeerID:    c.ID,
        MessageID: generateMessageID(),
        Timestamp: time.Now().Unix(),
        Data: map[string]interface{}{
            "sdp": offer,
        },
    }

    return c.sendMessage(response)
}

func (c *SignalingClient) handleAnswer(message *SignalingMessage) error {
    answerData, ok := message.Data.(map[string]interface{})
    if !ok {
        return errors.New("invalid answer data format")
    }

    sdpData, exists := answerData["sdp"]
    if !exists {
        return errors.New("missing SDP in answer")
    }

    // 处理 SDP answer
    err := c.Server.peerConnectionManager.HandleAnswer(c.ID, sdpData)
    if err != nil {
        return fmt.Errorf("failed to handle answer: %w", err)
    }

    // 发送确认
    ack := SignalingMessage{
        Type:      "answer-ack",
        PeerID:    c.ID,
        MessageID: generateMessageID(),
        Timestamp: time.Now().Unix(),
    }

    return c.sendMessage(ack)
}

func (c *SignalingClient) handleIceCandidate(message *SignalingMessage) error {
    candidateData, ok := message.Data.(map[string]interface{})
    if !ok {
        return errors.New("invalid ICE candidate data format")
    }

    // 处理 ICE candidate
    err := c.Server.peerConnectionManager.HandleICECandidate(c.ID, candidateData)
    if err != nil {
        return fmt.Errorf("failed to handle ICE candidate: %w", err)
    }

    // 发送确认
    ack := SignalingMessage{
        Type:      "ice-ack",
        PeerID:    c.ID,
        MessageID: generateMessageID(),
        Timestamp: time.Now().Unix(),
    }

    return c.sendMessage(ack)
}
```

## 连接管理机制

### 1. 客户端注册和注销

```go
func (s *SignalingServer) registerClient(client *SignalingClient) {
    s.mutex.Lock()
    defer s.mutex.Unlock()

    // 检查连接数限制
    if len(s.clients) >= s.config.MaxConnections {
        client.sendError(&SignalingError{
            Code:    ErrorCodeServerUnavailable,
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

    // 更新状态
    client.setState(ClientStateConnected)

    s.logger.Info("Client registered",
        "clientID", client.ID,
        "appName", client.AppName,
        "remoteAddr", client.RemoteAddr,
        "protocol", client.Protocol,
        "totalClients", len(s.clients))
}

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

        // 关闭发送通道
        close(client.Send)

        // 清理 WebRTC 连接
        if s.peerConnectionManager != nil {
            s.peerConnectionManager.RemovePeerConnection(client.ID)
        }

        connectionDuration := time.Since(client.ConnectedAt)

        s.logger.Info("Client unregistered",
            "clientID", client.ID,
            "appName", client.AppName,
            "connectionDuration", connectionDuration,
            "messageCount", client.MessageCount,
            "errorCount", client.ErrorCount,
            "remainingClients", len(s.clients))
    }
}
```

### 2. 连接状态监控

```go
func (s *SignalingServer) startMonitoring() {
    ticker := time.NewTicker(s.config.CleanupInterval)
    defer ticker.Stop()

    for s.running {
        select {
        case <-ticker.C:
            s.cleanupExpiredClients()
            s.collectMetrics()

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
        s.logger.Warn("Cleaning up expired client",
            "clientID", client.ID,
            "lastSeen", client.LastSeen,
            "inactiveFor", now.Sub(client.LastSeen))

        client.recordError(&SignalingError{
            Code:    ErrorCodeConnectionTimeout,
            Message: "Connection expired due to inactivity",
            Details: fmt.Sprintf("Last seen %v ago", now.Sub(client.LastSeen)),
        })

        s.unregisterClient(client)
    }
}
```

## 协议适配器实现

### 1. 协议适配器接口

```go
type ProtocolAdapter interface {
    // 协议检测
    DetectProtocol(message []byte) bool

    // 消息转换
    ParseMessage(data []byte) (*StandardMessage, error)
    FormatMessage(msg *StandardMessage) ([]byte, error)

    // 消息验证
    ValidateMessage(msg *StandardMessage) error

    // 协议信息
    GetProtocolName() string
    GetProtocolVersion() string
}

type StandardMessage struct {
    Version   string      `json:"version"`
    Type      string      `json:"type"`
    ID        string      `json:"id"`
    Timestamp int64       `json:"timestamp"`
    PeerID    string      `json:"peer_id"`
    Data      interface{} `json:"data"`
    Error     *SignalingError `json:"error,omitempty"`
}
```

### 2. GStreamer 标准协议适配器

```go
type GStreamerProtocolAdapter struct {
    version      string
    capabilities []string
}

func NewGStreamerProtocolAdapter() *GStreamerProtocolAdapter {
    return &GStreamerProtocolAdapter{
        version:      "1.0",
        capabilities: []string{"webrtc", "datachannel", "input", "stats"},
    }
}

func (g *GStreamerProtocolAdapter) DetectProtocol(message []byte) bool {
    var msg map[string]interface{}
    if err := json.Unmarshal(message, &msg); err != nil {
        return false
    }

    // 检查是否包含版本字段和标准消息结构
    if version, exists := msg["version"]; exists {
        if versionStr, ok := version.(string); ok && versionStr == g.version {
            return true
        }
    }

    // 检查是否包含标准字段
    _, hasType := msg["type"]
    _, hasID := msg["id"]
    _, hasTimestamp := msg["timestamp"]

    return hasType && hasID && hasTimestamp
}

func (g *GStreamerProtocolAdapter) ParseMessage(data []byte) (*StandardMessage, error) {
    var msg StandardMessage
    if err := json.Unmarshal(data, &msg); err != nil {
        return nil, fmt.Errorf("failed to parse standard message: %w", err)
    }

    // 设置默认版本
    if msg.Version == "" {
        msg.Version = g.version
    }

    return &msg, nil
}

func (g *GStreamerProtocolAdapter) FormatMessage(msg *StandardMessage) ([]byte, error) {
    // 确保版本字段
    if msg.Version == "" {
        msg.Version = g.version
    }

    return json.Marshal(msg)
}
```

### 3. Selkies 兼容协议适配器

```go
type SelkiesProtocolAdapter struct {
    legacyMode bool
    mappings   map[string]string
}

func NewSelkiesProtocolAdapter() *SelkiesProtocolAdapter {
    return &SelkiesProtocolAdapter{
        legacyMode: true,
        mappings: map[string]string{
            "sdp": "offer",
            "ice": "ice-candidate",
        },
    }
}

func (s *SelkiesProtocolAdapter) DetectProtocol(message []byte) bool {
    messageText := string(message)

    // 检查 selkies 文本协议
    if strings.HasPrefix(messageText, "HELLO ") ||
       messageText == "HELLO" ||
       strings.HasPrefix(messageText, "ERROR") {
        return true
    }

    // 检查 selkies JSON 协议
    var msg map[string]interface{}
    if err := json.Unmarshal(message, &msg); err != nil {
        return false
    }

    // selkies 消息通常只包含 sdp 或 ice 字段
    _, hasSDP := msg["sdp"]
    _, hasICE := msg["ice"]

    return hasSDP || hasICE
}

func (s *SelkiesProtocolAdapter) ParseMessage(data []byte) (*StandardMessage, error) {
    messageText := string(data)

    // 处理文本协议
    if !strings.HasPrefix(messageText, "{") {
        return s.parseTextMessage(messageText)
    }

    // 处理 JSON 协议
    return s.parseJSONMessage(data)
}

func (s *SelkiesProtocolAdapter) parseTextMessage(text string) (*StandardMessage, error) {
    if strings.HasPrefix(text, "HELLO ") {
        parts := strings.SplitN(text, " ", 3)
        if len(parts) >= 2 {
            peerID := parts[1]
            var meta map[string]interface{}

            if len(parts) == 3 {
                if metaData, err := base64.StdEncoding.DecodeString(parts[2]); err == nil {
                    json.Unmarshal(metaData, &meta)
                }
            }

            return &StandardMessage{
                Version:   "selkies",
                Type:      "hello",
                ID:        generateMessageID(),
                Timestamp: time.Now().Unix(),
                PeerID:    peerID,
                Data:      meta,
            }, nil
        }
    }

    return nil, fmt.Errorf("unknown selkies text message: %s", text)
}

func (s *SelkiesProtocolAdapter) parseJSONMessage(data []byte) (*StandardMessage, error) {
    var msg map[string]interface{}
    if err := json.Unmarshal(data, &msg); err != nil {
        return nil, err
    }

    standardMsg := &StandardMessage{
        Version:   "selkies",
        ID:        generateMessageID(),
        Timestamp: time.Now().Unix(),
    }

    if sdp, exists := msg["sdp"]; exists {
        standardMsg.Type = "offer"
        standardMsg.Data = map[string]interface{}{"sdp": sdp}
    } else if ice, exists := msg["ice"]; exists {
        standardMsg.Type = "ice-candidate"
        standardMsg.Data = map[string]interface{}{"candidate": ice}
    } else {
        return nil, fmt.Errorf("unknown selkies JSON message format")
    }

    return standardMsg, nil
}
```

## 消息验证机制

### 1. 消息格式验证

```go
func validateSignalingMessage(message *SignalingMessage) *SignalingError {
    if message == nil {
        return &SignalingError{
            Code:    ErrorCodeInvalidMessage,
            Message: "Message is null",
        }
    }

    // 验证必需字段
    if message.Type == "" {
        return &SignalingError{
            Code:    ErrorCodeInvalidMessageType,
            Message: "Message type is required",
        }
    }

    // 验证消息大小
    if dataSize := getDataSize(message.Data); dataSize > MaxDataSize {
        return &SignalingError{
            Code:    ErrorCodeMessageTooLarge,
            Message: fmt.Sprintf("Message data too large: %d bytes (max: %d)", dataSize, MaxDataSize),
        }
    }

    // 验证特定消息类型
    return validateMessageByType(message)
}

func validateMessageByType(message *SignalingMessage) *SignalingError {
    switch message.Type {
    case "answer":
        return validateAnswerMessage(message)
    case "ice-candidate":
        return validateICECandidateMessage(message)
    case "mouse-click", "mouse-move":
        return validateMouseEventMessage(message)
    case "key-press":
        return validateKeyEventMessage(message)
    default:
        // 允许未知消息类型，但记录警告
        return nil
    }
}
```

### 2. 内容验证

```go
func validateAnswerMessage(message *SignalingMessage) *SignalingError {
    data, ok := message.Data.(map[string]interface{})
    if !ok {
        return &SignalingError{
            Code:    ErrorCodeInvalidMessageData,
            Message: "Answer data must be an object",
        }
    }

    sdp, exists := data["sdp"]
    if !exists {
        return &SignalingError{
            Code:    ErrorCodeInvalidMessageData,
            Message: "Answer must contain 'sdp' field",
        }
    }

    // 验证 SDP 格式
    if err := validateSDP(sdp); err != nil {
        return &SignalingError{
            Code:    ErrorCodeSDPProcessingFailed,
            Message: "Invalid SDP format",
            Details: err.Error(),
        }
    }

    return nil
}

func validateSDP(sdp interface{}) error {
    sdpData, ok := sdp.(map[string]interface{})
    if !ok {
        return errors.New("SDP must be an object")
    }

    sdpType, exists := sdpData["type"]
    if !exists {
        return errors.New("SDP must contain 'type' field")
    }

    typeStr, ok := sdpType.(string)
    if !ok {
        return errors.New("SDP type must be a string")
    }

    if typeStr != "offer" && typeStr != "answer" {
        return fmt.Errorf("invalid SDP type: %s", typeStr)
    }

    sdpContent, exists := sdpData["sdp"]
    if !exists {
        return errors.New("SDP must contain 'sdp' field")
    }

    sdpStr, ok := sdpContent.(string)
    if !ok || sdpStr == "" {
        return errors.New("SDP content must be a non-empty string")
    }

    // 基本 SDP 格式验证
    if !strings.HasPrefix(sdpStr, "v=") {
        return errors.New("SDP must start with version line")
    }

    return nil
}
```

## 资源管理

### 1. 内存管理

```go
func (s *SignalingServer) collectMetrics() {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)

    metrics := ServerMetrics{
        ClientCount:     len(s.clients),
        AppCount:        len(s.apps),
        MemoryUsage:     m.Alloc,
        MemoryTotal:     m.TotalAlloc,
        GoroutineCount:  runtime.NumGoroutine(),
        Timestamp:       time.Now(),
    }

    // 记录指标
    s.logger.Debug("Server metrics", "metrics", metrics)

    // 检查内存使用
    if metrics.MemoryUsage > s.config.MaxMemoryUsage {
        s.logger.Warn("High memory usage detected", "usage", metrics.MemoryUsage)
        s.triggerGarbageCollection()
    }
}

func (s *SignalingServer) triggerGarbageCollection() {
    runtime.GC()
    s.logger.Info("Garbage collection triggered")
}
```

### 2. 连接池管理

```go
type ConnectionPool struct {
    connections chan *websocket.Conn
    maxSize     int
    mutex       sync.Mutex
}

func NewConnectionPool(maxSize int) *ConnectionPool {
    return &ConnectionPool{
        connections: make(chan *websocket.Conn, maxSize),
        maxSize:     maxSize,
    }
}

func (p *ConnectionPool) Get() *websocket.Conn {
    select {
    case conn := <-p.connections:
        return conn
    default:
        return nil
    }
}

func (p *ConnectionPool) Put(conn *websocket.Conn) bool {
    select {
    case p.connections <- conn:
        return true
    default:
        return false
    }
}
```

## 性能优化

### 1. 消息批处理

```go
type MessageBatcher struct {
    messages []SignalingMessage
    timer    *time.Timer
    mutex    sync.Mutex
    callback func([]SignalingMessage)
}

func NewMessageBatcher(batchSize int, timeout time.Duration, callback func([]SignalingMessage)) *MessageBatcher {
    return &MessageBatcher{
        messages: make([]SignalingMessage, 0, batchSize),
        callback: callback,
    }
}

func (b *MessageBatcher) Add(message SignalingMessage) {
    b.mutex.Lock()
    defer b.mutex.Unlock()

    b.messages = append(b.messages, message)

    if len(b.messages) >= cap(b.messages) {
        b.flush()
    } else if b.timer == nil {
        b.timer = time.AfterFunc(100*time.Millisecond, b.flush)
    }
}

func (b *MessageBatcher) flush() {
    if len(b.messages) > 0 {
        b.callback(b.messages)
        b.messages = b.messages[:0]
    }

    if b.timer != nil {
        b.timer.Stop()
        b.timer = nil
    }
}
```

### 2. 并发控制

```go
type ConcurrencyLimiter struct {
    semaphore chan struct{}
}

func NewConcurrencyLimiter(limit int) *ConcurrencyLimiter {
    return &ConcurrencyLimiter{
        semaphore: make(chan struct{}, limit),
    }
}

func (c *ConcurrencyLimiter) Acquire() {
    c.semaphore <- struct{}{}
}

func (c *ConcurrencyLimiter) Release() {
    <-c.semaphore
}

func (c *ConcurrencyLimiter) Do(fn func()) {
    c.Acquire()
    defer c.Release()
    fn()
}
```

## 监控和日志

### 1. 结构化日志

```go
type StructuredLogger struct {
    logger *slog.Logger
}

func (l *StructuredLogger) LogClientEvent(event string, client *SignalingClient, extra ...interface{}) {
    l.logger.Info(event,
        "clientID", client.ID,
        "appName", client.AppName,
        "remoteAddr", client.RemoteAddr,
        "protocol", client.Protocol,
        "messageCount", client.MessageCount,
        "errorCount", client.ErrorCount,
        "extra", extra)
}

func (l *StructuredLogger) LogMessageEvent(event string, message *SignalingMessage, client *SignalingClient) {
    l.logger.Debug(event,
        "messageType", message.Type,
        "messageID", message.MessageID,
        "clientID", client.ID,
        "dataSize", getDataSize(message.Data),
        "timestamp", message.Timestamp)
}
```

### 2. 性能指标

```go
type PerformanceMetrics struct {
    MessageProcessingTime time.Duration
    ConnectionCount       int
    MessageThroughput     float64
    ErrorRate            float64
    MemoryUsage          uint64
}

func (s *SignalingServer) recordMetrics() {
    metrics := s.calculateMetrics()

    // 发送到监控系统
    if s.metricsCollector != nil {
        s.metricsCollector.Record(metrics)
    }

    // 记录到日志
    s.logger.Info("Performance metrics", "metrics", metrics)
}
```

## 测试要求

### 1. 单元测试覆盖

- 消息解析和验证
- 协议适配器功能
- 客户端管理
- 错误处理

### 2. 集成测试

- WebSocket 连接处理
- 多客户端并发
- 协议兼容性
- 性能基准测试

### 3. 压力测试

- 大量并发连接
- 高频消息传输
- 内存泄漏检测
- 长时间稳定性测试

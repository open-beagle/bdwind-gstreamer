# GStreamer WebRTC 信令协议兼容性指南

## 概述

本文档提供了 GStreamer WebRTC 信令协议的兼容性指南，包括与现有系统的兼容性分析、迁移策略、最佳实践和故障排除方法。

## 协议版本兼容性

### 支持的协议版本

| 协议版本 | 状态 | 描述 | 兼容性 |
|---------|------|------|--------|
| GStreamer 1.0 | 当前 | 标准 GStreamer WebRTC 协议 | 完全支持 |
| Selkies | 兼容 | selkies-gstreamer 协议 | 向后兼容 |
| 自定义扩展 | 实验性 | 项目特定扩展 | 部分支持 |

### 版本协商机制

```javascript
// 客户端协议协商示例
const supportedProtocols = [
  'gstreamer-1.0',  // 首选协议
  'selkies'         // 备用协议
];

const negotiationResult = await signalingClient.negotiateProtocol(supportedProtocols);
console.log('Selected protocol:', negotiationResult.selected_protocol);
```

```go
// 服务端协议选择逻辑
func (s *SignalingServer) selectProtocol(clientProtocols []string) string {
    // 优先级顺序
    preferredOrder := []string{"gstreamer-1.0", "selkies"}
    
    for _, preferred := range preferredOrder {
        for _, clientProtocol := range clientProtocols {
            if preferred == clientProtocol {
                return preferred
            }
        }
    }
    
    // 默认协议
    return "gstreamer-1.0"
}
```

## Selkies 协议兼容性

### Selkies 消息格式支持

#### 文本协议兼容

```javascript
// Selkies HELLO 消息处理
function handleSelkiesHello(message) {
  if (message.startsWith('HELLO ')) {
    const parts = message.split(' ');
    const peerId = parts[1];
    const metaData = parts[2] ? JSON.parse(atob(parts[2])) : {};
    
    return {
      type: 'hello',
      peer_id: peerId,
      data: {
        client_info: {
          screen_resolution: metaData.res || '1920x1080',
          device_pixel_ratio: metaData.scale || 1
        }
      }
    };
  }
  return null;
}

// 标准协议到 Selkies 协议转换
function convertToSelkiesHello(standardMessage) {
  const meta = {
    res: standardMessage.data.client_info.screen_resolution,
    scale: standardMessage.data.client_info.device_pixel_ratio
  };
  
  return `HELLO ${standardMessage.peer_id} ${btoa(JSON.stringify(meta))}`;
}
```

#### JSON 协议兼容

```javascript
// Selkies SDP 消息转换
function convertSelkiesSDP(selkiesMessage, standardFormat = false) {
  if (standardFormat) {
    // Selkies 到标准格式
    return {
      version: '1.0',
      type: 'offer',
      id: generateMessageId(),
      timestamp: Date.now(),
      data: {
        sdp: selkiesMessage.sdp
      }
    };
  } else {
    // 标准格式到 Selkies
    return {
      sdp: standardMessage.data.sdp
    };
  }
}

// Selkies ICE 候选转换
function convertSelkiesICE(selkiesMessage, standardFormat = false) {
  if (standardFormat) {
    return {
      version: '1.0',
      type: 'ice-candidate',
      id: generateMessageId(),
      timestamp: Date.now(),
      data: {
        candidate: selkiesMessage.ice
      }
    };
  } else {
    return {
      ice: standardMessage.data.candidate
    };
  }
}
```

### 服务端 Selkies 兼容实现

```go
// Selkies 协议适配器
type SelkiesProtocolAdapter struct {
    legacyMode bool
    logger     *log.Logger
}

func (s *SelkiesProtocolAdapter) DetectProtocol(message []byte) bool {
    messageText := string(message)
    
    // 检测 Selkies 文本协议
    if strings.HasPrefix(messageText, "HELLO ") ||
       messageText == "HELLO" ||
       strings.HasPrefix(messageText, "ERROR") {
        return true
    }
    
    // 检测 Selkies JSON 协议
    var msg map[string]interface{}
    if err := json.Unmarshal(message, &msg); err != nil {
        return false
    }
    
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
            var clientInfo map[string]interface{}
            
            if len(parts) == 3 {
                if metaData, err := base64.StdEncoding.DecodeString(parts[2]); err == nil {
                    json.Unmarshal(metaData, &clientInfo)
                }
            }
            
            return &StandardMessage{
                Version:   "selkies",
                Type:      "hello",
                ID:        generateMessageID(),
                Timestamp: time.Now().Unix(),
                PeerID:    peerID,
                Data: map[string]interface{}{
                    "client_info": clientInfo,
                },
            }, nil
        }
    }
    
    return nil, fmt.Errorf("unknown selkies text message: %s", text)
}

func (s *SelkiesProtocolAdapter) FormatMessage(msg *StandardMessage) ([]byte, error) {
    switch msg.Type {
    case "hello":
        if msg.Data != nil {
            if clientInfo, ok := msg.Data.(map[string]interface{})["client_info"]; ok {
                metaBytes, _ := json.Marshal(clientInfo)
                metaB64 := base64.StdEncoding.EncodeToString(metaBytes)
                return []byte(fmt.Sprintf("HELLO %s %s", msg.PeerID, metaB64)), nil
            }
        }
        return []byte("HELLO"), nil
        
    case "offer":
        if sdpData, ok := msg.Data.(map[string]interface{})["sdp"]; ok {
            selkiesMsg := map[string]interface{}{"sdp": sdpData}
            return json.Marshal(selkiesMsg)
        }
        
    case "ice-candidate":
        if candidateData, ok := msg.Data.(map[string]interface{})["candidate"]; ok {
            selkiesMsg := map[string]interface{}{"ice": candidateData}
            return json.Marshal(selkiesMsg)
        }
        
    case "error":
        return []byte(fmt.Sprintf("ERROR %s", msg.Error.Message)), nil
    }
    
    return nil, fmt.Errorf("unsupported message type for selkies format: %s", msg.Type)
}
```

## 浏览器兼容性

### 支持的浏览器版本

| 浏览器 | 最低版本 | 推荐版本 | WebRTC 支持 | 注意事项 |
|--------|----------|----------|-------------|----------|
| Chrome | 80+ | 最新版本 | 完全支持 | 最佳性能 |
| Firefox | 75+ | 最新版本 | 完全支持 | 需要启用某些特性 |
| Safari | 13+ | 最新版本 | 部分支持 | 限制较多 |
| Edge | 80+ | 最新版本 | 完全支持 | 基于 Chromium |

### 浏览器特性检测

```javascript
// WebRTC 特性检测
function detectWebRTCSupport() {
  const support = {
    webrtc: false,
    datachannel: false,
    getUserMedia: false,
    getDisplayMedia: false,
    insertableStreams: false
  };
  
  // 基础 WebRTC 支持
  if (typeof RTCPeerConnection !== 'undefined') {
    support.webrtc = true;
    
    // 数据通道支持
    try {
      const pc = new RTCPeerConnection();
      const dc = pc.createDataChannel('test');
      support.datachannel = true;
      pc.close();
    } catch (e) {
      console.warn('DataChannel not supported:', e);
    }
  }
  
  // getUserMedia 支持
  if (navigator.mediaDevices && navigator.mediaDevices.getUserMedia) {
    support.getUserMedia = true;
  }
  
  // getDisplayMedia 支持（屏幕共享）
  if (navigator.mediaDevices && navigator.mediaDevices.getDisplayMedia) {
    support.getDisplayMedia = true;
  }
  
  // Insertable Streams 支持（高级特性）
  if (typeof RTCRtpSender !== 'undefined' && 
      RTCRtpSender.prototype.createEncodedStreams) {
    support.insertableStreams = true;
  }
  
  return support;
}

// 浏览器兼容性适配
function createCompatiblePeerConnection(config) {
  // 标准化配置
  const normalizedConfig = {
    iceServers: config.iceServers || [],
    ...config
  };
  
  // Safari 特殊处理
  if (isSafari()) {
    // Safari 需要特殊的 ICE 服务器配置
    normalizedConfig.iceServers = normalizedConfig.iceServers.map(server => {
      if (server.urls && Array.isArray(server.urls)) {
        return {
          ...server,
          url: server.urls[0], // Safari 偏好单个 URL
          urls: server.urls
        };
      }
      return server;
    });
  }
  
  return new RTCPeerConnection(normalizedConfig);
}

function isSafari() {
  return /^((?!chrome|android).)*safari/i.test(navigator.userAgent);
}

function isFirefox() {
  return navigator.userAgent.toLowerCase().indexOf('firefox') > -1;
}
```

### 浏览器特定优化

```javascript
// Chrome 优化
function optimizeForChrome(peerConnection) {
  // 启用硬件加速
  const transceivers = peerConnection.getTransceivers();
  transceivers.forEach(transceiver => {
    if (transceiver.sender && transceiver.sender.track) {
      const params = transceiver.sender.getParameters();
      if (params.encodings) {
        params.encodings.forEach(encoding => {
          encoding.maxBitrate = 2000000; // 2 Mbps
        });
        transceiver.sender.setParameters(params);
      }
    }
  });
}

// Firefox 优化
function optimizeForFirefox(peerConnection) {
  // Firefox 特定的编解码器偏好
  const transceivers = peerConnection.getTransceivers();
  transceivers.forEach(transceiver => {
    if (transceiver.setCodecPreferences) {
      const codecs = RTCRtpReceiver.getCapabilities('video').codecs;
      const h264Codecs = codecs.filter(codec => 
        codec.mimeType.toLowerCase() === 'video/h264'
      );
      if (h264Codecs.length > 0) {
        transceiver.setCodecPreferences(h264Codecs);
      }
    }
  });
}

// Safari 优化
function optimizeForSafari(peerConnection) {
  // Safari 需要更保守的配置
  peerConnection.addEventListener('icecandidate', (event) => {
    if (event.candidate) {
      // Safari 有时需要延迟发送 ICE 候选
      setTimeout(() => {
        // 发送 ICE 候选
      }, 100);
    }
  });
}
```

## 网络兼容性

### NAT 和防火墙处理

```javascript
// ICE 配置优化
function createOptimizedICEConfig() {
  return {
    iceServers: [
      // STUN 服务器（用于 NAT 穿越）
      { urls: 'stun:stun.l.google.com:19302' },
      { urls: 'stun:stun1.l.google.com:19302' },
      
      // TURN 服务器（用于严格的防火墙环境）
      {
        urls: [
          'turn:turn.example.com:3478',
          'turns:turn.example.com:5349'
        ],
        username: 'turnuser',
        credential: 'turnpass'
      }
    ],
    
    // ICE 传输策略
    iceTransportPolicy: 'all', // 'all' 或 'relay'
    
    // 捆绑策略
    bundlePolicy: 'max-bundle',
    
    // RTCP 复用策略
    rtcpMuxPolicy: 'require'
  };
}

// 网络质量检测
async function detectNetworkQuality() {
  const quality = {
    type: 'unknown',
    bandwidth: 0,
    latency: 0,
    packetLoss: 0
  };
  
  // 使用 Network Information API（如果可用）
  if ('connection' in navigator) {
    const connection = navigator.connection;
    quality.type = connection.effectiveType || 'unknown';
    quality.bandwidth = connection.downlink || 0;
  }
  
  // RTT 测量
  try {
    const startTime = Date.now();
    await fetch('/api/ping', { method: 'HEAD' });
    quality.latency = Date.now() - startTime;
  } catch (error) {
    console.warn('Failed to measure latency:', error);
  }
  
  return quality;
}

// 自适应比特率
function adaptBitrate(peerConnection, networkQuality) {
  const transceivers = peerConnection.getTransceivers();
  
  transceivers.forEach(transceiver => {
    if (transceiver.sender && transceiver.sender.track && 
        transceiver.sender.track.kind === 'video') {
      
      const params = transceiver.sender.getParameters();
      if (params.encodings && params.encodings.length > 0) {
        let maxBitrate;
        
        switch (networkQuality.type) {
          case 'slow-2g':
            maxBitrate = 100000; // 100 kbps
            break;
          case '2g':
            maxBitrate = 250000; // 250 kbps
            break;
          case '3g':
            maxBitrate = 500000; // 500 kbps
            break;
          case '4g':
            maxBitrate = 2000000; // 2 Mbps
            break;
          default:
            maxBitrate = 1000000; // 1 Mbps
        }
        
        params.encodings[0].maxBitrate = maxBitrate;
        transceiver.sender.setParameters(params);
      }
    }
  });
}
```

### 代理和企业网络支持

```javascript
// 代理检测和配置
function detectProxyEnvironment() {
  const proxyIndicators = {
    hasProxy: false,
    proxyType: 'unknown',
    restrictions: []
  };
  
  // 检查常见的代理指示器
  if (typeof navigator.connection !== 'undefined') {
    const connection = navigator.connection;
    
    // 检查是否在企业网络中
    if (connection.effectiveType === 'slow-2g' && 
        connection.downlink > 1) {
      proxyIndicators.hasProxy = true;
      proxyIndicators.proxyType = 'corporate';
    }
  }
  
  // 检查 WebSocket 连接限制
  const wsTest = new WebSocket('ws://echo.websocket.org');
  wsTest.onopen = () => {
    wsTest.close();
  };
  wsTest.onerror = () => {
    proxyIndicators.restrictions.push('websocket_blocked');
  };
  
  return proxyIndicators;
}

// 企业网络优化配置
function createEnterpriseConfig() {
  return {
    // 只使用 TURN 服务器（绕过防火墙）
    iceTransportPolicy: 'relay',
    
    // 使用 TCP 传输（更容易通过防火墙）
    iceServers: [
      {
        urls: 'turn:turn.example.com:443?transport=tcp',
        username: 'enterprise_user',
        credential: 'enterprise_pass'
      }
    ],
    
    // 启用 DTLS
    dtlsSrtpKeyAgreement: true
  };
}
```

## 迁移策略

### 从 Selkies 到标准协议的迁移

#### 阶段 1: 双协议支持

```javascript
// 迁移适配器
class MigrationAdapter {
  constructor(targetProtocol = 'gstreamer-1.0') {
    this.targetProtocol = targetProtocol;
    this.currentProtocol = 'selkies'; // 默认从 selkies 开始
    this.migrationPhase = 'dual-support';
  }
  
  async connect(serverUrl) {
    // 尝试协议协商
    try {
      const negotiationResult = await this.negotiateProtocol([
        this.targetProtocol,
        this.currentProtocol
      ]);
      
      this.currentProtocol = negotiationResult.selected_protocol;
      
      if (this.currentProtocol === this.targetProtocol) {
        this.migrationPhase = 'migrated';
        console.log('Successfully migrated to', this.targetProtocol);
      } else {
        this.migrationPhase = 'fallback';
        console.log('Falling back to', this.currentProtocol);
      }
      
    } catch (error) {
      console.warn('Protocol negotiation failed, using fallback');
      this.currentProtocol = 'selkies';
      this.migrationPhase = 'fallback';
    }
  }
  
  sendMessage(type, data) {
    if (this.currentProtocol === 'selkies') {
      return this.sendSelkiesMessage(type, data);
    } else {
      return this.sendStandardMessage(type, data);
    }
  }
  
  sendSelkiesMessage(type, data) {
    // Selkies 格式消息发送
    switch (type) {
      case 'sdp':
        return this.ws.send(JSON.stringify({ sdp: data }));
      case 'ice':
        return this.ws.send(JSON.stringify({ ice: data }));
      default:
        console.warn('Unsupported Selkies message type:', type);
    }
  }
  
  sendStandardMessage(type, data) {
    // 标准格式消息发送
    const message = {
      version: '1.0',
      type: type,
      id: this.generateMessageId(),
      timestamp: Date.now(),
      data: data
    };
    
    return this.ws.send(JSON.stringify(message));
  }
}
```

#### 阶段 2: 渐进式迁移

```go
// 服务端渐进式迁移支持
type MigrationManager struct {
    clients           map[string]*ClientMigrationState
    migrationPolicy   MigrationPolicy
    mutex            sync.RWMutex
}

type ClientMigrationState struct {
    ClientID          string
    CurrentProtocol   string
    TargetProtocol    string
    MigrationPhase    string
    LastMigrationAttempt time.Time
    MigrationAttempts int
}

type MigrationPolicy struct {
    MaxAttempts       int
    AttemptInterval   time.Duration
    ForceAfter        time.Duration
    AllowFallback     bool
}

func (m *MigrationManager) HandleClientConnection(client *SignalingClient) {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    state := &ClientMigrationState{
        ClientID:        client.ID,
        CurrentProtocol: client.Protocol,
        TargetProtocol:  "gstreamer-1.0",
        MigrationPhase:  "initial",
    }
    
    m.clients[client.ID] = state
    
    // 尝试协议升级
    go m.attemptProtocolUpgrade(client, state)
}

func (m *MigrationManager) attemptProtocolUpgrade(client *SignalingClient, state *ClientMigrationState) {
    if state.CurrentProtocol == state.TargetProtocol {
        state.MigrationPhase = "completed"
        return
    }
    
    if state.MigrationAttempts >= m.migrationPolicy.MaxAttempts {
        if m.migrationPolicy.AllowFallback {
            state.MigrationPhase = "fallback"
            log.Printf("Client %s migration failed, using fallback protocol", client.ID)
        } else {
            state.MigrationPhase = "forced"
            client.Protocol = state.TargetProtocol
            log.Printf("Client %s forced to use target protocol", client.ID)
        }
        return
    }
    
    // 发送协议升级请求
    upgradeRequest := SignalingMessage{
        Type: "protocol-upgrade",
        Data: map[string]interface{}{
            "target_protocol": state.TargetProtocol,
            "migration_benefits": []string{
                "improved_performance",
                "better_error_handling",
                "enhanced_features"
            },
        },
    }
    
    if err := client.sendMessage(upgradeRequest); err != nil {
        log.Printf("Failed to send upgrade request to client %s: %v", client.ID, err)
    }
    
    state.MigrationAttempts++
    state.LastMigrationAttempt = time.Now()
    
    // 安排下次尝试
    time.AfterFunc(m.migrationPolicy.AttemptInterval, func() {
        m.attemptProtocolUpgrade(client, state)
    })
}
```

#### 阶段 3: 完全迁移

```javascript
// 迁移完成检查
class MigrationValidator {
  constructor() {
    this.validationTests = [
      this.testBasicConnectivity,
      this.testMessageExchange,
      this.testWebRTCNegotiation,
      this.testErrorHandling,
      this.testPerformance
    ];
  }
  
  async validateMigration(signalingClient) {
    const results = {
      passed: 0,
      failed: 0,
      tests: []
    };
    
    for (const test of this.validationTests) {
      try {
        const result = await test.call(this, signalingClient);
        results.tests.push({
          name: test.name,
          status: 'passed',
          result: result
        });
        results.passed++;
      } catch (error) {
        results.tests.push({
          name: test.name,
          status: 'failed',
          error: error.message
        });
        results.failed++;
      }
    }
    
    results.success = results.failed === 0;
    results.score = results.passed / this.validationTests.length;
    
    return results;
  }
  
  async testBasicConnectivity(client) {
    if (!client.isConnected()) {
      throw new Error('Client not connected');
    }
    
    const state = client.getConnectionState();
    if (state.protocol !== 'gstreamer-1.0') {
      throw new Error(`Expected gstreamer-1.0, got ${state.protocol}`);
    }
    
    return { protocol: state.protocol, connected: true };
  }
  
  async testMessageExchange(client) {
    const startTime = Date.now();
    
    const response = await new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(new Error('Message exchange timeout'));
      }, 5000);
      
      client.sendMessage('ping', { test: 'migration_validation' }, {
        expectAck: true,
        resolve: (response) => {
          clearTimeout(timeout);
          resolve(response);
        },
        reject: (error) => {
          clearTimeout(timeout);
          reject(error);
        }
      });
    });
    
    const latency = Date.now() - startTime;
    
    return { latency, response: response.type };
  }
  
  async testWebRTCNegotiation(client) {
    // 模拟 WebRTC 协商测试
    const offerReceived = new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(new Error('Offer timeout'));
      }, 10000);
      
      client.onsdp = (sdp) => {
        clearTimeout(timeout);
        resolve(sdp);
      };
      
      client.sendMessage('request-offer', {
        constraints: { video: true, audio: false }
      });
    });
    
    const offer = await offerReceived;
    
    return { 
      offerReceived: true, 
      sdpType: offer.type,
      sdpSize: offer.sdp.length 
    };
  }
  
  async testErrorHandling(client) {
    // 发送无效消息测试错误处理
    const errorReceived = new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(new Error('Error handling timeout'));
      }, 5000);
      
      client.onerror = (error) => {
        clearTimeout(timeout);
        resolve(error);
      };
      
      // 发送无效消息
      client.sendMessage('invalid-message-type', { test: 'error' });
    });
    
    const error = await errorReceived;
    
    return { 
      errorHandled: true, 
      errorCode: error.code,
      errorMessage: error.message 
    };
  }
  
  async testPerformance(client) {
    const messageCount = 100;
    const startTime = Date.now();
    
    const promises = [];
    for (let i = 0; i < messageCount; i++) {
      promises.push(
        client.sendMessage('ping', { sequence: i })
      );
    }
    
    await Promise.all(promises);
    
    const duration = Date.now() - startTime;
    const throughput = messageCount / (duration / 1000);
    
    if (throughput < 50) { // 最低性能要求
      throw new Error(`Performance too low: ${throughput} msg/s`);
    }
    
    return { 
      throughput, 
      duration, 
      messageCount 
    };
  }
}
```

## 故障排除

### 常见问题和解决方案

#### 1. 连接问题

```javascript
// 连接诊断工具
class ConnectionDiagnostics {
  async diagnose(serverUrl) {
    const results = {
      websocket: await this.testWebSocket(serverUrl),
      network: await this.testNetwork(),
      browser: this.testBrowserSupport(),
      firewall: await this.testFirewall()
    };
    
    return this.generateReport(results);
  }
  
  async testWebSocket(serverUrl) {
    return new Promise((resolve) => {
      const ws = new WebSocket(serverUrl);
      const startTime = Date.now();
      
      const timeout = setTimeout(() => {
        ws.close();
        resolve({
          success: false,
          error: 'Connection timeout',
          duration: Date.now() - startTime
        });
      }, 5000);
      
      ws.onopen = () => {
        clearTimeout(timeout);
        ws.close();
        resolve({
          success: true,
          duration: Date.now() - startTime
        });
      };
      
      ws.onerror = (error) => {
        clearTimeout(timeout);
        resolve({
          success: false,
          error: error.message || 'WebSocket error',
          duration: Date.now() - startTime
        });
      };
    });
  }
  
  async testNetwork() {
    const tests = {
      connectivity: false,
      latency: 0,
      bandwidth: 0
    };
    
    try {
      const startTime = Date.now();
      const response = await fetch('/api/ping', { method: 'HEAD' });
      tests.latency = Date.now() - startTime;
      tests.connectivity = response.ok;
    } catch (error) {
      tests.connectivity = false;
    }
    
    // 带宽测试（简化版）
    if ('connection' in navigator) {
      tests.bandwidth = navigator.connection.downlink || 0;
    }
    
    return tests;
  }
  
  testBrowserSupport() {
    const support = detectWebRTCSupport();
    
    return {
      webrtc: support.webrtc,
      websocket: typeof WebSocket !== 'undefined',
      json: typeof JSON !== 'undefined',
      promises: typeof Promise !== 'undefined',
      compatibility: this.calculateCompatibilityScore(support)
    };
  }
  
  async testFirewall() {
    const tests = {
      websocket_80: await this.testPort('ws://echo.websocket.org:80'),
      websocket_443: await this.testPort('wss://echo.websocket.org:443'),
      stun: await this.testSTUN(),
      turn: await this.testTURN()
    };
    
    return tests;
  }
  
  generateReport(results) {
    const issues = [];
    const recommendations = [];
    
    if (!results.websocket.success) {
      issues.push('WebSocket connection failed');
      recommendations.push('Check server availability and network connectivity');
    }
    
    if (!results.browser.webrtc) {
      issues.push('WebRTC not supported');
      recommendations.push('Use a modern browser with WebRTC support');
    }
    
    if (results.network.latency > 1000) {
      issues.push('High network latency');
      recommendations.push('Check network connection quality');
    }
    
    return {
      overall: issues.length === 0 ? 'healthy' : 'issues_detected',
      issues,
      recommendations,
      details: results
    };
  }
}
```

#### 2. 协议兼容性问题

```javascript
// 协议兼容性检查器
class ProtocolCompatibilityChecker {
  checkCompatibility(clientProtocol, serverProtocol) {
    const compatibility = {
      compatible: false,
      issues: [],
      recommendations: []
    };
    
    // 检查协议版本兼容性
    if (clientProtocol === serverProtocol) {
      compatibility.compatible = true;
    } else if (this.isCompatibleProtocol(clientProtocol, serverProtocol)) {
      compatibility.compatible = true;
      compatibility.issues.push('Protocol version mismatch but compatible');
    } else {
      compatibility.issues.push('Incompatible protocol versions');
      compatibility.recommendations.push('Upgrade client or server to matching version');
    }
    
    return compatibility;
  }
  
  isCompatibleProtocol(client, server) {
    const compatibilityMatrix = {
      'gstreamer-1.0': ['gstreamer-1.0', 'selkies'],
      'selkies': ['selkies', 'gstreamer-1.0']
    };
    
    return compatibilityMatrix[client]?.includes(server) || false;
  }
  
  suggestMigrationPath(currentProtocol, targetProtocol) {
    const migrationPaths = {
      'selkies->gstreamer-1.0': [
        'Enable dual protocol support',
        'Test standard protocol compatibility',
        'Gradually migrate clients',
        'Disable legacy protocol support'
      ]
    };
    
    const pathKey = `${currentProtocol}->${targetProtocol}`;
    return migrationPaths[pathKey] || ['Direct migration not supported'];
  }
}
```

#### 3. 性能问题

```javascript
// 性能监控和优化
class PerformanceMonitor {
  constructor() {
    this.metrics = {
      messageLatency: [],
      connectionQuality: [],
      throughput: []
    };
  }
  
  startMonitoring(signalingClient) {
    // 监控消息延迟
    this.monitorMessageLatency(signalingClient);
    
    // 监控连接质量
    this.monitorConnectionQuality(signalingClient);
    
    // 监控吞吐量
    this.monitorThroughput(signalingClient);
  }
  
  monitorMessageLatency(client) {
    const originalSend = client.sendMessage.bind(client);
    
    client.sendMessage = (type, data, options = {}) => {
      const startTime = Date.now();
      
      const originalResolve = options.resolve;
      options.resolve = (response) => {
        const latency = Date.now() - startTime;
        this.recordLatency(latency);
        
        if (originalResolve) originalResolve(response);
      };
      
      return originalSend(type, data, options);
    };
  }
  
  recordLatency(latency) {
    this.metrics.messageLatency.push({
      timestamp: Date.now(),
      latency: latency
    });
    
    // 保持最近 100 个记录
    if (this.metrics.messageLatency.length > 100) {
      this.metrics.messageLatency.shift();
    }
    
    // 检查性能问题
    if (latency > 1000) {
      console.warn('High message latency detected:', latency + 'ms');
      this.suggestOptimizations('high_latency');
    }
  }
  
  suggestOptimizations(issue) {
    const optimizations = {
      high_latency: [
        'Check network connection',
        'Reduce message frequency',
        'Use message batching',
        'Consider using a closer server'
      ],
      low_throughput: [
        'Enable message compression',
        'Increase buffer sizes',
        'Use connection pooling',
        'Optimize message serialization'
      ],
      connection_instability: [
        'Implement exponential backoff',
        'Add connection health checks',
        'Use redundant connections',
        'Enable automatic reconnection'
      ]
    };
    
    console.log('Performance optimization suggestions:', optimizations[issue]);
  }
  
  generatePerformanceReport() {
    const report = {
      averageLatency: this.calculateAverageLatency(),
      maxLatency: this.calculateMaxLatency(),
      connectionStability: this.calculateConnectionStability(),
      recommendations: this.generateRecommendations()
    };
    
    return report;
  }
  
  calculateAverageLatency() {
    if (this.metrics.messageLatency.length === 0) return 0;
    
    const sum = this.metrics.messageLatency.reduce((acc, metric) => acc + metric.latency, 0);
    return sum / this.metrics.messageLatency.length;
  }
  
  generateRecommendations() {
    const recommendations = [];
    const avgLatency = this.calculateAverageLatency();
    
    if (avgLatency > 500) {
      recommendations.push('Consider optimizing network connection');
    }
    
    if (avgLatency > 1000) {
      recommendations.push('High latency detected - check server location');
    }
    
    return recommendations;
  }
}
```

## 最佳实践

### 1. 协议选择策略

```javascript
// 智能协议选择
class ProtocolSelector {
  selectOptimalProtocol(clientCapabilities, serverCapabilities, networkConditions) {
    const scores = {};
    
    // 为每个支持的协议计算分数
    const supportedProtocols = this.findCommonProtocols(clientCapabilities, serverCapabilities);
    
    supportedProtocols.forEach(protocol => {
      scores[protocol] = this.calculateProtocolScore(protocol, networkConditions);
    });
    
    // 选择得分最高的协议
    const bestProtocol = Object.keys(scores).reduce((a, b) => 
      scores[a] > scores[b] ? a : b
    );
    
    return {
      selected: bestProtocol,
      score: scores[bestProtocol],
      alternatives: scores
    };
  }
  
  calculateProtocolScore(protocol, networkConditions) {
    let score = 0;
    
    // 基础协议分数
    const baseScores = {
      'gstreamer-1.0': 100,
      'selkies': 80
    };
    
    score += baseScores[protocol] || 0;
    
    // 网络条件调整
    if (networkConditions.bandwidth < 1) { // 低带宽
      if (protocol === 'gstreamer-1.0') score += 20; // 更好的压缩
    }
    
    if (networkConditions.latency > 200) { // 高延迟
      if (protocol === 'gstreamer-1.0') score += 15; // 更好的缓冲
    }
    
    if (networkConditions.packetLoss > 0.05) { // 高丢包率
      if (protocol === 'gstreamer-1.0') score += 25; // 更好的错误恢复
    }
    
    return score;
  }
}
```

### 2. 错误恢复策略

```javascript
// 智能错误恢复
class ErrorRecoveryManager {
  constructor(signalingClient) {
    this.client = signalingClient;
    this.recoveryStrategies = new Map();
    this.setupRecoveryStrategies();
  }
  
  setupRecoveryStrategies() {
    this.recoveryStrategies.set('CONNECTION_LOST', this.handleConnectionLoss.bind(this));
    this.recoveryStrategies.set('PROTOCOL_ERROR', this.handleProtocolError.bind(this));
    this.recoveryStrategies.set('MESSAGE_TIMEOUT', this.handleMessageTimeout.bind(this));
    this.recoveryStrategies.set('WEBRTC_FAILURE', this.handleWebRTCFailure.bind(this));
  }
  
  async handleError(error) {
    const strategy = this.recoveryStrategies.get(error.code);
    
    if (strategy) {
      try {
        await strategy(error);
        console.log('Error recovered successfully:', error.code);
      } catch (recoveryError) {
        console.error('Error recovery failed:', recoveryError);
        this.escalateError(error, recoveryError);
      }
    } else {
      console.warn('No recovery strategy for error:', error.code);
      this.escalateError(error);
    }
  }
  
  async handleConnectionLoss(error) {
    // 实现指数退避重连
    let attempt = 0;
    const maxAttempts = 5;
    
    while (attempt < maxAttempts) {
      const delay = Math.min(1000 * Math.pow(2, attempt), 30000);
      
      console.log(`Reconnection attempt ${attempt + 1}/${maxAttempts} in ${delay}ms`);
      
      await new Promise(resolve => setTimeout(resolve, delay));
      
      try {
        await this.client.connect();
        return; // 成功重连
      } catch (reconnectError) {
        attempt++;
        if (attempt >= maxAttempts) {
          throw new Error('Max reconnection attempts exceeded');
        }
      }
    }
  }
  
  async handleProtocolError(error) {
    // 尝试协议降级
    const currentProtocol = this.client.getConnectionState().protocol;
    const fallbackProtocols = this.getFallbackProtocols(currentProtocol);
    
    for (const fallbackProtocol of fallbackProtocols) {
      try {
        console.log(`Trying fallback protocol: ${fallbackProtocol}`);
        
        this.client.disconnect();
        this.client.protocol = fallbackProtocol;
        await this.client.connect();
        
        return; // 成功使用备用协议
      } catch (fallbackError) {
        console.warn(`Fallback protocol ${fallbackProtocol} failed:`, fallbackError);
      }
    }
    
    throw new Error('All fallback protocols failed');
  }
  
  getFallbackProtocols(currentProtocol) {
    const fallbackMap = {
      'gstreamer-1.0': ['selkies'],
      'selkies': ['gstreamer-1.0']
    };
    
    return fallbackMap[currentProtocol] || [];
  }
}
```

### 3. 性能优化建议

```javascript
// 性能优化配置
const PERFORMANCE_CONFIGS = {
  // 高性能配置（良好网络环境）
  high_performance: {
    heartbeatInterval: 15000,
    connectionTimeout: 5000,
    maxRetries: 5,
    retryDelay: 1000,
    messageBufferSize: 1024,
    enableCompression: true,
    batchMessages: true,
    batchTimeout: 50
  },
  
  // 稳定性优先配置（不稳定网络）
  stability_focused: {
    heartbeatInterval: 45000,
    connectionTimeout: 15000,
    maxRetries: 10,
    retryDelay: 5000,
    messageBufferSize: 256,
    enableCompression: false,
    batchMessages: false,
    redundantConnections: true
  },
  
  // 低带宽配置
  low_bandwidth: {
    heartbeatInterval: 60000,
    connectionTimeout: 20000,
    maxRetries: 3,
    retryDelay: 10000,
    messageBufferSize: 128,
    enableCompression: true,
    batchMessages: true,
    batchTimeout: 200,
    reduceMessageFrequency: true
  }
};

// 自动配置选择
function selectOptimalConfig(networkConditions) {
  if (networkConditions.bandwidth > 5 && networkConditions.latency < 100) {
    return PERFORMANCE_CONFIGS.high_performance;
  } else if (networkConditions.packetLoss > 0.05 || networkConditions.latency > 500) {
    return PERFORMANCE_CONFIGS.stability_focused;
  } else if (networkConditions.bandwidth < 1) {
    return PERFORMANCE_CONFIGS.low_bandwidth;
  } else {
    // 默认平衡配置
    return {
      ...PERFORMANCE_CONFIGS.high_performance,
      heartbeatInterval: 30000,
      connectionTimeout: 10000
    };
  }
}
```

这个兼容性指南提供了全面的协议兼容性支持、迁移策略和故障排除方法，帮助开发者在不同环境和需求下成功实施 GStreamer WebRTC 信令协议。
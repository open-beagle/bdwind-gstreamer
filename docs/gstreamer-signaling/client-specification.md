# GStreamer WebRTC 信令客户端规范

## 概述

本文档详细说明了浏览器端 GStreamer WebRTC 信令客户端的实现要求，包括必需的方法、接口、状态管理和错误处理机制。

## 核心接口定义

### SignalingClient 类

```javascript
class SignalingClient {
  constructor(options = {}) {
    // 必需参数
    this.serverUrl = options.serverUrl;
    this.peerId = options.peerId || 1;
    
    // 可选参数
    this.eventBus = options.eventBus || null;
    this.logger = options.logger || console;
    this.config = options.config || {};
    
    // 连接配置
    this.maxRetries = options.maxRetries || 3;
    this.retryDelay = options.retryDelay || 3000;
    this.connectionTimeout = options.connectionTimeout || 10000;
    this.heartbeatInterval = options.heartbeatInterval || 30000;
    
    // 内部状态
    this._ws = null;
    this._state = 'disconnected';
    this._retryCount = 0;
    this._messageId = 0;
    this._pendingMessages = new Map();
    
    // 事件回调
    this.onopen = null;
    this.onclose = null;
    this.onerror = null;
    this.onmessage = null;
    this.onsdp = null;
    this.onice = null;
    this.onstatus = null;
  }
}
```

## 必需实现的方法

### 1. 连接管理方法

#### connect()
建立到信令服务器的 WebSocket 连接。

```javascript
async connect() {
  if (this._state === 'connected' || this._state === 'connecting') {
    throw new Error('Already connected or connecting');
  }
  
  this._state = 'connecting';
  
  try {
    this._ws = new WebSocket(this.serverUrl);
    this._setupWebSocketHandlers();
    
    // 等待连接建立
    await this._waitForConnection();
    
    // 发送 HELLO 消息
    await this._sendHello();
    
    this._state = 'connected';
    this._retryCount = 0;
    
    // 启动心跳
    this._startHeartbeat();
    
    if (this.onopen) this.onopen();
    
  } catch (error) {
    this._state = 'disconnected';
    throw error;
  }
}
```

#### disconnect()
断开与信令服务器的连接。

```javascript
disconnect() {
  this._state = 'disconnecting';
  this._stopHeartbeat();
  
  if (this._ws) {
    this._ws.close(1000, 'Client disconnect');
    this._ws = null;
  }
  
  this._state = 'disconnected';
  this._retryCount = 0;
  
  if (this.onclose) this.onclose();
}
```

#### reconnect()
重新连接到信令服务器。

```javascript
async reconnect() {
  this.disconnect();
  
  // 指数退避延迟
  const delay = Math.min(
    this.retryDelay * Math.pow(2, this._retryCount),
    30000
  );
  
  await new Promise(resolve => setTimeout(resolve, delay));
  
  this._retryCount++;
  
  if (this._retryCount > this.maxRetries) {
    throw new Error('Max retry attempts exceeded');
  }
  
  return this.connect();
}
```

### 2. 消息处理方法

#### sendMessage(type, data, options)
发送标准格式的信令消息。

```javascript
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
  
  // 添加到待确认消息列表
  if (options.expectAck) {
    this._pendingMessages.set(message.id, {
      message,
      timestamp: Date.now(),
      resolve: options.resolve,
      reject: options.reject
    });
  }
  
  this._ws.send(JSON.stringify(message));
  
  return message.id;
}
```

#### sendSDP(sdp)
发送 SDP 消息（支持多种格式）。

```javascript
sendSDP(sdp) {
  // 检测协议格式并发送相应格式的消息
  if (this._isSelkiesMode()) {
    // Selkies 格式
    this._ws.send(JSON.stringify({ sdp: sdp }));
  } else {
    // 标准格式
    this.sendMessage('answer', { sdp: sdp });
  }
}
```

#### sendICE(candidate)
发送 ICE 候选。

```javascript
sendICE(candidate) {
  if (this._isSelkiesMode()) {
    // Selkies 格式
    this._ws.send(JSON.stringify({ ice: candidate }));
  } else {
    // 标准格式
    this.sendMessage('ice-candidate', { candidate: candidate });
  }
}
```

### 3. 状态管理方法

#### getState()
获取当前连接状态。

```javascript
getState() {
  return {
    connectionState: this._state,
    readyState: this._ws ? this._ws.readyState : WebSocket.CLOSED,
    peerId: this.peerId,
    retryCount: this._retryCount,
    serverUrl: this.serverUrl,
    isConnected: this._state === 'connected',
    canRetry: this._retryCount < this.maxRetries
  };
}
```

#### isConnected()
检查是否已连接。

```javascript
isConnected() {
  return this._state === 'connected' && 
         this._ws && 
         this._ws.readyState === WebSocket.OPEN;
}
```

### 4. 协议协商方法

#### negotiateProtocol(supportedProtocols)
协商协议版本。

```javascript
async negotiateProtocol(supportedProtocols = ['gstreamer-1.0', 'selkies']) {
  const negotiationMessage = {
    type: 'protocol-negotiation',
    data: {
      supported_protocols: supportedProtocols,
      preferred_protocol: supportedProtocols[0],
      client_capabilities: this._getClientCapabilities()
    }
  };
  
  return new Promise((resolve, reject) => {
    this.sendMessage(
      negotiationMessage.type, 
      negotiationMessage.data,
      { 
        expectAck: true, 
        resolve: resolve, 
        reject: reject 
      }
    );
  });
}
```

## WebSocket 事件处理

### 连接事件处理

```javascript
_setupWebSocketHandlers() {
  this._ws.onopen = (event) => {
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
```

### 消息处理逻辑

```javascript
_handleMessage(event) {
  try {
    const data = event.data;
    
    // 处理 Selkies 协议文本消息
    if (typeof data === 'string' && !data.startsWith('{')) {
      this._handleSelkiesTextMessage(data);
      return;
    }
    
    // 解析 JSON 消息
    const message = JSON.parse(data);
    
    // 处理不同消息类型
    switch (message.type || this._detectMessageType(message)) {
      case 'welcome':
        this._handleWelcome(message);
        break;
      case 'offer':
        this._handleOffer(message);
        break;
      case 'ice-candidate':
        this._handleIceCandidate(message);
        break;
      case 'error':
        this._handleError(message);
        break;
      case 'pong':
        this._handlePong(message);
        break;
      default:
        // 处理 Selkies JSON 消息
        if (message.sdp) {
          this._handleSelkiesSDP(message);
        } else if (message.ice) {
          this._handleSelkiesICE(message);
        } else {
          this.logger.warn('Unknown message type:', message);
        }
    }
    
    // 触发通用消息回调
    if (this.onmessage) {
      this.onmessage(message);
    }
    
  } catch (error) {
    this.logger.error('Failed to handle message:', error);
    if (this.onerror) {
      this.onerror(new Error('Message parsing failed: ' + error.message));
    }
  }
}
```

## 协议兼容性处理

### Selkies 协议支持

```javascript
_handleSelkiesTextMessage(data) {
  if (data === 'HELLO') {
    // 服务器确认注册
    this._setSelkiesMode(true);
    if (this.onstatus) {
      this.onstatus('Registered with server (Selkies mode)');
    }
  } else if (data.startsWith('ERROR')) {
    // 服务器错误
    const error = new Error('Server error: ' + data);
    if (this.onerror) this.onerror(error);
  }
}

_handleSelkiesSDP(message) {
  if (message.sdp && this.onsdp) {
    this.onsdp(new RTCSessionDescription(message.sdp));
  }
}

_handleSelkiesICE(message) {
  if (message.ice && this.onice) {
    this.onice(new RTCIceCandidate(message.ice));
  }
}

_sendHello() {
  if (this._isSelkiesMode()) {
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
      peer_id: this.peerId,
      client_info: this._getClientInfo(),
      capabilities: this._getClientCapabilities()
    });
  }
}
```

## 错误处理机制

### 错误分类和处理

```javascript
_handleConnectionError(event) {
  const error = {
    type: 'connection_error',
    code: 'CONNECTION_FAILED',
    message: 'WebSocket connection failed',
    details: event,
    timestamp: Date.now(),
    recoverable: this._retryCount < this.maxRetries
  };
  
  this.logger.error('Connection error:', error);
  
  if (this.onerror) {
    this.onerror(error);
  }
  
  // 自动重连
  if (error.recoverable && this._state !== 'disconnecting') {
    this._scheduleReconnect();
  }
}

_handleConnectionClose(event) {
  const wasConnected = this._state === 'connected';
  this._state = 'disconnected';
  this._stopHeartbeat();
  
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
  
  if (this.onclose) {
    this.onclose(closeInfo);
  }
}
```

### 自动重连机制

```javascript
_scheduleReconnect() {
  if (this._retryCount >= this.maxRetries) {
    const error = new Error('Max retry attempts exceeded');
    error.type = 'max_retries_exceeded';
    if (this.onerror) this.onerror(error);
    return;
  }
  
  const delay = Math.min(
    this.retryDelay * Math.pow(2, this._retryCount),
    30000
  );
  
  this.logger.info(`Scheduling reconnect in ${delay}ms (attempt ${this._retryCount + 1}/${this.maxRetries})`);
  
  setTimeout(() => {
    if (this._state === 'disconnected') {
      this.reconnect().catch(error => {
        this.logger.error('Reconnect failed:', error);
        if (this.onerror) this.onerror(error);
      });
    }
  }, delay);
}
```

## 心跳机制

### 心跳实现

```javascript
_startHeartbeat() {
  this._stopHeartbeat();
  
  this._heartbeatTimer = setInterval(() => {
    if (this.isConnected()) {
      this._sendPing();
    } else {
      this._stopHeartbeat();
    }
  }, this.heartbeatInterval);
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

_handlePong(message) {
  const now = Date.now();
  const pingTime = message.data?.timestamp;
  
  if (pingTime) {
    const latency = now - pingTime;
    this.logger.debug(`Pong received, latency: ${latency}ms`);
    
    // 触发延迟测量事件
    if (this.eventBus) {
      this.eventBus.emit('signaling:latency', { latency });
    }
  }
}
```

## 工具方法

### 消息生成和验证

```javascript
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

_detectMessageType(message) {
  // 自动检测消息类型
  if (message.sdp) return 'sdp';
  if (message.ice) return 'ice';
  if (message.error) return 'error';
  return 'unknown';
}

_isSelkiesMode() {
  // 检测是否使用 Selkies 协议模式
  return this._protocolMode === 'selkies';
}

_setSelkiesMode(enabled) {
  this._protocolMode = enabled ? 'selkies' : 'standard';
}
```

## 使用示例

### 基本使用

```javascript
// 创建信令客户端
const signalingClient = new SignalingClient({
  serverUrl: 'ws://localhost:8080/ws',
  peerId: 1,
  maxRetries: 5,
  retryDelay: 2000
});

// 设置事件回调
signalingClient.onopen = () => {
  console.log('Connected to signaling server');
};

signalingClient.onsdp = (sdp) => {
  console.log('Received SDP offer:', sdp);
  // 处理 SDP offer
};

signalingClient.onice = (candidate) => {
  console.log('Received ICE candidate:', candidate);
  // 处理 ICE candidate
};

signalingClient.onerror = (error) => {
  console.error('Signaling error:', error);
};

// 连接到服务器
await signalingClient.connect();

// 发送 SDP answer
signalingClient.sendSDP(answerSDP);

// 发送 ICE candidate
signalingClient.sendICE(iceCandidate);
```

### 与 WebRTC 集成

```javascript
class WebRTCManager {
  constructor(signalingClient) {
    this.signaling = signalingClient;
    this.peerConnection = null;
    
    // 绑定信令事件
    this.signaling.onsdp = this.handleRemoteSDP.bind(this);
    this.signaling.onice = this.handleRemoteICE.bind(this);
  }
  
  async createPeerConnection() {
    this.peerConnection = new RTCPeerConnection({
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
    });
    
    this.peerConnection.onicecandidate = (event) => {
      if (event.candidate) {
        this.signaling.sendICE(event.candidate);
      }
    };
  }
  
  async handleRemoteSDP(sdp) {
    await this.peerConnection.setRemoteDescription(sdp);
    
    if (sdp.type === 'offer') {
      const answer = await this.peerConnection.createAnswer();
      await this.peerConnection.setLocalDescription(answer);
      this.signaling.sendSDP(answer);
    }
  }
  
  async handleRemoteICE(candidate) {
    await this.peerConnection.addIceCandidate(candidate);
  }
}
```

## 测试要求

### 单元测试

1. 连接建立和断开测试
2. 消息发送和接收测试
3. 错误处理和重连测试
4. 协议兼容性测试

### 集成测试

1. 与真实信令服务器的连接测试
2. WebRTC 端到端协商测试
3. 网络异常情况测试
4. 性能和稳定性测试

## 性能要求

1. **连接建立时间**: < 2 秒
2. **消息延迟**: < 100ms
3. **重连时间**: < 5 秒
4. **内存使用**: < 10MB
5. **CPU 使用**: < 5%

## 浏览器兼容性

- Chrome/Chromium 80+
- Firefox 75+
- Safari 13+
- Edge 80+

支持的 WebRTC 特性：
- RTCPeerConnection
- RTCDataChannel
- getUserMedia
- getDisplayMedia
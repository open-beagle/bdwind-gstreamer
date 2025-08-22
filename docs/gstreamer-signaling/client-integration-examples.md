# WebRTC 信令客户端集成示例

## 概述

本文档提供了与 WebRTC 信令服务器集成的完整客户端示例代码，包括 JavaScript、Python 和 Go 客户端实现，以及常见的集成模式和最佳实践。

## JavaScript 客户端示例

### 基础集成示例

```javascript
/**
 * 基础 WebRTC 信令客户端示例
 * 演示 request-offer 和 ping 消息的使用
 */
class WebRTCSignalingClient {
    constructor(serverUrl, options = {}) {
        this.serverUrl = serverUrl;
        this.peerId = options.peerId || this.generatePeerId();
        this.ws = null;
        this.peerConnection = null;
        this.isConnected = false;
        this.messageId = 0;
        
        // 事件回调
        this.onConnected = options.onConnected || (() => {});
        this.onDisconnected = options.onDisconnected || (() => {});
        this.onError = options.onError || console.error;
        this.onOffer = options.onOffer || (() => {});
        this.onStats = options.onStats || (() => {});
        
        // 心跳配置
        this.heartbeatInterval = options.heartbeatInterval || 30000;
        this.heartbeatTimer = null;
    }
    
    /**
     * 连接到信令服务器
     */
    async connect() {
        return new Promise((resolve, reject) => {
            try {
                this.ws = new WebSocket(this.serverUrl);
                
                this.ws.onopen = () => {
                    console.log('✅ Connected to signaling server');
                    this.isConnected = true;
                    this.startHeartbeat();
                    this.sendHello();
                    this.onConnected();
                    resolve();
                };
                
                this.ws.onclose = (event) => {
                    console.log('❌ Disconnected from signaling server', event);
                    this.isConnected = false;
                    this.stopHeartbeat();
                    this.onDisconnected(event);
                };
                
                this.ws.onerror = (error) => {
                    console.error('❌ WebSocket error:', error);
                    this.onError(error);
                    reject(error);
                };
                
                this.ws.onmessage = (event) => {
                    this.handleMessage(event.data);
                };
                
            } catch (error) {
                reject(error);
            }
        });
    }
    
    /**
     * 断开连接
     */
    disconnect() {
        this.isConnected = false;
        this.stopHeartbeat();
        
        if (this.peerConnection) {
            this.peerConnection.close();
            this.peerConnection = null;
        }
        
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
    }
    
    /**
     * 发送 Hello 消息
     */
    sendHello() {
        const message = {
            type: 'hello',
            peer_id: this.peerId,
            message_id: this.generateMessageId(),
            timestamp: Date.now(),
            data: {
                client_info: {
                    user_agent: navigator.userAgent,
                    screen_resolution: `${screen.width}x${screen.height}`,
                    device_pixel_ratio: devicePixelRatio,
                    language: navigator.language
                },
                capabilities: ['webrtc', 'video', 'audio', 'input-events'],
                supported_protocols: ['gstreamer-1.0']
            }
        };
        
        this.sendMessage(message);
    }
    
    /**
     * 请求 WebRTC Offer
     */
    async requestOffer(constraints = { video: true, audio: true }) {
        const message = {
            type: 'request-offer',
            peer_id: this.peerId,
            message_id: this.generateMessageId(),
            timestamp: Date.now(),
            data: {
                constraints: constraints,
                codec_preferences: ['H264', 'VP8', 'VP9']
            }
        };
        
        console.log('📞 Requesting WebRTC offer...');
        this.sendMessage(message);
    }
    
    /**
     * 发送 Answer
     */
    async sendAnswer(answer) {
        const message = {
            type: 'answer',
            peer_id: this.peerId,
            message_id: this.generateMessageId(),
            timestamp: Date.now(),
            data: {
                sdp: {
                    type: answer.type,
                    sdp: answer.sdp
                }
            }
        };
        
        console.log('📤 Sending answer...');
        this.sendMessage(message);
    }
    
    /**
     * 发送 ICE 候选
     */
    sendIceCandidate(candidate) {
        const message = {
            type: 'ice-candidate',
            peer_id: this.peerId,
            message_id: this.generateMessageId(),
            timestamp: Date.now(),
            data: {
                candidate: {
                    candidate: candidate.candidate,
                    sdpMid: candidate.sdpMid,
                    sdpMLineIndex: candidate.sdpMLineIndex,
                    usernameFragment: candidate.usernameFragment
                }
            }
        };
        
        this.sendMessage(message);
    }
    
    /**
     * 发送 Ping 消息
     */
    sendPing() {
        const message = {
            type: 'ping',
            peer_id: this.peerId,
            message_id: this.generateMessageId(),
            timestamp: Date.now(),
            data: {
                client_state: 'connected',
                sequence: this.messageId
            }
        };
        
        this.sendMessage(message);
    }
    
    /**
     * 请求统计信息
     */
    requestStats() {
        const message = {
            type: 'get-stats',
            peer_id: this.peerId,
            message_id: this.generateMessageId(),
            timestamp: Date.now(),
            data: {
                stats_type: ['webrtc', 'system', 'network']
            }
        };
        
        this.sendMessage(message);
    }
    
    /**
     * 发送鼠标事件
     */
    sendMouseEvent(eventType, x, y, button = 'left', modifiers = []) {
        const message = {
            type: eventType, // 'mouse-click' 或 'mouse-move'
            peer_id: this.peerId,
            message_id: this.generateMessageId(),
            timestamp: Date.now(),
            data: {
                x: x,
                y: y,
                button: button,
                action: 'down',
                modifiers: modifiers
            }
        };
        
        this.sendMessage(message);
    }
    
    /**
     * 发送键盘事件
     */
    sendKeyEvent(key, keyCode, action = 'down', modifiers = []) {
        const message = {
            type: 'key-press',
            peer_id: this.peerId,
            message_id: this.generateMessageId(),
            timestamp: Date.now(),
            data: {
                key: key,
                key_code: keyCode,
                action: action,
                modifiers: modifiers,
                repeat: false
            }
        };
        
        this.sendMessage(message);
    }
    
    /**
     * 处理接收到的消息
     */
    handleMessage(data) {
        try {
            const message = JSON.parse(data);
            console.log('📨 Received message:', message.type);
            
            switch (message.type) {
                case 'welcome':
                    this.handleWelcome(message);
                    break;
                case 'offer':
                    this.handleOffer(message);
                    break;
                case 'ice-candidate':
                    this.handleIceCandidate(message);
                    break;
                case 'pong':
                    this.handlePong(message);
                    break;
                case 'stats':
                    this.handleStats(message);
                    break;
                case 'error':
                    this.handleError(message);
                    break;
                default:
                    console.warn('⚠️ Unknown message type:', message.type);
            }
        } catch (error) {
            console.error('❌ Failed to parse message:', error);
        }
    }
    
    /**
     * 处理欢迎消息
     */
    handleWelcome(message) {
        console.log('👋 Welcome message received:', message.data);
        
        if (message.data && message.data.session_config) {
            const config = message.data.session_config;
            if (config.heartbeat_interval) {
                this.heartbeatInterval = config.heartbeat_interval;
                this.restartHeartbeat();
            }
        }
    }
    
    /**
     * 处理 Offer 消息
     */
    async handleOffer(message) {
        if (!message.data || !message.data.sdp) {
            console.error('❌ Invalid offer message');
            return;
        }
        
        console.log('📞 Received WebRTC offer');
        
        try {
            // 创建 PeerConnection
            await this.createPeerConnection();
            
            // 设置远程描述
            const offer = new RTCSessionDescription(message.data.sdp);
            await this.peerConnection.setRemoteDescription(offer);
            
            // 创建并发送 Answer
            const answer = await this.peerConnection.createAnswer();
            await this.peerConnection.setLocalDescription(answer);
            
            await this.sendAnswer(answer);
            
            console.log('✅ WebRTC negotiation completed');
            this.onOffer(offer);
            
        } catch (error) {
            console.error('❌ Failed to handle offer:', error);
            this.onError(error);
        }
    }
    
    /**
     * 处理 ICE 候选
     */
    async handleIceCandidate(message) {
        if (!message.data || !message.data.candidate) {
            console.error('❌ Invalid ICE candidate message');
            return;
        }
        
        try {
            const candidate = new RTCIceCandidate(message.data.candidate);
            await this.peerConnection.addIceCandidate(candidate);
            console.log('🧊 ICE candidate added');
        } catch (error) {
            console.error('❌ Failed to add ICE candidate:', error);
        }
    }
    
    /**
     * 处理 Pong 消息
     */
    handlePong(message) {
        const now = Date.now();
        const pingTime = message.data ? message.data.ping_timestamp : null;
        
        if (pingTime) {
            const latency = now - pingTime;
            console.log(`🏓 Pong received, latency: ${latency}ms`);
        }
    }
    
    /**
     * 处理统计信息
     */
    handleStats(message) {
        if (message.data) {
            console.log('📊 Stats received:', message.data);
            this.onStats(message.data);
        }
    }
    
    /**
     * 处理错误消息
     */
    handleError(message) {
        const error = message.error || { message: 'Unknown server error' };
        console.error('❌ Server error:', error);
        this.onError(error);
    }
    
    /**
     * 创建 PeerConnection
     */
    async createPeerConnection() {
        if (this.peerConnection) {
            return;
        }
        
        const config = {
            iceServers: [
                { urls: 'stun:stun.l.google.com:19302' }
            ]
        };
        
        this.peerConnection = new RTCPeerConnection(config);
        
        // ICE 候选事件
        this.peerConnection.onicecandidate = (event) => {
            if (event.candidate) {
                this.sendIceCandidate(event.candidate);
            }
        };
        
        // 连接状态变化
        this.peerConnection.onconnectionstatechange = () => {
            console.log('🔗 Connection state:', this.peerConnection.connectionState);
        };
        
        // 媒体流事件
        this.peerConnection.ontrack = (event) => {
            console.log('🎥 Remote track received');
            const video = document.getElementById('remoteVideo');
            if (video) {
                video.srcObject = event.streams[0];
            }
        };
    }
    
    /**
     * 开始心跳
     */
    startHeartbeat() {
        this.stopHeartbeat();
        this.heartbeatTimer = setInterval(() => {
            if (this.isConnected) {
                this.sendPing();
            }
        }, this.heartbeatInterval);
    }
    
    /**
     * 停止心跳
     */
    stopHeartbeat() {
        if (this.heartbeatTimer) {
            clearInterval(this.heartbeatTimer);
            this.heartbeatTimer = null;
        }
    }
    
    /**
     * 重启心跳
     */
    restartHeartbeat() {
        this.stopHeartbeat();
        this.startHeartbeat();
    }
    
    /**
     * 发送消息
     */
    sendMessage(message) {
        if (!this.isConnected || !this.ws) {
            console.error('❌ Not connected to signaling server');
            return;
        }
        
        try {
            this.ws.send(JSON.stringify(message));
            console.log('📤 Message sent:', message.type);
        } catch (error) {
            console.error('❌ Failed to send message:', error);
        }
    }
    
    /**
     * 生成对等端 ID
     */
    generatePeerId() {
        return Math.floor(Math.random() * 1000000);
    }
    
    /**
     * 生成消息 ID
     */
    generateMessageId() {
        return `msg_${Date.now()}_${++this.messageId}`;
    }
}

// 使用示例
const client = new WebRTCSignalingClient('ws://localhost:8080/ws', {
    peerId: 12345,
    onConnected: () => {
        console.log('🎉 Client connected successfully');
        // 连接成功后请求 offer
        setTimeout(() => {
            client.requestOffer();
        }, 1000);
    },
    onDisconnected: (event) => {
        console.log('👋 Client disconnected:', event);
    },
    onError: (error) => {
        console.error('💥 Client error:', error);
    },
    onOffer: (offer) => {
        console.log('📞 Offer received, starting video stream');
    },
    onStats: (stats) => {
        console.log('📊 Performance stats:', stats);
    }
});

// 连接到服务器
client.connect().catch(console.error);

// 页面卸载时断开连接
window.addEventListener('beforeunload', () => {
    client.disconnect();
});
```

### 高级集成示例

```javascript
/**
 * 高级 WebRTC 信令客户端
 * 包含重连机制、错误恢复和性能监控
 */
class AdvancedWebRTCClient extends WebRTCSignalingClient {
    constructor(serverUrl, options = {}) {
        super(serverUrl, options);
        
        // 重连配置
        this.maxRetries = options.maxRetries || 5;
        this.retryDelay = options.retryDelay || 3000;
        this.retryCount = 0;
        this.autoReconnect = options.autoReconnect !== false;
        
        // 性能监控
        this.metrics = {
            messagesReceived: 0,
            messagesSent: 0,
            errors: 0,
            connectionTime: null,
            lastLatency: null
        };
        
        // 消息队列（离线时缓存）
        this.messageQueue = [];
        this.maxQueueSize = options.maxQueueSize || 100;
    }
    
    /**
     * 带重连的连接方法
     */
    async connectWithRetry() {
        for (let attempt = 0; attempt <= this.maxRetries; attempt++) {
            try {
                await this.connect();
                this.retryCount = 0;
                this.metrics.connectionTime = Date.now();
                return;
            } catch (error) {
                console.error(`❌ Connection attempt ${attempt + 1} failed:`, error);
                
                if (attempt < this.maxRetries) {
                    const delay = this.retryDelay * Math.pow(2, attempt);
                    console.log(`⏳ Retrying in ${delay}ms...`);
                    await this.sleep(delay);
                } else {
                    throw new Error(`Failed to connect after ${this.maxRetries + 1} attempts`);
                }
            }
        }
    }
    
    /**
     * 自动重连处理
     */
    handleDisconnection(event) {
        super.onDisconnected(event);
        
        if (this.autoReconnect && !event.wasClean) {
            console.log('🔄 Auto-reconnecting...');
            setTimeout(() => {
                this.connectWithRetry().catch(error => {
                    console.error('❌ Auto-reconnect failed:', error);
                });
            }, this.retryDelay);
        }
    }
    
    /**
     * 增强的消息发送（支持离线队列）
     */
    sendMessage(message) {
        this.metrics.messagesSent++;
        
        if (!this.isConnected) {
            if (this.messageQueue.length < this.maxQueueSize) {
                this.messageQueue.push(message);
                console.log('📦 Message queued (offline)');
            } else {
                console.warn('⚠️ Message queue full, dropping message');
            }
            return;
        }
        
        super.sendMessage(message);
    }
    
    /**
     * 处理连接恢复后的消息队列
     */
    processMessageQueue() {
        if (this.messageQueue.length === 0) return;
        
        console.log(`📤 Processing ${this.messageQueue.length} queued messages`);
        
        while (this.messageQueue.length > 0 && this.isConnected) {
            const message = this.messageQueue.shift();
            super.sendMessage(message);
        }
    }
    
    /**
     * 增强的消息处理（包含指标收集）
     */
    handleMessage(data) {
        this.metrics.messagesReceived++;
        
        try {
            const message = JSON.parse(data);
            
            // 记录延迟
            if (message.type === 'pong' && message.data && message.data.ping_timestamp) {
                this.metrics.lastLatency = Date.now() - message.data.ping_timestamp;
            }
            
            super.handleMessage(data);
        } catch (error) {
            this.metrics.errors++;
            console.error('❌ Message handling error:', error);
        }
    }
    
    /**
     * 获取性能指标
     */
    getMetrics() {
        return {
            ...this.metrics,
            uptime: this.metrics.connectionTime ? Date.now() - this.metrics.connectionTime : 0,
            queueSize: this.messageQueue.length,
            isConnected: this.isConnected
        };
    }
    
    /**
     * 重置指标
     */
    resetMetrics() {
        this.metrics = {
            messagesReceived: 0,
            messagesSent: 0,
            errors: 0,
            connectionTime: Date.now(),
            lastLatency: null
        };
    }
    
    /**
     * 工具方法：延迟
     */
    sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }
}

// 使用高级客户端
const advancedClient = new AdvancedWebRTCClient('ws://localhost:8080/ws', {
    peerId: 12345,
    maxRetries: 3,
    retryDelay: 2000,
    autoReconnect: true,
    maxQueueSize: 50
});

// 连接并处理错误
advancedClient.connectWithRetry().then(() => {
    console.log('🎉 Advanced client connected');
    
    // 定期请求统计信息
    setInterval(() => {
        advancedClient.requestStats();
    }, 10000);
    
    // 定期显示客户端指标
    setInterval(() => {
        console.log('📊 Client metrics:', advancedClient.getMetrics());
    }, 30000);
    
}).catch(error => {
    console.error('💥 Failed to connect:', error);
});
```

## Python 客户端示例

```python
"""
Python WebRTC 信令客户端示例
支持异步操作和自动重连
"""

import asyncio
import json
import logging
import time
import websockets
from typing import Dict, Any, Optional, Callable

class WebRTCSignalingClient:
    def __init__(self, server_url: str, peer_id: int = None, **options):
        self.server_url = server_url
        self.peer_id = peer_id or self._generate_peer_id()
        self.websocket = None
        self.is_connected = False
        self.message_id = 0
        
        # 配置选项
        self.heartbeat_interval = options.get('heartbeat_interval', 30)
        self.max_retries = options.get('max_retries', 5)
        self.retry_delay = options.get('retry_delay', 3)
        
        # 事件回调
        self.on_connected = options.get('on_connected', lambda: None)
        self.on_disconnected = options.get('on_disconnected', lambda: None)
        self.on_error = options.get('on_error', lambda e: logging.error(f"Error: {e}"))
        self.on_offer = options.get('on_offer', lambda offer: None)
        self.on_stats = options.get('on_stats', lambda stats: None)
        
        # 内部状态
        self._heartbeat_task = None
        self._retry_count = 0
        
        # 设置日志
        logging.basicConfig(level=logging.INFO)
        self.logger = logging.getLogger(__name__)
    
    async def connect(self):
        """连接到信令服务器"""
        try:
            self.logger.info(f"Connecting to {self.server_url}")
            self.websocket = await websockets.connect(self.server_url)
            self.is_connected = True
            
            # 发送 Hello 消息
            await self.send_hello()
            
            # 启动心跳
            self._heartbeat_task = asyncio.create_task(self._heartbeat_loop())
            
            # 启动消息监听
            asyncio.create_task(self._message_loop())
            
            self.logger.info("✅ Connected successfully")
            self.on_connected()
            
        except Exception as e:
            self.logger.error(f"❌ Connection failed: {e}")
            self.on_error(e)
            raise
    
    async def disconnect(self):
        """断开连接"""
        self.is_connected = False
        
        if self._heartbeat_task:
            self._heartbeat_task.cancel()
        
        if self.websocket:
            await self.websocket.close()
            self.websocket = None
        
        self.logger.info("👋 Disconnected")
        self.on_disconnected()
    
    async def connect_with_retry(self):
        """带重连机制的连接"""
        for attempt in range(self.max_retries + 1):
            try:
                await self.connect()
                self._retry_count = 0
                return
            except Exception as e:
                self.logger.error(f"❌ Connection attempt {attempt + 1} failed: {e}")
                
                if attempt < self.max_retries:
                    delay = self.retry_delay * (2 ** attempt)
                    self.logger.info(f"⏳ Retrying in {delay}s...")
                    await asyncio.sleep(delay)
                else:
                    raise Exception(f"Failed to connect after {self.max_retries + 1} attempts")
    
    async def send_hello(self):
        """发送 Hello 消息"""
        message = {
            'type': 'hello',
            'peer_id': str(self.peer_id),
            'message_id': self._generate_message_id(),
            'timestamp': int(time.time() * 1000),
            'data': {
                'client_info': {
                    'platform': 'python',
                    'version': '1.0.0'
                },
                'capabilities': ['webrtc', 'video', 'audio'],
                'supported_protocols': ['gstreamer-1.0']
            }
        }
        
        await self.send_message(message)
    
    async def request_offer(self, constraints: Dict[str, bool] = None):
        """请求 WebRTC Offer"""
        if constraints is None:
            constraints = {'video': True, 'audio': True}
        
        message = {
            'type': 'request-offer',
            'peer_id': str(self.peer_id),
            'message_id': self._generate_message_id(),
            'timestamp': int(time.time() * 1000),
            'data': {
                'constraints': constraints,
                'codec_preferences': ['H264', 'VP8']
            }
        }
        
        self.logger.info("📞 Requesting WebRTC offer")
        await self.send_message(message)
    
    async def send_answer(self, answer: Dict[str, Any]):
        """发送 Answer"""
        message = {
            'type': 'answer',
            'peer_id': str(self.peer_id),
            'message_id': self._generate_message_id(),
            'timestamp': int(time.time() * 1000),
            'data': {
                'sdp': answer
            }
        }
        
        self.logger.info("📤 Sending answer")
        await self.send_message(message)
    
    async def send_ice_candidate(self, candidate: Dict[str, Any]):
        """发送 ICE 候选"""
        message = {
            'type': 'ice-candidate',
            'peer_id': str(self.peer_id),
            'message_id': self._generate_message_id(),
            'timestamp': int(time.time() * 1000),
            'data': {
                'candidate': candidate
            }
        }
        
        await self.send_message(message)
    
    async def send_ping(self):
        """发送 Ping 消息"""
        message = {
            'type': 'ping',
            'peer_id': str(self.peer_id),
            'message_id': self._generate_message_id(),
            'timestamp': int(time.time() * 1000),
            'data': {
                'client_state': 'connected',
                'sequence': self.message_id
            }
        }
        
        await self.send_message(message)
    
    async def request_stats(self):
        """请求统计信息"""
        message = {
            'type': 'get-stats',
            'peer_id': str(self.peer_id),
            'message_id': self._generate_message_id(),
            'timestamp': int(time.time() * 1000),
            'data': {
                'stats_type': ['webrtc', 'system']
            }
        }
        
        await self.send_message(message)
    
    async def send_message(self, message: Dict[str, Any]):
        """发送消息"""
        if not self.is_connected or not self.websocket:
            self.logger.error("❌ Not connected to server")
            return
        
        try:
            await self.websocket.send(json.dumps(message))
            self.logger.debug(f"📤 Message sent: {message['type']}")
        except Exception as e:
            self.logger.error(f"❌ Failed to send message: {e}")
            self.on_error(e)
    
    async def _message_loop(self):
        """消息接收循环"""
        try:
            async for message in self.websocket:
                await self._handle_message(message)
        except websockets.exceptions.ConnectionClosed:
            self.logger.info("Connection closed")
            self.is_connected = False
        except Exception as e:
            self.logger.error(f"❌ Message loop error: {e}")
            self.on_error(e)
    
    async def _handle_message(self, data: str):
        """处理接收到的消息"""
        try:
            message = json.loads(data)
            message_type = message.get('type')
            
            self.logger.debug(f"📨 Received message: {message_type}")
            
            if message_type == 'welcome':
                await self._handle_welcome(message)
            elif message_type == 'offer':
                await self._handle_offer(message)
            elif message_type == 'ice-candidate':
                await self._handle_ice_candidate(message)
            elif message_type == 'pong':
                await self._handle_pong(message)
            elif message_type == 'stats':
                await self._handle_stats(message)
            elif message_type == 'error':
                await self._handle_error(message)
            else:
                self.logger.warning(f"⚠️ Unknown message type: {message_type}")
                
        except json.JSONDecodeError as e:
            self.logger.error(f"❌ Failed to parse message: {e}")
        except Exception as e:
            self.logger.error(f"❌ Message handling error: {e}")
            self.on_error(e)
    
    async def _handle_welcome(self, message: Dict[str, Any]):
        """处理欢迎消息"""
        self.logger.info("👋 Welcome message received")
        
        data = message.get('data', {})
        session_config = data.get('session_config', {})
        
        if 'heartbeat_interval' in session_config:
            self.heartbeat_interval = session_config['heartbeat_interval'] / 1000
    
    async def _handle_offer(self, message: Dict[str, Any]):
        """处理 Offer 消息"""
        data = message.get('data', {})
        sdp = data.get('sdp')
        
        if not sdp:
            self.logger.error("❌ Invalid offer message")
            return
        
        self.logger.info("📞 Received WebRTC offer")
        self.on_offer(sdp)
    
    async def _handle_ice_candidate(self, message: Dict[str, Any]):
        """处理 ICE 候选"""
        data = message.get('data', {})
        candidate = data.get('candidate')
        
        if candidate:
            self.logger.info("🧊 ICE candidate received")
    
    async def _handle_pong(self, message: Dict[str, Any]):
        """处理 Pong 消息"""
        data = message.get('data', {})
        ping_timestamp = data.get('ping_timestamp')
        
        if ping_timestamp:
            latency = int(time.time() * 1000) - ping_timestamp
            self.logger.info(f"🏓 Pong received, latency: {latency}ms")
    
    async def _handle_stats(self, message: Dict[str, Any]):
        """处理统计信息"""
        data = message.get('data')
        if data:
            self.logger.info("📊 Stats received")
            self.on_stats(data)
    
    async def _handle_error(self, message: Dict[str, Any]):
        """处理错误消息"""
        error = message.get('error', {})
        error_msg = error.get('message', 'Unknown server error')
        self.logger.error(f"❌ Server error: {error_msg}")
        self.on_error(Exception(error_msg))
    
    async def _heartbeat_loop(self):
        """心跳循环"""
        while self.is_connected:
            try:
                await self.send_ping()
                await asyncio.sleep(self.heartbeat_interval)
            except asyncio.CancelledError:
                break
            except Exception as e:
                self.logger.error(f"❌ Heartbeat error: {e}")
                break
    
    def _generate_peer_id(self) -> int:
        """生成对等端 ID"""
        import random
        return random.randint(100000, 999999)
    
    def _generate_message_id(self) -> str:
        """生成消息 ID"""
        self.message_id += 1
        return f"msg_{int(time.time() * 1000)}_{self.message_id}"

# 使用示例
async def main():
    def on_connected():
        print("🎉 Client connected successfully")
    
    def on_offer(offer):
        print(f"📞 Offer received: {offer['type']}")
    
    def on_stats(stats):
        print(f"📊 Stats: {stats}")
    
    def on_error(error):
        print(f"💥 Error: {error}")
    
    client = WebRTCSignalingClient(
        'ws://localhost:8080/ws',
        peer_id=12345,
        on_connected=on_connected,
        on_offer=on_offer,
        on_stats=on_stats,
        on_error=on_error
    )
    
    try:
        # 连接到服务器
        await client.connect_with_retry()
        
        # 请求 offer
        await asyncio.sleep(1)
        await client.request_offer()
        
        # 定期请求统计信息
        while client.is_connected:
            await asyncio.sleep(10)
            await client.request_stats()
            
    except KeyboardInterrupt:
        print("👋 Shutting down...")
    finally:
        await client.disconnect()

if __name__ == "__main__":
    asyncio.run(main())
```

## Go 客户端示例

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "math/rand"
    "net/url"
    "time"
    
    "github.com/gorilla/websocket"
)

// SignalingMessage 信令消息结构
type SignalingMessage struct {
    Type      string                 `json:"type"`
    PeerID    string                 `json:"peer_id"`
    MessageID string                 `json:"message_id"`
    Timestamp int64                  `json:"timestamp"`
    Data      map[string]interface{} `json:"data,omitempty"`
    Error     *SignalingError        `json:"error,omitempty"`
}

// SignalingError 错误结构
type SignalingError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Details string `json:"details,omitempty"`
}

// WebRTCClient WebRTC 信令客户端
type WebRTCClient struct {
    serverURL   string
    peerID      string
    conn        *websocket.Conn
    isConnected bool
    messageID   int64
    
    // 配置
    heartbeatInterval time.Duration
    maxRetries        int
    retryDelay        time.Duration
    
    // 通道
    sendChan    chan SignalingMessage
    stopChan    chan struct{}
    
    // 回调函数
    OnConnected    func()
    OnDisconnected func()
    OnError        func(error)
    OnOffer        func(map[string]interface{})
    OnStats        func(map[string]interface{})
    
    logger *log.Logger
}

// NewWebRTCClient 创建新的 WebRTC 客户端
func NewWebRTCClient(serverURL string, peerID string) *WebRTCClient {
    if peerID == "" {
        peerID = fmt.Sprintf("%d", rand.Intn(1000000))
    }
    
    return &WebRTCClient{
        serverURL:         serverURL,
        peerID:           peerID,
        heartbeatInterval: 30 * time.Second,
        maxRetries:       5,
        retryDelay:       3 * time.Second,
        sendChan:         make(chan SignalingMessage, 100),
        stopChan:         make(chan struct{}),
        logger:           log.Default(),
    }
}

// Connect 连接到信令服务器
func (c *WebRTCClient) Connect(ctx context.Context) error {
    u, err := url.Parse(c.serverURL)
    if err != nil {
        return fmt.Errorf("invalid server URL: %w", err)
    }
    
    c.logger.Printf("Connecting to %s", c.serverURL)
    
    conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
    if err != nil {
        return fmt.Errorf("failed to connect: %w", err)
    }
    
    c.conn = conn
    c.isConnected = true
    
    // 启动消息处理协程
    go c.readLoop()
    go c.writeLoop()
    go c.heartbeatLoop()
    
    // 发送 Hello 消息
    if err := c.SendHello(); err != nil {
        return fmt.Errorf("failed to send hello: %w", err)
    }
    
    c.logger.Println("✅ Connected successfully")
    if c.OnConnected != nil {
        c.OnConnected()
    }
    
    return nil
}

// ConnectWithRetry 带重连机制的连接
func (c *WebRTCClient) ConnectWithRetry(ctx context.Context) error {
    for attempt := 0; attempt <= c.maxRetries; attempt++ {
        err := c.Connect(ctx)
        if err == nil {
            return nil
        }
        
        c.logger.Printf("❌ Connection attempt %d failed: %v", attempt+1, err)
        
        if attempt < c.maxRetries {
            delay := c.retryDelay * time.Duration(1<<attempt)
            c.logger.Printf("⏳ Retrying in %v...", delay)
            
            select {
            case <-time.After(delay):
                continue
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
    
    return fmt.Errorf("failed to connect after %d attempts", c.maxRetries+1)
}

// Disconnect 断开连接
func (c *WebRTCClient) Disconnect() {
    c.isConnected = false
    close(c.stopChan)
    
    if c.conn != nil {
        c.conn.Close()
        c.conn = nil
    }
    
    c.logger.Println("👋 Disconnected")
    if c.OnDisconnected != nil {
        c.OnDisconnected()
    }
}

// SendHello 发送 Hello 消息
func (c *WebRTCClient) SendHello() error {
    message := SignalingMessage{
        Type:      "hello",
        PeerID:    c.peerID,
        MessageID: c.generateMessageID(),
        Timestamp: time.Now().Unix(),
        Data: map[string]interface{}{
            "client_info": map[string]interface{}{
                "platform": "go",
                "version":  "1.0.0",
            },
            "capabilities": []string{"webrtc", "video", "audio"},
            "supported_protocols": []string{"gstreamer-1.0"},
        },
    }
    
    return c.sendMessage(message)
}

// RequestOffer 请求 WebRTC Offer
func (c *WebRTCClient) RequestOffer(constraints map[string]bool) error {
    if constraints == nil {
        constraints = map[string]bool{
            "video": true,
            "audio": true,
        }
    }
    
    message := SignalingMessage{
        Type:      "request-offer",
        PeerID:    c.peerID,
        MessageID: c.generateMessageID(),
        Timestamp: time.Now().Unix(),
        Data: map[string]interface{}{
            "constraints": constraints,
            "codec_preferences": []string{"H264", "VP8"},
        },
    }
    
    c.logger.Println("📞 Requesting WebRTC offer")
    return c.sendMessage(message)
}

// SendAnswer 发送 Answer
func (c *WebRTCClient) SendAnswer(answer map[string]interface{}) error {
    message := SignalingMessage{
        Type:      "answer",
        PeerID:    c.peerID,
        MessageID: c.generateMessageID(),
        Timestamp: time.Now().Unix(),
        Data: map[string]interface{}{
            "sdp": answer,
        },
    }
    
    c.logger.Println("📤 Sending answer")
    return c.sendMessage(message)
}

// SendICECandidate 发送 ICE 候选
func (c *WebRTCClient) SendICECandidate(candidate map[string]interface{}) error {
    message := SignalingMessage{
        Type:      "ice-candidate",
        PeerID:    c.peerID,
        MessageID: c.generateMessageID(),
        Timestamp: time.Now().Unix(),
        Data: map[string]interface{}{
            "candidate": candidate,
        },
    }
    
    return c.sendMessage(message)
}

// SendPing 发送 Ping 消息
func (c *WebRTCClient) SendPing() error {
    message := SignalingMessage{
        Type:      "ping",
        PeerID:    c.peerID,
        MessageID: c.generateMessageID(),
        Timestamp: time.Now().Unix(),
        Data: map[string]interface{}{
            "client_state": "connected",
            "sequence":     c.messageID,
        },
    }
    
    return c.sendMessage(message)
}

// RequestStats 请求统计信息
func (c *WebRTCClient) RequestStats() error {
    message := SignalingMessage{
        Type:      "get-stats",
        PeerID:    c.peerID,
        MessageID: c.generateMessageID(),
        Timestamp: time.Now().Unix(),
        Data: map[string]interface{}{
            "stats_type": []string{"webrtc", "system"},
        },
    }
    
    return c.sendMessage(message)
}

// 内部方法

func (c *WebRTCClient) sendMessage(message SignalingMessage) error {
    if !c.isConnected {
        return fmt.Errorf("not connected to server")
    }
    
    select {
    case c.sendChan <- message:
        return nil
    case <-c.stopChan:
        return fmt.Errorf("client stopped")
    default:
        return fmt.Errorf("send channel full")
    }
}

func (c *WebRTCClient) readLoop() {
    defer func() {
        if r := recover(); r != nil {
            c.logger.Printf("❌ Read loop panic: %v", r)
        }
    }()
    
    for c.isConnected {
        _, messageBytes, err := c.conn.ReadMessage()
        if err != nil {
            if c.isConnected {
                c.logger.Printf("❌ Read error: %v", err)
                if c.OnError != nil {
                    c.OnError(err)
                }
            }
            break
        }
        
        c.handleMessage(messageBytes)
    }
}

func (c *WebRTCClient) writeLoop() {
    defer func() {
        if r := recover(); r != nil {
            c.logger.Printf("❌ Write loop panic: %v", r)
        }
    }()
    
    for {
        select {
        case message := <-c.sendChan:
            messageBytes, err := json.Marshal(message)
            if err != nil {
                c.logger.Printf("❌ Failed to marshal message: %v", err)
                continue
            }
            
            if err := c.conn.WriteMessage(websocket.TextMessage, messageBytes); err != nil {
                c.logger.Printf("❌ Failed to send message: %v", err)
                if c.OnError != nil {
                    c.OnError(err)
                }
                return
            }
            
            c.logger.Printf("📤 Message sent: %s", message.Type)
            
        case <-c.stopChan:
            return
        }
    }
}

func (c *WebRTCClient) heartbeatLoop() {
    ticker := time.NewTicker(c.heartbeatInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            if c.isConnected {
                if err := c.SendPing(); err != nil {
                    c.logger.Printf("❌ Failed to send ping: %v", err)
                }
            }
        case <-c.stopChan:
            return
        }
    }
}

func (c *WebRTCClient) handleMessage(messageBytes []byte) {
    var message SignalingMessage
    if err := json.Unmarshal(messageBytes, &message); err != nil {
        c.logger.Printf("❌ Failed to parse message: %v", err)
        return
    }
    
    c.logger.Printf("📨 Received message: %s", message.Type)
    
    switch message.Type {
    case "welcome":
        c.handleWelcome(message)
    case "offer":
        c.handleOffer(message)
    case "ice-candidate":
        c.handleICECandidate(message)
    case "pong":
        c.handlePong(message)
    case "stats":
        c.handleStats(message)
    case "error":
        c.handleError(message)
    default:
        c.logger.Printf("⚠️ Unknown message type: %s", message.Type)
    }
}

func (c *WebRTCClient) handleWelcome(message SignalingMessage) {
    c.logger.Println("👋 Welcome message received")
    
    if sessionConfig, exists := message.Data["session_config"]; exists {
        if config, ok := sessionConfig.(map[string]interface{}); ok {
            if interval, exists := config["heartbeat_interval"]; exists {
                if intervalMs, ok := interval.(float64); ok {
                    c.heartbeatInterval = time.Duration(intervalMs) * time.Millisecond
                }
            }
        }
    }
}

func (c *WebRTCClient) handleOffer(message SignalingMessage) {
    if sdp, exists := message.Data["sdp"]; exists {
        c.logger.Println("📞 Received WebRTC offer")
        if c.OnOffer != nil {
            if sdpData, ok := sdp.(map[string]interface{}); ok {
                c.OnOffer(sdpData)
            }
        }
    }
}

func (c *WebRTCClient) handleICECandidate(message SignalingMessage) {
    if candidate, exists := message.Data["candidate"]; exists {
        c.logger.Println("🧊 ICE candidate received")
        _ = candidate // 处理 ICE 候选
    }
}

func (c *WebRTCClient) handlePong(message SignalingMessage) {
    if pingTimestamp, exists := message.Data["ping_timestamp"]; exists {
        if timestamp, ok := pingTimestamp.(float64); ok {
            latency := time.Now().Unix() - int64(timestamp)
            c.logger.Printf("🏓 Pong received, latency: %dms", latency)
        }
    }
}

func (c *WebRTCClient) handleStats(message SignalingMessage) {
    c.logger.Println("📊 Stats received")
    if c.OnStats != nil {
        c.OnStats(message.Data)
    }
}

func (c *WebRTCClient) handleError(message SignalingMessage) {
    if message.Error != nil {
        err := fmt.Errorf("server error: %s - %s", message.Error.Code, message.Error.Message)
        c.logger.Printf("❌ %v", err)
        if c.OnError != nil {
            c.OnError(err)
        }
    }
}

func (c *WebRTCClient) generateMessageID() string {
    c.messageID++
    return fmt.Sprintf("msg_%d_%d", time.Now().UnixMilli(), c.messageID)
}

// 使用示例
func main() {
    client := NewWebRTCClient("ws://localhost:8080/ws", "12345")
    
    // 设置回调
    client.OnConnected = func() {
        fmt.Println("🎉 Client connected successfully")
        
        // 连接成功后请求 offer
        time.AfterFunc(1*time.Second, func() {
            client.RequestOffer(nil)
        })
    }
    
    client.OnOffer = func(offer map[string]interface{}) {
        fmt.Printf("📞 Offer received: %v\n", offer["type"])
    }
    
    client.OnStats = func(stats map[string]interface{}) {
        fmt.Printf("📊 Stats: %+v\n", stats)
    }
    
    client.OnError = func(err error) {
        fmt.Printf("💥 Error: %v\n", err)
    }
    
    // 连接到服务器
    ctx := context.Background()
    if err := client.ConnectWithRetry(ctx); err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    
    // 定期请求统计信息
    go func() {
        ticker := time.NewTicker(10 * time.Second)
        defer ticker.Stop()
        
        for range ticker.C {
            if client.isConnected {
                client.RequestStats()
            }
        }
    }()
    
    // 等待中断信号
    select {}
}
```

## 集成最佳实践

### 1. 错误处理和重连

```javascript
// 实现指数退避重连
class ReconnectManager {
    constructor(client, options = {}) {
        this.client = client;
        this.maxRetries = options.maxRetries || 5;
        this.baseDelay = options.baseDelay || 1000;
        this.maxDelay = options.maxDelay || 30000;
        this.retryCount = 0;
    }
    
    async reconnect() {
        if (this.retryCount >= this.maxRetries) {
            throw new Error('Max reconnection attempts exceeded');
        }
        
        const delay = Math.min(
            this.baseDelay * Math.pow(2, this.retryCount),
            this.maxDelay
        );
        
        console.log(`Reconnecting in ${delay}ms (attempt ${this.retryCount + 1})`);
        await this.sleep(delay);
        
        try {
            await this.client.connect();
            this.retryCount = 0; // 重置计数器
        } catch (error) {
            this.retryCount++;
            throw error;
        }
    }
    
    sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }
}
```

### 2. 消息队列和离线处理

```javascript
// 消息队列管理器
class MessageQueue {
    constructor(maxSize = 100) {
        this.queue = [];
        this.maxSize = maxSize;
    }
    
    enqueue(message) {
        if (this.queue.length >= this.maxSize) {
            this.queue.shift(); // 移除最旧的消息
        }
        this.queue.push({
            message,
            timestamp: Date.now()
        });
    }
    
    dequeue() {
        return this.queue.shift();
    }
    
    clear() {
        this.queue = [];
    }
    
    size() {
        return this.queue.length;
    }
}
```

### 3. 性能监控

```javascript
// 性能监控器
class PerformanceMonitor {
    constructor() {
        this.metrics = {
            messagesSent: 0,
            messagesReceived: 0,
            errors: 0,
            latencies: [],
            connectionTime: null
        };
    }
    
    recordMessageSent() {
        this.metrics.messagesSent++;
    }
    
    recordMessageReceived() {
        this.metrics.messagesReceived++;
    }
    
    recordError() {
        this.metrics.errors++;
    }
    
    recordLatency(latency) {
        this.metrics.latencies.push(latency);
        // 只保留最近100个延迟记录
        if (this.metrics.latencies.length > 100) {
            this.metrics.latencies.shift();
        }
    }
    
    getAverageLatency() {
        if (this.metrics.latencies.length === 0) return 0;
        const sum = this.metrics.latencies.reduce((a, b) => a + b, 0);
        return sum / this.metrics.latencies.length;
    }
    
    getMetrics() {
        return {
            ...this.metrics,
            averageLatency: this.getAverageLatency(),
            errorRate: this.metrics.errors / (this.metrics.messagesSent || 1)
        };
    }
}
```

### 4. 配置管理

```javascript
// 配置管理器
class ConfigManager {
    constructor(defaultConfig = {}) {
        this.config = {
            serverUrl: 'ws://localhost:8080/ws',
            heartbeatInterval: 30000,
            maxRetries: 5,
            retryDelay: 3000,
            maxMessageSize: 65536,
            ...defaultConfig
        };
    }
    
    get(key) {
        return this.config[key];
    }
    
    set(key, value) {
        this.config[key] = value;
    }
    
    update(newConfig) {
        this.config = { ...this.config, ...newConfig };
    }
    
    validate() {
        const required = ['serverUrl'];
        for (const key of required) {
            if (!this.config[key]) {
                throw new Error(`Missing required config: ${key}`);
            }
        }
    }
}
```

这些示例展示了如何在不同编程语言中集成 WebRTC 信令客户端，包括基础功能、高级特性和最佳实践。客户端实现了完整的消息处理流程，支持自动重连、错误恢复和性能监控。
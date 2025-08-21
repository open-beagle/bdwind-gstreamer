/**
 * 优化的信令管理器 - 基于 selkies WebRTCDemoSignaling
 * 移除调试代码，优化性能，简化实现
 */
class SignalingManager {
  constructor(eventBus, config) {
    // 核心属性
    this.eventBus = eventBus;
    this.config = config;

    // selkies 核心属性
    this._server = null;
    this.peer_id = 1;
    this._ws_conn = null;
    this.state = 'disconnected';
    this.retry_count = 0;

    // 事件回调
    this.onstatus = null;
    this.onerror = null;
    this.ondebug = null;
    this.onice = null;
    this.onsdp = null;
    this.ondisconnect = null;

    // 连接管理 - 优化配置
    this.maxRetries = 3;
    this.retryDelay = 3000;
    this.connectionTimeout = 10000;
    this.connectionTimer = null;

    // 初始化
    this._loadConfig();
    this._setupEventCallbacks();
  }

  /**
   * 加载配置 - 优化版本
   */
  _loadConfig() {
    let serverUrl;
    
    if (this.config) {
      const signalingConfig = this.config.get("signaling", {});
      serverUrl = signalingConfig.url;
      this.maxRetries = signalingConfig.maxRetries || 3;
      this.retryDelay = signalingConfig.retryDelay || 3000;
      this.peer_id = signalingConfig.peerId || 1;
    }

    // 自动生成URL
    if (!serverUrl) {
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      serverUrl = `${protocol}//${window.location.host}/ws`;
    }

    this._server = new URL(serverUrl);
  }

  /**
   * 设置事件回调 - 优化版本
   */
  _setupEventCallbacks() {
    this.onstatus = (message) => {
      this.eventBus?.emit("signaling:status", { message });
    };

    this.onerror = (message) => {
      this.eventBus?.emit("signaling:error", { message });
    };

    this.ondebug = (message) => {
      this.eventBus?.emit("signaling:debug", { message });
    };

    this.onsdp = (sdp) => {
      this.eventBus?.emit("signaling:sdp", sdp);
    };

    this.onice = (ice) => {
      this.eventBus?.emit("signaling:ice-candidate", ice);
    };

    this.ondisconnect = () => {
      this.eventBus?.emit("signaling:disconnected");
    };
  }

  /**
   * 连接到信令服务器 - 优化版本
   */
  connect() {
    this._clearConnectionTimer();
    this.state = 'connecting';
    this._setStatus("Connecting to server.");

    try {
      this._ws_conn = new WebSocket(this._server);

      // 设置连接超时
      this.connectionTimer = setTimeout(() => {
        if (this._ws_conn?.readyState === WebSocket.CONNECTING) {
          this._setError("Connection timeout");
          this._ws_conn.close();
          this._onServerError();
        }
      }, this.connectionTimeout);

      // 绑定事件处理器
      this._ws_conn.addEventListener('open', this._onServerOpen.bind(this));
      this._ws_conn.addEventListener('error', this._onServerError.bind(this));
      this._ws_conn.addEventListener('message', this._onServerMessage.bind(this));
      this._ws_conn.addEventListener('close', this._onServerClose.bind(this));
    } catch (error) {
      this._setError("Failed to create WebSocket: " + error.message);
      this._onServerError();
    }
  }

  /**
   * 断开连接 - 优化版本
   */
  disconnect() {
    this.state = 'disconnecting';
    this._clearConnectionTimer();
    
    if (this._ws_conn) {
      this._ws_conn.close(1000, "Client disconnect");
      this._ws_conn = null;
    }
    
    this.state = 'disconnected';
    this.retry_count = 0;
  }

  /**
   * WebSocket 连接打开事件 - 优化版本
   */
  _onServerOpen() {
    this._clearConnectionTimer();
    
    // 获取分辨率信息
    const currRes = [window.screen.width, window.screen.height];
    const meta = {
      "res": `${currRes[0]}x${currRes[1]}`,
      "scale": window.devicePixelRatio
    };
    
    this.state = 'connected';
    this._ws_conn.send(`HELLO ${this.peer_id} ${btoa(JSON.stringify(meta))}`);
    this._setStatus("Registering with server, peer ID: " + this.peer_id);
    this.retry_count = 0;
    
    this.eventBus?.emit("signaling:connected", {
      peerId: this.peer_id,
      server: this._server.toString(),
      retryCount: this.retry_count
    });
  }

  /**
   * WebSocket 错误事件 - 优化版本，指数退避重连
   */
  _onServerError() {
    this._clearConnectionTimer();
    
    if (this.state === 'disconnecting') return;
    
    this.retry_count++;
    const delay = Math.min(this.retryDelay * Math.pow(2, this.retry_count - 1), 30000);
    
    this._setStatus(`Connection error, retry ${this.retry_count}/${this.maxRetries} in ${delay/1000} seconds.`);
    
    this.eventBus?.emit("signaling:reconnecting", {
      attempt: this.retry_count,
      maxAttempts: this.maxRetries,
      delay: delay
    });
    
    if (this.retry_count > this.maxRetries) {
      this._setError("Max retries reached, reloading page.");
      this.eventBus?.emit("signaling:max-retries-reached");
      
      setTimeout(() => window.location.reload(), 2000);
    } else {
      setTimeout(() => {
        if (this.state !== 'disconnecting') {
          this.connect();
        }
      }, delay);
    }
  }

  /**
   * WebSocket 消息事件 - 优化版本
   */
  _onServerMessage(event) {
    const data = event.data;

    // 处理 selkies 协议消息
    if (data === "HELLO") {
      this._setStatus("Registered with server.");
      this._setStatus("Waiting for stream.");
      this.eventBus?.emit("signaling:registered");
      return;
    }

    if (data.startsWith("ERROR")) {
      this._setStatus("Error from server: " + data);
      this.eventBus?.emit("signaling:server-error", { message: data });
      return;
    }

    // 解析 JSON 消息
    let msg;
    try {
      msg = JSON.parse(data);
    } catch (e) {
      this._setError("Failed to parse message: " + data);
      return;
    }

    // 处理 SDP 和 ICE 消息
    if (msg.sdp) {
      this._setSDP(new RTCSessionDescription(msg.sdp));
    } else if (msg.ice) {
      this._setICE(new RTCIceCandidate(msg.ice));
    } else {
      this._setError("Unhandled JSON message: " + JSON.stringify(msg));
    }
  }

  /**
   * WebSocket 关闭事件 - 优化版本
   */
  _onServerClose(event) {
    this._clearConnectionTimer();
    
    const wasConnected = this.state === 'connected';
    
    if (this.state !== 'connecting' && this.state !== 'disconnecting') {
      this.state = 'disconnected';
      
      if (!event.wasClean && event.code !== 1000 && wasConnected) {
        this._onServerError();
        return;
      }
      
      if (this.ondisconnect) {
        this.ondisconnect();
      }
    }
  }

  /**
   * 发送 ICE 候选 - 优化版本
   */
  sendICE(ice) {
    if (this._ws_conn?.readyState === WebSocket.OPEN) {
      this._ws_conn.send(JSON.stringify({ 'ice': ice }));
    }
  }

  /**
   * 发送 SDP - 优化版本
   */
  sendSDP(sdp) {
    if (this._ws_conn?.readyState === WebSocket.OPEN) {
      this._ws_conn.send(JSON.stringify({ 'sdp': sdp }));
    }
  }

  /**
   * 兼容性方法：发送消息
   */
  send(type, data = null) {
    if (!this._ws_conn || this._ws_conn.readyState !== WebSocket.OPEN) {
      return Promise.reject(new Error("WebSocket connection not ready"));
    }

    const message = { type, data, timestamp: Date.now() };

    try {
      this._ws_conn.send(JSON.stringify(message));
      return Promise.resolve();
    } catch (error) {
      return Promise.reject(error);
    }
  }

  /**
   * 获取连接状态
   */
  getConnectionState() {
    return {
      state: this.state,
      readyState: this._ws_conn ? this._ws_conn.readyState : WebSocket.CLOSED,
      retryCount: this.retry_count,
      peerId: this.peer_id,
      server: this._server ? this._server.toString() : null,
    };
  }

  /**
   * 获取连接统计信息
   */
  getConnectionStats() {
    return {
      state: this.state,
      peerId: this.peer_id,
      retryCount: this.retry_count,
      maxRetries: this.maxRetries,
      server: this._server ? this._server.toString() : null,
      isConnected: this.state === 'connected',
      canRetry: this.retry_count < this.maxRetries
    };
  }

  /**
   * 重置连接状态
   */
  reset() {
    this.retry_count = 0;
    this.state = 'disconnected';
    this._clearConnectionTimer();
    
    if (this._ws_conn) {
      this._ws_conn.close();
      this._ws_conn = null;
    }
    
    this._setStatus("Connection reset.");
  }

  // 内部方法
  _setStatus(message) {
    if (this.onstatus) this.onstatus(message);
  }

  _setDebug(message) {
    if (this.ondebug) this.ondebug(message);
  }

  _setError(message) {
    if (this.onerror) this.onerror(message);
  }

  _setSDP(sdp) {
    if (this.onsdp) this.onsdp(sdp);
  }

  _setICE(icecandidate) {
    if (this.onice) this.onice(icecandidate);
  }

  _clearConnectionTimer() {
    if (this.connectionTimer) {
      clearTimeout(this.connectionTimer);
      this.connectionTimer = null;
    }
  }

  // 兼容性方法（已弃用）
  registerHandler() {
    return () => {};
  }

  unregisterHandler() {
    // 空实现
  }
}

// 导出类
if (typeof module !== "undefined" && module.exports) {
  module.exports = SignalingManager;
} else if (typeof window !== "undefined") {
  window.SignalingManager = SignalingManager;
}
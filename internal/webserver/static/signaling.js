/**
 * SignalingManager - 信令通信管理器
 * 管理WebSocket连接、消息路由和自动重连
 */
class SignalingManager {
  constructor(eventBus, config) {
    this.eventBus = eventBus;
    this.config = config;

    // WebSocket连接
    this.ws = null;
    this.url = null;
    this.readyState = WebSocket.CLOSED;

    // 连接状态
    this.isConnecting = false;
    this.isReconnecting = false;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 10;
    this.reconnectDelay = 2000;
    this.maxReconnectDelay = 30000;
    this.connectionTimeout = 5000;

    // 心跳机制
    this.heartbeatInterval = 30000;
    this.heartbeatTimer = null;
    this.lastHeartbeat = null;
    this.heartbeatTimeout = null;

    // 消息处理
    this.messageHandlers = new Map();
    this.messageQueue = [];
    this.messageTimeout = 10000;
    this.pendingMessages = new Map();
    this.messageId = 0;

    // 连接质量监控
    this.connectionQuality = {
      latency: 0,
      packetsLost: 0,
      reconnectCount: 0,
      lastConnected: null,
      totalUptime: 0,
    };

    this._setupDefaultHandlers();
    this._loadConfig();
  }

  /**
   * 加载配置
   * @private
   */
  _loadConfig() {
    if (this.config) {
      const signalingConfig = this.config.get("signaling", {});
      this.url = signalingConfig.url;
      this.reconnectDelay = signalingConfig.reconnectDelay || 2000;
      this.maxReconnectAttempts = signalingConfig.maxReconnectAttempts || 10;
      this.heartbeatInterval = signalingConfig.heartbeatInterval || 30000;
      this.connectionTimeout = signalingConfig.connectionTimeout || 5000;
      this.messageTimeout = signalingConfig.messageTimeout || 10000;
    }

    // 如果没有配置URL，自动生成
    if (!this.url) {
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      const host = window.location.host;
      this.url = `${protocol}//${host}/ws`;
    }
  }

  /**
   * 设置默认消息处理器
   * @private
   */
  _setupDefaultHandlers() {
    this.registerHandler("welcome", (data) => {
      Logger.info("SignalingManager: 收到欢迎消息", data);
      this.eventBus?.emit("signaling:welcome", data);
    });

    this.registerHandler("offer", (data) => {
      Logger.info("SignalingManager: 收到WebRTC offer");
      this.eventBus?.emit("signaling:offer", data);
    });

    this.registerHandler("answer", (data) => {
      Logger.info("SignalingManager: 收到WebRTC answer");
      this.eventBus?.emit("signaling:answer", data);
    });

    this.registerHandler("ice-candidate", (data) => {
      Logger.info("SignalingManager: 收到ICE候选");
      this.eventBus?.emit("signaling:ice-candidate", data);
    });

    this.registerHandler("status", (data) => {
      Logger.info("SignalingManager: 状态更新", data.message);
      this.eventBus?.emit("signaling:status", data);
    });

    this.registerHandler("pong", (data) => {
      this._handlePong(data);
    });

    this.registerHandler("error", (data) => {
      Logger.error("SignalingManager: 服务器错误", data);
      this.eventBus?.emit("signaling:error", data);
    });

    this.registerHandler("answer-ack", (data) => {
      Logger.info("SignalingManager: 收到answer确认", data);
      this.eventBus?.emit("signaling:answer-ack", data);
    });

    this.registerHandler("ice-ack", (data) => {
      Logger.info("SignalingManager: 收到ICE确认", data);
      this.eventBus?.emit("signaling:ice-ack", data);
    });
  }

  /**
   * 连接到信令服务器
   */
  async connect() {
    if (this.isConnecting || this.readyState === WebSocket.OPEN) {
      Logger.warn("SignalingManager: 连接已存在或正在连接中");
      return;
    }

    this.isConnecting = true;
    Logger.info(`SignalingManager: 连接到 ${this.url}`);

    try {
      await this._createConnection();
    } catch (error) {
      this.isConnecting = false;
      Logger.error("SignalingManager: 连接失败", error);
      this._scheduleReconnect();
    }
  }

  /**
   * 断开连接
   */
  disconnect() {
    Logger.info("SignalingManager: 断开连接");

    this.isReconnecting = false;
    this._clearTimers();

    if (this.ws) {
      this.ws.close(1000, "Client disconnect");
      this.ws = null;
    }

    this.readyState = WebSocket.CLOSED;
    this.eventBus?.emit("signaling:disconnected");
  }

  /**
   * 发送消息
   */
  send(type, data = null, options = {}) {
    const message = {
      id: options.expectResponse ? ++this.messageId : undefined,
      type,
      data,
      timestamp: Date.now(),
    };

    if (this.readyState !== WebSocket.OPEN) {
      if (options.queue !== false) {
        Logger.warn("SignalingManager: 连接未就绪，消息已加入队列", type);
        this.messageQueue.push(message);
        return Promise.resolve();
      } else {
        return Promise.reject(new Error("WebSocket connection not ready"));
      }
    }

    return this._sendMessage(message, options);
  }

  /**
   * 发送消息并等待响应
   */
  sendWithResponse(type, data = null, timeout = null) {
    return new Promise((resolve, reject) => {
      const messageId = ++this.messageId;
      const timeoutMs = timeout || this.messageTimeout;

      const message = {
        id: messageId,
        type,
        data,
        timestamp: Date.now(),
      };

      // 设置响应处理器
      const timeoutTimer = setTimeout(() => {
        this.pendingMessages.delete(messageId);
        reject(new Error(`Message timeout: ${type}`));
      }, timeoutMs);

      this.pendingMessages.set(messageId, {
        resolve,
        reject,
        timer: timeoutTimer,
        type,
      });

      this._sendMessage(message).catch((error) => {
        this.pendingMessages.delete(messageId);
        clearTimeout(timeoutTimer);
        reject(error);
      });
    });
  }

  /**
   * 注册消息处理器
   */
  registerHandler(type, handler) {
    if (typeof handler !== "function") {
      throw new Error("SignalingManager: Handler must be a function");
    }

    if (!this.messageHandlers.has(type)) {
      this.messageHandlers.set(type, []);
    }

    this.messageHandlers.get(type).push(handler);

    // 返回取消注册函数
    return () => {
      const handlers = this.messageHandlers.get(type);
      if (handlers) {
        const index = handlers.indexOf(handler);
        if (index !== -1) {
          handlers.splice(index, 1);
          if (handlers.length === 0) {
            this.messageHandlers.delete(type);
          }
        }
      }
    };
  }

  /**
   * 取消注册消息处理器
   */
  unregisterHandler(type, handler = null) {
    if (handler) {
      const handlers = this.messageHandlers.get(type);
      if (handlers) {
        const index = handlers.indexOf(handler);
        if (index !== -1) {
          handlers.splice(index, 1);
          if (handlers.length === 0) {
            this.messageHandlers.delete(type);
          }
        }
      }
    } else {
      this.messageHandlers.delete(type);
    }
  }

  /**
   * 获取连接状态
   */
  getConnectionState() {
    return {
      readyState: this.readyState,
      isConnecting: this.isConnecting,
      isReconnecting: this.isReconnecting,
      reconnectAttempts: this.reconnectAttempts,
      url: this.url,
      quality: { ...this.connectionQuality },
    };
  }

  /**
   * 获取连接质量信息
   */
  getConnectionQuality() {
    return { ...this.connectionQuality };
  }

  /**
   * 测量延迟
   */
  async measureLatency() {
    if (this.readyState !== WebSocket.OPEN) {
      throw new Error("Connection not ready");
    }

    const startTime = Date.now();

    try {
      await this.sendWithResponse("ping", { timestamp: startTime }, 5000);
      const latency = Date.now() - startTime;
      this.connectionQuality.latency = latency;
      return latency;
    } catch (error) {
      Logger.warn("SignalingManager: 延迟测量失败", error);
      throw error;
    }
  }

  /**
   * 创建WebSocket连接
   * @private
   */
  _createConnection() {
    return new Promise((resolve, reject) => {
      try {
        this.ws = new WebSocket(this.url);

        // 连接超时处理
        const connectionTimer = setTimeout(() => {
          if (this.ws && this.ws.readyState === WebSocket.CONNECTING) {
            this.ws.close();
            reject(new Error("Connection timeout"));
          }
        }, this.connectionTimeout);

        this.ws.onopen = (event) => {
          clearTimeout(connectionTimer);
          this._handleOpen(event);
          resolve();
        };

        this.ws.onmessage = (event) => {
          this._handleMessage(event);
        };

        this.ws.onclose = (event) => {
          clearTimeout(connectionTimer);
          this._handleClose(event);
          if (this.isConnecting) {
            reject(
              new Error(`Connection closed: ${event.code} ${event.reason}`)
            );
          }
        };

        this.ws.onerror = (event) => {
          clearTimeout(connectionTimer);
          this._handleError(event);
          if (this.isConnecting) {
            reject(new Error("WebSocket error"));
          }
        };
      } catch (error) {
        reject(error);
      }
    });
  }

  /**
   * 处理连接打开
   * @private
   */
  _handleOpen(event) {
    Logger.info("SignalingManager: WebSocket连接已建立");

    this.isConnecting = false;
    this.isReconnecting = false;
    this.reconnectAttempts = 0;
    this.readyState = WebSocket.OPEN;
    this.connectionQuality.lastConnected = Date.now();

    // 发送队列中的消息
    this._flushMessageQueue();

    // 启动心跳
    this._startHeartbeat();

    // 触发事件
    this.eventBus?.emit("signaling:connected", {
      url: this.url,
      reconnectCount: this.connectionQuality.reconnectCount,
    });
  }

  /**
   * 处理消息接收
   * @private
   */
  _handleMessage(event) {
    try {
      const message = JSON.parse(event.data);
      Logger.debug("SignalingManager: 收到消息", message.type);

      // 处理响应消息
      if (message.id && this.pendingMessages.has(message.id)) {
        const pending = this.pendingMessages.get(message.id);
        this.pendingMessages.delete(message.id);
        clearTimeout(pending.timer);

        if (message.error) {
          pending.reject(new Error(message.error));
        } else {
          pending.resolve(message.data);
        }
        return;
      }

      // 分发消息到处理器
      this._dispatchMessage(message);
    } catch (error) {
      Logger.error("SignalingManager: 消息解析失败", error, event.data);
      this.eventBus?.emit("signaling:parse-error", { error, data: event.data });
    }
  }

  /**
   * 处理连接关闭
   * @private
   */
  _handleClose(event) {
    Logger.info(
      `SignalingManager: WebSocket连接已关闭 (${event.code}: ${event.reason})`
    );

    this.readyState = WebSocket.CLOSED;
    this.ws = null;
    this._clearTimers();

    // 更新连接质量统计
    if (this.connectionQuality.lastConnected) {
      this.connectionQuality.totalUptime +=
        Date.now() - this.connectionQuality.lastConnected;
    }

    // 触发事件
    this.eventBus?.emit("signaling:disconnected", {
      code: event.code,
      reason: event.reason,
      wasClean: event.wasClean,
    });

    // 如果不是主动断开，尝试重连
    if (!event.wasClean && event.code !== 1000) {
      this._scheduleReconnect();
    }
  }

  /**
   * 处理连接错误
   * @private
   */
  _handleError(event) {
    Logger.error("SignalingManager: WebSocket错误", event);
    this.eventBus?.emit("signaling:error", { event });
  }

  /**
   * 发送消息
   * @private
   */
  _sendMessage(message, options = {}) {
    return new Promise((resolve, reject) => {
      try {
        const messageStr = JSON.stringify(message);
        this.ws.send(messageStr);
        Logger.debug("SignalingManager: 发送消息", message.type);
        resolve();
      } catch (error) {
        Logger.error("SignalingManager: 发送消息失败", error);
        reject(error);
      }
    });
  }

  /**
   * 分发消息到处理器
   * @private
   */
  _dispatchMessage(message) {
    const handlers = this.messageHandlers.get(message.type);
    if (handlers && handlers.length > 0) {
      handlers.forEach((handler) => {
        try {
          handler(message.data, message);
        } catch (error) {
          Logger.error(
            `SignalingManager: 消息处理器错误 (${message.type})`,
            error
          );
        }
      });
    } else {
      Logger.warn(`SignalingManager: 未找到消息处理器: ${message.type}`);
      this.eventBus?.emit("signaling:unhandled-message", message);
    }
  }

  /**
   * 发送队列中的消息
   * @private
   */
  _flushMessageQueue() {
    if (this.messageQueue.length > 0) {
      Logger.info(
        `SignalingManager: 发送队列中的 ${this.messageQueue.length} 条消息`
      );

      const queue = [...this.messageQueue];
      this.messageQueue = [];

      queue.forEach((message) => {
        this._sendMessage(message).catch((error) => {
          Logger.error("SignalingManager: 队列消息发送失败", error);
        });
      });
    }
  }

  /**
   * 安排重连
   * @private
   */
  _scheduleReconnect() {
    if (
      this.isReconnecting ||
      this.reconnectAttempts >= this.maxReconnectAttempts
    ) {
      if (this.reconnectAttempts >= this.maxReconnectAttempts) {
        Logger.error("SignalingManager: 达到最大重连次数，停止重连");
        this.eventBus?.emit("signaling:max-reconnect-reached");
      }
      return;
    }

    this.isReconnecting = true;
    this.reconnectAttempts++;

    // 指数退避算法
    const delay = Math.min(
      this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1),
      this.maxReconnectDelay
    );

    Logger.info(
      `SignalingManager: ${delay}ms 后进行第 ${this.reconnectAttempts} 次重连`
    );

    this.eventBus?.emit("signaling:reconnecting", {
      attempt: this.reconnectAttempts,
      delay,
      maxAttempts: this.maxReconnectAttempts,
    });

    setTimeout(() => {
      if (this.isReconnecting) {
        this.connectionQuality.reconnectCount++;
        this.connect();
      }
    }, delay);
  }

  /**
   * 启动心跳
   * @private
   */
  _startHeartbeat() {
    this._clearHeartbeat();

    if (this.heartbeatInterval > 0) {
      this.heartbeatTimer = setInterval(() => {
        this._sendHeartbeat();
      }, this.heartbeatInterval);
    }
  }

  /**
   * 发送心跳
   * @private
   */
  _sendHeartbeat() {
    if (this.readyState === WebSocket.OPEN) {
      this.lastHeartbeat = Date.now();
      this.send(
        "ping",
        { timestamp: this.lastHeartbeat },
        { queue: false }
      ).catch((error) => {
        Logger.warn("SignalingManager: 心跳发送失败", error);
      });

      // 设置心跳超时
      this.heartbeatTimeout = setTimeout(() => {
        Logger.warn("SignalingManager: 心跳超时，可能连接异常");
        this.eventBus?.emit("signaling:heartbeat-timeout");
      }, 10000);
    }
  }

  /**
   * 处理心跳响应
   * @private
   */
  _handlePong(data) {
    if (this.heartbeatTimeout) {
      clearTimeout(this.heartbeatTimeout);
      this.heartbeatTimeout = null;
    }

    if (data && data.timestamp && this.lastHeartbeat) {
      const latency = Date.now() - this.lastHeartbeat;
      this.connectionQuality.latency = latency;
      Logger.debug(`SignalingManager: 心跳延迟 ${latency}ms`);
    }
  }

  /**
   * 清理定时器
   * @private
   */
  _clearTimers() {
    this._clearHeartbeat();

    // 清理待处理的消息
    this.pendingMessages.forEach((pending) => {
      clearTimeout(pending.timer);
      pending.reject(new Error("Connection closed"));
    });
    this.pendingMessages.clear();
  }

  /**
   * 清理心跳定时器
   * @private
   */
  _clearHeartbeat() {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }

    if (this.heartbeatTimeout) {
      clearTimeout(this.heartbeatTimeout);
      this.heartbeatTimeout = null;
    }
  }
}

// 导出SignalingManager类
if (typeof module !== "undefined" && module.exports) {
  module.exports = SignalingManager;
} else if (typeof window !== "undefined") {
  window.SignalingManager = SignalingManager;
}

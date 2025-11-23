/**
 * 信令客户端
 * 基于 GStreamer WebRTC 信令协议规范实现
 * 支持协议协商和多协议兼容
 */
class SignalingClient {
  constructor(url, peerId, options = {}) {
    // 验证必需参数
    if (!url) {
      throw new Error("SignalingClient: url parameter is required");
    }
    if (peerId === undefined || peerId === null) {
      throw new Error("SignalingClient: peerId parameter is required");
    }

    // 设置核心参数
    this.serverUrl = url;
    this.peerId = peerId;

    // 可选参数
    this.eventBus = options.eventBus || null;
    this.logger = options.logger || console;
    this.config = options.config || {};

    // 连接配置
    this.maxRetries = options.maxRetries || 1;
    this.retryDelay = options.retryDelay || 3000;
    this.connectionTimeout = options.connectionTimeout || 10000;
    this.heartbeatInterval = options.heartbeatInterval || 30000;

    // 内部状态
    this._ws = null;
    this._state = "disconnected";
    this._retryCount = 0;
    this._messageId = 0;
    this._pendingMessages = new Map();
    this._heartbeatTimer = null;
    this._connectionTimer = null;
    this._protocolMode = "auto"; // 'auto', 'standard', 'selkies'
    this._connectionStartTime = null;
    this._iceServers = []; // 从服务器获取的 ICE 服务器配置

    // 事件回调
    this.onopen = null;
    this.onclose = null;
    this.onerror = null;
    this.onmessage = null;
    this.onsdp = null;
    this.onice = null;
    this.onstatus = null;

    // 初始化
    this._initializeServerUrl();
  }

  /**
   * 初始化服务器URL
   */
  _initializeServerUrl() {
    // 如果提供的URL是相对路径或需要补全，则构建完整的URL
    if (this.serverUrl) {
      // 如果URL不包含协议，则构建完整的WebSocket URL
      if (
        !this.serverUrl.startsWith("ws://") &&
        !this.serverUrl.startsWith("wss://")
      ) {
        const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
        const host = window.location.host;

        // 如果是相对路径，直接拼接
        if (this.serverUrl.startsWith("/")) {
          this.serverUrl = `${protocol}//${host}${this.serverUrl}`;
        } else {
          // 如果只是主机名或其他格式，使用默认路径
          this.serverUrl = `${protocol}//${this.serverUrl}/api/signaling`;
        }
      }
    } else {
      // 如果没有提供URL，构建默认URL（使用新的API路径）
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      const host = window.location.host;
      this.serverUrl = `${protocol}//${host}/api/signaling`;
    }
  }

  /**
   * 连接到信令服务器
   */
  async connect() {
    if (this._state === "connected" || this._state === "connecting") {
      throw new Error("Already connected or connecting");
    }

    this._state = "connecting";
    this._connectionStartTime = performance.now();
    this._setStatus("Connecting to server...");

    // 触发连接开始事件
    if (this.eventBus) {
      this.eventBus.emit("signaling:connecting", {
        timestamp: this._connectionStartTime,
        attempt: this._retryCount + 1,
      });
    }

    try {
      this._ws = new WebSocket(this.serverUrl);
      this._setupWebSocketHandlers();

      // 等待连接建立
      await this._waitForConnection();

      // 发送 HELLO 消息
      await this._sendHello();

      // 如果启用自动协议选择，进行协议协商
      if (this._protocolMode === "auto") {
        try {
          const selectedProtocol = await this.autoSelectProtocol();
          this.logger.info(`Protocol auto-selected: ${selectedProtocol}`);
        } catch (error) {
          this.logger.warn(
            "Auto protocol selection failed, using standard:",
            error
          );
          this._protocolMode = "standard"; // 默认使用标准协议
        }
      }

      // 记录连接建立时间
      const connectionTime = performance.now() - this._connectionStartTime;

      // 启动心跳
      this._startHeartbeat();

      this._setStatus("Connected to server");

      // 触发连接成功事件
      if (this.eventBus) {
        this.eventBus.emit("signaling:connected", {
          connectionTime: connectionTime,
          peerId: this.peerId,
          server: this.serverUrl,
          protocol: this._protocolMode,
          retryCount: this._retryCount,
        });
      }

      if (this.onopen) this.onopen();
    } catch (error) {
      this._state = "disconnected";

      this._setError("Connection failed: " + error.message);
      throw error;
    }
  }

  /**
   * 断开连接
   */
  disconnect() {
    this._state = "disconnecting";
    this._stopHeartbeat();
    this._clearConnectionTimer();

    if (this._ws) {
      this._ws.close(1000, "Client disconnect");
      this._ws = null;
    }

    this._state = "disconnected";
    this._retryCount = 0;
    this._pendingMessages.clear();

    this._setStatus("Disconnected");
    if (this.onclose) this.onclose();
  }

  /**
   * 重新连接
   */
  async reconnect() {
    this.disconnect();

    // 指数退避延迟
    const delay = Math.min(
      this.retryDelay * Math.pow(2, this._retryCount),
      30000
    );

    await new Promise((resolve) => setTimeout(resolve, delay));

    this._retryCount++;

    if (this._retryCount > this.maxRetries) {
      throw new Error("Max retry attempts exceeded");
    }

    return this.connect();
  }

  /**
   * 发送标准格式消息
   */
  sendMessage(type, data = null, options = {}) {
    if (this._state !== "connected") {
      throw new Error("Not connected to signaling server");
    }

    const messageId = this._generateMessageId();
    const message = {
      type: type,
      peer_id: String(this.peerId),
      data: data,
      message_id: messageId,
      timestamp: Date.now(),
    };

    // 添加到待确认消息列表
    if (options.expectAck) {
      this._pendingMessages.set(messageId, {
        message,
        timestamp: Date.now(),
        resolve: options.resolve,
        reject: options.reject,
      });
    }

    const messageStr = JSON.stringify(message);
    this._ws.send(messageStr);

    // 触发消息发送事件
    if (this.eventBus) {
      this.eventBus.emit("signaling:message-sent", {
        type: type,
        size: messageStr.length,
        id: messageId,
        timestamp: message.timestamp,
      });
    }

    return messageId;
  }

  /**
   * 发送 SDP 消息（支持多种格式）
   */
  sendSDP(sdp) {
    if (this._isSelkiesMode()) {
      // Selkies 格式
      this._ws.send(JSON.stringify({ sdp: sdp }));
    } else {
      // 标准格式 - 提取 SDP 字符串
      const sdpString = typeof sdp === "string" ? sdp : sdp.sdp;
      this.sendMessage("answer", { sdp: { sdp: sdpString, type: "answer" } });
    }
  }

  /**
   * 发送 ICE 候选
   */
  sendICE(candidate) {
    if (this._isSelkiesMode()) {
      // Selkies 格式
      this._ws.send(JSON.stringify({ ice: candidate }));
    } else {
      // 标准格式 - 构造正确的候选对象
      let candidateData;
      
      if (typeof candidate === "string") {
        // 如果是字符串，构造基本对象
        candidateData = {
          candidate: candidate,
          sdpMLineIndex: 0,
          sdpMid: "0"
        };
      } else if (candidate.candidate) {
        // 如果是 RTCIceCandidate 对象，提取所需字段
        candidateData = {
          candidate: candidate.candidate,
          sdpMid: candidate.sdpMid || "0",
          sdpMLineIndex: candidate.sdpMLineIndex !== undefined ? candidate.sdpMLineIndex : 0
        };
        
        // 可选字段
        if (candidate.usernameFragment) {
          candidateData.usernameFragment = candidate.usernameFragment;
        }
      } else {
        this.logger.error('Invalid candidate format:', candidate);
        return;
      }
      
      this.sendMessage("ice-candidate", {
        candidate: candidateData
      });
    }
  }

  /**
   * 手动请求 WebRTC Offer
   * 允许客户端主动请求新的 offer
   */
  requestOffer(constraints = null, codecPreferences = null) {
    if (this._state !== "connected") {
      throw new Error("Not connected to signaling server");
    }

    const data = {
      constraints: constraints || {
        video: true,
        audio: true,
        data_channel: true,
      },
      codec_preferences: codecPreferences || ["H264", "VP8", "VP9"],
    };

    return this.sendMessage("request-offer", data);
  }

  /**
   * 获取连接状态
   */
  getState() {
    return {
      connectionState: this._state,
      readyState: this._ws ? this._ws.readyState : WebSocket.CLOSED,
      peerId: this.peerId,
      retryCount: this._retryCount,
      serverUrl: this.serverUrl,
      isConnected: this._state === "connected",
      canRetry: this._retryCount < this.maxRetries,
      protocolMode: this._protocolMode,
    };
  }

  /**
   * 获取从服务器接收的 ICE 服务器配置
   */
  getICEServers() {
    return this._iceServers;
  }

  /**
   * 检查是否已连接
   */
  isConnected() {
    return (
      this._state === "connected" &&
      this._ws &&
      this._ws.readyState === WebSocket.OPEN
    );
  }

  /**
   * 协商协议版本
   */
  async negotiateProtocol(supportedProtocols = ["gstreamer-1.0", "selkies"]) {
    const negotiationMessage = {
      type: "protocol-negotiation",
      data: {
        supported_protocols: supportedProtocols,
        preferred_protocol: supportedProtocols[0],
        client_capabilities: this._getClientCapabilities(),
        client_version: this._getClientVersion(),
        fallback_protocols: this._getFallbackProtocols(supportedProtocols),
      },
    };

    return new Promise((resolve, reject) => {
      const timeoutId = setTimeout(() => {
        reject(new Error("Protocol negotiation timeout"));
      }, 10000); // 10 second timeout

      this.sendMessage(negotiationMessage.type, negotiationMessage.data, {
        expectAck: true,
        resolve: (result) => {
          clearTimeout(timeoutId);
          resolve(result);
        },
        reject: (error) => {
          clearTimeout(timeoutId);
          reject(error);
        },
      });
    });
  }

  /**
   * 自动检测并选择最佳协议
   */
  async autoSelectProtocol() {
    const supportedProtocols = this.getSupportedProtocols();

    try {
      // 尝试协议协商
      const negotiationResult = await this.negotiateProtocol(
        supportedProtocols
      );

      if (negotiationResult.success) {
        this._protocolMode = negotiationResult.selected_protocol;
        this._setStatus(
          `Protocol negotiated: ${negotiationResult.selected_protocol}`
        );
        return negotiationResult.selected_protocol;
      } else {
        throw new Error(
          negotiationResult.error || "Protocol negotiation failed"
        );
      }
    } catch (error) {
      this.logger.warn(
        "Protocol negotiation failed, falling back to detection:",
        error
      );

      // 协商失败，尝试协议检测
      return this._detectProtocolFromServerResponse();
    }
  }

  /**
   * 协议降级机制
   */
  async downgradeProtocol(currentProtocol, reason = "compatibility_issue") {
    const protocolHierarchy = ["gstreamer-1.0", "selkies", "legacy"];

    const currentIndex = protocolHierarchy.indexOf(currentProtocol);

    if (currentIndex === -1 || currentIndex >= protocolHierarchy.length - 1) {
      throw new Error("No lower protocol version available for downgrade");
    }

    const targetProtocol = protocolHierarchy[currentIndex + 1];

    this.logger.info(
      `Downgrading protocol from ${currentProtocol} to ${targetProtocol}, reason: ${reason}`
    );

    try {
      // 尝试协商降级后的协议
      const negotiationResult = await this.negotiateProtocol([targetProtocol]);

      if (negotiationResult.success) {
        this._protocolMode = targetProtocol;
        this._setStatus(`Protocol downgraded to: ${targetProtocol}`);

        // 触发协议降级事件
        if (this.eventBus) {
          this.eventBus.emit("signaling:protocol-downgraded", {
            from: currentProtocol,
            to: targetProtocol,
            reason: reason,
          });
        }

        return targetProtocol;
      } else {
        throw new Error(
          `Failed to downgrade to ${targetProtocol}: ${negotiationResult.error}`
        );
      }
    } catch (error) {
      // 如果协商失败，直接设置协议模式
      this._protocolMode = targetProtocol;
      this._setStatus(
        `Force downgraded to: ${targetProtocol} (negotiation failed)`
      );

      if (this.eventBus) {
        this.eventBus.emit("signaling:protocol-force-downgraded", {
          from: currentProtocol,
          to: targetProtocol,
          reason: reason,
          error: error.message,
        });
      }

      return targetProtocol;
    }
  }

  /**
   * 检测协议版本
   */
  detectProtocolVersion(message) {
    // 检测消息中的协议版本信息
    if (typeof message === "string") {
      // Selkies 文本协议
      if (message.startsWith("HELLO") || message.startsWith("ERROR")) {
        return {
          protocol: "selkies",
          version: "1.0",
          confidence: 0.9,
        };
      }
    } else if (typeof message === "object") {
      // JSON 消息
      if (message.version) {
        // 标准协议有版本字段
        return {
          protocol: message.metadata?.protocol || "gstreamer-webrtc",
          version: message.version,
          confidence: 0.95,
        };
      } else if (message.sdp || message.ice) {
        // Selkies JSON 格式
        return {
          protocol: "selkies",
          version: "1.0",
          confidence: 0.8,
        };
      }
    }

    // 默认检测结果
    return {
      protocol: "unknown",
      version: "1.0",
      confidence: 0.1,
    };
  }

  /**
   * 从服务器响应中检测协议
   */
  async _detectProtocolFromServerResponse() {
    return new Promise((resolve) => {
      const originalOnMessage = this.onmessage;
      let detectionTimeout;

      // 临时消息处理器用于协议检测
      this.onmessage = (message) => {
        const detection = this.detectProtocolVersion(message);

        if (detection.confidence > 0.7) {
          // 检测成功
          clearTimeout(detectionTimeout);
          this.onmessage = originalOnMessage;
          this._protocolMode = detection.protocol;
          this._setStatus(
            `Protocol detected: ${detection.protocol} v${detection.version}`
          );
          resolve(detection.protocol);
        }

        // 调用原始处理器
        if (originalOnMessage) {
          originalOnMessage(message);
        }
      };

      // 设置检测超时
      detectionTimeout = setTimeout(() => {
        this.onmessage = originalOnMessage;
        this._protocolMode = "standard"; // 默认使用标准协议
        this._setStatus(
          "Protocol detection timeout, using standard as fallback"
        );
        resolve("standard");
      }, 5000);

      // 发送测试消息触发服务器响应
      this._sendProtocolDetectionMessage();
    });
  }

  /**
   * 发送协议检测消息
   */
  _sendProtocolDetectionMessage() {
    try {
      // 发送一个简单的 ping 消息来触发服务器响应
      if (this._ws && this._ws.readyState === WebSocket.OPEN) {
        this.sendMessage("ping", {
          timestamp: Date.now(),
          purpose: "protocol_detection",
        });
      }
    } catch (error) {
      this.logger.warn("Failed to send protocol detection message:", error);
    }
  }

  /**
   * 获取支持的协议列表
   */
  getSupportedProtocols() {
    return ["gstreamer-1.0", "selkies"];
  }

  /**
   * 设置协议模式
   */
  setProtocolMode(mode) {
    if (["auto", "standard", "selkies"].includes(mode)) {
      this._protocolMode = mode;
    } else {
      throw new Error("Invalid protocol mode: " + mode);
    }
  }

  /**
   * 等待连接建立
   */
  _waitForConnection() {
    return new Promise((resolve, reject) => {
      this._connectionTimer = setTimeout(() => {
        reject(new Error("Connection timeout"));
      }, this.connectionTimeout);

      this._ws.addEventListener(
        "open",
        () => {
          this._clearConnectionTimer();
          resolve();
        },
        { once: true }
      );

      this._ws.addEventListener(
        "error",
        (event) => {
          this._clearConnectionTimer();
          reject(new Error("WebSocket connection failed"));
        },
        { once: true }
      );
    });
  }

  /**
   * 设置 WebSocket 事件处理器
   */
  _setupWebSocketHandlers() {
    this._ws.onopen = (event) => {
      this.logger.info("WebSocket connection opened");
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

  /**
   * 处理接收到的消息
   */
  _handleMessage(event) {
    try {
      const data = event.data;

      // 处理 Selkies 协议文本消息
      if (typeof data === "string" && !data.startsWith("{")) {
        this._handleSelkiesTextMessage(data);

        // 触发消息接收事件
        if (this.eventBus) {
          this.eventBus.emit("signaling:message-received", {
            type: "selkies-text",
            size: data.length,
            timestamp: Date.now(),
          });
        }
        return;
      }

      // 解析 JSON 消息
      const message = JSON.parse(data);

      // 触发消息接收事件
      if (this.eventBus) {
        this.eventBus.emit("signaling:message-received", {
          type: message.type || "unknown",
          size: data.length,
          id: message.message_id || message.id,
          timestamp: Date.now(),
        });
      }

      // 处理不同消息类型
      const messageType = message.type || this._detectMessageType(message);
      
      // 调试日志
      if (messageType === "ice-candidate" || messageType === "ice" || message.ice) {
        this.logger.debug("ICE message routing:", {
          messageType,
          hasType: !!message.type,
          hasIce: !!message.ice,
          hasData: !!message.data,
          dataHasCandidate: !!(message.data && message.data.candidate)
        });
      }
      
      switch (messageType) {
        case "welcome":
          this._handleWelcome(message);
          break;
        case "offer":
          this._handleOffer(message);
          break;
        case "ice-candidate":
        case "ice":  // 也处理 "ice" 类型
          this._handleIceCandidate(message);
          break;
        case "error":
          this._handleError(message);
          break;
        case "pong":
          this._handlePong(message);
          break;
        case "protocol-negotiation-response":
          this._handleProtocolNegotiationResponse(message);
          break;
        default:
          // 处理 Selkies JSON 消息
          if (message.sdp) {
            this._handleSelkiesSDP(message);
          } else if (message.ice) {
            // 如果有 ice 字段但没有 type，可能是 Selkies 格式
            this.logger.warn("Received message with 'ice' field but no type, treating as Selkies format");
            this._handleSelkiesICE(message);
          } else {
            this.logger.warn("Unknown message type:", message);
          }
      }

      // 触发通用消息回调
      if (this.onmessage) {
        this.onmessage(message);
      }
    } catch (error) {
      this.logger.error("Failed to handle message:", error);
      if (this.onerror) {
        this.onerror(new Error("Message parsing failed: " + error.message));
      }
    }
  }

  /**
   * 处理 Selkies 文本消息
   */
  _handleSelkiesTextMessage(data) {
    if (data === "HELLO") {
      // 服务器确认注册
      this._setSelkiesMode(true);
      this._setStatus("Registered with server (Selkies mode)");

      // Selkies 模式下尝试请求 offer
      // 传统 Selkies 协议可能不支持 request-offer，失败是正常的
      this._sendRequestOffer();
    } else if (data.startsWith("ERROR")) {
      // 服务器错误
      const error = new Error("Server error: " + data);
      if (this.onerror) this.onerror(error);
    }
  }

  /**
   * 处理 Selkies SDP 消息
   */
  _handleSelkiesSDP(message) {
    if (message.sdp && this.onsdp) {
      this.onsdp(new RTCSessionDescription(message.sdp));
    }
  }

  /**
   * 处理 Selkies ICE 消息
   */
  _handleSelkiesICE(message) {
    if (message.ice && this.onice) {
      // Selkies 格式的 ICE 候选，直接传递对象
      this.onice(message.ice);
    }
  }

  /**
   * 处理欢迎消息
   */
  _handleWelcome(message) {
    this._setStatus("Received welcome from server");

    // 检测协议模式
    if (message.data && message.data.protocol) {
      this._protocolMode = message.data.protocol;
    }

    // 提取 ICE 服务器配置
    if (message.data && message.data.sessionConfig && message.data.sessionConfig.iceServers) {
      this._iceServers = message.data.sessionConfig.iceServers;
      console.log(`🔧 [Signaling] 从服务器获取到 ${this._iceServers.length} 个 ICE 服务器配置:`, this._iceServers);
      this._setStatus(`Received ${this._iceServers.length} ICE servers from server`);
    }

    // 变更状态为已连接
    this._state = "connected";
    this._retryCount = 0;

    // 根据 GStreamer 信令协议规范，收到 welcome 消息后立即发送 request-offer
    this._sendRequestOffer();
  }

  /**
   * 处理 SDP offer
   */
  _handleOffer(message) {
    if (message.data && message.data.sdp && this.onsdp) {
      this.onsdp(new RTCSessionDescription(message.data.sdp));
    }
  }

  /**
   * 处理 ICE 候选
   */
  _handleIceCandidate(message) {
    if (message.data && this.onice) {
      // 服务器发送的格式：{ candidate: "candidate:...", sdpMid: "0", sdpMLineIndex: 0 }
      // 需要构造完整的 RTCIceCandidateInit 对象
      const candidateData = message.data;
      
      // 检查是否是嵌套的候选格式
      let candidateInit;
      if (typeof candidateData.candidate === 'string') {
        // 直接格式：{ candidate: "candidate:...", sdpMid, sdpMLineIndex }
        candidateInit = {
          candidate: candidateData.candidate,
          sdpMid: candidateData.sdpMid,
          sdpMLineIndex: candidateData.sdpMLineIndex
        };
      } else if (candidateData.candidate && typeof candidateData.candidate === 'object') {
        // 嵌套格式：{ candidate: { candidate: "...", sdpMid, sdpMLineIndex } }
        candidateInit = candidateData.candidate;
      } else {
        this.logger.error('Invalid ICE candidate format:', candidateData);
        return;
      }
      
      this.onice(candidateInit);
    }
  }

  /**
   * 处理错误消息
   */
  _handleError(message) {
    // 添加详细日志以便调试
    this.logger.warn("Received error message:", message);
    
    const errorMessage = message.data?.message || message.error?.message || "Unknown server error";
    const error = new Error(errorMessage);
    error.code = message.data?.code || message.error?.code;
    error.details = message.data?.details || message.error?.details;
    error.rawMessage = message;

    if (this.onerror) this.onerror(error);
  }

  /**
   * 处理 Pong 消息
   */
  _handlePong(message) {
    const now = performance.now();
    const pingTime = message.data?.timestamp;

    if (pingTime) {
      const latency = now - pingTime;
      this.logger.debug(`Pong received, latency: ${latency}ms`);

      // 触发延迟测量事件
      if (this.eventBus) {
        this.eventBus.emit("signaling:latency", {
          latency: latency,
          id: message.message_id || message.id,
          timestamp: now,
        });
      }
    }
  }

  /**
   * 处理协议协商响应
   */
  _handleProtocolNegotiationResponse(message) {
    const messageId = message.message_id || message.id;
    const pending = this._pendingMessages.get(messageId);

    if (pending) {
      this._pendingMessages.delete(messageId);

      if (message.data?.success) {
        this._protocolMode = message.data.selected_protocol;
        pending.resolve(message.data);
      } else {
        pending.reject(
          new Error(message.data?.error || "Protocol negotiation failed")
        );
      }
    }
  }

  /**
   * 处理连接错误
   */
  _handleConnectionError(event) {
    const error = {
      type: "connection_error",
      code: "CONNECTION_FAILED",
      message: "WebSocket connection failed",
      details: event,
      timestamp: Date.now(),
      recoverable: this._retryCount < this.maxRetries,
    };

    this.logger.error("Connection error:", error);

    if (this.onerror) {
      this.onerror(error);
    }

    // 自动重连
    if (error.recoverable && this._state !== "disconnecting") {
      // TODO：修复核心错误为主，这里暂时中断自动重连
      // this._scheduleReconnect();
    }
  }

  /**
   * 处理连接关闭
   */
  _handleConnectionClose(event) {
    const wasConnected = this._state === "connected";
    this._state = "disconnected";
    this._stopHeartbeat();

    const closeInfo = {
      code: event.code,
      reason: event.reason,
      wasClean: event.wasClean,
      wasConnected: wasConnected,
    };

    this.logger.info("WebSocket connection closed:", closeInfo);

    // 非正常关闭且之前已连接，尝试重连
    if (!event.wasClean && wasConnected && this._state !== "disconnecting") {
      this._scheduleReconnect();
    }

    if (this.onclose) {
      this.onclose(closeInfo);
    }
  }

  /**
   * 安排重连
   */
  _scheduleReconnect() {
    if (this._retryCount >= this.maxRetries) {
      const error = new Error("Max retry attempts exceeded");
      error.type = "max_retries_exceeded";
      if (this.onerror) this.onerror(error);
      return;
    }

    const delay = Math.min(
      this.retryDelay * Math.pow(2, this._retryCount),
      30000
    );

    this._setStatus(
      `Reconnecting in ${delay / 1000} seconds (attempt ${
        this._retryCount + 1
      }/${this.maxRetries})`
    );

    setTimeout(() => {
      if (this._state === "disconnected") {
        this.reconnect().catch((error) => {
          this.logger.error("Reconnect failed:", error);
          if (this.onerror) this.onerror(error);
        });
      }
    }, delay);
  }

  /**
   * 发送 HELLO 消息
   */
  _sendHello() {
    if (this._protocolMode === "selkies") {
      // Selkies HELLO 格式
      const meta = {
        res: `${window.screen.width}x${window.screen.height}`,
        scale: window.devicePixelRatio,
      };
      const helloMessage = `HELLO ${this.peerId} ${btoa(JSON.stringify(meta))}`;
      this._ws.send(helloMessage);
    } else {
      // 标准 HELLO 格式 - 直接发送，不使用 sendMessage (因为此时状态还是 connecting)
      const messageId = this._generateMessageId();
      const message = {
        type: "hello",
        peer_id: String(this.peerId),
        data: {
          client_type: "ui",  // 标识为UI客户端
          client_info: this._getClientInfo(),
          capabilities: this._getClientCapabilities(),
          supported_protocols: this.getSupportedProtocols(),
          preferred_protocol: "gstreamer-1.0"
        },
        message_id: messageId,
        timestamp: Date.now(),
      };

      const messageStr = JSON.stringify(message);
      this._ws.send(messageStr);

      // 触发消息发送事件
      if (this.eventBus) {
        this.eventBus.emit("signaling:message-sent", {
          type: "hello",
          size: messageStr.length,
          id: messageId,
          timestamp: message.timestamp,
        });
      }
    }
  }

  /**
   * 启动心跳
   */
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

  /**
   * 停止心跳
   */
  _stopHeartbeat() {
    if (this._heartbeatTimer) {
      clearInterval(this._heartbeatTimer);
      this._heartbeatTimer = null;
    }
  }

  /**
   * 发送 request-offer 消息
   * 根据 GStreamer 信令协议规范，在收到 welcome 消息后立即发送
   */
  _sendRequestOffer() {
    try {
      // 检查协议模式，只在标准模式或支持的情况下发送
      if (this._protocolMode === "selkies" && this._state !== "connected") {
        // Selkies 模式下如果连接状态不是 connected，可能不支持标准消息格式
        this.logger.info(
          "Selkies mode: skipping request-offer, waiting for automatic offer"
        );
        return null;
      }

      const requestOfferId = this.sendMessage("request-offer", {
        constraints: {
          video: true,
          audio: true,
          data_channel: true,
        },
        codec_preferences: ["H264", "VP8", "VP9"],
      });

      this._setStatus("Requested WebRTC offer from server");

      // 触发 request-offer 发送事件
      if (this.eventBus) {
        this.eventBus.emit("signaling:request-offer-sent", {
          id: requestOfferId,
          timestamp: Date.now(),
        });
      }

      return requestOfferId;
    } catch (error) {
      this.logger.error("Failed to send request-offer:", error);

      // 在 Selkies 模式下，request-offer 失败是可以接受的
      if (this._protocolMode === "selkies") {
        this.logger.info(
          "Selkies mode: request-offer failed as expected, waiting for automatic offer"
        );
        return null;
      }

      this._setError("Failed to request WebRTC offer: " + error.message);
      throw error;
    }
  }

  /**
   * 发送 Ping
   */
  _sendPing() {
    try {
      const pingId = this.sendMessage("ping", {
        timestamp: Date.now(),
        client_state: this._state,
      });
      return pingId;
    } catch (error) {
      this.logger.error("Failed to send ping:", error);
    }
  }

  /**
   * 清除连接定时器
   */
  _clearConnectionTimer() {
    if (this._connectionTimer) {
      clearTimeout(this._connectionTimer);
      this._connectionTimer = null;
    }
  }

  /**
   * 生成消息ID
   */
  _generateMessageId() {
    return `msg_${Date.now()}_${++this._messageId}`;
  }

  /**
   * 获取客户端信息
   */
  _getClientInfo() {
    return {
      user_agent: navigator.userAgent,
      screen_resolution: `${window.screen.width}x${window.screen.height}`,
      device_pixel_ratio: window.devicePixelRatio,
      language: navigator.language,
      platform: navigator.platform,
    };
  }

  /**
   * 获取客户端能力
   */
  _getClientCapabilities() {
    const capabilities = [
      "webrtc",
      "datachannel",
      "video",
      "audio",
      "input-events",
      "statistics",
    ];

    // 检测浏览器特定能力
    if (window.RTCPeerConnection) {
      capabilities.push("rtc-peer-connection");
    }

    if (navigator.mediaDevices && navigator.mediaDevices.getUserMedia) {
      capabilities.push("user-media");
    }

    if (window.RTCDataChannel) {
      capabilities.push("data-channel");
    }

    return capabilities;
  }

  /**
   * 获取客户端版本信息
   */
  _getClientVersion() {
    return {
      client: "signaling-client",
      version: "1.0.0",
      build: Date.now(),
      webrtc_version: this._getWebRTCVersion(),
    };
  }

  /**
   * 获取 WebRTC 版本信息
   */
  _getWebRTCVersion() {
    // 尝试检测 WebRTC 版本
    if (window.RTCPeerConnection) {
      const pc = new RTCPeerConnection();
      const version = pc.constructor.name;
      pc.close();
      return version;
    }
    return "unknown";
  }

  /**
   * 获取协议降级列表
   */
  _getFallbackProtocols(supportedProtocols) {
    const allProtocols = ["gstreamer-1.0", "selkies", "legacy"];
    const fallbacks = [];

    for (const protocol of allProtocols) {
      if (!supportedProtocols.includes(protocol)) {
        fallbacks.push(protocol);
      }
    }

    return fallbacks;
  }

  /**
   * 检测消息类型
   */
  _detectMessageType(message) {
    // 自动检测消息类型
    if (message.sdp) return "sdp";
    if (message.ice) return "ice";
    if (message.error) return "error";
    return "unknown";
  }

  /**
   * 检查是否为 Selkies 模式
   */
  _isSelkiesMode() {
    return this._protocolMode === "selkies";
  }

  /**
   * 设置 Selkies 模式
   */
  _setSelkiesMode(enabled) {
    this._protocolMode = enabled ? "selkies" : "standard";
  }

  /**
   * 设置状态消息
   */
  _setStatus(message) {
    if (this.onstatus) this.onstatus(message);
    if (this.eventBus) this.eventBus.emit("signaling:status", { message });
  }

  /**
   * 设置错误消息
   */
  _setError(message) {
    if (this.onerror) this.onerror(new Error(message));
    if (this.eventBus) this.eventBus.emit("signaling:error", { message });
  }
}

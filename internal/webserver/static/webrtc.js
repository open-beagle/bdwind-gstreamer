/**
 * WebRTC管理器 - 简化版本
 * 基于selkies-gstreamer最佳实践的简单可靠实现
 */
class WebRTCManager {
  constructor(eventBus, config) {
    this.eventBus = eventBus;
    this.config = config;

    // 核心组件
    this.pc = null;
    this.signalingManager = null;

    // 状态管理
    this.connectionState = "disconnected"; // disconnected, connecting, connected, failed
    this.iceConnectionState = "closed";
    this.signalingState = "closed";
    this.connectionAttempts = 0;
    this.maxConnectionAttempts = 3;

    // 媒体元素
    this.videoElement = null;
    this.audioElement = null;
    this.remoteStream = null;

    // 配置 - 将在_loadConfig中从服务器获取
    this.iceServers = [];

    // 超时设置
    this.connectionTimeout = 30000;
    this.iceTimeout = 15000;

    // 定时器
    this.connectionTimer = null;
    this.iceTimer = null;
    this.reconnectTimer = null;
    this.statsTimer = null;

    // 统计
    this.connectionStats = {
      connectTime: null,
      disconnectTime: null,
      bytesReceived: 0,
      packetsLost: 0,
      totalConnections: 0,
      reconnectAttempts: 0,
    };

    this._loadConfig();
  }

  /**
   * 加载配置
   */
  _loadConfig() {
    // 设置默认ICE服务器（如果没有从服务器获取到配置）
    const defaultIceServers = [
      { urls: "stun:stun.ali.wodcloud.com:3478" },
    ];

    if (this.config) {
      const webrtcConfig = this.config.get("webrtc", {});
      this.iceServers = webrtcConfig.iceServers || defaultIceServers;
      this.connectionTimeout =
        webrtcConfig.connectionTimeout || this.connectionTimeout;
      this.maxConnectionAttempts =
        webrtcConfig.maxRetryAttempts || this.maxConnectionAttempts;
    } else {
      this.iceServers = defaultIceServers;
    }

    // 打印ICE服务器配置
    console.log("🧊 本地配置ICE服务器:", this.iceServers);
  }

  /**
   * 设置信令管理器
   */
  setSignalingManager(signalingManager) {
    this.signalingManager = signalingManager;
    this._setupSignalingEvents();
  }

  /**
   * 设置媒体元素
   */
  setMediaElements(videoElement, audioElement) {
    this.videoElement = videoElement;
    this.audioElement = audioElement;
    Logger.info("WebRTC: 媒体元素已设置");
  }

  /**
   * 初始化WebRTC管理器（仅准备配置，不自动连接）
   */
  async initialize() {
    // 首先从服务器获取最新的ICE配置
    await this._fetchServerConfig();
    
    // 打印最终使用的ICE服务器配置
    console.log("🚀 WebRTC初始化完成，将使用以下ICE服务器配置:");
    this._printICEServerConfig();
    
    console.log("⏳ WebRTC已准备就绪，等待用户开始捕获...");
    
    // 不自动连接，等待用户手动触发
    return Promise.resolve();
  }

  /**
   * 从服务器获取配置
   */
  async _fetchServerConfig() {
    try {
      console.log("🔄 从服务器获取WebRTC配置...");

      if (this.config && typeof this.config.fetchWebRTCConfig === "function") {
        const serverConfig = await this.config.fetchWebRTCConfig(true); // 强制刷新

        console.log("🔍 服务器返回的完整配置:", serverConfig);

        if (serverConfig && serverConfig.iceServers && Array.isArray(serverConfig.iceServers)) {
          console.log("🔍 服务器返回的ICE服务器配置:", serverConfig.iceServers);
          
          // 直接使用服务器返回的ICE服务器配置
          this.iceServers = serverConfig.iceServers;
          console.log("✅ 已更新ICE服务器配置，共", this.iceServers.length, "个服务器");
          
          // 详细打印每个ICE服务器
          this.iceServers.forEach((server, index) => {
            console.log(`  服务器 #${index + 1}:`, {
              urls: server.urls,
              username: server.username ? '***' : undefined,
              credential: server.credential ? '***' : undefined
            });
          });
        } else {
          console.warn("⚠️ 服务器配置中没有有效的ICE服务器数组，保持当前配置");
        }
      } else {
        console.warn("⚠️ 配置管理器不可用，使用当前ICE服务器配置");
      }
    } catch (error) {
      console.error("❌ 获取服务器配置失败，使用当前配置:", error);
    }
  }

  /**
   * 开始捕获（用户主动触发WebRTC连接）
   */
  async startCapture() {
    console.log("🎬 用户开始捕获，启动WebRTC连接...");
    return this.connect();
  }

  /**
   * 停止捕获
   */
  stopCapture() {
    console.log("⏹️ 用户停止捕获，断开WebRTC连接...");
    this.disconnect();
  }

  /**
   * 开始连接
   */
  async connect() {
    if (
      this.connectionState === "connecting" ||
      this.connectionState === "connected"
    ) {
      Logger.warn("WebRTC: 连接已在进行中或已连接");
      return;
    }

    this._setState("connecting");
    this.connectionAttempts++;

    Logger.info(`WebRTC: 开始连接 (第 ${this.connectionAttempts} 次尝试)`);

    // 打印连接开始信息
    console.log("🚀 开始WebRTC连接，ICE服务器配置已准备就绪");

    try {
      await this._createPeerConnection();
      await this._startConnection();
    } catch (error) {
      Logger.error("WebRTC: 连接失败", error);
      this._handleConnectionError(error);
    }
  }

  /**
   * 打印ICE服务器配置信息
   */
  _printICEServerConfig() {
    if (!Array.isArray(this.iceServers)) {
      console.error("❌ ICE服务器配置不是数组格式:", this.iceServers);
      return;
    }

    if (this.iceServers.length === 0) {
      console.warn("⚠️ 没有配置ICE服务器");
      return;
    }

    console.log(`📊 配置了 ${this.iceServers.length} 个ICE服务器，WebRTC将自动选择最佳连接路径:`);

    this.iceServers.forEach((server, index) => {
      if (!server || !server.urls) {
        console.error(`❌ 服务器 #${index + 1} 配置无效:`, server);
        return;
      }

      // 处理urls可能是字符串或数组的情况
      const urls = Array.isArray(server.urls) ? server.urls : [server.urls];
      
      urls.forEach((url) => {
        const serverType = this._getServerType(url);
        const serverHost = this._extractHost(url);
        const serverPort = this._extractPort(url);

        console.log(`   ${serverType} 服务器: ${serverHost}:${serverPort}`);

        if (server.username) {
          console.log(`     用户名: ${server.username}`);
          console.log(`     密码: ${"*".repeat(server.credential?.length || 0)}`);
        }
      });
    });

    console.log("🔄 让WebRTC自动处理ICE候选收集和连接建立...");
  }

  /**
   * 获取服务器类型
   */
  _getServerType(url) {
    if (url.startsWith("stun:")) return "STUN";
    if (url.startsWith("turn:")) return "TURN";
    if (url.startsWith("turns:")) return "TURNS";
    return "UNKNOWN";
  }

  /**
   * 提取主机名
   */
  _extractHost(url) {
    const match = url.match(/:\/\/([^:/?]+)/);
    if (match) return match[1];

    const hostMatch = url.match(/:([^:/?]+):/);
    if (hostMatch) return hostMatch[1];

    return url.replace(/^[^:]+:/, "").split(":")[0];
  }

  /**
   * 提取端口号
   */
  _extractPort(url) {
    const match = url.match(/:(\d+)/);
    return match ? match[1] : "3478";
  }

  /**
   * 断开连接
   */
  disconnect() {
    Logger.info("WebRTC: 断开连接");

    this._clearTimers();
    this._setState("disconnected");

    if (this.pc) {
      this.pc.close();
      this.pc = null;
    }

    this.connectionAttempts = 0;
    this.connectionStats.disconnectTime = Date.now();
  }

  /**
   * 清理连接
   */
  async cleanup() {
    this.disconnect();

    if (this.videoElement) {
      this.videoElement.srcObject = null;
    }
    if (this.audioElement) {
      this.audioElement.srcObject = null;
    }

    this.eventBus?.emit("webrtc:cleaned-up");
  }

  /**
   * 创建PeerConnection
   */
  async _createPeerConnection() {
    if (this.pc) {
      this.pc.close();
    }

    const config = {
      iceServers: this.iceServers,
      iceTransportPolicy: "all",
      bundlePolicy: "balanced",
      rtcpMuxPolicy: "require",
    };

    console.log("🔧 创建PeerConnection，使用配置:", config);

    this.pc = new RTCPeerConnection(config);
    this._setupPeerConnectionEvents();

    Logger.info("WebRTC: PeerConnection已创建");
  }

  /**
   * 设置PeerConnection事件
   */
  _setupPeerConnectionEvents() {
    // ICE候选事件
    this.pc.onicecandidate = (event) => {
      if (event.candidate) {
        const candidate = event.candidate;
        console.log("🎯 收到ICE候选:", {
          type: candidate.type,
          protocol: candidate.protocol,
          address: candidate.address || "hidden",
          port: candidate.port,
          foundation: candidate.foundation,
          priority: candidate.priority,
        });
        
        Logger.debug("WebRTC: 发送ICE候选");
        this.signalingManager?.send("ice-candidate", candidate).catch((error) => {
          console.error("❌ 发送ICE候选失败:", error);
        });
      } else {
        console.log("🏁 ICE候选收集完成");
        this._clearTimer("iceTimer");
      }
    };

    // ICE连接状态变化
    this.pc.oniceconnectionstatechange = () => {
      const state = this.pc.iceConnectionState;
      this.iceConnectionState = state;
      console.log(`🔗 ICE连接状态变化: ${state}`);

      switch (state) {
        case "checking":
          console.log("🔍 ICE连接检查中...");
          break;
        case "connected":
          console.log("✅ ICE连接已建立");
          this._handleConnectionSuccess();
          break;
        case "completed":
          console.log("🎉 ICE连接完成");
          this._handleConnectionSuccess();
          break;
        case "failed":
          console.error("❌ ICE连接失败");
          this._handleConnectionError(new Error(`ICE连接失败: ${state}`));
          break;
        case "disconnected":
          console.warn("⚠️ ICE连接断开");
          this._handleConnectionError(new Error(`ICE连接断开: ${state}`));
          break;
        case "closed":
          console.log("🔒 ICE连接已关闭");
          break;
      }

      this.eventBus?.emit("webrtc:ice-connection-state-change", { state });
    };

    // 连接状态变化
    this.pc.onconnectionstatechange = () => {
      const state = this.pc.connectionState;
      console.log(`📡 PeerConnection状态变化: ${state}`);

      if (state === "failed") {
        console.error("❌ PeerConnection失败");
        this._handleConnectionError(new Error("PeerConnection失败"));
      }

      this.eventBus?.emit("webrtc:connection-state-change", { state });
    };

    // 信令状态变化
    this.pc.onsignalingstatechange = () => {
      this.signalingState = this.pc.signalingState;
      console.log(`📋 信令状态变化: ${this.signalingState}`);
      this.eventBus?.emit("webrtc:signaling-state-change", {
        state: this.signalingState,
      });
    };

    // 媒体轨道接收
    this.pc.ontrack = (event) => {
      console.log("🎬 接收到媒体轨道:", event.track.kind);
      this._handleRemoteTrack(event);
    };
  }



  /**
   * 设置信令事件
   */
  _setupSignalingEvents() {
    if (!this.signalingManager) return;

    // 信令连接建立后，只是记录状态，不自动请求offer
    this.eventBus?.on("signaling:connected", () => {
      console.log("🔗 信令连接已建立，等待用户开始捕获...");
    });

    // 接收欢迎消息
    this.eventBus?.on("signaling:welcome", (data) => {
      console.log("👋 收到服务器欢迎消息:", data);
    });

    // 接收offer
    this.eventBus?.on("signaling:offer", async (data) => {
      try {
        await this.handleOffer(data);
      } catch (error) {
        Logger.error("WebRTC: 处理offer失败", error);
        this._handleConnectionError(error);
      }
    });

    // 接收ICE候选
    this.eventBus?.on("signaling:ice-candidate", async (data) => {
      try {
        await this.handleIceCandidate(data);
      } catch (error) {
        Logger.error("WebRTC: 处理ICE候选失败", error);
      }
    });

    // 处理信令错误
    this.eventBus?.on("signaling:error", (error) => {
      console.error("❌ 信令错误:", error);
      // 只有在WebRTC连接过程中才处理信令错误
      if (this.connectionState === "connecting") {
        this._handleConnectionError(new Error(`信令错误: ${error.message || error}`));
      }
    });

    // 处理信令断开
    this.eventBus?.on("signaling:disconnected", () => {
      console.warn("⚠️ 信令连接断开");
      // 只有在WebRTC连接过程中才处理信令断开
      if (this.connectionState === "connecting" || this.connectionState === "connected") {
        this._handleConnectionError(new Error("信令连接断开"));
      }
    });
  }

  /**
   * 开始连接流程
   */
  async _startConnection() {
    // 设置连接超时
    this._setTimer(
      "connectionTimer",
      () => {
        console.error("⏰ WebRTC连接超时");
        this._handleConnectionError(new Error("连接超时"));
      },
      this.connectionTimeout
    );

    // 设置ICE超时
    this._setTimer(
      "iceTimer",
      () => {
        console.warn("⏰ ICE收集超时");
      },
      this.iceTimeout
    );

    // 检查信令连接状态
    if (!this._isSignalingConnected()) {
      console.log("❌ 信令连接未就绪，无法启动WebRTC连接");
      throw new Error("信令连接未就绪");
    }

    // 发送request-offer请求
    console.log("📤 发送request-offer请求...");
    this._requestOffer();

    console.log("✅ 连接流程已启动，等待offer...");
  }

  /**
   * 检查信令连接状态
   */
  _isSignalingConnected() {
    if (!this.signalingManager) {
      console.log("🔍 信令管理器不存在");
      return false;
    }
    
    const isConnected = this.signalingManager.readyState === WebSocket.OPEN;
    console.log("🔍 信令连接状态检查:", {
      readyState: this.signalingManager.readyState,
      WebSocket_OPEN: WebSocket.OPEN,
      isConnected: isConnected
    });
    
    return isConnected;
  }

  /**
   * 等待信令连接
   */
  _waitForSignaling() {
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(new Error("信令连接超时"));
      }, 10000);

      const checkConnection = () => {
        if (this._isSignalingConnected()) {
          clearTimeout(timeout);
          resolve();
        } else {
          setTimeout(checkConnection, 100);
        }
      };

      checkConnection();
    });
  }

  /**
   * 请求WebRTC offer
   */
  _requestOffer() {
    if (!this.signalingManager) {
      console.error("❌ 信令管理器不可用");
      return;
    }

    console.log("📤 发送request-offer请求...");
    
    this.signalingManager.send("request-offer", {
      timestamp: Date.now(),
      client_info: {
        user_agent: navigator.userAgent,
        platform: navigator.platform,
        webrtc_support: true
      }
    }).then(() => {
      console.log("✅ request-offer请求已发送");
    }).catch((error) => {
      console.error("❌ 发送request-offer失败:", error);
      this._handleConnectionError(error);
    });
  }

  /**
   * 处理offer
   */
  async handleOffer(offer) {
    console.log("📥 收到offer，开始处理...");

    if (!this.pc) {
      await this._createPeerConnection();
    }

    await this.pc.setRemoteDescription(offer);
    console.log("✅ 远程描述已设置");

    const answer = await this.pc.createAnswer();
    await this.pc.setLocalDescription(answer);
    console.log("✅ 本地描述已设置，开始ICE候选收集...");

    this.signalingManager?.send("answer", answer).then(() => {
      console.log("📤 answer已发送");
    }).catch((error) => {
      console.error("❌ 发送answer失败:", error);
      throw error;
    });

    return answer;
  }

  /**
   * 处理ICE候选
   */
  async handleIceCandidate(candidateData) {
    console.log("📥 收到远程ICE候选原始数据:", candidateData);
    
    // 检查数据格式
    if (!candidateData || typeof candidateData !== 'object') {
      console.warn("⚠️ ICE候选数据格式无效:", candidateData);
      return;
    }

    console.log("📥 ICE候选详细信息:", {
      candidate: candidateData.candidate,
      sdpMid: candidateData.sdpMid,
      sdpMLineIndex: candidateData.sdpMLineIndex,
    });

    if (this.pc && this.pc.remoteDescription) {
      try {
        // 检查必需字段
        if (!candidateData.candidate || candidateData.candidate.trim() === '') {
          console.warn("⚠️ ICE候选缺少candidate字段或为空");
          return;
        }

        // 创建RTCIceCandidate对象
        const candidate = new RTCIceCandidate({
          candidate: candidateData.candidate,
          sdpMid: candidateData.sdpMid,
          sdpMLineIndex: candidateData.sdpMLineIndex,
        });
        
        await this.pc.addIceCandidate(candidate);
        console.log("✅ 远程ICE候选已添加");
      } catch (error) {
        console.warn("⚠️ ICE候选添加失败:", error);
        console.warn("⚠️ 候选数据:", candidateData);
        // 不抛出错误，继续处理其他候选
      }
    } else {
      console.warn("⚠️ 远程描述未设置，缓存ICE候选");
    }
  }

  /**
   * 处理远程媒体轨道
   */
  _handleRemoteTrack(event) {
    const stream = event.streams[0];
    const track = event.track;

    console.log(`🎬 接收到${track.kind}轨道，设置到媒体元素`);

    this.remoteStream = stream;

    if (track.kind === "video" && this.videoElement) {
      this.videoElement.srcObject = stream;
      this.videoElement
        .play()
        .then(() => {
          console.log("▶️ 视频播放开始");
        })
        .catch((error) => {
          console.warn("⚠️ 视频自动播放失败:", error);
        });
    } else if (track.kind === "audio" && this.audioElement) {
      this.audioElement.srcObject = stream;
      this.audioElement
        .play()
        .then(() => {
          console.log("🔊 音频播放开始");
        })
        .catch((error) => {
          console.warn("⚠️ 音频自动播放失败:", error);
        });
    }

    this.eventBus?.emit("webrtc:track-received", {
      kind: track.kind,
      stream: stream,
    });
  }

  /**
   * 处理连接成功
   */
  _handleConnectionSuccess() {
    console.log("🎉 WebRTC连接成功建立！");

    this._clearTimers();
    this._setState("connected");
    this.connectionAttempts = 0;
    this.connectionStats.connectTime = Date.now();
    this.connectionStats.totalConnections++;

    this.eventBus?.emit("webrtc:connected");
    this._startStatsCollection();
  }

  /**
   * 处理连接错误
   */
  _handleConnectionError(error) {
    console.error("❌ WebRTC连接错误:", error.message);

    this._clearTimers();
    this._setState("failed");

    this.eventBus?.emit("webrtc:connection-failed", { error });

    // 决定是否重连
    if (this.connectionAttempts < this.maxConnectionAttempts) {
      const delay = Math.min(2000 * Math.pow(2, this.connectionAttempts - 1), 30000);
      console.log(`🔄 ${delay}ms后重连 (${this.connectionAttempts}/${this.maxConnectionAttempts})`);

      this.connectionStats.reconnectAttempts++;
      this.eventBus?.emit("webrtc:reconnecting", {
        attempt: this.connectionAttempts,
        maxAttempts: this.maxConnectionAttempts,
        delay,
      });

      this._setTimer("reconnectTimer", () => {
        this.connect();
      }, delay);
    } else {
      console.error("🛑 达到最大重连次数，停止重连");
      this.eventBus?.emit("webrtc:connection-permanently-failed");
    }
  }

  /**
   * 开始统计收集
   */
  _startStatsCollection() {
    if (!this.pc) return;

    const collectStats = async () => {
      try {
        const stats = await this.pc.getStats();
        this._processStats(stats);
      } catch (error) {
        Logger.error("WebRTC: 统计收集失败", error);
      }
    };

    // 立即收集一次
    collectStats();

    // 定期收集
    this.statsTimer = setInterval(collectStats, 5000);
  }

  /**
   * 处理统计数据
   */
  _processStats(stats) {
    let bytesReceived = 0;
    let packetsLost = 0;

    stats.forEach((stat) => {
      if (stat.type === "inbound-rtp") {
        bytesReceived += stat.bytesReceived || 0;
        packetsLost += stat.packetsLost || 0;
      }
    });

    this.connectionStats.bytesReceived = bytesReceived;
    this.connectionStats.packetsLost = packetsLost;

    this.eventBus?.emit("webrtc:stats-updated", this.connectionStats);
  }

  /**
   * 获取连接统计
   */
  getConnectionStats() {
    return {
      connectionState: this.connectionState,
      iceConnectionState: this.iceConnectionState,
      signalingState: this.signalingState,
      ...this.connectionStats,
    };
  }

  /**
   * 设置状态
   */
  _setState(newState) {
    const oldState = this.connectionState;
    this.connectionState = newState;

    console.log(`📊 WebRTC状态变化: ${oldState} -> ${newState}`);
    this.eventBus?.emit("webrtc:state-changed", {
      from: oldState,
      to: newState,
    });
  }

  /**
   * 设置定时器
   */
  _setTimer(name, callback, delay) {
    this._clearTimer(name);
    this[name] = setTimeout(callback, delay);
  }

  /**
   * 清除定时器
   */
  _clearTimer(name) {
    if (this[name]) {
      clearTimeout(this[name]);
      this[name] = null;
    }
  }

  /**
   * 清除所有定时器
   */
  _clearTimers() {
    this._clearTimer("connectionTimer");
    this._clearTimer("iceTimer");
    this._clearTimer("reconnectTimer");

    if (this.statsTimer) {
      clearInterval(this.statsTimer);
      this.statsTimer = null;
    }
  }

  /**
   * 停止重连
   */
  stopReconnecting() {
    console.log("🛑 手动停止重连");
    this._clearTimer("reconnectTimer");
    this.connectionAttempts = this.maxConnectionAttempts; // 防止进一步重连
    this.eventBus?.emit("webrtc:reconnection-stopped");
  }
}

// 导出
if (typeof module !== "undefined" && module.exports) {
  module.exports = WebRTCManager;
} else if (typeof window !== "undefined") {
  window.WebRTCManager = WebRTCManager;

  // 添加全局调试函数
  window.printWebRTCConfig = function () {
    console.log("📋 当前WebRTC配置:");

    if (window.appManager && window.appManager.modules.webrtc) {
      const webrtc = window.appManager.modules.webrtc;
      console.log("ICE服务器:", webrtc.iceServers);
      console.log("连接超时:", webrtc.connectionTimeout);
      console.log("最大重试次数:", webrtc.maxConnectionAttempts);
      console.log("当前状态:", webrtc.getConnectionStats());
    } else {
      console.error("❌ WebRTC管理器不可用");
    }
  };
}
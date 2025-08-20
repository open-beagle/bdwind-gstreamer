/**
 * WebRTC Adapter 验证
 * 验证 webrtc-adapter 库是否正确加载
 * 优化：缓存验证结果，避免重复检查
 */
let _adapterVerified = null;
function verifyWebRTCAdapter() {
  if (_adapterVerified !== null) return _adapterVerified;
  if (typeof adapter !== "undefined") {
    console.log("✅ WebRTC Adapter 已加载:", {
      version: adapter.browserDetails?.version || "unknown",
      browser: adapter.browserDetails?.browser || "unknown",
      adapterVersion: "9.0.1",
    });

    // 验证关键功能是否可用
    const hasRTCPeerConnection = typeof RTCPeerConnection !== "undefined";
    const hasGetUserMedia =
      navigator.mediaDevices &&
      typeof navigator.mediaDevices.getUserMedia === "function";

    console.log("🔍 WebRTC API 可用性检查:", {
      RTCPeerConnection: hasRTCPeerConnection,
      getUserMedia: hasGetUserMedia,
      adapter: true,
    });

    _adapterVerified = true;
    return true;
  } else {
    console.error(
      "❌ WebRTC Adapter 未加载！请检查 webrtc-adapter-9.0.1.min.js 是否正确引入"
    );
    _adapterVerified = false;
    return false;
  }
}

// 立即验证 adapter 是否可用
verifyWebRTCAdapter();

/**
 * 性能监控工具
 */
class PerformanceMonitor {
  static startTimer(name) {
    if (!window._perfTimers) window._perfTimers = {};
    window._perfTimers[name] = performance.now();
  }

  static endTimer(name) {
    if (!window._perfTimers || !window._perfTimers[name]) return 0;
    const duration = performance.now() - window._perfTimers[name];
    delete window._perfTimers[name];
    return duration;
  }

  static logTiming(name, duration) {
    if (duration > 100) { // 只记录超过100ms的操作
      console.log(`⏱️ ${name}: ${duration.toFixed(2)}ms`);
    }
  }
}

/**
 * 简化的 WebRTC 管理器
 * 基于 webrtc-adapter 的标准化实现，移除复杂的重试和状态验证逻辑
 * 优化版本：减少内存使用，提高连接速度，简化错误处理
 */
class WebRTCManager {
  constructor(eventBus, config) {
    // 核心依赖
    this.eventBus = eventBus;
    this.config = config;
    this.logger = window.EnhancedLogger || console;

    // WebRTC 核心组件
    this.pc = null;
    this.signalingManager = null;

    // 连接状态管理
    this.connectionState = "disconnected"; // disconnected, connecting, connected, failed
    this.iceConnectionState = "closed";
    this.signalingState = "closed";

    // 媒体元素
    this.videoElement = null;
    this.audioElement = null;
    this.remoteStream = null;

    // 数据通道
    this.sendChannel = null;
    this.receiveChannel = null;

    // 配置参数（优化：减少超时时间）
    this.iceServers = [];
    this.connectionTimeout = 15000; // 减少到15秒
    this.maxRetryAttempts = 2; // 减少重试次数
    this.retryDelay = 1500; // 减少重试延迟

    // 定时器
    this.connectionTimer = null;
    this.retryTimer = null;

    // 重试状态（简化版）
    this.retryCount = 0;
    this.connectionEstablished = false;
    this.lastError = null;
    this.errorHistory = [];

    // 统计信息（增强版）
    this.connectionStats = {
      connectTime: null,
      disconnectTime: null,
      totalConnections: 0,
      reconnectAttempts: 0,
      // 新增的详细统计
      connectionDuration: 0,
      bytesReceived: 0,
      bytesSent: 0,
      packetsReceived: 0,
      packetsSent: 0,
      packetsLost: 0,
      jitter: 0,
      roundTripTime: 0,
      availableIncomingBitrate: 0,
      availableOutgoingBitrate: 0,
      currentRoundTripTime: 0,
      totalRoundTripTime: 0,
      roundTripTimeMeasurements: 0,
      // 连接质量指标
      connectionQuality: "unknown", // excellent, good, fair, poor, unknown
      qualityScore: 0, // 0-100
      // 媒体流统计
      videoStats: {
        framesReceived: 0,
        framesDecoded: 0,
        framesDropped: 0,
        frameWidth: 0,
        frameHeight: 0,
        frameRate: 0,
        bitrate: 0,
      },
      audioStats: {
        samplesReceived: 0,
        samplesDecoded: 0,
        audioLevel: 0,
        bitrate: 0,
      },
      // 网络统计
      networkStats: {
        candidatePairs: 0,
        selectedCandidatePair: null,
        localCandidateType: "unknown",
        remoteCandidateType: "unknown",
        transportType: "unknown",
      },
    };

    // 统计收集定时器（优化：减少频率）
    this.statsCollectionTimer = null;
    this.statsCollectionInterval = 2000; // 2秒收集一次统计，减少CPU使用
    this.lastStatsTimestamp = 0;
    this._lastBytes = 0;

    // 错误分类常量
    this.ERROR_TYPES = {
      CONNECTION: "connection",
      MEDIA: "media",
      NETWORK: "network",
      CONFIG: "config",
      SIGNALING: "signaling",
      TIMEOUT: "timeout",
    };

    // 错误严重级别
    this.ERROR_LEVELS = {
      WARNING: "warning",
      ERROR: "error",
      FATAL: "fatal",
    };

    // 初始化
    this._loadConfig();
    this._initializeLogging();

    this.logger.info("WebRTC", "WebRTCManager 初始化完成", {
      adapterVersion: typeof adapter !== "undefined" ? "9.0.1" : "not-loaded",
      browserDetails:
        typeof adapter !== "undefined" ? adapter.browserDetails : null,
    });
  }

  /**
   * 加载配置
   * @private
   */
  _loadConfig() {
    // 设置默认 ICE 服务器
    const defaultIceServers = [{ urls: "stun:stun.ali.wodcloud.com:3478" }];

    if (this.config) {
      const webrtcConfig = this.config.get("webrtc", {});
      this.iceServers = webrtcConfig.iceServers || defaultIceServers;
      this.connectionTimeout =
        webrtcConfig.connectionTimeout || this.connectionTimeout;
      this.maxRetryAttempts =
        webrtcConfig.maxRetryAttempts || this.maxRetryAttempts;
      this.retryDelay = webrtcConfig.retryDelay || this.retryDelay;
    } else {
      this.iceServers = defaultIceServers;
    }

    this.logger.info("WebRTC", "配置加载完成", {
      iceServersCount: this.iceServers.length,
      connectionTimeout: this.connectionTimeout,
      maxRetryAttempts: this.maxRetryAttempts,
    });
  }

  /**
   * 初始化日志记录
   * @private
   */
  _initializeLogging() {
    // 设置日志级别
    if (this.config) {
      const logLevel = this.config.get("ui.logLevel", "info");
      if (this.logger.setLevel) {
        this.logger.setLevel(logLevel);
      }
    }

    this.logger.info("WebRTC", "日志系统初始化完成");
  }

  /**
   * 初始化 WebRTC 管理器
   */
  async initialize() {
    try {
      this.logger.info("WebRTC", "开始初始化 WebRTC 管理器");

      // 验证 webrtc-adapter 是否可用
      if (!verifyWebRTCAdapter()) {
        throw new Error("WebRTC Adapter 未正确加载");
      }

      // 从服务器获取最新配置
      await this._fetchServerConfig();

      this.logger.info("WebRTC", "WebRTC 管理器初始化完成", {
        iceServersCount: this.iceServers.length,
        adapterReady: typeof adapter !== "undefined",
      });

      // 触发初始化完成事件
      this.eventBus?.emit("webrtc:initialized", {
        iceServers: this.iceServers,
        config: this._getPublicConfig(),
      });

      return Promise.resolve();
    } catch (error) {
      this.logger.error("WebRTC", "初始化失败", error);
      this.eventBus?.emit("webrtc:initialization-failed", { error });
      throw error;
    }
  }

  /**
   * 从服务器获取配置
   * @private
   */
  async _fetchServerConfig() {
    try {
      this.logger.info("WebRTC", "从服务器获取 WebRTC 配置");

      if (this.config && typeof this.config.fetchWebRTCConfig === "function") {
        const serverConfig = await this.config.fetchWebRTCConfig(true);

        if (
          serverConfig &&
          serverConfig.iceServers &&
          Array.isArray(serverConfig.iceServers)
        ) {
          this.iceServers = serverConfig.iceServers;
          this.logger.info("WebRTC", "服务器配置更新成功", {
            iceServersCount: this.iceServers.length,
          });

          // 触发配置更新事件
          this.eventBus?.emit("webrtc:config-updated", {
            source: "server",
            iceServers: this.iceServers,
          });
        } else {
          this.logger.warn("WebRTC", "服务器配置无效，使用默认配置");
        }
      } else {
        this.logger.warn("WebRTC", "配置管理器不可用，使用当前配置");
      }
    } catch (error) {
      this.logger.error("WebRTC", "获取服务器配置失败", error);
      // 继续使用当前配置，不抛出错误
    }
  }

  /**
   * 设置信令管理器
   */
  setSignalingManager(signalingManager) {
    this.signalingManager = signalingManager;
    this._setupSignalingEvents();
    this.logger.info("WebRTC", "信令管理器已设置");
  }

  /**
   * 设置媒体元素
   * 增强版本，支持更好的错误处理和跨浏览器兼容性
   */
  setMediaElements(videoElement, audioElement) {
    try {
      // 验证媒体元素
      if (videoElement && !(videoElement instanceof HTMLVideoElement)) {
        throw new Error("提供的视频元素不是有效的 HTMLVideoElement");
      }

      if (audioElement && !(audioElement instanceof HTMLAudioElement)) {
        throw new Error("提供的音频元素不是有效的 HTMLAudioElement");
      }

      // 清理之前的媒体元素事件监听器
      this._cleanupMediaElementEvents();

      // 设置新的媒体元素
      this.videoElement = videoElement;
      this.audioElement = audioElement;

      // 设置媒体元素事件监听器
      this._setupMediaElementEvents();

      // 如果已有远程流，立即应用到新的媒体元素
      if (this.remoteStream) {
        this._applyRemoteStreamToElements(this.remoteStream);
      }

      this.logger.info("WebRTC", "媒体元素已设置", {
        hasVideo: !!videoElement,
        hasAudio: !!audioElement,
        videoElementType: videoElement ? videoElement.constructor.name : null,
        audioElementType: audioElement ? audioElement.constructor.name : null,
      });

      // 触发媒体元素设置事件
      this.eventBus?.emit("webrtc:media-elements-set", {
        hasVideo: !!videoElement,
        hasAudio: !!audioElement,
        timestamp: Date.now(),
      });
    } catch (error) {
      this.logger.error("WebRTC", "设置媒体元素失败", error);
      this._handleMediaError("MEDIA_ELEMENT_SETUP_FAILED", error);
      throw error;
    }
  }

  /**
   * 获取连接状态
   */
  getConnectionState() {
    return {
      connectionState: this.connectionState,
      iceConnectionState: this.iceConnectionState,
      signalingState: this.signalingState,
      connectionEstablished: this.connectionEstablished,
      retryCount: this.retryCount,
    };
  }

  /**
   * 获取连接统计信息（增强版）
   */
  getConnectionStats() {
    const currentTime = Date.now();
    const uptime = this.connectionStats.connectTime
      ? currentTime - this.connectionStats.connectTime
      : 0;

    return {
      // 基础统计信息
      ...this.connectionStats,
      currentState: this.getConnectionState(),
      uptime: uptime,
      uptimeFormatted: this._formatDuration(uptime),
      errorStats: this._getErrorStats(),

      // 连接质量评估
      qualityAssessment: this._assessConnectionQuality(),

      // 性能指标
      performanceMetrics: {
        averageRtt:
          this.connectionStats.roundTripTimeMeasurements > 0
            ? this.connectionStats.totalRoundTripTime /
              this.connectionStats.roundTripTimeMeasurements
            : 0,
        packetLossRate:
          this.connectionStats.packetsSent > 0
            ? (this.connectionStats.packetsLost /
                this.connectionStats.packetsSent) *
              100
            : 0,
        jitterMs: this.connectionStats.jitter,
        bitrateMbps: {
          incoming: (
            this.connectionStats.availableIncomingBitrate / 1000000
          ).toFixed(2),
          outgoing: (
            this.connectionStats.availableOutgoingBitrate / 1000000
          ).toFixed(2),
        },
      },

      // 数据传输统计
      dataTransfer: {
        totalBytesReceived: this.connectionStats.bytesReceived,
        totalBytesSent: this.connectionStats.bytesSent,
        totalBytesReceivedFormatted: this._formatBytes(
          this.connectionStats.bytesReceived
        ),
        totalBytesSentFormatted: this._formatBytes(
          this.connectionStats.bytesSent
        ),
        packetsReceived: this.connectionStats.packetsReceived,
        packetsSent: this.connectionStats.packetsSent,
        packetsLost: this.connectionStats.packetsLost,
      },

      // 媒体流统计
      media: {
        video: {
          ...this.connectionStats.videoStats,
          resolution: `${this.connectionStats.videoStats.frameWidth}x${this.connectionStats.videoStats.frameHeight}`,
          bitrateFormatted: this._formatBitrate(
            this.connectionStats.videoStats.bitrate
          ),
        },
        audio: {
          ...this.connectionStats.audioStats,
          bitrateFormatted: this._formatBitrate(
            this.connectionStats.audioStats.bitrate
          ),
        },
      },

      // 网络信息
      network: {
        ...this.connectionStats.networkStats,
        connectionType: this._getConnectionType(),
      },

      // 时间戳
      lastUpdated: currentTime,
      lastUpdatedFormatted: new Date(currentTime).toLocaleString(),
    };
  }

  /**
   * 获取错误统计信息
   * @private
   */
  _getErrorStats() {
    const errorCounts = {};
    const errorLevelCounts = {};

    // 统计错误类型和级别
    this.errorHistory.forEach((error) => {
      errorCounts[error.type] = (errorCounts[error.type] || 0) + 1;
      errorLevelCounts[error.level] = (errorLevelCounts[error.level] || 0) + 1;
    });

    return {
      totalErrors: this.errorHistory.length,
      errorCounts,
      errorLevelCounts,
      lastError: this.lastError,
      recentErrors: this.errorHistory.slice(-3), // 最近3个错误
    };
  }

  /**
   * 获取详细的错误信息
   */
  getErrorInfo() {
    return {
      lastError: this.lastError,
      errorHistory: [...this.errorHistory],
      errorStats: this._getErrorStats(),
      retryInfo: {
        currentRetryCount: this.retryCount,
        maxRetryAttempts: this.maxRetryAttempts,
        totalReconnectAttempts: this.connectionStats.reconnectAttempts,
      },
    };
  }

  /**
   * 获取实时 WebRTC 统计信息
   * 从 PeerConnection 获取详细的统计数据
   */
  async getRealTimeStats() {
    if (!this.pc || this.pc.connectionState !== "connected") {
      return null;
    }

    try {
      const stats = await this.pc.getStats();
      const parsedStats = this._parseWebRTCStats(stats);

      // 更新内部统计信息
      this._updateInternalStats(parsedStats);

      return parsedStats;
    } catch (error) {
      this.logger.error("WebRTC", "获取实时统计信息失败", error);
      return null;
    }
  }

  /**
   * 解析 WebRTC 统计信息
   * @private
   */
  _parseWebRTCStats(stats) {
    const parsedStats = {
      timestamp: Date.now(),
      connection: {},
      inbound: {},
      outbound: {},
      candidate: {},
      transport: {},
    };

    stats.forEach((report) => {
      switch (report.type) {
        case "inbound-rtp":
          if (report.mediaType === "video") {
            parsedStats.inbound.video = {
              bytesReceived: report.bytesReceived || 0,
              packetsReceived: report.packetsReceived || 0,
              packetsLost: report.packetsLost || 0,
              framesReceived: report.framesReceived || 0,
              framesDecoded: report.framesDecoded || 0,
              framesDropped: report.framesDropped || 0,
              frameWidth: report.frameWidth || 0,
              frameHeight: report.frameHeight || 0,
              framesPerSecond: report.framesPerSecond || 0,
              jitter: report.jitter || 0,
              totalDecodeTime: report.totalDecodeTime || 0,
            };
          } else if (report.mediaType === "audio") {
            parsedStats.inbound.audio = {
              bytesReceived: report.bytesReceived || 0,
              packetsReceived: report.packetsReceived || 0,
              packetsLost: report.packetsLost || 0,
              jitter: report.jitter || 0,
              audioLevel: report.audioLevel || 0,
              totalAudioEnergy: report.totalAudioEnergy || 0,
              totalSamplesDuration: report.totalSamplesDuration || 0,
            };
          }
          break;

        case "outbound-rtp":
          if (report.mediaType === "video") {
            parsedStats.outbound.video = {
              bytesSent: report.bytesSent || 0,
              packetsSent: report.packetsSent || 0,
              framesEncoded: report.framesEncoded || 0,
              frameWidth: report.frameWidth || 0,
              frameHeight: report.frameHeight || 0,
              framesPerSecond: report.framesPerSecond || 0,
              totalEncodeTime: report.totalEncodeTime || 0,
            };
          } else if (report.mediaType === "audio") {
            parsedStats.outbound.audio = {
              bytesSent: report.bytesSent || 0,
              packetsSent: report.packetsSent || 0,
            };
          }
          break;

        case "candidate-pair":
          if (report.state === "succeeded" || report.nominated) {
            parsedStats.candidate = {
              state: report.state,
              nominated: report.nominated,
              bytesReceived: report.bytesReceived || 0,
              bytesSent: report.bytesSent || 0,
              currentRoundTripTime: report.currentRoundTripTime || 0,
              totalRoundTripTime: report.totalRoundTripTime || 0,
              responsesReceived: report.responsesReceived || 0,
              availableIncomingBitrate: report.availableIncomingBitrate || 0,
              availableOutgoingBitrate: report.availableOutgoingBitrate || 0,
              localCandidateId: report.localCandidateId,
              remoteCandidateId: report.remoteCandidateId,
            };
          }
          break;

        case "local-candidate":
          if (report.id === parsedStats.candidate.localCandidateId) {
            parsedStats.transport.local = {
              candidateType: report.candidateType,
              protocol: report.protocol,
              address: report.address,
              port: report.port,
            };
          }
          break;

        case "remote-candidate":
          if (report.id === parsedStats.candidate.remoteCandidateId) {
            parsedStats.transport.remote = {
              candidateType: report.candidateType,
              protocol: report.protocol,
              address: report.address,
              port: report.port,
            };
          }
          break;

        case "transport":
          parsedStats.transport.info = {
            bytesReceived: report.bytesReceived || 0,
            bytesSent: report.bytesSent || 0,
            dtlsState: report.dtlsState,
            selectedCandidatePairId: report.selectedCandidatePairId,
          };
          break;
      }
    });

    return parsedStats;
  }

  /**
   * 更新内部统计信息（优化版）
   * @private
   */
  _updateInternalStats(parsedStats) {
    const currentTime = Date.now();

    // 优化：只更新关键统计信息
    if (parsedStats.candidate) {
      this.connectionStats.bytesReceived = parsedStats.candidate.bytesReceived || 0;
      this.connectionStats.bytesSent = parsedStats.candidate.bytesSent || 0;
      this.connectionStats.currentRoundTripTime = parsedStats.candidate.currentRoundTripTime || 0;
      this.connectionStats.availableIncomingBitrate = parsedStats.candidate.availableIncomingBitrate || 0;
      this.connectionStats.availableOutgoingBitrate = parsedStats.candidate.availableOutgoingBitrate || 0;

      // 优化：减少RTT计算频率
      if (parsedStats.candidate.currentRoundTripTime > 0 && this.connectionStats.roundTripTimeMeasurements % 5 === 0) {
        this.connectionStats.totalRoundTripTime += parsedStats.candidate.currentRoundTripTime;
        this.connectionStats.roundTripTimeMeasurements++;
        this.connectionStats.roundTripTime = parsedStats.candidate.currentRoundTripTime;
      }
    }

    // 更新视频统计
    if (parsedStats.inbound.video) {
      const video = parsedStats.inbound.video;
      this.connectionStats.videoStats = {
        framesReceived: video.framesReceived,
        framesDecoded: video.framesDecoded,
        framesDropped: video.framesDropped,
        frameWidth: video.frameWidth,
        frameHeight: video.frameHeight,
        frameRate: video.framesPerSecond,
        bitrate: this._calculateBitrate(video.bytesReceived, currentTime),
      };

      this.connectionStats.packetsReceived += video.packetsReceived || 0;
      this.connectionStats.packetsLost += video.packetsLost || 0;
      this.connectionStats.jitter = video.jitter || 0;
    }

    // 更新音频统计
    if (parsedStats.inbound.audio) {
      const audio = parsedStats.inbound.audio;
      this.connectionStats.audioStats = {
        samplesReceived: audio.packetsReceived || 0,
        samplesDecoded: audio.packetsReceived || 0, // 近似值
        audioLevel: audio.audioLevel || 0,
        bitrate: this._calculateBitrate(audio.bytesReceived, currentTime),
      };
    }

    // 更新网络统计
    if (parsedStats.transport) {
      this.connectionStats.networkStats = {
        candidatePairs: 1, // 当前活跃的候选对
        selectedCandidatePair: parsedStats.candidate,
        localCandidateType:
          parsedStats.transport.local?.candidateType || "unknown",
        remoteCandidateType:
          parsedStats.transport.remote?.candidateType || "unknown",
        transportType: parsedStats.transport.local?.protocol || "unknown",
      };
    }

    // 更新连接质量
    this.connectionStats.connectionQuality = this._calculateConnectionQuality();
    this.connectionStats.qualityScore = this._calculateQualityScore();

    this.lastStatsTimestamp = currentTime;
  }

  /**
   * 计算比特率（优化版）
   * @private
   */
  _calculateBitrate(bytes, currentTime) {
    if (!this.lastStatsTimestamp || this.lastStatsTimestamp === 0) {
      this._lastBytes = bytes;
      return 0;
    }

    const timeDiff = (currentTime - this.lastStatsTimestamp) / 1000;
    if (timeDiff <= 0) return 0;

    const bytesDiff = bytes - (this._lastBytes || 0);
    this._lastBytes = bytes;

    // 避免负值和异常大的值
    return Math.max(0, Math.min((bytesDiff * 8) / timeDiff, 1000000000)); // 限制最大1Gbps
  }

  /**
   * 计算连接质量
   * @private
   */
  _calculateConnectionQuality() {
    const rtt = this.connectionStats.currentRoundTripTime * 1000; // 转换为毫秒
    const packetLossRate =
      this.connectionStats.packetsSent > 0
        ? (this.connectionStats.packetsLost /
            this.connectionStats.packetsSent) *
          100
        : 0;
    const jitter = this.connectionStats.jitter * 1000; // 转换为毫秒

    // 基于 RTT、丢包率和抖动评估质量
    if (rtt < 50 && packetLossRate < 0.5 && jitter < 10) {
      return "excellent";
    } else if (rtt < 100 && packetLossRate < 1 && jitter < 20) {
      return "good";
    } else if (rtt < 200 && packetLossRate < 3 && jitter < 50) {
      return "fair";
    } else if (rtt > 0 || packetLossRate > 0 || jitter > 0) {
      return "poor";
    } else {
      return "unknown";
    }
  }

  /**
   * 计算质量分数 (0-100)
   * @private
   */
  _calculateQualityScore() {
    const rtt = this.connectionStats.currentRoundTripTime * 1000;
    const packetLossRate =
      this.connectionStats.packetsSent > 0
        ? (this.connectionStats.packetsLost /
            this.connectionStats.packetsSent) *
          100
        : 0;
    const jitter = this.connectionStats.jitter * 1000;

    // RTT 分数 (0-40分)
    let rttScore = 40;
    if (rtt > 0) {
      rttScore = Math.max(0, 40 - rtt / 10);
    }

    // 丢包率分数 (0-40分)
    let lossScore = Math.max(0, 40 - packetLossRate * 10);

    // 抖动分数 (0-20分)
    let jitterScore = 20;
    if (jitter > 0) {
      jitterScore = Math.max(0, 20 - jitter / 5);
    }

    return Math.round(rttScore + lossScore + jitterScore);
  }

  /**
   * 评估连接质量
   * @private
   */
  _assessConnectionQuality() {
    const quality = this.connectionStats.connectionQuality;
    const score = this.connectionStats.qualityScore;
    const rtt = this.connectionStats.currentRoundTripTime * 1000;
    const packetLossRate =
      this.connectionStats.packetsSent > 0
        ? (this.connectionStats.packetsLost /
            this.connectionStats.packetsSent) *
          100
        : 0;

    let recommendation = "";
    let issues = [];

    switch (quality) {
      case "excellent":
        recommendation = "连接质量优秀，体验流畅";
        break;
      case "good":
        recommendation = "连接质量良好，体验稳定";
        break;
      case "fair":
        recommendation = "连接质量一般，可能有轻微延迟";
        if (rtt > 100) issues.push("网络延迟较高");
        if (packetLossRate > 1) issues.push("存在少量丢包");
        break;
      case "poor":
        recommendation = "连接质量较差，建议检查网络";
        if (rtt > 200) issues.push("网络延迟过高");
        if (packetLossRate > 3) issues.push("丢包率过高");
        if (this.connectionStats.jitter * 1000 > 50)
          issues.push("网络抖动严重");
        break;
      default:
        recommendation = "连接质量未知，正在评估中";
    }

    return {
      quality: quality,
      score: score,
      recommendation: recommendation,
      issues: issues,
      metrics: {
        rtt: Math.round(rtt),
        packetLoss: Math.round(packetLossRate * 100) / 100,
        jitter: Math.round(this.connectionStats.jitter * 1000),
      },
    };
  }

  /**
   * 清除错误历史记录
   */
  clearErrorHistory() {
    this.errorHistory = [];
    this.lastError = null;
    this.logger.info("WebRTC", "错误历史记录已清除");
  }

  /**
   * 优化连接建立时间
   * @private
   */
  _optimizeConnectionEstablishment() {
    if (!this.pc) return;

    // 优化：预先收集ICE候选
    this.pc.addEventListener('icegatheringstatechange', () => {
      if (this.pc.iceGatheringState === 'complete') {
        this.logger.debug("WebRTC", "ICE候选收集完成，连接优化");
      }
    });

    // 优化：设置更短的ICE超时
    if (this.pc.setConfiguration) {
      this.pc.setConfiguration({
        ...this.pc.getConfiguration(),
        iceTransportPolicy: 'all',
        iceCandidatePoolSize: 4
      });
    }
  }

  /**
   * 完全清理资源（优化：防止内存泄漏）
   */
  async cleanup() {
    try {
      this.logger.info("WebRTC", "开始清理 WebRTC 资源");

      // 停止统计收集
      if (this.statsCollectionTimer) {
        clearInterval(this.statsCollectionTimer);
        this.statsCollectionTimer = null;
      }

      // 清理定时器
      if (this.connectionTimer) {
        clearTimeout(this.connectionTimer);
        this.connectionTimer = null;
      }

      if (this.retryTimer) {
        clearTimeout(this.retryTimer);
        this.retryTimer = null;
      }

      // 关闭数据通道
      if (this.sendChannel) {
        this.sendChannel.close();
        this.sendChannel = null;
      }

      if (this.receiveChannel) {
        this.receiveChannel.close();
        this.receiveChannel = null;
      }

      // 清理媒体元素
      this._cleanupMediaElementEvents();
      if (this.remoteStream) {
        this.remoteStream.getTracks().forEach(track => track.stop());
        this.remoteStream = null;
      }

      // 关闭 PeerConnection
      if (this.pc) {
        this.pc.close();
        this.pc = null;
      }

      // 清理状态
      this.connectionState = "disconnected";
      this.iceConnectionState = "closed";
      this.signalingState = "closed";
      this.connectionEstablished = false;
      this.retryCount = 0;

      // 清理错误历史
      this.clearErrorHistory();

      this.logger.info("WebRTC", "WebRTC 资源清理完成");
    } catch (error) {
      this.logger.error("WebRTC", "清理资源时出错", error);
    }
  }

  /**
   * 获取用户友好的错误信息和恢复建议
   */
  getUserFriendlyErrorInfo() {
    if (!this.lastError) {
      return {
        hasError: false,
        message: "连接正常",
        suggestion: null,
      };
    }

    const error = this.lastError;
    let suggestion = "";

    switch (error.type) {
      case this.ERROR_TYPES.NETWORK:
        suggestion = "请检查网络连接，系统会自动重试";
        break;
      case this.ERROR_TYPES.CONNECTION:
        suggestion = "连接出现问题，正在尝试重新连接";
        break;
      case this.ERROR_TYPES.SIGNALING:
        suggestion = "信令服务器连接问题，请稍后重试";
        break;
      case this.ERROR_TYPES.CONFIG:
        suggestion = "配置错误，请联系管理员检查设置";
        break;
      case this.ERROR_TYPES.TIMEOUT:
        suggestion = "连接超时，请检查网络状况";
        break;
      case this.ERROR_TYPES.MEDIA:
        suggestion = "媒体处理问题，可能影响音视频质量";
        break;
      default:
        suggestion = "出现未知问题，正在尝试恢复";
    }

    // 如果正在重试，添加重试信息
    if (this.retryCount > 0 && this.retryCount < this.maxRetryAttempts) {
      suggestion += ` (重试 ${this.retryCount}/${this.maxRetryAttempts})`;
    } else if (this.retryCount >= this.maxRetryAttempts) {
      suggestion = "多次重试失败，请刷新页面或联系技术支持";
    }

    return {
      hasError: true,
      message: error.userMessage,
      suggestion: suggestion,
      errorType: error.type,
      errorLevel: error.level,
      canRetry: error.shouldRetry && this.retryCount < this.maxRetryAttempts,
      retryInfo: {
        currentRetry: this.retryCount,
        maxRetries: this.maxRetryAttempts,
      },
    };
  }

  /**
   * 手动触发重试（用于用户手动重试）
   */
  async manualRetry() {
    if (this.connectionState === "connected") {
      this.logger.warn("WebRTC", "连接已建立，无需重试");
      return false;
    }

    if (this.connectionState === "connecting") {
      this.logger.warn("WebRTC", "正在连接中，请等待");
      return false;
    }

    this.logger.info("WebRTC", "用户手动触发重试", {
      currentState: this.connectionState,
      retryCount: this.retryCount,
    });

    // 重置重试计数，允许手动重试
    const originalRetryCount = this.retryCount;
    this.retryCount = Math.max(0, this.retryCount - 1);

    try {
      await this.connect();
      return true;
    } catch (error) {
      this.retryCount = originalRetryCount;
      this.logger.error("WebRTC", "手动重试失败", error);
      return false;
    }
  }

  /**
   * 发送数据通道消息（增强版）
   * 支持多种消息类型和格式，保持与现有协议的兼容性
   */
  sendDataChannelMessage(message, options = {}) {
    const {
      channel = "send", // 'send' 或 'receive'，指定使用哪个通道
      validateMessage = true, // 是否验证消息格式
      maxRetries = 3, // 最大重试次数
      retryDelay = 100, // 重试延迟（毫秒）
      timeout = 5000, // 发送超时（毫秒）
    } = options;

    // 选择数据通道
    const targetChannel =
      channel === "receive" ? this.receiveChannel : this.sendChannel;

    if (!targetChannel || targetChannel.readyState !== "open") {
      this.logger.warn("WebRTC", "数据通道未就绪", {
        channel: channel,
        channelState: targetChannel?.readyState || "null",
        sendChannelState: this.sendChannel?.readyState || "null",
        receiveChannelState: this.receiveChannel?.readyState || "null",
      });

      // 触发数据通道未就绪事件
      this.eventBus?.emit("webrtc:datachannel-not-ready", {
        channel: channel,
        channelState: targetChannel?.readyState || "null",
        message: message,
      });

      return false;
    }

    // 验证消息格式
    if (validateMessage && !this._validateDataChannelMessage(message)) {
      this.logger.error("WebRTC", "数据通道消息格式无效", { message });
      return false;
    }

    // 准备发送的数据
    let messageData;
    try {
      // 支持多种消息格式
      if (typeof message === "string") {
        messageData = message;
      } else if (typeof message === "object") {
        // 保持与现有协议的兼容性
        messageData = JSON.stringify(message);
      } else {
        messageData = String(message);
      }
    } catch (error) {
      this.logger.error("WebRTC", "消息序列化失败", error);
      return false;
    }

    // 发送消息（带重试机制）
    return this._sendDataChannelMessageWithRetry(
      targetChannel,
      messageData,
      message,
      maxRetries,
      retryDelay,
      timeout
    );
  }

  /**
   * 带重试机制的数据通道消息发送
   * @private
   */
  _sendDataChannelMessageWithRetry(
    channel,
    messageData,
    originalMessage,
    maxRetries,
    retryDelay,
    timeout
  ) {
    let retryCount = 0;

    const attemptSend = () => {
      try {
        // 检查通道状态
        if (channel.readyState !== "open") {
          throw new Error(`数据通道状态不正确: ${channel.readyState}`);
        }

        // 发送消息
        channel.send(messageData);

        // 更新统计信息
        this._updateDataChannelStats(
          "sent",
          messageData.length,
          originalMessage.type || "unknown"
        );

        this.logger.debug("WebRTC", "数据通道消息已发送", {
          message: originalMessage,
          channel: channel.label,
          dataSize: messageData.length,
          retryCount: retryCount,
        });

        // 触发消息发送成功事件
        this.eventBus?.emit("webrtc:datachannel-message-sent", {
          channel: channel.label,
          message: originalMessage,
          dataSize: messageData.length,
          retryCount: retryCount,
          timestamp: Date.now(),
        });

        return true;
      } catch (error) {
        retryCount++;

        this.logger.warn(
          "WebRTC",
          `数据通道消息发送失败 (尝试 ${retryCount}/${maxRetries + 1})`,
          {
            error: error.message,
            channel: channel.label,
            retryCount: retryCount,
          }
        );

        // 如果还有重试机会且通道可能恢复
        if (
          retryCount <= maxRetries &&
          this._shouldRetryDataChannelSend(error)
        ) {
          // 延迟后重试
          setTimeout(() => {
            if (channel.readyState === "open") {
              attemptSend();
            } else {
              this._handleDataChannelSendFailure(
                originalMessage,
                error,
                retryCount
              );
            }
          }, retryDelay * retryCount); // 递增延迟

          return false; // 表示正在重试
        } else {
          // 重试次数用完或不应重试
          this._handleDataChannelSendFailure(
            originalMessage,
            error,
            retryCount
          );
          return false;
        }
      }
    };

    return attemptSend();
  }

  /**
   * 判断是否应该重试数据通道发送
   * @private
   */
  _shouldRetryDataChannelSend(error) {
    const retryableErrors = [
      "InvalidStateError",
      "NetworkError",
      "OperationError",
    ];

    return retryableErrors.some(
      (errorType) =>
        error.name === errorType || error.message.includes(errorType)
    );
  }

  /**
   * 处理数据通道发送失败
   * @private
   */
  _handleDataChannelSendFailure(message, error, retryCount) {
    this.logger.error("WebRTC", "数据通道消息发送最终失败", {
      message: message,
      error: error.message,
      retryCount: retryCount,
    });

    // 更新错误统计
    this._updateDataChannelStats("failed", 0, message.type || "unknown");

    // 触发发送失败事件
    this.eventBus?.emit("webrtc:datachannel-message-failed", {
      message: message,
      error: error,
      retryCount: retryCount,
      timestamp: Date.now(),
    });
  }

  /**
   * 验证数据通道消息格式（简化版）
   * @private
   */
  _validateDataChannelMessage(message) {
    if (message === null || message === undefined) return false;
    
    // 简化验证：只检查基本类型和大小
    if (typeof message === "string") {
      return message.length > 0 && message.length <= 32768; // 减少到32KB
    }
    
    if (typeof message === "object") {
      try {
        const serialized = JSON.stringify(message);
        return serialized.length <= 32768; // 减少到32KB
      } catch {
        return false;
      }
    }
    
    return String(message).length <= 32768;
  }

  /**
   * 更新数据通道统计信息
   * @private
   */
  _updateDataChannelStats(operation, dataSize, messageType) {
    if (!this.connectionStats.dataChannelStats) {
      this.connectionStats.dataChannelStats = {
        messagesSent: 0,
        messagesReceived: 0,
        messagesFailed: 0,
        bytesSent: 0,
        bytesReceived: 0,
        messageTypes: {},
        lastActivity: null,
      };
    }

    const stats = this.connectionStats.dataChannelStats;
    const now = Date.now();

    switch (operation) {
      case "sent":
        stats.messagesSent++;
        stats.bytesSent += dataSize;
        break;
      case "received":
        stats.messagesReceived++;
        stats.bytesReceived += dataSize;
        break;
      case "failed":
        stats.messagesFailed++;
        break;
    }

    // 更新消息类型统计
    if (messageType) {
      if (!stats.messageTypes[messageType]) {
        stats.messageTypes[messageType] = { sent: 0, received: 0, failed: 0 };
      }
      stats.messageTypes[messageType][operation]++;
    }

    stats.lastActivity = now;
  }

  /**
   * 发送 SDP Offer 到信令服务器
   * @private
   */
  _sendOffer(offer) {
    if (!this.signalingManager) {
      throw new Error("信令管理器未设置");
    }

    this.logger.info("WebRTC", "发送 SDP Offer", {
      type: offer.type,
      sdpLength: offer.sdp.length,
    });

    // 发送 offer 消息，保持与服务器端协议兼容
    return this.signalingManager.send("offer", {
      type: offer.type,
      sdp: offer.sdp,
    });
  }

  /**
   * 发送 SDP Answer 到信令服务器
   * @private
   */
  _sendAnswer(answer) {
    if (!this.signalingManager) {
      throw new Error("信令管理器未设置");
    }

    this.logger.info("WebRTC", "发送 SDP Answer", {
      type: answer.type,
      sdpLength: answer.sdp.length,
    });

    // 发送 answer 消息，保持与服务器端协议兼容
    return this.signalingManager.send("answer", {
      type: answer.type,
      sdp: answer.sdp,
    });
  }

  /**
   * 发送 ICE 候选到信令服务器
   * @private
   */
  _sendIceCandidate(candidate) {
    if (!this.signalingManager) {
      throw new Error("信令管理器未设置");
    }

    this.logger.debug("WebRTC", "发送 ICE 候选", {
      candidate: candidate.candidate,
      sdpMLineIndex: candidate.sdpMLineIndex,
      sdpMid: candidate.sdpMid,
    });

    // 发送 ICE 候选消息，保持与服务器端协议兼容
    return this.signalingManager.send("ice-candidate", {
      candidate: candidate.candidate,
      sdpMLineIndex: candidate.sdpMLineIndex,
      sdpMid: candidate.sdpMid,
    });
  }

  /**
   * 请求服务器创建 Offer
   * @private
   */
  _requestOffer() {
    if (!this.signalingManager) {
      throw new Error("信令管理器未设置");
    }

    this.logger.info("WebRTC", "请求服务器创建 Offer");

    // 发送 request-offer 消息
    return this.signalingManager.send("request-offer", {
      timestamp: Date.now(),
    });
  }

  /**
   * 设置连接状态（优化版）
   * @private
   */
  _setState(newState) {
    if (this.connectionState === newState) return; // 避免重复设置
    
    const previousState = this.connectionState;
    this.connectionState = newState;

    this.logger.logStateChange("WebRTC", previousState, newState, {
      timestamp: Date.now(),
    });

    // 优化：只在状态真正改变时触发事件
    this.eventBus?.emit("webrtc:state-change", {
      previousState,
      newState,
      timestamp: Date.now(),
    });
  }

  /**
   * 获取公共配置信息
   * @private
   */
  _getPublicConfig() {
    return {
      connectionTimeout: this.connectionTimeout,
      maxRetryAttempts: this.maxRetryAttempts,
      retryDelay: this.retryDelay,
      iceServersCount: this.iceServers.length,
    };
  }

  /**
   * 格式化持续时间
   * @private
   */
  _formatDuration(milliseconds) {
    if (milliseconds < 1000) {
      return `${milliseconds}ms`;
    }

    const seconds = Math.floor(milliseconds / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);

    if (hours > 0) {
      return `${hours}h ${minutes % 60}m ${seconds % 60}s`;
    } else if (minutes > 0) {
      return `${minutes}m ${seconds % 60}s`;
    } else {
      return `${seconds}s`;
    }
  }

  /**
   * 格式化字节数
   * @private
   */
  _formatBytes(bytes) {
    if (bytes === 0) return "0 B";

    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));

    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
  }

  /**
   * 格式化比特率
   * @private
   */
  _formatBitrate(bps) {
    if (bps === 0) return "0 bps";

    const k = 1000;
    const sizes = ["bps", "Kbps", "Mbps", "Gbps"];
    const i = Math.floor(Math.log(bps) / Math.log(k));

    return parseFloat((bps / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
  }

  /**
   * 获取连接类型
   * @private
   */
  _getConnectionType() {
    const local = this.connectionStats.networkStats.localCandidateType;
    const remote = this.connectionStats.networkStats.remoteCandidateType;
    const transport = this.connectionStats.networkStats.transportType;

    if (local === "relay" || remote === "relay") {
      return `TURN (${transport})`;
    } else if (local === "srflx" || remote === "srflx") {
      return `STUN (${transport})`;
    } else if (local === "host" && remote === "host") {
      return `Direct (${transport})`;
    } else {
      return `${local}-${remote} (${transport})`;
    }
  }

  /**
   * 设置信令事件处理
   * @private
   */
  _setupSignalingEvents() {
    if (!this.signalingManager || !this.eventBus) {
      return;
    }

    // 信令连接建立
    this.eventBus.on("signaling:connected", () => {
      this.logger.info("WebRTC", "信令连接已建立");
    });

    // 信令断开
    this.eventBus.on("signaling:disconnected", () => {
      this.logger.warn("WebRTC", "信令连接断开");
      this._handleSignalingDisconnected();
    });

    // 接收 SDP 消息（统一处理 offer 和 answer）
    this.eventBus.on("signaling:sdp", async (sdp) => {
      try {
        if (sdp.type === "offer") {
          await this._handleOffer(sdp);
        } else if (sdp.type === "answer") {
          await this._handleAnswer(sdp);
        } else {
          this.logger.warn("WebRTC", "未知的 SDP 类型", { type: sdp.type });
        }
      } catch (error) {
        this.logger.error("WebRTC", "处理 SDP 失败", error);
        this._handleConnectionError(error);
      }
    });

    // 接收 ICE 候选
    this.eventBus.on("signaling:ice-candidate", async (candidate) => {
      try {
        await this._handleIceCandidate(candidate);
      } catch (error) {
        this.logger.error("WebRTC", "处理 ICE 候选失败", error);
      }
    });

    // 处理信令错误
    this.eventBus.on("signaling:error", (errorData) => {
      this.logger.error("WebRTC", "信令错误", errorData);
      this._handleConnectionError(new Error(errorData.message || "信令错误"));
    });

    // 处理服务器错误
    this.eventBus.on("signaling:server-error", (errorData) => {
      this.logger.error("WebRTC", "服务器错误", errorData);

      const serverError = new Error(errorData.message || "服务器错误");
      serverError.code = "SERVER_ERROR";
      serverError.details = errorData;

      if (errorData.critical) {
        this._handleConnectionError(serverError, {
          isServerError: true,
          isCritical: true,
        });
      } else {
        // 非关键服务器错误，记录但不中断连接
        this._recordError(
          this._classifyError(serverError, {
            isServerError: true,
            isCritical: false,
          })
        );
      }
    });

    // 处理欢迎消息
    this.eventBus.on("signaling:welcome", (data) => {
      this.logger.info("WebRTC", "收到服务器欢迎消息", data);
      // 可以在这里触发自动连接或其他初始化逻辑
    });

    this.logger.info("WebRTC", "信令事件处理已设置");
  }

  /**
   * 处理信令断开（增强版）
   * @private
   */
  _handleSignalingDisconnected() {
    this.logger.warn("WebRTC", "信令连接断开", {
      connectionEstablished: this.connectionEstablished,
      retryCount: this.retryCount,
      maxRetries: this.maxRetryAttempts,
      iceConnectionState: this.iceConnectionState,
    });

    // 创建信令断开错误
    const signalingError = new Error("信令服务器连接断开");
    signalingError.code = "SIGNALING_DISCONNECTED";
    signalingError.details = {
      connectionEstablished: this.connectionEstablished,
      iceConnectionState: this.iceConnectionState,
    };

    // 如果连接已建立，这是网络中断，应该重试
    if (this.connectionEstablished) {
      this._handleConnectionError(signalingError, {
        isSignalingDisconnect: true,
        shouldRetry: true,
      });
    } else {
      // 如果连接未建立，这可能是初始连接失败
      this._handleConnectionError(signalingError, {
        isSignalingDisconnect: true,
        shouldRetry: this.retryCount < this.maxRetryAttempts,
      });
    }
  }

  /**
   * 分类错误类型和严重级别
   * @private
   */
  _classifyError(error, context = {}) {
    let errorType = this.ERROR_TYPES.CONNECTION;
    let errorLevel = this.ERROR_LEVELS.ERROR;
    let shouldRetry = false;
    let userMessage = "连接出现问题";

    // 根据错误消息和上下文分类错误
    const errorMessage = error.message?.toLowerCase() || "";

    if (errorMessage.includes("timeout") || errorMessage.includes("超时")) {
      errorType = this.ERROR_TYPES.TIMEOUT;
      errorLevel = this.ERROR_LEVELS.ERROR;
      shouldRetry = this.connectionEstablished;
      userMessage = "连接超时，正在重试";
    } else if (
      errorMessage.includes("ice") ||
      errorMessage.includes("connection failed")
    ) {
      errorType = this.ERROR_TYPES.CONNECTION;
      errorLevel = this.ERROR_LEVELS.ERROR;
      shouldRetry = this.connectionEstablished;
      userMessage = "网络连接失败";
    } else if (
      errorMessage.includes("signaling") ||
      errorMessage.includes("信令")
    ) {
      errorType = this.ERROR_TYPES.SIGNALING;
      errorLevel = this.ERROR_LEVELS.ERROR;
      shouldRetry = true;
      userMessage = "信令服务器连接问题";
    } else if (
      errorMessage.includes("media") ||
      errorMessage.includes("stream")
    ) {
      errorType = this.ERROR_TYPES.MEDIA;
      errorLevel = this.ERROR_LEVELS.WARNING;
      shouldRetry = false;
      userMessage = "媒体流处理问题";
    } else if (
      errorMessage.includes("config") ||
      errorMessage.includes("ice server") ||
      errorMessage.includes("invalid")
    ) {
      errorType = this.ERROR_TYPES.CONFIG;
      errorLevel = this.ERROR_LEVELS.FATAL;
      shouldRetry = false;
      userMessage = "配置错误，请检查设置";
    } else if (
      errorMessage.includes("network") ||
      errorMessage.includes("网络")
    ) {
      errorType = this.ERROR_TYPES.NETWORK;
      errorLevel = this.ERROR_LEVELS.ERROR;
      shouldRetry = this.connectionEstablished;
      userMessage = "网络中断，正在重连";
    }

    // 基于上下文调整分类
    if (context.iceConnectionState === "failed") {
      errorType = this.ERROR_TYPES.CONNECTION;
      shouldRetry = this.connectionEstablished;
    }

    if (context.signalingState === "closed" && this.connectionEstablished) {
      errorType = this.ERROR_TYPES.NETWORK;
      shouldRetry = true;
    }

    return {
      type: errorType,
      level: errorLevel,
      shouldRetry: shouldRetry && this.retryCount < this.maxRetryAttempts,
      userMessage,
      originalError: error,
      context,
      timestamp: Date.now(),
    };
  }

  /**
   * 记录错误到历史记录（优化版）
   * @private
   */
  _recordError(classifiedError) {
    this.lastError = classifiedError;
    this.errorHistory.push(classifiedError);

    // 优化：保持更小的错误历史记录，减少内存使用
    if (this.errorHistory.length > 5) {
      this.errorHistory.shift();
    }

    // 根据错误级别记录日志
    switch (classifiedError.level) {
      case this.ERROR_LEVELS.WARNING:
        this.logger.warn("WebRTC", `${classifiedError.type}错误`, {
          message: classifiedError.userMessage,
          error: classifiedError.originalError,
          context: classifiedError.context,
        });
        break;
      case this.ERROR_LEVELS.ERROR:
        this.logger.error("WebRTC", `${classifiedError.type}错误`, {
          message: classifiedError.userMessage,
          error: classifiedError.originalError,
          context: classifiedError.context,
        });
        break;
      case this.ERROR_LEVELS.FATAL:
        this.logger.error("WebRTC", `致命${classifiedError.type}错误`, {
          message: classifiedError.userMessage,
          error: classifiedError.originalError,
          context: classifiedError.context,
        });
        break;
    }
  }

  /**
   * 处理连接错误（增强版）
   * @private
   */
  _handleConnectionError(error, context = {}) {
    // 分类错误
    const classifiedError = this._classifyError(error, {
      ...context,
      iceConnectionState: this.iceConnectionState,
      signalingState: this.signalingState,
      connectionState: this.connectionState,
      retryCount: this.retryCount,
    });

    // 记录错误
    this._recordError(classifiedError);

    // 触发错误事件
    this.eventBus?.emit("webrtc:error", {
      ...classifiedError,
      retryCount: this.retryCount,
      maxRetries: this.maxRetryAttempts,
    });

    // 根据错误分类决定处理策略
    if (classifiedError.level === this.ERROR_LEVELS.FATAL) {
      // 致命错误，不重试
      this._setState("failed");
      this.eventBus?.emit("webrtc:connection-failed", {
        error: classifiedError,
        retryCount: this.retryCount,
        finalFailure: true,
        reason: "fatal_error",
      });
    } else if (classifiedError.shouldRetry) {
      // 可重试错误
      this.logger.info("WebRTC", "错误可重试，安排重试", {
        errorType: classifiedError.type,
        retryCount: this.retryCount,
        maxRetries: this.maxRetryAttempts,
      });
      this._scheduleRetry(classifiedError);
    } else {
      // 不可重试错误或已达到最大重试次数
      this._setState("failed");
      this.eventBus?.emit("webrtc:connection-failed", {
        error: classifiedError,
        retryCount: this.retryCount,
        finalFailure: true,
        reason: classifiedError.shouldRetry
          ? "max_retries_exceeded"
          : "non_retryable_error",
      });
    }
  }

  /**
   * 安排重试（增强版）
   * @private
   */
  _scheduleRetry(classifiedError = null) {
    if (this.retryTimer) {
      clearTimeout(this.retryTimer);
    }

    this.retryCount++;
    this.connectionStats.reconnectAttempts++;

    // 根据错误类型调整重试延迟
    let baseDelay = this.retryDelay;
    if (classifiedError) {
      switch (classifiedError.type) {
        case this.ERROR_TYPES.NETWORK:
          // 网络错误使用较短延迟
          baseDelay = Math.min(this.retryDelay, 1000);
          break;
        case this.ERROR_TYPES.SIGNALING:
          // 信令错误使用中等延迟
          baseDelay = this.retryDelay;
          break;
        case this.ERROR_TYPES.CONNECTION:
          // 连接错误使用较长延迟
          baseDelay = this.retryDelay * 1.5;
          break;
        case this.ERROR_TYPES.TIMEOUT:
          // 超时错误使用更长延迟
          baseDelay = this.retryDelay * 2;
          break;
        default:
          baseDelay = this.retryDelay;
      }
    }

    // 应用指数退避，但设置最大延迟限制
    const exponentialDelay = baseDelay * Math.pow(2, this.retryCount - 1);
    const maxDelay = 30000; // 最大30秒
    const delay = Math.min(exponentialDelay, maxDelay);

    this.logger.info("WebRTC", "安排重试连接", {
      retryCount: this.retryCount,
      delay: delay,
      maxRetries: this.maxRetryAttempts,
      errorType: classifiedError?.type || "unknown",
      baseDelay: baseDelay,
    });

    this.retryTimer = setTimeout(() => {
      this._attemptReconnect(classifiedError);
    }, delay);

    this.eventBus?.emit("webrtc:retry-scheduled", {
      retryCount: this.retryCount,
      delay: delay,
      errorType: classifiedError?.type,
      errorLevel: classifiedError?.level,
    });
  }

  /**
   * 尝试重新连接（增强版）
   * @private
   */
  async _attemptReconnect(previousError = null) {
    try {
      this.logger.info("WebRTC", "尝试重新连接", {
        attempt: this.retryCount,
        maxRetries: this.maxRetryAttempts,
        previousErrorType: previousError?.type || "unknown",
      });

      // 触发重连开始事件
      this.eventBus?.emit("webrtc:reconnect-attempt", {
        attempt: this.retryCount,
        previousError: previousError,
      });

      this._setState("connecting");

      // 根据之前的错误类型采取不同的重连策略
      if (previousError?.type === this.ERROR_TYPES.SIGNALING) {
        // 信令错误：等待信令重新连接
        if (!this.signalingManager || !this.signalingManager.isConnected()) {
          this.logger.info("WebRTC", "等待信令服务器重新连接");
          // 等待信令连接恢复，或者超时后继续
          await this._waitForSignalingConnection(5000);
        }
      }

      // 执行重连
      await this.connect();

      this.logger.info("WebRTC", "重新连接成功", {
        attempt: this.retryCount,
        totalReconnects: this.connectionStats.reconnectAttempts,
      });
    } catch (error) {
      this.logger.error("WebRTC", "重新连接失败", {
        attempt: this.retryCount,
        error: error.message,
      });

      // 递归处理重连失败
      this._handleConnectionError(error, {
        isReconnectAttempt: true,
        previousError: previousError,
      });
    }
  }

  /**
   * 等待信令连接恢复
   * @private
   */
  async _waitForSignalingConnection(timeout = 5000) {
    return new Promise((resolve) => {
      const startTime = Date.now();

      const checkConnection = () => {
        if (this.signalingManager && this.signalingManager.isConnected()) {
          this.logger.info("WebRTC", "信令连接已恢复");
          resolve(true);
        } else if (Date.now() - startTime >= timeout) {
          this.logger.warn("WebRTC", "等待信令连接超时");
          resolve(false);
        } else {
          setTimeout(checkConnection, 100);
        }
      };

      checkConnection();
    });
  }

  /**
   * 创建 PeerConnection
   * 使用 webrtc-adapter 标准化的 API
   * @private
   */
  _createPeerConnection() {
    try {
      PerformanceMonitor.startTimer('createPeerConnection');
      
      this.logger.info("WebRTC", "创建 PeerConnection", {
        iceServersCount: this.iceServers.length,
      });

      // 优化的 PeerConnection 配置
      const pcConfig = {
        iceServers: this.iceServers,
        iceCandidatePoolSize: 4, // 减少候选池大小
        bundlePolicy: "max-bundle",
        rtcpMuxPolicy: "require",
        iceTransportPolicy: "all", // 允许所有传输类型
      };

      this.pc = new RTCPeerConnection(pcConfig);

      // 设置基本的 PeerConnection 事件处理
      this._setupPeerConnectionEvents();

      // 优化连接建立
      this._optimizeConnectionEstablishment();

      const duration = PerformanceMonitor.endTimer('createPeerConnection');
      PerformanceMonitor.logTiming('PeerConnection创建', duration);

      this.logger.info("WebRTC", "PeerConnection 创建成功", {
        signalingState: this.pc.signalingState,
        iceConnectionState: this.pc.iceConnectionState,
        creationTime: `${duration.toFixed(2)}ms`
      });

      return this.pc;
    } catch (error) {
      this.logger.error("WebRTC", "创建 PeerConnection 失败", error);
      throw error;
    }
  }

  /**
   * 设置 PeerConnection 事件处理
   * @private
   */
  _setupPeerConnectionEvents() {
    if (!this.pc) {
      throw new Error("PeerConnection not created");
    }

    // ICE 候选事件处理
    this.pc.onicecandidate = (event) => {
      this.logger.debug("WebRTC", "ICE 候选生成", {
        candidate: event.candidate?.candidate || null,
        sdpMLineIndex: event.candidate?.sdpMLineIndex || null,
      });

      if (event.candidate && this.signalingManager) {
        // 发送 ICE 候选到信令服务器
        this._sendIceCandidate(event.candidate);
      } else if (!event.candidate) {
        this.logger.info("WebRTC", "ICE 候选收集完成");
      }
    };

    // 远程媒体流事件处理 - 增强版本
    this.pc.ontrack = (event) => {
      this.logger.info("WebRTC", "接收到远程媒体轨道", {
        streamId: event.streams[0]?.id || "unknown",
        trackKind: event.track?.kind || "unknown",
        trackId: event.track?.id || "unknown",
        trackLabel: event.track?.label || "unknown",
        trackEnabled: event.track?.enabled,
        trackMuted: event.track?.muted,
        trackReadyState: event.track?.readyState,
      });

      // 设置轨道事件监听器
      this._setupTrackEvents(event.track);

      // 处理流
      if (event.streams && event.streams[0]) {
        const stream = event.streams[0];

        // 如果是新的流或流发生变化，重新处理
        if (!this.remoteStream || this.remoteStream.id !== stream.id) {
          this.logger.info("WebRTC", "检测到新的远程媒体流", {
            oldStreamId: this.remoteStream?.id || "none",
            newStreamId: stream.id,
          });

          this._handleRemoteStream(stream);
        } else {
          // 同一个流，但可能添加了新轨道
          this.logger.info("WebRTC", "现有流添加了新轨道", {
            streamId: stream.id,
            trackKind: event.track.kind,
          });

          // 重新应用流到媒体元素以包含新轨道
          this._applyRemoteStreamToElements(stream);
        }
      } else {
        this.logger.warn("WebRTC", "接收到轨道但没有关联的流");
      }

      // 触发轨道接收事件
      this.eventBus?.emit("webrtc:track-received", {
        track: event.track,
        streams: event.streams,
        timestamp: Date.now(),
      });
    };

    // 数据通道事件处理
    this.pc.ondatachannel = (event) => {
      this.logger.info("WebRTC", "接收到数据通道", {
        label: event.channel.label,
        readyState: event.channel.readyState,
      });

      this.receiveChannel = event.channel;
      this._setupDataChannelEvents(this.receiveChannel);
    };

    // ICE 连接状态变化
    this.pc.oniceconnectionstatechange = () => {
      const newState = this.pc.iceConnectionState;
      this.logger.info("WebRTC", "ICE 连接状态变化", {
        previousState: this.iceConnectionState,
        newState: newState,
      });

      this.iceConnectionState = newState;
      this._handleIceConnectionStateChange(newState);
    };

    // 信令状态变化
    this.pc.onsignalingstatechange = () => {
      const newState = this.pc.signalingState;
      this.logger.info("WebRTC", "信令状态变化", {
        previousState: this.signalingState,
        newState: newState,
      });

      this.signalingState = newState;
      this._handleSignalingStateChange(newState);
    };

    // 连接状态变化（现代浏览器支持）
    if (this.pc.onconnectionstatechange !== undefined) {
      this.pc.onconnectionstatechange = () => {
        const newState = this.pc.connectionState;
        this.logger.info("WebRTC", "连接状态变化", {
          connectionState: newState,
          iceConnectionState: this.pc.iceConnectionState,
          signalingState: this.pc.signalingState,
        });

        this._handleConnectionStateChange(newState);
      };
    }

    this.logger.info("WebRTC", "PeerConnection 事件处理已设置");
  }

  /**
   * 处理远程媒体流
   * 增强版本，支持更好的错误处理和跨浏览器兼容性
   * @private
   */
  _handleRemoteStream(stream) {
    try {
      this.logger.info("WebRTC", "开始处理远程媒体流", {
        streamId: stream.id,
        videoTracks: stream.getVideoTracks().length,
        audioTracks: stream.getAudioTracks().length,
        totalTracks: stream.getTracks().length,
      });

      // 存储远程流引用
      this.remoteStream = stream;

      // 设置流事件监听器
      this._setupRemoteStreamEvents(stream);

      // 应用流到媒体元素
      this._applyRemoteStreamToElements(stream);

      // 触发媒体流接收事件
      this.eventBus?.emit("webrtc:remote-stream", {
        stream: stream,
        streamId: stream.id,
        videoTracks: stream.getVideoTracks().length,
        audioTracks: stream.getAudioTracks().length,
        timestamp: Date.now(),
      });

      this.logger.info("WebRTC", "远程媒体流处理完成");
    } catch (error) {
      this.logger.error("WebRTC", "处理远程媒体流失败", error);
      this._handleMediaError("REMOTE_STREAM_PROCESSING_FAILED", error);
    }
  }

  /**
   * 应用远程流到媒体元素
   * @private
   */
  _applyRemoteStreamToElements(stream) {
    try {
      const videoTracks = stream.getVideoTracks();
      const audioTracks = stream.getAudioTracks();

      // 处理视频流
      if (this.videoElement && videoTracks.length > 0) {
        this._setVideoStream(stream, videoTracks);
      }

      // 处理音频流
      if (this.audioElement && audioTracks.length > 0) {
        this._setAudioStream(stream, audioTracks);
      }

      // 如果没有对应的媒体元素，记录警告
      if (videoTracks.length > 0 && !this.videoElement) {
        this.logger.warn("WebRTC", "接收到视频流但未设置视频元素");
      }

      if (audioTracks.length > 0 && !this.audioElement) {
        this.logger.warn("WebRTC", "接收到音频流但未设置音频元素");
      }
    } catch (error) {
      this.logger.error("WebRTC", "应用远程流到媒体元素失败", error);
      this._handleMediaError("STREAM_APPLICATION_FAILED", error);
    }
  }

  /**
   * 设置视频流到视频元素
   * @private
   */
  _setVideoStream(stream, videoTracks) {
    try {
      // 跨浏览器兼容性处理
      if (this.videoElement.srcObject !== undefined) {
        // 现代浏览器
        this.videoElement.srcObject = stream;
      } else {
        // 旧版浏览器兼容性
        this.videoElement.src = window.URL.createObjectURL(stream);
      }

      // 设置视频元素属性以确保跨浏览器兼容性
      this.videoElement.autoplay = true;
      this.videoElement.playsInline = true; // iOS Safari 兼容性
      this.videoElement.muted = true; // 避免自动播放策略问题

      // 监听视频元素事件
      this._setupVideoElementEvents();

      this.logger.info("WebRTC", "视频流已设置到视频元素", {
        videoTracks: videoTracks.length,
        trackIds: videoTracks.map((track) => track.id),
        videoElementReady: this.videoElement.readyState,
      });

      // 触发视频流设置事件
      this.eventBus?.emit("webrtc:video-stream-set", {
        videoTracks: videoTracks.length,
        trackIds: videoTracks.map((track) => track.id),
        timestamp: Date.now(),
      });
    } catch (error) {
      this.logger.error("WebRTC", "设置视频流失败", error);
      this._handleMediaError("VIDEO_STREAM_SETUP_FAILED", error);
    }
  }

  /**
   * 设置音频流到音频元素
   * @private
   */
  _setAudioStream(stream, audioTracks) {
    try {
      // 跨浏览器兼容性处理
      if (this.audioElement.srcObject !== undefined) {
        // 现代浏览器
        this.audioElement.srcObject = stream;
      } else {
        // 旧版浏览器兼容性
        this.audioElement.src = window.URL.createObjectURL(stream);
      }

      // 设置音频元素属性
      this.audioElement.autoplay = true;

      // 监听音频元素事件
      this._setupAudioElementEvents();

      this.logger.info("WebRTC", "音频流已设置到音频元素", {
        audioTracks: audioTracks.length,
        trackIds: audioTracks.map((track) => track.id),
        audioElementReady: this.audioElement.readyState,
      });

      // 触发音频流设置事件
      this.eventBus?.emit("webrtc:audio-stream-set", {
        audioTracks: audioTracks.length,
        trackIds: audioTracks.map((track) => track.id),
        timestamp: Date.now(),
      });
    } catch (error) {
      this.logger.error("WebRTC", "设置音频流失败", error);
      this._handleMediaError("AUDIO_STREAM_SETUP_FAILED", error);
    }
  }

  /**
   * 设置媒体元素事件监听器
   * @private
   */
  _setupMediaElementEvents() {
    // 设置视频元素事件
    if (this.videoElement) {
      this._setupVideoElementEvents();
    }

    // 设置音频元素事件
    if (this.audioElement) {
      this._setupAudioElementEvents();
    }
  }

  /**
   * 设置视频元素事件监听器
   * @private
   */
  _setupVideoElementEvents() {
    if (!this.videoElement) return;

    // 清理之前的事件监听器
    this._cleanupVideoElementEvents();

    // 视频加载开始
    this.videoElement.addEventListener(
      "loadstart",
      this._onVideoLoadStart.bind(this)
    );

    // 视频元数据加载完成
    this.videoElement.addEventListener(
      "loadedmetadata",
      this._onVideoLoadedMetadata.bind(this)
    );

    // 视频可以开始播放
    this.videoElement.addEventListener(
      "canplay",
      this._onVideoCanPlay.bind(this)
    );

    // 视频开始播放
    this.videoElement.addEventListener("play", this._onVideoPlay.bind(this));

    // 视频暂停
    this.videoElement.addEventListener("pause", this._onVideoPause.bind(this));

    // 视频播放结束
    this.videoElement.addEventListener("ended", this._onVideoEnded.bind(this));

    // 视频错误
    this.videoElement.addEventListener("error", this._onVideoError.bind(this));

    // 视频尺寸变化
    this.videoElement.addEventListener(
      "resize",
      this._onVideoResize.bind(this)
    );

    this.logger.debug("WebRTC", "视频元素事件监听器已设置");
  }

  /**
   * 设置音频元素事件监听器
   * @private
   */
  _setupAudioElementEvents() {
    if (!this.audioElement) return;

    // 清理之前的事件监听器
    this._cleanupAudioElementEvents();

    // 音频加载开始
    this.audioElement.addEventListener(
      "loadstart",
      this._onAudioLoadStart.bind(this)
    );

    // 音频元数据加载完成
    this.audioElement.addEventListener(
      "loadedmetadata",
      this._onAudioLoadedMetadata.bind(this)
    );

    // 音频可以开始播放
    this.audioElement.addEventListener(
      "canplay",
      this._onAudioCanPlay.bind(this)
    );

    // 音频开始播放
    this.audioElement.addEventListener("play", this._onAudioPlay.bind(this));

    // 音频暂停
    this.audioElement.addEventListener("pause", this._onAudioPause.bind(this));

    // 音频播放结束
    this.audioElement.addEventListener("ended", this._onAudioEnded.bind(this));

    // 音频错误
    this.audioElement.addEventListener("error", this._onAudioError.bind(this));

    this.logger.debug("WebRTC", "音频元素事件监听器已设置");
  }

  /**
   * 设置远程流事件监听器
   * @private
   */
  _setupRemoteStreamEvents(stream) {
    if (!stream) return;

    // 监听轨道添加事件
    stream.addEventListener("addtrack", (event) => {
      this.logger.info("WebRTC", "远程流添加了新轨道", {
        trackKind: event.track.kind,
        trackId: event.track.id,
        trackLabel: event.track.label,
      });

      this.eventBus?.emit("webrtc:remote-track-added", {
        track: event.track,
        stream: stream,
        timestamp: Date.now(),
      });

      // 如果是新的视频或音频轨道，重新应用到媒体元素
      if (event.track.kind === "video" && this.videoElement) {
        this._setVideoStream(stream, stream.getVideoTracks());
      } else if (event.track.kind === "audio" && this.audioElement) {
        this._setAudioStream(stream, stream.getAudioTracks());
      }
    });

    // 监听轨道移除事件
    stream.addEventListener("removetrack", (event) => {
      this.logger.info("WebRTC", "远程流移除了轨道", {
        trackKind: event.track.kind,
        trackId: event.track.id,
      });

      this.eventBus?.emit("webrtc:remote-track-removed", {
        track: event.track,
        stream: stream,
        timestamp: Date.now(),
      });
    });

    this.logger.debug("WebRTC", "远程流事件监听器已设置");
  }

  /**
   * 清理媒体元素事件监听器
   * @private
   */
  _cleanupMediaElementEvents() {
    this._cleanupVideoElementEvents();
    this._cleanupAudioElementEvents();
  }

  /**
   * 清理视频元素事件监听器
   * @private
   */
  _cleanupVideoElementEvents() {
    if (!this.videoElement) return;

    const events = [
      "loadstart",
      "loadedmetadata",
      "canplay",
      "play",
      "pause",
      "ended",
      "error",
      "resize",
    ];
    events.forEach((event) => {
      this.videoElement.removeEventListener(
        event,
        this[`_onVideo${event.charAt(0).toUpperCase() + event.slice(1)}`]
      );
    });
  }

  /**
   * 清理音频元素事件监听器
   * @private
   */
  _cleanupAudioElementEvents() {
    if (!this.audioElement) return;

    const events = [
      "loadstart",
      "loadedmetadata",
      "canplay",
      "play",
      "pause",
      "ended",
      "error",
    ];
    events.forEach((event) => {
      this.audioElement.removeEventListener(
        event,
        this[`_onAudio${event.charAt(0).toUpperCase() + event.slice(1)}`]
      );
    });
  }

  // 视频元素事件处理器
  _onVideoLoadStart(event) {
    this.logger.debug("WebRTC", "视频开始加载");
    this.eventBus?.emit("webrtc:video-load-start", { timestamp: Date.now() });
  }

  _onVideoLoadedMetadata(event) {
    this.logger.info("WebRTC", "视频元数据已加载", {
      videoWidth: this.videoElement.videoWidth,
      videoHeight: this.videoElement.videoHeight,
      duration: this.videoElement.duration,
    });

    this.eventBus?.emit("webrtc:video-metadata-loaded", {
      width: this.videoElement.videoWidth,
      height: this.videoElement.videoHeight,
      duration: this.videoElement.duration,
      timestamp: Date.now(),
    });
  }

  _onVideoCanPlay(event) {
    this.logger.debug("WebRTC", "视频可以开始播放");
    this.eventBus?.emit("webrtc:video-can-play", { timestamp: Date.now() });
  }

  _onVideoPlay(event) {
    this.logger.info("WebRTC", "视频开始播放");
    this.eventBus?.emit("webrtc:video-play", { timestamp: Date.now() });
  }

  _onVideoPause(event) {
    this.logger.info("WebRTC", "视频暂停");
    this.eventBus?.emit("webrtc:video-pause", { timestamp: Date.now() });
  }

  _onVideoEnded(event) {
    this.logger.info("WebRTC", "视频播放结束");
    this.eventBus?.emit("webrtc:video-ended", { timestamp: Date.now() });
  }

  _onVideoError(event) {
    const error = this.videoElement.error;
    this.logger.error("WebRTC", "视频播放错误", {
      code: error?.code,
      message: error?.message,
      networkState: this.videoElement.networkState,
      readyState: this.videoElement.readyState,
    });

    this._handleMediaError("VIDEO_PLAYBACK_ERROR", error);
  }

  _onVideoResize(event) {
    this.logger.debug("WebRTC", "视频尺寸变化", {
      videoWidth: this.videoElement.videoWidth,
      videoHeight: this.videoElement.videoHeight,
    });

    this.eventBus?.emit("webrtc:video-resize", {
      width: this.videoElement.videoWidth,
      height: this.videoElement.videoHeight,
      timestamp: Date.now(),
    });
  }

  // 音频元素事件处理器
  _onAudioLoadStart(event) {
    this.logger.debug("WebRTC", "音频开始加载");
    this.eventBus?.emit("webrtc:audio-load-start", { timestamp: Date.now() });
  }

  _onAudioLoadedMetadata(event) {
    this.logger.info("WebRTC", "音频元数据已加载", {
      duration: this.audioElement.duration,
    });

    this.eventBus?.emit("webrtc:audio-metadata-loaded", {
      duration: this.audioElement.duration,
      timestamp: Date.now(),
    });
  }

  _onAudioCanPlay(event) {
    this.logger.debug("WebRTC", "音频可以开始播放");
    this.eventBus?.emit("webrtc:audio-can-play", { timestamp: Date.now() });
  }

  _onAudioPlay(event) {
    this.logger.info("WebRTC", "音频开始播放");
    this.eventBus?.emit("webrtc:audio-play", { timestamp: Date.now() });
  }

  _onAudioPause(event) {
    this.logger.info("WebRTC", "音频暂停");
    this.eventBus?.emit("webrtc:audio-pause", { timestamp: Date.now() });
  }

  _onAudioEnded(event) {
    this.logger.info("WebRTC", "音频播放结束");
    this.eventBus?.emit("webrtc:audio-ended", { timestamp: Date.now() });
  }

  _onAudioError(event) {
    const error = this.audioElement.error;
    this.logger.error("WebRTC", "音频播放错误", {
      code: error?.code,
      message: error?.message,
      networkState: this.audioElement.networkState,
      readyState: this.audioElement.readyState,
    });

    this._handleMediaError("AUDIO_PLAYBACK_ERROR", error);
  }

  /**
   * 处理媒体错误
   * @private
   */
  _handleMediaError(errorType, error) {
    const mediaError = {
      type: this.ERROR_TYPES.MEDIA,
      level: this.ERROR_LEVELS.ERROR,
      code: errorType,
      message: error?.message || "媒体处理错误",
      userMessage: this._getMediaErrorUserMessage(errorType, error),
      timestamp: Date.now(),
      shouldRetry: this._shouldRetryMediaError(errorType),
      details: {
        errorType: errorType,
        originalError: error,
        videoElementState: this.videoElement
          ? {
              readyState: this.videoElement.readyState,
              networkState: this.videoElement.networkState,
              error: this.videoElement.error,
            }
          : null,
        audioElementState: this.audioElement
          ? {
              readyState: this.audioElement.readyState,
              networkState: this.audioElement.networkState,
              error: this.audioElement.error,
            }
          : null,
      },
    };

    // 记录错误
    this.lastError = mediaError;
    this.errorHistory.push(mediaError);

    // 限制错误历史记录长度
    if (this.errorHistory.length > 50) {
      this.errorHistory = this.errorHistory.slice(-50);
    }

    // 触发错误事件
    this.eventBus?.emit("webrtc:media-error", mediaError);

    this.logger.error("WebRTC", `媒体错误: ${errorType}`, mediaError);
  }

  /**
   * 获取媒体错误的用户友好消息
   * @private
   */
  _getMediaErrorUserMessage(errorType, error) {
    switch (errorType) {
      case "MEDIA_ELEMENT_SETUP_FAILED":
        return "媒体元素设置失败，请检查浏览器兼容性";
      case "REMOTE_STREAM_PROCESSING_FAILED":
        return "远程媒体流处理失败，可能影响音视频播放";
      case "STREAM_APPLICATION_FAILED":
        return "媒体流应用失败，请尝试刷新页面";
      case "VIDEO_STREAM_SETUP_FAILED":
        return "视频流设置失败，视频可能无法正常显示";
      case "AUDIO_STREAM_SETUP_FAILED":
        return "音频流设置失败，音频可能无法正常播放";
      case "VIDEO_PLAYBACK_ERROR":
        return this._getVideoErrorMessage(error);
      case "AUDIO_PLAYBACK_ERROR":
        return this._getAudioErrorMessage(error);
      default:
        return "媒体处理出现未知错误";
    }
  }

  /**
   * 获取视频错误消息
   * @private
   */
  _getVideoErrorMessage(error) {
    if (!error || !error.code) {
      return "视频播放出现未知错误";
    }

    switch (error.code) {
      case 1: // MEDIA_ERR_ABORTED
        return "视频播放被中止";
      case 2: // MEDIA_ERR_NETWORK
        return "视频网络错误，请检查网络连接";
      case 3: // MEDIA_ERR_DECODE
        return "视频解码错误，可能是格式不支持";
      case 4: // MEDIA_ERR_SRC_NOT_SUPPORTED
        return "视频格式不支持";
      default:
        return `视频播放错误 (代码: ${error.code})`;
    }
  }

  /**
   * 获取音频错误消息
   * @private
   */
  _getAudioErrorMessage(error) {
    if (!error || !error.code) {
      return "音频播放出现未知错误";
    }

    switch (error.code) {
      case 1: // MEDIA_ERR_ABORTED
        return "音频播放被中止";
      case 2: // MEDIA_ERR_NETWORK
        return "音频网络错误，请检查网络连接";
      case 3: // MEDIA_ERR_DECODE
        return "音频解码错误，可能是格式不支持";
      case 4: // MEDIA_ERR_SRC_NOT_SUPPORTED
        return "音频格式不支持";
      default:
        return `音频播放错误 (代码: ${error.code})`;
    }
  }

  /**
   * 判断媒体错误是否应该重试
   * @private
   */
  _shouldRetryMediaError(errorType) {
    const retryableErrors = [
      "REMOTE_STREAM_PROCESSING_FAILED",
      "STREAM_APPLICATION_FAILED",
      "VIDEO_STREAM_SETUP_FAILED",
      "AUDIO_STREAM_SETUP_FAILED",
    ];

    return retryableErrors.includes(errorType);
  }

  /**
   * 设置轨道事件监听器
   * @private
   */
  _setupTrackEvents(track) {
    if (!track) return;

    // 轨道结束事件
    track.addEventListener("ended", () => {
      this.logger.info("WebRTC", "媒体轨道结束", {
        trackKind: track.kind,
        trackId: track.id,
        trackLabel: track.label,
      });

      this.eventBus?.emit("webrtc:track-ended", {
        track: track,
        timestamp: Date.now(),
      });

      // 如果所有轨道都结束了，清理媒体元素
      if (this.remoteStream) {
        const activeTracks = this.remoteStream
          .getTracks()
          .filter((t) => t.readyState === "live");
        if (activeTracks.length === 0) {
          this.logger.info("WebRTC", "所有媒体轨道已结束，清理媒体元素");
          this._clearMediaElements();
        }
      }
    });

    // 轨道静音状态变化
    track.addEventListener("mute", () => {
      this.logger.info("WebRTC", "媒体轨道静音", {
        trackKind: track.kind,
        trackId: track.id,
      });

      this.eventBus?.emit("webrtc:track-muted", {
        track: track,
        timestamp: Date.now(),
      });
    });

    track.addEventListener("unmute", () => {
      this.logger.info("WebRTC", "媒体轨道取消静音", {
        trackKind: track.kind,
        trackId: track.id,
      });

      this.eventBus?.emit("webrtc:track-unmuted", {
        track: track,
        timestamp: Date.now(),
      });
    });

    this.logger.debug("WebRTC", "轨道事件监听器已设置", {
      trackKind: track.kind,
      trackId: track.id,
    });
  }

  /**
   * 清理媒体元素
   * @private
   */
  _clearMediaElements() {
    try {
      // 清理视频元素
      if (this.videoElement) {
        this.videoElement.srcObject = null;
        this.videoElement.load(); // 重置元素状态
        this.logger.debug("WebRTC", "视频元素已清理");
      }

      // 清理音频元素
      if (this.audioElement) {
        this.audioElement.srcObject = null;
        this.audioElement.load(); // 重置元素状态
        this.logger.debug("WebRTC", "音频元素已清理");
      }

      // 触发媒体元素清理事件
      this.eventBus?.emit("webrtc:media-elements-cleared", {
        timestamp: Date.now(),
      });
    } catch (error) {
      this.logger.error("WebRTC", "清理媒体元素失败", error);
    }
  }

  /**
   * 获取媒体流状态信息
   */
  getMediaStreamStatus() {
    const status = {
      hasRemoteStream: !!this.remoteStream,
      streamId: this.remoteStream?.id || null,
      tracks: {
        video: [],
        audio: [],
      },
      elements: {
        video: {
          present: !!this.videoElement,
          ready: this.videoElement ? this.videoElement.readyState : null,
          playing: this.videoElement ? !this.videoElement.paused : false,
          muted: this.videoElement ? this.videoElement.muted : null,
          dimensions: this.videoElement
            ? {
                width: this.videoElement.videoWidth,
                height: this.videoElement.videoHeight,
              }
            : null,
        },
        audio: {
          present: !!this.audioElement,
          ready: this.audioElement ? this.audioElement.readyState : null,
          playing: this.audioElement ? !this.audioElement.paused : false,
          muted: this.audioElement ? this.audioElement.muted : null,
          volume: this.audioElement ? this.audioElement.volume : null,
        },
      },
      timestamp: Date.now(),
    };

    // 收集轨道信息
    if (this.remoteStream) {
      this.remoteStream.getVideoTracks().forEach((track) => {
        status.tracks.video.push({
          id: track.id,
          label: track.label,
          enabled: track.enabled,
          muted: track.muted,
          readyState: track.readyState,
          settings: track.getSettings ? track.getSettings() : null,
        });
      });

      this.remoteStream.getAudioTracks().forEach((track) => {
        status.tracks.audio.push({
          id: track.id,
          label: track.label,
          enabled: track.enabled,
          muted: track.muted,
          readyState: track.readyState,
          settings: track.getSettings ? track.getSettings() : null,
        });
      });
    }

    return status;
  }

  /**
   * 控制媒体播放
   */
  async controlMediaPlayback(action, mediaType = "both") {
    try {
      const results = {
        video: null,
        audio: null,
        timestamp: Date.now(),
      };

      // 控制视频
      if (
        (mediaType === "both" || mediaType === "video") &&
        this.videoElement
      ) {
        try {
          switch (action) {
            case "play":
              await this.videoElement.play();
              results.video = "playing";
              break;
            case "pause":
              this.videoElement.pause();
              results.video = "paused";
              break;
            case "mute":
              this.videoElement.muted = true;
              results.video = "muted";
              break;
            case "unmute":
              this.videoElement.muted = false;
              results.video = "unmuted";
              break;
            default:
              throw new Error(`未知的媒体控制动作: ${action}`);
          }
        } catch (error) {
          results.video = `error: ${error.message}`;
          this.logger.error("WebRTC", `视频${action}操作失败`, error);
        }
      }

      // 控制音频
      if (
        (mediaType === "both" || mediaType === "audio") &&
        this.audioElement
      ) {
        try {
          switch (action) {
            case "play":
              await this.audioElement.play();
              results.audio = "playing";
              break;
            case "pause":
              this.audioElement.pause();
              results.audio = "paused";
              break;
            case "mute":
              this.audioElement.muted = true;
              results.audio = "muted";
              break;
            case "unmute":
              this.audioElement.muted = false;
              results.audio = "unmuted";
              break;
            default:
              throw new Error(`未知的媒体控制动作: ${action}`);
          }
        } catch (error) {
          results.audio = `error: ${error.message}`;
          this.logger.error("WebRTC", `音频${action}操作失败`, error);
        }
      }

      this.logger.info("WebRTC", `媒体控制操作完成: ${action}`, results);

      // 触发媒体控制事件
      this.eventBus?.emit("webrtc:media-control", {
        action: action,
        mediaType: mediaType,
        results: results,
        timestamp: Date.now(),
      });

      return results;
    } catch (error) {
      this.logger.error("WebRTC", "媒体控制操作失败", error);
      throw error;
    }
  }

  /**
   * 清理媒体流
   * @private
   */
  _cleanupMediaStreams() {
    try {
      // 停止所有远程流轨道
      if (this.remoteStream) {
        this.logger.info("WebRTC", "清理远程媒体流", {
          streamId: this.remoteStream.id,
          trackCount: this.remoteStream.getTracks().length,
        });

        this.remoteStream.getTracks().forEach((track) => {
          try {
            track.stop();
            this.logger.debug("WebRTC", "媒体轨道已停止", {
              trackKind: track.kind,
              trackId: track.id,
            });
          } catch (error) {
            this.logger.warn("WebRTC", "停止媒体轨道失败", error);
          }
        });

        this.remoteStream = null;
      }

      // 清理媒体元素
      this._clearMediaElements();

      this.logger.info("WebRTC", "媒体流清理完成");
    } catch (error) {
      this.logger.error("WebRTC", "清理媒体流失败", error);
    }
  }

  /**
   * 设置数据通道事件（增强版）
   * 支持更好的消息处理和错误恢复
   * @private
   */
  _setupDataChannelEvents(channel) {
    // 通道打开事件
    channel.onopen = () => {
      this.logger.info("WebRTC", "数据通道已打开", {
        label: channel.label,
        readyState: channel.readyState,
        id: channel.id,
        ordered: channel.ordered,
        maxRetransmits: channel.maxRetransmits,
        protocol: channel.protocol,
      });

      // 更新通道状态统计
      this._updateDataChannelConnectionStats(channel, "opened");

      this.eventBus?.emit("webrtc:datachannel-open", {
        label: channel.label,
        channel: channel,
        channelInfo: {
          id: channel.id,
          ordered: channel.ordered,
          maxRetransmits: channel.maxRetransmits,
          protocol: channel.protocol,
        },
        timestamp: Date.now(),
      });
    };

    // 通道关闭事件
    channel.onclose = () => {
      this.logger.info("WebRTC", "数据通道已关闭", {
        label: channel.label,
        readyState: channel.readyState,
      });

      // 更新通道状态统计
      this._updateDataChannelConnectionStats(channel, "closed");

      this.eventBus?.emit("webrtc:datachannel-close", {
        label: channel.label,
        timestamp: Date.now(),
      });
    };

    // 消息接收事件（增强版）
    channel.onmessage = (event) => {
      this._handleDataChannelMessage(channel, event);
    };

    // 错误处理事件（增强版）
    channel.onerror = (error) => {
      this._handleDataChannelError(channel, error);
    };
  }

  /**
   * 处理数据通道消息接收（增强版）
   * 支持多种消息格式和协议兼容性
   * @private
   */
  _handleDataChannelMessage(channel, event) {
    try {
      const rawData = event.data;
      const dataSize = rawData.length || 0;

      this.logger.debug("WebRTC", "接收到数据通道原始消息", {
        label: channel.label,
        dataType: typeof rawData,
        dataSize: dataSize,
      });

      // 解析消息内容
      let parsedMessage;
      let messageType = "unknown";

      try {
        // 尝试解析为 JSON（保持协议兼容性）
        parsedMessage = JSON.parse(rawData);
        messageType = parsedMessage.type || "object";
      } catch (parseError) {
        // 如果不是 JSON，作为字符串处理
        parsedMessage = rawData;
        messageType = "string";

        this.logger.debug("WebRTC", "消息不是 JSON 格式，作为字符串处理", {
          label: channel.label,
          messagePreview: rawData.substring(0, 100),
        });
      }

      // 验证消息内容
      if (!this._validateReceivedMessage(parsedMessage, rawData)) {
        this.logger.warn("WebRTC", "接收到的消息验证失败", {
          label: channel.label,
          messageType: messageType,
        });
        return;
      }

      // 更新接收统计
      this._updateDataChannelStats("received", dataSize, messageType);

      // 处理特殊消息类型（保持协议兼容性）
      const processedMessage = this._processReceivedMessage(
        parsedMessage,
        messageType
      );

      this.logger.debug("WebRTC", "数据通道消息处理完成", {
        label: channel.label,
        messageType: messageType,
        dataSize: dataSize,
        processed: !!processedMessage,
      });

      // 触发消息接收事件
      this.eventBus?.emit("webrtc:datachannel-message", {
        label: channel.label,
        message: processedMessage || parsedMessage,
        originalMessage: parsedMessage,
        messageType: messageType,
        dataSize: dataSize,
        timestamp: Date.now(),
        channel: {
          label: channel.label,
          readyState: channel.readyState,
        },
      });

      // 触发特定类型的消息事件（用于更精确的处理）
      if (messageType !== "unknown") {
        this.eventBus?.emit(`webrtc:datachannel-message:${messageType}`, {
          label: channel.label,
          message: processedMessage || parsedMessage,
          dataSize: dataSize,
          timestamp: Date.now(),
        });
      }
    } catch (error) {
      this.logger.error("WebRTC", "处理数据通道消息时发生错误", {
        label: channel.label,
        error: error.message,
        stack: error.stack,
      });

      // 更新错误统计
      this._updateDataChannelStats("failed", 0, "parse-error");

      // 触发消息处理错误事件
      this.eventBus?.emit("webrtc:datachannel-message-error", {
        label: channel.label,
        error: error,
        rawData: event.data,
        timestamp: Date.now(),
      });
    }
  }

  /**
   * 验证接收到的消息
   * @private
   */
  _validateReceivedMessage(message, rawData) {
    // 检查消息大小限制
    if (rawData && rawData.length > 65536) {
      // 64KB 限制
      this.logger.warn("WebRTC", "接收到的消息过大", { size: rawData.length });
      return false;
    }

    // 检查消息内容
    if (message === null || message === undefined) {
      return false;
    }

    return true;
  }

  /**
   * 处理接收到的消息（协议兼容性处理）
   * @private
   */
  _processReceivedMessage(message, messageType) {
    // 处理特殊的协议消息
    if (messageType === "object" && message.type) {
      switch (message.type) {
        case "ping":
          // 处理 ping 消息，自动回复 pong
          this._handlePingMessage(message);
          break;
        case "pong":
          // 处理 pong 消息，更新延迟统计
          this._handlePongMessage(message);
          break;
        case "input":
          // 处理输入消息（鼠标、键盘等）
          return this._processInputMessage(message);
        case "control":
          // 处理控制消息
          return this._processControlMessage(message);
        default:
          // 其他消息类型直接返回
          break;
      }
    }

    return message;
  }

  /**
   * 处理 ping 消息
   * @private
   */
  _handlePingMessage(message) {
    try {
      const pongMessage = {
        type: "pong",
        timestamp: message.timestamp,
        responseTime: Date.now(),
      };

      this.sendDataChannelMessage(pongMessage, { validateMessage: false });

      this.logger.debug("WebRTC", "已回复 ping 消息", {
        originalTimestamp: message.timestamp,
        responseTime: pongMessage.responseTime,
      });
    } catch (error) {
      this.logger.error("WebRTC", "回复 ping 消息失败", error);
    }
  }

  /**
   * 处理 pong 消息
   * @private
   */
  _handlePongMessage(message) {
    if (message.timestamp) {
      const latency = Date.now() - message.timestamp;

      // 更新延迟统计
      if (!this.connectionStats.latencyStats) {
        this.connectionStats.latencyStats = {
          current: 0,
          average: 0,
          min: Infinity,
          max: 0,
          samples: [],
        };
      }

      const latencyStats = this.connectionStats.latencyStats;
      latencyStats.current = latency;
      latencyStats.min = Math.min(latencyStats.min, latency);
      latencyStats.max = Math.max(latencyStats.max, latency);

      // 保持最近 10 个样本
      latencyStats.samples.push(latency);
      if (latencyStats.samples.length > 10) {
        latencyStats.samples.shift();
      }

      // 计算平均延迟
      latencyStats.average =
        latencyStats.samples.reduce((a, b) => a + b, 0) /
        latencyStats.samples.length;

      this.logger.debug("WebRTC", "延迟统计更新", {
        current: latency,
        average: Math.round(latencyStats.average),
        min: latencyStats.min,
        max: latencyStats.max,
      });
    }
  }

  /**
   * 处理输入消息
   * @private
   */
  _processInputMessage(message) {
    // 为输入消息添加时间戳（如果没有）
    if (!message.timestamp) {
      message.timestamp = Date.now();
    }

    // 验证输入消息格式
    if (!message.inputType || !message.data) {
      this.logger.warn("WebRTC", "输入消息格式不完整", message);
      return message;
    }

    this.logger.debug("WebRTC", "处理输入消息", {
      inputType: message.inputType,
      dataKeys: Object.keys(message.data),
    });

    return message;
  }

  /**
   * 处理控制消息
   * @private
   */
  _processControlMessage(message) {
    this.logger.debug("WebRTC", "处理控制消息", {
      controlType: message.controlType || "unknown",
      action: message.action || "unknown",
    });

    return message;
  }

  /**
   * 处理数据通道错误（增强版）
   * @private
   */
  _handleDataChannelError(channel, error) {
    this.logger.error("WebRTC", "数据通道错误", {
      label: channel.label,
      readyState: channel.readyState,
      error: error,
      errorType: error.type || "unknown",
      errorMessage: error.message || "unknown",
    });

    // 更新错误统计
    this._updateDataChannelConnectionStats(channel, "error");

    // 尝试错误恢复
    this._attemptDataChannelErrorRecovery(channel, error);

    this.eventBus?.emit("webrtc:datachannel-error", {
      label: channel.label,
      error: error,
      channelState: channel.readyState,
      timestamp: Date.now(),
      recovery: {
        attempted: true,
        strategy: this._getErrorRecoveryStrategy(error),
      },
    });
  }

  /**
   * 尝试数据通道错误恢复
   * @private
   */
  _attemptDataChannelErrorRecovery(channel, error) {
    const strategy = this._getErrorRecoveryStrategy(error);

    this.logger.info("WebRTC", "尝试数据通道错误恢复", {
      label: channel.label,
      strategy: strategy,
      error: error.message,
    });

    switch (strategy) {
      case "recreate":
        // 重新创建数据通道
        setTimeout(() => {
          if (channel === this.sendChannel) {
            this._recreateSendDataChannel();
          }
        }, 1000);
        break;
      case "reconnect":
        // 触发连接重建
        this.eventBus?.emit("webrtc:request-reconnect", {
          reason: "datachannel-error",
          error: error,
        });
        break;
      case "wait":
        // 等待自动恢复
        this.logger.info("WebRTC", "等待数据通道自动恢复");
        break;
      default:
        this.logger.warn("WebRTC", "无可用的错误恢复策略");
    }
  }

  /**
   * 获取错误恢复策略
   * @private
   */
  _getErrorRecoveryStrategy(error) {
    if (!error) return "wait";

    const errorMessage = error.message || "";
    const errorType = error.type || "";

    if (errorMessage.includes("closed") || errorType === "close") {
      return "recreate";
    } else if (errorMessage.includes("network") || errorType === "network") {
      return "reconnect";
    } else {
      return "wait";
    }
  }

  /**
   * 更新数据通道连接统计
   * @private
   */
  _updateDataChannelConnectionStats(channel, event) {
    if (!this.connectionStats.dataChannelConnectionStats) {
      this.connectionStats.dataChannelConnectionStats = {
        channels: {},
        totalOpened: 0,
        totalClosed: 0,
        totalErrors: 0,
      };
    }

    const stats = this.connectionStats.dataChannelConnectionStats;
    const channelLabel = channel.label;

    if (!stats.channels[channelLabel]) {
      stats.channels[channelLabel] = {
        opened: 0,
        closed: 0,
        errors: 0,
        lastEvent: null,
        lastEventTime: null,
      };
    }

    const channelStats = stats.channels[channelLabel];
    channelStats.lastEvent = event;
    channelStats.lastEventTime = Date.now();

    switch (event) {
      case "opened":
        channelStats.opened++;
        stats.totalOpened++;
        break;
      case "closed":
        channelStats.closed++;
        stats.totalClosed++;
        break;
      case "error":
        channelStats.errors++;
        stats.totalErrors++;
        break;
    }
  }

  /**
   * 处理 ICE 连接状态变化
   * @private
   */
  _handleIceConnectionStateChange(state) {
    switch (state) {
      case "connected":
      case "completed":
        if (!this.connectionEstablished) {
          this.connectionEstablished = true;
          this.connectionStats.connectTime = Date.now();
          this.connectionStats.totalConnections++;

          // 重置错误和重试状态
          this._resetRetryState();

          this._setState("connected");
          this._clearConnectionTimer();

          // 开始统计收集
          this._startStatsCollection();

          this.logger.info("WebRTC", "连接建立成功", {
            connectTime: this.connectionStats.connectTime,
            totalConnections: this.connectionStats.totalConnections,
          });

          this.eventBus?.emit("webrtc:connected", {
            connectTime: this.connectionStats.connectTime,
            stats: this.getConnectionStats(),
          });
        }
        break;

      case "disconnected":
        this.logger.warn("WebRTC", "ICE 连接断开");
        if (this.connectionEstablished) {
          this._setState("connecting"); // 可能是临时断开
        }
        break;

      case "failed":
        this.logger.error("WebRTC", "ICE 连接失败", {
          iceConnectionState: state,
          signalingState: this.signalingState,
          retryCount: this.retryCount,
        });

        const iceError = new Error("ICE 连接失败");
        iceError.code = "ICE_CONNECTION_FAILED";
        iceError.details = {
          iceConnectionState: state,
          signalingState: this.signalingState,
        };

        this._handleConnectionError(iceError, {
          iceConnectionState: state,
          isIceFailure: true,
        });
        break;

      case "closed":
        this.logger.info("WebRTC", "ICE 连接已关闭");
        this._setState("disconnected");
        break;

      default:
        this.logger.debug("WebRTC", "ICE 连接状态", { state });
    }

    // 触发 ICE 状态变化事件
    this.eventBus?.emit("webrtc:ice-state-change", {
      state: state,
      timestamp: Date.now(),
    });
  }

  /**
   * 处理信令状态变化
   * @private
   */
  _handleSignalingStateChange(state) {
    this.logger.debug("WebRTC", "信令状态变化", { state });

    // 触发信令状态变化事件
    this.eventBus?.emit("webrtc:signaling-state-change", {
      state: state,
      timestamp: Date.now(),
    });
  }

  /**
   * 处理连接状态变化（现代浏览器）
   * @private
   */
  _handleConnectionStateChange(state) {
    this.logger.debug("WebRTC", "连接状态变化", { state });

    switch (state) {
      case "connected":
        // 已在 ICE 状态处理中处理
        break;
      case "disconnected":
        this.logger.warn("WebRTC", "连接断开");
        break;
      case "failed":
        this.logger.error("WebRTC", "连接失败");
        this._handleConnectionError(new Error("Connection failed"));
        break;
      case "closed":
        this.logger.info("WebRTC", "连接已关闭");
        this._setState("disconnected");
        break;
    }

    // 触发连接状态变化事件
    this.eventBus?.emit("webrtc:connection-state-change", {
      state: state,
      timestamp: Date.now(),
    });
  }

  /**
   * 连接到远程端
   */
  async connect() {
    try {
      this.logger.info("WebRTC", "开始建立 WebRTC 连接");

      // 检查前置条件
      if (!this.signalingManager) {
        throw new Error("信令管理器未设置");
      }

      if (!verifyWebRTCAdapter()) {
        throw new Error("WebRTC Adapter 不可用");
      }

      // 设置连接状态
      this._setState("connecting");

      // 清理现有连接
      if (this.pc) {
        await this.disconnect();
      }

      // 创建新的 PeerConnection
      this._createPeerConnection();

      // 创建发送数据通道
      this._createSendDataChannel();

      // 设置连接超时
      this._setConnectionTimer();

      // 开始信令握手（请求服务器创建 offer）
      await this._requestOffer();

      this.logger.info("WebRTC", "连接请求已发送，等待响应");

      return Promise.resolve();
    } catch (error) {
      this.logger.error("WebRTC", "连接失败", error);

      // 分类连接错误
      const connectionError = new Error(`连接建立失败: ${error.message}`);
      connectionError.code = "CONNECTION_SETUP_FAILED";
      connectionError.originalError = error;

      this._handleConnectionError(connectionError, {
        isConnectionSetup: true,
        phase: "initial_connection",
      });

      throw connectionError;
    }
  }

  /**
   * 断开连接
   */
  async disconnect() {
    try {
      this.logger.info("WebRTC", "开始断开 WebRTC 连接");

      // 清理定时器
      this._clearConnectionTimer();
      this._clearRetryTimer();
      this._stopStatsCollection();

      // 关闭数据通道（使用增强的清理方法）
      this._cleanupDataChannels();

      // 关闭 PeerConnection
      if (this.pc) {
        this.pc.close();
        this.pc = null;
      }

      // 清理媒体流和元素 - 使用增强的清理方法
      this._cleanupMediaStreams();
      this._cleanupMediaElementEvents();

      // 更新状态
      this.connectionEstablished = false;
      this.connectionStats.disconnectTime = Date.now();
      this._setState("disconnected");

      this.logger.info("WebRTC", "连接已断开", {
        disconnectTime: this.connectionStats.disconnectTime,
      });

      // 触发断开事件
      this.eventBus?.emit("webrtc:disconnected", {
        disconnectTime: this.connectionStats.disconnectTime,
        stats: this.getConnectionStats(),
      });

      return Promise.resolve();
    } catch (error) {
      this.logger.error("WebRTC", "断开连接失败", error);

      // 记录断开连接错误，但不触发重试
      const disconnectError = this._classifyError(error, {
        isDisconnect: true,
        phase: "cleanup",
      });
      disconnectError.shouldRetry = false; // 断开连接错误不应重试

      this._recordError(disconnectError);

      // 确保状态正确设置
      this._setState("disconnected");

      throw error;
    }
  }

  /**
   * 创建发送数据通道（增强版）
   * 支持更灵活的配置和更好的错误处理
   * @private
   */
  _createSendDataChannel(options = {}) {
    if (!this.pc) {
      throw new Error("PeerConnection not created");
    }

    const {
      label = "input", // 通道标签，保持与现有协议兼容
      ordered = true, // 是否保证消息顺序
      maxRetransmits = 3, // 最大重传次数
      maxPacketLifeTime = null, // 最大包生存时间
      protocol = "", // 子协议
      negotiated = false, // 是否预协商
      id = null, // 通道 ID
    } = options;

    try {
      // 准备数据通道配置
      const channelConfig = {
        ordered: ordered,
        protocol: protocol,
        negotiated: negotiated,
      };

      // 设置重传策略（互斥选项）
      if (maxPacketLifeTime !== null) {
        channelConfig.maxPacketLifeTime = maxPacketLifeTime;
      } else {
        channelConfig.maxRetransmits = maxRetransmits;
      }

      // 设置通道 ID（如果指定）
      if (id !== null) {
        channelConfig.id = id;
      }

      this.logger.info("WebRTC", "创建发送数据通道", {
        label: label,
        config: channelConfig,
      });

      // 创建数据通道
      this.sendChannel = this.pc.createDataChannel(label, channelConfig);

      // 设置事件处理器
      this._setupDataChannelEvents(this.sendChannel);

      // 添加通道状态监控
      this._monitorDataChannelState(this.sendChannel);

      this.logger.info("WebRTC", "发送数据通道已创建", {
        label: this.sendChannel.label,
        id: this.sendChannel.id,
        readyState: this.sendChannel.readyState,
        ordered: this.sendChannel.ordered,
        maxRetransmits: this.sendChannel.maxRetransmits,
        maxPacketLifeTime: this.sendChannel.maxPacketLifeTime,
        protocol: this.sendChannel.protocol,
      });

      // 触发数据通道创建事件
      this.eventBus?.emit("webrtc:datachannel-created", {
        type: "send",
        label: this.sendChannel.label,
        channel: this.sendChannel,
        config: channelConfig,
        timestamp: Date.now(),
      });
    } catch (error) {
      this.logger.error("WebRTC", "创建发送数据通道失败", {
        error: error.message,
        label: label,
        config: options,
      });

      // 触发创建失败事件
      this.eventBus?.emit("webrtc:datachannel-create-failed", {
        type: "send",
        label: label,
        error: error,
        timestamp: Date.now(),
      });

      throw error;
    }
  }

  /**
   * 重新创建发送数据通道
   * @private
   */
  _recreateSendDataChannel() {
    this.logger.info("WebRTC", "重新创建发送数据通道");

    try {
      // 清理旧的通道
      if (this.sendChannel) {
        this.sendChannel.close();
        this.sendChannel = null;
      }

      // 创建新的通道
      this._createSendDataChannel();

      this.logger.info("WebRTC", "发送数据通道重新创建成功");

      // 触发重新创建成功事件
      this.eventBus?.emit("webrtc:datachannel-recreated", {
        type: "send",
        label: this.sendChannel.label,
        timestamp: Date.now(),
      });
    } catch (error) {
      this.logger.error("WebRTC", "重新创建发送数据通道失败", error);

      // 触发重新创建失败事件
      this.eventBus?.emit("webrtc:datachannel-recreate-failed", {
        type: "send",
        error: error,
        timestamp: Date.now(),
      });
    }
  }

  /**
   * 监控数据通道状态
   * @private
   */
  _monitorDataChannelState(channel) {
    const checkInterval = 5000; // 5秒检查一次

    const monitor = () => {
      if (!channel) return;

      const currentState = channel.readyState;

      this.logger.debug("WebRTC", "数据通道状态监控", {
        label: channel.label,
        readyState: currentState,
        bufferedAmount: channel.bufferedAmount,
        bufferedAmountLowThreshold: channel.bufferedAmountLowThreshold,
      });

      // 检查缓冲区状态
      if (channel.bufferedAmount > 0) {
        this.logger.debug("WebRTC", "数据通道缓冲区状态", {
          label: channel.label,
          bufferedAmount: channel.bufferedAmount,
          threshold: channel.bufferedAmountLowThreshold,
        });
      }

      // 如果通道仍然存在且未关闭，继续监控
      if (currentState !== "closed" && currentState !== "closing") {
        setTimeout(monitor, checkInterval);
      }
    };

    // 开始监控
    setTimeout(monitor, checkInterval);
  }

  /**
   * 获取数据通道状态信息
   */
  getDataChannelStatus() {
    const sendChannelStatus = this.sendChannel
      ? {
          label: this.sendChannel.label,
          id: this.sendChannel.id,
          readyState: this.sendChannel.readyState,
          bufferedAmount: this.sendChannel.bufferedAmount,
          bufferedAmountLowThreshold:
            this.sendChannel.bufferedAmountLowThreshold,
          ordered: this.sendChannel.ordered,
          maxRetransmits: this.sendChannel.maxRetransmits,
          maxPacketLifeTime: this.sendChannel.maxPacketLifeTime,
          protocol: this.sendChannel.protocol,
        }
      : null;

    const receiveChannelStatus = this.receiveChannel
      ? {
          label: this.receiveChannel.label,
          id: this.receiveChannel.id,
          readyState: this.receiveChannel.readyState,
          bufferedAmount: this.receiveChannel.bufferedAmount,
          bufferedAmountLowThreshold:
            this.receiveChannel.bufferedAmountLowThreshold,
          ordered: this.receiveChannel.ordered,
          maxRetransmits: this.receiveChannel.maxRetransmits,
          maxPacketLifeTime: this.receiveChannel.maxPacketLifeTime,
          protocol: this.receiveChannel.protocol,
        }
      : null;

    return {
      sendChannel: sendChannelStatus,
      receiveChannel: receiveChannelStatus,
      statistics: this.connectionStats.dataChannelStats || null,
      connectionStats: this.connectionStats.dataChannelConnectionStats || null,
      isReady: !!(this.sendChannel && this.sendChannel.readyState === "open"),
      canReceive: !!(
        this.receiveChannel && this.receiveChannel.readyState === "open"
      ),
      timestamp: Date.now(),
    };
  }

  /**
   * 清理数据通道资源
   * @private
   */
  _cleanupDataChannels() {
    this.logger.info("WebRTC", "清理数据通道资源");

    // 关闭发送通道
    if (this.sendChannel) {
      try {
        if (
          this.sendChannel.readyState === "open" ||
          this.sendChannel.readyState === "connecting"
        ) {
          this.sendChannel.close();
        }
      } catch (error) {
        this.logger.warn("WebRTC", "关闭发送数据通道时出错", error);
      }
      this.sendChannel = null;
    }

    // 关闭接收通道
    if (this.receiveChannel) {
      try {
        if (
          this.receiveChannel.readyState === "open" ||
          this.receiveChannel.readyState === "connecting"
        ) {
          this.receiveChannel.close();
        }
      } catch (error) {
        this.logger.warn("WebRTC", "关闭接收数据通道时出错", error);
      }
      this.receiveChannel = null;
    }

    // 清理统计信息
    if (this.connectionStats.dataChannelStats) {
      this.connectionStats.dataChannelStats.lastActivity = Date.now();
    }

    this.logger.info("WebRTC", "数据通道资源清理完成");
  }

  /**
   * 发送 ping 消息测试数据通道延迟
   */
  pingDataChannel() {
    const pingMessage = {
      type: "ping",
      timestamp: Date.now(),
      id: Math.random().toString(36).substr(2, 9),
    };

    const success = this.sendDataChannelMessage(pingMessage, {
      validateMessage: false,
    });

    if (success) {
      this.logger.debug("WebRTC", "Ping 消息已发送", {
        id: pingMessage.id,
        timestamp: pingMessage.timestamp,
      });
    }

    return success;
  }

  /**
   * 获取数据通道统计信息
   */
  getDataChannelStats() {
    const stats = this.connectionStats.dataChannelStats || {};
    const connectionStats =
      this.connectionStats.dataChannelConnectionStats || {};
    const latencyStats = this.connectionStats.latencyStats || {};

    return {
      // 消息统计
      messages: {
        sent: stats.messagesSent || 0,
        received: stats.messagesReceived || 0,
        failed: stats.messagesFailed || 0,
        total: (stats.messagesSent || 0) + (stats.messagesReceived || 0),
      },

      // 数据传输统计
      bytes: {
        sent: stats.bytesSent || 0,
        received: stats.bytesReceived || 0,
        total: (stats.bytesSent || 0) + (stats.bytesReceived || 0),
        sentFormatted: this._formatBytes(stats.bytesSent || 0),
        receivedFormatted: this._formatBytes(stats.bytesReceived || 0),
        totalFormatted: this._formatBytes(
          (stats.bytesSent || 0) + (stats.bytesReceived || 0)
        ),
      },

      // 消息类型统计
      messageTypes: stats.messageTypes || {},

      // 连接统计
      connections: {
        totalOpened: connectionStats.totalOpened || 0,
        totalClosed: connectionStats.totalClosed || 0,
        totalErrors: connectionStats.totalErrors || 0,
        channels: connectionStats.channels || {},
      },

      // 延迟统计
      latency: {
        current: latencyStats.current || 0,
        average: Math.round(latencyStats.average || 0),
        min: latencyStats.min === Infinity ? 0 : latencyStats.min || 0,
        max: latencyStats.max || 0,
        samples: latencyStats.samples ? latencyStats.samples.length : 0,
      },

      // 活动状态
      lastActivity: stats.lastActivity,
      lastActivityFormatted: stats.lastActivity
        ? new Date(stats.lastActivity).toLocaleString()
        : "Never",

      // 当前状态
      currentStatus: this.getDataChannelStatus(),

      // 时间戳
      timestamp: Date.now(),
    };
  }

  /**
   * 重置数据通道统计信息
   */
  resetDataChannelStats() {
    this.connectionStats.dataChannelStats = {
      messagesSent: 0,
      messagesReceived: 0,
      messagesFailed: 0,
      bytesSent: 0,
      bytesReceived: 0,
      messageTypes: {},
      lastActivity: null,
    };

    this.connectionStats.dataChannelConnectionStats = {
      channels: {},
      totalOpened: 0,
      totalClosed: 0,
      totalErrors: 0,
    };

    this.connectionStats.latencyStats = {
      current: 0,
      average: 0,
      min: Infinity,
      max: 0,
      samples: [],
    };

    this.logger.info("WebRTC", "数据通道统计信息已重置");

    // 触发统计重置事件
    this.eventBus?.emit("webrtc:datachannel-stats-reset", {
      timestamp: Date.now(),
    });
  }

  /**
   * 检查数据通道健康状态
   */
  checkDataChannelHealth() {
    const status = this.getDataChannelStatus();
    const stats = this.getDataChannelStats();

    const health = {
      overall: "unknown",
      issues: [],
      recommendations: [],
      score: 0,
      details: {
        sendChannel: "unknown",
        receiveChannel: "unknown",
        latency: "unknown",
        errorRate: "unknown",
      },
    };

    // 检查发送通道
    if (status.sendChannel) {
      if (status.sendChannel.readyState === "open") {
        health.details.sendChannel = "healthy";
        health.score += 25;
      } else if (status.sendChannel.readyState === "connecting") {
        health.details.sendChannel = "connecting";
        health.score += 10;
        health.issues.push("发送通道正在连接中");
      } else {
        health.details.sendChannel = "unhealthy";
        health.issues.push("发送通道未就绪");
      }
    } else {
      health.details.sendChannel = "missing";
      health.issues.push("发送通道不存在");
    }

    // 检查接收通道
    if (status.receiveChannel) {
      if (status.receiveChannel.readyState === "open") {
        health.details.receiveChannel = "healthy";
        health.score += 25;
      } else if (status.receiveChannel.readyState === "connecting") {
        health.details.receiveChannel = "connecting";
        health.score += 10;
        health.issues.push("接收通道正在连接中");
      } else {
        health.details.receiveChannel = "unhealthy";
        health.issues.push("接收通道未就绪");
      }
    } else {
      health.details.receiveChannel = "missing";
      health.issues.push("接收通道不存在");
    }

    // 检查延迟
    const avgLatency = stats.latency.average;
    if (avgLatency > 0) {
      if (avgLatency < 50) {
        health.details.latency = "excellent";
        health.score += 25;
      } else if (avgLatency < 100) {
        health.details.latency = "good";
        health.score += 20;
      } else if (avgLatency < 200) {
        health.details.latency = "fair";
        health.score += 15;
        health.issues.push("延迟较高");
      } else {
        health.details.latency = "poor";
        health.score += 5;
        health.issues.push("延迟过高");
        health.recommendations.push("检查网络连接质量");
      }
    } else {
      health.details.latency = "unknown";
      health.score += 10;
    }

    // 检查错误率
    const totalMessages = stats.messages.total;
    const failedMessages = stats.messages.failed;
    if (totalMessages > 0) {
      const errorRate = (failedMessages / totalMessages) * 100;
      if (errorRate < 1) {
        health.details.errorRate = "excellent";
        health.score += 25;
      } else if (errorRate < 5) {
        health.details.errorRate = "good";
        health.score += 20;
      } else if (errorRate < 10) {
        health.details.errorRate = "fair";
        health.score += 15;
        health.issues.push("消息失败率较高");
      } else {
        health.details.errorRate = "poor";
        health.score += 5;
        health.issues.push("消息失败率过高");
        health.recommendations.push("检查数据通道稳定性");
      }
    } else {
      health.details.errorRate = "unknown";
      health.score += 15;
    }

    // 确定整体健康状态
    if (health.score >= 90) {
      health.overall = "excellent";
    } else if (health.score >= 70) {
      health.overall = "good";
    } else if (health.score >= 50) {
      health.overall = "fair";
    } else if (health.score >= 30) {
      health.overall = "poor";
    } else {
      health.overall = "critical";
    }

    // 添加通用建议
    if (health.issues.length === 0) {
      health.recommendations.push("数据通道运行正常");
    } else if (health.overall === "critical") {
      health.recommendations.push("建议重新建立连接");
    }

    return health;
  }

  /**
   * 创建并发送 Offer（当作为发起方时使用）
   * @private
   */
  async _createAndSendOffer() {
    if (!this.pc || !this.signalingManager) {
      throw new Error("PeerConnection or SignalingManager not available");
    }

    try {
      this.logger.info("WebRTC", "创建 SDP Offer");

      // 创建 offer
      const offer = await this.pc.createOffer({
        offerToReceiveAudio: true,
        offerToReceiveVideo: true,
      });

      // 设置本地描述
      await this.pc.setLocalDescription(offer);

      this.logger.info("WebRTC", "本地描述已设置", {
        type: offer.type,
        sdpLength: offer.sdp.length,
      });

      // 发送 offer 到信令服务器
      await this._sendOffer(offer);

      this.logger.info("WebRTC", "SDP Offer 已发送");
    } catch (error) {
      this.logger.error("WebRTC", "创建或发送 Offer 失败", error);
      throw error;
    }
  }

  /**
   * 设置连接超时定时器（增强版）
   * @private
   */
  _setConnectionTimer() {
    this._clearConnectionTimer();

    // 根据重试次数调整超时时间
    const timeoutMultiplier = Math.min(1 + this.retryCount * 0.5, 3); // 最多3倍超时
    const adjustedTimeout = this.connectionTimeout * timeoutMultiplier;

    this.connectionTimer = setTimeout(() => {
      if (this.connectionState === "connecting") {
        this.logger.error("WebRTC", "连接超时", {
          timeout: adjustedTimeout,
          originalTimeout: this.connectionTimeout,
          retryCount: this.retryCount,
          iceConnectionState: this.iceConnectionState,
          signalingState: this.signalingState,
        });

        // 创建详细的超时错误
        const timeoutError = new Error(`连接超时 (${adjustedTimeout}ms)`);
        timeoutError.code = "CONNECTION_TIMEOUT";
        timeoutError.details = {
          timeout: adjustedTimeout,
          retryCount: this.retryCount,
          iceConnectionState: this.iceConnectionState,
          signalingState: this.signalingState,
        };

        this._handleConnectionError(timeoutError, {
          isTimeout: true,
          adjustedTimeout: adjustedTimeout,
        });
      }
    }, adjustedTimeout);

    this.logger.debug("WebRTC", "连接超时定时器已设置", {
      timeout: adjustedTimeout,
      originalTimeout: this.connectionTimeout,
      multiplier: timeoutMultiplier,
      retryCount: this.retryCount,
    });
  }

  /**
   * 清理连接超时定时器
   * @private
   */
  _clearConnectionTimer() {
    if (this.connectionTimer) {
      clearTimeout(this.connectionTimer);
      this.connectionTimer = null;
    }
  }

  /**
   * 清理重试定时器
   * @private
   */
  _clearRetryTimer() {
    if (this.retryTimer) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
  }

  /**
   * 开始统计收集
   * @private
   */
  _startStatsCollection() {
    // 清理现有定时器
    this._stopStatsCollection();

    this.logger.info("WebRTC", "开始收集连接统计信息", {
      interval: this.statsCollectionInterval,
    });

    // 立即收集一次统计
    this._collectStats();

    // 设置定期收集
    this.statsCollectionTimer = setInterval(() => {
      this._collectStats();
    }, this.statsCollectionInterval);

    // 触发统计收集开始事件
    this.eventBus?.emit("webrtc:stats-collection-started", {
      interval: this.statsCollectionInterval,
    });
  }

  /**
   * 停止统计收集
   * @private
   */
  _stopStatsCollection() {
    if (this.statsCollectionTimer) {
      clearInterval(this.statsCollectionTimer);
      this.statsCollectionTimer = null;

      this.logger.info("WebRTC", "统计收集已停止");

      // 触发统计收集停止事件
      this.eventBus?.emit("webrtc:stats-collection-stopped");
    }
  }

  /**
   * 收集统计信息
   * @private
   */
  async _collectStats() {
    try {
      if (!this.pc || this.connectionState !== "connected") {
        return;
      }

      // 获取实时统计
      const realTimeStats = await this.getRealTimeStats();

      if (realTimeStats) {
        // 触发统计更新事件
        this.eventBus?.emit("webrtc:stats-updated", {
          stats: this.getConnectionStats(),
          realTimeStats: realTimeStats,
          timestamp: Date.now(),
        });

        // 记录详细统计到日志（调试级别）
        this.logger.debug("WebRTC", "统计信息已更新", {
          quality: this.connectionStats.connectionQuality,
          score: this.connectionStats.qualityScore,
          rtt: this.connectionStats.currentRoundTripTime,
          packetLoss: this.connectionStats.packetsLost,
          bitrate: {
            incoming: this.connectionStats.availableIncomingBitrate,
            outgoing: this.connectionStats.availableOutgoingBitrate,
          },
        });
      }
    } catch (error) {
      this.logger.error("WebRTC", "收集统计信息失败", error);
    }
  }

  /**
   * 设置统计收集间隔
   */
  setStatsCollectionInterval(intervalMs) {
    if (intervalMs < 100 || intervalMs > 10000) {
      this.logger.warn("WebRTC", "统计收集间隔超出范围，使用默认值", {
        requested: intervalMs,
        default: this.statsCollectionInterval,
      });
      return;
    }

    const oldInterval = this.statsCollectionInterval;
    this.statsCollectionInterval = intervalMs;

    this.logger.info("WebRTC", "统计收集间隔已更新", {
      oldInterval: oldInterval,
      newInterval: intervalMs,
    });

    // 如果正在收集统计，重启定时器
    if (this.statsCollectionTimer) {
      this._startStatsCollection();
    }

    // 触发间隔更新事件
    this.eventBus?.emit("webrtc:stats-interval-changed", {
      oldInterval: oldInterval,
      newInterval: intervalMs,
    });
  }

  /**
   * 获取连接质量监控信息
   */
  getConnectionQualityMonitoring() {
    const stats = this.getConnectionStats();
    const assessment = stats.qualityAssessment;

    return {
      // 当前质量状态
      currentQuality: {
        level: assessment.quality,
        score: assessment.score,
        description: assessment.recommendation,
      },

      // 关键指标
      keyMetrics: {
        latency: {
          current: assessment.metrics.rtt,
          average: stats.performanceMetrics.averageRtt * 1000,
          unit: "ms",
          status:
            assessment.metrics.rtt < 100
              ? "good"
              : assessment.metrics.rtt < 200
              ? "fair"
              : "poor",
        },
        packetLoss: {
          current: assessment.metrics.packetLoss,
          rate: stats.performanceMetrics.packetLossRate,
          unit: "%",
          status:
            assessment.metrics.packetLoss < 1
              ? "good"
              : assessment.metrics.packetLoss < 3
              ? "fair"
              : "poor",
        },
        jitter: {
          current: assessment.metrics.jitter,
          unit: "ms",
          status:
            assessment.metrics.jitter < 20
              ? "good"
              : assessment.metrics.jitter < 50
              ? "fair"
              : "poor",
        },
        bandwidth: {
          incoming: stats.performanceMetrics.bitrateMbps.incoming,
          outgoing: stats.performanceMetrics.bitrateMbps.outgoing,
          unit: "Mbps",
        },
      },

      // 问题和建议
      issues: assessment.issues,

      // 连接稳定性
      stability: {
        uptime: stats.uptime,
        uptimeFormatted: stats.uptimeFormatted,
        reconnectAttempts: stats.reconnectAttempts,
        totalConnections: stats.totalConnections,
        errorRate:
          stats.errorStats.totalErrors / Math.max(1, stats.totalConnections),
      },

      // 媒体质量
      mediaQuality: {
        video: {
          resolution: stats.media.video.resolution,
          frameRate: stats.media.video.frameRate,
          bitrate: stats.media.video.bitrateFormatted,
          droppedFrames: stats.media.video.framesDropped,
          status: stats.media.video.framesDropped < 10 ? "good" : "poor",
        },
        audio: {
          bitrate: stats.media.audio.bitrateFormatted,
          level: stats.media.audio.audioLevel,
          status: "good", // 音频通常比较稳定
        },
      },

      // 网络信息
      networkInfo: {
        connectionType: stats.network.connectionType,
        localCandidate: stats.network.localCandidateType,
        remoteCandidate: stats.network.remoteCandidateType,
        transport: stats.network.transportType,
      },

      // 时间戳
      timestamp: stats.lastUpdated,
      lastUpdated: stats.lastUpdatedFormatted,
    };
  }

  /**
   * 获取性能指标（与现有监控接口兼容）
   */
  getPerformanceMetrics() {
    const stats = this.getConnectionStats();

    return {
      // 连接时间和性能指标
      connectionTime: stats.connectTime,
      uptime: stats.uptime,
      uptimeSeconds: Math.floor(stats.uptime / 1000),

      // 网络性能
      latency: stats.performanceMetrics.averageRtt * 1000, // 转换为毫秒
      currentLatency: stats.currentRoundTripTime * 1000,
      jitter: stats.jitter * 1000,
      packetLossRate: stats.performanceMetrics.packetLossRate,

      // 带宽
      incomingBitrate: stats.availableIncomingBitrate,
      outgoingBitrate: stats.availableOutgoingBitrate,
      incomingBitrateMbps: parseFloat(
        stats.performanceMetrics.bitrateMbps.incoming
      ),
      outgoingBitrateMbps: parseFloat(
        stats.performanceMetrics.bitrateMbps.outgoing
      ),

      // 数据传输
      bytesReceived: stats.bytesReceived,
      bytesSent: stats.bytesSent,
      packetsReceived: stats.packetsReceived,
      packetsSent: stats.packetsSent,
      packetsLost: stats.packetsLost,

      // 连接质量
      qualityScore: stats.qualityScore,
      qualityLevel: stats.connectionQuality,

      // 媒体统计
      videoFrameRate: stats.videoStats.frameRate,
      videoResolution: `${stats.videoStats.frameWidth}x${stats.videoStats.frameHeight}`,
      videoFramesReceived: stats.videoStats.framesReceived,
      videoFramesDecoded: stats.videoStats.framesDecoded,
      videoFramesDropped: stats.videoStats.framesDropped,

      // 连接信息
      connectionType: this._getConnectionType(),
      iceConnectionState: this.iceConnectionState,
      signalingState: this.signalingState,
      connectionState: this.connectionState,

      // 错误统计
      totalErrors: stats.errorStats.totalErrors,
      reconnectAttempts: stats.reconnectAttempts,
      totalConnections: stats.totalConnections,

      // 时间戳
      timestamp: Date.now(),
      lastStatsUpdate: this.lastStatsTimestamp,
    };
  }

  /**
   * 重置统计信息
   */
  resetStats() {
    this.logger.info("WebRTC", "重置连接统计信息");

    // 保留基本连接信息，重置累计统计
    const connectTime = this.connectionStats.connectTime;
    const totalConnections = this.connectionStats.totalConnections;

    this.connectionStats = {
      connectTime: connectTime,
      disconnectTime: null,
      totalConnections: totalConnections,
      reconnectAttempts: 0,
      connectionDuration: 0,
      bytesReceived: 0,
      bytesSent: 0,
      packetsReceived: 0,
      packetsSent: 0,
      packetsLost: 0,
      jitter: 0,
      roundTripTime: 0,
      availableIncomingBitrate: 0,
      availableOutgoingBitrate: 0,
      currentRoundTripTime: 0,
      totalRoundTripTime: 0,
      roundTripTimeMeasurements: 0,
      connectionQuality: "unknown",
      qualityScore: 0,
      videoStats: {
        framesReceived: 0,
        framesDecoded: 0,
        framesDropped: 0,
        frameWidth: 0,
        frameHeight: 0,
        frameRate: 0,
        bitrate: 0,
      },
      audioStats: {
        samplesReceived: 0,
        samplesDecoded: 0,
        audioLevel: 0,
        bitrate: 0,
      },
      networkStats: {
        candidatePairs: 0,
        selectedCandidatePair: null,
        localCandidateType: "unknown",
        remoteCandidateType: "unknown",
        transportType: "unknown",
      },
    };

    // 重置时间戳
    this.lastStatsTimestamp = 0;
    this._lastBytes = 0;

    // 清除错误历史
    this.clearErrorHistory();

    // 触发统计重置事件
    this.eventBus?.emit("webrtc:stats-reset", {
      timestamp: Date.now(),
    });
  }

  /**
   * 重置重试状态
   * @private
   */
  _resetRetryState() {
    const previousRetryCount = this.retryCount;
    this.retryCount = 0;
    this._clearRetryTimer();

    if (previousRetryCount > 0) {
      this.logger.info("WebRTC", "连接成功，重试状态已重置", {
        previousRetryCount: previousRetryCount,
        totalReconnectAttempts: this.connectionStats.reconnectAttempts,
      });

      // 触发重试状态重置事件
      this.eventBus?.emit("webrtc:retry-state-reset", {
        previousRetryCount: previousRetryCount,
        totalReconnectAttempts: this.connectionStats.reconnectAttempts,
      });
    }
  }

  /**
   * 处理接收到的 Offer
   * @private
   */
  async _handleOffer(sdp) {
    if (!this.pc) {
      this.logger.warn("WebRTC", "接收到 Offer 但 PeerConnection 未创建");
      return;
    }

    try {
      this.logger.info("WebRTC", "处理接收到的 Offer", {
        type: sdp.type,
        sdpLength: sdp.sdp?.length || 0,
      });

      // 如果已经是 RTCSessionDescription 对象，直接使用；否则创建新的
      const sessionDescription =
        sdp instanceof RTCSessionDescription
          ? sdp
          : new RTCSessionDescription({
              type: sdp.type,
              sdp: sdp.sdp,
            });

      // 设置远程描述
      await this.pc.setRemoteDescription(sessionDescription);

      // 创建并发送 answer
      const answer = await this.pc.createAnswer();
      await this.pc.setLocalDescription(answer);

      await this._sendAnswer(answer);

      this.logger.info("WebRTC", "Answer 已创建并发送");
    } catch (error) {
      this.logger.error("WebRTC", "处理 Offer 失败", error);
      throw error;
    }
  }

  /**
   * 处理接收到的 Answer
   * @private
   */
  async _handleAnswer(sdp) {
    if (!this.pc) {
      this.logger.warn("WebRTC", "接收到 Answer 但 PeerConnection 未创建");
      return;
    }

    try {
      this.logger.info("WebRTC", "处理接收到的 Answer", {
        type: sdp.type,
        sdpLength: sdp.sdp?.length || 0,
      });

      // 如果已经是 RTCSessionDescription 对象，直接使用；否则创建新的
      const sessionDescription =
        sdp instanceof RTCSessionDescription
          ? sdp
          : new RTCSessionDescription({
              type: sdp.type,
              sdp: sdp.sdp,
            });

      // 设置远程描述
      await this.pc.setRemoteDescription(sessionDescription);

      this.logger.info("WebRTC", "远程描述已设置");
    } catch (error) {
      this.logger.error("WebRTC", "处理 Answer 失败", error);
      throw error;
    }
  }

  /**
   * 处理接收到的 ICE 候选
   * @private
   */
  async _handleIceCandidate(candidate) {
    if (!this.pc) {
      this.logger.warn("WebRTC", "接收到 ICE 候选但 PeerConnection 未创建");
      return;
    }

    try {
      if (candidate && candidate.candidate) {
        this.logger.debug("WebRTC", "添加 ICE 候选", {
          candidate: candidate.candidate,
          sdpMLineIndex: candidate.sdpMLineIndex,
          sdpMid: candidate.sdpMid,
        });

        // 如果已经是 RTCIceCandidate 对象，直接使用；否则创建新的
        const iceCandidate =
          candidate instanceof RTCIceCandidate
            ? candidate
            : new RTCIceCandidate({
                candidate: candidate.candidate,
                sdpMLineIndex: candidate.sdpMLineIndex,
                sdpMid: candidate.sdpMid,
              });

        await this.pc.addIceCandidate(iceCandidate);
        this.logger.debug("WebRTC", "ICE 候选已添加");
      } else {
        this.logger.info("WebRTC", "接收到空 ICE 候选，候选收集完成");
      }
    } catch (error) {
      this.logger.error("WebRTC", "处理 ICE 候选失败", error);
      // ICE 候选错误通常不是致命的，继续处理
    }
  }

  /**
   * 处理接收到的 Offer（公共接口）
   * 这是对私有方法 _handleOffer 的公共包装
   */
  async handleOffer(sdp) {
    this.logger.debug("WebRTC", "公共接口：处理 Offer", { type: sdp?.type });
    return await this._handleOffer(sdp);
  }

  /**
   * 处理接收到的 ICE 候选（公共接口）
   * 这是对私有方法 _handleIceCandidate 的公共包装
   */
  async handleIceCandidate(candidate) {
    this.logger.debug("WebRTC", "公共接口：处理 ICE 候选", { 
      candidate: candidate?.candidate?.substring(0, 50) + "..." 
    });
    return await this._handleIceCandidate(candidate);
  }
}

// 导出 WebRTCManager 类
if (typeof module !== "undefined" && module.exports) {
  module.exports = WebRTCManager;
} else if (typeof window !== "undefined") {
  window.WebRTCManager = WebRTCManager;
}

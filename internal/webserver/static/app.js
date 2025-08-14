/**
 * 应用主入口文件 - 模块协调器
 * 负责模块的动态加载、初始化、生命周期管理和协调
 */

class ApplicationManager {
  constructor() {
    // 应用状态
    this.state = {
      phase: "initializing", // initializing, loading, ready, running, error, shutdown
      startTime: Date.now(),
      modules: new Map(),
      dependencies: new Map(),
      errors: [],
    };

    // 核心模块
    this.core = {
      eventBus: null,
      config: null,
      logger: null,
    };

    // 功能模块
    this.modules = {
      signaling: null,
      webrtc: null,
      input: null,
      ui: null,
      monitoring: null,
    };

    // 模块配置
    this.moduleConfig = {
      signaling: {
        dependencies: ["eventBus", "config"],
        required: true,
        retryCount: 3,
      },
      webrtc: {
        dependencies: ["eventBus", "config", "signaling"],
        required: true,
        retryCount: 3,
      },
      input: {
        dependencies: ["eventBus", "config"],
        required: false,
        retryCount: 2,
      },
      ui: {
        dependencies: ["eventBus", "config"],
        required: true,
        retryCount: 3,
      },
      monitoring: {
        dependencies: ["eventBus", "config"],
        required: false,
        retryCount: 1,
      },
    };

    // 性能监控
    this.performance = {
      initTime: 0,
      moduleLoadTimes: new Map(),
      memoryUsage: 0,
    };

    // 错误处理
    this.errorHandler = this.createErrorHandler();

    // 绑定方法
    this.handleUnhandledRejection = this.handleUnhandledRejection.bind(this);
    this.handleError = this.handleError.bind(this);
    this.handleBeforeUnload = this.handleBeforeUnload.bind(this);
  }

  /**
   * 启动应用
   */
  async start() {
    const startTime = Date.now();
    
    try {
      Logger.pushContext('ApplicationManager');
      Logger.info("开始启动应用", { startTime });
      this.state.phase = "loading";

      // 设置全局错误处理
      Logger.pushContext('GlobalErrorHandling');
      this.setupGlobalErrorHandling();
      Logger.popContext();

      // 初始化核心模块
      Logger.pushContext('CoreInitialization');
      await this.initializeCore();
      Logger.popContext();

      // 加载功能模块
      Logger.pushContext('ModuleLoading');
      await this.loadModules();
      Logger.popContext();

      // 设置模块间通信
      Logger.pushContext('ModuleCommunication');
      this.setupModuleCommunication();
      Logger.popContext();

      // 启动模块
      Logger.pushContext('ModuleStartup');
      await this.startModules();
      Logger.popContext();

      // 设置兼容性层
      Logger.pushContext('CompatibilityLayer');
      this.setupCompatibilityLayer();
      Logger.popContext();

      // 应用就绪
      this.state.phase = "ready";
      this.performance.initTime = Date.now() - this.state.startTime;

      Logger.logPerformance('app-startup', this.performance.initTime, 'ms', {
        phase: 'startup-complete'
      });

      Logger.info("应用启动完成", {
        initTime: this.performance.initTime,
        modules: Array.from(this.state.modules.keys())
      });

      // 开始运行
      await this.run();
      
      Logger.popContext();
    } catch (error) {
      this.state.phase = "error";
      this.state.errors.push(error);
      
      Logger.error("应用启动失败", error, {
        phase: this.state.phase,
        startupTime: Date.now() - startTime,
        loadedModules: Array.from(this.state.modules.keys())
      });
      
      Logger.popContext();
      throw error;
    }
  }

  /**
   * 初始化核心模块
   */
  async initializeCore() {
    console.log("ApplicationManager: 初始化核心模块...");

    // 创建事件总线
    this.core.eventBus = new EventBus();

    // 创建配置管理器
    this.core.config = new ConfigManager(this.core.eventBus);
    await this.core.config.loadConfig();

    // 初始化和设置Logger
    Logger.init();
    Logger.setEventBus(this.core.eventBus);
    Logger.setLevel(this.core.config.get("ui.logLevel", "info"));
    Logger.setMaxEntries(this.core.config.get("ui.maxLogEntries", 1000));
    
    // 检查是否启用调试模式
    const debugMode = this.core.config.get("debug.enabled", false) || 
                     new URLSearchParams(window.location.search).get('debug') === 'true';
    Logger.setDebugMode(debugMode);
    
    this.core.logger = Logger;

    // 注册核心模块
    this.state.modules.set("eventBus", {
      instance: this.core.eventBus,
      status: "ready",
    });
    this.state.modules.set("config", {
      instance: this.core.config,
      status: "ready",
    });
    this.state.modules.set("logger", {
      instance: this.core.logger,
      status: "ready",
    });

    Logger.info("ApplicationManager: 核心模块初始化完成");
  }

  /**
   * 加载功能模块
   */
  async loadModules() {
    Logger.info("ApplicationManager: 开始加载功能模块...");

    // 按依赖顺序加载模块
    const loadOrder = this.calculateLoadOrder();

    // 串行加载模块以确保依赖关系正确
    for (const moduleName of loadOrder) {
      try {
        await this.loadModule(moduleName);
      } catch (error) {
        // 如果是非必需模块，记录错误但继续加载其他模块
        if (!this.moduleConfig[moduleName].required) {
          Logger.warn(
            `ApplicationManager: 非必需模块 ${moduleName} 加载失败，继续加载其他模块`,
            error
          );
        } else {
          // 必需模块加载失败，抛出错误
          throw error;
        }
      }
    }

    Logger.info("ApplicationManager: 功能模块加载完成");
  }

  /**
   * 加载单个模块
   */
  async loadModule(moduleName) {
    const startTime = Date.now();
    const config = this.moduleConfig[moduleName];

    try {
      Logger.debug(`ApplicationManager: 加载模块 ${moduleName}...`);

      // 检查依赖
      this.checkDependencies(moduleName);

      // 创建模块实例
      let moduleInstance;
      switch (moduleName) {
        case "signaling":
          moduleInstance = new SignalingManager(
            this.core.eventBus,
            this.core.config
          );
          break;
        case "webrtc":
          moduleInstance = new WebRTCManager(
            this.core.eventBus,
            this.core.config
          );
          break;
        case "input":
          moduleInstance = new InputManager(
            this.core.eventBus,
            this.core.config
          );
          break;
        case "ui":
          moduleInstance = new UIManager(this.core.eventBus, this.core.config);
          break;
        case "monitoring":
          moduleInstance = new MonitoringManager(
            this.core.eventBus,
            this.core.config
          );
          break;
        default:
          throw new Error(`Unknown module: ${moduleName}`);
      }

      // 注册模块
      this.modules[moduleName] = moduleInstance;
      this.state.modules.set(moduleName, {
        instance: moduleInstance,
        status: "ready",
        loadTime: Date.now() - startTime,
      });

      this.performance.moduleLoadTimes.set(moduleName, Date.now() - startTime);

      Logger.debug(
        `ApplicationManager: 模块 ${moduleName} 加载完成 (${
          Date.now() - startTime
        }ms)`
      );
    } catch (error) {
      Logger.error(`ApplicationManager: 模块 ${moduleName} 加载失败:`, error);

      this.state.modules.set(moduleName, {
        instance: null,
        status: "error",
        error: error.message,
      });

      if (config.required) {
        throw error;
      }
    }
  }

  /**
   * 计算模块加载顺序
   */
  calculateLoadOrder() {
    const order = [];
    const visited = new Set();
    const visiting = new Set();

    const visit = (moduleName) => {
      if (visiting.has(moduleName)) {
        throw new Error(`Circular dependency detected: ${moduleName}`);
      }

      if (visited.has(moduleName)) {
        return;
      }

      visiting.add(moduleName);

      const config = this.moduleConfig[moduleName];
      if (config && config.dependencies) {
        for (const dep of config.dependencies) {
          // 只处理功能模块之间的依赖，核心模块已经在initializeCore中加载
          if (this.moduleConfig[dep]) {
            visit(dep);
          }
        }
      }

      visiting.delete(moduleName);
      visited.add(moduleName);
      order.push(moduleName);
    };

    for (const moduleName of Object.keys(this.moduleConfig)) {
      if (!visited.has(moduleName)) {
        visit(moduleName);
      }
    }

    Logger.debug("ApplicationManager: 模块加载顺序:", order);
    return order;
  }

  /**
   * 检查模块依赖
   */
  checkDependencies(moduleName) {
    const config = this.moduleConfig[moduleName];
    if (!config || !config.dependencies) {
      return;
    }

    for (const dep of config.dependencies) {
      // 检查核心模块
      if (["eventBus", "config", "logger"].includes(dep)) {
        const depModule = this.state.modules.get(dep);
        if (!depModule || depModule.status !== "ready") {
          throw new Error(
            `Module ${moduleName} depends on ${dep}, but ${dep} is not ready`
          );
        }
      }
      // 检查功能模块
      else if (this.moduleConfig[dep]) {
        const depModule = this.state.modules.get(dep);
        if (!depModule || depModule.status !== "ready") {
          throw new Error(
            `Module ${moduleName} depends on ${dep}, but ${dep} is not ready`
          );
        }
      } else {
        Logger.warn(`ApplicationManager: 未知的依赖模块: ${dep}`);
      }
    }
  }

  /**
   * 设置模块间通信
   */
  setupModuleCommunication() {
    Logger.info("ApplicationManager: 设置模块间通信...");

    // WebRTC和信令通信
    this.core.eventBus.on("signaling:offer", async (data) => {
      try {
        if (this.modules.webrtc) {
          Logger.logConnection('offer-received', {
            sdpType: data.type,
            sdpLength: data.sdp ? data.sdp.length : 0
          }, { phase: 'webrtc-negotiation' });

          // 更新UI状态
          if (this.modules.ui) {
            this.modules.ui.updateConnectionStatus("connecting", {
              message: "处理WebRTC offer...",
            });
          }

          const answer = await this.modules.webrtc.handleOffer(data);
          await this.modules.signaling.send("answer", answer);

          Logger.logConnection('answer-sent', {
            sdpType: answer.type,
            sdpLength: answer.sdp ? answer.sdp.length : 0
          }, { phase: 'webrtc-negotiation' });

          // 更新UI状态
          if (this.modules.ui) {
            this.modules.ui.updateConnectionStatus("connecting", {
              message: "已发送answer，等待连接建立...",
            });
          }
        }
      } catch (error) {
        Logger.error("ApplicationManager: 处理offer失败", error);

        // 更新UI显示错误
        if (this.modules.ui) {
          this.modules.ui.updateConnectionStatus("error", {
            error: `处理offer失败: ${error.message}`,
          });
        }

        // 触发错误恢复
        this._handleWebRTCError("offer-processing-failed", error);
      }
    });

    this.core.eventBus.on("signaling:ice-candidate", async (data) => {
      try {
        if (this.modules.webrtc) {
          Logger.debug("ApplicationManager: 处理ICE候选");
          await this.modules.webrtc.handleIceCandidate(data);
        }
      } catch (error) {
        Logger.error("ApplicationManager: 处理ICE候选失败", error);

        // 触发错误恢复
        this._handleWebRTCError("ice-candidate-failed", error);
      }
    });

    this.core.eventBus.on("webrtc:ice-candidate-generated", async (data) => {
      try {
        if (this.modules.signaling) {
          Logger.debug("ApplicationManager: 发送ICE候选到服务器");
          await this.modules.signaling.send("ice-candidate", data.candidate);
        }
      } catch (error) {
        Logger.error("ApplicationManager: 发送ICE候选失败", error);

        // 触发错误恢复
        this._handleSignalingError("ice-candidate-send-failed", error);
      }
    });

    // 新增：answer-ack和ice-ack处理
    this.core.eventBus.on("signaling:answer-ack", (data) => {
      Logger.info("ApplicationManager: Answer已被服务器确认", data);

      // 更新连接状态跟踪
      this._updateConnectionPhase("answer-acknowledged");

      // 更新UI状态
      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("connecting", {
          message: "Answer已确认，建立连接中...",
        });
      }
    });

    this.core.eventBus.on("signaling:ice-ack", (data) => {
      Logger.info("ApplicationManager: ICE候选已被服务器确认", data);

      // 更新ICE收集状态
      this._updateConnectionPhase("ice-acknowledged");

      // 可以在这里添加ICE收集完成的逻辑
      if (this.connectionState && this.connectionState.iceGatheringComplete) {
        Logger.info("ApplicationManager: ICE收集和确认都已完成");
      }
    });

    // 视频轨道接收处理
    this.core.eventBus.on("webrtc:track-received", (data) => {
      Logger.info(`ApplicationManager: 接收到${data.kind}轨道`, data);

      if (data.kind === "video") {
        // 更新连接状态
        this._updateConnectionPhase("video-track-received");

        // 延迟获取视频信息，等待元数据加载
        setTimeout(() => {
          const videoInfo = this._getVideoInfo();

          if (this.modules.ui) {
            this.modules.ui.updateConnectionStatus("connected", {
              message: "视频轨道已连接",
              videoInfo: videoInfo,
            });
          }

          Logger.info("ApplicationManager: 视频轨道信息已更新", videoInfo);
        }, 500);
      } else if (data.kind === "audio") {
        Logger.info("ApplicationManager: 音频轨道已接收");

        // 更新连接状态
        this._updateConnectionPhase("audio-track-received");
      }

      // 检查是否所有预期的轨道都已接收
      this._checkAllTracksReceived();
    });

    // 视频播放事件处理
    this.core.eventBus.on("webrtc:video-playing", () => {
      Logger.info("ApplicationManager: 视频开始播放");
      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("connected", {
          message: "视频播放中",
        });
      }
    });

    // 视频自动播放失败处理
    this.core.eventBus.on("webrtc:video-autoplay-failed", (data) => {
      Logger.warn("ApplicationManager: 视频自动播放失败", data);
      if (data.needsUserInteraction && this.modules.ui) {
        this.modules.ui.showPlayButton();
      }
    });

    // 连接健康状态处理
    this.core.eventBus.on("webrtc:connection-unhealthy", (data) => {
      Logger.warn("ApplicationManager: WebRTC连接不健康", data);
      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("error", {
          error: `连接状态异常: ${data.connectionState}`,
        });
      }
    });

    // 媒体轨道结束处理
    this.core.eventBus.on("webrtc:media-track-ended", (data) => {
      Logger.warn("ApplicationManager: 媒体轨道结束，尝试重连", data);

      // 更新连接状态
      this._updateConnectionPhase("media-track-ended");

      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("connecting", {
          message: "媒体流中断，正在重连...",
        });
      }

      // 启动自动重连逻辑
      this._startMediaReconnection(data);
    });

    // 添加更多WebRTC事件处理
    this.core.eventBus.on("webrtc:video-metadata-loaded", (data) => {
      Logger.info("ApplicationManager: 视频元数据已加载", data);

      // 更新视频信息显示
      if (this.modules.ui) {
        this.modules.ui.updateVideoInfo({
          width: data.width,
          height: data.height,
          codec: "H264", // 可以从其他地方获取
        });
      }
    });

    this.core.eventBus.on("webrtc:ice-gathering-complete", (data) => {
      Logger.info("ApplicationManager: ICE收集完成", data);
      this._updateConnectionPhase("ice-gathering-complete");
    });

    this.core.eventBus.on("webrtc:connection-state-change", (data) => {
      Logger.info(`ApplicationManager: WebRTC连接状态变化 - ${data.state}`);

      // 根据连接状态更新UI
      switch (data.state) {
        case "connecting":
          if (this.modules.ui) {
            this.modules.ui.updateConnectionStatus("connecting", {
              message: "WebRTC连接建立中...",
            });
          }
          break;
        case "connected":
          if (this.modules.ui) {
            this.modules.ui.updateConnectionStatus("connected", {
              message: "WebRTC连接已建立",
            });
          }
          break;
        case "disconnected":
        case "failed":
          if (this.modules.ui) {
            this.modules.ui.updateConnectionStatus("error", {
              error: `WebRTC连接${data.state === "failed" ? "失败" : "断开"}`,
            });
          }
          break;
      }
    });

    // 添加信令事件处理
    this.core.eventBus.on("signaling:reconnecting", (data) => {
      Logger.info(
        `ApplicationManager: 信令重连中 (${data.attempt}/${data.maxAttempts})`
      );

      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("connecting", {
          message: `重连中 (${data.attempt}/${data.maxAttempts})...`,
        });
      }
    });

    this.core.eventBus.on("signaling:max-reconnect-reached", () => {
      Logger.error("ApplicationManager: 信令达到最大重连次数");

      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("error", {
          error: "连接失败，已达到最大重连次数",
        });
      }
    });

    // 添加统计更新处理
    this.core.eventBus.on("webrtc:stats-updated", (stats) => {
      Logger.debug("ApplicationManager: WebRTC统计已更新", stats);

      // 更新连接质量显示
      if (this.modules.ui && stats) {
        this.modules.ui.updateConnectionQuality({
          rtt: stats.rtt || 0,
          packetsLost: stats.packetsLost || 0,
          jitter: stats.jitter || 0,
        });
      }
    });

    // 状态同步
    this.core.eventBus.on("signaling:connected", () => {
      this.updateModuleState("signaling", "connected");

      // 更新UI状态
      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("connecting", {
          message: "信令已连接，等待媒体流...",
        });
      }
    });

    this.core.eventBus.on("signaling:disconnected", () => {
      this.updateModuleState("signaling", "disconnected");

      // 重置连接状态
      this.connectionState = null;

      // 更新UI状态
      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("disconnected", {
          message: "信令连接已断开",
        });
      }
    });

    this.core.eventBus.on("webrtc:connected", () => {
      this.updateModuleState("webrtc", "connected");
    });

    this.core.eventBus.on("webrtc:disconnected", () => {
      this.updateModuleState("webrtc", "disconnected");
    });

    // 添加错误事件处理
    this.core.eventBus.on("signaling:error", async (data) => {
      await this._handleSignalingError(
        "connection-failed",
        new Error(data.error || "Signaling error")
      );
    });

    this.core.eventBus.on("webrtc:connection-failed", async () => {
      await this._handleWebRTCError(
        "connection-failed",
        new Error("WebRTC connection failed")
      );
    });

    this.core.eventBus.on("webrtc:ice-connection-failed", async () => {
      await this._handleWebRTCError(
        "ice-connection-failed",
        new Error("ICE connection failed")
      );
    });

    this.core.eventBus.on("webrtc:video-autoplay-failed", async (data) => {
      await this.errorHandler.handleError(
        "media",
        "video-autoplay-failed",
        data.error || new Error("Video autoplay failed"),
        data
      );
    });

    this.core.eventBus.on("webrtc:video-error", async (data) => {
      await this.errorHandler.handleError(
        "media",
        "video-decode-error",
        data.error || new Error("Video decode error")
      );
    });

    this.core.eventBus.on("webrtc:media-flow-stalled", async () => {
      await this.errorHandler.handleError(
        "media",
        "stream-ended",
        new Error("Media flow stalled")
      );
    });

    // 连接成功时重置重试计数器
    this.core.eventBus.on("webrtc:connected", () => {
      this.updateModuleState("webrtc", "connected");

      // 重置所有重试计数器
      this.errorHandler.resetAllRetryCounters();
      Logger.info("ApplicationManager: 连接成功，重试计数器已重置");
    });

    this.core.eventBus.on("signaling:connected", () => {
      // 重置信令相关的重试计数器
      this.errorHandler.resetRetryCounter("signaling", "connection-failed");
      this.errorHandler.resetRetryCounter("signaling", "message-timeout");
    });

    // 添加UI事件处理
    this.core.eventBus.on("ui:request-reconnect", async (data) => {
      Logger.info("ApplicationManager: UI请求重新连接", data);

      try {
        // 根据原因选择重连策略
        if (
          data.reason === "no-video-source" ||
          data.reason === "network-error"
        ) {
          await this._reconnectMedia();
        } else {
          // 通用重连
          await this.errorHandler.handleError(
            "webrtc",
            "connection-failed",
            new Error(data.reason)
          );
        }
      } catch (error) {
        Logger.error("ApplicationManager: UI请求的重连失败", error);
      }
    });

    this.core.eventBus.on("ui:request-video-recovery", async (data) => {
      Logger.info("ApplicationManager: UI请求视频恢复", data);

      try {
        // 尝试视频恢复
        await this.errorHandler.handleError(
          "media",
          "video-decode-error",
          data.error
        );
      } catch (error) {
        Logger.error("ApplicationManager: 视频恢复失败", error);
      }
    });

    this.core.eventBus.on("ui:video-play-failed", async (data) => {
      Logger.warn("ApplicationManager: UI视频播放失败", data);

      // 根据错误类型处理
      if (data.errorType === "autoplay-blocked") {
        // 自动播放被阻止，不需要特殊处理，UI已经显示播放按钮
        Logger.info("ApplicationManager: 自动播放被阻止，等待用户交互");
      } else {
        // 其他播放错误，尝试恢复
        await this.errorHandler.handleError(
          "media",
          "video-decode-error",
          data.error
        );
      }
    });

    this.core.eventBus.on("ui:video-play-success", () => {
      Logger.info("ApplicationManager: 视频播放成功");

      // 重置媒体相关的重试计数器
      this.errorHandler.resetRetryCounter("media", "video-autoplay-failed");
      this.errorHandler.resetRetryCounter("media", "video-decode-error");
    });

    // 添加连接监控事件处理
    this.core.eventBus.on("webrtc:health-check-completed", (healthData) => {
      Logger.debug("ApplicationManager: 连接健康检查完成", healthData);

      // 更新UI中的连接质量显示
      if (this.modules.ui && healthData.stats) {
        this.modules.ui.updateConnectionQuality({
          rtt: healthData.stats.summary.rtt || 0,
          packetsLost: healthData.stats.summary.packetsLost || 0,
          jitter: healthData.stats.summary.jitter || 0,
          frameRate: healthData.stats.summary.frameRate || 0,
          bitrate: healthData.stats.summary.bytesReceived || 0,
        });

        // 更新详细的连接统计信息
        this.modules.ui.updateConnectionStats(healthData);
      }
    });

    this.core.eventBus.on("webrtc:resolution-changed", (data) => {
      Logger.info("ApplicationManager: 视频分辨率变化", data);

      // 更新UI中的视频信息
      if (this.modules.ui) {
        this.modules.ui.updateVideoInfo({
          width: data.newResolution.width,
          height: data.newResolution.height,
          codec: "H264", // 可以从healthData中获取更准确的信息
        });
      }
    });

    this.core.eventBus.on("webrtc:frame-rate-updated", (data) => {
      Logger.debug("ApplicationManager: 帧率更新", data);

      // 如果帧率过低，可以触发警告
      if (data.currentFrameRate < 10 && data.currentFrameRate > 0) {
        Logger.warn(
          `ApplicationManager: 帧率过低 ${data.currentFrameRate.toFixed(1)} fps`
        );
      }
    });

    this.core.eventBus.on("webrtc:quality-updated", (data) => {
      Logger.debug("ApplicationManager: 连接质量更新", data);

      // 更新UI中的质量指示器
      if (this.modules.ui) {
        this.modules.ui.updateConnectionQuality({
          rtt: data.rtt,
          packetsLost: data.packetLoss,
          jitter: data.jitter,
          bitrate: data.bitrate,
        });
      }
    });

    this.core.eventBus.on("webrtc:media-flow-issues", (data) => {
      Logger.warn("ApplicationManager: 媒体流问题检测", data);

      // 根据问题严重程度采取不同的处理策略
      const criticalIssues = data.issues.filter(
        (issue) => issue.severity === "critical"
      );
      const warningIssues = data.issues.filter(
        (issue) => issue.severity === "warning"
      );

      if (criticalIssues.length > 0) {
        // 严重问题，尝试重连
        Logger.error("ApplicationManager: 检测到严重媒体流问题，尝试重连");
        this.errorHandler.handleError(
          "webrtc",
          "media-track-ended",
          new Error("Critical media flow issues")
        );
      } else if (warningIssues.length > 0) {
        // 警告问题，显示给用户但不自动重连
        const warningMessages = warningIssues
          .map((issue) => issue.message)
          .join(", ");
        if (this.modules.ui) {
          this.modules.ui.updateConnectionStatus("connected", {
            message: `连接质量问题: ${warningMessages}`,
          });
        }
      }
    });

    Logger.info("ApplicationManager: 模块间通信设置完成");
  }

  /**
   * 启动模块
   */
  async startModules() {
    Logger.info("ApplicationManager: 启动模块...");

    // UI模块初始化
    if (this.modules.ui) {
      this.modules.ui.initialize();
      this.state.modules.get("ui").status = "running";
    }

    // 设置媒体元素
    if (this.modules.webrtc) {
      const video = document.getElementById("video");
      const audio = document.getElementById("audio");
      if (video && audio) {
        this.modules.webrtc.setMediaElements(video, audio);
      }
      await this.modules.webrtc.initialize();
      this.state.modules.get("webrtc").status = "running";
    }

    // 启用输入处理
    if (this.modules.input) {
      const video = document.getElementById("video");
      const videoContainer = document.getElementById("video-container");
      if (video && videoContainer) {
        this.modules.input.setTargetElement(video, videoContainer);
      }
      this.modules.input.enable();
      this.state.modules.get("input").status = "running";
    }

    // 启动监控
    if (
      this.modules.monitoring &&
      this.core.config.get("monitoring.enabled", true)
    ) {
      this.modules.monitoring.start();
      this.state.modules.get("monitoring").status = "running";
    }

    // 连接信令服务器
    if (this.modules.signaling) {
      await this.modules.signaling.connect();
      this.state.modules.get("signaling").status = "running";
    }

    Logger.info("ApplicationManager: 模块启动完成");
  }

  /**
   * 运行应用
   */
  async run() {
    this.state.phase = "running";
    Logger.info("ApplicationManager: 应用开始运行");

    // 启动定期任务
    this.startPeriodicTasks();

    // 触发应用就绪事件
    this.core.eventBus.emit("app:ready", {
      startTime: this.state.startTime,
      initTime: this.performance.initTime,
      modules: Array.from(this.state.modules.keys()),
    });
  }

  /**
   * 启动定期任务
   */
  startPeriodicTasks() {
    // 统计更新
    setInterval(async () => {
      try {
        await this.updateStats();
      } catch (error) {
        Logger.warn("ApplicationManager: 统计更新失败", error);
      }
    }, 5000);

    // 性能监控
    setInterval(() => {
      this.monitorPerformance();
    }, 10000);

    // 模块健康检查
    setInterval(() => {
      this.checkModuleHealth();
    }, 30000);
  }

  /**
   * 更新统计信息
   */
  async updateStats() {
    try {
      const [statusRes, statsRes] = await Promise.all([
        fetch("/api/status"),
        fetch("/api/stats"),
      ]);

      if (statusRes.ok && statsRes.ok) {
        const status = await statusRes.json();
        const stats = await statsRes.json();

        this.core.eventBus.emit("stats:updated", { status, stats });
      }
    } catch (error) {
      Logger.debug("ApplicationManager: 统计更新失败", error);
    }
  }

  /**
   * 性能监控
   */
  monitorPerformance() {
    if ("memory" in performance) {
      this.performance.memoryUsage =
        performance.memory.usedJSHeapSize / 1024 / 1024;

      // 内存使用过高警告
      if (this.performance.memoryUsage > 100) {
        Logger.warn(
          `ApplicationManager: 内存使用过高 ${this.performance.memoryUsage.toFixed(
            1
          )}MB`
        );
      }
    }

    this.core.eventBus.emit("app:performance", this.performance);
  }

  /**
   * 模块健康检查
   */
  checkModuleHealth() {
    let healthyModules = 0;
    let totalModules = 0;

    this.state.modules.forEach((module, name) => {
      totalModules++;
      if (module.status === "running" || module.status === "ready") {
        healthyModules++;
      } else {
        Logger.warn(
          `ApplicationManager: 模块 ${name} 状态异常: ${module.status}`
        );
      }
    });

    const healthRatio = healthyModules / totalModules;
    if (healthRatio < 0.8) {
      Logger.error(
        `ApplicationManager: 应用健康度过低 ${(healthRatio * 100).toFixed(1)}%`
      );
    }

    this.core.eventBus.emit("app:health-check", {
      healthy: healthyModules,
      total: totalModules,
      ratio: healthRatio,
    });
  }

  /**
   * 更新模块状态
   */
  updateModuleState(moduleName, status) {
    const module = this.state.modules.get(moduleName);
    if (module) {
      module.status = status;
      Logger.debug(
        `ApplicationManager: 模块 ${moduleName} 状态更新为 ${status}`
      );
    }
  }

  /**
   * 设置兼容性层
   */
  setupCompatibilityLayer() {
    Logger.info("ApplicationManager: 设置兼容性层...");

    // 全局应用访问
    window.app = {
      manager: this,
      modules: this.modules,
      eventBus: this.core.eventBus,
      config: this.core.config,

      // 兼容性方法
      startCapture: () => this.startCapture(),
      stopCapture: () => this.stopCapture(),
      getDisplays: () => this.getDisplays(),
      updateStats: () => this.updateStats(),
    };

    // WebSocket兼容性
    Object.defineProperty(window, "ws", {
      get: () => this.modules.signaling || null,
    });

    // PeerConnection兼容性
    Object.defineProperty(window, "pc", {
      get: () => this.modules.webrtc || null,
    });

    // 状态兼容性
    Object.defineProperty(window, "isCapturing", {
      get: () => this.state.isCapturing || false,
      set: (value) => {
        this.state.isCapturing = value;
      },
    });

    Logger.info("ApplicationManager: 兼容性层设置完成");
  }

  /**
   * 设置全局错误处理
   */
  setupGlobalErrorHandling() {
    window.addEventListener("error", this.handleError);
    window.addEventListener(
      "unhandledrejection",
      this.handleUnhandledRejection
    );
    window.addEventListener("beforeunload", this.handleBeforeUnload);
  }

  /**
   * 创建错误处理器
   */
  createErrorHandler() {
    return {
      // 信令错误处理器
      signaling: {
        "connection-failed": {
          strategy: "retry",
          maxRetries: 5,
          backoffMs: 2000,
          maxBackoffMs: 30000,
          action: async (error, attempt) => {
            Logger.warn(
              `ApplicationManager: 信令连接失败，第${attempt}次重试`,
              error
            );

            if (this.modules.ui) {
              this.modules.ui.updateConnectionStatus("connecting", {
                message: `连接失败，正在重试 (${attempt}/5)...`,
              });
            }

            // 等待退避时间
            const backoffTime = Math.min(
              this.errorHandler.signaling["connection-failed"].backoffMs *
                Math.pow(2, attempt - 1),
              this.errorHandler.signaling["connection-failed"].maxBackoffMs
            );
            await this._delay(backoffTime);

            // 重新连接
            if (this.modules.signaling) {
              await this.modules.signaling.connect();
            }
          },
        },

        "message-timeout": {
          strategy: "reconnect",
          maxRetries: 3,
          action: async (error, attempt) => {
            Logger.warn(`ApplicationManager: 信令消息超时，重新连接`, error);

            if (this.modules.ui) {
              this.modules.ui.updateConnectionStatus("connecting", {
                message: "消息超时，重新连接中...",
              });
            }

            await this._restartSignaling();
          },
        },

        "unknown-message": {
          strategy: "log",
          action: (error) => {
            Logger.warn("ApplicationManager: 未知信令消息", error);
            // 只记录，不采取行动
          },
        },
      },

      // WebRTC错误处理器
      webrtc: {
        "connection-failed": {
          strategy: "reset",
          maxRetries: 3,
          backoffMs: 3000,
          action: async (error, attempt) => {
            Logger.warn(`ApplicationManager: WebRTC连接失败，重置连接`, error);

            if (this.modules.ui) {
              this.modules.ui.updateConnectionStatus("connecting", {
                message: `WebRTC连接失败，正在重置 (${attempt}/3)...`,
              });
            }

            // 等待退避时间
            await this._delay(
              this.errorHandler.webrtc["connection-failed"].backoffMs
            );

            // 重置WebRTC连接
            if (this.modules.webrtc) {
              await this.modules.webrtc.reset();
            }

            // 重新请求offer
            if (this.modules.signaling) {
              await this.modules.signaling.send("request-offer", {});
            }
          },
        },

        "ice-connection-failed": {
          strategy: "restart-ice",
          maxRetries: 2,
          action: async (error, attempt) => {
            Logger.warn(`ApplicationManager: ICE连接失败，重启ICE`, error);

            if (this.modules.ui) {
              this.modules.ui.updateConnectionStatus("connecting", {
                message: "ICE连接失败，正在重启...",
              });
            }

            // 重启ICE收集
            if (this.modules.webrtc && this.modules.webrtc.pc) {
              try {
                await this.modules.webrtc.pc.restartIce();
              } catch (restartError) {
                Logger.error("ApplicationManager: ICE重启失败", restartError);
                // 如果ICE重启失败，回退到完全重置
                await this.errorHandler.webrtc["connection-failed"].action(
                  error,
                  attempt
                );
              }
            }
          },
        },

        "media-track-ended": {
          strategy: "reconnect",
          maxRetries: 5,
          backoffMs: 2000,
          action: async (error, attempt) => {
            Logger.warn(`ApplicationManager: 媒体轨道结束，重连媒体流`, error);

            if (this.modules.ui) {
              this.modules.ui.updateConnectionStatus("connecting", {
                message: `媒体流中断，正在重连 (${attempt}/5)...`,
              });
            }

            // 等待退避时间
            await this._delay(
              this.errorHandler.webrtc["media-track-ended"].backoffMs * attempt
            );

            // 重连媒体流
            await this._reconnectMedia();
          },
        },
      },

      // 媒体流错误处理器
      media: {
        "video-autoplay-failed": {
          strategy: "user-interaction",
          action: (error) => {
            Logger.warn(
              "ApplicationManager: 视频自动播放失败，需要用户交互",
              error
            );

            if (this.modules.ui) {
              this.modules.ui.showPlayButton();
              this.modules.ui.updateConnectionStatus("connected", {
                message: "视频已连接，点击播放按钮开始观看",
              });
            }
          },
        },

        "video-decode-error": {
          strategy: "codec-fallback",
          maxRetries: 2,
          action: async (error, attempt) => {
            Logger.warn(
              `ApplicationManager: 视频解码错误，尝试编解码器回退`,
              error
            );

            if (this.modules.ui) {
              this.modules.ui.updateConnectionStatus("connecting", {
                message: "视频解码错误，正在尝试其他编解码器...",
              });
            }

            // 这里可以实现编解码器回退逻辑
            await this._tryFallbackCodec();
          },
        },

        "stream-ended": {
          strategy: "reconnect",
          maxRetries: 3,
          backoffMs: 5000,
          action: async (error, attempt) => {
            Logger.warn(`ApplicationManager: 媒体流结束，重启捕获`, error);

            if (this.modules.ui) {
              this.modules.ui.updateConnectionStatus("connecting", {
                message: `媒体流结束，正在重启 (${attempt}/3)...`,
              });
            }

            // 等待退避时间
            await this._delay(
              this.errorHandler.media["stream-ended"].backoffMs
            );

            // 重启捕获
            await this._restartCapture();
          },
        },
      },

      // 通用错误处理方法
      handleError: async (category, errorType, error, context = {}) => {
        const handler = this.errorHandler[category]?.[errorType];

        if (!handler) {
          Logger.error(
            `ApplicationManager: 未知错误类型 ${category}:${errorType}`,
            error
          );
          return false;
        }

        // 记录错误
        this.state.errors.push({
          category,
          errorType,
          error,
          context,
          timestamp: Date.now(),
        });

        // 检查重试次数
        const retryKey = `${category}:${errorType}`;
        if (!this.retryCounters) {
          this.retryCounters = {};
        }

        if (!this.retryCounters[retryKey]) {
          this.retryCounters[retryKey] = 0;
        }

        this.retryCounters[retryKey]++;

        // 检查是否超过最大重试次数
        if (
          handler.maxRetries &&
          this.retryCounters[retryKey] > handler.maxRetries
        ) {
          Logger.error(
            `ApplicationManager: ${category}:${errorType} 超过最大重试次数 (${handler.maxRetries})`
          );

          if (this.modules.ui) {
            this.modules.ui.updateConnectionStatus("error", {
              error: `连接失败，已达到最大重试次数`,
            });
          }

          return false;
        }

        // 执行错误处理动作
        try {
          await handler.action(error, this.retryCounters[retryKey], context);
          return true;
        } catch (actionError) {
          Logger.error(
            `ApplicationManager: 错误处理动作失败 ${category}:${errorType}`,
            actionError
          );
          return false;
        }
      },

      // 重置重试计数器
      resetRetryCounter: (category, errorType) => {
        const retryKey = `${category}:${errorType}`;
        if (this.retryCounters) {
          delete this.retryCounters[retryKey];
        }
      },

      // 重置所有重试计数器
      resetAllRetryCounters: () => {
        this.retryCounters = {};
      },

      // 传统方法保持兼容性
      handleModuleError: (moduleName, error) => {
        Logger.error(`ApplicationManager: 模块 ${moduleName} 错误:`, error);
        this.state.errors.push({
          module: moduleName,
          error,
          timestamp: Date.now(),
        });

        // 尝试重启模块
        this.restartModule(moduleName);
      },

      handleCriticalError: (error) => {
        Logger.error("ApplicationManager: 严重错误:", error);
        this.state.phase = "error";
        this.state.errors.push({
          type: "critical",
          error,
          timestamp: Date.now(),
        });

        // 触发错误恢复
        this.recoverFromError();
      },
    };
  }

  /**
   * 处理未捕获的错误
   */
  handleError(event) {
    Logger.error("ApplicationManager: 未捕获的错误:", event.error);
    this.state.errors.push({
      type: "uncaught",
      error: event.error,
      timestamp: Date.now(),
    });
  }

  /**
   * 处理未处理的Promise拒绝
   */
  handleUnhandledRejection(event) {
    Logger.error("ApplicationManager: 未处理的Promise拒绝:", event.reason);
    this.state.errors.push({
      type: "unhandled-rejection",
      error: event.reason,
      timestamp: Date.now(),
    });
  }

  /**
   * 处理页面卸载
   */
  handleBeforeUnload(event) {
    this.shutdown();
  }

  /**
   * 重启模块
   */
  async restartModule(moduleName) {
    try {
      Logger.info(`ApplicationManager: 重启模块 ${moduleName}...`);

      // 停止模块
      if (
        this.modules[moduleName] &&
        typeof this.modules[moduleName].stop === "function"
      ) {
        await this.modules[moduleName].stop();
      }

      // 重新加载模块
      await this.loadModule(moduleName);

      Logger.info(`ApplicationManager: 模块 ${moduleName} 重启完成`);
    } catch (error) {
      Logger.error(`ApplicationManager: 模块 ${moduleName} 重启失败:`, error);
    }
  }

  /**
   * 错误恢复
   */
  async recoverFromError() {
    try {
      Logger.info("ApplicationManager: 开始错误恢复...");

      // 重置状态
      this.state.phase = "recovering";

      // 重启关键模块
      const criticalModules = ["signaling", "webrtc", "ui"];
      for (const moduleName of criticalModules) {
        if (this.state.modules.get(moduleName)?.status === "error") {
          await this.restartModule(moduleName);
        }
      }

      this.state.phase = "running";
      Logger.info("ApplicationManager: 错误恢复完成");
    } catch (error) {
      Logger.error("ApplicationManager: 错误恢复失败:", error);
      this.state.phase = "error";
    }
  }

  /**
   * 应用关闭
   */
  async shutdown() {
    Logger.info("ApplicationManager: 开始关闭应用...");
    this.state.phase = "shutdown";

    // 停止所有模块
    const shutdownPromises = [];

    if (this.modules.monitoring) {
      shutdownPromises.push(this.modules.monitoring.stop());
    }

    if (this.modules.input) {
      shutdownPromises.push(this.modules.input.disable());
    }

    if (this.modules.webrtc) {
      shutdownPromises.push(this.modules.webrtc.cleanup());
    }

    if (this.modules.signaling) {
      shutdownPromises.push(this.modules.signaling.disconnect());
    }

    await Promise.allSettled(shutdownPromises);

    // 清理事件监听器
    window.removeEventListener("error", this.handleError);
    window.removeEventListener(
      "unhandledrejection",
      this.handleUnhandledRejection
    );
    window.removeEventListener("beforeunload", this.handleBeforeUnload);

    Logger.info("ApplicationManager: 应用关闭完成");
  }

  /**
   * 更新连接阶段状态
   * @private
   */
  _updateConnectionPhase(phase) {
    if (!this.connectionState) {
      this.connectionState = {
        phase: "disconnected",
        offerReceived: false,
        answerSent: false,
        answerAcknowledged: false,
        iceGatheringComplete: false,
        iceAcknowledged: false,
        videoTrackReceived: false,
        audioTrackReceived: false,
        mediaConnected: false,
        lastUpdate: Date.now(),
      };
    }

    // 更新对应的状态
    switch (phase) {
      case "offer-received":
        this.connectionState.offerReceived = true;
        break;
      case "answer-sent":
        this.connectionState.answerSent = true;
        break;
      case "answer-acknowledged":
        this.connectionState.answerAcknowledged = true;
        break;
      case "ice-gathering-complete":
        this.connectionState.iceGatheringComplete = true;
        break;
      case "ice-acknowledged":
        this.connectionState.iceAcknowledged = true;
        break;
      case "video-track-received":
        this.connectionState.videoTrackReceived = true;
        break;
      case "audio-track-received":
        this.connectionState.audioTrackReceived = true;
        break;
      case "media-connected":
        this.connectionState.mediaConnected = true;
        this.connectionState.phase = "connected";
        break;
      case "media-track-ended":
        this.connectionState.mediaConnected = false;
        this.connectionState.phase = "reconnecting";
        break;
    }

    this.connectionState.lastUpdate = Date.now();

    Logger.debug(
      `ApplicationManager: 连接阶段更新 - ${phase}`,
      this.connectionState
    );
    this.core.eventBus?.emit("app:connection-phase-updated", {
      phase,
      state: this.connectionState,
    });
  }

  /**
   * 获取视频信息
   * @private
   */
  _getVideoInfo() {
    const videoInfo = {};

    if (this.modules.webrtc && this.modules.webrtc.videoElement) {
      const video = this.modules.webrtc.videoElement;

      if (video.videoWidth && video.videoHeight) {
        videoInfo.width = video.videoWidth;
        videoInfo.height = video.videoHeight;
      }

      // 尝试获取更多视频信息
      if (video.srcObject) {
        const stream = video.srcObject;
        const videoTracks = stream.getVideoTracks();

        if (videoTracks.length > 0) {
          const track = videoTracks[0];
          const settings = track.getSettings();

          if (settings.frameRate) {
            videoInfo.frameRate = settings.frameRate;
          }

          if (settings.width && settings.height) {
            videoInfo.width = settings.width;
            videoInfo.height = settings.height;
          }
        }
      }
    }

    return videoInfo;
  }

  /**
   * 检查所有轨道是否已接收
   * @private
   */
  _checkAllTracksReceived() {
    if (!this.connectionState) return;

    // 目前只检查视频轨道（音频暂时禁用）
    if (this.connectionState.videoTrackReceived) {
      Logger.info("ApplicationManager: 所有预期轨道已接收");
      this._updateConnectionPhase("media-connected");

      // 更新UI状态
      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("connected", {
          message: "媒体连接已建立",
        });
      }
    }
  }

  /**
   * 处理WebRTC错误
   * @private
   */
  async _handleWebRTCError(errorType, error, context = {}) {
    Logger.error(`ApplicationManager: WebRTC错误 - ${errorType}`, error);

    // 使用新的错误处理系统
    const handled = await this.errorHandler.handleError(
      "webrtc",
      errorType,
      error,
      context
    );

    if (!handled) {
      Logger.error(`ApplicationManager: WebRTC错误处理失败 - ${errorType}`);

      // 如果错误处理失败，显示错误给用户
      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("error", {
          error: `WebRTC错误: ${error.message || errorType}`,
        });
      }
    }
  }

  /**
   * 处理信令错误
   * @private
   */
  async _handleSignalingError(errorType, error, context = {}) {
    Logger.error(`ApplicationManager: 信令错误 - ${errorType}`, error);

    // 使用新的错误处理系统
    const handled = await this.errorHandler.handleError(
      "signaling",
      errorType,
      error,
      context
    );

    if (!handled) {
      Logger.error(`ApplicationManager: 信令错误处理失败 - ${errorType}`);

      // 如果错误处理失败，显示错误给用户
      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("error", {
          error: `信令错误: ${error.message || errorType}`,
        });
      }
    }
  }

  /**
   * 启动媒体重连
   * @private
   */
  _startMediaReconnection(data) {
    Logger.info("ApplicationManager: 启动媒体重连流程", data);

    // 防止重复重连
    if (this.mediaReconnecting) {
      Logger.warn("ApplicationManager: 媒体重连已在进行中");
      return;
    }

    this.mediaReconnecting = true;

    // 延迟重连，给系统一些时间
    setTimeout(async () => {
      try {
        Logger.info("ApplicationManager: 开始媒体重连");

        // 重置WebRTC连接
        if (this.modules.webrtc) {
          await this.modules.webrtc.reset();
        }

        // 重新请求offer
        if (this.modules.signaling) {
          await this.modules.signaling.send("request-offer", {});
        }

        Logger.info("ApplicationManager: 媒体重连请求已发送");
      } catch (error) {
        Logger.error("ApplicationManager: 媒体重连失败", error);

        if (this.modules.ui) {
          this.modules.ui.updateConnectionStatus("error", {
            error: "重连失败，请手动重试",
          });
        }
      } finally {
        this.mediaReconnecting = false;
      }
    }, 2000);
  }

  /**
   * 重试offer请求
   * @private
   */
  _retryOfferRequest() {
    if (
      this.modules.signaling &&
      this.modules.signaling.readyState === WebSocket.OPEN
    ) {
      Logger.info("ApplicationManager: 重试offer请求");

      this.modules.signaling.send("request-offer", {}).catch((error) => {
        Logger.error("ApplicationManager: 重试offer请求失败", error);
      });
    } else {
      Logger.warn("ApplicationManager: 信令连接不可用，无法重试offer请求");
    }
  }

  /**
   * 延迟函数
   * @private
   */
  _delay(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  /**
   * 重启信令连接
   * @private
   */
  async _restartSignaling() {
    try {
      Logger.info("ApplicationManager: 重启信令连接");

      if (this.modules.signaling) {
        // 断开现有连接
        this.modules.signaling.disconnect();

        // 等待一段时间
        await this._delay(1000);

        // 重新连接
        await this.modules.signaling.connect();

        Logger.info("ApplicationManager: 信令连接重启完成");
      }
    } catch (error) {
      Logger.error("ApplicationManager: 重启信令连接失败", error);
      throw error;
    }
  }

  /**
   * 重连媒体流
   * @private
   */
  async _reconnectMedia() {
    try {
      Logger.info("ApplicationManager: 重连媒体流");

      // 重置WebRTC连接
      if (this.modules.webrtc) {
        await this.modules.webrtc.reset();
      }

      // 等待一段时间
      await this._delay(1000);

      // 重新请求offer
      if (this.modules.signaling) {
        await this.modules.signaling.send("request-offer", {});
      }

      Logger.info("ApplicationManager: 媒体流重连请求已发送");
    } catch (error) {
      Logger.error("ApplicationManager: 重连媒体流失败", error);
      throw error;
    }
  }

  /**
   * 尝试回退编解码器
   * @private
   */
  async _tryFallbackCodec() {
    try {
      Logger.info("ApplicationManager: 尝试编解码器回退");

      // 这里可以实现编解码器回退逻辑
      // 例如从H264回退到VP8或VP9

      // 重置连接并请求新的offer
      if (this.modules.webrtc) {
        await this.modules.webrtc.reset();
      }

      if (this.modules.signaling) {
        await this.modules.signaling.send("request-offer", {
          preferredCodec: "vp8", // 请求使用VP8编解码器
        });
      }

      Logger.info("ApplicationManager: 编解码器回退请求已发送");
    } catch (error) {
      Logger.error("ApplicationManager: 编解码器回退失败", error);
      throw error;
    }
  }

  /**
   * 重启捕获
   * @private
   */
  async _restartCapture() {
    try {
      Logger.info("ApplicationManager: 重启桌面捕获");

      // 停止当前捕获
      try {
        await fetch("/api/capture/stop", { method: "POST" });
      } catch (stopError) {
        Logger.warn("ApplicationManager: 停止捕获时出错", stopError);
      }

      // 等待一段时间
      await this._delay(2000);

      // 重新启动捕获
      const response = await fetch("/api/capture/start", { method: "POST" });
      const result = await response.json();

      if (response.ok) {
        Logger.info("ApplicationManager: 桌面捕获重启成功");

        // 请求新的WebRTC offer
        if (this.modules.signaling) {
          await this.modules.signaling.send("request-offer", {});
        }
      } else {
        throw new Error(result.error || "重启捕获失败");
      }
    } catch (error) {
      Logger.error("ApplicationManager: 重启捕获失败", error);
      throw error;
    }
  }

  /**
   * 获取应用状态
   */
  getState() {
    return {
      ...this.state,
      performance: this.performance,
      uptime: Date.now() - this.state.startTime,
    };
  }

  /**
   * 业务方法 - 开始捕获
   */
  async startCapture() {
    try {
      if (this.modules.ui) {
        this.modules.ui.updateStatus("connecting", "正在启动...");
      }

      Logger.info("ApplicationManager: 正在启动桌面捕获...");

      const response = await fetch("/api/capture/start", { method: "POST" });
      const result = await response.json();

      if (response.ok) {
        this.state.isCapturing = true;
        Logger.info("ApplicationManager: 桌面捕获已启动");

        // 请求WebRTC offer
        if (this.modules.signaling) {
          await this.modules.signaling.send("request-offer", {});
        }
      } else {
        throw new Error(result.error || "启动失败");
      }
    } catch (error) {
      if (this.modules.ui) {
        this.modules.ui.updateStatus("offline", "启动失败");
      }
      Logger.error("ApplicationManager: 启动捕获失败", error);
      throw error;
    }
  }

  /**
   * 业务方法 - 停止捕获
   */
  async stopCapture() {
    try {
      Logger.info("ApplicationManager: 正在停止桌面捕获...");

      // 清理WebRTC连接
      if (this.modules.webrtc) {
        await this.modules.webrtc.cleanup();
      }

      const response = await fetch("/api/capture/stop", { method: "POST" });

      this.state.isCapturing = false;

      if (this.modules.ui) {
        this.modules.ui.updateStatus("offline", "已停止");
      }

      Logger.info("ApplicationManager: 桌面捕获已停止");
    } catch (error) {
      Logger.error("ApplicationManager: 停止捕获失败", error);
      this.state.isCapturing = false;

      if (this.modules.ui) {
        this.modules.ui.updateStatus("offline", "停止时出错");
      }
    }
  }

  /**
   * 业务方法 - 获取显示器列表
   */
  async getDisplays() {
    try {
      const response = await fetch("/api/displays");
      const displays = await response.json();

      if (response.ok) {
        Logger.info("ApplicationManager: 获取显示器列表成功", displays);
        return displays;
      } else {
        throw new Error(displays.error || "获取显示器列表失败");
      }
    } catch (error) {
      Logger.error("ApplicationManager: 获取显示器列表失败", error);
      throw error;
    }
  }
}

// 创建应用管理器实例
const appManager = new ApplicationManager();

// DOM加载完成后启动应用
document.addEventListener("DOMContentLoaded", async function () {
  try {
    await appManager.start();
  } catch (error) {
    console.error("应用启动失败:", error);

    // 显示错误信息
    const statusText = document.getElementById("status-text");
    if (statusText) {
      statusText.textContent = "应用启动失败";
    }

    const statusIndicator = document.querySelector(".status-indicator");
    if (statusIndicator) {
      statusIndicator.className = "status-indicator status-offline";
    }
  }
});

// 导出应用管理器
window.appManager = appManager;

// ============================================================================
// 兼容性函数 - 为HTML中的onclick事件提供全局函数
// ============================================================================

/**
 * 视频控制函数
 */
function toggleVideoPlay() {
  if (window.appManager && window.appManager.modules.ui) {
    window.appManager.modules.ui.toggleVideoPlay();
  }
}

function toggleFullscreen() {
  if (window.appManager && window.appManager.modules.ui) {
    window.appManager.modules.ui.toggleFullscreen();
  }
}

function toggleAudioMute() {
  if (window.appManager && window.appManager.modules.ui) {
    window.appManager.modules.ui.toggleAudioMute();
  }
}

function setVolume(volume) {
  if (window.appManager && window.appManager.modules.ui) {
    const audio = document.getElementById("audio");
    if (audio) {
      audio.volume = parseFloat(volume);
      Logger.info(`音量设置为: ${Math.round(volume * 100)}%`);
    }
  }
}

/**
 * 应用控制函数
 */
function startCapture() {
  if (window.appManager) {
    window.appManager.startCapture().catch((error) => {
      Logger.error("启动捕获失败:", error);
    });
  }
}

function stopCapture() {
  if (window.appManager) {
    window.appManager.stopCapture().catch((error) => {
      Logger.error("停止捕获失败:", error);
    });
  }
}

function getDisplays() {
  if (window.appManager) {
    window.appManager
      .getDisplays()
      .then((displays) => {
        Logger.info("显示器列表:", displays);
        // 可以在这里添加显示器选择的UI逻辑
      })
      .catch((error) => {
        Logger.error("获取显示器列表失败:", error);
      });
  }
}

/**
 * 监控面板函数
 */
function switchMonitoringTab(tabName) {
  // 隐藏所有面板
  const panels = document.querySelectorAll(".monitoring-panel");
  panels.forEach((panel) => {
    panel.classList.remove("active");
  });

  // 移除所有按钮的active类
  const buttons = document.querySelectorAll(".tab-btn");
  buttons.forEach((btn) => {
    btn.classList.remove("active");
  });

  // 显示选中的面板
  const targetPanel = document.getElementById(`${tabName}-panel`);
  if (targetPanel) {
    targetPanel.classList.add("active");
  }

  // 激活对应的按钮
  const targetButton = document.querySelector(
    `[onclick="switchMonitoringTab('${tabName}')"]`
  );
  if (targetButton) {
    targetButton.classList.add("active");
  }

  Logger.debug(`切换到监控面板: ${tabName}`);
}

/**
 * 日志管理函数
 */
function filterLogs() {
  const level = document.getElementById("log-level")?.value || "all";
  const component = document.getElementById("log-component")?.value || "all";
  const search = document.getElementById("log-search")?.value || "";

  Logger.debug(
    `过滤日志: level=${level}, component=${component}, search=${search}`
  );

  // 这里可以添加实际的日志过滤逻辑
  // 目前只是记录日志，实际的过滤逻辑需要根据具体的日志显示实现
}

function clearLogs() {
  const logContainer = document.getElementById("log-container");
  if (logContainer) {
    logContainer.innerHTML =
      '<div class="log-stats"><span>日志已清空</span></div>';
    Logger.info("日志已清空");
  }
}

function exportLogs() {
  try {
    const logs = Logger.getEntries();
    const logData = {
      timestamp: new Date().toISOString(),
      logs: logs,
      userAgent: navigator.userAgent,
      url: window.location.href,
    };

    const blob = new Blob([JSON.stringify(logData, null, 2)], {
      type: "application/json",
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `logs-${new Date()
      .toISOString()
      .slice(0, 19)
      .replace(/:/g, "-")}.json`;
    link.click();
    URL.revokeObjectURL(url);

    Logger.info("日志已导出");
  } catch (error) {
    Logger.error("导出日志失败:", error);
  }
}

function refreshLogs() {
  Logger.info("刷新日志显示");
  // 这里可以添加刷新日志显示的逻辑
  // 目前只是记录日志
}

/**
 * 系统监控函数（兼容性）
 */
window.systemMonitor = {
  exportMonitoringData() {
    if (window.appManager && window.appManager.modules.monitoring) {
      try {
        window.appManager.modules.monitoring.exportData();
        Logger.info("监控数据已导出");
      } catch (error) {
        Logger.error("导出监控数据失败:", error);
      }
    }
  },

  resetMonitoringData() {
    if (window.appManager && window.appManager.modules.monitoring) {
      try {
        window.appManager.modules.monitoring.resetStats();
        Logger.info("监控数据已重置");
      } catch (error) {
        Logger.error("重置监控数据失败:", error);
      }
    }
  },

  startLogStream() {
    Logger.info("开始实时日志流");
    // 这里可以添加实时日志流的逻辑
  },
};

// 确保所有函数都是全局可访问的
window.toggleVideoPlay = toggleVideoPlay;
window.toggleFullscreen = toggleFullscreen;
window.toggleAudioMute = toggleAudioMute;
window.setVolume = setVolume;
window.startCapture = startCapture;
window.stopCapture = stopCapture;
window.getDisplays = getDisplays;
window.switchMonitoringTab = switchMonitoringTab;
window.filterLogs = filterLogs;
window.clearLogs = clearLogs;
window.exportLogs = exportLogs;
window.refreshLogs = refreshLogs;

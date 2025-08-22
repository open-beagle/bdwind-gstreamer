/**
 * 应用主入口文件 - 核心业务功能
 * 专注于桌面流媒体的核心功能
 */

class ApplicationManager {
  constructor() {
    // 应用状态
    this.state = {
      phase: "initializing",
      startTime: Date.now(),
      isCapturing: false,
      errors: [],
    };

    // 核心组件
    this.eventBus = null;
    this.config = null;
    this.logger = null;

    // 功能模块 - 核心模块
    this.modules = {
      signaling: null,
      webrtc: null,
      ui: null,
    };

    // 媒体元素
    this.videoElement = null;
    this.audioElement = null;

    // 绑定方法
    this.handleUnhandledRejection = this.handleUnhandledRejection.bind(this);
    this.handleError = this.handleError.bind(this);
    this.handleBeforeUnload = this.handleBeforeUnload.bind(this);
    this.updateStats = this.updateStats.bind(this);
  }

  /**
   * 启动应用
   */
  async start() {
    try {
      this.setupGlobalErrorHandling();
      await this.initializeCore();
      await this.initializeModules();
      this.setupEventHandlers();
      await this.connectSignaling();
      this.setupGlobalAccess();
      this.state.phase = "ready";
      this.run();
    } catch (error) {
      this.state.phase = "error";
      this.state.errors.push(error);
      throw error;
    }
  }

  /**
   * 初始化核心组件
   */
  async initializeCore() {
    this.eventBus = new EventBus();
    this.config = new ConfigManager(this.eventBus);
    await this.config.loadConfig();

    Logger.init();
    Logger.setEventBus(this.eventBus);
    Logger.setLevel(this.config.get("ui.logLevel", "warn"));
    Logger.setMaxEntries(this.config.get("ui.maxLogEntries", 500));

    const debugMode =
      this.config.get("debug.enabled", false) ||
      new URLSearchParams(window.location.search).get("debug") === "true";
    Logger.setDebugMode(debugMode);

    this.logger = Logger;

    this.videoElement = document.getElementById("video");
    this.audioElement = document.getElementById("audio");

    if (!this.videoElement) {
      throw new Error("视频元素未找到");
    }
  }

  /**
   * 初始化模块
   */
  async initializeModules() {
    this.modules.signaling = this.createSignalingClient();

    this.modules.webrtc = new WebRTCManager(
      this.modules.signaling,
      this.videoElement,
      1,
      {
        eventBus: this.eventBus,
        config: this.config,
      }
    );

    if (typeof UIManager !== "undefined") {
      this.modules.ui = new UIManager(this.eventBus, this.config);
    }

    if (!this.modules.webrtc || !this.modules.signaling) {
      throw new Error("核心模块创建失败");
    }
  }

  /**
   * 设置事件处理
   */
  setupEventHandlers() {
    if (this.modules.webrtc) {
      this.modules.webrtc.onconnectionstatechange = (state) => {
        if (this.modules.ui) {
          this.updateConnectionStatus(state);
        }
      };

      this.modules.webrtc.ondatachannelopen = () => {
        if (this.modules.ui) {
          this.modules.ui.updateConnectionStatus("connected", {
            message: "数据通道已建立",
          });
        }
        this.eventBus?.emit("webrtc:datachannel-open");
      };

      this.modules.webrtc.onplaystreamrequired = () => {
        if (this.modules.ui) {
          this.modules.ui.showPlayButton();
        }
      };

      this.modules.webrtc.onerror = (error) => {
        this.handleWebRTCError(new Error(error));
      };
    }

    if (this.modules.signaling) {
      this.modules.signaling.ondisconnect = () => {
        if (this.modules.ui) {
          this.modules.ui.updateConnectionStatus("disconnected", {
            message: "信令连接已断开",
          });
        }

        if (this.modules.webrtc) {
          this.modules.webrtc.reset();
        }
      };

      this.modules.signaling.onstatus = (message) => {
        if (this.modules.ui) {
          this.modules.ui.updateConnectionStatus("connecting", {
            message: message,
          });
        }
      };

      this.modules.signaling.onerror = (error) => {
        this.handleSignalingError(new Error(error));
      };
    }
  }

  /**
   * 连接状态更新
   */
  updateConnectionStatus(state) {
    if (!this.modules.ui) return;

    const statusMap = {
      connecting: { status: "connecting", message: "WebRTC连接建立中..." },
      connected: { status: "connected", message: "WebRTC连接已建立" },
      disconnected: { status: "error", message: "WebRTC连接断开" },
      failed: { status: "error", message: "WebRTC连接失败" },
    };

    const statusInfo = statusMap[state];
    if (statusInfo) {
      this.modules.ui.updateConnectionStatus(statusInfo.status, {
        message: statusInfo.message,
      });
    }
  }

  /**
   * 连接信令服务器
   */
  async connectSignaling() {
    if (this.modules.webrtc) {
      this.modules.webrtc.connect();
    }

    if (this.modules.ui) {
      this.modules.ui.initialize();
    }
  }

  /**
   * 处理信令错误
   */
  handleSignalingError(error) {
    const errorMessage = `信令错误: ${error.message || error}`;

    if (
      this.modules.ui &&
      typeof this.modules.ui.updateConnectionStatus === "function"
    ) {
      this.modules.ui.updateConnectionStatus("error", {
        error: errorMessage,
      });
    } else {
      this.updateBasicStatus("error", errorMessage);
    }

    // setTimeout(() => {
    //   if (this.modules.signaling) {
    //     this.modules.signaling.connect().catch(() => {});
    //   }
    // }, 3000);
  }

  /**
   * 处理WebRTC错误
   */
  handleWebRTCError(error) {
    const errorMessage = `WebRTC错误: ${error.message || error}`;

    if (
      this.modules.ui &&
      typeof this.modules.ui.updateConnectionStatus === "function"
    ) {
      this.modules.ui.updateConnectionStatus("error", {
        error: errorMessage,
      });
    } else {
      this.updateBasicStatus("error", errorMessage);
    }

    // setTimeout(() => {
    //   if (this.modules.webrtc) {
    //     this.modules.webrtc.reset();
    //   }
    // }, 3000);
  }

  /**
   * 基本状态更新
   */
  updateBasicStatus(status, message) {
    const statusText = document.getElementById("status-text");
    if (statusText) {
      statusText.textContent = message;
    }

    const statusIndicator = document.querySelector(".status-indicator");
    if (statusIndicator) {
      statusIndicator.className = `status-indicator status-${status}`;
    }
  }

  /**
   * 发送数据通道消息
   */
  sendDataChannelMessage(message) {
    if (
      this.modules.webrtc &&
      typeof this.modules.webrtc.sendDataChannelMessage === "function"
    ) {
      return this.modules.webrtc.sendDataChannelMessage(message);
    }
    return false;
  }

  /**
   * 获取连接统计
   */
  getConnectionStats() {
    if (
      this.modules.webrtc &&
      typeof this.modules.webrtc.getConnectionStats === "function"
    ) {
      return this.modules.webrtc.getConnectionStats();
    }
    return null;
  }

  /**
   * 运行应用
   */
  run() {
    this.state.phase = "running";
    this.startPeriodicTasks();
    this.eventBus.emit("app:ready", {
      startTime: this.state.startTime,
    });
  }

  /**
   * 启动定期任务
   */
  startPeriodicTasks() {
    setInterval(this.updateStats, 10000);
  }

  /**
   * 更新统计信息
   */
  async updateStats() {
    try {
      const [statusRes, statsRes] = await Promise.all([
        fetch("/api/status").catch(() => null),
        fetch("/api/system/stats").catch(() => null),
      ]);

      if (statusRes?.ok && statsRes?.ok) {
        const status = await statusRes.json();
        const stats = await statsRes.json();
        this.eventBus.emit("stats:updated", { status, stats });
      }
    } catch (error) {
      // 静默处理统计更新失败
    }
  }

  /**
   * 设置全局访问层
   */
  setupGlobalAccess() {
    // 全局应用访问
    window.app = {
      manager: this,
      modules: this.modules,
      eventBus: this.eventBus,
      config: this.config,
      playStream: () => this.playStream(),
      updateStats: () => this.updateStats(),
    };

    // WebSocket 全局访问
    Object.defineProperty(window, "ws", {
      get: () => this.modules.signaling || null,
    });

    // PeerConnection 全局访问
    Object.defineProperty(window, "pc", {
      get: () => this.modules.webrtc || null,
    });

    // 状态全局访问
    Object.defineProperty(window, "isCapturing", {
      get: () => this.state.isCapturing || false,
      set: (value) => {
        this.state.isCapturing = value;
      },
    });
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
   * 处理未捕获的错误
   */
  handleError(event) {
    this.state.errors.push({
      type: "uncaught",
      error: event.error,
      timestamp: Date.now(),
    });

    // 记录错误到控制台
    console.error("Uncaught error:", event.error);
  }

  /**
   * 处理未处理的Promise拒绝
   */
  handleUnhandledRejection(event) {
    this.state.errors.push({
      type: "unhandled-rejection",
      error: event.reason,
      timestamp: Date.now(),
    });
  }

  /**
   * 处理页面卸载
   */
  handleBeforeUnload() {
    this.shutdown();
  }

  /**
   * 应用关闭
   */
  async shutdown() {
    this.state.phase = "shutdown";

    if (this.modules.webrtc) {
      try {
        this.modules.webrtc.reset();
      } catch (error) {
        // 静默处理
      }
    }

    if (this.modules.signaling) {
      try {
        this.modules.signaling.disconnect();
      } catch (error) {
        // 静默处理
      }
    }

    window.removeEventListener("error", this.handleError);
    window.removeEventListener(
      "unhandledrejection",
      this.handleUnhandledRejection
    );
    window.removeEventListener("beforeunload", this.handleBeforeUnload);
  }

  /**
   * 创建信令客户端
   */
  createSignalingClient() {
    let serverUrl = this.config?.get("signaling.url");
    if (!serverUrl) {
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      serverUrl = `${protocol}//${window.location.host}/api/signaling`;
    }

    const client = new SignalingClient(
      serverUrl,
      this.config?.get("signaling.peerId", 1),
      {
        eventBus: this.eventBus,
        logger: this.logger || console,
        config: this.config?.get("signaling", {}),
        maxRetries: this.config?.get("signaling.maxRetries", 3),
        retryDelay: this.config?.get("signaling.retryDelay", 3000),
        connectionTimeout: this.config?.get(
          "signaling.connectionTimeout",
          10000
        ),
        heartbeatInterval: this.config?.get(
          "signaling.heartbeatInterval",
          30000
        ),
      }
    );

    this.setupClientCallbacks(client);
    return client;
  }

  /**
   * 设置客户端事件回调
   */
  setupClientCallbacks(client) {
    // 设置事件回调接口
    client.onstatus = (message) => {
      this.eventBus?.emit("signaling:status", { message });
      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("connecting", { message });
      }
    };

    client.onerror = (error) => {
      const message = error.message || error.toString();
      this.eventBus?.emit("signaling:error", { message });
      this.handleSignalingError(error);
    };

    client.onopen = () => {
      this.eventBus?.emit("signaling:connected", {
        peerId: client.peerId,
        server: client.serverUrl,
        protocol: client.getState().protocolMode,
      });

      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("connected", {
          message: "信令连接已建立",
        });
      }
    };

    client.onclose = () => {
      this.eventBus?.emit("signaling:disconnected");
      if (client.ondisconnect) {
        client.ondisconnect();
      }
    };

    client.onsdp = (sdp) => {
      this.eventBus?.emit("signaling:sdp", sdp);
    };

    client.onice = (ice) => {
      this.eventBus?.emit("signaling:ice-candidate", ice);
    };

    // 添加便捷方法
    client.getConnectionState = () => client.getState();
    client.getConnectionStats = () => client.getState();

    // 添加发送方法别名
    client.send = (type, data) => {
      try {
        return client.sendMessage(type, data);
      } catch (error) {
        return Promise.reject(error);
      }
    };

    // 添加重置方法别名
    client.reset = () => {
      client.disconnect();
    };

    // 添加状态属性别名
    Object.defineProperty(client, "state", {
      get: () => client.getState().connectionState,
    });

    Object.defineProperty(client, "retry_count", {
      get: () => client.getState().retryCount,
    });

    Object.defineProperty(client, "peer_id", {
      get: () => client.peerId,
      set: (value) => {
        client.peerId = value;
      },
    });
  }

  /**
   * 获取应用状态
   */
  getState() {
    return {
      ...this.state,
      uptime: Date.now() - this.state.startTime,
    };
  }

  /**
   * 开始捕获
   */
  async playStream() {
    if (!this.modules.webrtc) {
      throw new Error("WebRTC模块未初始化");
    }

    this.modules.webrtc.playStream();
    this.state.isCapturing = true;

    return { success: true };
  }
}

// 创建应用管理器实例
let appManager;

// 应用启动函数
async function startApplication() {
  try {
    appManager = new ApplicationManager();
    await appManager.start();
    window.appManager = appManager;
  } catch (error) {
    console.error("应用启动失败:", error);
  }
}

// 页面加载完成后启动应用
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", startApplication);
} else {
  startApplication();
}

// ============================================================================
// 全局函数支持
// ============================================================================

// 视频控制函数
function toggleVideoPlay() {
  const video = document.getElementById("video");
  if (video) {
    if (video.paused) {
      video.play().catch(() => {});
    } else {
      video.pause();
    }
  }
}

function toggleFullscreen() {
  const video = document.getElementById("video");
  if (video) {
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {});
    } else {
      video.requestFullscreen().catch(() => {});
    }
  }
}

function toggleAudioMute() {
  const video = document.getElementById("video");
  const audio = document.getElementById("audio");
  const muteIcon = document.getElementById("mute-icon");

  if (video) {
    video.muted = !video.muted;
    if (muteIcon) {
      muteIcon.textContent = video.muted ? "🔇" : "🔊";
    }
  }

  if (audio) {
    audio.muted = video ? video.muted : !audio.muted;
  }
}

function setVolume(value) {
  const video = document.getElementById("video");
  const audio = document.getElementById("audio");

  if (video) {
    video.volume = parseFloat(value);
  }

  if (audio) {
    audio.volume = parseFloat(value);
  }
}

function playStream() {
  if (window.appManager) {
    return window.appManager.playStream();
  } else if (window.app && window.app.manager) {
    return window.app.manager.playStream();
  } else {
    console.error("应用管理器未找到");
  }
}

function getDisplays() {
  // 发送获取显示器信息的请求
  if (window.appManager && window.appManager.sendDataChannelMessage) {
    window.appManager.sendDataChannelMessage("get_displays");
  } else {
    console.log("获取显示器信息功能需要建立数据通道连接");
  }
}

// 执行快速测试的全局函数
function executeQuickTests() {
  console.log("快速测试功能");
}

// 系统监控相关函数
function switchMonitoringTab(tabName) {
  // 隐藏所有面板
  const panels = document.querySelectorAll(".monitoring-panel");
  panels.forEach((panel) => panel.classList.remove("active"));

  // 移除所有按钮的active类
  const buttons = document.querySelectorAll(".tab-btn");
  buttons.forEach((btn) => btn.classList.remove("active"));

  // 显示选中的面板
  const targetPanel = document.getElementById(`${tabName}-panel`);
  if (targetPanel) {
    targetPanel.classList.add("active");
  }

  // 激活选中的按钮
  const targetButton = document.querySelector(
    `[onclick="switchMonitoringTab('${tabName}')"]`
  );
  if (targetButton) {
    targetButton.classList.add("active");
  }
}

function filterLogs() {
  // 日志过滤功能的占位符实现
  console.log("日志过滤功能");
}

function clearLogs() {
  const logContainer = document.getElementById("log-container");
  if (logContainer) {
    logContainer.innerHTML =
      '<div class="log-stats"><span>日志已清空</span><span>最后更新: ' +
      new Date().toLocaleTimeString() +
      "</span></div>";
  }
}

function exportLogs() {
  console.log("导出日志功能");
}

function refreshLogs() {
  console.log("刷新日志功能");
}



// 导出全局函数
window.toggleVideoPlay = toggleVideoPlay;
window.toggleFullscreen = toggleFullscreen;
window.toggleAudioMute = toggleAudioMute;
window.setVolume = setVolume;
window.playStream = playStream;
window.getDisplays = getDisplays;
window.executeQuickTests = executeQuickTests;
window.switchMonitoringTab = switchMonitoringTab;
window.filterLogs = filterLogs;
window.clearLogs = clearLogs;
window.exportLogs = exportLogs;
window.refreshLogs = refreshLogs;

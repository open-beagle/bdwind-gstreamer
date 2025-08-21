/**
 * 优化的应用主入口文件 - 简化的应用管理器
 * 移除不必要的复杂代码，优化连接建立速度和资源使用
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

    // 功能模块 - 简化为核心模块
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

    // 性能优化：预绑定常用方法
    this.updateStats = this.updateStats.bind(this);
  }

  /**
   * 启动应用 - 优化版本
   */
  async start() {
    try {
      // 设置全局错误处理
      this.setupGlobalErrorHandling();

      // 初始化核心组件
      await this.initializeCore();

      // 初始化模块
      await this.initializeModules();

      // 设置事件处理
      this.setupEventHandlers();

      // 连接信令服务器
      await this.connectSignaling();

      // 设置兼容性层
      this.setupCompatibilityLayer();

      // 应用就绪
      this.state.phase = "ready";

      // 开始运行
      this.run();
      
    } catch (error) {
      this.state.phase = "error";
      this.state.errors.push(error);
      throw error;
    }
  }

  /**
   * 初始化核心组件 - 优化版本
   */
  async initializeCore() {
    // 验证 WebRTC Adapter
    if (typeof verifyWebRTCAdapter === 'function' && !verifyWebRTCAdapter()) {
      throw new Error('WebRTC Adapter 未正确加载，无法继续初始化');
    }

    // 创建事件总线
    this.eventBus = new EventBus();

    // 创建配置管理器
    this.config = new ConfigManager(this.eventBus);
    await this.config.loadConfig();

    // 初始化Logger - 优化配置
    Logger.init();
    Logger.setEventBus(this.eventBus);
    Logger.setLevel(this.config.get("ui.logLevel", "warn")); // 默认warn级别
    Logger.setMaxEntries(this.config.get("ui.maxLogEntries", 500)); // 减少日志条目
    
    const debugMode = this.config.get("debug.enabled", false) || 
                     new URLSearchParams(window.location.search).get('debug') === 'true';
    Logger.setDebugMode(debugMode);
    
    this.logger = Logger;

    // 获取媒体元素
    this.videoElement = document.getElementById("video");
    this.audioElement = document.getElementById("audio");
    
    if (!this.videoElement) {
      throw new Error('视频元素未找到');
    }
  }

  /**
   * 初始化模块 - 优化版本
   */
  async initializeModules() {
    try {
      // 创建信令管理器
      this.modules.signaling = new SignalingManager(this.eventBus, this.config);

      // 创建WebRTC管理器
      this.modules.webrtc = new WebRTCManager(
        this.modules.signaling, 
        this.videoElement, 
        1,
        {
          eventBus: this.eventBus,
          config: this.config
        }
      );

      // 创建UI管理器（如果可用）
      if (typeof UIManager !== 'undefined') {
        this.modules.ui = new UIManager(this.eventBus, this.config);
      }

      // 验证模块创建成功
      if (!this.modules.webrtc || !this.modules.signaling) {
        throw new Error('核心模块创建失败');
      }

    } catch (error) {
      throw error;
    }
  }

  /**
   * 设置事件处理 - 优化版本，减少事件监听器
   */
  setupEventHandlers() {
    // 设置 WebRTC 事件回调
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

      this.modules.webrtc.onstatus = (message) => {
        // 只记录重要状态
      };

      this.modules.webrtc.onerror = (message) => {
        this.handleWebRTCError(new Error(message));
      };
    }

    // 设置信令事件回调
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

      this.modules.signaling.onerror = (message) => {
        this.handleSignalingError(new Error(message));
      };
    }
  }

  /**
   * 优化的连接状态更新
   */
  updateConnectionStatus(state) {
    if (!this.modules.ui) return;

    const statusMap = {
      "connecting": { status: "connecting", message: "WebRTC连接建立中..." },
      "connected": { status: "connected", message: "WebRTC连接已建立" },
      "disconnected": { status: "error", message: "WebRTC连接断开" },
      "failed": { status: "error", message: "WebRTC连接失败" }
    };

    const statusInfo = statusMap[state];
    if (statusInfo) {
      this.modules.ui.updateConnectionStatus(statusInfo.status, {
        message: statusInfo.message,
      });
    }
  }

  /**
   * 连接信令服务器 - 优化版本
   */
  async connectSignaling() {
    try {
      if (this.modules.signaling) {
        await this.modules.signaling.connect();
      }

      if (this.modules.webrtc) {
        this.modules.webrtc.connect();
      }

      if (this.modules.ui) {
        this.modules.ui.initialize();
      }

    } catch (error) {
      throw error;
    }
  }

  /**
   * 处理信令错误 - 简化版本
   */
  handleSignalingError(error) {
    if (this.modules.ui && typeof this.modules.ui.updateConnectionStatus === 'function') {
      this.modules.ui.updateConnectionStatus("error", {
        error: `信令错误: ${error.message || error}`,
      });
    } else {
      this.updateBasicStatus("error", `信令错误: ${error.message || error}`);
    }

    // 简化的重连逻辑
    setTimeout(() => {
      if (this.modules.signaling) {
        this.modules.signaling.connect().catch(() => {});
      }
    }, 3000);
  }

  /**
   * 处理WebRTC错误 - 简化版本
   */
  handleWebRTCError(error) {
    if (this.modules.ui && typeof this.modules.ui.updateConnectionStatus === 'function') {
      this.modules.ui.updateConnectionStatus("error", {
        error: `WebRTC错误: ${error.message || error}`,
      });
    } else {
      this.updateBasicStatus("error", `WebRTC错误: ${error.message || error}`);
    }

    // 简化的重置逻辑
    setTimeout(() => {
      if (this.modules.webrtc) {
        this.modules.webrtc.reset();
      }
    }, 3000);
  }

  /**
   * 基本状态更新 - 向后兼容性
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
    if (this.modules.webrtc && typeof this.modules.webrtc.sendDataChannelMessage === 'function') {
      return this.modules.webrtc.sendDataChannelMessage(message);
    }
    return false;
  }

  /**
   * 获取连接统计
   */
  getConnectionStats() {
    if (this.modules.webrtc && typeof this.modules.webrtc.getConnectionStats === 'function') {
      return this.modules.webrtc.getConnectionStats();
    }
    return null;
  }

  /**
   * 运行应用 - 优化版本
   */
  run() {
    this.state.phase = "running";

    // 启动优化的定期任务
    this.startPeriodicTasks();

    // 触发应用就绪事件
    this.eventBus.emit("app:ready", {
      startTime: this.state.startTime,
    });
  }

  /**
   * 启动定期任务 - 优化版本，减少频率
   */
  startPeriodicTasks() {
    // 优化：减少统计更新频率到10秒
    setInterval(this.updateStats, 10000);
  }

  /**
   * 更新统计信息 - 优化版本
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
   * 设置兼容性层 - 优化版本
   */
  setupCompatibilityLayer() {
    // 全局应用访问
    window.app = {
      manager: this,
      modules: this.modules,
      eventBus: this.eventBus,
      config: this.config,
      startCapture: () => this.startCapture(),
      stopCapture: () => this.stopCapture(),
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
  }

  /**
   * 设置全局错误处理 - 优化版本
   */
  setupGlobalErrorHandling() {
    window.addEventListener("error", this.handleError);
    window.addEventListener("unhandledrejection", this.handleUnhandledRejection);
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
  handleBeforeUnload(event) {
    this.shutdown();
  }

  /**
   * 应用关闭 - 优化版本
   */
  async shutdown() {
    this.state.phase = "shutdown";

    // 停止模块
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

    // 清理事件监听器
    window.removeEventListener("error", this.handleError);
    window.removeEventListener("unhandledrejection", this.handleUnhandledRejection);
    window.removeEventListener("beforeunload", this.handleBeforeUnload);
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
   * 开始捕获 - 优化版本
   */
  async startCapture() {
    try {
      if (!this.modules.webrtc) {
        throw new Error("WebRTC模块未初始化");
      }

      this.modules.webrtc.connect();
      this.state.isCapturing = true;
      
      return { success: true };
    } catch (error) {
      if (this.modules.ui) {
        this.modules.ui.updateConnectionStatus("error", {
          error: `启动失败: ${error.message}`,
        });
      }
      throw error;
    }
  }

  /**
   * 停止捕获 - 优化版本
   */
  async stopCapture() {
    try {
      if (this.modules.webrtc) {
        this.modules.webrtc.reset();
      }
      
      if (this.modules.signaling) {
        this.modules.signaling.disconnect();
      }

      this.state.isCapturing = false;
      return { success: true };
    } catch (error) {
      this.state.isCapturing = false;
    }
  }
}

// 创建应用管理器实例
let appManager;

// 应用启动函数 - 优化版本
async function startApplication() {
  try {
    appManager = new ApplicationManager();
    await appManager.start();
    window.appManager = appManager;
  } catch (error) {
    console.error("应用启动失败:", error);
  }
}

// 页面加载完成后启动应用 - 优化版本
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', startApplication);
} else {
  startApplication();
}

// ============================================================================
// 向后兼容性支持 - 简化版本
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

// 导出全局函数
window.toggleVideoPlay = toggleVideoPlay;
window.toggleFullscreen = toggleFullscreen;
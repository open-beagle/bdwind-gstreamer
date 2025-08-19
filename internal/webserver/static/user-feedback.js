/**
 * 用户反馈系统
 * 提供交互式通知、建议和用户引导
 */

/**
 * 通知类型
 */
const NotificationTypes = {
  INFO: "info",
  SUCCESS: "success",
  WARNING: "warning",
  ERROR: "error",
  SUGGESTION: "suggestion",
};

/**
 * 反馈动作类型
 */
const FeedbackActions = {
  RETRY: "retry",
  REFRESH: "refresh",
  SETTINGS: "settings",
  HELP: "help",
  DISMISS: "dismiss",
  DETAILS: "details",
};

/**
 * 用户反馈管理器
 */
class UserFeedbackManager {
  constructor(eventBus, config = {}) {
    this.eventBus = eventBus;
    this.config = config;

    // UI元素
    this.elements = {
      feedbackContainer: null,
      toastContainer: null,
      modalContainer: null,
      helpPanel: null,
    };

    // 反馈状态
    this.activeNotifications = new Map();
    this.feedbackHistory = [];
    this.userPreferences = {
      showSuggestions: true,
      autoHideToasts: true,
      detailedErrors: false,
      soundEnabled: false,
    };

    // 配置
    this.feedbackConfig = {
      maxToasts: config.maxToasts || 5,
      toastDuration: config.toastDuration || 5000,
      enableSuggestions: config.enableSuggestions !== false,
      enableSounds: config.enableSounds === true,
      enableHaptics: config.enableHaptics === true,
    };

    // 建议系统
    this.suggestionEngine = new SuggestionEngine(eventBus);

    this._initialize();
  }

  /**
   * 初始化用户反馈系统
   * @private
   */
  _initialize() {
    this._createUIElements();
    this._setupEventListeners();
    this._loadUserPreferences();

    Logger.info("UserFeedbackManager: 用户反馈系统初始化完成");
  }

  /**
   * 创建UI元素
   * @private
   */
  _createUIElements() {
    this._createFeedbackContainer();
    this._createToastContainer();
    this._createModalContainer();
    this._createHelpPanel();
  }

  /**
   * 创建反馈容器
   * @private
   */
  _createFeedbackContainer() {
    const container = document.createElement("div");
    container.id = "user-feedback-container";
    container.className = "user-feedback-container";
    container.style.cssText = `
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            pointer-events: none;
            z-index: 10000;
        `;

    document.body.appendChild(container);
    this.elements.feedbackContainer = container;
  }

  /**
   * 创建Toast容器
   * @private
   */
  _createToastContainer() {
    const container = document.createElement("div");
    container.id = "toast-container";
    container.className = "toast-container";
    container.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            width: 350px;
            max-width: 90vw;
            z-index: 10001;
            pointer-events: none;
        `;

    this.elements.feedbackContainer.appendChild(container);
    this.elements.toastContainer = container;
  }

  /**
   * 创建模态框容器
   * @private
   */
  _createModalContainer() {
    const container = document.createElement("div");
    container.id = "modal-container";
    container.className = "modal-container";
    container.style.cssText = `
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0, 0, 0, 0.5);
            display: none;
            align-items: center;
            justify-content: center;
            z-index: 10002;
            pointer-events: auto;
        `;

    this.elements.feedbackContainer.appendChild(container);
    this.elements.modalContainer = container;
  }

  /**
   * 创建帮助面板
   * @private
   */
  _createHelpPanel() {
    const panel = document.createElement("div");
    panel.id = "help-panel";
    panel.className = "help-panel";
    panel.style.cssText = `
            position: fixed;
            right: -400px;
            top: 0;
            width: 400px;
            height: 100%;
            background: white;
            box-shadow: -2px 0 10px rgba(0, 0, 0, 0.3);
            transition: right 0.3s ease;
            z-index: 10003;
            pointer-events: auto;
            overflow-y: auto;
        `;

    panel.innerHTML = `
            <div class="help-header" style="padding: 20px; border-bottom: 1px solid #eee; background: #f8f9fa;">
                <h3 style="margin: 0; color: #333;">帮助与建议</h3>
                <button class="help-close" style="position: absolute; right: 15px; top: 15px; background: none; border: none; font-size: 24px; cursor: pointer; color: #666;">×</button>
            </div>
            <div class="help-content" style="padding: 20px;">
                <div class="help-section">
                    <h4>常见问题</h4>
                    <div class="help-faq"></div>
                </div>
                <div class="help-section">
                    <h4>连接诊断</h4>
                    <div class="help-diagnostics"></div>
                </div>
                <div class="help-section">
                    <h4>系统要求</h4>
                    <div class="help-requirements"></div>
                </div>
            </div>
        `;

    // 添加关闭事件
    const closeBtn = panel.querySelector(".help-close");
    closeBtn.addEventListener("click", () => {
      this.hideHelpPanel();
    });

    this.elements.feedbackContainer.appendChild(panel);
    this.elements.helpPanel = panel;
  }

  /**
   * 设置事件监听器
   * @private
   */
  _setupEventListeners() {
    if (!this.eventBus) return;

    // 监听错误处理器事件
    this.eventBus.on("error-handler:error-occurred", (errorInfo) => {
      this._handleErrorFeedback(errorInfo);
    });

    this.eventBus.on("error-handler:status-updated", (statusInfo) => {
      this._handleStatusFeedback(statusInfo);
    });

    // 监听连接事件
    this.eventBus.on("webrtc:connected", () => {
      this.showToast(
        "连接成功",
        "视频流已建立，可以开始观看",
        NotificationTypes.SUCCESS
      );
    });

    this.eventBus.on("webrtc:disconnected", () => {
      this.showToast("连接断开", "视频流已断开", NotificationTypes.WARNING, {
        actions: [{ type: FeedbackActions.RETRY, text: "重新连接" }],
      });
    });

    this.eventBus.on("webrtc:reconnecting", (data) => {
      this.showToast(
        "正在重连",
        `尝试重新连接 (${data.attempt}/${data.maxAttempts})`,
        NotificationTypes.INFO
      );
    });

    // 监听配置事件
    this.eventBus.on("config:webrtc-fetch-failed", (data) => {
      this.showSuggestion(
        "配置获取失败",
        "无法获取服务器配置，将使用默认设置",
        ["检查网络连接", "确认服务器状态", "尝试刷新页面"]
      );
    });

    // 监听质量事件
    this.eventBus.on("webrtc:quality-degraded", (data) => {
      this._handleQualityDegradation(data);
    });

    // 监听用户交互事件
    this.eventBus.on("ui:user-action-needed", (data) => {
      this._handleUserActionNeeded(data);
    });
  }

  /**
   * 处理错误反馈
   * @private
   */
  _handleErrorFeedback(errorInfo) {
    const suggestions = this.suggestionEngine.getSuggestions(errorInfo);

    if (errorInfo.severity === "critical" || errorInfo.severity === "high") {
      this.showModal({
        title: "连接错误",
        message: errorInfo.message,
        type: NotificationTypes.ERROR,
        suggestions: suggestions,
        actions: [
          { type: FeedbackActions.RETRY, text: "重试连接", primary: true },
          { type: FeedbackActions.HELP, text: "获取帮助" },
          { type: FeedbackActions.DISMISS, text: "关闭" },
        ],
      });
    } else {
      this.showToast(
        this._getErrorTitle(errorInfo.type),
        errorInfo.message,
        NotificationTypes.ERROR,
        {
          duration: 8000,
          actions:
            suggestions.length > 0
              ? [{ type: FeedbackActions.DETAILS, text: "查看建议" }]
              : [],
        }
      );
    }
  }

  /**
   * 处理状态反馈
   * @private
   */
  _handleStatusFeedback(statusInfo) {
    if (
      statusInfo.current === "connected" &&
      statusInfo.previous !== "connected"
    ) {
      // 连接成功的庆祝反馈
      this._showSuccessFeedback();
    } else if (statusInfo.current === "error") {
      // 错误状态的反馈
      this._showErrorStateFeedback(statusInfo.details);
    }
  }

  /**
   * 处理质量下降
   * @private
   */
  _handleQualityDegradation(data) {
    const suggestions = [
      "检查网络连接稳定性",
      "关闭其他占用带宽的应用",
      "尝试切换到更稳定的网络",
    ];

    this.showSuggestion(
      "连接质量下降",
      "检测到网络质量问题，可能影响观看体验",
      suggestions
    );
  }

  /**
   * 处理需要用户操作的情况
   * @private
   */
  _handleUserActionNeeded(data) {
    switch (data.action) {
      case "enable-autoplay":
        this.showModal({
          title: "需要用户交互",
          message: "浏览器阻止了自动播放，请点击播放按钮开始观看",
          type: NotificationTypes.INFO,
          actions: [
            { type: "play", text: "开始播放", primary: true },
            { type: FeedbackActions.DISMISS, text: "稍后" },
          ],
        });
        break;

      case "grant-permissions":
        this.showModal({
          title: "权限请求",
          message: "需要摄像头和麦克风权限才能正常工作",
          type: NotificationTypes.WARNING,
          actions: [
            { type: "grant", text: "授予权限", primary: true },
            { type: FeedbackActions.HELP, text: "如何设置权限" },
            { type: FeedbackActions.DISMISS, text: "取消" },
          ],
        });
        break;
    }
  }

  /**
   * 显示Toast通知
   * @param {string} title - 标题
   * @param {string} message - 消息
   * @param {string} type - 类型
   * @param {Object} options - 选项
   */
  showToast(title, message, type = NotificationTypes.INFO, options = {}) {
    const toastId = this._generateId();
    const toast = this._createToast(toastId, title, message, type, options);

    this.elements.toastContainer.appendChild(toast);
    this.activeNotifications.set(toastId, {
      element: toast,
      type: "toast",
      timestamp: Date.now(),
    });

    // 限制Toast数量
    this._limitToasts();

    // 自动隐藏
    if (this.userPreferences.autoHideToasts && options.duration !== 0) {
      const duration = options.duration || this.feedbackConfig.toastDuration;
      setTimeout(() => {
        this.hideToast(toastId);
      }, duration);
    }

    // 播放声音
    if (this.feedbackConfig.enableSounds) {
      this._playNotificationSound(type);
    }

    // 触觉反馈
    if (this.feedbackConfig.enableHaptics) {
      this._triggerHapticFeedback(type);
    }

    return toastId;
  }

  /**
   * 创建Toast元素
   * @private
   */
  _createToast(id, title, message, type, options) {
    const toast = document.createElement("div");
    toast.id = `toast-${id}`;
    toast.className = `toast toast-${type}`;
    toast.style.cssText = `
            background: ${this._getTypeColor(type)};
            color: white;
            padding: 16px;
            border-radius: 8px;
            margin-bottom: 10px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
            pointer-events: auto;
            cursor: pointer;
            transition: all 0.3s ease;
            border-left: 4px solid ${this._getTypeAccentColor(type)};
            animation: slideInRight 0.3s ease-out;
        `;

    const actions = options.actions || [];

    toast.innerHTML = `
            <div class="toast-header" style="display: flex; align-items: center; margin-bottom: 8px;">
                <span class="toast-icon" style="font-size: 18px; margin-right: 8px;">${this._getTypeIcon(
                  type
                )}</span>
                <span class="toast-title" style="font-weight: bold; flex: 1;">${title}</span>
                <button class="toast-close" style="background: none; border: none; color: white; cursor: pointer; font-size: 18px; opacity: 0.7;">×</button>
            </div>
            <div class="toast-message" style="margin-bottom: ${
              actions.length > 0 ? "12px" : "0"
            }; opacity: 0.9;">
                ${message}
            </div>
            ${
              actions.length > 0
                ? `
                <div class="toast-actions" style="display: flex; gap: 8px; flex-wrap: wrap;">
                    ${actions
                      .map(
                        (action) => `
                        <button class="toast-action" data-action="${action.type}" style="
                            background: rgba(255, 255, 255, 0.2);
                            border: 1px solid rgba(255, 255, 255, 0.3);
                            color: white;
                            padding: 6px 12px;
                            border-radius: 4px;
                            cursor: pointer;
                            font-size: 12px;
                            transition: background 0.2s;
                        ">${action.text}</button>
                    `
                      )
                      .join("")}
                </div>
            `
                : ""
            }
        `;

    // 添加事件监听器
    const closeBtn = toast.querySelector(".toast-close");
    closeBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      this.hideToast(id);
    });

    // 添加动作按钮事件
    const actionBtns = toast.querySelectorAll(".toast-action");
    actionBtns.forEach((btn) => {
      btn.addEventListener("click", (e) => {
        e.stopPropagation();
        const actionType = btn.dataset.action;
        this._handleToastAction(id, actionType, { title, message, type });
      });

      // 悬停效果
      btn.addEventListener("mouseenter", () => {
        btn.style.background = "rgba(255, 255, 255, 0.3)";
      });
      btn.addEventListener("mouseleave", () => {
        btn.style.background = "rgba(255, 255, 255, 0.2)";
      });
    });

    return toast;
  }

  /**
   * 显示建议通知
   * @param {string} title - 标题
   * @param {string} message - 消息
   * @param {Array} suggestions - 建议列表
   */
  showSuggestion(title, message, suggestions = []) {
    if (
      !this.feedbackConfig.enableSuggestions ||
      !this.userPreferences.showSuggestions
    ) {
      return;
    }

    const suggestionHtml =
      suggestions.length > 0
        ? `
            <div class="suggestion-list" style="margin-top: 12px; padding-top: 12px; border-top: 1px solid rgba(255, 255, 255, 0.2);">
                <div style="font-size: 12px; font-weight: bold; margin-bottom: 8px;">建议解决方案:</div>
                <ul style="margin: 0; padding-left: 16px; font-size: 12px; opacity: 0.9;">
                    ${suggestions
                      .map((s) => `<li style="margin-bottom: 4px;">${s}</li>`)
                      .join("")}
                </ul>
            </div>
        `
        : "";

    return this.showToast(
      title,
      message + suggestionHtml,
      NotificationTypes.SUGGESTION,
      {
        duration: 10000,
        actions: [
          { type: FeedbackActions.HELP, text: "更多帮助" },
          { type: FeedbackActions.DISMISS, text: "知道了" },
        ],
      }
    );
  }

  /**
   * 显示模态框
   * @param {Object} options - 模态框选项
   */
  showModal(options) {
    const modal = this._createModal(options);

    this.elements.modalContainer.innerHTML = "";
    this.elements.modalContainer.appendChild(modal);
    this.elements.modalContainer.style.display = "flex";

    // 添加动画
    setTimeout(() => {
      modal.style.transform = "scale(1)";
      modal.style.opacity = "1";
    }, 10);

    // 播放声音
    if (this.feedbackConfig.enableSounds) {
      this._playNotificationSound(options.type);
    }

    return modal;
  }

  /**
   * 创建模态框
   * @private
   */
  _createModal(options) {
    const modal = document.createElement("div");
    modal.className = `modal modal-${options.type}`;
    modal.style.cssText = `
            background: white;
            border-radius: 12px;
            padding: 24px;
            max-width: 500px;
            width: 90%;
            max-height: 80vh;
            overflow-y: auto;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.3);
            transform: scale(0.9);
            opacity: 0;
            transition: all 0.3s ease;
        `;

    const suggestions = options.suggestions || [];
    const actions = options.actions || [];

    modal.innerHTML = `
            <div class="modal-header" style="display: flex; align-items: center; margin-bottom: 16px;">
                <span class="modal-icon" style="font-size: 24px; margin-right: 12px; color: ${this._getTypeColor(
                  options.type
                )};">
                    ${this._getTypeIcon(options.type)}
                </span>
                <h3 style="margin: 0; flex: 1; color: #333;">${
                  options.title
                }</h3>
                <button class="modal-close" style="background: none; border: none; font-size: 24px; cursor: pointer; color: #666;">×</button>
            </div>
            <div class="modal-content">
                <p style="margin: 0 0 16px 0; color: #666; line-height: 1.5;">${
                  options.message
                }</p>
                ${
                  suggestions.length > 0
                    ? `
                    <div class="modal-suggestions" style="background: #f8f9fa; padding: 16px; border-radius: 8px; margin-bottom: 16px;">
                        <h4 style="margin: 0 0 12px 0; color: #333; font-size: 14px;">建议解决方案:</h4>
                        <ul style="margin: 0; padding-left: 20px; color: #666;">
                            ${suggestions
                              .map(
                                (s) =>
                                  `<li style="margin-bottom: 8px;">${s}</li>`
                              )
                              .join("")}
                        </ul>
                    </div>
                `
                    : ""
                }
            </div>
            ${
              actions.length > 0
                ? `
                <div class="modal-actions" style="display: flex; gap: 12px; justify-content: flex-end; margin-top: 20px; flex-wrap: wrap;">
                    ${actions
                      .map(
                        (action) => `
                        <button class="modal-action ${
                          action.primary ? "primary" : "secondary"
                        }" data-action="${action.type}" style="
                            padding: 10px 20px;
                            border: none;
                            border-radius: 6px;
                            cursor: pointer;
                            font-size: 14px;
                            font-weight: 500;
                            transition: all 0.2s;
                            ${
                              action.primary
                                ? "background: #007bff; color: white;"
                                : "background: #f8f9fa; color: #666; border: 1px solid #dee2e6;"
                            }
                        ">${action.text}</button>
                    `
                      )
                      .join("")}
                </div>
            `
                : ""
            }
        `;

    // 添加事件监听器
    const closeBtn = modal.querySelector(".modal-close");
    closeBtn.addEventListener("click", () => {
      this.hideModal();
    });

    // 添加动作按钮事件
    const actionBtns = modal.querySelectorAll(".modal-action");
    actionBtns.forEach((btn) => {
      btn.addEventListener("click", () => {
        const actionType = btn.dataset.action;
        this._handleModalAction(actionType, options);
      });

      // 悬停效果
      btn.addEventListener("mouseenter", () => {
        if (btn.classList.contains("primary")) {
          btn.style.background = "#0056b3";
        } else {
          btn.style.background = "#e9ecef";
        }
      });
      btn.addEventListener("mouseleave", () => {
        if (btn.classList.contains("primary")) {
          btn.style.background = "#007bff";
        } else {
          btn.style.background = "#f8f9fa";
        }
      });
    });

    // 点击背景关闭
    this.elements.modalContainer.addEventListener("click", (e) => {
      if (e.target === this.elements.modalContainer) {
        this.hideModal();
      }
    });

    return modal;
  }

  /**
   * 显示帮助面板
   */
  showHelpPanel() {
    this.elements.helpPanel.style.right = "0";
    this._updateHelpContent();
  }

  /**
   * 隐藏帮助面板
   */
  hideHelpPanel() {
    this.elements.helpPanel.style.right = "-400px";
  }

  /**
   * 更新帮助内容
   * @private
   */
  _updateHelpContent() {
    const faqSection = this.elements.helpPanel.querySelector(".help-faq");
    const diagnosticsSection =
      this.elements.helpPanel.querySelector(".help-diagnostics");
    const requirementsSection =
      this.elements.helpPanel.querySelector(".help-requirements");

    // FAQ内容
    if (faqSection) {
      faqSection.innerHTML = `
                <div class="faq-item" style="margin-bottom: 16px;">
                    <h5 style="margin: 0 0 8px 0; color: #333;">无法看到视频？</h5>
                    <p style="margin: 0; color: #666; font-size: 14px;">
                        检查网络连接，确认浏览器支持WebRTC，允许必要的权限。
                    </p>
                </div>
                <div class="faq-item" style="margin-bottom: 16px;">
                    <h5 style="margin: 0 0 8px 0; color: #333;">视频卡顿怎么办？</h5>
                    <p style="margin: 0; color: #666; font-size: 14px;">
                        检查网络带宽，关闭其他占用网络的应用，尝试降低视频质量。
                    </p>
                </div>
                <div class="faq-item" style="margin-bottom: 16px;">
                    <h5 style="margin: 0 0 8px 0; color: #333;">连接失败？</h5>
                    <p style="margin: 0; color: #666; font-size: 14px;">
                        检查防火墙设置，确认STUN/TURN服务器可用，尝试刷新页面。
                    </p>
                </div>
            `;
    }

    // 诊断内容
    if (diagnosticsSection) {
      diagnosticsSection.innerHTML = `
                <button class="diagnostic-btn" style="
                    width: 100%;
                    padding: 10px;
                    margin-bottom: 8px;
                    background: #007bff;
                    color: white;
                    border: none;
                    border-radius: 4px;
                    cursor: pointer;
                ">运行连接测试</button>
                <button class="diagnostic-btn" style="
                    width: 100%;
                    padding: 10px;
                    margin-bottom: 8px;
                    background: #28a745;
                    color: white;
                    border: none;
                    border-radius: 4px;
                    cursor: pointer;
                ">检查浏览器兼容性</button>
                <button class="diagnostic-btn" style="
                    width: 100%;
                    padding: 10px;
                    background: #ffc107;
                    color: #212529;
                    border: none;
                    border-radius: 4px;
                    cursor: pointer;
                ">导出诊断信息</button>
            `;

      // 添加诊断按钮事件
      const diagnosticBtns =
        diagnosticsSection.querySelectorAll(".diagnostic-btn");
      diagnosticBtns.forEach((btn, index) => {
        btn.addEventListener("click", () => {
          switch (index) {
            case 0:
              this._runConnectionTest();
              break;
            case 1:
              this._checkBrowserCompatibility();
              break;
            case 2:
              this._exportDiagnostics();
              break;
          }
        });
      });
    }

    // 系统要求内容
    if (requirementsSection) {
      requirementsSection.innerHTML = `
                <div class="requirement-item" style="margin-bottom: 12px;">
                    <strong>浏览器:</strong> Chrome 60+, Firefox 55+, Safari 11+, Edge 79+
                </div>
                <div class="requirement-item" style="margin-bottom: 12px;">
                    <strong>网络:</strong> 稳定的互联网连接，建议带宽 > 2Mbps
                </div>
                <div class="requirement-item" style="margin-bottom: 12px;">
                    <strong>权限:</strong> 摄像头和麦克风访问权限（如需要）
                </div>
                <div class="requirement-item" style="margin-bottom: 12px;">
                    <strong>协议:</strong> HTTPS连接（WebRTC要求）
                </div>
            `;
    }
  }

  /**
   * 处理Toast动作
   * @private
   */
  _handleToastAction(toastId, actionType, context) {
    switch (actionType) {
      case FeedbackActions.RETRY:
        this.eventBus?.emit("ui:request-reconnect", { reason: "user-retry" });
        this.hideToast(toastId);
        break;

      case FeedbackActions.REFRESH:
        window.location.reload();
        break;

      case FeedbackActions.HELP:
        this.showHelpPanel();
        break;

      case FeedbackActions.DETAILS:
        this.showModal({
          title: context.title,
          message: context.message,
          type: context.type,
          suggestions: this.suggestionEngine.getSuggestions({
            type: context.type,
          }),
          actions: [
            { type: FeedbackActions.RETRY, text: "重试", primary: true },
            { type: FeedbackActions.HELP, text: "获取帮助" },
            { type: FeedbackActions.DISMISS, text: "关闭" },
          ],
        });
        this.hideToast(toastId);
        break;

      case FeedbackActions.DISMISS:
        this.hideToast(toastId);
        break;
    }
  }

  /**
   * 处理模态框动作
   * @private
   */
  _handleModalAction(actionType, context) {
    switch (actionType) {
      case FeedbackActions.RETRY:
        this.eventBus?.emit("ui:request-reconnect", { reason: "user-retry" });
        this.hideModal();
        break;

      case FeedbackActions.HELP:
        this.hideModal();
        this.showHelpPanel();
        break;

      case "play":
        this.eventBus?.emit("ui:request-video-play");
        this.hideModal();
        break;

      case "grant":
        this.eventBus?.emit("ui:request-permissions");
        this.hideModal();
        break;

      case FeedbackActions.DISMISS:
        this.hideModal();
        break;
    }
  }

  /**
   * 隐藏Toast
   * @param {string} toastId - Toast ID
   */
  hideToast(toastId) {
    const notification = this.activeNotifications.get(toastId);
    if (notification && notification.element) {
      notification.element.style.transform = "translateX(100%)";
      notification.element.style.opacity = "0";
      setTimeout(() => {
        if (notification.element.parentNode) {
          notification.element.parentNode.removeChild(notification.element);
        }
        this.activeNotifications.delete(toastId);
      }, 300);
    }
  }

  /**
   * 隐藏模态框
   */
  hideModal() {
    const modal = this.elements.modalContainer.querySelector(".modal");
    if (modal) {
      modal.style.transform = "scale(0.9)";
      modal.style.opacity = "0";
      setTimeout(() => {
        this.elements.modalContainer.style.display = "none";
      }, 300);
    }
  }

  /**
   * 限制Toast数量
   * @private
   */
  _limitToasts() {
    const toasts = Array.from(this.activeNotifications.entries())
      .filter(([id, notification]) => notification.type === "toast")
      .sort((a, b) => a[1].timestamp - b[1].timestamp);

    while (toasts.length > this.feedbackConfig.maxToasts) {
      const [oldestId] = toasts.shift();
      this.hideToast(oldestId);
    }
  }

  /**
   * 获取类型颜色
   * @private
   */
  _getTypeColor(type) {
    const colors = {
      [NotificationTypes.INFO]: "#2196F3",
      [NotificationTypes.SUCCESS]: "#4CAF50",
      [NotificationTypes.WARNING]: "#FF9800",
      [NotificationTypes.ERROR]: "#F44336",
      [NotificationTypes.SUGGESTION]: "#9C27B0",
    };
    return colors[type] || colors[NotificationTypes.INFO];
  }

  /**
   * 获取类型强调色
   * @private
   */
  _getTypeAccentColor(type) {
    const colors = {
      [NotificationTypes.INFO]: "#1976D2",
      [NotificationTypes.SUCCESS]: "#388E3C",
      [NotificationTypes.WARNING]: "#F57C00",
      [NotificationTypes.ERROR]: "#D32F2F",
      [NotificationTypes.SUGGESTION]: "#7B1FA2",
    };
    return colors[type] || colors[NotificationTypes.INFO];
  }

  /**
   * 获取类型图标
   * @private
   */
  _getTypeIcon(type) {
    const icons = {
      [NotificationTypes.INFO]: "ℹ️",
      [NotificationTypes.SUCCESS]: "✅",
      [NotificationTypes.WARNING]: "⚠️",
      [NotificationTypes.ERROR]: "❌",
      [NotificationTypes.SUGGESTION]: "💡",
    };
    return icons[type] || icons[NotificationTypes.INFO];
  }

  /**
   * 获取错误标题
   * @private
   */
  _getErrorTitle(errorType) {
    const titles = {
      configuration: "配置错误",
      signaling: "信令错误",
      ice_connection: "连接错误",
      media_stream: "媒体错误",
      network: "网络错误",
      browser_compatibility: "浏览器兼容性",
      permission: "权限错误",
    };
    return titles[errorType] || "未知错误";
  }

  /**
   * 显示成功反馈
   * @private
   */
  _showSuccessFeedback() {
    // 可以添加庆祝动画或特效
    this.showToast(
      "连接成功！",
      "视频流已建立，享受观看体验",
      NotificationTypes.SUCCESS,
      {
        duration: 3000,
      }
    );
  }

  /**
   * 显示错误状态反馈
   * @private
   */
  _showErrorStateFeedback(details) {
    const suggestions = this.suggestionEngine.getSuggestions(details);
    this.showSuggestion(
      "连接出现问题",
      details.message || "连接遇到错误",
      suggestions
    );
  }

  /**
   * 播放通知声音
   * @private
   */
  _playNotificationSound(type) {
    if (!this.userPreferences.soundEnabled) return;

    // 这里可以播放不同类型的通知声音
    // 由于浏览器限制，需要用户交互后才能播放声音
    try {
      const audioContext = new (window.AudioContext ||
        window.webkitAudioContext)();
      const oscillator = audioContext.createOscillator();
      const gainNode = audioContext.createGain();

      oscillator.connect(gainNode);
      gainNode.connect(audioContext.destination);

      // 根据类型设置不同的频率
      const frequencies = {
        [NotificationTypes.SUCCESS]: 800,
        [NotificationTypes.ERROR]: 400,
        [NotificationTypes.WARNING]: 600,
        [NotificationTypes.INFO]: 500,
        [NotificationTypes.SUGGESTION]: 700,
      };

      oscillator.frequency.setValueAtTime(
        frequencies[type] || 500,
        audioContext.currentTime
      );
      gainNode.gain.setValueAtTime(0.1, audioContext.currentTime);
      gainNode.gain.exponentialRampToValueAtTime(
        0.01,
        audioContext.currentTime + 0.2
      );

      oscillator.start(audioContext.currentTime);
      oscillator.stop(audioContext.currentTime + 0.2);
    } catch (error) {
      // 忽略音频播放错误
    }
  }

  /**
   * 触发触觉反馈
   * @private
   */
  _triggerHapticFeedback(type) {
    if (!navigator.vibrate || !this.userPreferences.hapticEnabled) return;

    // 根据类型设置不同的振动模式
    const patterns = {
      [NotificationTypes.SUCCESS]: [100],
      [NotificationTypes.ERROR]: [100, 50, 100],
      [NotificationTypes.WARNING]: [150],
      [NotificationTypes.INFO]: [50],
      [NotificationTypes.SUGGESTION]: [50, 50, 50],
    };

    navigator.vibrate(patterns[type] || [50]);
  }

  /**
   * 运行连接测试
   * @private
   */
  _runConnectionTest() {
    this.showToast("连接测试", "正在运行连接诊断...", NotificationTypes.INFO);

    // 触发连接测试事件
    this.eventBus?.emit("diagnostics:run-connection-test");
  }

  /**
   * 检查浏览器兼容性
   * @private
   */
  _checkBrowserCompatibility() {
    const compatibility = this._getBrowserCompatibility();

    this.showModal({
      title: "浏览器兼容性检查",
      message: "检查结果:",
      type: compatibility.overall
        ? NotificationTypes.SUCCESS
        : NotificationTypes.WARNING,
      suggestions: compatibility.issues,
      actions: [{ type: FeedbackActions.DISMISS, text: "关闭", primary: true }],
    });
  }

  /**
   * 获取浏览器兼容性信息
   * @private
   */
  _getBrowserCompatibility() {
    const issues = [];
    let overall = true;

    // 检查WebRTC支持
    if (!window.RTCPeerConnection) {
      issues.push("浏览器不支持WebRTC");
      overall = false;
    }

    // 检查getUserMedia支持
    if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
      issues.push("浏览器不支持媒体设备访问");
      overall = false;
    }

    // 检查WebSocket支持
    if (!window.WebSocket) {
      issues.push("浏览器不支持WebSocket");
      overall = false;
    }

    // 检查HTTPS
    if (location.protocol !== "https:" && location.hostname !== "localhost") {
      issues.push("建议使用HTTPS协议以获得完整功能");
      overall = false;
    }

    if (overall) {
      issues.push("浏览器完全兼容所有功能");
    }

    return { overall, issues };
  }

  /**
   * 导出诊断信息
   * @private
   */
  _exportDiagnostics() {
    const diagnostics = {
      timestamp: new Date().toISOString(),
      userAgent: navigator.userAgent,
      url: window.location.href,
      compatibility: this._getBrowserCompatibility(),
      feedbackHistory: this.feedbackHistory.slice(-20),
      activeNotifications: Array.from(this.activeNotifications.keys()),
    };

    const blob = new Blob([JSON.stringify(diagnostics, null, 2)], {
      type: "application/json",
    });

    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `diagnostics-${Date.now()}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);

    this.showToast("诊断信息", "诊断信息已导出", NotificationTypes.SUCCESS);
  }

  /**
   * 加载用户偏好
   * @private
   */
  _loadUserPreferences() {
    try {
      const saved = localStorage.getItem("user-feedback-preferences");
      if (saved) {
        this.userPreferences = {
          ...this.userPreferences,
          ...JSON.parse(saved),
        };
      }
    } catch (error) {
      Logger.warn("UserFeedbackManager: 加载用户偏好失败", error);
    }
  }

  /**
   * 保存用户偏好
   */
  saveUserPreferences() {
    try {
      localStorage.setItem(
        "user-feedback-preferences",
        JSON.stringify(this.userPreferences)
      );
    } catch (error) {
      Logger.warn("UserFeedbackManager: 保存用户偏好失败", error);
    }
  }

  /**
   * 生成ID
   * @private
   */
  _generateId() {
    return `${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  }

  /**
   * 清除所有通知
   */
  clearAllNotifications() {
    this.activeNotifications.forEach((notification, id) => {
      if (notification.type === "toast") {
        this.hideToast(id);
      }
    });
    this.hideModal();
  }

  /**
   * 获取反馈统计
   */
  getFeedbackStats() {
    return {
      totalNotifications: this.feedbackHistory.length,
      activeNotifications: this.activeNotifications.size,
      userPreferences: { ...this.userPreferences },
    };
  }

  /**
   * 销毁用户反馈管理器
   */
  destroy() {
    this.clearAllNotifications();

    // 移除UI元素
    if (
      this.elements.feedbackContainer &&
      this.elements.feedbackContainer.parentNode
    ) {
      this.elements.feedbackContainer.parentNode.removeChild(
        this.elements.feedbackContainer
      );
    }

    // 清理数据
    this.activeNotifications.clear();
    this.feedbackHistory = [];

    Logger.info("UserFeedbackManager: 用户反馈管理器已销毁");
  }
}

/**
 * 建议引擎
 */
class SuggestionEngine {
  constructor(eventBus) {
    this.eventBus = eventBus;
    this.suggestions = this._initializeSuggestions();
  }

  /**
   * 初始化建议数据库
   * @private
   */
  _initializeSuggestions() {
    return {
      configuration: [
        "检查服务器配置文件",
        "验证ICE服务器地址",
        "确认认证信息正确",
      ],
      signaling: [
        "检查网络连接",
        "确认WebSocket服务器状态",
        "尝试刷新页面重新连接",
      ],
      ice_connection: [
        "检查防火墙设置",
        "确认STUN/TURN服务器可用",
        "尝试使用不同的网络",
      ],
      media_stream: [
        "检查摄像头和麦克风权限",
        "确认媒体设备正常工作",
        "尝试重新启动浏览器",
      ],
      network: [
        "检查网络连接稳定性",
        "尝试切换到更稳定的网络",
        "关闭其他占用带宽的应用",
      ],
    };
  }

  /**
   * 获取建议
   * @param {Object} context - 上下文信息
   * @returns {Array} 建议列表
   */
  getSuggestions(context) {
    const type = context.type || "unknown";
    return (
      this.suggestions[type] || ["尝试刷新页面", "检查网络连接", "联系技术支持"]
    );
  }
}

// 添加CSS样式
const userFeedbackStyle = document.createElement("style");
userFeedbackStyle.textContent = `
    @keyframes slideInRight {
        from {
            transform: translateX(100%);
            opacity: 0;
        }
        to {
            transform: translateX(0);
            opacity: 1;
        }
    }
    
    .toast:hover {
        transform: translateY(-2px);
        box-shadow: 0 6px 16px rgba(0, 0, 0, 0.4);
    }
    
    .modal {
        animation: modalFadeIn 0.3s ease-out;
    }
    
    @keyframes modalFadeIn {
        from {
            transform: scale(0.8);
            opacity: 0;
        }
        to {
            transform: scale(1);
            opacity: 1;
        }
    }
`;
document.head.appendChild(userFeedbackStyle);

// 导出
if (typeof module !== "undefined" && module.exports) {
  module.exports = {
    UserFeedbackManager,
    SuggestionEngine,
    NotificationTypes,
    FeedbackActions,
  };
}

if (typeof window !== "undefined") {
  window.UserFeedbackManager = UserFeedbackManager;
  window.SuggestionEngine = SuggestionEngine;
  window.NotificationTypes = NotificationTypes;
  window.FeedbackActions = FeedbackActions;
}

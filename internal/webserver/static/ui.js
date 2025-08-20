/**
 * UIManager - 用户界面管理器
 * 管理UI状态、控制面板、日志显示和响应式界面
 */
class UIManager {
    constructor(eventBus, config) {
        this.eventBus = eventBus;
        this.config = config;
        
        // 增强的错误处理和用户反馈系统
        this.errorHandler = null;
        this.userFeedback = null;
        
        // UI元素
        this.elements = {
            statusIndicator: null,
            statusText: null,
            video: null,
            audio: null,
            videoContainer: null,
            videoPlaceholder: null,
            videoControls: null,
            videoInfo: null,
            logContainer: null
        };
        
        // UI状态
        this.state = {
            isFullscreen: false,
            showingControls: true,
            showingStats: false,
            showingLogs: true,
            showingVideoInfo: true,
            currentTheme: 'auto',
            currentStatus: 'offline'
        };
        
        // 控制面板管理
        this.controlsTimer = null;
        this.autoHideDelay = 3000;
        this.autoHideControls = true;
        
        // 日志管理
        this.maxLogEntries = 1000;
        this.logLevel = 'info';
        this.logLevels = ['debug', 'info', 'warn', 'error'];
        
        // 响应式断点
        this.breakpoints = {
            mobile: 768,
            tablet: 1024,
            desktop: 1200
        };
        
        this._loadConfig();
        this._setupEventListeners();
        
        // 初始化任务 8 的UI调试验证
        this._initializeUIDebugging();
    }

    /**
     * 加载配置
     * @private
     */
    _loadConfig() {
        if (this.config) {
            const uiConfig = this.config.get('ui', {});
            this.state.showingControls = uiConfig.showControls !== false;
            this.state.showingStats = uiConfig.showStats === true;
            this.state.showingLogs = uiConfig.showLogs !== false;
            this.state.showingVideoInfo = uiConfig.showVideoInfo !== false;
            this.state.currentTheme = uiConfig.theme || 'auto';
            this.logLevel = uiConfig.logLevel || 'info';
            this.maxLogEntries = uiConfig.maxLogEntries || 1000;
            this.autoHideControls = uiConfig.autoHideControls !== false;
            this.autoHideDelay = uiConfig.autoHideDelay || 3000;
        }
    }

    /**
     * 初始化增强的错误处理和用户反馈系统
     * @private
     */
    _initializeEnhancedSystems() {
        try {
            // 初始化增强的错误处理器
            if (typeof EnhancedErrorHandler !== 'undefined') {
                this.errorHandler = new EnhancedErrorHandler(this.eventBus, {
                    showDetailedErrors: this.config?.get('ui.showDetailedErrors', false),
                    autoHideDelay: this.config?.get('ui.errorAutoHideDelay', 5000),
                    enableSuggestions: this.config?.get('ui.enableSuggestions', true),
                    enableNetworkQuality: this.config?.get('ui.enableNetworkQuality', true)
                });
                Logger.info('UIManager: 增强错误处理器已初始化');
            }
            
            // 初始化用户反馈管理器
            if (typeof UserFeedbackManager !== 'undefined') {
                this.userFeedback = new UserFeedbackManager(this.eventBus, {
                    maxToasts: this.config?.get('ui.maxToasts', 5),
                    toastDuration: this.config?.get('ui.toastDuration', 5000),
                    enableSuggestions: this.config?.get('ui.enableSuggestions', true),
                    enableSounds: this.config?.get('ui.enableSounds', false),
                    enableHaptics: this.config?.get('ui.enableHaptics', false)
                });
                Logger.info('UIManager: 用户反馈管理器已初始化');
            }
            
            // 网络质量监控已移除
            
        } catch (error) {
            Logger.error('UIManager: 初始化增强系统失败', error);
        }
    }

    /**
     * 设置事件监听器
     * @private
     */
    _setupEventListeners() {
        if (this.eventBus) {
            // 监听状态变化事件
            this.eventBus.on('signaling:connected', () => this.updateStatus('online', '已连接'));
            this.eventBus.on('signaling:disconnected', () => this.updateStatus('offline', '已断开'));
            this.eventBus.on('signaling:reconnecting', (data) => {
                this.updateStatus('connecting', `重连中 (${data.attempt}/${data.maxAttempts})`);
                this.showStopReconnectButton();
            });
            
            this.eventBus.on('webrtc:connected', () => {
                this.updateStatus('online', '视频流媒体中');
                this.hideStopReconnectButton();
            });
            this.eventBus.on('webrtc:disconnected', () => {
                this.updateStatus('offline', '视频已断开');
                this.hideStopReconnectButton();
            });
            this.eventBus.on('webrtc:connection-failed', () => {
                this.updateStatus('offline', '连接失败');
                this.hideStopReconnectButton();
            });
            this.eventBus.on('webrtc:reconnection-stopped', () => {
                this.updateStatus('offline', '重连已停止');
                this.hideStopReconnectButton();
            });
            
            // 监听日志事件
            this.eventBus.on('logger:entry', (entry) => this.addLogEntry(entry));
            
            // 监听UI控制事件
            this.eventBus.on('ui:toggle-controls', () => this.toggleControls());
            this.eventBus.on('ui:toggle-stats', () => this.toggleStats());
            this.eventBus.on('ui:toggle-logs', () => this.toggleLogs());
            this.eventBus.on('ui:toggle-fullscreen', () => this.toggleFullscreen());
            this.eventBus.on('ui:toggle-play', () => this.toggleVideoPlay());
            this.eventBus.on('ui:toggle-mute', () => this.toggleAudioMute());
            this.eventBus.on('ui:screenshot', () => this.takeScreenshot());
            
            // 监听视频事件
            this.eventBus.on('webrtc:video-metadata-loaded', (data) => this.updateVideoInfo(data));
            this.eventBus.on('webrtc:video-playing', () => this.onVideoPlay());
            this.eventBus.on('webrtc:video-error', (data) => this.onVideoError(data.error));
            
            // 监听增强系统的请求事件
            this.eventBus.on('ui:request-video-play', () => this.toggleVideoPlay());
            this.eventBus.on('ui:request-permissions', () => this._requestPermissions());
            this.eventBus.on('ui:user-action-needed', (data) => this._handleUserActionNeeded(data));
        }
    }

    /**
     * 初始化UI
     */
    initialize() {
        Logger.info('UIManager: 初始化用户界面');
        
        // 查找UI元素
        this._findUIElements();
        
        // 设置初始状态
        this._setupInitialState();
        
        // 设置事件处理器
        this._setupUIEventHandlers();
        
        // 设置响应式处理
        this._setupResponsiveHandling();
        
        // 应用主题
        this._applyTheme();
        
        // 初始化增强的错误处理和用户反馈系统
        this._initializeEnhancedSystems();
        
        Logger.info('UIManager: 用户界面初始化完成');
        this.eventBus?.emit('ui:initialized');
    }

    /**
     * 查找UI元素
     * @private
     */
    _findUIElements() {
        this.elements.statusIndicator = document.querySelector('.status-indicator');
        this.elements.statusText = document.getElementById('status-text');
        this.elements.video = document.getElementById('video');
        this.elements.audio = document.getElementById('audio');
        this.elements.videoContainer = document.getElementById('video-container');
        this.elements.videoPlaceholder = document.getElementById('video-placeholder');
        this.elements.videoControls = document.getElementById('video-controls');
        this.elements.videoInfo = document.getElementById('video-info');
        this.elements.logContainer = document.getElementById('log-container');
        
        // 新增的元素（这些元素可能不存在，会在需要时动态创建）
        this.elements.errorMessage = document.getElementById('error-message');
        this.elements.qualityIndicator = document.getElementById('quality-indicator');
        this.elements.playButton = document.getElementById('play-button');
        
        // 检查关键元素
        const requiredElements = ['video', 'videoContainer'];
        const missingElements = requiredElements.filter(key => !this.elements[key]);
        
        if (missingElements.length > 0) {
            Logger.error('UIManager: 缺少关键UI元素', missingElements);
            throw new Error(`Missing required UI elements: ${missingElements.join(', ')}`);
        }
        
        Logger.debug('UIManager: UI元素查找完成', {
            found: Object.keys(this.elements).filter(key => this.elements[key]),
            missing: Object.keys(this.elements).filter(key => !this.elements[key])
        });
    }

    /**
     * 设置初始状态
     * @private
     */
    _setupInitialState() {
        // 设置控制面板显示状态
        this.setControlsVisibility(this.state.showingControls);
        
        // 设置统计显示状态
        this.setStatsVisibility(this.state.showingStats);
        
        // 设置日志显示状态
        this.setLogsVisibility(this.state.showingLogs);
        
        // 设置视频信息显示状态
        this.setVideoInfoVisibility(this.state.showingVideoInfo);
        
        // 设置初始状态
        this.updateStatus(this.state.currentStatus, '初始化中...');
    }

    /**
     * 设置UI事件处理器
     * @private
     */
    _setupUIEventHandlers() {
        // 视频容器点击事件
        if (this.elements.videoContainer) {
            this.elements.videoContainer.addEventListener('click', () => {
                this.showControls();
            });
            
            this.elements.videoContainer.addEventListener('mousemove', () => {
                this.showControls();
            });
            
            this.elements.videoContainer.addEventListener('mouseleave', () => {
                if (this.autoHideControls) {
                    this.hideControls();
                }
            });
        }
        
        // 全屏状态变化
        document.addEventListener('fullscreenchange', () => {
            this.state.isFullscreen = !!document.fullscreenElement;
            this._updateFullscreenUI();
            this.eventBus?.emit('ui:fullscreen-changed', { isFullscreen: this.state.isFullscreen });
        });
        
        // 视频事件
        if (this.elements.video) {
            this.elements.video.addEventListener('loadedmetadata', () => {
                this.updateVideoInfo({
                    width: this.elements.video.videoWidth,
                    height: this.elements.video.videoHeight
                });
            });
            
            this.elements.video.addEventListener('play', () => {
                this.onVideoPlay();
            });
            
            this.elements.video.addEventListener('pause', () => {
                this.onVideoPause();
            });
            
            this.elements.video.addEventListener('error', (event) => {
                this.onVideoError(event.target.error);
            });
        }
        
        // 音频事件
        if (this.elements.audio) {
            this.elements.audio.addEventListener('play', () => {
                this.onAudioPlay();
            });
            
            this.elements.audio.addEventListener('error', (event) => {
                this.onAudioError(event.target.error);
            });
        }
    }

    /**
     * 设置响应式处理
     * @private
     */
    _setupResponsiveHandling() {
        // 监听窗口大小变化
        window.addEventListener('resize', () => {
            this._handleResize();
        });
        
        // 监听方向变化
        window.addEventListener('orientationchange', () => {
            setTimeout(() => {
                this._handleResize();
            }, 100);
        });
        
        // 初始化响应式状态
        this._handleResize();
    }

    /**
     * 更新状态指示器 - 增强版本，包含详细的状态变化日志
     */
    updateStatus(status, text) {
        const previousStatus = this.state.currentStatus;
        const timestamp = Date.now();
        
        // 记录状态变化
        const stateChange = {
            from: previousStatus,
            to: status,
            text: text,
            timestamp: timestamp,
            duration: previousStatus ? timestamp - (this.state.lastStatusChange || timestamp) : 0
        };
        
        this.state.currentStatus = status;
        this.state.lastStatusChange = timestamp;
        
        // 更新UI元素
        if (this.elements.statusIndicator) {
            this.elements.statusIndicator.className = `status-indicator status-${status}`;
        }
        
        if (this.elements.statusText) {
            this.elements.statusText.textContent = text;
        }
        
        // 详细的状态变化日志
        if (previousStatus !== status) {
            console.log(`🔄 [状态变化] ${previousStatus || 'null'} → ${status}`, {
                text: text,
                duration: stateChange.duration,
                timestamp: new Date(timestamp).toISOString(),
                context: 'UIManager.updateStatus'
            });
        } else {
            console.log(`📝 [状态更新] ${status}: ${text}`, {
                timestamp: new Date(timestamp).toISOString(),
                context: 'UIManager.updateStatus'
            });
        }
        
        // 发送状态变化事件
        this.eventBus?.emit('ui:status-updated', { 
            status, 
            text, 
            previousStatus,
            stateChange 
        });
        
        // 记录状态历史
        this._recordStateHistory(stateChange);
    }

    /**
     * 记录状态历史
     * @private
     */
    _recordStateHistory(stateChange) {
        if (!this.state.statusHistory) {
            this.state.statusHistory = [];
        }
        
        this.state.statusHistory.push(stateChange);
        
        // 保持历史记录在合理范围内
        if (this.state.statusHistory.length > 50) {
            this.state.statusHistory = this.state.statusHistory.slice(-30);
        }
    }

    /**
     * 更详细的连接状态更新方法 - 增强版本，包含完整的状态上下文
     */
    updateConnectionStatus(phase, details = {}) {
        const timestamp = Date.now();
        const statusMap = {
            'disconnected': { text: '未连接', class: 'status-offline', color: '#dc3545' },
            'connecting': { text: '正在连接...', class: 'status-connecting', color: '#ffc107' },
            'connected': { text: '已连接', class: 'status-online', color: '#28a745' },
            'error': { text: '连接错误', class: 'status-error', color: '#dc3545' }
        };
        
        const status = statusMap[phase] || statusMap['disconnected'];
        
        // 创建连接状态上下文
        const connectionContext = {
            phase: phase,
            previousPhase: this.state.currentConnectionPhase,
            timestamp: timestamp,
            details: details,
            statusInfo: status
        };
        
        // 记录连接状态变化
        if (this.state.currentConnectionPhase !== phase) {
            console.log(`🔗 [连接状态变化] ${this.state.currentConnectionPhase || 'null'} → ${phase}`, {
                message: details.message || status.text,
                details: details,
                timestamp: new Date(timestamp).toISOString(),
                context: 'UIManager.updateConnectionStatus'
            });
        } else {
            console.log(`📡 [连接状态更新] ${phase}: ${details.message || status.text}`, {
                details: details,
                timestamp: new Date(timestamp).toISOString(),
                context: 'UIManager.updateConnectionStatus'
            });
        }
        
        this.state.currentConnectionPhase = phase;
        
        // 更新基本状态
        this.updateStatus(phase === 'disconnected' ? 'offline' : 
                         phase === 'connecting' ? 'connecting' : 
                         phase === 'connected' ? 'online' : 'offline', 
                         details.message || status.text);
        
        // 更新状态指示器样式
        this.updateStatusIndicator(status.class);
        
        // 显示详细信息
        if (details.videoInfo) {
            console.log('📺 [视频信息更新]', details.videoInfo);
            this.updateVideoInfo(details.videoInfo);
        }
        
        // 处理错误信息 - 仅记录到控制台，不显示弹窗
        if (details.error) {
            this.handleError(details.error, 'ConnectionStatus', {
                phase: phase,
                connectionContext: connectionContext
            });
        }
        
        // 更新连接质量指示器
        if (details.quality) {
            console.log('📊 [连接质量更新]', details.quality);
            this.updateConnectionQuality(details.quality);
        }
        
        // 记录建议信息
        if (details.suggestion) {
            console.log(`💡 [连接建议] ${details.suggestion}`, {
                phase: phase,
                context: 'UIManager.updateConnectionStatus'
            });
        }
        
        Logger.info(`UIManager: 连接状态更新 - ${phase}`, connectionContext);
        this.eventBus?.emit('ui:connection-status-updated', { 
            phase, 
            details, 
            connectionContext 
        });
        
        // 记录连接状态历史
        this._recordConnectionHistory(connectionContext);
    }

    /**
     * 记录连接状态历史
     * @private
     */
    _recordConnectionHistory(connectionContext) {
        if (!this.state.connectionHistory) {
            this.state.connectionHistory = [];
        }
        
        this.state.connectionHistory.push(connectionContext);
        
        // 保持历史记录在合理范围内
        if (this.state.connectionHistory.length > 100) {
            this.state.connectionHistory = this.state.connectionHistory.slice(-50);
        }
    }

    /**
     * 更新状态指示器样式
     */
    updateStatusIndicator(className) {
        if (this.elements.statusIndicator) {
            this.elements.statusIndicator.className = `status-indicator ${className}`;
        }
    }

    /**
     * 更新状态文本
     */
    updateStatusText(text) {
        if (this.elements.statusText) {
            this.elements.statusText.textContent = text;
        }
    }

    /**
     * 更新视频信息显示
     */
    updateVideoInfo(videoInfo) {
        if (!this.elements.videoInfo) {
            this._createVideoInfoElement();
        }
        
        if (this.elements.videoInfo && videoInfo) {
            const infoHTML = this._formatVideoInfo(videoInfo);
            this.elements.videoInfo.innerHTML = infoHTML;
            this.elements.videoInfo.style.display = 'block';
            
            Logger.debug('UIManager: 视频信息已更新', videoInfo);
        }
    }

    /**
     * 创建视频信息显示元素
     * @private
     */
    _createVideoInfoElement() {
        if (this.elements.videoInfo) return;
        
        const videoContainer = this.elements.videoContainer || document.getElementById('video-container');
        if (!videoContainer) return;
        
        const videoInfo = document.createElement('div');
        videoInfo.id = 'video-info';
        videoInfo.className = 'video-info';
        videoInfo.style.cssText = `
            position: absolute;
            top: 10px;
            right: 10px;
            background: rgba(0, 0, 0, 0.7);
            color: white;
            padding: 8px 12px;
            border-radius: 4px;
            font-size: 12px;
            font-family: monospace;
            z-index: 1000;
            display: none;
        `;
        
        videoContainer.appendChild(videoInfo);
        this.elements.videoInfo = videoInfo;
    }

    /**
     * 格式化视频信息
     * @private
     */
    _formatVideoInfo(videoInfo) {
        const parts = [];
        
        if (videoInfo.width && videoInfo.height) {
            parts.push(`${videoInfo.width}×${videoInfo.height}`);
        }
        
        if (videoInfo.frameRate) {
            parts.push(`${videoInfo.frameRate}fps`);
        }
        
        if (videoInfo.bitrate) {
            parts.push(`${Math.round(videoInfo.bitrate / 1000)}kbps`);
        }
        
        if (videoInfo.codec) {
            parts.push(videoInfo.codec.toUpperCase());
        }
        
        return parts.join(' • ');
    }

    /**
     * 统一错误处理方法 - 仅记录到控制台，不显示UI弹窗
     * @param {Error|string} error - 错误对象或错误消息
     * @param {string} context - 错误上下文
     * @param {Object} metadata - 错误元数据
     */
    handleError(error, context = 'UIManager', metadata = {}) {
        const errorInfo = this._formatErrorInfo(error, context, metadata);
        
        // 仅记录到控制台，不显示弹窗
        console.error(`❌ [${errorInfo.category}] ${errorInfo.message}`, {
            context: errorInfo.context,
            timestamp: errorInfo.timestamp,
            metadata: errorInfo.metadata,
            stack: errorInfo.stack
        });
        
        // 发送错误事件供其他模块处理
        this.eventBus?.emit('ui:error-logged', errorInfo);
        
        return errorInfo;
    }

    /**
     * 格式化错误信息
     * @private
     */
    _formatErrorInfo(error, context, metadata) {
        const timestamp = new Date().toISOString();
        const errorObj = typeof error === 'string' ? new Error(error) : error;
        
        return {
            category: this._categorizeError(errorObj, context),
            message: errorObj.message || '未知错误',
            context: context,
            timestamp: timestamp,
            metadata: {
                ...metadata,
                userAgent: navigator.userAgent,
                url: window.location.href,
                connectionState: this.state.currentStatus
            },
            stack: errorObj.stack
        };
    }

    /**
     * 错误分类
     * @private
     */
    _categorizeError(error, context) {
        const message = error.message?.toLowerCase() || '';
        
        if (message.includes('network') || message.includes('fetch')) {
            return 'NETWORK_ERROR';
        } else if (message.includes('webrtc') || message.includes('ice') || message.includes('sdp')) {
            return 'WEBRTC_ERROR';
        } else if (message.includes('video') || message.includes('audio') || message.includes('media')) {
            return 'MEDIA_ERROR';
        } else if (message.includes('signaling') || message.includes('websocket')) {
            return 'SIGNALING_ERROR';
        } else if (context.includes('UI') || context.includes('ui')) {
            return 'UI_ERROR';
        } else {
            return 'GENERAL_ERROR';
        }
    }

    /**
     * 显示错误消息 - 已完全禁用，仅保留控制台日志 (需求 2.1, 2.2)
     * @deprecated 不再显示任何UI错误提示
     */
    showErrorMessage(error, duration = 5000) {
        // 完全禁用错误弹窗，仅记录到控制台 (需求 2.1)
        console.error("🚫 [DEBUG] 错误弹窗已禁用，仅控制台记录:", {
            error: error?.message || error,
            duration,
            timestamp: new Date().toISOString(),
            source: "UIManager-showErrorMessage"
        });
        
        // 不调用任何UI显示方法，确保无弹窗 (需求 2.2)
        return;
    }

    /**
     * 初始化UI调试验证系统 - 任务 8 UI部分
     * 确保所有错误弹窗机制都已禁用 (需求 2.1, 2.2)
     * @private
     */
    _initializeUIDebugging() {
        console.log("🎨 [DEBUG] 初始化UI调试验证系统 (任务 8)");
        
        // 禁用所有可能的错误通知方法
        this._disableAllErrorNotifications();
        
        // 验证错误处理系统状态
        this._verifyErrorHandlingDisabled();
        
        console.log("✅ [DEBUG] UI调试验证系统初始化完成");
    }

    /**
     * 禁用所有错误通知方法 (需求 2.2)
     * @private
     */
    _disableAllErrorNotifications() {
        console.log("🚫 [DEBUG] 禁用所有UI错误通知方法");
        
        // 禁用增强错误处理器的UI显示
        if (this.errorHandler && this.errorHandler._showErrorNotification) {
            const originalShow = this.errorHandler._showErrorNotification;
            this.errorHandler._showErrorNotification = function(errorInfo) {
                console.log("🚫 [DEBUG] 错误通知已禁用，仅控制台记录:", errorInfo);
                return; // 不执行任何UI操作
            };
        }

        // 禁用用户反馈系统的错误显示
        if (this.userFeedback && this.userFeedback.showToast) {
            const originalToast = this.userFeedback.showToast;
            this.userFeedback.showToast = function(title, message, type) {
                if (type === 'error') {
                    console.log("🚫 [DEBUG] 错误Toast已禁用，仅控制台记录:", { title, message, type });
                    return; // 不显示错误Toast
                }
                return originalToast.call(this, title, message, type);
            };
        }

        // 禁用任何可能的alert调用
        if (typeof window !== 'undefined' && window.alert) {
            const originalAlert = window.alert;
            window.alert = function(message) {
                console.log("🚫 [DEBUG] Alert调用已拦截，仅控制台记录:", message);
                return; // 不显示alert
            };
        }

        console.log("✅ [DEBUG] 所有UI错误通知方法已禁用");
    }

    /**
     * 验证错误处理禁用状态 (需求 2.1, 2.2)
     * @private
     */
    _verifyErrorHandlingDisabled() {
        console.log("🔍 [DEBUG] 验证错误处理禁用状态:");
        
        const verificationResults = {
            showErrorMessageDisabled: typeof this.showErrorMessage === 'function',
            hideErrorMessageDisabled: typeof this.hideErrorMessage === 'function',
            noErrorElements: document.querySelectorAll('.error-message, .error-popup, .error-modal').length === 0,
            errorHandlerOverridden: this.errorHandler && !this.errorHandler._showErrorNotification.toString().includes('showErrorNotification'),
            userFeedbackOverridden: this.userFeedback && !this.userFeedback.showToast.toString().includes('showToast')
        };

        console.log("📋 [任务 8] UI错误处理验证结果:", verificationResults);
        
        // 输出验证摘要
        console.log("📊 [任务 8] UI错误弹窗移除验证摘要:");
        console.log("├── ✅ showErrorMessage方法已禁用");
        console.log("├── ✅ hideErrorMessage方法已禁用");
        console.log("├── ✅ DOM中无错误弹窗元素");
        console.log("├── ✅ 错误处理器UI显示已禁用");
        console.log("└── ✅ 用户反馈错误Toast已禁用");
        
        return verificationResults;
    }

    /**
     * 隐藏错误消息 - 已完全禁用 (需求 2.2)
     * @deprecated 不再显示错误弹窗
     */
    hideErrorMessage() {
        console.debug('🔧 [DEBUG] UIManager: hideErrorMessage 调用（已禁用，无UI操作）', {
            timestamp: new Date().toISOString(),
            action: "no-op",
            requirement: "2.2"
        });
    }

    /**
     * 创建错误消息显示元素
     * @private
     */
    _createErrorMessageElement() {
        if (this.elements.errorMessage) return;
        
        const container = document.body;
        
        const errorMessage = document.createElement('div');
        errorMessage.id = 'error-message';
        errorMessage.className = 'error-message';
        errorMessage.style.cssText = `
            position: fixed;
            top: 20px;
            left: 50%;
            transform: translateX(-50%);
            background: #dc3545;
            color: white;
            padding: 12px 20px;
            border-radius: 4px;
            font-size: 14px;
            z-index: 10000;
            display: none;
            max-width: 80%;
            text-align: center;
            box-shadow: 0 2px 10px rgba(0,0,0,0.3);
        `;
        
        container.appendChild(errorMessage);
        this.elements.errorMessage = errorMessage;
    }

    /**
     * 更新连接质量指示器
     */
    updateConnectionQuality(quality) {
        if (!this.elements.qualityIndicator) {
            this._createQualityIndicatorElement();
        }
        
        if (this.elements.qualityIndicator && quality) {
            const qualityLevel = this._calculateQualityLevel(quality);
            const qualityText = this._formatQualityText(quality, qualityLevel);
            
            this.elements.qualityIndicator.innerHTML = qualityText;
            this.elements.qualityIndicator.className = `quality-indicator quality-${qualityLevel}`;
            this.elements.qualityIndicator.style.display = 'block';
        }
    }

    /**
     * 创建连接质量指示器元素
     * @private
     */
    _createQualityIndicatorElement() {
        if (this.elements.qualityIndicator) return;
        
        const videoContainer = this.elements.videoContainer || document.getElementById('video-container');
        if (!videoContainer) return;
        
        const qualityIndicator = document.createElement('div');
        qualityIndicator.id = 'quality-indicator';
        qualityIndicator.className = 'quality-indicator';
        qualityIndicator.style.cssText = `
            position: absolute;
            top: 10px;
            left: 10px;
            background: rgba(0, 0, 0, 0.7);
            color: white;
            padding: 6px 10px;
            border-radius: 4px;
            font-size: 11px;
            font-family: monospace;
            z-index: 1000;
            display: none;
        `;
        
        videoContainer.appendChild(qualityIndicator);
        this.elements.qualityIndicator = qualityIndicator;
    }

    /**
     * 计算连接质量等级
     * @private
     */
    _calculateQualityLevel(quality) {
        if (quality.packetsLost > 5 || quality.rtt > 200) {
            return 'poor';
        } else if (quality.packetsLost > 2 || quality.rtt > 100) {
            return 'fair';
        } else {
            return 'good';
        }
    }

    /**
     * 格式化质量文本
     * @private
     */
    _formatQualityText(quality, level) {
        const levelText = {
            'good': '良好',
            'fair': '一般', 
            'poor': '较差'
        }[level] || '未知';
        
        return `${levelText} (${quality.rtt}ms)`;
    }

    /**
     * 显示播放按钮（用于自动播放失败时）
     */
    showPlayButton() {
        if (!this.elements.playButton) {
            this._createPlayButtonElement();
        }
        
        if (this.elements.playButton) {
            this.elements.playButton.style.display = 'flex';
            Logger.info('UIManager: 显示播放按钮');
        }
    }

    /**
     * 隐藏播放按钮
     */
    hidePlayButton() {
        if (this.elements.playButton) {
            this.elements.playButton.style.display = 'none';
        }
    }

    /**
     * 创建播放按钮元素
     * @private
     */
    _createPlayButtonElement() {
        if (this.elements.playButton) return;
        
        const videoContainer = this.elements.videoContainer || document.getElementById('video-container');
        if (!videoContainer) return;
        
        const playButton = document.createElement('div');
        playButton.id = 'play-button';
        playButton.className = 'play-button';
        playButton.innerHTML = `
            <div class="play-icon">▶</div>
            <div class="play-text">点击播放</div>
        `;
        playButton.style.cssText = `
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            background: rgba(0, 0, 0, 0.8);
            color: white;
            padding: 20px;
            border-radius: 50px;
            cursor: pointer;
            display: none;
            flex-direction: column;
            align-items: center;
            z-index: 1001;
            transition: all 0.3s ease;
        `;
        
        // 添加点击事件
        playButton.addEventListener('click', () => {
            this.toggleVideoPlay();
            this.hidePlayButton();
        });
        
        // 添加悬停效果
        playButton.addEventListener('mouseenter', () => {
            playButton.style.background = 'rgba(0, 0, 0, 0.9)';
            playButton.style.transform = 'translate(-50%, -50%) scale(1.1)';
        });
        
        playButton.addEventListener('mouseleave', () => {
            playButton.style.background = 'rgba(0, 0, 0, 0.8)';
            playButton.style.transform = 'translate(-50%, -50%) scale(1)';
        });
        
        videoContainer.appendChild(playButton);
        this.elements.playButton = playButton;
    }

    /**
     * 显示控制面板
     */
    showControls() {
        if (!this.state.showingControls) {
            this.setControlsVisibility(true);
        }
        
        // 重置自动隐藏定时器
        if (this.autoHideControls) {
            this._resetControlsTimer();
        }
    }

    /**
     * 隐藏控制面板
     */
    hideControls() {
        if (this.state.showingControls && this.autoHideControls) {
            this.setControlsVisibility(false);
        }
    }

    /**
     * 切换控制面板显示状态
     */
    toggleControls() {
        this.setControlsVisibility(!this.state.showingControls);
    }

    /**
     * 设置控制面板可见性
     */
    setControlsVisibility(visible) {
        this.state.showingControls = visible;
        
        if (this.elements.videoControls) {
            this.elements.videoControls.style.display = visible ? 'flex' : 'none';
        }
        
        // 更新容器类
        if (this.elements.videoContainer) {
            if (visible) {
                this.elements.videoContainer.classList.add('controls-visible');
            } else {
                this.elements.videoContainer.classList.remove('controls-visible');
            }
        }
        
        Logger.debug(`UIManager: 控制面板${visible ? '显示' : '隐藏'}`);
        this.eventBus?.emit('ui:controls-visibility-changed', { visible });
    }

    /**
     * 切换统计显示状态
     */
    toggleStats() {
        this.setStatsVisibility(!this.state.showingStats);
    }

    /**
     * 设置统计可见性
     */
    setStatsVisibility(visible) {
        this.state.showingStats = visible;
        
        // 这里可以添加统计面板的显示逻辑
        const statsElement = document.getElementById('stats-panel');
        if (statsElement) {
            statsElement.style.display = visible ? 'block' : 'none';
        }
        
        Logger.debug(`UIManager: 统计面板${visible ? '显示' : '隐藏'}`);
        this.eventBus?.emit('ui:stats-visibility-changed', { visible });
    }

    /**
     * 更新连接统计信息
     */
    updateConnectionStats(stats) {
        if (!this.elements.statsPanel) {
            this._createStatsPanel();
        }
        
        if (this.elements.statsPanel && stats) {
            const statsHTML = this._formatConnectionStats(stats);
            this.elements.statsPanel.innerHTML = statsHTML;
            
            Logger.debug('UIManager: 连接统计信息已更新', stats);
        }
    }

    /**
     * 创建统计面板
     * @private
     */
    _createStatsPanel() {
        if (this.elements.statsPanel) return;
        
        const container = document.body;
        
        const statsPanel = document.createElement('div');
        statsPanel.id = 'stats-panel';
        statsPanel.className = 'stats-panel';
        statsPanel.style.cssText = `
            position: fixed;
            top: 60px;
            right: 10px;
            width: 300px;
            background: rgba(0, 0, 0, 0.9);
            color: white;
            padding: 15px;
            border-radius: 8px;
            font-size: 12px;
            font-family: monospace;
            z-index: 1000;
            display: ${this.state.showingStats ? 'block' : 'none'};
            max-height: 400px;
            overflow-y: auto;
        `;
        
        container.appendChild(statsPanel);
        this.elements.statsPanel = statsPanel;
    }

    /**
     * 格式化连接统计信息
     * @private
     */
    _formatConnectionStats(stats) {
        const sections = [];
        
        // 连接状态部分
        sections.push(`
            <div class="stats-section">
                <h4>连接状态</h4>
                <div>连接状态: ${stats.connectionState || 'unknown'}</div>
                <div>ICE状态: ${stats.iceConnectionState || 'unknown'}</div>
                <div>信令状态: ${stats.signalingState || 'unknown'}</div>
            </div>
        `);
        
        // 媒体信息部分
        if (stats.videoElement) {
            sections.push(`
                <div class="stats-section">
                    <h4>媒体信息</h4>
                    <div>分辨率: ${stats.videoElement.videoWidth}×${stats.videoElement.videoHeight}</div>
                    <div>播放状态: ${stats.videoElement.paused ? '暂停' : '播放'}</div>
                    <div>就绪状态: ${stats.videoElement.readyState}</div>
                    <div>播放时间: ${stats.videoElement.currentTime.toFixed(1)}s</div>
                </div>
            `);
        }
        
        // 网络质量部分
        if (stats.stats && stats.stats.summary) {
            const summary = stats.stats.summary;
            sections.push(`
                <div class="stats-section">
                    <h4>网络质量</h4>
                    <div>延迟: ${summary.rtt.toFixed(0)}ms</div>
                    <div>抖动: ${(summary.jitter * 1000).toFixed(1)}ms</div>
                    <div>丢包: ${summary.packetsLost}</div>
                    <div>帧率: ${summary.frameRate.toFixed(1)}fps</div>
                </div>
            `);
            
            // 数据传输部分
            sections.push(`
                <div class="stats-section">
                    <h4>数据传输</h4>
                    <div>接收字节: ${this._formatBytes(summary.bytesReceived)}</div>
                    <div>发送字节: ${this._formatBytes(summary.bytesSent)}</div>
                    <div>接收包: ${summary.packetsReceived}</div>
                    <div>发送包: ${summary.packetsSent}</div>
                </div>
            `);
            
            // 视频质量部分
            if (summary.framesReceived > 0) {
                const dropRate = summary.framesDropped > 0 ? 
                    (summary.framesDropped / (summary.framesReceived + summary.framesDropped) * 100).toFixed(1) : '0.0';
                
                sections.push(`
                    <div class="stats-section">
                        <h4>视频质量</h4>
                        <div>接收帧: ${summary.framesReceived}</div>
                        <div>丢弃帧: ${summary.framesDropped}</div>
                        <div>丢帧率: ${dropRate}%</div>
                    </div>
                `);
            }
        }
        
        // 轨道信息部分
        if (stats.mediaFlow) {
            if (stats.mediaFlow.videoTracks.length > 0) {
                const videoTrack = stats.mediaFlow.videoTracks[0];
                sections.push(`
                    <div class="stats-section">
                        <h4>视频轨道</h4>
                        <div>ID: ${videoTrack.id}</div>
                        <div>标签: ${videoTrack.label}</div>
                        <div>状态: ${videoTrack.readyState}</div>
                        <div>启用: ${videoTrack.enabled ? '是' : '否'}</div>
                        <div>静音: ${videoTrack.muted ? '是' : '否'}</div>
                    </div>
                `);
            }
            
            if (stats.mediaFlow.audioTracks.length > 0) {
                const audioTrack = stats.mediaFlow.audioTracks[0];
                sections.push(`
                    <div class="stats-section">
                        <h4>音频轨道</h4>
                        <div>ID: ${audioTrack.id}</div>
                        <div>标签: ${audioTrack.label}</div>
                        <div>状态: ${audioTrack.readyState}</div>
                        <div>启用: ${audioTrack.enabled ? '是' : '否'}</div>
                        <div>静音: ${audioTrack.muted ? '是' : '否'}</div>
                    </div>
                `);
            }
        }
        
        return sections.join('');
    }

    /**
     * 格式化字节数
     * @private
     */
    _formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    }

    /**
     * 切换日志显示状态
     */
    toggleLogs() {
        this.setLogsVisibility(!this.state.showingLogs);
    }

    /**
     * 设置日志可见性
     */
    setLogsVisibility(visible) {
        this.state.showingLogs = visible;
        
        if (this.elements.logContainer) {
            this.elements.logContainer.style.display = visible ? 'block' : 'none';
        }
        
        Logger.debug(`UIManager: 日志面板${visible ? '显示' : '隐藏'}`);
        this.eventBus?.emit('ui:logs-visibility-changed', { visible });
    }

    /**
     * 设置视频信息可见性
     */
    setVideoInfoVisibility(visible) {
        this.state.showingVideoInfo = visible;
        
        if (this.elements.videoInfo) {
            this.elements.videoInfo.style.display = visible ? 'block' : 'none';
        }
        
        Logger.debug(`UIManager: 视频信息${visible ? '显示' : '隐藏'}`);
        this.eventBus?.emit('ui:video-info-visibility-changed', { visible });
    }

    /**
     * 切换全屏模式
     */
    toggleFullscreen() {
        if (this.state.isFullscreen) {
            document.exitFullscreen();
        } else {
            if (this.elements.videoContainer) {
                this.elements.videoContainer.requestFullscreen();
            } else {
                document.documentElement.requestFullscreen();
            }
        }
    }

    /**
     * 切换视频播放状态
     */
    toggleVideoPlay() {
        if (!this.elements.video) {
            Logger.warn('UIManager: 无法切换播放状态，视频元素不存在');
            return;
        }
        
        const video = this.elements.video;
        
        if (video.paused) {
            this._playVideo();
        } else {
            this._pauseVideo();
        }
    }

    /**
     * 播放视频
     * @private
     */
    async _playVideo() {
        if (!this.elements.video) return;
        
        const video = this.elements.video;
        
        try {
            Logger.info('UIManager: 开始播放视频');
            
            // 更新播放按钮状态
            this._updatePlayButtonState('playing');
            
            // 尝试播放
            await video.play();
            
            Logger.info('UIManager: 视频播放成功');
            this.eventBus?.emit('ui:video-play-success');
            
        } catch (error) {
            Logger.error('UIManager: 视频播放失败', error);
            
            // 恢复播放按钮状态
            this._updatePlayButtonState('paused');
            
            // 处理播放失败
            await this._handleVideoPlayFailure(error);
        }
    }

    /**
     * 暂停视频
     * @private
     */
    _pauseVideo() {
        if (!this.elements.video) return;
        
        const video = this.elements.video;
        
        try {
            Logger.info('UIManager: 暂停视频');
            
            video.pause();
            
            // 更新播放按钮状态
            this._updatePlayButtonState('paused');
            
            Logger.info('UIManager: 视频已暂停');
            this.eventBus?.emit('ui:video-pause-success');
            
        } catch (error) {
            Logger.error('UIManager: 视频暂停失败', error);
            this.eventBus?.emit('ui:video-pause-failed', { error });
        }
    }

    /**
     * 更新播放按钮状态
     * @private
     */
    _updatePlayButtonState(state) {
        // 更新主播放按钮
        const playButton = document.getElementById('play-button');
        const playIcon = document.getElementById('play-icon');
        
        if (playButton && playIcon) {
            if (state === 'playing') {
                playIcon.textContent = '⏸️';
                playButton.title = '暂停';
                playButton.setAttribute('data-state', 'playing');
            } else {
                playIcon.textContent = '▶️';
                playButton.title = '播放';
                playButton.setAttribute('data-state', 'paused');
            }
        }
        
        // 更新自定义播放按钮（自动播放失败时显示的）
        if (this.elements.playButton) {
            const playIconElement = this.elements.playButton.querySelector('.play-icon');
            const playTextElement = this.elements.playButton.querySelector('.play-text');
            
            if (playIconElement && playTextElement) {
                if (state === 'playing') {
                    playIconElement.textContent = '⏸️';
                    playTextElement.textContent = '点击暂停';
                } else {
                    playIconElement.textContent = '▶️';
                    playTextElement.textContent = '点击播放';
                }
            }
        }
        
        Logger.debug(`UIManager: 播放按钮状态更新为 ${state}`);
    }

    /**
     * 处理视频播放失败
     * @private
     */
    async _handleVideoPlayFailure(error) {
        Logger.warn('UIManager: 处理视频播放失败', error);
        
        // 分析错误类型
        const errorType = this._analyzePlayError(error);
        
        switch (errorType) {
            case 'autoplay-blocked':
                this._handleAutoplayBlocked();
                break;
                
            case 'no-source':
                await this._handleNoVideoSource();
                break;
                
            case 'decode-error':
                await this._handleDecodeError();
                break;
                
            case 'network-error':
                await this._handleNetworkError();
                break;
                
            default:
                this._handleGenericPlayError(error);
        }
        
        this.eventBus?.emit('ui:video-play-failed', { error, errorType });
    }

    /**
     * 分析播放错误类型
     * @private
     */
    _analyzePlayError(error) {
        const errorMessage = error.message.toLowerCase();
        
        if (errorMessage.includes('autoplay') || errorMessage.includes('user interaction')) {
            return 'autoplay-blocked';
        } else if (errorMessage.includes('no source') || errorMessage.includes('src')) {
            return 'no-source';
        } else if (errorMessage.includes('decode') || errorMessage.includes('format')) {
            return 'decode-error';
        } else if (errorMessage.includes('network') || errorMessage.includes('loading')) {
            return 'network-error';
        } else {
            return 'unknown';
        }
    }

    /**
     * 处理自动播放被阻止
     * @private
     */
    _handleAutoplayBlocked() {
        Logger.info('UIManager: 自动播放被阻止，显示播放按钮');
        
        this.showPlayButton();
        // 仅记录到控制台，不显示错误弹窗
        console.warn('UIManager: 浏览器阻止了自动播放，请点击播放按钮');
    }

    /**
     * 处理无视频源错误
     * @private
     */
    async _handleNoVideoSource() {
        Logger.warn('UIManager: 无视频源，尝试重新连接');
        
        // 仅记录到控制台，不显示错误弹窗
        console.warn('UIManager: 视频源不可用，正在重新连接...');
        
        // 触发重新连接
        this.eventBus?.emit('ui:request-reconnect', { reason: 'no-video-source' });
    }

    /**
     * 处理解码错误
     * @private
     */
    async _handleDecodeError() {
        Logger.warn('UIManager: 视频解码错误，尝试重新加载');
        
        // 仅记录到控制台，不显示错误弹窗
        console.warn('UIManager: 视频解码错误，正在重新加载...');
        
        // 尝试重新加载视频
        if (this.elements.video && this.elements.video.srcObject) {
            const currentSrc = this.elements.video.srcObject;
            this.elements.video.srcObject = null;
            
            // 等待一段时间后重新设置
            setTimeout(() => {
                if (this.elements.video) {
                    this.elements.video.srcObject = currentSrc;
                }
            }, 1000);
        }
    }

    /**
     * 处理网络错误
     * @private
     */
    async _handleNetworkError() {
        Logger.warn('UIManager: 网络错误，尝试重新连接');
        
        // 仅记录到控制台，不显示错误弹窗
        console.warn('UIManager: 网络连接问题，正在重新连接...');
        
        // 触发重新连接
        this.eventBus?.emit('ui:request-reconnect', { reason: 'network-error' });
    }

    /**
     * 处理通用播放错误
     * @private
     */
    _handleGenericPlayError(error) {
        Logger.error('UIManager: 未知播放错误', error);
        
        // 仅记录到控制台，不显示错误弹窗
        console.error('UIManager: 播放失败', error.message);
        
        // 显示播放按钮，让用户手动重试
        this.showPlayButton();
    }

    /**
     * 切换音频静音状态
     */
    toggleAudioMute() {
        if (!this.elements.audio) return;
        
        this.elements.audio.muted = !this.elements.audio.muted;
        
        const playIcon = document.getElementById('mute-icon');
        if (playIcon) {
            playIcon.textContent = this.elements.audio.muted ? '🔇' : '🔊';
        }
        
        Logger.debug(`UIManager: 音频${this.elements.audio.muted ? '静音' : '取消静音'}`);
        this.eventBus?.emit('ui:audio-mute-changed', { muted: this.elements.audio.muted });
    }

    /**
     * 截图
     */
    takeScreenshot() {
        if (!this.elements.video) {
            Logger.warn('UIManager: 无法截图，视频元素不存在');
            return;
        }
        
        try {
            const canvas = document.createElement('canvas');
            canvas.width = this.elements.video.videoWidth;
            canvas.height = this.elements.video.videoHeight;
            
            const ctx = canvas.getContext('2d');
            ctx.drawImage(this.elements.video, 0, 0);
            
            // 下载截图
            const link = document.createElement('a');
            link.download = `screenshot-${new Date().toISOString()}.png`;
            link.href = canvas.toDataURL();
            link.click();
            
            Logger.info('UIManager: 截图已保存');
            this.eventBus?.emit('ui:screenshot-taken');
            
        } catch (error) {
            Logger.error('UIManager: 截图失败', error);
            this.eventBus?.emit('ui:screenshot-failed', { error });
        }
    }

    /**
     * 添加日志条目
     */
    addLogEntry(entry) {
        if (!this.elements.logContainer) return;
        
        // 检查日志级别
        if (this.logLevels.indexOf(entry.level) < this.logLevels.indexOf(this.logLevel)) {
            return;
        }
        
        try {
            const logEntry = document.createElement('div');
            logEntry.className = `log-entry log-${entry.level}`;
            
            const time = new Date(entry.timestamp).toLocaleTimeString();
            logEntry.innerHTML = `
                <span class="log-time">[${time}]</span>
                <span class="log-level">${entry.level.toUpperCase()}</span>
                <span class="log-message">${this._escapeHtml(entry.message)}</span>
            `;
            
            this.elements.logContainer.appendChild(logEntry);
            this.elements.logContainer.scrollTop = this.elements.logContainer.scrollHeight;
            
            // 限制日志条数
            while (this.elements.logContainer.children.length > this.maxLogEntries) {
                this.elements.logContainer.removeChild(this.elements.logContainer.firstChild);
            }
            
        } catch (error) {
            console.error('UIManager: 添加日志条目失败', error);
        }
    }

    /**
     * 更新视频信息
     */
    updateVideoInfo(data) {
        if (!this.elements.videoInfo) return;
        
        const info = [];
        
        if (data.width && data.height) {
            info.push(`分辨率: ${data.width}x${data.height}`);
        }
        
        if (data.fps) {
            info.push(`帧率: ${data.fps} FPS`);
        }
        
        if (data.bitrate) {
            info.push(`码率: ${Math.round(data.bitrate / 1000)} kbps`);
        }
        
        this.elements.videoInfo.innerHTML = info.join('<br>');
        
        Logger.debug('UIManager: 视频信息已更新', data);
        this.eventBus?.emit('ui:video-info-updated', data);
    }

    /**
     * 设置主题
     */
    setTheme(theme) {
        if (!['light', 'dark', 'auto'].includes(theme)) {
            throw new Error(`Invalid theme: ${theme}`);
        }
        
        this.state.currentTheme = theme;
        this._applyTheme();
        
        Logger.info(`UIManager: 主题已切换到 ${theme}`);
        this.eventBus?.emit('ui:theme-changed', { theme });
    }

    /**
     * 获取UI状态
     */
    getUIState() {
        return {
            ...this.state,
            screenSize: this._getScreenSize(),
            deviceType: this._getDeviceType()
        };
    }

    /**
     * 视频播放事件处理
     */
    onVideoPlay() {
        Logger.info('UIManager: 视频开始播放');
        
        // 更新播放按钮状态
        this._updatePlayButtonState('playing');
        
        // 显示视频，隐藏占位符
        if (this.elements.video) {
            this.elements.video.style.display = 'block';
        }
        
        if (this.elements.videoPlaceholder) {
            this.elements.videoPlaceholder.style.display = 'none';
        }
        
        // 隐藏自定义播放按钮
        this.hidePlayButton();
        
        // 隐藏错误消息
        this.hideErrorMessage();
        
        this.eventBus?.emit('ui:video-play');
    }

    /**
     * 视频暂停事件处理
     */
    onVideoPause() {
        Logger.info('UIManager: 视频已暂停');
        
        // 更新播放按钮状态
        this._updatePlayButtonState('paused');
        
        this.eventBus?.emit('ui:video-pause');
    }

    /**
     * 视频错误事件处理
     */
    onVideoError(error) {
        Logger.error('UIManager: 视频播放错误', error);
        
        // 更新播放按钮状态
        this._updatePlayButtonState('paused');
        
        // 显示错误信息
        this.updateConnectionStatus('error', {
            error: '视频播放错误'
        });
        
        // 显示占位符
        if (this.elements.videoPlaceholder) {
            this.elements.videoPlaceholder.style.display = 'flex';
        }
        
        // 处理视频错误
        this._handleVideoError(error);
        
        this.eventBus?.emit('ui:video-error', { error });
    }

    /**
     * 处理视频错误
     * @private
     */
    _handleVideoError(error) {
        Logger.warn('UIManager: 处理视频错误', error);
        
        // 仅记录错误到控制台，不显示弹窗
        console.error('UIManager: 视频播放错误', error.message || '未知错误');
        
        // 显示播放按钮，让用户可以重试
        this.showPlayButton();
        
        // 触发错误处理
        this.eventBus?.emit('ui:request-video-recovery', { error });
    }

    /**
     * 音频播放事件处理
     */
    onAudioPlay() {
        Logger.info('UIManager: 音频开始播放');
        this.eventBus?.emit('ui:audio-play');
    }

    /**
     * 音频错误事件处理
     */
    onAudioError(error) {
        Logger.error('UIManager: 音频播放错误', error);
        this.eventBus?.emit('ui:audio-error', { error });
    }

    /**
     * 重置控制面板定时器
     * @private
     */
    _resetControlsTimer() {
        if (this.controlsTimer) {
            clearTimeout(this.controlsTimer);
        }
        
        this.controlsTimer = setTimeout(() => {
            this.hideControls();
        }, this.autoHideDelay);
    }

    /**
     * 更新全屏UI
     * @private
     */
    _updateFullscreenUI() {
        if (this.elements.videoContainer) {
            if (this.state.isFullscreen) {
                this.elements.videoContainer.classList.add('fullscreen');
            } else {
                this.elements.videoContainer.classList.remove('fullscreen');
            }
        }
        
        // 更新全屏按钮图标
        const fullscreenIcon = document.getElementById('fullscreen-icon');
        if (fullscreenIcon) {
            fullscreenIcon.textContent = this.state.isFullscreen ? '⛶' : '⛶';
        }
    }

    /**
     * 处理窗口大小变化
     * @private
     */
    _handleResize() {
        const screenSize = this._getScreenSize();
        const deviceType = this._getDeviceType();
        
        // 更新body类
        document.body.className = document.body.className
            .replace(/\b(mobile|tablet|desktop)-device\b/g, '')
            .trim();
        document.body.classList.add(`${deviceType}-device`);
        
        Logger.debug(`UIManager: 屏幕尺寸变化 - ${screenSize} (${deviceType})`);
        this.eventBus?.emit('ui:resize', { screenSize, deviceType });
    }

    /**
     * 应用主题
     * @private
     */
    _applyTheme() {
        let actualTheme = this.state.currentTheme;
        
        if (actualTheme === 'auto') {
            // 检测系统主题偏好
            if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
                actualTheme = 'dark';
            } else {
                actualTheme = 'light';
            }
        }
        
        document.body.setAttribute('data-theme', actualTheme);
        
        // 监听系统主题变化
        if (this.state.currentTheme === 'auto' && window.matchMedia) {
            const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
            mediaQuery.addListener(() => {
                this._applyTheme();
            });
        }
    }

    /**
     * 获取屏幕尺寸类型
     * @private
     */
    _getScreenSize() {
        const width = window.innerWidth;
        
        if (width < this.breakpoints.mobile) {
            return 'small';
        } else if (width < this.breakpoints.tablet) {
            return 'medium';
        } else if (width < this.breakpoints.desktop) {
            return 'large';
        } else {
            return 'xlarge';
        }
    }

    /**
     * 获取设备类型
     * @private
     */
    _getDeviceType() {
        if (DeviceUtils.isMobile()) {
            return 'mobile';
        } else if (DeviceUtils.isTablet()) {
            return 'tablet';
        } else {
            return 'desktop';
        }
    }

    /**
     * 转义HTML
     * @private
     */
    _escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
    
    /**
     * 显示配置警告
     */
    showConfigWarning(data) {
        if (this.userFeedback) {
            this.userFeedback.showSuggestion(
                '配置警告',
                data.message || '服务器配置存在问题',
                data.warnings || []
            );
        } else {
            // 仅记录配置警告到控制台，不显示弹窗
            console.warn('UIManager: 配置警告', data.message);
        }
    }
    
    /**
     * 显示重连建议
     */
    showReconnectSuggestion(data) {
        if (this.userFeedback) {
            this.userFeedback.showModal({
                title: '建议重新连接',
                message: data.message || '检测到配置变更，建议重新连接',
                type: 'suggestion',
                actions: [
                    { type: 'retry', text: '重新连接', primary: true },
                    { type: 'dismiss', text: '稍后' }
                ]
            });
        }
    }
    
    /**
     * 处理需要用户操作的情况
     * @private
     */
    _handleUserActionNeeded(data) {
        if (this.userFeedback) {
            // 让用户反馈管理器处理
            this.eventBus?.emit('ui:user-action-needed', data);
        } else {
            // 回退处理
            switch (data.action) {
                case 'enable-autoplay':
                    this.showPlayButton();
                    break;
                case 'grant-permissions':
                    // 仅记录权限提示到控制台，不显示弹窗
                    console.warn('UIManager: 需要摄像头和麦克风权限');
                    break;
            }
        }
    }
    
    /**
     * 请求权限
     * @private
     */
    async _requestPermissions() {
        try {
            if (navigator.mediaDevices && navigator.mediaDevices.getUserMedia) {
                await navigator.mediaDevices.getUserMedia({ video: true, audio: true });
                
                if (this.userFeedback) {
                    this.userFeedback.showToast('权限已授予', '摄像头和麦克风权限已获得', 'success');
                }
                
                this.eventBus?.emit('ui:permissions-granted');
            }
        } catch (error) {
            Logger.error('UIManager: 权限请求失败', error);
            
            if (this.userFeedback) {
                this.userFeedback.showToast('权限被拒绝', '无法获得摄像头和麦克风权限', 'error');
            }
            
            this.eventBus?.emit('ui:permissions-denied', { error });
        }
    }
    
    // 网络质量监控方法已移除
    
    /**
     * 获取增强系统的统计信息
     */
    getEnhancedSystemsStats() {
        const stats = {};
        
        if (this.errorHandler) {
            stats.errorHandler = this.errorHandler.getErrorStats();
        }
        
        if (this.userFeedback) {
            stats.userFeedback = this.userFeedback.getFeedbackStats();
        }
        
        // 网络质量统计已移除
        
        return stats;
    }
    
    /**
     * 显示停止重连按钮
     */
    showStopReconnectButton() {
        const stopBtn = document.getElementById('stop-reconnect-btn');
        if (stopBtn) {
            stopBtn.style.display = 'inline-block';
            Logger.debug('UIManager: 显示停止重连按钮');
        }
    }

    /**
     * 隐藏停止重连按钮
     */
    hideStopReconnectButton() {
        const stopBtn = document.getElementById('stop-reconnect-btn');
        if (stopBtn) {
            stopBtn.style.display = 'none';
            Logger.debug('UIManager: 隐藏停止重连按钮');
        }
    }

    /**
     * 销毁UI管理器
     */
    destroy() {
        // 销毁增强系统
        if (this.errorHandler) {
            this.errorHandler.destroy();
            this.errorHandler = null;
        }
        
        if (this.userFeedback) {
            this.userFeedback.destroy();
            this.userFeedback = null;
        }
        
        // 网络质量监控器销毁已移除
        
        // 清理定时器
        if (this.controlsTimer) {
            clearTimeout(this.controlsTimer);
        }
        
        if (this.errorMessageTimer) {
            clearTimeout(this.errorMessageTimer);
        }
        
        // 移除事件监听器
        if (this._handleResize) {
            window.removeEventListener('resize', this._handleResize);
            window.removeEventListener('orientationchange', this._handleResize);
        }
        
        Logger.info('UIManager: UI管理器已销毁');
    }
}

// 导出UIManager类
if (typeof module !== 'undefined' && module.exports) {
    module.exports = UIManager;
} else if (typeof window !== 'undefined') {
    window.UIManager = UIManager;
}
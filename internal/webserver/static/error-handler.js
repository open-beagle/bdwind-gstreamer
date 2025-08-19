/**
 * 增强的错误处理和用户反馈系统
 * 实现用户友好的错误提示、连接状态显示和网络质量监控
 */

/**
 * 错误类型定义
 */
const ErrorTypes = {
    CONFIGURATION: 'configuration',
    SIGNALING: 'signaling',
    ICE_CONNECTION: 'ice_connection',
    MEDIA_STREAM: 'media_stream',
    NETWORK: 'network',
    BROWSER_COMPATIBILITY: 'browser_compatibility',
    PERMISSION: 'permission',
    UNKNOWN: 'unknown'
};

/**
 * 错误严重程度
 */
const ErrorSeverity = {
    LOW: 'low',
    MEDIUM: 'medium',
    HIGH: 'high',
    CRITICAL: 'critical'
};

/**
 * 连接状态类型
 */
const ConnectionStatusTypes = {
    DISCONNECTED: 'disconnected',
    CONNECTING: 'connecting',
    CONNECTED: 'connected',
    ERROR: 'error',
    RECONNECTING: 'reconnecting'
};

/**
 * 网络质量等级
 */
const NetworkQualityLevels = {
    EXCELLENT: 'excellent',
    GOOD: 'good',
    FAIR: 'fair',
    POOR: 'poor',
    UNKNOWN: 'unknown'
};

/**
 * 增强的错误处理器类
 */
class EnhancedErrorHandler {
    constructor(eventBus, config = {}) {
        this.eventBus = eventBus;
        this.config = config;
        
        // UI元素
        this.elements = {
            errorContainer: null,
            statusContainer: null,
            qualityContainer: null,
            notificationContainer: null
        };
        
        // 错误历史和统计
        this.errorHistory = [];
        this.errorStats = new Map();
        this.maxErrorHistory = 100;
        
        // 连接状态
        this.connectionStatus = {
            current: ConnectionStatusTypes.DISCONNECTED,
            lastUpdate: Date.now(),
            history: [],
            details: {}
        };
        
        // 网络质量监控
        this.networkQuality = {
            current: NetworkQualityLevels.UNKNOWN,
            metrics: {
                rtt: 0,
                packetLoss: 0,
                jitter: 0,
                bitrate: 0,
                frameRate: 0
            },
            history: [],
            lastUpdate: Date.now()
        };
        
        // 用户反馈配置
        this.feedbackConfig = {
            showDetailedErrors: config.showDetailedErrors !== false,
            autoHideDelay: config.autoHideDelay || 5000,
            maxNotifications: config.maxNotifications || 3,
            enableSuggestions: config.enableSuggestions !== false,
            enableNetworkQuality: config.enableNetworkQuality !== false
        };
        
        // 错误消息模板
        this.errorMessages = this._initializeErrorMessages();
        
        // 建议解决方案
        this.suggestions = this._initializeSuggestions();
        
        this._initialize();
    }
    
    /**
     * 初始化错误处理器
     * @private
     */
    _initialize() {
        this._createUIElements();
        this._setupEventListeners();
        this._startQualityMonitoring();
        
        Logger.info('EnhancedErrorHandler: 错误处理器初始化完成');
    }
    
    /**
     * 创建UI元素
     * @private
     */
    _createUIElements() {
        this._createErrorContainer();
        this._createStatusContainer();
        this._createQualityContainer();
        this._createNotificationContainer();
    }
    
    /**
     * 创建错误容器
     * @private
     */
    _createErrorContainer() {
        const container = document.createElement('div');
        container.id = 'enhanced-error-container';
        container.className = 'enhanced-error-container';
        container.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            width: 400px;
            max-width: 90vw;
            z-index: 10000;
            pointer-events: none;
        `;
        
        document.body.appendChild(container);
        this.elements.errorContainer = container;
    }
    
    /**
     * 创建状态容器
     * @private
     */
    _createStatusContainer() {
        const container = document.createElement('div');
        container.id = 'enhanced-status-container';
        container.className = 'enhanced-status-container';
        container.style.cssText = `
            position: fixed;
            bottom: 20px;
            left: 20px;
            background: rgba(0, 0, 0, 0.9);
            color: white;
            padding: 12px 16px;
            border-radius: 8px;
            font-size: 14px;
            z-index: 9999;
            display: flex;
            align-items: center;
            gap: 10px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255, 255, 255, 0.1);
        `;
        
        container.innerHTML = `
            <div class="status-indicator-enhanced"></div>
            <div class="status-text-enhanced">未连接</div>
            <div class="status-details-enhanced" style="display: none;"></div>
        `;
        
        document.body.appendChild(container);
        this.elements.statusContainer = container;
    }
    
    /**
     * 创建网络质量容器
     * @private
     */
    _createQualityContainer() {
        if (!this.feedbackConfig.enableNetworkQuality) return;
        
        const container = document.createElement('div');
        container.id = 'enhanced-quality-container';
        container.className = 'enhanced-quality-container';
        container.style.cssText = `
            position: fixed;
            bottom: 20px;
            right: 20px;
            background: rgba(0, 0, 0, 0.9);
            color: white;
            padding: 12px 16px;
            border-radius: 8px;
            font-size: 12px;
            z-index: 9999;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255, 255, 255, 0.1);
            min-width: 200px;
        `;
        
        container.innerHTML = `
            <div class="quality-header" style="font-weight: bold; margin-bottom: 8px;">
                <span class="quality-level">网络质量</span>
                <span class="quality-score" style="float: right;">--</span>
            </div>
            <div class="quality-metrics">
                <div class="quality-metric">
                    <span>延迟:</span>
                    <span class="metric-rtt">-- ms</span>
                </div>
                <div class="quality-metric">
                    <span>丢包:</span>
                    <span class="metric-loss">--%</span>
                </div>
                <div class="quality-metric">
                    <span>抖动:</span>
                    <span class="metric-jitter">-- ms</span>
                </div>
                <div class="quality-metric">
                    <span>帧率:</span>
                    <span class="metric-fps">-- fps</span>
                </div>
            </div>
        `;
        
        document.body.appendChild(container);
        this.elements.qualityContainer = container;
    }
    
    /**
     * 创建通知容器
     * @private
     */
    _createNotificationContainer() {
        const container = document.createElement('div');
        container.id = 'enhanced-notification-container';
        container.className = 'enhanced-notification-container';
        container.style.cssText = `
            position: fixed;
            top: 80px;
            right: 20px;
            width: 400px;
            max-width: 90vw;
            z-index: 10001;
            pointer-events: none;
        `;
        
        document.body.appendChild(container);
        this.elements.notificationContainer = container;
    }
    
    /**
     * 设置事件监听器
     * @private
     */
    _setupEventListeners() {
        if (!this.eventBus) return;
        
        // 监听连接状态变化
        this.eventBus.on('webrtc:state-changed', (data) => {
            this._handleConnectionStateChange(data);
        });
        
        this.eventBus.on('signaling:connected', () => {
            this.updateConnectionStatus(ConnectionStatusTypes.CONNECTING, {
                message: '信令已连接',
                phase: 'signaling'
            });
        });
        
        this.eventBus.on('signaling:disconnected', () => {
            this.updateConnectionStatus(ConnectionStatusTypes.DISCONNECTED, {
                message: '信令连接断开',
                phase: 'signaling'
            });
        });
        
        this.eventBus.on('webrtc:connected', () => {
            this.updateConnectionStatus(ConnectionStatusTypes.CONNECTED, {
                message: '媒体流已连接',
                phase: 'media'
            });
        });
        
        this.eventBus.on('webrtc:disconnected', () => {
            this.updateConnectionStatus(ConnectionStatusTypes.DISCONNECTED, {
                message: '媒体流已断开',
                phase: 'media'
            });
        });
        
        // 监听错误事件
        this.eventBus.on('webrtc:connection-failed', (data) => {
            this.handleError(ErrorTypes.ICE_CONNECTION, 'WebRTC连接失败', data.error, {
                severity: ErrorSeverity.HIGH,
                context: data
            });
        });
        
        this.eventBus.on('signaling:error', (data) => {
            this.handleError(ErrorTypes.SIGNALING, '信令服务器错误', data.error, {
                severity: ErrorSeverity.HIGH,
                context: data
            });
        });
        
        this.eventBus.on('webrtc:media-track-ended', (data) => {
            this.handleError(ErrorTypes.MEDIA_STREAM, '媒体流中断', null, {
                severity: ErrorSeverity.MEDIUM,
                context: data,
                suggestion: '正在尝试重新连接媒体流...'
            });
        });
        
        // 监听质量更新事件
        this.eventBus.on('webrtc:stats-updated', (stats) => {
            this._updateNetworkQuality(stats);
        });
        
        this.eventBus.on('webrtc:health-check-completed', (healthData) => {
            this._updateNetworkQualityFromHealth(healthData);
        });
        
        // 监听重连事件
        this.eventBus.on('webrtc:reconnecting', (data) => {
            this.updateConnectionStatus(ConnectionStatusTypes.RECONNECTING, {
                message: `重连中 (${data.attempt}/${data.maxAttempts})`,
                phase: 'reconnecting',
                attempt: data.attempt,
                maxAttempts: data.maxAttempts
            });
        });
    }
    
    /**
     * 处理连接状态变化
     * @private
     */
    _handleConnectionStateChange(data) {
        const stateMapping = {
            'disconnected': ConnectionStatusTypes.DISCONNECTED,
            'connecting': ConnectionStatusTypes.CONNECTING,
            'connected': ConnectionStatusTypes.CONNECTED,
            'failed': ConnectionStatusTypes.ERROR,
            'reconnecting': ConnectionStatusTypes.RECONNECTING
        };
        
        const status = stateMapping[data.to] || ConnectionStatusTypes.DISCONNECTED;
        const message = this._getStateMessage(data.to, data.context);
        
        this.updateConnectionStatus(status, {
            message,
            phase: data.to,
            context: data.context
        });
    }
    
    /**
     * 获取状态消息
     * @private
     */
    _getStateMessage(state, context = {}) {
        const messages = {
            'disconnected': '未连接',
            'initializing': '初始化中...',
            'connecting': '连接中...',
            'signaling': '信令协商中...',
            'ice_gathering': '收集网络信息...',
            'ice_connecting': '建立连接...',
            'connected': '已连接',
            'failed': '连接失败',
            'reconnecting': '重连中...',
            'closed': '连接已关闭'
        };
        
        return messages[state] || '未知状态';
    }
    
    /**
     * 初始化错误消息模板
     * @private
     */
    _initializeErrorMessages() {
        return {
            [ErrorTypes.CONFIGURATION]: {
                title: '配置错误',
                defaultMessage: '系统配置存在问题',
                icon: '⚙️'
            },
            [ErrorTypes.SIGNALING]: {
                title: '信令错误',
                defaultMessage: '信令服务器连接失败',
                icon: '📡'
            },
            [ErrorTypes.ICE_CONNECTION]: {
                title: '连接错误',
                defaultMessage: 'WebRTC连接建立失败',
                icon: '🔗'
            },
            [ErrorTypes.MEDIA_STREAM]: {
                title: '媒体错误',
                defaultMessage: '媒体流处理失败',
                icon: '🎥'
            },
            [ErrorTypes.NETWORK]: {
                title: '网络错误',
                defaultMessage: '网络连接存在问题',
                icon: '🌐'
            },
            [ErrorTypes.BROWSER_COMPATIBILITY]: {
                title: '浏览器兼容性',
                defaultMessage: '浏览器不支持某些功能',
                icon: '🌍'
            },
            [ErrorTypes.PERMISSION]: {
                title: '权限错误',
                defaultMessage: '缺少必要的权限',
                icon: '🔒'
            },
            [ErrorTypes.UNKNOWN]: {
                title: '未知错误',
                defaultMessage: '发生了未知错误',
                icon: '❓'
            }
        };
    }
    
    /**
     * 初始化建议解决方案
     * @private
     */
    _initializeSuggestions() {
        return {
            [ErrorTypes.CONFIGURATION]: [
                '检查服务器配置是否正确',
                '确认ICE服务器地址可访问',
                '验证认证信息是否有效'
            ],
            [ErrorTypes.SIGNALING]: [
                '检查网络连接',
                '确认信令服务器状态',
                '尝试刷新页面重新连接'
            ],
            [ErrorTypes.ICE_CONNECTION]: [
                '检查防火墙设置',
                '确认STUN/TURN服务器可用',
                '尝试使用不同的网络环境'
            ],
            [ErrorTypes.MEDIA_STREAM]: [
                '检查摄像头和麦克风权限',
                '确认媒体设备正常工作',
                '尝试重新启动浏览器'
            ],
            [ErrorTypes.NETWORK]: [
                '检查网络连接稳定性',
                '尝试切换到更稳定的网络',
                '检查网络带宽是否充足'
            ],
            [ErrorTypes.BROWSER_COMPATIBILITY]: [
                '更新浏览器到最新版本',
                '尝试使用Chrome或Firefox',
                '启用必要的浏览器功能'
            ],
            [ErrorTypes.PERMISSION]: [
                '允许网站访问摄像头和麦克风',
                '检查浏览器权限设置',
                '确认系统权限配置正确'
            ]
        };
    }
    
    /**
     * 处理错误
     * @param {string} type - 错误类型
     * @param {string} message - 错误消息
     * @param {Error} error - 错误对象
     * @param {Object} options - 选项
     */
    handleError(type, message, error = null, options = {}) {
        const errorInfo = {
            id: this._generateErrorId(),
            type,
            message,
            error,
            timestamp: Date.now(),
            severity: options.severity || ErrorSeverity.MEDIUM,
            context: options.context || {},
            suggestion: options.suggestion || null,
            resolved: false
        };
        
        // 记录错误
        this._recordError(errorInfo);
        
        // 显示错误通知
        this._showErrorNotification(errorInfo);
        
        // 更新连接状态（如果是严重错误）
        if (errorInfo.severity === ErrorSeverity.HIGH || errorInfo.severity === ErrorSeverity.CRITICAL) {
            this.updateConnectionStatus(ConnectionStatusTypes.ERROR, {
                message: message,
                error: errorInfo
            });
        }
        
        // 触发错误事件
        this.eventBus?.emit('error-handler:error-occurred', errorInfo);
        
        Logger.error(`EnhancedErrorHandler: ${type} - ${message}`, error, options.context);
    }
    
    /**
     * 显示错误通知
     * @private
     */
    _showErrorNotification(errorInfo) {
        const template = this.errorMessages[errorInfo.type] || this.errorMessages[ErrorTypes.UNKNOWN];
        
        const notification = document.createElement('div');
        notification.className = `error-notification severity-${errorInfo.severity}`;
        notification.style.cssText = `
            background: ${this._getSeverityColor(errorInfo.severity)};
            color: white;
            padding: 16px;
            border-radius: 8px;
            margin-bottom: 10px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
            pointer-events: auto;
            cursor: pointer;
            transition: all 0.3s ease;
            border-left: 4px solid ${this._getSeverityAccentColor(errorInfo.severity)};
        `;
        
        const suggestions = this.feedbackConfig.enableSuggestions ? 
            this._getSuggestions(errorInfo) : [];
        
        notification.innerHTML = `
            <div class="error-header" style="display: flex; align-items: center; margin-bottom: 8px;">
                <span class="error-icon" style="font-size: 18px; margin-right: 8px;">${template.icon}</span>
                <span class="error-title" style="font-weight: bold;">${template.title}</span>
                <button class="error-close" style="margin-left: auto; background: none; border: none; color: white; cursor: pointer; font-size: 18px;">×</button>
            </div>
            <div class="error-message" style="margin-bottom: ${suggestions.length > 0 ? '12px' : '0'};">
                ${errorInfo.message || template.defaultMessage}
            </div>
            ${suggestions.length > 0 ? `
                <div class="error-suggestions" style="font-size: 12px; opacity: 0.9;">
                    <div style="font-weight: bold; margin-bottom: 4px;">建议解决方案:</div>
                    <ul style="margin: 0; padding-left: 16px;">
                        ${suggestions.map(s => `<li>${s}</li>`).join('')}
                    </ul>
                </div>
            ` : ''}
            ${this.feedbackConfig.showDetailedErrors && errorInfo.error ? `
                <details style="margin-top: 8px; font-size: 11px; opacity: 0.8;">
                    <summary style="cursor: pointer;">技术详情</summary>
                    <pre style="margin-top: 4px; white-space: pre-wrap; font-family: monospace;">${errorInfo.error.message || errorInfo.error}</pre>
                </details>
            ` : ''}
        `;
        
        // 添加事件监听器
        const closeBtn = notification.querySelector('.error-close');
        closeBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this._removeNotification(notification);
        });
        
        notification.addEventListener('click', () => {
            this._showErrorDetails(errorInfo);
        });
        
        // 添加到容器
        this.elements.errorContainer.appendChild(notification);
        
        // 限制通知数量
        this._limitNotifications();
        
        // 自动隐藏
        if (this.feedbackConfig.autoHideDelay > 0) {
            setTimeout(() => {
                this._removeNotification(notification);
            }, this.feedbackConfig.autoHideDelay);
        }
    }
    
    /**
     * 获取严重程度颜色
     * @private
     */
    _getSeverityColor(severity) {
        const colors = {
            [ErrorSeverity.LOW]: '#2196F3',
            [ErrorSeverity.MEDIUM]: '#FF9800',
            [ErrorSeverity.HIGH]: '#F44336',
            [ErrorSeverity.CRITICAL]: '#9C27B0'
        };
        return colors[severity] || colors[ErrorSeverity.MEDIUM];
    }
    
    /**
     * 获取严重程度强调色
     * @private
     */
    _getSeverityAccentColor(severity) {
        const colors = {
            [ErrorSeverity.LOW]: '#1976D2',
            [ErrorSeverity.MEDIUM]: '#F57C00',
            [ErrorSeverity.HIGH]: '#D32F2F',
            [ErrorSeverity.CRITICAL]: '#7B1FA2'
        };
        return colors[severity] || colors[ErrorSeverity.MEDIUM];
    }
    
    /**
     * 获取建议解决方案
     * @private
     */
    _getSuggestions(errorInfo) {
        const suggestions = this.suggestions[errorInfo.type] || [];
        
        if (errorInfo.suggestion) {
            return [errorInfo.suggestion, ...suggestions.slice(0, 2)];
        }
        
        return suggestions.slice(0, 3);
    }
    
    /**
     * 限制通知数量
     * @private
     */
    _limitNotifications() {
        const notifications = this.elements.errorContainer.children;
        while (notifications.length > this.feedbackConfig.maxNotifications) {
            this._removeNotification(notifications[0]);
        }
    }
    
    /**
     * 移除通知
     * @private
     */
    _removeNotification(notification) {
        if (notification && notification.parentNode) {
            notification.style.transform = 'translateX(100%)';
            notification.style.opacity = '0';
            setTimeout(() => {
                if (notification.parentNode) {
                    notification.parentNode.removeChild(notification);
                }
            }, 300);
        }
    }
    
    /**
     * 显示错误详情
     * @private
     */
    _showErrorDetails(errorInfo) {
        // 这里可以实现一个详细的错误信息弹窗
        console.group(`错误详情 - ${errorInfo.type}`);
        console.log('错误ID:', errorInfo.id);
        console.log('时间:', new Date(errorInfo.timestamp).toLocaleString());
        console.log('消息:', errorInfo.message);
        console.log('严重程度:', errorInfo.severity);
        console.log('上下文:', errorInfo.context);
        if (errorInfo.error) {
            console.log('错误对象:', errorInfo.error);
        }
        console.groupEnd();
    }
    
    /**
     * 更新连接状态
     * @param {string} status - 状态类型
     * @param {Object} details - 状态详情
     */
    updateConnectionStatus(status, details = {}) {
        const previousStatus = this.connectionStatus.current;
        
        this.connectionStatus = {
            current: status,
            lastUpdate: Date.now(),
            history: [...this.connectionStatus.history, {
                status: previousStatus,
                timestamp: Date.now(),
                duration: Date.now() - this.connectionStatus.lastUpdate
            }].slice(-20), // 保留最近20条记录
            details
        };
        
        this._updateStatusDisplay();
        
        // 触发状态变化事件
        this.eventBus?.emit('error-handler:status-updated', {
            previous: previousStatus,
            current: status,
            details
        });
    }
    
    /**
     * 更新状态显示
     * @private
     */
    _updateStatusDisplay() {
        if (!this.elements.statusContainer) return;
        
        const indicator = this.elements.statusContainer.querySelector('.status-indicator-enhanced');
        const text = this.elements.statusContainer.querySelector('.status-text-enhanced');
        const details = this.elements.statusContainer.querySelector('.status-details-enhanced');
        
        if (indicator) {
            indicator.className = `status-indicator-enhanced status-${this.connectionStatus.current}`;
            indicator.style.cssText = `
                width: 12px;
                height: 12px;
                border-radius: 50%;
                background: ${this._getStatusColor(this.connectionStatus.current)};
                animation: ${this.connectionStatus.current === ConnectionStatusTypes.CONNECTING || 
                           this.connectionStatus.current === ConnectionStatusTypes.RECONNECTING ? 
                           'pulse 1.5s infinite' : 'none'};
            `;
        }
        
        if (text) {
            text.textContent = this.connectionStatus.details.message || 
                              this._getStatusText(this.connectionStatus.current);
        }
        
        if (details && this.connectionStatus.details.phase) {
            details.textContent = `阶段: ${this.connectionStatus.details.phase}`;
            details.style.display = 'block';
        } else if (details) {
            details.style.display = 'none';
        }
    }
    
    /**
     * 获取状态颜色
     * @private
     */
    _getStatusColor(status) {
        const colors = {
            [ConnectionStatusTypes.DISCONNECTED]: '#f44336',
            [ConnectionStatusTypes.CONNECTING]: '#ff9800',
            [ConnectionStatusTypes.CONNECTED]: '#4caf50',
            [ConnectionStatusTypes.ERROR]: '#f44336',
            [ConnectionStatusTypes.RECONNECTING]: '#2196f3'
        };
        return colors[status] || colors[ConnectionStatusTypes.DISCONNECTED];
    }
    
    /**
     * 获取状态文本
     * @private
     */
    _getStatusText(status) {
        const texts = {
            [ConnectionStatusTypes.DISCONNECTED]: '未连接',
            [ConnectionStatusTypes.CONNECTING]: '连接中...',
            [ConnectionStatusTypes.CONNECTED]: '已连接',
            [ConnectionStatusTypes.ERROR]: '连接错误',
            [ConnectionStatusTypes.RECONNECTING]: '重连中...'
        };
        return texts[status] || '未知状态';
    }
    
    /**
     * 开始质量监控
     * @private
     */
    _startQualityMonitoring() {
        if (!this.feedbackConfig.enableNetworkQuality) return;
        
        // 每2秒更新一次网络质量显示
        setInterval(() => {
            this._updateQualityDisplay();
        }, 2000);
    }
    
    /**
     * 更新网络质量
     * @private
     */
    _updateNetworkQuality(stats) {
        if (!stats) return;
        
        const metrics = {
            rtt: stats.rtt || 0,
            packetLoss: this._calculatePacketLoss(stats),
            jitter: stats.jitter || 0,
            bitrate: stats.bitrate || 0,
            frameRate: stats.frameRate || 0
        };
        
        this.networkQuality.metrics = metrics;
        this.networkQuality.lastUpdate = Date.now();
        this.networkQuality.current = this._calculateQualityLevel(metrics);
        
        // 记录历史
        this.networkQuality.history.push({
            timestamp: Date.now(),
            metrics: { ...metrics },
            level: this.networkQuality.current
        });
        
        // 保留最近100条记录
        if (this.networkQuality.history.length > 100) {
            this.networkQuality.history = this.networkQuality.history.slice(-50);
        }
    }
    
    /**
     * 从健康检查数据更新网络质量
     * @private
     */
    _updateNetworkQualityFromHealth(healthData) {
        if (!healthData || !healthData.stats || !healthData.stats.summary) return;
        
        const summary = healthData.stats.summary;
        this._updateNetworkQuality({
            rtt: summary.rtt,
            packetLoss: summary.packetsLost,
            packetsReceived: summary.packetsReceived,
            jitter: summary.jitter,
            bitrate: summary.bytesReceived,
            frameRate: summary.frameRate
        });
    }
    
    /**
     * 计算丢包率
     * @private
     */
    _calculatePacketLoss(stats) {
        if (!stats.packetsLost || !stats.packetsReceived) return 0;
        
        const totalPackets = stats.packetsLost + stats.packetsReceived;
        return totalPackets > 0 ? (stats.packetsLost / totalPackets) * 100 : 0;
    }
    
    /**
     * 计算网络质量等级
     * @private
     */
    _calculateQualityLevel(metrics) {
        let score = 100;
        
        // RTT评分 (0-40分)
        if (metrics.rtt > 200) score -= 40;
        else if (metrics.rtt > 100) score -= 20;
        else if (metrics.rtt > 50) score -= 10;
        
        // 丢包率评分 (0-30分)
        if (metrics.packetLoss > 5) score -= 30;
        else if (metrics.packetLoss > 2) score -= 15;
        else if (metrics.packetLoss > 1) score -= 5;
        
        // 抖动评分 (0-20分)
        const jitterMs = metrics.jitter * 1000;
        if (jitterMs > 50) score -= 20;
        else if (jitterMs > 20) score -= 10;
        else if (jitterMs > 10) score -= 5;
        
        // 帧率评分 (0-10分)
        if (metrics.frameRate < 15) score -= 10;
        else if (metrics.frameRate < 25) score -= 5;
        
        // 根据分数确定等级
        if (score >= 90) return NetworkQualityLevels.EXCELLENT;
        if (score >= 75) return NetworkQualityLevels.GOOD;
        if (score >= 60) return NetworkQualityLevels.FAIR;
        if (score >= 0) return NetworkQualityLevels.POOR;
        
        return NetworkQualityLevels.UNKNOWN;
    }
    
    /**
     * 更新质量显示
     * @private
     */
    _updateQualityDisplay() {
        if (!this.elements.qualityContainer) return;
        
        const levelElement = this.elements.qualityContainer.querySelector('.quality-level');
        const scoreElement = this.elements.qualityContainer.querySelector('.quality-score');
        const rttElement = this.elements.qualityContainer.querySelector('.metric-rtt');
        const lossElement = this.elements.qualityContainer.querySelector('.metric-loss');
        const jitterElement = this.elements.qualityContainer.querySelector('.metric-jitter');
        const fpsElement = this.elements.qualityContainer.querySelector('.metric-fps');
        
        if (levelElement) {
            levelElement.textContent = this._getQualityLevelText(this.networkQuality.current);
            levelElement.style.color = this._getQualityLevelColor(this.networkQuality.current);
        }
        
        if (scoreElement) {
            const score = this._calculateQualityScore();
            scoreElement.textContent = score >= 0 ? `${score}%` : '--';
        }
        
        const metrics = this.networkQuality.metrics;
        
        if (rttElement) {
            rttElement.textContent = metrics.rtt > 0 ? `${Math.round(metrics.rtt)} ms` : '-- ms';
        }
        
        if (lossElement) {
            lossElement.textContent = metrics.packetLoss > 0 ? `${metrics.packetLoss.toFixed(1)}%` : '--%';
        }
        
        if (jitterElement) {
            const jitterMs = metrics.jitter * 1000;
            jitterElement.textContent = jitterMs > 0 ? `${jitterMs.toFixed(1)} ms` : '-- ms';
        }
        
        if (fpsElement) {
            fpsElement.textContent = metrics.frameRate > 0 ? `${Math.round(metrics.frameRate)} fps` : '-- fps';
        }
    }
    
    /**
     * 获取质量等级文本
     * @private
     */
    _getQualityLevelText(level) {
        const texts = {
            [NetworkQualityLevels.EXCELLENT]: '优秀',
            [NetworkQualityLevels.GOOD]: '良好',
            [NetworkQualityLevels.FAIR]: '一般',
            [NetworkQualityLevels.POOR]: '较差',
            [NetworkQualityLevels.UNKNOWN]: '未知'
        };
        return texts[level] || '未知';
    }
    
    /**
     * 获取质量等级颜色
     * @private
     */
    _getQualityLevelColor(level) {
        const colors = {
            [NetworkQualityLevels.EXCELLENT]: '#4caf50',
            [NetworkQualityLevels.GOOD]: '#8bc34a',
            [NetworkQualityLevels.FAIR]: '#ff9800',
            [NetworkQualityLevels.POOR]: '#f44336',
            [NetworkQualityLevels.UNKNOWN]: '#9e9e9e'
        };
        return colors[level] || colors[NetworkQualityLevels.UNKNOWN];
    }
    
    /**
     * 计算质量分数
     * @private
     */
    _calculateQualityScore() {
        const metrics = this.networkQuality.metrics;
        
        if (metrics.rtt === 0 && metrics.packetLoss === 0 && metrics.jitter === 0) {
            return -1; // 无数据
        }
        
        let score = 100;
        
        // RTT评分
        if (metrics.rtt > 200) score -= 40;
        else if (metrics.rtt > 100) score -= 20;
        else if (metrics.rtt > 50) score -= 10;
        
        // 丢包率评分
        if (metrics.packetLoss > 5) score -= 30;
        else if (metrics.packetLoss > 2) score -= 15;
        else if (metrics.packetLoss > 1) score -= 5;
        
        // 抖动评分
        const jitterMs = metrics.jitter * 1000;
        if (jitterMs > 50) score -= 20;
        else if (jitterMs > 20) score -= 10;
        else if (jitterMs > 10) score -= 5;
        
        // 帧率评分
        if (metrics.frameRate < 15) score -= 10;
        else if (metrics.frameRate < 25) score -= 5;
        
        return Math.max(0, Math.round(score));
    }
    
    /**
     * 记录错误
     * @private
     */
    _recordError(errorInfo) {
        this.errorHistory.push(errorInfo);
        
        // 更新错误统计
        const count = this.errorStats.get(errorInfo.type) || 0;
        this.errorStats.set(errorInfo.type, count + 1);
        
        // 保持历史记录在合理范围内
        if (this.errorHistory.length > this.maxErrorHistory) {
            this.errorHistory = this.errorHistory.slice(-Math.floor(this.maxErrorHistory / 2));
        }
    }
    
    /**
     * 生成错误ID
     * @private
     */
    _generateErrorId() {
        return `error_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
    }
    
    /**
     * 获取错误统计
     */
    getErrorStats() {
        return {
            total: this.errorHistory.length,
            byType: Object.fromEntries(this.errorStats),
            recent: this.errorHistory.slice(-10),
            resolved: this.errorHistory.filter(e => e.resolved).length
        };
    }
    
    /**
     * 获取连接状态
     */
    getConnectionStatus() {
        return { ...this.connectionStatus };
    }
    
    /**
     * 获取网络质量
     */
    getNetworkQuality() {
        return { ...this.networkQuality };
    }
    
    /**
     * 清除所有通知
     */
    clearAllNotifications() {
        if (this.elements.errorContainer) {
            while (this.elements.errorContainer.firstChild) {
                this.elements.errorContainer.removeChild(this.elements.errorContainer.firstChild);
            }
        }
    }
    
    /**
     * 销毁错误处理器
     */
    destroy() {
        this.clearAllNotifications();
        
        // 移除UI元素
        Object.values(this.elements).forEach(element => {
            if (element && element.parentNode) {
                element.parentNode.removeChild(element);
            }
        });
        
        // 清理数据
        this.errorHistory = [];
        this.errorStats.clear();
        this.networkQuality.history = [];
        
        Logger.info('EnhancedErrorHandler: 错误处理器已销毁');
    }
}

// 添加CSS动画
const errorHandlerStyle = document.createElement('style');
errorHandlerStyle.textContent = `
    @keyframes pulse {
        0% { opacity: 1; }
        50% { opacity: 0.5; }
        100% { opacity: 1; }
    }
    
    .error-notification {
        animation: slideInRight 0.3s ease-out;
    }
    
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
    
    .quality-metric {
        display: flex;
        justify-content: space-between;
        margin-bottom: 4px;
    }
`;
document.head.appendChild(errorHandlerStyle);

// 导出类和常量
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        EnhancedErrorHandler,
        ErrorTypes,
        ErrorSeverity,
        ConnectionStatusTypes,
        NetworkQualityLevels
    };
}

// 设置到全局作用域
if (typeof window !== 'undefined') {
    window.EnhancedErrorHandler = EnhancedErrorHandler;
    window.ErrorTypes = ErrorTypes;
    window.ErrorSeverity = ErrorSeverity;
    window.ConnectionStatusTypes = ConnectionStatusTypes;
    window.NetworkQualityLevels = NetworkQualityLevels;
}
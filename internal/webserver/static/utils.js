/**
 * Utils - 通用工具模块
 * 提供日志、DOM操作、网络检测、设备检测等工具函数
 */

/**
 * Logger - 日志工具
 */
const Logger = {
    _logLevel: 'info',
    _logLevels: {
        debug: 0,
        info: 1,
        warn: 2,
        error: 3
    },
    _maxLogEntries: 1000,
    _logEntries: [],
    _eventBus: null,
    _debugMode: false,
    _sessionId: null,
    _contextStack: [],
    _errorCategories: new Map(),

    /**
     * 初始化Logger
     */
    init() {
        this._sessionId = this._generateSessionId();
        this._debugMode = this._detectDebugMode();
        
        // 如果是调试模式，自动设置为debug级别
        if (this._debugMode) {
            this._logLevel = 'debug';
        }
        
        this.info('Logger: 初始化完成', {
            sessionId: this._sessionId,
            debugMode: this._debugMode,
            logLevel: this._logLevel
        });
    },

    /**
     * 设置日志级别
     */
    setLevel(level) {
        if (level in this._logLevels) {
            this._logLevel = level;
            this.info(`Logger: 日志级别设置为 ${level}`);
        }
    },

    /**
     * 设置事件总线
     */
    setEventBus(eventBus) {
        this._eventBus = eventBus;
    },

    /**
     * 设置最大日志条目数
     */
    setMaxEntries(max) {
        this._maxLogEntries = max;
        this._trimLogs();
    },

    /**
     * 启用/禁用调试模式
     */
    setDebugMode(enabled) {
        this._debugMode = enabled;
        if (enabled) {
            this._logLevel = 'debug';
            this.info('Logger: 调试模式已启用');
        } else {
            this._logLevel = 'info';
            this.info('Logger: 调试模式已禁用');
        }
    },

    /**
     * 推入上下文
     */
    pushContext(context) {
        this._contextStack.push(context);
        if (this._debugMode) {
            this.debug(`Logger: 推入上下文 - ${context}`);
        }
    },

    /**
     * 弹出上下文
     */
    popContext() {
        const context = this._contextStack.pop();
        if (this._debugMode && context) {
            this.debug(`Logger: 弹出上下文 - ${context}`);
        }
    },

    /**
     * 结构化错误日志
     */
    errorStructured(errorType, message, details = {}) {
        const structuredError = {
            type: errorType,
            message,
            details,
            timestamp: new Date().toISOString(),
            sessionId: this._sessionId,
            url: window.location.href,
            userAgent: navigator.userAgent
        };
        this.error(`[${errorType}] ${message}`, structuredError);
        
        // 触发错误事件，供其他组件监听
        if (this._eventBus) {
            this._eventBus.emit('logger:structured-error', structuredError);
        }
        return structuredError;
    },

    /**
     * 连接流程日志
     */
    connectionFlow(step, details = {}) {
        const flowEntry = {
            step,
            details,
            timestamp: new Date().toISOString(),
            sessionId: this._sessionId
        };
        this.info(`Connection Flow: ${step}`, flowEntry);
        
        // 保存连接流程到专门的存储
        this._saveToStorage('connectionFlow', flowEntry);
    },

    /**
     * WebRTC状态日志
     */
    webrtcState(component, state, data = {}) {
        const stateEntry = {
            component,
            state,
            data,
            timestamp: new Date().toISOString(),
            sessionId: this._sessionId
        };
        this.debug(`WebRTC State [${component}]: ${state}`, stateEntry);
        
        // 在调试模式下保存WebRTC状态历史
        if (this._debugMode) {
            this._saveToStorage('webrtcStates', stateEntry);
        }
    },

    /**
     * 性能计时开始
     */
    time(label) {
        if (!this._timers) {
            this._timers = new Map();
        }
        this._timers.set(label, {
            start: performance.now(),
            timestamp: new Date().toISOString()
        });
        this.debug(`Timer started: ${label}`);
    },

    /**
     * 性能计时结束
     */
    timeEnd(label) {
        if (!this._timers) {
            this._timers = new Map();
        }
        const timer = this._timers.get(label);
        if (timer) {
            const duration = performance.now() - timer.start;
            this._timers.delete(label);
            this.info(`Timer ${label}: ${duration.toFixed(2)}ms`);
            return duration;
        } else {
            this.warn(`Timer not found: ${label}`);
            return null;
        }
    },

    /**
     * 获取日志历史
     */
    getHistory(level = null, limit = 100) {
        let history = this._logEntries;
        if (level) {
            const levelValue = this._logLevels[level.toLowerCase()];
            history = history.filter(entry => this._logLevels[entry.level] >= levelValue);
        }
        return history.slice(-limit);
    },

    /**
     * 获取错误日志
     */
    getErrors() {
        try {
            return JSON.parse(localStorage.getItem('logger_errors') || '[]');
        } catch (error) {
            return [];
        }
    },

    /**
     * 获取连接流程日志
     */
    getConnectionFlow() {
        try {
            return JSON.parse(localStorage.getItem('logger_connectionFlow') || '[]');
        } catch (error) {
            return [];
        }
    },

    /**
     * 清理日志
     */
    clearLogs(category = 'all') {
        if (category === 'all') {
            this._logEntries = [];
            localStorage.removeItem('logger_errors');
            localStorage.removeItem('logger_warnings');
            localStorage.removeItem('logger_connectionFlow');
            localStorage.removeItem('logger_webrtcStates');
        } else {
            localStorage.removeItem(`logger_${category}`);
        }
        this.info(`Logger: 已清理${category}日志`);
    },

    /**
     * 导出日志
     */
    exportLogs() {
        const exportData = {
            timestamp: new Date().toISOString(),
            sessionId: this._sessionId,
            history: this.getHistory(),
            errors: this.getErrors(),
            connectionFlow: this.getConnectionFlow(),
            userAgent: navigator.userAgent,
            url: window.location.href
        };
        const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `webrtc-logs-${new Date().toISOString().slice(0, 19).replace(/:/g, '-')}.json`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
        this.info('Logger: 日志已导出');
    },

    /**
     * 保存到本地存储
     * @private
     */
    _saveToStorage(category, entry) {
        try {
            const key = `logger_${category}`;
            let entries = JSON.parse(localStorage.getItem(key) || '[]');
            entries.push(entry);
            // 限制存储大小
            const maxEntries = category === 'errors' ? 100 : 50;
            if (entries.length > maxEntries) {
                entries = entries.slice(-maxEntries);
            }
            localStorage.setItem(key, JSON.stringify(entries));
        } catch (error) {
            console.error('Logger: 保存到存储失败', error);
        }
    },

    /**
     * 获取会话ID
     * @private
     */
    _getSessionId() {
        if (!this._sessionId) {
            this._sessionId = 'session_' + Date.now() + '_' + Math.random().toString(36).substr(2, 9);
        }
        return this._sessionId;
    },

    /**
     * Debug级别日志
     */
    debug(message, data = null, context = null) {
        this._log('debug', message, data, context);
    },

    /**
     * Info级别日志
     */
    info(message, data = null, context = null) {
        this._log('info', message, data, context);
    },

    /**
     * Warn级别日志
     */
    warn(message, data = null, context = null) {
        this._log('warn', message, data, context);
    },

    /**
     * Error级别日志
     */
    error(message, error = null, context = null) {
        // 如果第二个参数是Error对象，进行结构化处理
        if (error instanceof Error) {
            const structuredError = this._structureError(error, context);
            this._log('error', message, structuredError, context);
            
            // 记录错误分类
            this._categorizeError(error, message, context);
        } else {
            this._log('error', message, error, context);
        }
    },

    /**
     * 记录连接事件（专用于WebRTC连接调试）
     */
    logConnection(event, data = null, context = null) {
        const connectionContext = {
            type: 'connection',
            event: event,
            timestamp: Date.now(),
            ...context
        };
        
        this.info(`Connection: ${event}`, data, connectionContext);
    },

    /**
     * 记录性能指标
     */
    logPerformance(metric, value, unit = 'ms', context = null) {
        const perfContext = {
            type: 'performance',
            metric: metric,
            value: value,
            unit: unit,
            ...context
        };
        
        this.info(`Performance: ${metric} = ${value}${unit}`, null, perfContext);
    },

    /**
     * 记录用户操作
     */
    logUserAction(action, data = null, context = null) {
        const actionContext = {
            type: 'user-action',
            action: action,
            ...context
        };
        
        this.info(`UserAction: ${action}`, data, actionContext);
    },

    /**
     * 获取日志条目
     */
    getEntries(level = null, category = null, limit = null) {
        let entries = [...this._logEntries];
        
        if (level) {
            entries = entries.filter(entry => entry.level === level);
        }
        
        if (category) {
            entries = entries.filter(entry => 
                entry.context && entry.context.type === category
            );
        }
        
        if (limit) {
            entries = entries.slice(-limit);
        }
        
        return entries;
    },

    /**
     * 获取错误统计
     */
    getErrorStats() {
        const stats = {
            totalErrors: 0,
            categories: {},
            recentErrors: []
        };
        
        this._errorCategories.forEach((errors, category) => {
            stats.categories[category] = errors.length;
            stats.totalErrors += errors.length;
        });
        
        // 获取最近的错误
        const recentErrorEntries = this._logEntries
            .filter(entry => entry.level === 'error')
            .slice(-10);
        
        stats.recentErrors = recentErrorEntries.map(entry => ({
            timestamp: entry.timestamp,
            message: entry.message,
            category: entry.context?.errorCategory || 'unknown'
        }));
        
        return stats;
    },

    /**
     * 清空日志
     */
    clear() {
        this._logEntries = [];
        this._errorCategories.clear();
        if (this._eventBus) {
            this._eventBus.emit('logger:cleared');
        }
        this.info('Logger: 日志已清空');
    },

    /**
     * 导出日志
     */
    export(format = 'json', options = {}) {
        const exportData = {
            sessionId: this._sessionId,
            exportTime: new Date().toISOString(),
            debugMode: this._debugMode,
            logLevel: this._logLevel,
            totalEntries: this._logEntries.length,
            errorStats: this.getErrorStats(),
            entries: this._logEntries
        };
        
        if (format === 'json') {
            return JSON.stringify(exportData, null, 2);
        } else if (format === 'text') {
            const lines = [
                `Session ID: ${exportData.sessionId}`,
                `Export Time: ${exportData.exportTime}`,
                `Debug Mode: ${exportData.debugMode}`,
                `Log Level: ${exportData.logLevel}`,
                `Total Entries: ${exportData.totalEntries}`,
                '',
                'Error Statistics:',
                ...Object.entries(exportData.errorStats.categories).map(
                    ([category, count]) => `  ${category}: ${count}`
                ),
                '',
                'Log Entries:',
                ...this._logEntries.map(entry => this._formatLogEntry(entry))
            ];
            return lines.join('\n');
        } else if (format === 'csv') {
            const headers = ['Timestamp', 'Level', 'Message', 'Context', 'Data'];
            const rows = this._logEntries.map(entry => [
                entry.timestamp,
                entry.level,
                entry.message,
                entry.context ? JSON.stringify(entry.context) : '',
                entry.data ? JSON.stringify(entry.data) : ''
            ]);
            
            return [headers, ...rows].map(row => 
                row.map(cell => `"${String(cell).replace(/"/g, '""')}"`).join(',')
            ).join('\n');
        }
        
        throw new Error(`Unsupported export format: ${format}`);
    },

    /**
     * 内部日志方法
     * @private
     */
    _log(level, message, data, context) {
        if (this._logLevels[level] < this._logLevels[this._logLevel]) {
            return;
        }

        const timestamp = new Date().toISOString();
        const logEntry = {
            id: this._generateLogId(),
            timestamp,
            level,
            message,
            data: data || undefined,
            context: this._buildContext(context),
            sessionId: this._sessionId
        };

        // 添加到日志条目
        this._logEntries.push(logEntry);
        this._trimLogs();

        // 控制台输出（增强格式）
        this._consoleOutput(logEntry);

        // 通过事件总线通知
        if (this._eventBus) {
            this._eventBus.emit('logger:entry', logEntry);
        }
    },

    /**
     * 构建上下文信息
     * @private
     */
    _buildContext(context) {
        const fullContext = {
            stack: [...this._contextStack],
            userAgent: navigator.userAgent,
            url: window.location.href,
            timestamp: Date.now()
        };
        
        if (context) {
            Object.assign(fullContext, context);
        }
        
        return fullContext;
    },

    /**
     * 结构化错误信息
     * @private
     */
    _structureError(error, context) {
        return {
            name: error.name,
            message: error.message,
            stack: error.stack,
            cause: error.cause,
            fileName: error.fileName,
            lineNumber: error.lineNumber,
            columnNumber: error.columnNumber,
            context: context,
            timestamp: Date.now()
        };
    },

    /**
     * 错误分类
     * @private
     */
    _categorizeError(error, message, context) {
        let category = 'unknown';
        
        // 根据错误类型和消息内容进行分类
        if (error.name === 'TypeError') {
            category = 'type-error';
        } else if (error.name === 'ReferenceError') {
            category = 'reference-error';
        } else if (error.name === 'NetworkError' || message.includes('network') || message.includes('fetch')) {
            category = 'network-error';
        } else if (message.includes('WebRTC') || message.includes('PeerConnection')) {
            category = 'webrtc-error';
        } else if (message.includes('Signaling') || message.includes('WebSocket')) {
            category = 'signaling-error';
        } else if (message.includes('Media') || message.includes('Video') || message.includes('Audio')) {
            category = 'media-error';
        } else if (context && context.type) {
            category = `${context.type}-error`;
        }
        
        if (!this._errorCategories.has(category)) {
            this._errorCategories.set(category, []);
        }
        
        this._errorCategories.get(category).push({
            timestamp: Date.now(),
            error: error,
            message: message,
            context: context
        });
        
        // 限制每个分类的错误数量
        const categoryErrors = this._errorCategories.get(category);
        if (categoryErrors.length > 50) {
            this._errorCategories.set(category, categoryErrors.slice(-50));
        }
    },

    /**
     * 控制台输出
     * @private
     */
    _consoleOutput(logEntry) {
        const consoleMethod = console[logEntry.level] || console.log;
        const prefix = `[${logEntry.timestamp}] ${logEntry.level.toUpperCase()}`;
        
        if (this._debugMode) {
            // 调试模式下显示更详细的信息
            const contextInfo = logEntry.context ? 
                ` [${logEntry.context.stack.join(' > ')}]` : '';
            
            consoleMethod(
                `${prefix}${contextInfo}: ${logEntry.message}`,
                logEntry.data || '',
                logEntry.context || ''
            );
        } else {
            // 普通模式下的简洁输出
            if (logEntry.data) {
                consoleMethod(`${prefix}: ${logEntry.message}`, logEntry.data);
            } else {
                consoleMethod(`${prefix}: ${logEntry.message}`);
            }
        }
    },

    /**
     * 格式化日志条目
     * @private
     */
    _formatLogEntry(entry) {
        const contextStr = entry.context && entry.context.stack.length > 0 ? 
            ` [${entry.context.stack.join(' > ')}]` : '';
        
        const dataStr = entry.data ? ` | Data: ${JSON.stringify(entry.data)}` : '';
        
        return `[${entry.timestamp}] ${entry.level.toUpperCase()}${contextStr}: ${entry.message}${dataStr}`;
    },

    /**
     * 生成会话ID
     * @private
     */
    _generateSessionId() {
        return `session_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
    },

    /**
     * 生成日志ID
     * @private
     */
    _generateLogId() {
        return `log_${Date.now()}_${Math.random().toString(36).substr(2, 6)}`;
    },

    /**
     * 检测调试模式
     * @private
     */
    _detectDebugMode() {
        // 检查URL参数
        const urlParams = new URLSearchParams(window.location.search);
        if (urlParams.get('debug') === 'true') {
            return true;
        }
        
        // 检查localStorage
        if (localStorage.getItem('debug') === 'true') {
            return true;
        }
        
        // 检查开发者工具是否打开
        if (window.console && window.console.firebug) {
            return true;
        }
        
        return false;
    },

    /**
     * 修剪日志条目
     * @private
     */
    _trimLogs() {
        if (this._logEntries.length > this._maxLogEntries) {
            this._logEntries = this._logEntries.slice(-this._maxLogEntries);
        }
    }
};

/**
 * DOMUtils - DOM操作工具
 */
const DOMUtils = {
    /**
     * 创建元素
     */
    createElement(tag, attributes = {}, children = []) {
        const element = document.createElement(tag);

        // 设置属性
        for (const [key, value] of Object.entries(attributes)) {
            if (key === 'className') {
                element.className = value;
            } else if (key === 'innerHTML') {
                element.innerHTML = value;
            } else if (key === 'textContent') {
                element.textContent = value;
            } else if (key.startsWith('data-')) {
                element.setAttribute(key, value);
            } else {
                element[key] = value;
            }
        }

        // 添加子元素
        children.forEach(child => {
            if (typeof child === 'string') {
                element.appendChild(document.createTextNode(child));
            } else if (child instanceof Node) {
                element.appendChild(child);
            }
        });

        return element;
    },

    /**
     * 添加CSS类
     */
    addClass(element, className) {
        if (element && className) {
            element.classList.add(className);
        }
    },

    /**
     * 移除CSS类
     */
    removeClass(element, className) {
        if (element && className) {
            element.classList.remove(className);
        }
    },

    /**
     * 切换CSS类
     */
    toggleClass(element, className) {
        if (element && className) {
            element.classList.toggle(className);
        }
    },

    /**
     * 检查是否有CSS类
     */
    hasClass(element, className) {
        return element && className && element.classList.contains(className);
    },

    /**
     * 查找元素
     */
    find(selector, parent = document) {
        return parent.querySelector(selector);
    },

    /**
     * 查找所有元素
     */
    findAll(selector, parent = document) {
        return Array.from(parent.querySelectorAll(selector));
    },

    /**
     * 显示元素
     */
    show(element) {
        if (element) {
            element.style.display = '';
        }
    },

    /**
     * 隐藏元素
     */
    hide(element) {
        if (element) {
            element.style.display = 'none';
        }
    },

    /**
     * 切换元素显示状态
     */
    toggle(element) {
        if (element) {
            if (element.style.display === 'none') {
                this.show(element);
            } else {
                this.hide(element);
            }
        }
    },

    /**
     * 设置元素内容
     */
    setContent(element, content) {
        if (element) {
            if (typeof content === 'string') {
                element.textContent = content;
            } else if (content instanceof Node) {
                element.innerHTML = '';
                element.appendChild(content);
            }
        }
    },

    /**
     * 添加事件监听器
     */
    on(element, event, handler, options = false) {
        if (element && event && handler) {
            element.addEventListener(event, handler, options);
            return () => element.removeEventListener(event, handler, options);
        }
    },

    /**
     * 移除事件监听器
     */
    off(element, event, handler, options = false) {
        if (element && event && handler) {
            element.removeEventListener(event, handler, options);
        }
    },

    /**
     * 获取元素位置
     */
    getPosition(element) {
        if (!element) return { x: 0, y: 0 };
        const rect = element.getBoundingClientRect();
        return {
            x: rect.left + window.scrollX,
            y: rect.top + window.scrollY,
            width: rect.width,
            height: rect.height
        };
    },

    /**
     * 检查元素是否在视口中
     */
    isInViewport(element) {
        if (!element) return false;
        const rect = element.getBoundingClientRect();
        return (
            rect.top >= 0 &&
            rect.left >= 0 &&
            rect.bottom <= (window.innerHeight || document.documentElement.clientHeight) &&
            rect.right <= (window.innerWidth || document.documentElement.clientWidth)
        );
    }
};

/**
 * NetworkUtils - 网络工具
 */
const NetworkUtils = {
    /**
     * 检查是否在线
     */
    isOnline() {
        return navigator.onLine;
    },

    /**
     * 获取连接类型
     */
    getConnectionType() {
        if ('connection' in navigator) {
            return navigator.connection.effectiveType || 'unknown';
        }
        return 'unknown';
    },

    /**
     * 获取网络信息
     */
    getNetworkInfo() {
        const connection = navigator.connection || navigator.mozConnection || navigator.webkitConnection;
        
        if (connection) {
            return {
                effectiveType: connection.effectiveType,
                downlink: connection.downlink,
                rtt: connection.rtt,
                saveData: connection.saveData
            };
        }
        
        return null;
    },

    /**
     * 测量延迟
     */
    async measureLatency(url = '/ping') {
        const start = performance.now();
        
        try {
            await fetch(url, { 
                method: 'HEAD',
                cache: 'no-cache'
            });
            return performance.now() - start;
        } catch (error) {
            Logger.warn('NetworkUtils: Failed to measure latency:', error);
            return -1;
        }
    },

    /**
     * 测试网络速度
     */
    async measureBandwidth(testUrl = '/test-file', testSize = 1024 * 1024) {
        const start = performance.now();
        
        try {
            const response = await fetch(testUrl);
            const data = await response.blob();
            const duration = (performance.now() - start) / 1000; // 转换为秒
            const sizeInBits = data.size * 8;
            
            return {
                downloadSpeed: sizeInBits / duration, // bits per second
                duration,
                size: data.size
            };
        } catch (error) {
            Logger.warn('NetworkUtils: Failed to measure bandwidth:', error);
            return null;
        }
    },

    /**
     * 监听网络状态变化
     */
    onNetworkChange(callback) {
        const handleOnline = () => callback({ online: true });
        const handleOffline = () => callback({ online: false });
        
        window.addEventListener('online', handleOnline);
        window.addEventListener('offline', handleOffline);
        
        // 如果支持连接API，也监听连接变化
        if ('connection' in navigator) {
            const handleConnectionChange = () => {
                callback({
                    online: navigator.onLine,
                    connectionType: this.getConnectionType(),
                    networkInfo: this.getNetworkInfo()
                });
            };
            
            navigator.connection.addEventListener('change', handleConnectionChange);
            
            return () => {
                window.removeEventListener('online', handleOnline);
                window.removeEventListener('offline', handleOffline);
                navigator.connection.removeEventListener('change', handleConnectionChange);
            };
        }
        
        return () => {
            window.removeEventListener('online', handleOnline);
            window.removeEventListener('offline', handleOffline);
        };
    }
};

/**
 * DeviceUtils - 设备检测工具
 */
const DeviceUtils = {
    /**
     * 检查是否为移动设备
     */
    isMobile() {
        return /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
    },

    /**
     * 检查是否为平板设备
     */
    isTablet() {
        return /iPad|Android(?!.*Mobile)/i.test(navigator.userAgent);
    },

    /**
     * 检查是否为桌面设备
     */
    isDesktop() {
        return !this.isMobile() && !this.isTablet();
    },

    /**
     * 检查是否支持触摸
     */
    hasTouch() {
        return 'ontouchstart' in window || navigator.maxTouchPoints > 0;
    },

    /**
     * 获取屏幕尺寸
     */
    getScreenSize() {
        return {
            width: window.screen.width,
            height: window.screen.height,
            availWidth: window.screen.availWidth,
            availHeight: window.screen.availHeight,
            innerWidth: window.innerWidth,
            innerHeight: window.innerHeight
        };
    },

    /**
     * 获取设备像素比
     */
    getPixelRatio() {
        return window.devicePixelRatio || 1;
    },

    /**
     * 检查是否为iOS设备
     */
    isIOS() {
        return /iPad|iPhone|iPod/.test(navigator.userAgent);
    },

    /**
     * 检查是否为Android设备
     */
    isAndroid() {
        return /Android/i.test(navigator.userAgent);
    },

    /**
     * 获取操作系统信息
     */
    getOS() {
        const userAgent = navigator.userAgent;
        
        if (/Windows NT/i.test(userAgent)) return 'Windows';
        if (/Mac OS X/i.test(userAgent)) return 'macOS';
        if (/Linux/i.test(userAgent)) return 'Linux';
        if (/Android/i.test(userAgent)) return 'Android';
        if (/iPad|iPhone|iPod/i.test(userAgent)) return 'iOS';
        
        return 'Unknown';
    },

    /**
     * 获取浏览器信息
     */
    getBrowser() {
        const userAgent = navigator.userAgent;
        
        if (/Chrome/i.test(userAgent) && !/Edge/i.test(userAgent)) return 'Chrome';
        if (/Firefox/i.test(userAgent)) return 'Firefox';
        if (/Safari/i.test(userAgent) && !/Chrome/i.test(userAgent)) return 'Safari';
        if (/Edge/i.test(userAgent)) return 'Edge';
        if (/Opera/i.test(userAgent)) return 'Opera';
        
        return 'Unknown';
    },

    /**
     * 检查WebRTC支持
     */
    supportsWebRTC() {
        return !!(window.RTCPeerConnection || window.webkitRTCPeerConnection || window.mozRTCPeerConnection);
    },

    /**
     * 检查WebSocket支持
     */
    supportsWebSocket() {
        return 'WebSocket' in window;
    },

    /**
     * 检查本地存储支持
     */
    supportsLocalStorage() {
        try {
            const test = 'test';
            localStorage.setItem(test, test);
            localStorage.removeItem(test);
            return true;
        } catch (e) {
            return false;
        }
    },

    /**
     * 获取设备能力信息
     */
    getCapabilities() {
        return {
            webrtc: this.supportsWebRTC(),
            websocket: this.supportsWebSocket(),
            localStorage: this.supportsLocalStorage(),
            touch: this.hasTouch(),
            mobile: this.isMobile(),
            tablet: this.isTablet(),
            desktop: this.isDesktop(),
            os: this.getOS(),
            browser: this.getBrowser(),
            pixelRatio: this.getPixelRatio(),
            screen: this.getScreenSize()
        };
    },

    /**
     * 监听屏幕方向变化
     */
    onOrientationChange(callback) {
        const handleOrientationChange = () => {
            callback({
                orientation: window.orientation || 0,
                screen: this.getScreenSize()
            });
        };
        
        window.addEventListener('orientationchange', handleOrientationChange);
        window.addEventListener('resize', handleOrientationChange);
        
        return () => {
            window.removeEventListener('orientationchange', handleOrientationChange);
            window.removeEventListener('resize', handleOrientationChange);
        };
    }
};

// 导出工具对象
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { Logger, DOMUtils, NetworkUtils, DeviceUtils };
} else if (typeof window !== 'undefined') {
    window.Logger = Logger;
    window.DOMUtils = DOMUtils;
    window.NetworkUtils = NetworkUtils;
    window.DeviceUtils = DeviceUtils;
}
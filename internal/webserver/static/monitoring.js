/**
 * MonitoringManager - 监控和统计管理器
 * 收集系统性能指标、连接质量数据和错误日志
 */
class MonitoringManager {
    constructor(eventBus, config) {
        this.eventBus = eventBus;
        this.config = config;
        
        // 监控状态
        this.isEnabled = false;
        this.isCollecting = false;
        
        // 监控任务
        this.tasks = new Map();
        this.taskScheduler = null;
        this.defaultInterval = 5000;
        
        // 性能指标
        this.performanceMetrics = {
            cpu: 0,
            memory: 0,
            fps: 0,
            renderTime: 0,
            networkLatency: 0,
            timestamp: null
        };
        
        // 连接质量数据
        this.connectionQuality = {
            webrtc: {
                connectionState: 'closed',
                iceConnectionState: 'closed',
                bytesReceived: 0,
                bytesSent: 0,
                packetsLost: 0,
                jitter: 0,
                rtt: 0,
                bandwidth: 0
            },
            signaling: {
                connectionState: 'closed',
                latency: 0,
                reconnectCount: 0,
                messagesSent: 0,
                messagesReceived: 0,
                errors: 0
            }
        };
        
        // 错误统计
        this.errorStats = {
            total: 0,
            byType: {},
            byModule: {},
            recent: [],
            maxRecentErrors: 100
        };
        
        // 历史数据
        this.historyData = {
            performance: [],
            connection: [],
            errors: [],
            maxHistorySize: 1000
        };
        
        // 配置
        this.statsInterval = 5000;
        this.performanceMonitoring = true;
        this.errorReporting = true;
        this.exportFormat = 'json';
        
        this._loadConfig();
        this._setupEventListeners();
    }

    /**
     * 加载配置
     * @private
     */
    _loadConfig() {
        if (this.config) {
            const monitoringConfig = this.config.get('monitoring', {});
            this.isEnabled = monitoringConfig.enabled !== false;
            this.statsInterval = monitoringConfig.statsInterval || 5000;
            this.performanceMonitoring = monitoringConfig.performanceMonitoring !== false;
            this.errorReporting = monitoringConfig.errorReporting !== false;
            this.exportFormat = monitoringConfig.exportFormat || 'json';
        }
    }

    /**
     * 设置事件监听器
     * @private
     */
    _setupEventListeners() {
        if (this.eventBus) {
            // 监听WebRTC统计事件
            this.eventBus.on('webrtc:stats-updated', (stats) => {
                this._updateWebRTCStats(stats);
            });
            
            this.eventBus.on('webrtc:connection-state-change', (data) => {
                this.connectionQuality.webrtc.connectionState = data.state;
                this._recordConnectionEvent('webrtc', 'state-change', data);
            });
            
            // 监听信令统计事件
            this.eventBus.on('signaling:connected', () => {
                this.connectionQuality.signaling.connectionState = 'connected';
                this._recordConnectionEvent('signaling', 'connected');
            });
            
            this.eventBus.on('signaling:disconnected', () => {
                this.connectionQuality.signaling.connectionState = 'disconnected';
                this._recordConnectionEvent('signaling', 'disconnected');
            });
            
            this.eventBus.on('signaling:reconnecting', (data) => {
                this.connectionQuality.signaling.reconnectCount++;
                this._recordConnectionEvent('signaling', 'reconnecting', data);
            });
            
            // 监听错误事件
            this.eventBus.on('logger:entry', (entry) => {
                if (entry.level === 'error') {
                    this._recordError(entry);
                }
            });
            
            // 监听性能事件
            this.eventBus.on('performance:fps-updated', (data) => {
                this.performanceMetrics.fps = data.fps;
            });
            
            this.eventBus.on('performance:render-time-updated', (data) => {
                this.performanceMetrics.renderTime = data.renderTime;
            });
        }
    }

    /**
     * 启动监控
     */
    start() {
        if (!this.isEnabled) {
            Logger.warn('MonitoringManager: 监控已禁用');
            return;
        }
        
        if (this.isCollecting) {
            Logger.warn('MonitoringManager: 监控已在运行');
            return;
        }
        
        Logger.info('MonitoringManager: 启动监控');
        
        this.isCollecting = true;
        
        // 注册默认监控任务
        this._registerDefaultTasks();
        
        // 启动任务调度器
        this._startTaskScheduler();
        
        // 启动性能监控
        if (this.performanceMonitoring) {
            this._startPerformanceMonitoring();
        }
        
        this.eventBus?.emit('monitoring:started');
    }

    /**
     * 停止监控
     */
    stop() {
        if (!this.isCollecting) {
            Logger.warn('MonitoringManager: 监控未在运行');
            return;
        }
        
        Logger.info('MonitoringManager: 停止监控');
        
        this.isCollecting = false;
        
        // 停止任务调度器
        this._stopTaskScheduler();
        
        // 停止性能监控
        this._stopPerformanceMonitoring();
        
        this.eventBus?.emit('monitoring:stopped');
    }

    /**
     * 注册监控任务
     */
    registerTask(name, callback, interval = null) {
        if (typeof callback !== 'function') {
            throw new Error('MonitoringManager: Task callback must be a function');
        }
        
        const task = {
            name,
            callback,
            interval: interval || this.defaultInterval,
            lastRun: 0,
            runCount: 0,
            errors: 0
        };
        
        this.tasks.set(name, task);
        Logger.debug(`MonitoringManager: 注册监控任务 - ${name}`);
        
        return () => this.unregisterTask(name);
    }

    /**
     * 取消注册监控任务
     */
    unregisterTask(name) {
        if (this.tasks.has(name)) {
            this.tasks.delete(name);
            Logger.debug(`MonitoringManager: 取消注册监控任务 - ${name}`);
        }
    }

    /**
     * 获取性能指标
     */
    getPerformanceMetrics() {
        return {
            ...this.performanceMetrics,
            timestamp: Date.now()
        };
    }

    /**
     * 获取连接质量数据
     */
    getConnectionQuality() {
        return {
            ...this.connectionQuality,
            timestamp: Date.now()
        };
    }

    /**
     * 获取错误统计
     */
    getErrorStats() {
        return {
            ...this.errorStats,
            timestamp: Date.now()
        };
    }

    /**
     * 获取历史数据
     */
    getHistoryData(type = null, limit = null) {
        if (type && this.historyData[type]) {
            const data = this.historyData[type];
            return limit ? data.slice(-limit) : data;
        }
        
        const result = {};
        Object.keys(this.historyData).forEach(key => {
            const data = this.historyData[key];
            result[key] = limit ? data.slice(-limit) : data;
        });
        
        return result;
    }

    /**
     * 生成监控报告
     */
    generateReport(options = {}) {
        const {
            includePerformance = true,
            includeConnection = true,
            includeErrors = true,
            includeHistory = false,
            format = this.exportFormat
        } = options;
        
        const report = {
            timestamp: Date.now(),
            generatedAt: new Date().toISOString(),
            summary: this._generateSummary()
        };
        
        if (includePerformance) {
            report.performance = this.getPerformanceMetrics();
        }
        
        if (includeConnection) {
            report.connection = this.getConnectionQuality();
        }
        
        if (includeErrors) {
            report.errors = this.getErrorStats();
        }
        
        if (includeHistory) {
            report.history = this.getHistoryData();
        }
        
        return this._formatReport(report, format);
    }

    /**
     * 导出数据
     */
    exportData(format = this.exportFormat, filename = null) {
        const report = this.generateReport({ includeHistory: true, format });
        
        if (!filename) {
            filename = `monitoring-report-${new Date().toISOString().slice(0, 19).replace(/:/g, '-')}.${format}`;
        }
        
        this._downloadData(report, filename, format);
        
        Logger.info(`MonitoringManager: 数据已导出 - ${filename}`);
        this.eventBus?.emit('monitoring:data-exported', { filename, format });
    }

    /**
     * 清理历史数据
     */
    clearHistory(type = null) {
        if (type && this.historyData[type]) {
            this.historyData[type] = [];
            Logger.info(`MonitoringManager: 已清理${type}历史数据`);
        } else {
            Object.keys(this.historyData).forEach(key => {
                this.historyData[key] = [];
            });
            Logger.info('MonitoringManager: 已清理所有历史数据');
        }
        
        this.eventBus?.emit('monitoring:history-cleared', { type });
    }

    /**
     * 重置统计数据
     */
    resetStats() {
        // 重置性能指标
        this.performanceMetrics = {
            cpu: 0,
            memory: 0,
            fps: 0,
            renderTime: 0,
            networkLatency: 0,
            timestamp: null
        };
        
        // 重置连接质量数据
        this.connectionQuality.webrtc = {
            connectionState: 'closed',
            iceConnectionState: 'closed',
            bytesReceived: 0,
            bytesSent: 0,
            packetsLost: 0,
            jitter: 0,
            rtt: 0,
            bandwidth: 0
        };
        
        this.connectionQuality.signaling = {
            connectionState: 'closed',
            latency: 0,
            reconnectCount: 0,
            messagesSent: 0,
            messagesReceived: 0,
            errors: 0
        };
        
        // 重置错误统计
        this.errorStats = {
            total: 0,
            byType: {},
            byModule: {},
            recent: [],
            maxRecentErrors: 100
        };
        
        Logger.info('MonitoringManager: 统计数据已重置');
        this.eventBus?.emit('monitoring:stats-reset');
    }

    /**
     * 注册默认监控任务
     * @private
     */
    _registerDefaultTasks() {
        // 性能监控任务
        this.registerTask('performance', () => {
            this._collectPerformanceMetrics();
        }, this.statsInterval);
        
        // 连接质量监控任务
        this.registerTask('connection-quality', () => {
            this._assessConnectionQuality();
        }, this.statsInterval);
        
        // 内存使用监控任务
        this.registerTask('memory-usage', () => {
            this._monitorMemoryUsage();
        }, this.statsInterval * 2);
        
        // 网络延迟监控任务
        this.registerTask('network-latency', () => {
            this._measureNetworkLatency();
        }, this.statsInterval * 3);
    }

    /**
     * 启动任务调度器
     * @private
     */
    _startTaskScheduler() {
        this.taskScheduler = setInterval(() => {
            this._runScheduledTasks();
        }, 1000); // 每秒检查一次
    }

    /**
     * 停止任务调度器
     * @private
     */
    _stopTaskScheduler() {
        if (this.taskScheduler) {
            clearInterval(this.taskScheduler);
            this.taskScheduler = null;
        }
    }

    /**
     * 运行计划任务
     * @private
     */
    _runScheduledTasks() {
        const now = Date.now();
        
        this.tasks.forEach((task, name) => {
            if (now - task.lastRun >= task.interval) {
                try {
                    task.callback();
                    task.lastRun = now;
                    task.runCount++;
                } catch (error) {
                    task.errors++;
                    Logger.error(`MonitoringManager: 任务执行失败 - ${name}`, error);
                    this._recordError({
                        message: `Task execution failed: ${name}`,
                        error,
                        module: 'MonitoringManager'
                    });
                }
            }
        });
    }

    /**
     * 启动性能监控
     * @private
     */
    _startPerformanceMonitoring() {
        // 使用Performance Observer监控性能
        if ('PerformanceObserver' in window) {
            try {
                const observer = new PerformanceObserver((list) => {
                    this._processPerformanceEntries(list.getEntries());
                });
                
                observer.observe({ entryTypes: ['measure', 'navigation', 'paint'] });
                this.performanceObserver = observer;
                
            } catch (error) {
                Logger.warn('MonitoringManager: Performance Observer不可用', error);
            }
        }
        
        // 启动FPS监控
        this._startFPSMonitoring();
    }

    /**
     * 停止性能监控
     * @private
     */
    _stopPerformanceMonitoring() {
        if (this.performanceObserver) {
            this.performanceObserver.disconnect();
            this.performanceObserver = null;
        }
        
        this._stopFPSMonitoring();
    }

    /**
     * 启动FPS监控
     * @private
     */
    _startFPSMonitoring() {
        let lastTime = performance.now();
        let frameCount = 0;
        
        const measureFPS = (currentTime) => {
            frameCount++;
            
            if (currentTime - lastTime >= 1000) {
                this.performanceMetrics.fps = Math.round((frameCount * 1000) / (currentTime - lastTime));
                this.eventBus?.emit('performance:fps-updated', { fps: this.performanceMetrics.fps });
                
                frameCount = 0;
                lastTime = currentTime;
            }
            
            if (this.isCollecting) {
                this.fpsAnimationFrame = requestAnimationFrame(measureFPS);
            }
        };
        
        this.fpsAnimationFrame = requestAnimationFrame(measureFPS);
    }

    /**
     * 停止FPS监控
     * @private
     */
    _stopFPSMonitoring() {
        if (this.fpsAnimationFrame) {
            cancelAnimationFrame(this.fpsAnimationFrame);
            this.fpsAnimationFrame = null;
        }
    }

    /**
     * 收集性能指标
     * @private
     */
    _collectPerformanceMetrics() {
        // 收集内存使用情况
        if ('memory' in performance) {
            this.performanceMetrics.memory = performance.memory.usedJSHeapSize / 1024 / 1024; // MB
        }
        
        // 更新时间戳
        this.performanceMetrics.timestamp = Date.now();
        
        // 记录到历史数据
        this._addToHistory('performance', { ...this.performanceMetrics });
        
        this.eventBus?.emit('monitoring:performance-updated', this.performanceMetrics);
    }

    /**
     * 评估连接质量
     * @private
     */
    _assessConnectionQuality() {
        const quality = {
            webrtc: this._assessWebRTCQuality(),
            signaling: this._assessSignalingQuality(),
            overall: 'unknown'
        };
        
        // 计算整体质量
        const scores = [quality.webrtc.score, quality.signaling.score].filter(s => s !== null);
        if (scores.length > 0) {
            const avgScore = scores.reduce((a, b) => a + b, 0) / scores.length;
            quality.overall = this._getQualityLevel(avgScore);
        }
        
        // 记录到历史数据
        this._addToHistory('connection', quality);
        
        this.eventBus?.emit('monitoring:connection-quality-updated', quality);
    }

    /**
     * 评估WebRTC连接质量
     * @private
     */
    _assessWebRTCQuality() {
        const webrtc = this.connectionQuality.webrtc;
        let score = 0;
        let issues = [];
        
        // 连接状态评分
        if (webrtc.connectionState === 'connected') {
            score += 40;
        } else if (webrtc.connectionState === 'connecting') {
            score += 20;
        } else {
            issues.push('WebRTC连接未建立');
        }
        
        // RTT评分
        if (webrtc.rtt < 50) {
            score += 30;
        } else if (webrtc.rtt < 100) {
            score += 20;
        } else if (webrtc.rtt < 200) {
            score += 10;
        } else {
            issues.push('网络延迟过高');
        }
        
        // 丢包率评分
        const packetLossRate = webrtc.packetsLost / (webrtc.packetsLost + 1000); // 假设基数
        if (packetLossRate < 0.01) {
            score += 20;
        } else if (packetLossRate < 0.05) {
            score += 10;
        } else {
            issues.push('丢包率过高');
        }
        
        // 抖动评分
        if (webrtc.jitter < 10) {
            score += 10;
        } else if (webrtc.jitter < 30) {
            score += 5;
        } else {
            issues.push('网络抖动过大');
        }
        
        return {
            score: Math.min(score, 100),
            level: this._getQualityLevel(score),
            issues
        };
    }

    /**
     * 评估信令连接质量
     * @private
     */
    _assessSignalingQuality() {
        const signaling = this.connectionQuality.signaling;
        let score = 0;
        let issues = [];
        
        // 连接状态评分
        if (signaling.connectionState === 'connected') {
            score += 50;
        } else {
            issues.push('信令连接未建立');
        }
        
        // 延迟评分
        if (signaling.latency < 100) {
            score += 30;
        } else if (signaling.latency < 300) {
            score += 20;
        } else if (signaling.latency < 500) {
            score += 10;
        } else {
            issues.push('信令延迟过高');
        }
        
        // 重连次数评分
        if (signaling.reconnectCount === 0) {
            score += 20;
        } else if (signaling.reconnectCount < 3) {
            score += 10;
        } else {
            issues.push('频繁重连');
        }
        
        return {
            score: Math.min(score, 100),
            level: this._getQualityLevel(score),
            issues
        };
    }

    /**
     * 监控内存使用
     * @private
     */
    _monitorMemoryUsage() {
        if ('memory' in performance) {
            const memory = performance.memory;
            const usage = {
                used: memory.usedJSHeapSize / 1024 / 1024,
                total: memory.totalJSHeapSize / 1024 / 1024,
                limit: memory.jsHeapSizeLimit / 1024 / 1024,
                timestamp: Date.now()
            };
            
            // 检查内存使用率
            const usageRate = usage.used / usage.limit;
            if (usageRate > 0.8) {
                Logger.warn(`MonitoringManager: 内存使用率过高 ${(usageRate * 100).toFixed(1)}%`);
                this.eventBus?.emit('monitoring:high-memory-usage', usage);
            }
            
            this.eventBus?.emit('monitoring:memory-usage-updated', usage);
        }
    }

    /**
     * 测量网络延迟
     * @private
     */
    async _measureNetworkLatency() {
        try {
            const startTime = performance.now();
            await fetch('/ping', { method: 'HEAD', cache: 'no-cache' });
            const latency = performance.now() - startTime;
            
            this.performanceMetrics.networkLatency = latency;
            this.eventBus?.emit('monitoring:network-latency-updated', { latency });
            
        } catch (error) {
            Logger.warn('MonitoringManager: 网络延迟测量失败', error);
        }
    }

    /**
     * 更新WebRTC统计
     * @private
     */
    _updateWebRTCStats(stats) {
        Object.assign(this.connectionQuality.webrtc, stats);
    }

    /**
     * 记录连接事件
     * @private
     */
    _recordConnectionEvent(type, event, data = null) {
        const record = {
            type,
            event,
            data,
            timestamp: Date.now()
        };
        
        this._addToHistory('connection', record);
        Logger.debug(`MonitoringManager: 连接事件 - ${type}:${event}`, data);
    }

    /**
     * 记录错误
     * @private
     */
    _recordError(errorData) {
        if (!this.errorReporting) return;
        
        this.errorStats.total++;
        
        // 按类型统计
        const errorType = errorData.error?.name || 'Unknown';
        this.errorStats.byType[errorType] = (this.errorStats.byType[errorType] || 0) + 1;
        
        // 按模块统计
        const module = errorData.module || 'Unknown';
        this.errorStats.byModule[module] = (this.errorStats.byModule[module] || 0) + 1;
        
        // 添加到最近错误列表
        const errorRecord = {
            message: errorData.message,
            type: errorType,
            module,
            timestamp: Date.now(),
            stack: errorData.error?.stack
        };
        
        this.errorStats.recent.unshift(errorRecord);
        if (this.errorStats.recent.length > this.errorStats.maxRecentErrors) {
            this.errorStats.recent.pop();
        }
        
        // 记录到历史数据
        this._addToHistory('errors', errorRecord);
        
        this.eventBus?.emit('monitoring:error-recorded', errorRecord);
    }

    /**
     * 处理性能条目
     * @private
     */
    _processPerformanceEntries(entries) {
        entries.forEach(entry => {
            if (entry.entryType === 'measure') {
                this.performanceMetrics.renderTime = entry.duration;
                this.eventBus?.emit('performance:render-time-updated', { renderTime: entry.duration });
            }
        });
    }

    /**
     * 添加到历史数据
     * @private
     */
    _addToHistory(type, data) {
        if (!this.historyData[type]) {
            this.historyData[type] = [];
        }
        
        this.historyData[type].push({
            ...data,
            timestamp: data.timestamp || Date.now()
        });
        
        // 限制历史数据大小
        if (this.historyData[type].length > this.maxHistorySize) {
            this.historyData[type].shift();
        }
    }

    /**
     * 获取质量等级
     * @private
     */
    _getQualityLevel(score) {
        if (score >= 80) return 'excellent';
        if (score >= 60) return 'good';
        if (score >= 40) return 'fair';
        if (score >= 20) return 'poor';
        return 'bad';
    }

    /**
     * 生成摘要
     * @private
     */
    _generateSummary() {
        return {
            monitoringDuration: this.isCollecting ? Date.now() - this.startTime : 0,
            tasksRegistered: this.tasks.size,
            totalErrors: this.errorStats.total,
            connectionQuality: this._assessConnectionQuality().overall,
            performanceScore: this._calculatePerformanceScore()
        };
    }

    /**
     * 计算性能评分
     * @private
     */
    _calculatePerformanceScore() {
        let score = 100;
        
        // FPS评分
        if (this.performanceMetrics.fps < 30) score -= 20;
        else if (this.performanceMetrics.fps < 50) score -= 10;
        
        // 内存使用评分
        if (this.performanceMetrics.memory > 100) score -= 15;
        else if (this.performanceMetrics.memory > 50) score -= 5;
        
        // 网络延迟评分
        if (this.performanceMetrics.networkLatency > 200) score -= 15;
        else if (this.performanceMetrics.networkLatency > 100) score -= 5;
        
        return Math.max(score, 0);
    }

    /**
     * 格式化报告
     * @private
     */
    _formatReport(report, format) {
        switch (format) {
            case 'json':
                return JSON.stringify(report, null, 2);
            case 'csv':
                return this._convertToCSV(report);
            default:
                return report;
        }
    }

    /**
     * 转换为CSV格式
     * @private
     */
    _convertToCSV(data) {
        // 简化的CSV转换，实际实现可能需要更复杂的逻辑
        const rows = [];
        
        // 添加标题行
        rows.push('Timestamp,Type,Value');
        
        // 添加性能数据
        if (data.performance) {
            Object.entries(data.performance).forEach(([key, value]) => {
                if (key !== 'timestamp') {
                    rows.push(`${data.performance.timestamp},performance.${key},${value}`);
                }
            });
        }
        
        return rows.join('\n');
    }

    /**
     * 下载数据
     * @private
     */
    _downloadData(data, filename, format) {
        const blob = new Blob([data], {
            type: format === 'json' ? 'application/json' : 'text/csv'
        });
        
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = filename;
        link.click();
        
        URL.revokeObjectURL(url);
    }
}

// 导出MonitoringManager类
if (typeof module !== 'undefined' && module.exports) {
    module.exports = MonitoringManager;
} else if (typeof window !== 'undefined') {
    window.MonitoringManager = MonitoringManager;
}
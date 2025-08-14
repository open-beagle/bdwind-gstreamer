/**
 * ConfigManager - 配置管理模块
 * 管理系统配置和用户设置
 */
class ConfigManager {
    constructor(eventBus = null) {
        this.eventBus = eventBus;
        this._config = {};
        this._defaultConfig = this._getDefaultConfig();
        this._changeCallbacks = [];
        this._storageKey = 'bdwind-gstreamer-config';
    }

    /**
     * 获取默认配置
     * @private
     */
    _getDefaultConfig() {
        return {
            webrtc: {
                iceServers: [
                    { urls: 'stun:stun.l.google.com:19302' },
                    { urls: 'stun:stun1.l.google.com:19302' }
                ],
                connectionTimeout: 10000,
                maxReconnectAttempts: 5,
                reconnectDelay: 2000,
                iceTransportPolicy: 'all',
                bundlePolicy: 'balanced',
                rtcpMuxPolicy: 'require'
            },
            signaling: {
                url: null, // 将在运行时设置
                reconnectDelay: 2000,
                maxReconnectAttempts: 10,
                heartbeatInterval: 30000,
                connectionTimeout: 5000,
                messageTimeout: 10000
            },
            input: {
                enableKeyboard: true,
                enableMouse: true,
                enableTouch: true,
                enableGamepad: false,
                mouseSensitivity: 1.0,
                keyboardLayout: 'qwerty',
                shortcuts: {
                    fullscreen: 'F11',
                    toggleControls: 'Ctrl+Shift+C',
                    toggleStats: 'Ctrl+Shift+S',
                    screenshot: 'Ctrl+Shift+P'
                },
                mobile: {
                    enableVirtualKeyboard: true,
                    enableTouchGestures: true,
                    touchSensitivity: 1.0,
                    preventZoom: true
                }
            },
            ui: {
                showControls: true,
                showStats: false,
                showLogs: true,
                logLevel: 'info', // debug, info, warn, error
                maxLogEntries: 1000,
                theme: 'auto', // light, dark, auto
                language: 'zh-CN',
                autoHideControls: true,
                autoHideDelay: 3000,
                showVideoInfo: true,
                showConnectionStatus: true
            },
            video: {
                autoplay: true,
                muted: false,
                volume: 1.0,
                preferredCodec: 'auto', // h264, vp8, vp9, av1, auto
                maxBitrate: 0, // 0 = unlimited
                maxFramerate: 0, // 0 = unlimited
                preferredResolution: 'auto' // 1080p, 720p, 480p, auto
            },
            audio: {
                enabled: true,
                volume: 1.0,
                preferredCodec: 'opus', // opus, g722, pcmu, pcma
                echoCancellation: true,
                noiseSuppression: true,
                autoGainControl: true
            },
            monitoring: {
                enabled: true,
                statsInterval: 5000,
                performanceMonitoring: true,
                errorReporting: true,
                exportFormat: 'json' // json, csv
            },
            debug: {
                enabled: false,
                verboseLogging: false,
                showWebRTCStats: false,
                showNetworkStats: false,
                enablePerformanceMarks: false
            }
        };
    }

    /**
     * 加载配置
     */
    async loadConfig() {
        try {
            // 从本地存储加载用户配置
            const savedConfig = this._loadFromStorage();
            
            // 合并默认配置和用户配置
            this._config = this._mergeConfigs(this._defaultConfig, savedConfig);
            
            // 设置动态配置
            this._setDynamicConfig();
            
            // 触发配置加载事件
            this._notifyChange('config:loaded', this._config);
            
            return this._config;
        } catch (error) {
            console.error('ConfigManager: Failed to load config:', error);
            this._config = { ...this._defaultConfig };
            this._setDynamicConfig();
            return this._config;
        }
    }

    /**
     * 获取配置值
     * @param {string} key - 配置键，支持点号分隔的路径
     * @param {*} defaultValue - 默认值
     */
    get(key, defaultValue = undefined) {
        if (!key) {
            return this._config;
        }

        const keys = key.split('.');
        let value = this._config;

        for (const k of keys) {
            if (value && typeof value === 'object' && k in value) {
                value = value[k];
            } else {
                return defaultValue;
            }
        }

        return value;
    }

    /**
     * 设置配置值
     * @param {string} key - 配置键，支持点号分隔的路径
     * @param {*} value - 配置值
     */
    set(key, value) {
        if (!key) {
            throw new Error('ConfigManager.set: key is required');
        }

        const keys = key.split('.');
        const lastKey = keys.pop();
        let target = this._config;

        // 创建嵌套对象路径
        for (const k of keys) {
            if (!target[k] || typeof target[k] !== 'object') {
                target[k] = {};
            }
            target = target[k];
        }

        const oldValue = target[lastKey];
        target[lastKey] = value;

        // 触发变化事件
        this._notifyChange('config:changed', {
            key,
            value,
            oldValue,
            config: this._config
        });
    }

    /**
     * 批量设置配置
     * @param {Object} config - 配置对象
     */
    setMultiple(config) {
        if (!config || typeof config !== 'object') {
            throw new Error('ConfigManager.setMultiple: config must be an object');
        }

        const changes = [];
        
        for (const [key, value] of Object.entries(config)) {
            const oldValue = this.get(key);
            this.set(key, value);
            changes.push({ key, value, oldValue });
        }

        // 触发批量变化事件
        this._notifyChange('config:batch-changed', {
            changes,
            config: this._config
        });
    }

    /**
     * 保存配置到本地存储
     */
    save() {
        try {
            const configToSave = this._getSerializableConfig();
            localStorage.setItem(this._storageKey, JSON.stringify(configToSave));
            this._notifyChange('config:saved', configToSave);
            return true;
        } catch (error) {
            console.error('ConfigManager: Failed to save config:', error);
            return false;
        }
    }

    /**
     * 重置为默认配置
     */
    reset() {
        const oldConfig = { ...this._config };
        this._config = { ...this._defaultConfig };
        this._setDynamicConfig();
        
        this._notifyChange('config:reset', {
            config: this._config,
            oldConfig
        });
    }

    /**
     * 监听配置变化
     * @param {Function} callback - 回调函数
     * @returns {Function} 取消监听函数
     */
    onChange(callback) {
        if (typeof callback !== 'function') {
            throw new Error('ConfigManager.onChange: callback must be a function');
        }

        this._changeCallbacks.push(callback);

        // 返回取消监听函数
        return () => {
            const index = this._changeCallbacks.indexOf(callback);
            if (index !== -1) {
                this._changeCallbacks.splice(index, 1);
            }
        };
    }

    /**
     * 获取配置模式
     */
    getSchema() {
        return {
            webrtc: {
                type: 'object',
                properties: {
                    iceServers: { type: 'array' },
                    connectionTimeout: { type: 'number', min: 1000, max: 60000 },
                    maxReconnectAttempts: { type: 'number', min: 0, max: 20 }
                }
            },
            signaling: {
                type: 'object',
                properties: {
                    reconnectDelay: { type: 'number', min: 1000, max: 30000 },
                    maxReconnectAttempts: { type: 'number', min: 0, max: 50 }
                }
            },
            ui: {
                type: 'object',
                properties: {
                    logLevel: { type: 'string', enum: ['debug', 'info', 'warn', 'error'] },
                    theme: { type: 'string', enum: ['light', 'dark', 'auto'] }
                }
            }
        };
    }

    /**
     * 验证配置
     * @param {Object} config - 要验证的配置
     */
    validate(config) {
        const errors = [];
        const schema = this.getSchema();

        // 简单的配置验证
        if (config.webrtc?.connectionTimeout && 
            (config.webrtc.connectionTimeout < 1000 || config.webrtc.connectionTimeout > 60000)) {
            errors.push('webrtc.connectionTimeout must be between 1000 and 60000');
        }

        if (config.ui?.logLevel && 
            !['debug', 'info', 'warn', 'error'].includes(config.ui.logLevel)) {
            errors.push('ui.logLevel must be one of: debug, info, warn, error');
        }

        return {
            valid: errors.length === 0,
            errors
        };
    }

    /**
     * 从本地存储加载配置
     * @private
     */
    _loadFromStorage() {
        try {
            const saved = localStorage.getItem(this._storageKey);
            return saved ? JSON.parse(saved) : {};
        } catch (error) {
            console.warn('ConfigManager: Failed to load from storage:', error);
            return {};
        }
    }

    /**
     * 合并配置对象
     * @private
     */
    _mergeConfigs(defaultConfig, userConfig) {
        const merged = { ...defaultConfig };

        for (const [key, value] of Object.entries(userConfig)) {
            if (value && typeof value === 'object' && !Array.isArray(value)) {
                merged[key] = this._mergeConfigs(merged[key] || {}, value);
            } else {
                merged[key] = value;
            }
        }

        return merged;
    }

    /**
     * 设置动态配置
     * @private
     */
    _setDynamicConfig() {
        // 设置信令服务器URL
        if (!this._config.signaling.url) {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const host = window.location.host;
            this._config.signaling.url = `${protocol}//${host}/ws`;
        }

        // 根据设备类型调整配置
        if (this._isMobileDevice()) {
            this._config.input.enableGamepad = false;
            this._config.ui.autoHideControls = true;
            this._config.ui.autoHideDelay = 2000;
        }
    }

    /**
     * 获取可序列化的配置（排除动态生成的配置）
     * @private
     */
    _getSerializableConfig() {
        const config = { ...this._config };
        
        // 不保存动态生成的URL
        if (config.signaling) {
            delete config.signaling.url;
        }

        return config;
    }

    /**
     * 通知配置变化
     * @private
     */
    _notifyChange(eventType, data) {
        // 通过事件总线通知
        if (this.eventBus) {
            this.eventBus.emit(eventType, data);
        }

        // 通过回调函数通知
        this._changeCallbacks.forEach(callback => {
            try {
                callback(eventType, data);
            } catch (error) {
                console.error('ConfigManager: Error in change callback:', error);
            }
        });
    }

    /**
     * 检测是否为移动设备
     * @private
     */
    _isMobileDevice() {
        return /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
    }
}

// 导出ConfigManager类
if (typeof module !== 'undefined' && module.exports) {
    module.exports = ConfigManager;
} else if (typeof window !== 'undefined') {
    window.ConfigManager = ConfigManager;
}
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
        
        // 配置同步相关
        this._serverConfig = null;
        this._configCache = new Map();
        this._lastFetchTime = 0;
        this._fetchRetryCount = 0;
        this._maxRetries = 3;
        this._retryDelay = 2000;
        this._cacheTimeout = 300000; // 5分钟缓存超时
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

    /**
     * 从服务器获取WebRTC配置
     * @param {boolean} forceRefresh - 是否强制刷新缓存
     * @returns {Promise<Object>} WebRTC配置对象
     */
    async fetchWebRTCConfig(forceRefresh = false) {
        const now = Date.now();
        const cacheKey = 'webrtc-config';
        
        // 检查缓存是否有效
        if (!forceRefresh && this._configCache.has(cacheKey)) {
            const cached = this._configCache.get(cacheKey);
            if (now - cached.timestamp < this._cacheTimeout) {
                console.log('ConfigManager: 使用缓存的WebRTC配置');
                return cached.data;
            }
        }

        // 检查是否需要等待重试间隔
        if (this._fetchRetryCount > 0 && now - this._lastFetchTime < this._retryDelay) {
            console.log('ConfigManager: 等待重试间隔');
            await new Promise(resolve => setTimeout(resolve, this._retryDelay - (now - this._lastFetchTime)));
        }

        try {
            console.log('ConfigManager: 从服务器获取WebRTC配置...');
            const response = await fetch('/api/webrtc/config', {
                method: 'GET',
                headers: {
                    'Accept': 'application/json',
                    'Cache-Control': 'no-cache'
                },
                timeout: 10000 // 10秒超时
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

            const result = await response.json();
            this._lastFetchTime = now;

            // 验证响应格式
            if (!this._validateServerConfigResponse(result)) {
                throw new Error('Invalid server configuration response format');
            }

            // 提取配置数据
            const config = result.success ? result.data : null;
            if (!config) {
                throw new Error(result.error?.message || 'Server returned unsuccessful response');
            }

            // 验证配置内容
            const validatedConfig = this._validateAndNormalizeWebRTCConfig(config);

            // 缓存配置
            this._configCache.set(cacheKey, {
                data: validatedConfig,
                timestamp: now
            });

            // 重置重试计数
            this._fetchRetryCount = 0;

            console.log('ConfigManager: WebRTC配置获取成功', validatedConfig);
            
            // 详细打印ICE服务器信息
            if (validatedConfig.iceServers && validatedConfig.iceServers.length > 0) {
                console.log('🌐 从服务器API获取的ICE服务器配置:');
                validatedConfig.iceServers.forEach((server, index) => {
                    console.log(`  服务器 #${index + 1}:`, {
                        urls: server.urls,
                        username: server.username ? '***' : undefined,
                        credential: server.credential ? '***' : undefined
                    });
                });
            } else {
                console.warn('⚠️ 服务器返回的配置中没有ICE服务器');
            }
            
            // 触发配置更新事件
            this._notifyChange('config:webrtc-fetched', {
                config: validatedConfig,
                source: 'server'
            });

            return validatedConfig;

        } catch (error) {
            this._fetchRetryCount++;
            this._lastFetchTime = now;

            console.warn(`ConfigManager: WebRTC配置获取失败 (尝试 ${this._fetchRetryCount}/${this._maxRetries})`, error);

            // 如果还有重试机会，递归重试
            if (this._fetchRetryCount < this._maxRetries) {
                console.log(`ConfigManager: ${this._retryDelay}ms后重试...`);
                await new Promise(resolve => setTimeout(resolve, this._retryDelay));
                return this.fetchWebRTCConfig(forceRefresh);
            }

            // 重试次数用完，使用默认配置
            console.error('ConfigManager: WebRTC配置获取失败，使用默认配置', error);
            
            const fallbackConfig = this._getDefaultWebRTCConfig();
            
            // 触发配置获取失败事件
            this._notifyChange('config:webrtc-fetch-failed', {
                error,
                fallbackConfig,
                retryCount: this._fetchRetryCount
            });

            return fallbackConfig;
        }
    }

    /**
     * 验证服务器配置响应格式
     * @param {Object} response - 服务器响应
     * @returns {boolean} 是否有效
     * @private
     */
    _validateServerConfigResponse(response) {
        if (!response || typeof response !== 'object') {
            return false;
        }

        // 检查必需字段
        if (typeof response.success !== 'boolean') {
            return false;
        }

        if (response.success) {
            // 成功响应必须有data字段
            return response.data && typeof response.data === 'object';
        } else {
            // 失败响应必须有error字段
            return response.error && typeof response.error === 'object';
        }
    }

    /**
     * 验证和标准化WebRTC配置
     * @param {Object} config - 原始配置
     * @returns {Object} 验证后的配置
     * @private
     */
    _validateAndNormalizeWebRTCConfig(config) {
        const validated = {
            iceServers: [],
            videoEnabled: true,
            audioEnabled: false,
            videoCodec: 'h264',
            audioCodec: 'opus',
            videoWidth: 1920,
            videoHeight: 1080,
            videoFPS: 30,
            connectionTimeout: 10000,
            iceTransportPolicy: 'all',
            bundlePolicy: 'balanced',
            rtcpMuxPolicy: 'require'
        };

        const validationErrors = [];

        try {
            // 验证ICE服务器配置 - 支持两种字段名格式
            const iceServersField = config.iceServers || config.ice_servers;
            if (Array.isArray(iceServersField)) {
                for (let i = 0; i < iceServersField.length; i++) {
                    const server = iceServersField[i];
                    if (this._validateICEServer(server)) {
                        validated.iceServers.push(server);
                    } else {
                        validationErrors.push(`Invalid ICE server at index ${i}`);
                    }
                }
            } else if (iceServersField !== undefined) {
                validationErrors.push('iceServers/ice_servers must be an array');
            }

            // 如果没有有效的ICE服务器，使用默认的
            if (validated.iceServers.length === 0) {
                console.warn('ConfigManager: 没有有效的ICE服务器，使用默认配置');
                validated.iceServers = this._getDefaultWebRTCConfig().iceServers;
                validationErrors.push('No valid ICE servers found, using defaults');
            }

            // 验证布尔值配置
            if (config.video_enabled !== undefined) {
                if (typeof config.video_enabled === 'boolean') {
                    validated.videoEnabled = config.video_enabled;
                } else {
                    validationErrors.push('video_enabled must be boolean');
                }
            }
            
            if (config.audio_enabled !== undefined) {
                if (typeof config.audio_enabled === 'boolean') {
                    validated.audioEnabled = config.audio_enabled;
                } else {
                    validationErrors.push('audio_enabled must be boolean');
                }
            }

            // 验证字符串配置
            const stringConfigs = [
                { key: 'video_codec', target: 'videoCodec', allowedValues: ['h264', 'vp8', 'vp9', 'av1'] },
                { key: 'audio_codec', target: 'audioCodec', allowedValues: ['opus', 'g722', 'pcmu', 'pcma'] },
                { key: 'ice_transport_policy', target: 'iceTransportPolicy', allowedValues: ['all', 'relay'] },
                { key: 'bundle_policy', target: 'bundlePolicy', allowedValues: ['balanced', 'max-compat', 'max-bundle'] },
                { key: 'rtcp_mux_policy', target: 'rtcpMuxPolicy', allowedValues: ['negotiate', 'require'] }
            ];

            for (const stringConfig of stringConfigs) {
                if (config[stringConfig.key] !== undefined) {
                    if (typeof config[stringConfig.key] === 'string' && config[stringConfig.key].trim()) {
                        const value = config[stringConfig.key].trim();
                        if (stringConfig.allowedValues.includes(value)) {
                            validated[stringConfig.target] = value;
                        } else {
                            validationErrors.push(`${stringConfig.key} must be one of: ${stringConfig.allowedValues.join(', ')}`);
                        }
                    } else {
                        validationErrors.push(`${stringConfig.key} must be a non-empty string`);
                    }
                }
            }

            // 验证数值配置
            const numberConfigs = [
                { key: 'video_width', target: 'videoWidth', min: 320, max: 7680 },
                { key: 'video_height', target: 'videoHeight', min: 240, max: 4320 },
                { key: 'video_fps', target: 'videoFPS', min: 1, max: 120 },
                { key: 'connection_timeout', target: 'connectionTimeout', min: 1000, max: 60000 }
            ];

            for (const numberConfig of numberConfigs) {
                if (config[numberConfig.key] !== undefined) {
                    const value = Number(config[numberConfig.key]);
                    if (Number.isInteger(value) && value >= numberConfig.min && value <= numberConfig.max) {
                        validated[numberConfig.target] = value;
                    } else {
                        validationErrors.push(`${numberConfig.key} must be an integer between ${numberConfig.min} and ${numberConfig.max}`);
                    }
                }
            }

            // 记录验证警告
            if (validationErrors.length > 0) {
                console.warn('ConfigManager: WebRTC配置验证警告:', validationErrors);
                
                // 触发验证警告事件
                this._notifyChange('config:webrtc-validation-warnings', {
                    warnings: validationErrors,
                    originalConfig: config,
                    validatedConfig: validated
                });
            }

            return validated;

        } catch (error) {
            console.error('ConfigManager: WebRTC配置验证失败', error);
            
            // 触发验证错误事件
            this._notifyChange('config:webrtc-validation-error', {
                error,
                originalConfig: config,
                fallbackConfig: this._getDefaultWebRTCConfig()
            });
            
            // 返回默认配置
            return this._getDefaultWebRTCConfig();
        }
    }

    /**
     * 验证单个ICE服务器配置
     * @param {Object} server - ICE服务器配置
     * @returns {boolean} 是否有效
     * @private
     */
    _validateICEServer(server) {
        if (!server || typeof server !== 'object') {
            return false;
        }

        // 检查urls字段
        if (!server.urls) {
            return false;
        }

        // urls可以是字符串或字符串数组
        if (typeof server.urls === 'string') {
            return server.urls.trim() !== '';
        }

        if (Array.isArray(server.urls)) {
            return server.urls.length > 0 && server.urls.every(url => 
                typeof url === 'string' && url.trim() !== ''
            );
        }

        return false;
    }

    /**
     * 获取默认WebRTC配置
     * @returns {Object} 默认配置
     * @private
     */
    _getDefaultWebRTCConfig() {
        return {
            iceServers: [
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' },
                { urls: 'stun:stun2.l.google.com:19302' }
            ],
            videoEnabled: true,
            audioEnabled: false,
            videoCodec: 'h264',
            audioCodec: 'opus',
            videoWidth: 1920,
            videoHeight: 1080,
            videoFPS: 30,
            connectionTimeout: 10000,
            iceTransportPolicy: 'all',
            bundlePolicy: 'balanced',
            rtcpMuxPolicy: 'require'
        };
    }

    /**
     * 同步WebRTC配置到本地配置
     * @param {Object} webrtcConfig - WebRTC配置
     */
    syncWebRTCConfig(webrtcConfig) {
        if (!webrtcConfig) {
            console.warn('ConfigManager: WebRTC配置为空，跳过同步');
            return;
        }

        // 更新本地配置中的WebRTC部分
        this.set('webrtc.iceServers', webrtcConfig.iceServers);
        this.set('webrtc.connectionTimeout', webrtcConfig.connectionTimeout);
        this.set('webrtc.iceTransportPolicy', webrtcConfig.iceTransportPolicy);
        this.set('webrtc.bundlePolicy', webrtcConfig.bundlePolicy);
        this.set('webrtc.rtcpMuxPolicy', webrtcConfig.rtcpMuxPolicy);

        // 更新视频配置
        this.set('video.preferredCodec', webrtcConfig.videoCodec);
        
        // 更新音频配置
        this.set('audio.enabled', webrtcConfig.audioEnabled);
        this.set('audio.preferredCodec', webrtcConfig.audioCodec);

        console.log('ConfigManager: WebRTC配置已同步到本地配置');
        
        // 触发配置同步事件
        this._notifyChange('config:webrtc-synced', {
            webrtcConfig,
            localConfig: this._config
        });
    }

    /**
     * 清除配置缓存
     * @param {string} key - 缓存键，如果不提供则清除所有缓存
     */
    clearConfigCache(key = null) {
        if (key) {
            this._configCache.delete(key);
            console.log(`ConfigManager: 已清除缓存 ${key}`);
        } else {
            this._configCache.clear();
            console.log('ConfigManager: 已清除所有配置缓存');
        }
        
        // 重置重试计数
        this._fetchRetryCount = 0;
    }

    /**
     * 获取配置缓存状态
     * @returns {Object} 缓存状态信息
     */
    getCacheStatus() {
        const now = Date.now();
        const status = {
            cacheSize: this._configCache.size,
            lastFetchTime: this._lastFetchTime,
            retryCount: this._fetchRetryCount,
            entries: []
        };

        for (const [key, value] of this._configCache.entries()) {
            status.entries.push({
                key,
                age: now - value.timestamp,
                expired: now - value.timestamp > this._cacheTimeout
            });
        }

        return status;
    }

    /**
     * 重置配置获取状态
     */
    resetFetchStatus() {
        this._fetchRetryCount = 0;
        this._lastFetchTime = 0;
        console.log('ConfigManager: 配置获取状态已重置');
        
        this._notifyChange('config:fetch-status-reset', {
            timestamp: Date.now()
        });
    }

    /**
     * 获取配置健康状态
     * @returns {Object} 健康状态信息
     */
    getConfigHealth() {
        const now = Date.now();
        const webrtcCache = this._configCache.get('webrtc-config');
        
        const health = {
            overall: 'healthy',
            issues: [],
            lastFetch: this._lastFetchTime,
            retryCount: this._fetchRetryCount,
            cacheValid: false,
            configSource: 'unknown'
        };

        // 检查缓存状态
        if (webrtcCache) {
            const cacheAge = now - webrtcCache.timestamp;
            health.cacheValid = cacheAge < this._cacheTimeout;
            health.configSource = 'server-cached';
            
            if (cacheAge > this._cacheTimeout) {
                health.issues.push('Configuration cache expired');
                health.overall = 'warning';
            }
        } else {
            health.issues.push('No cached configuration available');
            health.configSource = 'default';
            health.overall = 'warning';
        }

        // 检查重试状态
        if (this._fetchRetryCount > 0) {
            health.issues.push(`Configuration fetch failed ${this._fetchRetryCount} times`);
            health.overall = this._fetchRetryCount >= this._maxRetries ? 'critical' : 'warning';
        }

        // 检查最后获取时间
        if (this._lastFetchTime === 0) {
            health.issues.push('Configuration never fetched from server');
            health.overall = 'warning';
        } else if (now - this._lastFetchTime > 600000) { // 10分钟
            health.issues.push('Configuration not updated for more than 10 minutes');
            if (health.overall === 'healthy') {
                health.overall = 'warning';
            }
        }

        return health;
    }

    /**
     * 诊断配置问题
     * @returns {Object} 诊断结果
     */
    async diagnoseConfigIssues() {
        const diagnosis = {
            timestamp: Date.now(),
            health: this.getConfigHealth(),
            networkTest: null,
            serverTest: null,
            recommendations: []
        };

        // 网络连接测试
        try {
            const networkStart = Date.now();
            const response = await fetch('/api/status', { 
                method: 'HEAD',
                timeout: 5000 
            });
            diagnosis.networkTest = {
                success: response.ok,
                latency: Date.now() - networkStart,
                status: response.status
            };
        } catch (error) {
            diagnosis.networkTest = {
                success: false,
                error: error.message
            };
            diagnosis.recommendations.push('Check network connectivity');
        }

        // 服务器配置端点测试
        try {
            const serverStart = Date.now();
            const response = await fetch('/api/webrtc/config', {
                method: 'HEAD',
                timeout: 5000
            });
            diagnosis.serverTest = {
                success: response.ok,
                latency: Date.now() - serverStart,
                status: response.status
            };
        } catch (error) {
            diagnosis.serverTest = {
                success: false,
                error: error.message
            };
            diagnosis.recommendations.push('WebRTC configuration endpoint is not accessible');
        }

        // 生成建议
        if (diagnosis.health.overall === 'critical') {
            diagnosis.recommendations.push('Clear configuration cache and retry');
            diagnosis.recommendations.push('Check server status and network connectivity');
        } else if (diagnosis.health.overall === 'warning') {
            diagnosis.recommendations.push('Refresh configuration cache');
        }

        if (this._fetchRetryCount > 0) {
            diagnosis.recommendations.push('Reset fetch retry counter');
        }

        return diagnosis;
    }
}

// 导出ConfigManager类
if (typeof module !== 'undefined' && module.exports) {
    module.exports = ConfigManager;
} else if (typeof window !== 'undefined') {
    window.ConfigManager = ConfigManager;
}
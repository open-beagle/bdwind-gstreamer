/**
 * WebRTCManager - WebRTC连接管理器
 * 管理PeerConnection、媒体流和ICE候选处理
 */
class WebRTCManager {
    constructor(eventBus, config) {
        this.eventBus = eventBus;
        this.config = config;
        
        // PeerConnection
        this.pc = null;
        this.connectionState = 'closed';
        this.iceConnectionState = 'closed';
        this.signalingState = 'closed';
        
        // 媒体流
        this.videoElement = null;
        this.audioElement = null;
        this.remoteStream = null;
        
        // ICE候选统计
        this.iceStats = {
            sent: 0,
            received: 0,
            failed: 0,
            types: {
                host: 0,
                srflx: 0,
                prflx: 0,
                relay: 0,
                unknown: 0
            }
        };
        
        // 连接统计
        this.connectionStats = {
            connectTime: null,
            disconnectTime: null,
            totalConnections: 0,
            reconnectAttempts: 0,
            lastStatsUpdate: null,
            bytesReceived: 0,
            bytesSent: 0,
            packetsReceived: 0,
            packetsSent: 0,
            packetsLost: 0,
            jitter: 0,
            rtt: 0
        };
        
        // 配置
        this.iceServers = [];
        this.connectionTimeout = 10000;
        this.statsInterval = 5000;
        this.statsTimer = null;
        
        this._loadConfig();
    }

    /**
     * 加载配置
     * @private
     */
    _loadConfig() {
        if (this.config) {
            const webrtcConfig = this.config.get('webrtc', {});
            this.iceServers = webrtcConfig.iceServers || [
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' }
            ];
            this.connectionTimeout = webrtcConfig.connectionTimeout || 10000;
        }
        
        // 获取统计间隔配置
        if (this.config) {
            const monitoringConfig = this.config.get('monitoring', {});
            this.statsInterval = monitoringConfig.statsInterval || 5000;
        }
    }

    /**
     * 设置媒体元素
     */
    setMediaElements(videoElement, audioElement) {
        this.videoElement = videoElement;
        this.audioElement = audioElement;
        Logger.info('WebRTCManager: 媒体元素已设置');
    }

    /**
     * 初始化WebRTC连接
     */
    async initialize() {
        try {
            Logger.info('WebRTCManager: 开始初始化WebRTC连接');
            
            // 清理现有连接
            if (this.pc) {
                await this.cleanup();
            }
            
            // 获取ICE服务器配置
            await this._fetchIceServers();
            
            // 创建PeerConnection
            this._createPeerConnection();
            
            // 启动统计收集
            this._startStatsCollection();
            
            // 启动连接健康监控
            this._startConnectionHealthMonitoring();
            
            Logger.info('WebRTCManager: WebRTC连接初始化完成');
            this.eventBus?.emit('webrtc:initialized');
            
        } catch (error) {
            Logger.error('WebRTCManager: 初始化失败', error);
            this.eventBus?.emit('webrtc:initialization-failed', { error });
            throw error;
        }
    }

    /**
     * 处理远程offer
     */
    async handleOffer(offer) {
        if (!this.pc) {
            throw new Error('PeerConnection not initialized');
        }

        try {
            Logger.info('WebRTCManager: 处理远程offer');
            
            // 验证offer格式
            if (!offer || !offer.type || !offer.sdp) {
                throw new Error('Invalid offer format');
            }
            
            if (offer.type !== 'offer') {
                throw new Error(`Expected offer, got ${offer.type}`);
            }
            
            // 设置远程描述
            await this.pc.setRemoteDescription(new RTCSessionDescription(offer));
            Logger.info('WebRTCManager: 远程描述已设置');
            
            // 创建并设置本地answer
            const answer = await this.pc.createAnswer();
            await this.pc.setLocalDescription(answer);
            
            Logger.info('WebRTCManager: 本地answer已创建');
            this.eventBus?.emit('webrtc:answer-created', answer);
            
            return answer;
            
        } catch (error) {
            Logger.error('WebRTCManager: 处理offer失败', error);
            this.eventBus?.emit('webrtc:offer-failed', { error });
            throw error;
        }
    }

    /**
     * 处理远程answer
     */
    async handleAnswer(answer) {
        if (!this.pc) {
            throw new Error('PeerConnection not initialized');
        }

        try {
            Logger.info('WebRTCManager: 处理远程answer');
            
            // 验证answer格式
            if (!answer || !answer.type || !answer.sdp) {
                throw new Error('Invalid answer format');
            }
            
            if (answer.type !== 'answer') {
                throw new Error(`Expected answer, got ${answer.type}`);
            }
            
            // 设置远程描述
            await this.pc.setRemoteDescription(new RTCSessionDescription(answer));
            Logger.info('WebRTCManager: 远程answer已设置');
            
            this.eventBus?.emit('webrtc:answer-processed');
            
        } catch (error) {
            Logger.error('WebRTCManager: 处理answer失败', error);
            this.eventBus?.emit('webrtc:answer-failed', { error });
            throw error;
        }
    }

    /**
     * 处理ICE候选
     */
    async handleIceCandidate(candidate) {
        if (!this.pc) {
            Logger.warn('WebRTCManager: PeerConnection未初始化，忽略ICE候选');
            return;
        }

        try {
            if (!candidate || !candidate.candidate) {
                Logger.warn('WebRTCManager: 收到空的ICE候选');
                return;
            }
            
            Logger.debug('WebRTCManager: 处理ICE候选', candidate);
            
            // 添加ICE候选
            await this.pc.addIceCandidate(new RTCIceCandidate(candidate));
            
            // 更新统计
            this.iceStats.received++;
            const candidateType = this._getIceCandidateType(candidate.candidate);
            this.iceStats.types[candidateType]++;
            
            Logger.debug(`WebRTCManager: ICE候选已添加 (${candidateType})`);
            this.eventBus?.emit('webrtc:ice-candidate-added', { candidate, type: candidateType });
            
        } catch (error) {
            Logger.error('WebRTCManager: 添加ICE候选失败', error);
            this.iceStats.failed++;
            this.eventBus?.emit('webrtc:ice-candidate-failed', { candidate, error });
        }
    }

    /**
     * 创建offer
     */
    async createOffer(options = {}) {
        if (!this.pc) {
            throw new Error('PeerConnection not initialized');
        }

        try {
            Logger.info('WebRTCManager: 创建offer');
            
            const offer = await this.pc.createOffer(options);
            await this.pc.setLocalDescription(offer);
            
            Logger.info('WebRTCManager: Offer已创建');
            this.eventBus?.emit('webrtc:offer-created', offer);
            
            return offer;
            
        } catch (error) {
            Logger.error('WebRTCManager: 创建offer失败', error);
            this.eventBus?.emit('webrtc:offer-creation-failed', { error });
            throw error;
        }
    }

    /**
     * 获取连接状态
     */
    getConnectionState() {
        return {
            connectionState: this.connectionState,
            iceConnectionState: this.iceConnectionState,
            signalingState: this.signalingState,
            hasRemoteStream: !!this.remoteStream,
            iceStats: { ...this.iceStats },
            connectionStats: { ...this.connectionStats }
        };
    }

    /**
     * 获取连接统计
     */
    async getStats() {
        if (!this.pc) {
            return null;
        }

        try {
            const stats = await this.pc.getStats();
            const parsedStats = this._parseStats(stats);
            
            // 更新内部统计
            this.connectionStats.lastStatsUpdate = Date.now();
            Object.assign(this.connectionStats, parsedStats);
            
            return parsedStats;
            
        } catch (error) {
            Logger.error('WebRTCManager: 获取统计失败', error);
            return null;
        }
    }

    /**
     * 重置连接
     */
    async reset() {
        Logger.info('WebRTCManager: 重置连接');
        
        await this.cleanup();
        await this.initialize();
        
        this.connectionStats.reconnectAttempts++;
        this.eventBus?.emit('webrtc:reset');
    }

    /**
     * 启动连接健康监控
     * @private
     */
    _startConnectionHealthMonitoring() {
        // 清理现有的监控
        this._stopConnectionHealthMonitoring();
        
        // 每5秒检查一次连接健康状态
        this.healthCheckInterval = setInterval(() => {
            this._checkConnectionHealth();
        }, 5000);
        
        Logger.info('WebRTCManager: 连接健康监控已启动');
    }

    /**
     * 停止连接健康监控
     * @private
     */
    _stopConnectionHealthMonitoring() {
        if (this.healthCheckInterval) {
            clearInterval(this.healthCheckInterval);
            this.healthCheckInterval = null;
            Logger.info('WebRTCManager: 连接健康监控已停止');
        }
    }

    /**
     * 检查连接健康状态
     * @private
     */
    async _checkConnectionHealth() {
        if (!this.pc) return;

        // 收集连接健康数据
        const healthData = await this._collectHealthData();
        
        // 检查PeerConnection状态
        if (this.pc.connectionState === 'failed' || this.pc.connectionState === 'disconnected') {
            Logger.warn(`WebRTCManager: 连接状态异常 - ${this.pc.connectionState}`);
            this.eventBus?.emit('webrtc:connection-unhealthy', {
                connectionState: this.pc.connectionState,
                iceConnectionState: this.pc.iceConnectionState,
                healthData
            });
            return;
        }

        // 检查媒体流状态
        if (this.pc.connectionState === 'connected') {
            await this._checkMediaFlow(healthData);
        }
        
        // 触发健康检查完成事件
        this.eventBus?.emit('webrtc:health-check-completed', healthData);
    }

    /**
     * 收集连接健康数据
     * @private
     */
    async _collectHealthData() {
        const healthData = {
            timestamp: Date.now(),
            connectionState: this.pc?.connectionState || 'unknown',
            iceConnectionState: this.pc?.iceConnectionState || 'unknown',
            signalingState: this.pc?.signalingState || 'unknown',
            iceGatheringState: this.pc?.iceGatheringState || 'unknown',
            hasRemoteStream: !!this.remoteStream,
            videoElement: {
                hasSource: !!(this.videoElement?.srcObject),
                paused: this.videoElement?.paused || true,
                readyState: this.videoElement?.readyState || 0,
                videoWidth: this.videoElement?.videoWidth || 0,
                videoHeight: this.videoElement?.videoHeight || 0,
                currentTime: this.videoElement?.currentTime || 0
            },
            mediaFlow: await this._analyzeMediaFlow(),
            stats: await this._getDetailedStats()
        };
        
        return healthData;
    }

    /**
     * 分析媒体流状态
     * @private
     */
    async _analyzeMediaFlow() {
        const mediaFlow = {
            videoTracks: [],
            audioTracks: [],
            quality: {
                frameRate: 0,
                resolution: { width: 0, height: 0 },
                bitrate: 0,
                packetsLost: 0,
                jitter: 0,
                rtt: 0
            }
        };
        
        if (this.videoElement && this.videoElement.srcObject) {
            const stream = this.videoElement.srcObject;
            
            // 分析视频轨道
            const videoTracks = stream.getVideoTracks();
            videoTracks.forEach(track => {
                const settings = track.getSettings();
                mediaFlow.videoTracks.push({
                    id: track.id,
                    kind: track.kind,
                    label: track.label,
                    readyState: track.readyState,
                    enabled: track.enabled,
                    muted: track.muted,
                    settings: settings
                });
                
                // 更新质量信息
                if (settings.frameRate) {
                    mediaFlow.quality.frameRate = settings.frameRate;
                }
                if (settings.width && settings.height) {
                    mediaFlow.quality.resolution = {
                        width: settings.width,
                        height: settings.height
                    };
                }
            });
            
            // 分析音频轨道
            const audioTracks = stream.getAudioTracks();
            audioTracks.forEach(track => {
                const settings = track.getSettings();
                mediaFlow.audioTracks.push({
                    id: track.id,
                    kind: track.kind,
                    label: track.label,
                    readyState: track.readyState,
                    enabled: track.enabled,
                    muted: track.muted,
                    settings: settings
                });
            });
        }
        
        return mediaFlow;
    }

    /**
     * 获取详细的WebRTC统计信息
     * @private
     */
    async _getDetailedStats() {
        if (!this.pc) return null;
        
        try {
            const stats = await this.pc.getStats();
            const detailedStats = {
                inbound: [],
                outbound: [],
                candidatePairs: [],
                certificates: [],
                codecs: [],
                summary: {
                    bytesReceived: 0,
                    bytesSent: 0,
                    packetsReceived: 0,
                    packetsSent: 0,
                    packetsLost: 0,
                    jitter: 0,
                    rtt: 0,
                    frameRate: 0,
                    framesReceived: 0,
                    framesDropped: 0
                }
            };
            
            stats.forEach(report => {
                switch (report.type) {
                    case 'inbound-rtp':
                        detailedStats.inbound.push(report);
                        detailedStats.summary.bytesReceived += report.bytesReceived || 0;
                        detailedStats.summary.packetsReceived += report.packetsReceived || 0;
                        detailedStats.summary.packetsLost += report.packetsLost || 0;
                        detailedStats.summary.jitter = Math.max(detailedStats.summary.jitter, report.jitter || 0);
                        detailedStats.summary.framesReceived += report.framesReceived || 0;
                        detailedStats.summary.framesDropped += report.framesDropped || 0;
                        if (report.framesPerSecond) {
                            detailedStats.summary.frameRate = report.framesPerSecond;
                        }
                        break;
                        
                    case 'outbound-rtp':
                        detailedStats.outbound.push(report);
                        detailedStats.summary.bytesSent += report.bytesSent || 0;
                        detailedStats.summary.packetsSent += report.packetsSent || 0;
                        break;
                        
                    case 'candidate-pair':
                        detailedStats.candidatePairs.push(report);
                        if (report.state === 'succeeded' && report.currentRoundTripTime) {
                            detailedStats.summary.rtt = Math.max(detailedStats.summary.rtt, report.currentRoundTripTime * 1000);
                        }
                        break;
                        
                    case 'certificate':
                        detailedStats.certificates.push(report);
                        break;
                        
                    case 'codec':
                        detailedStats.codecs.push(report);
                        break;
                }
            });
            
            return detailedStats;
        } catch (error) {
            Logger.error('WebRTCManager: 获取详细统计失败', error);
            return null;
        }
    }

    /**
     * 检查媒体流状态
     * @private
     */
    async _checkMediaFlow(healthData) {
        const mediaFlowIssues = [];
        
        if (this.videoElement && this.videoElement.srcObject) {
            const stream = this.videoElement.srcObject;
            const videoTracks = stream.getVideoTracks();
            
            if (videoTracks.length > 0) {
                const track = videoTracks[0];
                
                // 检查轨道状态
                if (track.readyState === 'ended') {
                    Logger.warn('WebRTCManager: 视频轨道已结束，尝试重连');
                    mediaFlowIssues.push({
                        type: 'track-ended',
                        severity: 'critical',
                        message: '视频轨道已结束'
                    });
                    
                    this.eventBus?.emit('webrtc:media-track-ended', {
                        kind: 'video',
                        track: track,
                        healthData
                    });
                    return;
                }
                
                // 检查视频是否在播放
                if (this.videoElement.paused && this.videoElement.readyState >= 2) {
                    Logger.warn('WebRTCManager: 视频暂停，尝试恢复播放');
                    mediaFlowIssues.push({
                        type: 'video-paused',
                        severity: 'warning',
                        message: '视频处于暂停状态'
                    });
                    
                    this._ensureVideoPlayback();
                }
                
                // 检查分辨率变化
                await this._checkResolutionChanges(healthData);
                
                // 检查帧率
                await this._checkFrameRate(healthData, mediaFlowIssues);
                
                // 检查视频质量
                await this._checkVideoQuality(healthData, mediaFlowIssues);
            }
        } else {
            mediaFlowIssues.push({
                type: 'no-media-source',
                severity: 'critical',
                message: '没有媒体源'
            });
        }
        
        // 如果有媒体流问题，触发事件
        if (mediaFlowIssues.length > 0) {
            this.eventBus?.emit('webrtc:media-flow-issues', {
                issues: mediaFlowIssues,
                healthData
            });
        }
    }

    /**
     * 检查分辨率变化
     * @private
     */
    async _checkResolutionChanges(healthData) {
        if (!this.videoElement) return;
        
        const currentResolution = {
            width: this.videoElement.videoWidth,
            height: this.videoElement.videoHeight
        };
        
        if (!this.lastResolution) {
            this.lastResolution = currentResolution;
            return;
        }
        
        // 检查分辨率是否发生变化
        if (this.lastResolution.width !== currentResolution.width || 
            this.lastResolution.height !== currentResolution.height) {
            
            Logger.info(`WebRTCManager: 视频分辨率变化 ${this.lastResolution.width}x${this.lastResolution.height} -> ${currentResolution.width}x${currentResolution.height}`);
            
            this.eventBus?.emit('webrtc:resolution-changed', {
                oldResolution: this.lastResolution,
                newResolution: currentResolution,
                healthData
            });
            
            this.lastResolution = currentResolution;
        }
    }

    /**
     * 检查帧率
     * @private
     */
    async _checkFrameRate(healthData, mediaFlowIssues) {
        if (!healthData?.stats?.summary) return;
        
        const currentFrameRate = healthData.stats.summary.frameRate;
        const now = Date.now();
        
        if (!this.frameRateHistory) {
            this.frameRateHistory = [];
        }
        
        // 记录帧率历史
        this.frameRateHistory.push({
            timestamp: now,
            frameRate: currentFrameRate,
            framesReceived: healthData.stats.summary.framesReceived,
            framesDropped: healthData.stats.summary.framesDropped
        });
        
        // 保持最近30秒的历史记录
        this.frameRateHistory = this.frameRateHistory.filter(
            record => now - record.timestamp <= 30000
        );
        
        // 分析帧率趋势
        if (this.frameRateHistory.length >= 3) {
            const recentFrameRates = this.frameRateHistory.slice(-3).map(r => r.frameRate);
            const avgFrameRate = recentFrameRates.reduce((a, b) => a + b, 0) / recentFrameRates.length;
            
            // 检查帧率是否过低
            if (avgFrameRate < 15 && avgFrameRate > 0) {
                mediaFlowIssues.push({
                    type: 'low-frame-rate',
                    severity: 'warning',
                    message: `帧率过低: ${avgFrameRate.toFixed(1)} fps`
                });
            }
            
            // 检查丢帧率
            const totalFrames = healthData.stats.summary.framesReceived + healthData.stats.summary.framesDropped;
            if (totalFrames > 0) {
                const dropRate = (healthData.stats.summary.framesDropped / totalFrames) * 100;
                if (dropRate > 5) {
                    mediaFlowIssues.push({
                        type: 'high-frame-drop-rate',
                        severity: 'warning',
                        message: `丢帧率过高: ${dropRate.toFixed(1)}%`
                    });
                }
            }
        }
        
        // 触发帧率更新事件
        this.eventBus?.emit('webrtc:frame-rate-updated', {
            currentFrameRate,
            averageFrameRate: this.frameRateHistory.length > 0 ? 
                this.frameRateHistory.reduce((sum, r) => sum + r.frameRate, 0) / this.frameRateHistory.length : 0,
            framesReceived: healthData.stats.summary.framesReceived,
            framesDropped: healthData.stats.summary.framesDropped,
            healthData
        });
    }

    /**
     * 检查视频质量
     * @private
     */
    async _checkVideoQuality(healthData, mediaFlowIssues) {
        if (!healthData?.stats?.summary) return;
        
        const stats = healthData.stats.summary;
        
        // 检查网络质量指标
        if (stats.rtt > 200) {
            mediaFlowIssues.push({
                type: 'high-rtt',
                severity: 'warning',
                message: `网络延迟过高: ${stats.rtt.toFixed(0)}ms`
            });
        }
        
        if (stats.packetsLost > 0 && stats.packetsReceived > 0) {
            const lossRate = (stats.packetsLost / (stats.packetsReceived + stats.packetsLost)) * 100;
            if (lossRate > 2) {
                mediaFlowIssues.push({
                    type: 'high-packet-loss',
                    severity: lossRate > 5 ? 'critical' : 'warning',
                    message: `丢包率过高: ${lossRate.toFixed(1)}%`
                });
            }
        }
        
        if (stats.jitter > 0.05) {
            mediaFlowIssues.push({
                type: 'high-jitter',
                severity: 'warning',
                message: `网络抖动过高: ${(stats.jitter * 1000).toFixed(1)}ms`
            });
        }
        
        // 触发质量更新事件
        this.eventBus?.emit('webrtc:quality-updated', {
            rtt: stats.rtt,
            packetLoss: stats.packetsLost > 0 ? 
                (stats.packetsLost / (stats.packetsReceived + stats.packetsLost)) * 100 : 0,
            jitter: stats.jitter * 1000,
            bitrate: this._calculateBitrate(stats),
            healthData
        });
    }

    /**
     * 计算比特率
     * @private
     */
    _calculateBitrate(stats) {
        const now = Date.now();
        
        if (!this.lastBitrateCheck) {
            this.lastBitrateCheck = {
                timestamp: now,
                bytesReceived: stats.bytesReceived
            };
            return 0;
        }
        
        const timeDiff = (now - this.lastBitrateCheck.timestamp) / 1000; // 转换为秒
        const bytesDiff = stats.bytesReceived - this.lastBitrateCheck.bytesReceived;
        
        if (timeDiff > 0) {
            const bitrate = (bytesDiff * 8) / timeDiff; // 转换为比特每秒
            
            // 更新检查点
            this.lastBitrateCheck = {
                timestamp: now,
                bytesReceived: stats.bytesReceived
            };
            
            return bitrate;
        }
        
        return 0;
    }

    /**
     * 清理连接
     */
    async cleanup() {
        Logger.info('WebRTCManager: 清理WebRTC连接');
        
        // 停止统计收集
        this._stopStatsCollection();
        
        // 停止健康监控
        this._stopConnectionHealthMonitoring();
        
        // 清理媒体流
        this._cleanupMediaStreams();
        
        // 关闭PeerConnection
        if (this.pc) {
            try {
                // 关闭所有transceiver
                if (this.pc.getTransceivers) {
                    this.pc.getTransceivers().forEach(transceiver => {
                        if (transceiver.stop) {
                            transceiver.stop();
                        }
                    });
                }
                
                // 关闭连接
                this.pc.close();
                Logger.info('WebRTCManager: PeerConnection已关闭');
                
            } catch (error) {
                Logger.error('WebRTCManager: 关闭PeerConnection时出错', error);
            }
            
            this.pc = null;
        }
        
        // 重置状态
        this.connectionState = 'closed';
        this.iceConnectionState = 'closed';
        this.signalingState = 'closed';
        this.remoteStream = null;
        
        // 记录断开时间
        this.connectionStats.disconnectTime = Date.now();
        
        this.eventBus?.emit('webrtc:cleaned-up');
    }

    /**
     * 获取ICE服务器配置
     * @private
     */
    async _fetchIceServers() {
        try {
            Logger.info('WebRTCManager: 正在获取ICE服务器配置...');
            const response = await fetch('/api/webrtc/config');
            if (response.ok) {
                const config = await response.json();
                Logger.info('WebRTCManager: 收到服务器配置', config);
                if (config.ice_servers && config.ice_servers.length > 0) {
                    this.iceServers = config.ice_servers;
                    Logger.info('WebRTCManager: 使用服务器ICE配置', this.iceServers);
                } else {
                    Logger.warn('WebRTCManager: 服务器配置中没有ICE服务器，使用默认配置');
                }
            } else {
                Logger.warn('WebRTCManager: 获取配置失败，HTTP状态:', response.status);
            }
        } catch (error) {
            Logger.warn('WebRTCManager: 获取ICE配置失败，使用默认配置', error);
        }
    }

    /**
     * 创建PeerConnection
     * @private
     */
    _createPeerConnection() {
        const config = {
            iceServers: this.iceServers,
            iceTransportPolicy: this.config?.get('webrtc.iceTransportPolicy', 'all'),
            bundlePolicy: this.config?.get('webrtc.bundlePolicy', 'balanced'),
            rtcpMuxPolicy: this.config?.get('webrtc.rtcpMuxPolicy', 'require')
        };
        
        Logger.info('WebRTCManager: 创建PeerConnection', config);
        
        this.pc = new RTCPeerConnection(config);
        
        // 设置事件处理器
        this._setupEventHandlers();
        
        // 记录连接时间
        this.connectionStats.connectTime = Date.now();
        this.connectionStats.totalConnections++;
        
        Logger.info('WebRTCManager: PeerConnection创建成功');
    }

    /**
     * 设置事件处理器
     * @private
     */
    _setupEventHandlers() {
        // 媒体轨道事件
        this.pc.ontrack = (event) => {
            Logger.info(`WebRTCManager: 收到${event.track.kind}轨道`);
            
            const stream = event.streams[0];
            this.remoteStream = stream;
            
            if (event.track.kind === 'video' && this.videoElement) {
                // 确保视频元素正确设置
                this.videoElement.srcObject = stream;
                this.videoElement.autoplay = true;
                this.videoElement.playsInline = true;
                this.videoElement.muted = true; // 避免自动播放策略阻止
                
                this._handleVideoTrack();
            } else if (event.track.kind === 'audio' && this.audioElement) {
                this.audioElement.srcObject = stream;
                this._handleAudioTrack();
            }
            
            this.eventBus?.emit('webrtc:track-received', {
                kind: event.track.kind,
                stream,
                track: event.track
            });
        };

        // ICE候选事件
        this.pc.onicecandidate = (event) => {
            if (event.candidate) {
                const candidateType = this._getIceCandidateType(event.candidate.candidate);
                Logger.debug(`WebRTCManager: 生成ICE候选 (${candidateType})`);
                
                this.iceStats.sent++;
                this.iceStats.types[candidateType]++;
                
                this.eventBus?.emit('webrtc:ice-candidate-generated', {
                    candidate: event.candidate,
                    type: candidateType
                });
            } else {
                Logger.info('WebRTCManager: ICE候选收集完成');
                this.eventBus?.emit('webrtc:ice-gathering-complete', {
                    stats: { ...this.iceStats }
                });
            }
        };

        // 连接状态事件
        this.pc.onconnectionstatechange = () => {
            this.connectionState = this.pc.connectionState;
            Logger.info(`WebRTCManager: 连接状态变化 - ${this.connectionState}`);
            
            this.eventBus?.emit('webrtc:connection-state-change', {
                state: this.connectionState
            });
            
            if (this.connectionState === 'connected') {
                this.eventBus?.emit('webrtc:connected');
            } else if (this.connectionState === 'disconnected') {
                this.eventBus?.emit('webrtc:disconnected');
            } else if (this.connectionState === 'failed') {
                this.eventBus?.emit('webrtc:connection-failed');
            }
        };

        // ICE连接状态事件
        this.pc.oniceconnectionstatechange = () => {
            this.iceConnectionState = this.pc.iceConnectionState;
            Logger.info(`WebRTCManager: ICE连接状态变化 - ${this.iceConnectionState}`);
            
            this.eventBus?.emit('webrtc:ice-connection-state-change', {
                state: this.iceConnectionState
            });
        };

        // 信令状态事件
        this.pc.onsignalingstatechange = () => {
            this.signalingState = this.pc.signalingState;
            Logger.debug(`WebRTCManager: 信令状态变化 - ${this.signalingState}`);
            
            this.eventBus?.emit('webrtc:signaling-state-change', {
                state: this.signalingState
            });
        };

        // ICE收集状态事件
        this.pc.onicegatheringstatechange = () => {
            Logger.debug(`WebRTCManager: ICE收集状态变化 - ${this.pc.iceGatheringState}`);
            
            this.eventBus?.emit('webrtc:ice-gathering-state-change', {
                state: this.pc.iceGatheringState
            });
        };
    }

    /**
     * 处理视频轨道
     * @private
     */
    _handleVideoTrack() {
        if (!this.videoElement) return;
        
        // 强制播放机制
        this._ensureVideoPlayback();
        
        // 监听视频事件
        this.videoElement.addEventListener('loadedmetadata', () => {
            Logger.info(`WebRTCManager: 视频分辨率 ${this.videoElement.videoWidth}x${this.videoElement.videoHeight}`);
            this.eventBus?.emit('webrtc:video-metadata-loaded', {
                width: this.videoElement.videoWidth,
                height: this.videoElement.videoHeight
            });
            
            // 元数据加载后再次尝试播放
            this._ensureVideoPlayback();
        });
        
        this.videoElement.addEventListener('play', () => {
            Logger.info('WebRTCManager: 视频开始播放');
            this.eventBus?.emit('webrtc:video-playing');
        });
        
        this.videoElement.addEventListener('pause', () => {
            Logger.warn('WebRTCManager: 视频暂停，尝试恢复播放');
            this._ensureVideoPlayback();
        });
        
        this.videoElement.addEventListener('error', (event) => {
            Logger.error('WebRTCManager: 视频播放错误', event.target.error);
            this.eventBus?.emit('webrtc:video-error', { error: event.target.error });
            
            // 错误后尝试重新播放
            setTimeout(() => {
                this._ensureVideoPlayback();
            }, 1000);
        });
        
        this.videoElement.addEventListener('ended', () => {
            Logger.warn('WebRTCManager: 视频流结束');
            this.eventBus?.emit('webrtc:video-ended');
        });
    }

    /**
     * 确保视频播放的强制机制
     * @private
     */
    _ensureVideoPlayback() {
        if (!this.videoElement || !this.videoElement.srcObject) {
            return;
        }

        const playVideo = async () => {
            try {
                // 确保视频元素属性正确设置
                this.videoElement.autoplay = true;
                this.videoElement.playsInline = true;
                this.videoElement.muted = true;
                
                // 尝试播放
                await this.videoElement.play();
                Logger.info('WebRTCManager: 视频播放成功');
                
            } catch (error) {
                Logger.warn('WebRTCManager: 视频自动播放失败，尝试用户交互播放', error);
                
                // 如果自动播放失败，触发事件让UI显示播放按钮
                this.eventBus?.emit('webrtc:video-autoplay-failed', { 
                    error,
                    needsUserInteraction: true 
                });
                
                // 尝试点击播放（某些情况下可能有效）
                setTimeout(() => {
                    if (this.videoElement.paused) {
                        this.videoElement.play().catch(err => {
                            Logger.debug('WebRTCManager: 延迟播放也失败', err);
                        });
                    }
                }, 500);
            }
        };

        // 立即尝试播放
        playVideo();
        
        // 如果视频还没有加载完成，等待加载完成后再播放
        if (this.videoElement.readyState < 2) {
            this.videoElement.addEventListener('canplay', playVideo, { once: true });
        }
    }

    /**
     * 处理音频轨道
     * @private
     */
    _handleAudioTrack() {
        if (!this.audioElement) return;
        this.audioElement.muted = true;
        
        // 尝试自动播放
        this.audioElement.play().catch(error => {
            Logger.warn('WebRTCManager: 音频自动播放失败', error);
            this.eventBus?.emit('webrtc:audio-autoplay-failed', { error });
        });
        
        // 监听音频事件
        this.audioElement.addEventListener('play', () => {
            Logger.info('WebRTCManager: 音频开始播放');
            this.eventBus?.emit('webrtc:audio-playing');
        });
        
        this.audioElement.addEventListener('error', (event) => {
            Logger.error('WebRTCManager: 音频播放错误', event.target.error);
            this.eventBus?.emit('webrtc:audio-error', { error: event.target.error });
        });
    }

    /**
     * 清理媒体流
     * @private
     */
    _cleanupMediaStreams() {
        if (this.videoElement) {
            this.videoElement.srcObject = null;
        }
        
        if (this.audioElement) {
            this.audioElement.srcObject = null;
        }
        
        if (this.remoteStream) {
            this.remoteStream.getTracks().forEach(track => {
                track.stop();
            });
            this.remoteStream = null;
        }
    }

    /**
     * 获取ICE候选类型
     * @private
     */
    _getIceCandidateType(candidateString) {
        if (candidateString.includes('typ host')) return 'host';
        if (candidateString.includes('typ srflx')) return 'srflx';
        if (candidateString.includes('typ prflx')) return 'prflx';
        if (candidateString.includes('typ relay')) return 'relay';
        return 'unknown';
    }

    /**
     * 启动统计收集
     * @private
     */
    _startStatsCollection() {
        if (this.statsTimer) {
            clearInterval(this.statsTimer);
        }
        
        this.statsTimer = setInterval(async () => {
            try {
                const stats = await this.getStats();
                if (stats) {
                    this.eventBus?.emit('webrtc:stats-updated', stats);
                }
            } catch (error) {
                Logger.error('WebRTCManager: 统计收集失败', error);
            }
        }, this.statsInterval);
    }

    /**
     * 停止统计收集
     * @private
     */
    _stopStatsCollection() {
        if (this.statsTimer) {
            clearInterval(this.statsTimer);
            this.statsTimer = null;
        }
    }

    /**
     * 解析WebRTC统计
     * @private
     */
    _parseStats(stats) {
        const parsedStats = {
            bytesReceived: 0,
            bytesSent: 0,
            packetsReceived: 0,
            packetsSent: 0,
            packetsLost: 0,
            jitter: 0,
            rtt: 0,
            timestamp: Date.now()
        };
        
        stats.forEach(report => {
            if (report.type === 'inbound-rtp') {
                parsedStats.bytesReceived += report.bytesReceived || 0;
                parsedStats.packetsReceived += report.packetsReceived || 0;
                parsedStats.packetsLost += report.packetsLost || 0;
                parsedStats.jitter = Math.max(parsedStats.jitter, report.jitter || 0);
            } else if (report.type === 'outbound-rtp') {
                parsedStats.bytesSent += report.bytesSent || 0;
                parsedStats.packetsSent += report.packetsSent || 0;
            } else if (report.type === 'candidate-pair' && report.state === 'succeeded') {
                parsedStats.rtt = Math.max(parsedStats.rtt, report.currentRoundTripTime || 0);
            }
        });
        
        return parsedStats;
    }
}

// 导出WebRTCManager类
if (typeof module !== 'undefined' && module.exports) {
    module.exports = WebRTCManager;
} else if (typeof window !== 'undefined') {
    window.WebRTCManager = WebRTCManager;
}
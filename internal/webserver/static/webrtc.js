/**
 * 优化的 WebRTC 管理器 - 基于 selkies WebRTCDemo 实现
 * 移除调试代码，优化性能，简化实现
 */

// WebRTC Adapter 验证 - 优化版本
let _adapterVerified = null;
function verifyWebRTCAdapter() {
  if (_adapterVerified !== null) return _adapterVerified;
  
  if (typeof adapter !== "undefined") {
    _adapterVerified = true;
    return true;
  } else {
    _adapterVerified = false;
    return false;
  }
}

// 立即验证 adapter
verifyWebRTCAdapter();

/**
 * 优化的 WebRTC 管理器
 */
class WebRTCManager {
  constructor(signalingManager, videoElement, peerId, options = {}) {
    // 验证必需参数
    if (!signalingManager) {
      throw new Error('SignalingManager is required');
    }
    if (!videoElement) {
      throw new Error('Video element is required');
    }

    // 核心参数
    this.signaling = signalingManager;
    this.element = videoElement;
    this.peer_id = peerId || 1;

    // 可选参数（向后兼容）
    this.eventBus = options.eventBus || null;
    this.config = options.config || null;
    this.logger = window.EnhancedLogger || console;

    // WebRTC 核心组件
    this.peerConnection = null;
    this.rtcPeerConfig = {
      lifetimeDuration: "86400s",
      iceServers: [{ urls: ["stun:stun.l.google.com:19302"] }],
      blockStatus: "NOT_BLOCKED",
      iceTransportPolicy: "all",
    };

    // 连接状态
    this.connectionState = "disconnected";
    this._connected = false;
    this._send_channel = null;
    this.streams = null;

    // 回调函数
    this.onstatus = null;
    this.ondebug = null;
    this.onerror = null;
    this.onconnectionstatechange = null;
    this.ondatachannelopen = null;
    this.ondatachannelclose = null;
    this.onplaystreamrequired = null;

    // 初始化
    this._loadConfig();
    this._setupEventCallbacks();
    this._setupSignalingIntegration();
    this._setupVideoElementIntegration();
    
    // 自动调用 setMediaElements 方法
    this.setMediaElements(this.element, options.audioElement || null);
  }

  /**
   * 加载配置 - 优化版本
   */
  _loadConfig() {
    const defaultIceServers = [{ urls: ["stun:stun.ali.wodcloud.com:3478"] }];

    if (this.config) {
      const webrtcConfig = this.config.get("webrtc", {});
      this.rtcPeerConfig.iceServers = webrtcConfig.iceServers || defaultIceServers;
    } else {
      this.rtcPeerConfig.iceServers = defaultIceServers;
    }
  }

  /**
   * 设置事件回调 - 优化版本
   */
  _setupEventCallbacks() {
    this.onstatus = (message) => {
      this.eventBus?.emit("webrtc:status", { message });
    };

    this.ondebug = (message) => {
      this.eventBus?.emit("webrtc:debug", { message });
    };

    this.onerror = (message) => {
      this.eventBus?.emit("webrtc:error", { error: message });
    };

    this.onconnectionstatechange = (state) => {
      this.connectionState = state;
      this.eventBus?.emit("webrtc:connection-state-change", { state });
    };

    this.ondatachannelopen = () => {
      this.eventBus?.emit("webrtc:datachannel-open");
    };

    this.ondatachannelclose = () => {
      this.eventBus?.emit("webrtc:datachannel-close");
    };

    this.onplaystreamrequired = () => {
      this.eventBus?.emit("webrtc:video-autoplay-failed", {
        needsUserInteraction: true,
      });
    };
  }

  /**
   * 设置信令管理器自动集成
   */
  _setupSignalingIntegration() {
    if (this.signaling) {
      // 自动绑定 SDP 处理回调
      this.signaling.onsdp = this._onSDP.bind(this);
      
      // 自动绑定 ICE 候选处理回调
      this.signaling.onice = this._onSignalingICE.bind(this);
      
      // 自动设置对等ID
      this.signaling.peer_id = this.peer_id;
    }
  }

  /**
   * 设置视频元素自动配置
   */
  _setupVideoElementIntegration() {
    if (this.element) {
      // 存储视频元素引用
      this.videoElement = this.element;
      
      // 设置视频元素基本属性
      this.element.autoplay = true;
      this.element.muted = true;
      this.element.playsInline = true;
    }
  }

  /**
   * 初始化 WebRTC 管理器 - 优化版本
   */
  async initialize() {
    if (!verifyWebRTCAdapter()) {
      throw new Error("WebRTC Adapter 未正确加载");
    }

    await this._fetchServerConfig();
    this.eventBus?.emit("webrtc:initialized", {
      iceServers: this.rtcPeerConfig.iceServers,
      config: this.rtcPeerConfig,
    });
  }

  /**
   * 建立 WebRTC 连接 - 优化版本
   */
  connect() {
    this._setStatus("开始建立 WebRTC 连接");

    this.peerConnection = new RTCPeerConnection(this.rtcPeerConfig);

    // 绑定事件处理器
    this.peerConnection.ontrack = this._ontrack.bind(this);
    this.peerConnection.onicecandidate = this._onPeerICE.bind(this);
    this.peerConnection.ondatachannel = this._onPeerDataChannel.bind(this);
    this.peerConnection.onconnectionstatechange = () => {
      this._handleConnectionStateChange(this.peerConnection.connectionState);
      this._setConnectionState(this.peerConnection.connectionState);
    };

    if (this.signaling) {
      this.signaling.peer_id = this.peer_id;
      this.signaling.connect();
    }

    this.connectionState = "connecting";
    this._connected = false;
  }

  /**
   * 重置 WebRTC 连接 - 优化版本
   */
  reset() {
    const signalState = this.peerConnection?.signalingState || "stable";

    if (this._send_channel?.readyState === "open") {
      this._send_channel.close();
    }

    if (this.peerConnection) {
      this.peerConnection.close();
      this.peerConnection = null;
    }

    this._connected = false;
    this.connectionState = "disconnected";
    this._send_channel = null;
    this.streams = null;

    // 根据信令状态决定重连延迟
    const delay = signalState !== "stable" ? 3000 : 0;
    setTimeout(() => this.connect(), delay);
  }

  /**
   * 从服务器获取配置 - 优化版本
   */
  async _fetchServerConfig() {
    if (!this.config?.fetchWebRTCConfig) return;

    try {
      const serverConfig = await this.config.fetchWebRTCConfig(true);
      if (serverConfig?.iceServers?.length > 0) {
        this.rtcPeerConfig.iceServers = serverConfig.iceServers;
        this.eventBus?.emit("webrtc:config-updated", {
          source: "server",
          iceServers: this.rtcPeerConfig.iceServers,
        });
      }
    } catch (error) {
      // 静默处理配置获取失败
    }
  }

  /**
   * 处理 SDP - 优化版本
   */
  _onSDP(sdp) {
    if (sdp.type !== "offer") {
      this._setError("received SDP was not type offer.");
      return;
    }

    this.peerConnection
      .setRemoteDescription(sdp)
      .then(() => this.peerConnection.createAnswer())
      .then((local_sdp) => {
        // SDP 优化
        this._optimizeSDP(local_sdp);
        return this.peerConnection.setLocalDescription(local_sdp);
      })
      .then(() => {
        if (this.signaling?.sendSDP) {
          this.signaling.sendSDP(this.peerConnection.localDescription);
        }
      })
      .catch((error) => {
        this._setError("Error processing SDP: " + error.message);
      });
  }

  /**
   * SDP 优化 - 合并优化逻辑
   */
  _optimizeSDP(sdp) {
    let sdpString = sdp.sdp;

    // H.264 优化
    if (!/[^-]sps-pps-idr-in-keyframe=1[^\d]/gm.test(sdpString) &&
        /[^-]packetization-mode=/gm.test(sdpString)) {
      if (/[^-]sps-pps-idr-in-keyframe=\d+/gm.test(sdpString)) {
        sdpString = sdpString.replace(/sps-pps-idr-in-keyframe=\d+/gm, "sps-pps-idr-in-keyframe=1");
      } else {
        sdpString = sdpString.replace("packetization-mode=", "sps-pps-idr-in-keyframe=1;packetization-mode=");
      }
    }

    // 音频优化
    if (sdpString.indexOf("multiopus") === -1) {
      // 立体声优化
      if (!/[^-]stereo=1[^\d]/gm.test(sdpString) && /[^-]useinbandfec=/gm.test(sdpString)) {
        if (/[^-]stereo=\d+/gm.test(sdpString)) {
          sdpString = sdpString.replace(/stereo=\d+/gm, "stereo=1");
        } else {
          sdpString = sdpString.replace("useinbandfec=", "stereo=1;useinbandfec=");
        }
      }

      // 低延迟优化
      if (!/[^-]minptime=10[^\d]/gm.test(sdpString) && /[^-]useinbandfec=/gm.test(sdpString)) {
        if (/[^-]minptime=\d+/gm.test(sdpString)) {
          sdpString = sdpString.replace(/minptime=\d+/gm, "minptime=10");
        } else {
          sdpString = sdpString.replace("useinbandfec=", "minptime=10;useinbandfec=");
        }
      }
    }

    sdp.sdp = sdpString;
  }

  /**
   * 处理 ICE 候选 - 优化版本
   */
  _onSignalingICE(icecandidate) {
    if (!this.peerConnection) {
      this._setError("Cannot add ICE candidate: no peer connection");
      return;
    }

    this.peerConnection.addIceCandidate(icecandidate).catch((error) => {
      this._setError("Error adding ICE candidate: " + error.message);
    });
  }

  /**
   * 处理 PeerConnection ICE 候选 - 优化版本
   */
  _onPeerICE(event) {
    if (event.candidate === null) return;

    if (this.signaling?.sendICE) {
      this.signaling.sendICE(event.candidate);
    }
  }

  /**
   * 处理连接状态变化 - 优化版本
   */
  _handleConnectionStateChange(state) {
    switch (state) {
      case "connected":
        this._connected = true;
        break;
      case "disconnected":
        this._setError("Peer connection disconnected");
        if (this._send_channel?.readyState === "open") {
          this._send_channel.close();
        }
        if (this.element) {
          this.element.load();
        }
        break;
      case "failed":
        this._setError("Peer connection failed");
        if (this.element) {
          this.element.load();
        }
        break;
    }
  }

  /**
   * 处理接收到的媒体轨道 - 优化版本
   */
  _ontrack(event) {
    if (!this.streams) this.streams = [];
    this.streams.push([event.track.kind, event.streams]);

    if ((event.track.kind === "video" || event.track.kind === "audio") && this.element) {
      this.element.srcObject = event.streams[0];
      this.playStream();
    }

    this.eventBus?.emit("webrtc:track-received", {
      kind: event.track.kind,
      streams: event.streams,
      track: event.track,
    });
  }

  /**
   * 播放媒体流 - 优化版本
   */
  playStream() {
    if (!this.element) return;

    this.element.load();
    const playPromise = this.element.play();
    
    if (playPromise !== undefined) {
      playPromise
        .then(() => {
          this.eventBus?.emit("webrtc:video-playing");
        })
        .catch(() => {
          if (this.onplaystreamrequired) {
            this.onplaystreamrequired();
          }
        });
    }
  }

  /**
   * 处理数据通道 - 优化版本
   */
  _onPeerDataChannel(event) {
    this._send_channel = event.channel;
    this._send_channel.onmessage = this._onPeerDataChannelMessage.bind(this);
    this._send_channel.onopen = () => {
      if (this.ondatachannelopen) this.ondatachannelopen();
    };
    this._send_channel.onclose = () => {
      if (this.ondatachannelclose) this.ondatachannelclose();
    };
  }

  /**
   * 处理数据通道消息 - 优化版本
   */
  _onPeerDataChannelMessage(event) {
    let msg;
    try {
      msg = JSON.parse(event.data);
    } catch (e) {
      this._setError("Failed to parse data channel message");
      return;
    }

    // 优化的消息处理
    const messageHandlers = {
      pipeline: () => this._setStatus(msg.data.status),
      gpu_stats: () => this.eventBus?.emit("webrtc:gpu-stats", msg.data),
      clipboard: () => {
        if (msg.data?.content) {
          const text = this._base64ToString(msg.data.content);
          this.eventBus?.emit("webrtc:clipboard-content", { text });
        }
      },
      cursor: () => {
        if (msg.data) {
          this.eventBus?.emit("webrtc:cursor-change", msg.data);
        }
      },
      system: () => {
        if (msg.data?.action) {
          this.eventBus?.emit("webrtc:system-action", { action: msg.data.action });
        }
      },
      ping: () => {
        this.sendDataChannelMessage("pong," + Date.now() / 1000);
      },
      system_stats: () => this.eventBus?.emit("webrtc:system-stats", msg.data),
      latency_measurement: () => {
        this.eventBus?.emit("webrtc:latency-measurement", { latency: msg.data.latency_ms });
      }
    };

    const handler = messageHandlers[msg.type];
    if (handler) {
      handler();
    } else {
      this._setError("Unhandled message received: " + msg.type);
    }
  }

  /**
   * 发送数据通道消息 - 优化版本
   */
  sendDataChannelMessage(message) {
    if (this._send_channel?.readyState === "open") {
      this._send_channel.send(message);
      return true;
    }
    return false;
  }

  /**
   * Base64 转字符串
   */
  _base64ToString(base64) {
    try {
      return atob(base64);
    } catch (e) {
      return "";
    }
  }

  /**
   * 获取连接统计信息 - 优化版本
   */
  async getConnectionStats() {
    if (!this.peerConnection) return null;

    try {
      const stats = await this.peerConnection.getStats();
      const result = {
        general: { bytesReceived: 0, bytesSent: 0, currentRoundTripTime: null },
        video: { bytesReceived: 0, frameWidth: 0, frameHeight: 0, framesPerSecond: 0 },
        audio: { bytesReceived: 0, packetsReceived: 0, packetsLost: 0 },
        data: { bytesReceived: 0, bytesSent: 0 }
      };

      stats.forEach((report) => {
        if (report.type === "inbound-rtp") {
          if (report.kind === "video") {
            result.video = {
              bytesReceived: report.bytesReceived || 0,
              frameWidth: report.frameWidth || 0,
              frameHeight: report.frameHeight || 0,
              framesPerSecond: report.framesPerSecond || 0
            };
          } else if (report.kind === "audio") {
            result.audio = {
              bytesReceived: report.bytesReceived || 0,
              packetsReceived: report.packetsReceived || 0,
              packetsLost: report.packetsLost || 0
            };
          }
        } else if (report.type === "candidate-pair" && report.selected) {
          result.general.currentRoundTripTime = report.currentRoundTripTime;
        } else if (report.type === "data-channel") {
          result.data = {
            bytesReceived: report.bytesReceived || 0,
            bytesSent: report.bytesSent || 0
          };
        }
      });

      return result;
    } catch (error) {
      return null;
    }
  }

  /**
   * 设置媒体元素
   */
  setMediaElements(videoElement, audioElement) {
    this.element = videoElement;
    this.videoElement = videoElement;
    this.audioElement = audioElement;
    
    this.eventBus?.emit("webrtc:media-elements-set", {
      hasVideo: !!videoElement,
      hasAudio: !!audioElement,
    });
  }

  /**
   * 设置信令管理器
   */
  setSignalingManager(signalingManager) {
    this.signaling = signalingManager;
    if (this.signaling) {
      this.signaling.onsdp = this._onSDP.bind(this);
      this.signaling.onice = this._onSignalingICE.bind(this);
    }
  }

  /**
   * 获取连接状态
   */
  getConnectionState() {
    return {
      connectionState: this.connectionState,
      connected: this._connected,
      peerConnection: this.peerConnection?.connectionState || null,
      iceConnectionState: this.peerConnection?.iceConnectionState || null,
      signalingState: this.peerConnection?.signalingState || null,
    };
  }

  // 内部方法
  _setStatus(message) {
    if (this.onstatus) this.onstatus(message);
  }

  _setDebug(message) {
    if (this.ondebug) this.ondebug(message);
  }

  _setError(message) {
    if (this.onerror) this.onerror(message);
  }

  _setConnectionState(state) {
    if (this.onconnectionstatechange) this.onconnectionstatechange(state);
  }
}

// 导出类
if (typeof module !== "undefined" && module.exports) {
  module.exports = { WebRTCManager, verifyWebRTCAdapter };
} else if (typeof window !== "undefined") {
  window.WebRTCManager = WebRTCManager;
  window.verifyWebRTCAdapter = verifyWebRTCAdapter;
}
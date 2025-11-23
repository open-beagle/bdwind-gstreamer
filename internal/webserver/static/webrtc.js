/**
 * 优化的 WebRTC 管理器
 */
class WebRTCManager {
  constructor(signalingClient, mediaElement, peerId, options = {}) {
    // 验证必需参数
    if (!signalingClient) {
      throw new Error("SignalingClient is required");
    }
    if (!mediaElement) {
      throw new Error("Video element is required");
    }

    // 核心参数
    this.signaling = signalingClient;
    this.element = mediaElement;
    this.peer_id = peerId || 1;

    // 可选参数
    this.eventBus = options.eventBus || null;
    this.config = options.config || null;
    this.logger = window.EnhancedLogger || console;

    // WebRTC 核心组件
    this.peerConnection = null;
    this.rtcPeerConfig = {
      lifetimeDuration: "86400s",
      iceServers: [],
      blockStatus: "NOT_BLOCKED",
      iceTransportPolicy: "all",
    };

    // 连接状态
    this.connectionState = "disconnected";
    this._connected = false;
    this._send_channel = null;
    this.streams = null;
    this._processingOffer = false;  // 防止重复处理 offer

    // ICE 候选筛选配置
    this.iceFilterConfig = {
      preferredTypes: ['srflx'],  // 优先使用的候选类型：srflx (Server Reflexive)
      allowedTypes: ['srflx', 'relay', 'host'],  // 允许的候选类型
      blockIPv6LinkLocal: true,  // 阻止 IPv6 link-local 地址
      strictMode: false  // 严格模式：true=只使用preferredTypes，false=优先使用但允许其他
    };

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
      this.rtcPeerConfig.iceServers =
        webrtcConfig.iceServers || defaultIceServers;
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

      // 设置对等ID
      this.signaling.peerId = this.peer_id;
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

    // 先不创建 PeerConnection，等收到服务器的 ICE 配置后再创建
    // 这样可以确保使用服务器提供的 TURN 服务器配置

    if (this.signaling) {
      // 设置对等ID
      this.signaling.peerId = this.peer_id;

      // 连接信令服务器
      this.signaling.connect();
    }

    this.connectionState = "connecting";
    this._connected = false;
  }

  /**
   * 创建 PeerConnection（在收到服务器配置后调用）
   */
  _createPeerConnection() {
    if (this.peerConnection) {
      console.log(`⚠️ [WebRTC] PeerConnection 已存在，跳过创建`);
      return;
    }

    // 从 SignalingClient 获取 ICE 服务器配置
    if (this.signaling && this.signaling.getICEServers) {
      const serverICEServers = this.signaling.getICEServers();
      if (serverICEServers && serverICEServers.length > 0) {
        console.log(`🔧 [WebRTC] 使用服务器提供的 ${serverICEServers.length} 个 ICE 服务器:`, serverICEServers);
        this.rtcPeerConfig.iceServers = serverICEServers;
      } else {
        console.log(`⚠️ [WebRTC] 服务器未提供 ICE 配置，使用默认配置:`, this.rtcPeerConfig.iceServers);
      }
    }

    console.log(`🔧 [WebRTC] 创建 PeerConnection，ICE 配置:`, this.rtcPeerConfig);
    this.peerConnection = new RTCPeerConnection(this.rtcPeerConfig);

    // 绑定事件处理器
    this.peerConnection.ontrack = this._ontrack.bind(this);
    this.peerConnection.onicecandidate = this._onPeerICE.bind(this);
    this.peerConnection.ondatachannel = this._onPeerDataChannel.bind(this);
    this.peerConnection.onconnectionstatechange = () => {
      this._handleConnectionStateChange(this.peerConnection.connectionState);
      this._setConnectionState(this.peerConnection.connectionState);
    };

    // 监听 ICE 连接状态变化，查看哪些候选对正在尝试
    this.peerConnection.oniceconnectionstatechange = () => {
      const iceState = this.peerConnection.iceConnectionState;
      console.log(`🧊 [ICE] ICE 连接状态变化: ${iceState}`);
      this._setStatus(`🧊 ICE 连接状态: ${iceState}`);
      this._setDebug(`🧊 ICE 连接状态: ${iceState}`);
      
      // 当 ICE 状态变化时，打印当前选中的候选对
      if (iceState === 'connected' || iceState === 'completed') {
        this._logSelectedCandidatePair();
      } else if (iceState === 'failed') {
        this._setStatus(`❌ ICE 连接失败，所有候选对都无法连接`);
        this._logAllCandidatePairs();
      } else if (iceState === 'checking') {
        this._setStatus(`🔍 ICE 正在检查候选对...`);
      }
    };

    this._setStatus("PeerConnection 已创建");
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
    // 如果 PeerConnection 还未创建，先创建它
    if (!this.peerConnection) {
      console.log(`🔧 [WebRTC] 收到 SDP，但 PeerConnection 未创建，先创建 PeerConnection`);
      this._createPeerConnection();
    }

    if (!this.peerConnection) {
      this._setError("Cannot process SDP: failed to create peer connection");
      return;
    }

    // 检查当前信令状态
    const currentState = this.peerConnection.signalingState;
    this._setDebug(`Processing SDP offer, current signaling state: ${currentState}`);

    // 严格的状态检查：只在 stable 状态下处理新的 offer
    if (currentState !== 'stable') {
      this._setDebug(`⚠️ Ignoring duplicate offer in state: ${currentState}`);
      return;
    }

    // 标记正在处理 SDP，防止重复处理
    if (this._processingOffer) {
      this._setDebug(`⚠️ Already processing an offer, ignoring duplicate`);
      return;
    }
    this._processingOffer = true;

    this.peerConnection
      .setRemoteDescription(sdp)
      .then(() => {
        this._setDebug(`Remote description set, creating answer...`);
        return this.peerConnection.createAnswer();
      })
      .then((local_sdp) => {
        // SDP 优化
        this._optimizeSDP(local_sdp);
        return this.peerConnection.setLocalDescription(local_sdp);
      })
      .then(() => {
        this._setDebug(`Local description set, sending answer to server`);
        // 验证并发送 SDP - 兼容新旧接口
        this._sendSDP(this.peerConnection.localDescription);
        this._processingOffer = false;
      })
      .catch((error) => {
        this._setError("Error processing SDP: " + error.message);
        this._processingOffer = false;
      });
  }

  /**
   * 发送 SDP - 兼容新旧接口
   */
  _sendSDP(sdp) {
    if (!this.signaling) {
      this._setError("No signaling manager available");
      return;
    }

    if (typeof this.signaling.sendSDP !== "function") {
      this._setError("Signaling manager sendSDP method not available");
      return;
    }

    try {
      this.signaling.sendSDP(sdp);
      this._setStatus("SDP answer sent to signaling server");
    } catch (error) {
      this._setError("Failed to send SDP: " + error.message);
    }
  }

  /**
   * SDP 优化 - 合并优化逻辑
   */
  _optimizeSDP(sdp) {
    let sdpString = sdp.sdp;

    // H.264 优化
    if (
      !/[^-]sps-pps-idr-in-keyframe=1[^\d]/gm.test(sdpString) &&
      /[^-]packetization-mode=/gm.test(sdpString)
    ) {
      if (/[^-]sps-pps-idr-in-keyframe=\d+/gm.test(sdpString)) {
        sdpString = sdpString.replace(
          /sps-pps-idr-in-keyframe=\d+/gm,
          "sps-pps-idr-in-keyframe=1"
        );
      } else {
        sdpString = sdpString.replace(
          "packetization-mode=",
          "sps-pps-idr-in-keyframe=1;packetization-mode="
        );
      }
    }

    // 音频优化
    if (sdpString.indexOf("multiopus") === -1) {
      // 立体声优化
      if (
        !/[^-]stereo=1[^\d]/gm.test(sdpString) &&
        /[^-]useinbandfec=/gm.test(sdpString)
      ) {
        if (/[^-]stereo=\d+/gm.test(sdpString)) {
          sdpString = sdpString.replace(/stereo=\d+/gm, "stereo=1");
        } else {
          sdpString = sdpString.replace(
            "useinbandfec=",
            "stereo=1;useinbandfec="
          );
        }
      }

      // 低延迟优化
      if (
        !/[^-]minptime=10[^\d]/gm.test(sdpString) &&
        /[^-]useinbandfec=/gm.test(sdpString)
      ) {
        if (/[^-]minptime=\d+/gm.test(sdpString)) {
          sdpString = sdpString.replace(/minptime=\d+/gm, "minptime=10");
        } else {
          sdpString = sdpString.replace(
            "useinbandfec=",
            "minptime=10;useinbandfec="
          );
        }
      }
    }

    sdp.sdp = sdpString;
  }

  /**
   * 处理 ICE 候选 - 优化版本（带筛选和详细日志）
   */
  _onSignalingICE(icecandidate) {
    if (!this.peerConnection) {
      this._setError("Cannot add ICE candidate: no peer connection");
      return;
    }

    // 规范化候选格式
    // signaling.js 现在传递的是 { candidate: "...", sdpMid: "...", sdpMLineIndex: ... }
    let candidateInit = icecandidate;
    
    // 如果是旧格式的 RTCIceCandidate 对象，转换为 init 格式
    if (icecandidate.candidate && typeof icecandidate.toJSON === 'function') {
      candidateInit = icecandidate.toJSON();
    }

    // 验证 ICE 候选
    if (!candidateInit || typeof candidateInit.candidate !== 'string') {
      this._setError("Invalid ICE candidate received: " + JSON.stringify(icecandidate));
      return;
    }

    // 解析并记录候选信息
    const candidateInfo = this._parseICECandidate(candidateInit);
    this._logICECandidate('received', candidateInfo);

    // 筛选候选
    console.log(`🔍 [ICE] 准备筛选远程候选:`, candidateInfo);
    const filterResult = this._filterRemoteCandidate(candidateInfo);
    console.log(`🔍 [ICE] 筛选结果: ${filterResult ? '通过' : '拒绝'}`);
    
    if (!filterResult) {
      console.log(`⏭️ [ICE] 跳过远程候选（被筛选规则过滤）: ${candidateInfo.type} ${candidateInfo.address}`);
      this._setDebug(`⏭️ 跳过远程候选（被筛选规则过滤）: ${candidateInfo.type} ${candidateInfo.address}`);
      return;
    }

    console.log(`✅ [ICE] 添加远程候选到PeerConnection:`, candidateInfo.type, candidateInfo.address);
    this.peerConnection.addIceCandidate(candidateInit).catch((error) => {
      console.error(`❌ [ICE] 添加远程候选失败:`, error);
      this._setError("Error adding ICE candidate: " + error.message);
    });
  }

  /**
   * 处理 PeerConnection ICE 候选 - 优化版本（带筛选和详细日志）
   */
  _onPeerICE(event) {
    if (event.candidate === null) {
      this._setStatus("✅ 本地ICE候选收集完成");
      return;
    }

    const candidate = event.candidate;
    
    // 解析并记录候选信息
    const candidateInfo = this._parseICECandidate(candidate);
    this._logICECandidate('generated', candidateInfo);

    // 筛选候选
    console.log(`🔍 [ICE] 准备筛选本地候选:`, candidateInfo);
    const filterResult = this._filterLocalCandidate(candidateInfo);
    console.log(`🔍 [ICE] 筛选结果: ${filterResult ? '通过' : '拒绝'}`);
    
    if (!filterResult) {
      console.log(`⏭️ [ICE] 跳过本地候选（被筛选规则过滤）: ${candidateInfo.type} ${candidateInfo.address}`);
      this._setDebug(`⏭️ 跳过本地候选（被筛选规则过滤）: ${candidateInfo.type} ${candidateInfo.address}`);
      return;
    }

    // 发送 ICE 候选 - 兼容新旧接口
    console.log(`📤 [ICE] 发送本地候选到服务器:`, candidateInfo.type, candidateInfo.address);
    this._sendICE(candidate);
  }

  /**
   * 验证 ICE 候选
   */
  _validateICECandidate(candidate) {
    return (
      candidate &&
      typeof candidate === "object" &&
      (typeof candidate.candidate === "string" || candidate.candidate === null)
    );
  }

  /**
   * 解析 ICE 候选字符串
   */
  _parseICECandidate(candidate) {
    const candidateStr = candidate.candidate || '';
    
    // 解析候选字符串
    // 格式: candidate:foundation component protocol priority ip port typ type ...
    const parts = candidateStr.split(' ');
    
    const info = {
      raw: candidateStr,
      foundation: parts[0]?.replace('candidate:', '') || '',
      component: parts[1] || '',
      protocol: parts[2] || '',
      priority: parts[3] || '',
      address: parts[4] || '',
      port: parts[5] || '',
      type: '',
      relatedAddress: '',
      relatedPort: '',
      tcpType: '',
      generation: '',
      ufrag: '',
      networkCost: ''
    };

    // 解析类型和其他属性
    for (let i = 6; i < parts.length; i += 2) {
      const key = parts[i];
      const value = parts[i + 1];
      
      switch (key) {
        case 'typ':
          info.type = value;
          break;
        case 'raddr':
          info.relatedAddress = value;
          break;
        case 'rport':
          info.relatedPort = value;
          break;
        case 'tcptype':
          info.tcpType = value;
          break;
        case 'generation':
          info.generation = value;
          break;
        case 'ufrag':
          info.ufrag = value;
          break;
        case 'network-cost':
          info.networkCost = value;
          break;
      }
    }

    // 判断IP类型
    info.ipVersion = this._detectIPVersion(info.address);
    
    return info;
  }

  /**
   * 检测IP版本
   */
  _detectIPVersion(address) {
    if (!address) return 'unknown';
    
    // IPv6地址包含冒号
    if (address.includes(':')) {
      // 排除IPv4映射的IPv6地址 (::ffff:192.168.1.1)
      if (address.includes('.')) {
        return 'ipv4-mapped';
      }
      return 'ipv6';
    }
    
    // IPv4地址包含点
    if (address.includes('.')) {
      return 'ipv4';
    }
    
    return 'unknown';
  }

  /**
   * 记录 ICE 候选详细信息
   */
  _logICECandidate(direction, info) {
    const emoji = direction === 'generated' ? '📤' : '📥';
    const action = direction === 'generated' ? '生成本地' : '收到远程';
    
    this._setDebug(`${emoji} ${action} ICE 候选:`);
    this._setDebug(`   类型: ${info.type} (${info.ipVersion})`);
    this._setDebug(`   协议: ${info.protocol}`);
    this._setDebug(`   地址: ${info.address}:${info.port}`);
    this._setDebug(`   优先级: ${info.priority}`);
    
    if (info.relatedAddress) {
      this._setDebug(`   相关地址: ${info.relatedAddress}:${info.relatedPort}`);
    }
    
    if (info.tcpType) {
      this._setDebug(`   TCP类型: ${info.tcpType}`);
    }
    
    this._setDebug(`   完整候选: ${info.raw.substring(0, 100)}${info.raw.length > 100 ? '...' : ''}`);

    // 特别标记 srflx 候选，方便调试
    if (info.type === 'srflx') {
      console.log(`🌐 [SRFLX ${direction === 'generated' ? '本地' : '远程'}] ${info.address}:${info.port} (优先级: ${info.priority})`);
      this._setDebug(`🌐 [SRFLX ${direction === 'generated' ? '本地' : '远程'}] 这是通过 STUN 服务器获取的公网地址`);
    }
    
    // 特别标记 relay 候选
    if (info.type === 'relay') {
      console.log(`🔄 [RELAY ${direction === 'generated' ? '本地' : '远程'}] ${info.address}:${info.port} (优先级: ${info.priority})`);
      this._setDebug(`🔄 [RELAY ${direction === 'generated' ? '本地' : '远程'}] 这是通过 TURN 服务器中继的地址`);
    }
  }

  /**
   * 筛选本地 ICE 候选
   * 返回 true 表示保留，false 表示过滤掉
   */
  _filterLocalCandidate(info) {
    // 规则1: 过滤掉无效候选
    if (!info.address || !info.type) {
      this._setDebug(`   ❌ 筛选原因: 候选信息不完整`);
      return false;
    }

    // 规则2: 过滤IPv6 link-local地址 (fe80::)
    if (this.iceFilterConfig.blockIPv6LinkLocal && 
        info.ipVersion === 'ipv6' && 
        info.address.startsWith('fe80:')) {
      this._setDebug(`   ❌ 筛选原因: IPv6 link-local地址不适用于远程连接`);
      return false;
    }

    // 规则3: 类型筛选 - 优先使用 srflx
    const isPreferred = this.iceFilterConfig.preferredTypes.includes(info.type);
    const isAllowed = this.iceFilterConfig.allowedTypes.includes(info.type);

    if (this.iceFilterConfig.strictMode) {
      // 严格模式：只允许优先类型
      if (!isPreferred) {
        this._setDebug(`   ❌ 筛选原因: 严格模式下只接受 ${this.iceFilterConfig.preferredTypes.join(', ')} 类型`);
        return false;
      }
    } else {
      // 宽松模式：检查是否在允许列表中
      if (!isAllowed) {
        this._setDebug(`   ❌ 筛选原因: 候选���型 ${info.type} 不在允许列表中`);
        return false;
      }
    }

    if (isPreferred) {
      this._setDebug(`   ✅ 候选通过筛选（优先类型: ${info.type}），将发送到服务器`);
    } else {
      this._setDebug(`   ✅ 候选通过筛选（备用类型: ${info.type}），将发送到服务器`);
    }
    return true;
  }

  /**
   * 筛选远程 ICE 候选
   * 返回 true 表示保留，false 表示过滤掉
   */
  _filterRemoteCandidate(info) {
    // 规则1: 过滤掉无效候选
    if (!info.address || !info.type) {
      this._setDebug(`   ❌ 筛选原因: 候选信息不完整`);
      return false;
    }

    // 规则2: 过滤IPv6 link-local地址
    if (this.iceFilterConfig.blockIPv6LinkLocal && 
        info.ipVersion === 'ipv6' && 
        info.address.startsWith('fe80:')) {
      this._setDebug(`   ❌ 筛选原因: IPv6 link-local地址不适用于远程连接`);
      return false;
    }

    // 规则3: 类型筛选 - 优先使用 srflx
    const isPreferred = this.iceFilterConfig.preferredTypes.includes(info.type);
    const isAllowed = this.iceFilterConfig.allowedTypes.includes(info.type);

    if (this.iceFilterConfig.strictMode) {
      // 严格模式：只允许优先类型
      if (!isPreferred) {
        this._setDebug(`   ❌ 筛选原因: 严格模式下只接受 ${this.iceFilterConfig.preferredTypes.join(', ')} 类型`);
        return false;
      }
    } else {
      // 宽松模式：检查是否在允许列表中
      if (!isAllowed) {
        this._setDebug(`   ❌ 筛选原因: 候选类型 ${info.type} 不在允许列表中`);
        return false;
      }
    }

    if (isPreferred) {
      this._setDebug(`   ✅ 候选通过筛选（优先类型: ${info.type}），将添加到PeerConnection`);
    } else {
      this._setDebug(`   ✅ 候选通过筛选（备用类型: ${info.type}），将添加到PeerConnection`);
    }
    return true;
  }

  /**
   * 发送 ICE 候选 - 兼容新旧接口
   */
  _sendICE(candidate) {
    if (!this.signaling) {
      this._setError("No signaling manager available");
      return;
    }

    try {
      this.signaling.sendICE(candidate);
      this._setDebug("ICE candidate sent to signaling server");
    } catch (error) {
      this._setError("Failed to send ICE candidate: " + error.message);
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
        break;
      case "failed":
        this._setError("Peer connection failed");
        break;
    }
  }

  /**
   * 处理接收到的媒体轨道 - 优化版本
   */
  _ontrack(event) {
    if (!this.streams) this.streams = [];
    this.streams.push([event.track.kind, event.streams]);

    if (
      (event.track.kind === "video" || event.track.kind === "audio") &&
      this.element
    ) {
      this.element.srcObject = event.streams[0];
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
          this.eventBus?.emit("webrtc:system-action", {
            action: msg.data.action,
          });
        }
      },
      ping: () => {
        this.sendDataChannelMessage("pong," + Date.now() / 1000);
      },
      system_stats: () => this.eventBus?.emit("webrtc:system-stats", msg.data),
      latency_measurement: () => {
        this.eventBus?.emit("webrtc:latency-measurement", {
          latency: msg.data.latency_ms,
        });
      },
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
        video: {
          bytesReceived: 0,
          frameWidth: 0,
          frameHeight: 0,
          framesPerSecond: 0,
        },
        audio: { bytesReceived: 0, packetsReceived: 0, packetsLost: 0 },
        data: { bytesReceived: 0, bytesSent: 0 },
      };

      stats.forEach((report) => {
        if (report.type === "inbound-rtp") {
          if (report.kind === "video") {
            result.video = {
              bytesReceived: report.bytesReceived || 0,
              frameWidth: report.frameWidth || 0,
              frameHeight: report.frameHeight || 0,
              framesPerSecond: report.framesPerSecond || 0,
            };
          } else if (report.kind === "audio") {
            result.audio = {
              bytesReceived: report.bytesReceived || 0,
              packetsReceived: report.packetsReceived || 0,
              packetsLost: report.packetsLost || 0,
            };
          }
        } else if (report.type === "candidate-pair" && report.selected) {
          result.general.currentRoundTripTime = report.currentRoundTripTime;
        } else if (report.type === "data-channel") {
          result.data = {
            bytesReceived: report.bytesReceived || 0,
            bytesSent: report.bytesSent || 0,
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
   * 设置信令客户端
   */
  setSignalingClient(signalingClient) {
    this.signaling = signalingClient;
    if (this.signaling) {
      this.signaling.onsdp = this._onSDP.bind(this);
      this.signaling.onice = this._onSignalingICE.bind(this);

      // 设置对等ID
      this.signaling.peerId = this.peer_id;
    }
  }

  /**
   * 获取连接状态
   */
  getConnectionState() {
    const state = {
      connectionState: this.connectionState,
      connected: this._connected,
      peerConnection: this.peerConnection?.connectionState || null,
      iceConnectionState: this.peerConnection?.iceConnectionState || null,
      signalingState: this.peerConnection?.signalingState || null,
      signaling: this._getSignalingState(),
    };

    return state;
  }

  /**
   * 获取信令连接状态
   */
  _getSignalingState() {
    if (!this.signaling) {
      return { available: false, state: "unavailable" };
    }

    const signalingState = {
      available: true,
    };

    if (typeof this.signaling.getState === "function") {
      const clientState = this.signaling.getState();
      signalingState.state = clientState.connectionState;
      signalingState.connected = clientState.isConnected;
      signalingState.retryCount = clientState.retryCount;
      signalingState.protocolMode = clientState.protocolMode;
    } else {
      signalingState.state = "unknown";
      signalingState.connected = false;
    }

    return signalingState;
  }

  /**
   * 设置 ICE 候选筛选配置
   * @param {Object} config - 筛选配置
   * @param {Array<string>} config.preferredTypes - 优先使用的候选类型 ['srflx', 'relay', 'host']
   * @param {Array<string>} config.allowedTypes - 允许的候选类型
   * @param {boolean} config.strictMode - 严格模式（只使用优先类型）
   * @param {boolean} config.blockIPv6LinkLocal - 阻止 IPv6 link-local 地址
   */
  setICEFilterConfig(config) {
    if (config.preferredTypes) {
      this.iceFilterConfig.preferredTypes = config.preferredTypes;
    }
    if (config.allowedTypes) {
      this.iceFilterConfig.allowedTypes = config.allowedTypes;
    }
    if (typeof config.strictMode === 'boolean') {
      this.iceFilterConfig.strictMode = config.strictMode;
    }
    if (typeof config.blockIPv6LinkLocal === 'boolean') {
      this.iceFilterConfig.blockIPv6LinkLocal = config.blockIPv6LinkLocal;
    }

    this._setStatus(`ICE筛选配置已更新: 优先=${this.iceFilterConfig.preferredTypes.join(',')}, 严格模式=${this.iceFilterConfig.strictMode}`);
    this.eventBus?.emit("webrtc:ice-filter-config-updated", this.iceFilterConfig);
  }

  /**
   * 获取当前 ICE 候选筛选配置
   */
  getICEFilterConfig() {
    return { ...this.iceFilterConfig };
  }

  /**
   * 打印选中的候选对信息
   */
  async _logSelectedCandidatePair() {
    if (!this.peerConnection) return;

    try {
      const stats = await this.peerConnection.getStats();
      stats.forEach(report => {
        if (report.type === 'candidate-pair' && report.state === 'succeeded') {
          console.log(`✅ [ICE] 成功的候选对:`, report);
          this._setStatus(`✅ 成功的候选对: ${report.id}`);
          this._setDebug(`✅ 成功的候选对: ${report.id}`);
          
          // 查找本地和远程候选的详细信息
          stats.forEach(candidateReport => {
            if (candidateReport.id === report.localCandidateId) {
              console.log(`   📤 [本地候选] ${candidateReport.candidateType} ${candidateReport.address || candidateReport.ip}:${candidateReport.port} (协议: ${candidateReport.protocol})`);
              this._setStatus(`   📤 本地: ${candidateReport.candidateType} ${candidateReport.address || candidateReport.ip}:${candidateReport.port}`);
            }
            if (candidateReport.id === report.remoteCandidateId) {
              console.log(`   📥 [远程候选] ${candidateReport.candidateType} ${candidateReport.address || candidateReport.ip}:${candidateReport.port} (协议: ${candidateReport.protocol})`);
              this._setStatus(`   📥 远程: ${candidateReport.candidateType} ${candidateReport.address || candidateReport.ip}:${candidateReport.port}`);
            }
          });
        }
      });
    } catch (error) {
      console.error('获取候选对信息失败:', error);
    }
  }

  /**
   * 打印所有候选对的状态（用于调试失败情况）
   */
  async _logAllCandidatePairs() {
    if (!this.peerConnection) return;

    try {
      const stats = await this.peerConnection.getStats();
      const candidatePairs = [];
      const candidates = new Map();

      // 收集所有候选
      stats.forEach(report => {
        if (report.type === 'local-candidate' || report.type === 'remote-candidate') {
          candidates.set(report.id, report);
        }
      });

      // 收集所有候选对
      stats.forEach(report => {
        if (report.type === 'candidate-pair') {
          const localCandidate = candidates.get(report.localCandidateId);
          const remoteCandidate = candidates.get(report.remoteCandidateId);
          
          candidatePairs.push({
            state: report.state,
            local: localCandidate ? `${localCandidate.candidateType} ${localCandidate.address || localCandidate.ip}:${localCandidate.port}` : 'unknown',
            remote: remoteCandidate ? `${remoteCandidate.candidateType} ${remoteCandidate.address || remoteCandidate.ip}:${remoteCandidate.port}` : 'unknown',
            nominated: report.nominated,
            bytesSent: report.bytesSent || 0,
            bytesReceived: report.bytesReceived || 0
          });
        }
      });

      console.log(`📊 [ICE] 所有候选对状态 (共 ${candidatePairs.length} 对):`, candidatePairs);
      this._setStatus(`📊 检查了 ${candidatePairs.length} 个候选对，但都失败了`);
      
      candidatePairs.forEach((pair, index) => {
        const emoji = pair.state === 'succeeded' ? '✅' : pair.state === 'failed' ? '❌' : '⏸️';
        console.log(`   ${emoji} 候选对 ${index + 1}: ${pair.state}`);
        console.log(`      本地: ${pair.local}`);
        console.log(`      远程: ${pair.remote}`);
        this._setDebug(`   ${emoji} [${pair.state}] ${pair.local} <-> ${pair.remote}`);
      });
    } catch (error) {
      console.error('获取候选对信息失败:', error);
    }
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

# Web 管理界面设计

本文档描述了 BDWind-GStreamer 项目的 Web 管理界面设计，基于 selkies-gstreamer 项目的架构进行改进和扩展。

## 概述

Web 管理界面提供了一个完整的基于浏览器的桌面捕获和管理系统，用户可以通过 Web 浏览器远程访问和控制 Linux 桌面环境。

## 架构设计

### 整体架构

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Web Browser   │    │   Web Server    │    │  GStreamer      │
│                 │    │                 │    │  Pipeline       │
│  - Vue.js App   │◄──►│  - HTTP Server  │◄──►│                 │
│  - WebRTC       │    │  - WebSocket    │    │  - Desktop      │
│  - Media Player │    │  - Signaling    │    │    Capture      │
│  - Input Handler│    │  - REST API     │    │  - Encoding     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### 技术栈

#### 后端
- **Go HTTP Server**: 基于 `net/http` 的 Web 服务器
- **WebSocket**: 实时信令和数据传输
- **WebRTC**: 媒体流传输
- **GStreamer**: 桌面捕获和编码
- **Prometheus**: 监控指标收集

#### 前端
- **Vue.js 2.x**: 响应式 UI 框架
- **Vuetify**: Material Design 组件库
- **WebRTC API**: 浏览器媒体流处理
- **Canvas API**: 自定义渲染和输入处理

## 功能模块

### 1. Web 服务器基础架构

#### HTTP 服务器
```go
type WebServer struct {
    server     *http.Server
    router     *mux.Router
    config     *WebServerConfig
    signaling  *SignalingServer
    auth       auth.Authorizer
    metrics    *monitoring.MetricsCollector
}

type WebServerConfig struct {
    Port         int    `yaml:"port"`
    Host         string `yaml:"host"`
    TLSCertFile  string `yaml:"tls_cert_file"`
    TLSKeyFile   string `yaml:"tls_key_file"`
    StaticDir    string `yaml:"static_dir"`
    EnableTLS    bool   `yaml:"enable_tls"`
}
```

#### WebSocket 信令服务器
```go
type SignalingServer struct {
    clients    map[string]*SignalingClient
    broadcast  chan []byte
    register   chan *SignalingClient
    unregister chan *SignalingClient
    mutex      sync.RWMutex
}

type SignalingMessage struct {
    Type    string      `json:"type"`
    PeerID  string      `json:"peer_id"`
    Data    interface{} `json:"data"`
}
```

#### RESTful API 接口
- `GET /api/status` - 获取系统状态
- `GET /api/displays` - 获取可用显示器列表
- `POST /api/capture/start` - 开始桌面捕获
- `POST /api/capture/stop` - 停止桌面捕获
- `GET /api/stats` - 获取实时统计信息
- `POST /api/auth/login` - 用户登录
- `POST /api/auth/logout` - 用户登出

### 2. 前端核心框架

#### Vue.js 应用结构
```javascript
const app = new Vue({
    el: '#app',
    data: {
        // 连接状态
        status: 'connecting',
        
        // 媒体设置
        videoBitRate: 8000,
        videoFramerate: 60,
        audioBitRate: 128000,
        
        // 显示设置
        resizeRemote: true,
        scaleLocal: false,
        
        // 统计信息
        connectionStat: {},
        gpuStat: {},
        cpuStat: {},
        
        // UI 状态
        showDrawer: false,
        showStart: false,
        logEntries: [],
    },
    
    methods: {
        // 媒体控制方法
        playStream() {},
        enterFullscreen() {},
        
        // 设置控制方法
        updateVideoBitRate(value) {},
        updateVideoFramerate(value) {},
        
        // 统计信息方法
        updateStats() {},
    }
});
```

#### WebRTC 客户端
```javascript
class WebRTCClient {
    constructor(signaling, videoElement, audioElement) {
        this.signaling = signaling;
        this.videoElement = videoElement;
        this.audioElement = audioElement;
        this.peerConnection = null;
        this.dataChannel = null;
    }
    
    async connect() {
        // 创建 RTCPeerConnection
        // 设置媒体流处理
        // 建立数据通道
    }
    
    sendInputEvent(event) {
        // 发送输入事件到远程桌面
    }
    
    getConnectionStats() {
        // 获取连接统计信息
    }
}
```

### 3. 桌面流媒体界面

#### 视频流显示组件
```html
<div id="video_container" class="video-container">
    <video id="stream" 
           class="video" 
           preload="none" 
           disablePictureInPicture="true" 
           playsinline>
        Your browser doesn't support video
    </video>
</div>
```

#### 音频流播放组件
```html
<div id="audio_container" class="audio-container">
    <audio id="audio_stream" 
           class="audio" 
           preload="none" 
           playsinline>
        Your browser doesn't support audio
    </audio>
</div>
```

#### 输入事件处理
```javascript
class InputHandler {
    constructor(videoElement, webrtcClient) {
        this.videoElement = videoElement;
        this.webrtcClient = webrtcClient;
        this.attachEventListeners();
    }
    
    attachEventListeners() {
        // 鼠标事件
        this.videoElement.addEventListener('mousedown', this.handleMouseDown.bind(this));
        this.videoElement.addEventListener('mouseup', this.handleMouseUp.bind(this));
        this.videoElement.addEventListener('mousemove', this.handleMouseMove.bind(this));
        
        // 键盘事件
        document.addEventListener('keydown', this.handleKeyDown.bind(this));
        document.addEventListener('keyup', this.handleKeyUp.bind(this));
        
        // 滚轮事件
        this.videoElement.addEventListener('wheel', this.handleWheel.bind(this));
    }
    
    handleMouseDown(event) {
        const inputEvent = {
            type: 'mouse',
            action: 'down',
            button: event.button,
            x: event.offsetX,
            y: event.offsetY
        };
        this.webrtcClient.sendInputEvent(inputEvent);
    }
}
```

### 4. 控制面板界面

#### 视频质量控制
```html
<v-select 
    :items="videoBitRateOptions" 
    label="视频比特率" 
    v-model="videoBitRate"
    @change="updateVideoBitRate">
</v-select>

<v-select 
    :items="videoFramerateOptions" 
    label="视频帧率" 
    v-model="videoFramerate"
    @change="updateVideoFramerate">
</v-select>
```

#### 显示设置控制
```html
<v-switch 
    v-model="resizeRemote" 
    :label="`调整远程分辨率: ${resizeRemote.toString()}`">
</v-switch>

<v-switch 
    v-model="scaleLocal" 
    :label="`本地缩放适应: ${scaleLocal.toString()}`">
</v-switch>
```

#### 连接统计显示
```html
<div class="stats-panel">
    <v-progress-circular 
        :value="(connectionStat.connectionVideoBitrate / (videoBitRate / 1000))*100"
        color="teal">
        {{ connectionStat.connectionVideoBitrate }}
    </v-progress-circular>
    
    <v-progress-circular 
        :value="(connectionStat.connectionFrameRate / 60)*100" 
        color="blue-grey">
        {{ connectionStat.connectionFrameRate }}
    </v-progress-circular>
    
    <v-progress-circular 
        :value="(connectionStat.connectionLatency / 1000)*100" 
        color="red">
        {{ connectionStat.connectionLatency }}
    </v-progress-circular>
</div>
```

### 5. 系统监控界面

#### 实时性能监控
```html
<div class="monitoring-panel">
    <div class="metric-card">
        <h3>CPU 使用率</h3>
        <v-progress-circular 
            :value="cpuStat.serverCPUUsage" 
            color="blue">
            {{ cpuStat.serverCPUUsage }}%
        </v-progress-circular>
    </div>
    
    <div class="metric-card">
        <h3>GPU 使用率</h3>
        <v-progress-circular 
            :value="gpuStat.gpuLoad" 
            color="green">
            {{ gpuStat.gpuLoad }}%
        </v-progress-circular>
    </div>
    
    <div class="metric-card">
        <h3>内存使用</h3>
        <v-progress-circular 
            :value="(cpuStat.serverMemoryUsed / cpuStat.serverMemoryTotal) * 100" 
            color="orange">
            {{ (cpuStat.serverMemoryUsed / 1024 / 1024 / 1024).toFixed(2) }}GB
        </v-progress-circular>
    </div>
</div>
```

#### 连接状态监控
```html
<div class="connection-stats">
    <ul>
        <li>连接状态: <b>{{ status }}</b></li>
        <li>连接类型: <b>{{ connectionStat.connectionStatType }}</b></li>
        <li>接收包数: <b>{{ connectionStat.connectionPacketsReceived }}</b></li>
        <li>丢包数: <b>{{ connectionStat.connectionPacketsLost }}</b></li>
        <li>接收字节: <b>{{ connectionStat.connectionBytesReceived }}</b></li>
        <li>发送字节: <b>{{ connectionStat.connectionBytesSent }}</b></li>
    </ul>
</div>
```

### 6. 用户管理界面

#### 登录界面
```html
<v-form v-model="loginValid" @submit.prevent="login">
    <v-text-field
        v-model="username"
        label="用户名"
        :rules="[rules.required]"
        required>
    </v-text-field>
    
    <v-text-field
        v-model="password"
        label="密码"
        type="password"
        :rules="[rules.required]"
        required>
    </v-text-field>
    
    <v-btn 
        type="submit" 
        color="primary" 
        :disabled="!loginValid">
        登录
    </v-btn>
</v-form>
```

#### 会话管理界面
```html
<v-data-table
    :headers="sessionHeaders"
    :items="userSessions"
    class="elevation-1">
    
    <template v-slot:item.actions="{ item }">
        <v-btn 
            small 
            color="error" 
            @click="revokeSession(item.id)">
            撤销
        </v-btn>
    </template>
</v-data-table>
```

### 7. 移动端适配

#### 响应式设计
```css
/* 桌面端 */
@media (min-width: 1024px) {
    .video-container {
        width: 100%;
        height: 100vh;
    }
    
    .control-panel {
        position: fixed;
        right: 0;
        width: 400px;
    }
}

/* 平板端 */
@media (max-width: 1023px) and (min-width: 768px) {
    .video-container {
        width: 100%;
        height: 70vh;
    }
    
    .control-panel {
        position: relative;
        width: 100%;
        height: 30vh;
    }
}

/* 手机端 */
@media (max-width: 767px) {
    .video-container {
        width: 100%;
        height: 60vh;
    }
    
    .control-panel {
        position: relative;
        width: 100%;
        height: 40vh;
        overflow-y: auto;
    }
}
```

#### 触摸输入支持
```javascript
class TouchInputHandler {
    constructor(videoElement, webrtcClient) {
        this.videoElement = videoElement;
        this.webrtcClient = webrtcClient;
        this.attachTouchListeners();
    }
    
    attachTouchListeners() {
        this.videoElement.addEventListener('touchstart', this.handleTouchStart.bind(this));
        this.videoElement.addEventListener('touchmove', this.handleTouchMove.bind(this));
        this.videoElement.addEventListener('touchend', this.handleTouchEnd.bind(this));
    }
    
    handleTouchStart(event) {
        event.preventDefault();
        const touch = event.touches[0];
        const mouseEvent = {
            type: 'mouse',
            action: 'down',
            button: 0,
            x: touch.clientX,
            y: touch.clientY
        };
        this.webrtcClient.sendInputEvent(mouseEvent);
    }
}
```

## 部署配置

### Docker 容器化
```dockerfile
FROM node:16-alpine AS frontend-builder
WORKDIR /app
COPY web/package*.json ./
RUN npm install
COPY web/ .
RUN npm run build

FROM golang:1.21-alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o bdwind-gstreamer ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates gstreamer gst-plugins-base gst-plugins-good
WORKDIR /root/
COPY --from=backend-builder /app/bdwind-gstreamer .
COPY --from=frontend-builder /app/dist ./web/dist
EXPOSE 8080
CMD ["./bdwind-gstreamer"]
```

### Nginx 反向代理
```nginx
server {
    listen 80;
    server_name your-domain.com;
    
    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    
    location /ws {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }
}
```

## 安全考虑

### HTTPS/WSS 支持
- 强制使用 HTTPS 连接
- WebSocket 使用 WSS 加密传输
- 支持自签名证书和 Let's Encrypt

### 访问控制集成
- 集成现有的访问控制系统
- 支持基于角色的权限管理
- 会话超时和自动登出

### 输入验证和过滤
- 所有用户输入进行验证和过滤
- 防止 XSS 和 CSRF 攻击
- 限制文件上传和下载

## 性能优化

### 前端优化
- 代码分割和懒加载
- 静态资源压缩和缓存
- WebRTC 连接优化

### 后端优化
- WebSocket 连接池管理
- 静态文件缓存
- GStreamer 管道优化

### 网络优化
- 自适应比特率调整
- 网络拥塞控制
- 延迟优化

## 监控和日志

### 前端监控
- 用户行为分析
- 性能指标收集
- 错误日志上报

### 后端监控
- WebSocket 连接监控
- API 请求监控
- 系统资源监控

### 日志管理
- 结构化日志输出
- 日志轮转和归档
- 实时日志查看

## 未来扩展

### 多用户支持
- 多用户并发访问
- 用户隔离和资源管理
- 负载均衡

### 插件系统
- 自定义功能插件
- 第三方集成
- API 扩展

### 云原生支持
- Kubernetes 部署
- 微服务架构
- 服务网格集成

这个 Web 管理界面设计提供了一个完整的基于浏览器的桌面捕获和管理解决方案，结合了现代 Web 技术和高性能的媒体流处理能力。
# GStreamer 故障排除和调试指南

## 概述

本文档提供了重构后 GStreamer 组件的故障排除和调试指南，包括常见问题的诊断和解决方法、调试工具的使用、日志分析技巧，以及 `scripts/start-debug.sh` 脚本的详细使用说明。

## 调试环境设置

### 1. 使用 start-debug.sh 脚本

`scripts/start-debug.sh` 脚本提供了完整的调试环境设置，支持虚拟显示和详细日志记录。

#### 1.1 脚本功能

```bash
#!/bin/bash
# scripts/start-debug.sh - GStreamer 调试启动脚本

set -e

# 配置变量
DISPLAY_NUM=${DISPLAY_NUM:-:99}
RESOLUTION=${RESOLUTION:-1920x1080}
LOG_LEVEL=${LOG_LEVEL:-debug}
CONFIG_FILE=${CONFIG_FILE:-examples/debug_config.yaml}

# 日志配置
LOG_DIR="logs"
LOG_FILE="$LOG_DIR/gstreamer-debug-$(date +%Y%m%d-%H%M%S).log"

echo "=== GStreamer Debug Environment Setup ==="
echo "Display: $DISPLAY_NUM"
echo "Resolution: $RESOLUTION"
echo "Log Level: $LOG_LEVEL"
echo "Config File: $CONFIG_FILE"
echo "Log File: $LOG_FILE"
echo "=========================================="

# 创建日志目录
mkdir -p "$LOG_DIR"

# 启动虚拟显示
start_virtual_display() {
    echo "Starting virtual display..."
    
    # 检查是否已有 Xvfb 运行
    if pgrep -f "Xvfb $DISPLAY_NUM" > /dev/null; then
        echo "Virtual display $DISPLAY_NUM already running"
    else
        # 启动 Xvfb
        Xvfb $DISPLAY_NUM -screen 0 ${RESOLUTION}x24 -ac +extension GLX +render -noreset &
        XVFB_PID=$!
        echo "Started Xvfb with PID: $XVFB_PID"
        
        # 等待 X 服务器启动
        sleep 2
    fi
    
    # 设置 DISPLAY 环境变量
    export DISPLAY=$DISPLAY_NUM
    
    # 启动窗口管理器
    if ! pgrep -f "fluxbox" > /dev/null; then
        fluxbox &
        FLUXBOX_PID=$!
        echo "Started fluxbox with PID: $FLUXBOX_PID"
    fi
    
    # 启动一些测试应用
    xterm -geometry 80x24+100+100 &
    xclock -geometry 200x200+300+300 &
    
    echo "Virtual display setup complete"
}
```

#### 1.2 完整的启动脚本

```bash
# 启动虚拟显示
start_virtual_display

# 设置 GStreamer 调试环境变量
export GST_DEBUG=${GST_DEBUG:-4}
export GST_DEBUG_FILE="$LOG_DIR/gstreamer-pipeline-$(date +%Y%m%d-%H%M%S).log"
export GST_DEBUG_DUMP_DOT_DIR="$LOG_DIR/pipeline-graphs"

# 创建管道图目录
mkdir -p "$GST_DEBUG_DUMP_DOT_DIR"

# 设置其他调试变量
export GSTREAMER_LOG_LEVEL=$LOG_LEVEL
export GSTREAMER_CONFIG_FILE=$CONFIG_FILE
export GSTREAMER_DEBUG_MODE=true

echo "Environment variables set:"
echo "  GST_DEBUG=$GST_DEBUG"
echo "  GST_DEBUG_FILE=$GST_DEBUG_FILE"
echo "  GST_DEBUG_DUMP_DOT_DIR=$GST_DEBUG_DUMP_DOT_DIR"
echo "  DISPLAY=$DISPLAY"

# 编译应用程序
echo "Building application..."
go build -o bin/bdwind-gstreamer-debug cmd/bdwind-gstreamer/main.go

# 启动应用程序
echo "Starting GStreamer application..."
./bin/bdwind-gstreamer-debug 2>&1 | tee "$LOG_FILE"

# 清理函数
cleanup() {
    echo "Cleaning up..."
    
    # 停止应用程序
    if [ ! -z "$APP_PID" ]; then
        kill $APP_PID 2>/dev/null || true
    fi
    
    # 停止虚拟显示相关进程
    pkill -f "Xvfb $DISPLAY_NUM" 2>/dev/null || true
    pkill -f "fluxbox" 2>/dev/null || true
    pkill -f "xterm" 2>/dev/null || true
    pkill -f "xclock" 2>/dev/null || true
    
    echo "Cleanup complete"
}

# 设置信号处理
trap cleanup EXIT INT TERM

# 等待用户中断
wait
```

### 2. 调试配置文件

你的 `examples/debug_config.yaml` 配置文件已经很好了，简洁实用。如果需要更详细的调试，可以添加这些选项：

```yaml
# 在你现有配置基础上，可以添加这些调试选项：

# 启用详细日志
logging:
  level: "debug"        # 改为 debug 级别
  format: "text"        # 或 "json" 看个人喜好
  enable_caller: true   # 显示代码位置

# GStreamer 调试选项
gstreamer:
  debug:
    enabled: true
    dump_pipeline_graphs: true    # 生成管道图
    collect_statistics: true      # 收集性能统计
    statistics_interval: 5s
  
  capture:
    framerate: 15      # 调试时降低帧率
    use_gpu: false     # 禁用 GPU 避免驱动问题
  
  encoding:
    preset: "fast"     # 使用快速预设
    quality: 5         # 中等质量便于调试
```

主要调试技巧：
- 降低帧率和分辨率减少系统负载
- 禁用 GPU 加速避免驱动问题  
- 启用管道图生成便于可视化调试
- 使用文本格式日志便于阅读

## 常见问题诊断

### 1. 启动问题

#### 1.1 应用程序无法启动

**症状：** 应用程序启动失败，出现初始化错误

**可能原因：**
- GStreamer 库未正确安装
- 缺少必要的 GStreamer 插件
- 权限问题
- 配置文件错误

**诊断步骤：**

```bash
# 1. 检查 GStreamer 安装
gst-inspect-1.0 --version

# 2. 检查必要插件
gst-inspect-1.0 ximagesrc
gst-inspect-1.0 x264enc
gst-inspect-1.0 webrtcbin

# 3. 检查权限
ls -la /dev/dri/  # GPU 设备权限
xhost +local:     # X11 权限

# 4. 验证配置文件
go run cmd/bdwind-gstreamer/main.go --config examples/debug_config.yaml --validate-config
```

**解决方案：**

```bash
# 安装缺少的 GStreamer 插件
sudo apt-get install gstreamer1.0-plugins-base \
                     gstreamer1.0-plugins-good \
                     gstreamer1.0-plugins-bad \
                     gstreamer1.0-plugins-ugly \
                     gstreamer1.0-libav

# 添加用户到必要的组
sudo usermod -a -G video,audio $USER

# 重新登录或使用 newgrp
newgrp video
```

#### 1.2 虚拟显示启动失败

**症状：** Xvfb 无法启动或 DISPLAY 变量设置错误

**诊断命令：**

```bash
# 检查 Xvfb 进程
ps aux | grep Xvfb

# 检查 X11 连接
xdpyinfo -display :99

# 测试 X11 应用
DISPLAY=:99 xterm &
```

**解决方案：**

```bash
# 安装 Xvfb 和相关工具
sudo apt-get install xvfb fluxbox xterm x11-utils

# 手动启动虚拟显示
Xvfb :99 -screen 0 1920x1080x24 -ac +extension GLX +render -noreset &
export DISPLAY=:99
fluxbox &
```

### 2. 管道问题

#### 2.1 管道状态异常

**症状：** 管道无法切换到 PLAYING 状态

**诊断方法：**

```bash
# 启用详细的 GStreamer 调试
export GST_DEBUG=4
export GST_DEBUG_FILE=pipeline-debug.log

# 运行应用程序并检查日志
./bin/bdwind-gstreamer-debug

# 分析管道图
dot -Tpng pipeline-graph.dot -o pipeline-graph.png
```

**常见错误和解决方案：**

```bash
# 错误：Could not link elements
# 原因：元素之间的 caps 不兼容
# 解决：添加 capsfilter 或 videoconvert 元素

# 错误：No such element or plugin
# 原因：缺少 GStreamer 插件
# 解决：安装相应插件包

# 错误：Resource busy
# 原因：设备被其他进程占用
# 解决：停止占用设备的进程
lsof /dev/video0
```

#### 2.2 桌面捕获失败

**症状：** 无法捕获桌面内容或捕获到黑屏

**诊断步骤：**

```bash
# 1. 检查 X11 显示
echo $DISPLAY
xdpyinfo

# 2. 测试 ximagesrc
gst-launch-1.0 ximagesrc ! videoconvert ! autovideosink

# 3. 检查窗口管理器
ps aux | grep -E "(fluxbox|openbox|xfwm4)"

# 4. 测试不同的捕获源
gst-launch-1.0 ximagesrc startx=100 starty=100 endx=500 endy=400 ! \
    videoconvert ! autovideosink
```

**解决方案：**

```bash
# 启动窗口管理器
fluxbox &

# 启动一些可见的应用程序
xterm -geometry 80x24+100+100 &
xclock -geometry 200x200+300+300 &

# 使用 xwininfo 查看窗口信息
xwininfo -root -tree
```

### 3. 编码问题

#### 3.1 硬件编码器不可用

**症状：** 硬件编码器初始化失败，回退到软件编码

**诊断命令：**

```bash
# 检查 GPU 设备
ls -la /dev/dri/
lspci | grep VGA

# 检查 VAAPI 支持
vainfo

# 检查 NVENC 支持
nvidia-smi
```

**解决方案：**

```bash
# 安装 VAAPI 驱动
sudo apt-get install vainfo intel-media-va-driver

# 安装 NVIDIA 驱动和 NVENC 支持
sudo apt-get install nvidia-driver-470 libnvidia-encode-470

# 在配置中强制使用软件编码器
video_encoder:
  encoder_preference: "software"
```

#### 3.2 编码质量问题

**症状：** 视频质量差或比特率异常

**诊断方法：**

```bash
# 启用编码器调试
export GST_DEBUG=x264enc:5,vaapih264enc:5

# 检查编码器属性
gst-inspect-1.0 x264enc | grep -A 5 -B 5 bitrate
```

**调优参数：**

```yaml
video_encoder:
  codec: "h264"
  bitrate: 2000000
  quality: 7        # 提高质量
  preset: "medium"  # 平衡质量和性能
  profile: "high"   # 使用高级配置文件
  keyframe_interval: 30
```

### 4. 内存问题

#### 4.1 内存泄漏

**症状：** 应用程序内存使用持续增长

**诊断工具：**

```bash
# 使用 valgrind 检测内存泄漏
valgrind --tool=memcheck --leak-check=full \
         --show-leak-kinds=all --track-origins=yes \
         ./bin/bdwind-gstreamer-debug

# 使用 pprof 进行 Go 内存分析
go tool pprof http://localhost:8080/debug/pprof/heap

# 监控内存使用
watch -n 1 'ps -p $(pgrep bdwind-gstreamer) -o pid,vsz,rss,pmem'
```

**解决方案：**

```go
// 确保正确释放 GStreamer 对象
defer element.Unref()

// 使用内存管理器
ref := memManager.RegisterObject(element)
defer memManager.UnregisterObject(ref)

// 定期触发 GC
go func() {
    ticker := time.NewTicker(30 * time.Second)
    for range ticker.C {
        runtime.GC()
    }
}()
```

#### 4.2 缓冲区溢出

**症状：** 应用程序崩溃或出现 segmentation fault

**诊断方法：**

```bash
# 启用核心转储
ulimit -c unlimited

# 使用 gdb 分析崩溃
gdb ./bin/bdwind-gstreamer-debug core

# 在 gdb 中查看堆栈
(gdb) bt
(gdb) info registers
```

**预防措施：**

```go
// 使用缓冲区池
pool := NewBufferPool(1024*1024, 10, 50)
buffer, err := pool.AcquireBuffer()
if err != nil {
    return err
}
defer pool.ReleaseBuffer(buffer)

// 检查缓冲区大小
if len(data) > maxBufferSize {
    return fmt.Errorf("buffer too large: %d > %d", len(data), maxBufferSize)
}
```

## 日志分析

### 1. 日志级别和格式

#### 1.1 配置日志级别

```yaml
logging:
  level: "debug"    # trace, debug, info, warn, error
  format: "json"    # json, text
  output: "stdout"  # stdout, file
  file: "logs/gstreamer.log"
```

#### 1.2 GStreamer 调试日志

```bash
# 设置 GStreamer 调试级别
export GST_DEBUG=4                    # 全局调试级别
export GST_DEBUG=ximagesrc:5,x264enc:5  # 特定元素调试

# 调试类别
# 0 - 无调试信息
# 1 - 错误信息
# 2 - 警告信息
# 3 - 固定信息
# 4 - 信息
# 5 - 调试信息
# 6 - 日志信息
# 7 - 跟踪信息
# 9 - 内存转储
```

### 2. 日志分析工具

#### 2.1 日志解析脚本

```bash
#!/bin/bash
# scripts/analyze-logs.sh - 日志分析脚本

LOG_FILE=${1:-"logs/gstreamer-debug.log"}

if [ ! -f "$LOG_FILE" ]; then
    echo "Log file not found: $LOG_FILE"
    exit 1
fi

echo "=== Log Analysis Report ==="
echo "File: $LOG_FILE"
echo "Size: $(du -h $LOG_FILE | cut -f1)"
echo "Lines: $(wc -l < $LOG_FILE)"
echo

# 错误统计
echo "=== Error Summary ==="
grep -i "error" "$LOG_FILE" | wc -l | xargs echo "Total errors:"
grep -i "error" "$LOG_FILE" | head -5
echo

# 警告统计
echo "=== Warning Summary ==="
grep -i "warn" "$LOG_FILE" | wc -l | xargs echo "Total warnings:"
grep -i "warn" "$LOG_FILE" | head -5
echo

# 性能统计
echo "=== Performance Metrics ==="
grep -i "fps\|framerate\|bitrate" "$LOG_FILE" | tail -10
echo

# 内存使用
echo "=== Memory Usage ==="
grep -i "memory\|leak\|alloc" "$LOG_FILE" | tail -5
echo

# 状态变化
echo "=== State Changes ==="
grep -i "state.*change\|playing\|paused\|ready" "$LOG_FILE" | tail -10
```

#### 2.2 实时日志监控

```bash
#!/bin/bash
# scripts/monitor-logs.sh - 实时日志监控

LOG_FILE=${1:-"logs/gstreamer-debug.log"}

# 使用 multitail 同时监控多个日志
multitail \
    -ci red -I "$LOG_FILE" -l "grep -i error" \
    -ci yellow -I "$LOG_FILE" -l "grep -i warn" \
    -ci green -I "$LOG_FILE" -l "grep -i 'fps\|bitrate'"

# 或使用 tail 和 grep
tail -f "$LOG_FILE" | grep --line-buffered -E "(ERROR|WARN|fps|bitrate)" | \
while read line; do
    if echo "$line" | grep -q "ERROR"; then
        echo -e "\033[31m$line\033[0m"  # 红色
    elif echo "$line" | grep -q "WARN"; then
        echo -e "\033[33m$line\033[0m"  # 黄色
    else
        echo -e "\033[32m$line\033[0m"  # 绿色
    fi
done
```

### 3. 关键日志模式

#### 3.1 正常启动日志

```
INFO  Starting GStreamer application
DEBUG Created pipeline: debug-pipeline
DEBUG Added element: ximagesrc (desktop-capture-source)
DEBUG Added element: videoconvert (video-converter)
DEBUG Added element: x264enc (video-encoder)
DEBUG Linked elements: desktop-capture-source -> video-converter
DEBUG Linked elements: video-converter -> video-encoder
INFO  Pipeline state changed: NULL -> READY
INFO  Pipeline state changed: READY -> PAUSED
INFO  Pipeline state changed: PAUSED -> PLAYING
INFO  GStreamer application started successfully
DEBUG Video frame: 1920x1080, keyframe=true, bitrate=2000000
DEBUG Video frame: 1920x1080, keyframe=false, bitrate=2000000
```

#### 3.2 错误日志模式

```
ERROR Failed to create element: ximagesrc
ERROR Element linking failed: desktop-capture-source -> video-converter
WARN  Hardware encoder not available, falling back to software
ERROR Pipeline error: Could not negotiate format
ERROR State change failed: READY -> PAUSED
ERROR Memory allocation failed: size=1048576
```

## 性能调优

### 1. 性能监控

#### 1.1 内置性能指标

```go
// 启用性能监控
config.Debug.EnableProfiling = true
config.Debug.CollectStatistics = true
config.Debug.StatisticsInterval = 5 * time.Second

// 访问性能指标
http://localhost:8080/debug/pprof/
http://localhost:8080/api/v1/statistics
```

#### 1.2 系统资源监控

```bash
# CPU 使用率
top -p $(pgrep bdwind-gstreamer)

# 内存使用
ps -p $(pgrep bdwind-gstreamer) -o pid,vsz,rss,pmem

# GPU 使用率 (NVIDIA)
nvidia-smi -l 1

# 网络使用
iftop -i eth0
```

### 2. 性能优化建议

#### 2.1 编码器优化

```yaml
video_encoder:
  # 硬件编码器优先
  encoder_preference: "hardware"
  
  # 优化预设
  preset: "fast"        # ultrafast/fast/medium/slow
  
  # 合理的比特率
  bitrate: 2000000      # 2 Mbps for 1080p
  
  # 适当的关键帧间隔
  keyframe_interval: 30 # 1 second at 30fps
  
  # 使用高级配置文件
  profile: "high"
  level: "4.1"
```

#### 2.2 系统优化

```bash
# 增加文件描述符限制
ulimit -n 65536

# 优化网络缓冲区
echo 'net.core.rmem_max = 16777216' >> /etc/sysctl.conf
echo 'net.core.wmem_max = 16777216' >> /etc/sysctl.conf

# 设置 CPU 调度策略
chrt -f -p 50 $(pgrep bdwind-gstreamer)
```

## 调试工具集

### 1. GStreamer 工具

```bash
# gst-launch-1.0 - 测试管道
gst-launch-1.0 ximagesrc ! videoconvert ! autovideosink

# gst-inspect-1.0 - 检查元素
gst-inspect-1.0 ximagesrc

# gst-discoverer-1.0 - 媒体信息
gst-discoverer-1.0 file.mp4
```

### 2. 系统调试工具

```bash
# strace - 系统调用跟踪
strace -p $(pgrep bdwind-gstreamer) -e trace=file,network

# lsof - 打开文件列表
lsof -p $(pgrep bdwind-gstreamer)

# netstat - 网络连接
netstat -tulpn | grep bdwind-gstreamer
```

### 3. 自定义调试工具

#### 3.1 管道可视化工具

```bash
#!/bin/bash
# scripts/visualize-pipeline.sh - 管道可视化

DOT_DIR=${GST_DEBUG_DUMP_DOT_DIR:-"logs/pipeline-graphs"}
OUTPUT_DIR="logs/pipeline-images"

mkdir -p "$OUTPUT_DIR"

for dot_file in "$DOT_DIR"/*.dot; do
    if [ -f "$dot_file" ]; then
        base_name=$(basename "$dot_file" .dot)
        png_file="$OUTPUT_DIR/${base_name}.png"
        
        echo "Converting $dot_file to $png_file"
        dot -Tpng "$dot_file" -o "$png_file"
    fi
done

echo "Pipeline graphs generated in $OUTPUT_DIR"
```

#### 3.2 性能分析工具

```bash
#!/bin/bash
# scripts/performance-analysis.sh - 性能分析

PID=$(pgrep bdwind-gstreamer)
if [ -z "$PID" ]; then
    echo "GStreamer process not found"
    exit 1
fi

echo "Analyzing performance for PID: $PID"

# CPU 使用率
echo "=== CPU Usage ==="
ps -p $PID -o pid,pcpu,time

# 内存使用
echo "=== Memory Usage ==="
ps -p $PID -o pid,vsz,rss,pmem

# 线程信息
echo "=== Thread Information ==="
ps -p $PID -T -o pid,tid,pcpu,pmem,comm

# 文件描述符
echo "=== File Descriptors ==="
ls /proc/$PID/fd | wc -l | xargs echo "Open FDs:"

# 网络连接
echo "=== Network Connections ==="
netstat -p 2>/dev/null | grep $PID | wc -l | xargs echo "Network connections:"
```

## 故障恢复

### 1. 自动恢复机制

```go
// 配置自动恢复
config.Pipeline.AutoRecovery = true
config.Pipeline.MaxRestartAttempts = 3
config.Pipeline.RestartDelay = 5 * time.Second

// 实现恢复策略
type RecoveryManager struct {
    strategies map[ErrorType]RecoveryStrategy
}

func (rm *RecoveryManager) HandleError(err error) error {
    errorType := classifyError(err)
    strategy, exists := rm.strategies[errorType]
    
    if !exists {
        return fmt.Errorf("no recovery strategy for error type: %v", errorType)
    }
    
    if !strategy.CanRecover(err) {
        return fmt.Errorf("error is not recoverable: %v", err)
    }
    
    return strategy.Recover(context.Background(), rm.component)
}
```

### 2. 手动恢复步骤

#### 2.1 管道重启

```bash
# 通过 API 重启
curl -X POST http://localhost:8080/api/v1/restart

# 或发送信号
kill -USR1 $(pgrep bdwind-gstreamer)
```

#### 2.2 配置重载

```bash
# 重载配置文件
curl -X PUT http://localhost:8080/api/v1/config \
     -H "Content-Type: application/json" \
     -d @examples/debug_config.yaml

# 或发送信号
kill -HUP $(pgrep bdwind-gstreamer)
```

## 总结

本故障排除指南提供了完整的调试和问题解决方案：

1. **调试环境** - 使用 start-debug.sh 脚本设置完整的调试环境
2. **常见问题** - 启动、管道、编码、内存等问题的诊断和解决
3. **日志分析** - 日志配置、分析工具和关键模式识别
4. **性能调优** - 监控指标和优化建议
5. **调试工具** - GStreamer 工具、系统工具和自定义脚本
6. **故障恢复** - 自动恢复机制和手动恢复步骤

通过遵循本指南，开发者可以快速定位和解决 GStreamer 组件的各种问题，确保系统的稳定运行。
### 3. 
常见错误代码和解决方案

#### 3.1 GStreamer 错误代码

| 错误代码 | 描述 | 常见原因 | 解决方案 |
|---------|------|----------|----------|
| GST_CORE_ERROR_MISSING_PLUGIN | 缺少插件 | 插件未安装 | 安装相应的 GStreamer 插件包 |
| GST_CORE_ERROR_NEGOTIATION | 协商失败 | 元素间格式不兼容 | 添加转换元素或调整 caps |
| GST_RESOURCE_ERROR_NOT_FOUND | 资源未找到 | 设备不存在或权限不足 | 检查设备和权限 |
| GST_RESOURCE_ERROR_BUSY | 资源忙 | 设备被占用 | 停止占用设备的进程 |
| GST_STREAM_ERROR_CODEC_NOT_FOUND | 编解码器未找到 | 缺少编解码器 | 安装编解码器插件 |

#### 3.2 应用程序错误代码

| 错误代码 | 描述 | 解决方案 |
|---------|------|----------|
| GSTREAMER_ERROR_INIT | 初始化失败 | 检查 GStreamer 安装和环境变量 |
| GSTREAMER_ERROR_CONFIG | 配置错误 | 验证配置文件格式和内容 |
| GSTREAMER_ERROR_MEMORY | 内存错误 | 检查内存使用和泄漏 |
| GSTREAMER_ERROR_NETWORK | 网络错误 | 检查网络连接和防火墙 |

### 4. 高级调试技巧

#### 4.1 使用 GDB 调试

```bash
# 编译带调试信息的版本
go build -gcflags="-N -l" -o bin/bdwind-gstreamer-debug cmd/bdwind-gstreamer/main.go

# 使用 GDB 启动
gdb ./bin/bdwind-gstreamer-debug

# GDB 命令
(gdb) set environment GST_DEBUG=4
(gdb) set environment DISPLAY=:99
(gdb) run
(gdb) bt          # 查看调用栈
(gdb) info goroutines  # 查看 goroutine
(gdb) thread apply all bt  # 所有线程的调用栈
```

#### 4.2 使用 Delve 调试 Go 代码

```bash
# 安装 Delve
go install github.com/go-delve/delve/cmd/dlv@latest

# 启动调试会话
dlv debug cmd/bdwind-gstreamer/main.go

# Delve 命令
(dlv) break main.main
(dlv) continue
(dlv) goroutines      # 查看所有 goroutine
(dlv) goroutine 1 bt  # 查看特定 goroutine 的调用栈
(dlv) vars            # 查看变量
(dlv) print variable  # 打印变量值
```

#### 4.3 内存泄漏检测

```bash
# 使用 Go 内置的内存分析
go tool pprof http://localhost:8080/debug/pprof/heap

# 在 pprof 交互模式中
(pprof) top10        # 显示内存使用最多的函数
(pprof) list function_name  # 显示函数的内存分配
(pprof) web          # 生成可视化图表

# 使用 Valgrind (需要 CGO)
valgrind --tool=memcheck --leak-check=full --show-leak-kinds=all \
         --track-origins=yes --verbose --log-file=valgrind.log \
         ./bin/bdwind-gstreamer-debug
```

### 5. 网络问题调试

#### 5.1 WebRTC 连接问题

```bash
# 检查 ICE 连接
curl http://localhost:8080/api/v1/webrtc/ice-candidates

# 检查 STUN/TURN 服务器
stunclient stun.l.google.com 19302

# 网络连通性测试
nc -zv target_host target_port

# 防火墙检查
iptables -L -n
ufw status
```

#### 5.2 带宽和延迟测试

```bash
# 带宽测试
iperf3 -c target_server

# 延迟测试
ping -c 10 target_host

# 网络质量测试
mtr target_host
```

### 6. 容器化调试

#### 6.1 Docker 调试

```dockerfile
# Dockerfile.debug - 调试版本
FROM ubuntu:20.04

# 安装调试工具
RUN apt-get update && apt-get install -y \
    gdb \
    valgrind \
    strace \
    tcpdump \
    netcat \
    curl \
    htop \
    && rm -rf /var/lib/apt/lists/*

# 复制调试脚本
COPY scripts/debug/ /usr/local/bin/

# 设置调试环境
ENV GST_DEBUG=4
ENV GST_DEBUG_DUMP_DOT_DIR=/tmp/pipeline-graphs

# 暴露调试端口
EXPOSE 8080 9090 40000

CMD ["/usr/local/bin/start-debug.sh"]
```

```bash
# 构建调试镜像
docker build -f Dockerfile.debug -t bdwind-gstreamer:debug .

# 运行调试容器
docker run -it --privileged \
    -v /tmp/.X11-unix:/tmp/.X11-unix \
    -e DISPLAY=:0 \
    -p 8080:8080 \
    -p 9090:9090 \
    bdwind-gstreamer:debug

# 进入容器调试
docker exec -it container_id bash
```

#### 6.2 Kubernetes 调试

```yaml
# debug-pod.yaml - 调试 Pod
apiVersion: v1
kind: Pod
metadata:
  name: gstreamer-debug
spec:
  containers:
  - name: gstreamer
    image: bdwind-gstreamer:debug
    env:
    - name: GST_DEBUG
      value: "4"
    - name: LOG_LEVEL
      value: "debug"
    resources:
      requests:
        memory: "512Mi"
        cpu: "500m"
      limits:
        memory: "2Gi"
        cpu: "2000m"
    volumeMounts:
    - name: debug-logs
      mountPath: /var/log/gstreamer
  volumes:
  - name: debug-logs
    emptyDir: {}
```

```bash
# 部署调试 Pod
kubectl apply -f debug-pod.yaml

# 查看日志
kubectl logs -f gstreamer-debug

# 进入 Pod 调试
kubectl exec -it gstreamer-debug -- bash

# 端口转发
kubectl port-forward gstreamer-debug 8080:8080
```

### 7. 自动化测试和验证

#### 7.1 集成测试脚本

```bash
#!/bin/bash
# scripts/integration-test.sh - 集成测试

set -e

TEST_DIR="test-results"
mkdir -p "$TEST_DIR"

echo "=== GStreamer Integration Test ==="

# 1. 启动应用程序
echo "Starting application..."
./scripts/start-debug.sh &
APP_PID=$!

# 等待启动
sleep 10

# 2. 健康检查
echo "Performing health check..."
if ! curl -f http://localhost:8080/api/v1/health; then
    echo "Health check failed"
    kill $APP_PID
    exit 1
fi

# 3. 功能测试
echo "Testing video stream..."
timeout 30 curl -s http://localhost:8080/api/v1/statistics | \
    jq '.video.frame_rate' > "$TEST_DIR/framerate.txt"

if [ $(cat "$TEST_DIR/framerate.txt") == "null" ]; then
    echo "No video frames detected"
    kill $APP_PID
    exit 1
fi

# 4. 性能测试
echo "Performance test..."
for i in {1..10}; do
    curl -s http://localhost:8080/api/v1/statistics | \
        jq '.video.frame_rate' >> "$TEST_DIR/performance.txt"
    sleep 1
done

# 5. 内存泄漏检查
echo "Memory leak check..."
INITIAL_MEMORY=$(ps -p $APP_PID -o rss= | tr -d ' ')
sleep 30
FINAL_MEMORY=$(ps -p $APP_PID -o rss= | tr -d ' ')

MEMORY_GROWTH=$((FINAL_MEMORY - INITIAL_MEMORY))
if [ $MEMORY_GROWTH -gt 10000 ]; then  # 10MB threshold
    echo "Potential memory leak detected: ${MEMORY_GROWTH}KB growth"
    kill $APP_PID
    exit 1
fi

# 6. 清理
echo "Cleaning up..."
kill $APP_PID
wait $APP_PID 2>/dev/null || true

echo "All tests passed!"
```

#### 7.2 性能基准测试

```bash
#!/bin/bash
# scripts/benchmark.sh - 性能基准测试

BENCHMARK_DIR="benchmarks"
mkdir -p "$BENCHMARK_DIR"

echo "=== Performance Benchmark ==="

# 启动应用程序
./scripts/start-debug.sh &
APP_PID=$!
sleep 10

# 基准测试配置
RESOLUTIONS=("1280x720" "1920x1080" "2560x1440")
BITRATES=(1000000 2000000 4000000)
FRAMERATES=(15 30 60)

for resolution in "${RESOLUTIONS[@]}"; do
    for bitrate in "${BITRATES[@]}"; do
        for framerate in "${FRAMERATES[@]}"; do
            echo "Testing: ${resolution} @ ${framerate}fps, ${bitrate}bps"
            
            # 更新配置
            CONFIG_FILE="$BENCHMARK_DIR/config-${resolution}-${framerate}-${bitrate}.yaml"
            sed -e "s/width: .*/width: ${resolution%x*}/" \
                -e "s/height: .*/height: ${resolution#*x}/" \
                -e "s/framerate: .*/framerate: ${framerate}/" \
                -e "s/bitrate: .*/bitrate: ${bitrate}/" \
                examples/debug_config.yaml > "$CONFIG_FILE"
            
            # 重载配置
            curl -X PUT http://localhost:8080/api/v1/config \
                 -H "Content-Type: application/yaml" \
                 --data-binary @"$CONFIG_FILE"
            
            sleep 5
            
            # 收集性能数据
            RESULT_FILE="$BENCHMARK_DIR/result-${resolution}-${framerate}-${bitrate}.json"
            curl -s http://localhost:8080/api/v1/statistics > "$RESULT_FILE"
            
            # 提取关键指标
            ACTUAL_FPS=$(jq -r '.video.frame_rate' "$RESULT_FILE")
            ACTUAL_BITRATE=$(jq -r '.video.current_bitrate' "$RESULT_FILE")
            CPU_USAGE=$(ps -p $APP_PID -o pcpu= | tr -d ' ')
            MEMORY_USAGE=$(ps -p $APP_PID -o rss= | tr -d ' ')
            
            echo "Results: FPS=${ACTUAL_FPS}, Bitrate=${ACTUAL_BITRATE}, CPU=${CPU_USAGE}%, Memory=${MEMORY_USAGE}KB"
            
            # 记录到 CSV
            echo "${resolution},${framerate},${bitrate},${ACTUAL_FPS},${ACTUAL_BITRATE},${CPU_USAGE},${MEMORY_USAGE}" >> \
                "$BENCHMARK_DIR/benchmark-results.csv"
        done
    done
done

# 清理
kill $APP_PID
wait $APP_PID 2>/dev/null || true

echo "Benchmark completed. Results in $BENCHMARK_DIR/"
```

这个完整的故障排除和调试指南为开发者提供了全面的工具和方法来诊断、解决和预防 GStreamer 组件的各种问题。通过遵循这些指导原则和使用提供的工具，可以确保系统的稳定性和性能。
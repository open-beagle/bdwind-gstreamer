# X11 桌面捕获实现

本文档描述了 BDWind-GStreamer 项目中的 X11 桌面捕获实现。

## 概述

X11 桌面捕获模块为运行 X11 显示服务器的 Linux 系统提供高性能屏幕捕获功能。它与 GStreamer 的 `ximagesrc` 元素集成，捕获桌面内容，支持：

- 多显示器环境
- 基于区域的捕获
- 帧率和分辨率控制
- 硬件优化功能
- 实时统计

## 功能特性

### 1. GStreamer ximagesrc 集成

实现使用 GStreamer 的 `ximagesrc` 元素进行高效的 X11 屏幕捕获：

```go
// 创建 ximagesrc 元素
sourceElem, err := pipeline.GetElement("ximagesrc")
```

配置的关键属性：
- `display-name`：X11 显示标识符（例如 ":0"）
- `use-damage`：启用损坏事件优化
- `show-pointer`：控制鼠标指针可见性
- `startx`、`starty`、`endx`、`endy`：定义捕获区域

### 2. 分辨率和帧率设置

捕获管道支持动态分辨率和帧率配置：

```go
// 为分辨率和帧率配置 caps
capsStr := fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1", 
    width, height, framerate)
```

支持的分辨率：
- 最高 7680x4320（8K）
- 常见格式：1920x1080、1280x720、3840x2160
- 支持自定义分辨率

帧率支持：
- 1-120 fps 范围
- 基于系统能力的自适应帧率
- 硬件优化的丢帧

### 3. 多显示器支持

实现支持多显示器 X11 环境：

```go
// 显示格式：":display.screen"
// 示例：
// ":0" 或 ":0.0" - 主显示器
// ":0.1" - 第二个显示器
// ":0.2" - 第三个显示器
```

功能：
- 自动显示检测
- 特定屏幕捕获
- 显示验证
- 每个显示器的最佳设置检测

### 4. 区域捕获

捕获屏幕的特定区域而不是整个显示：

```go
region := &config.Rect{
    X:      100,    // 起始 X 坐标
    Y:      100,    // 起始 Y 坐标
    Width:  800,    // 区域宽度
    Height: 600,    // 区域高度
}
```

使用场景：
- 应用程序窗口捕获
- 特定桌面区域捕获
- 较小区域的性能优化
- 通过排除敏感区域进行隐私保护

## 配置

### 基本配置

```yaml
display_id: ":0"           # X11 显示标识符
frame_rate: 30             # 捕获帧率（fps）
width: 1920                # 捕获宽度（像素）
height: 1080               # 捕获高度（像素）
use_wayland: false         # 使用 X11（不是 Wayland）
show_pointer: true         # 显示鼠标指针
use_damage: true           # 使用损坏事件
quality: "high"            # 质量预设
```

### 高级配置

```yaml
# 多显示器设置
display_id: ":0.1"         # 第二个显示器

# 区域捕获
capture_region:
  x: 100
  y: 100
  width: 800
  height: 600

# 性能调优
buffer_size: 10            # 缓冲区大小
use_damage: true           # 损坏事件优化
quality: "medium"          # 平衡质量/性能
```

## API 使用

### 基本使用

```go
import "github.com/open-beagle/bdwind-gstreamer/internal/gstreamer"

// 创建配置
cfg := config.DefaultDesktopCaptureConfig()
cfg.DisplayID = ":0"
cfg.Width = 1920
cfg.Height = 1080
cfg.FrameRate = 30

// 创建捕获实例
capture, err := gstreamer.NewDesktopCapture(cfg)
if err != nil {
    log.Fatal(err)
}

// 开始捕获
err = capture.Start()
if err != nil {
    log.Fatal(err)
}

// 获取统计信息
stats := capture.GetStats()
fmt.Printf("FPS: %.1f, 帧数: %d\n", stats.CurrentFPS, stats.FramesCapture)

// 停止捕获
capture.Stop()
```

### 显示检测

```go
// 列出可用显示
displays, err := gstreamer.GetX11Displays()
for _, display := range displays {
    fmt.Printf("显示: %s.%d (%dx%d @ %.1f Hz)\n",
        display.DisplayName, display.ScreenNum,
        display.Width, display.Height, display.RefreshRate)
}

// 验证显示
err = gstreamer.ValidateX11Display(":0")
if err != nil {
    log.Fatal("显示不可用")
}

// 获取最佳设置
cfg, err := gstreamer.GetOptimalX11CaptureSettings(":0")
```

### 区域捕获

```go
// 创建捕获区域
region := gstreamer.CreateX11CaptureRegion(100, 100, 800, 600)
cfg.CaptureRegion = region

// 基于窗口的区域（未来增强的占位符）
region, err := gstreamer.CreateX11WindowCaptureRegion("window-id")
```

## 性能优化

### 1. 损坏事件

启用损坏事件优化以获得更好的性能：

```go
cfg.UseDamage = true
```

好处：
- 当屏幕内容静态时减少 CPU 使用
- 改善变化最小的应用程序的性能
- 基于屏幕活动的自动优化

### 2. 质量预设

根据使用场景选择适当的质量预设：

- `"low"`：RGB 格式，最佳性能
- `"medium"`：BGRx 格式，平衡（默认）
- `"high"`：BGRA 格式，最佳质量

### 3. 帧率控制

使用 videorate 元素进行智能丢帧：

```go
// 在管道中自动配置
videoRate.SetProperty("drop-only", true)
```

### 4. 缓冲区管理

根据系统能力配置缓冲区大小：

```go
cfg.BufferSize = 10  // 根据内存约束调整
```

## 管道架构

X11 捕获管道包含：

```
ximagesrc → videoconvert → videoscale → videorate → capsfilter
```

1. **ximagesrc**：捕获 X11 屏幕内容
2. **videoconvert**：处理格式转换
3. **videoscale**：如需要则缩放分辨率
4. **videorate**：控制帧率并丢帧
5. **capsfilter**：强制执行最终格式约束

## 错误处理

常见错误和解决方案：

### 显示不可用
```
错误：未找到显示 :0
解决方案：检查 DISPLAY 环境变量，验证 X11 正在运行
```

### 权限被拒绝
```
错误：无法访问 X11 显示
解决方案：检查 X11 权限，如需要运行 xhost +local:
```

### 性能问题
```
错误：高 CPU 使用率，丢帧
解决方案：启用损坏事件，降低分辨率/帧率，使用较低质量预设
```

## 测试

运行 X11 特定测试：

```bash
# 测试 X11 功能
go test -v ./internal/gstreamer -run TestX11

# 测试桌面捕获
go test -v ./internal/gstreamer -run TestDesktopCapture

# 运行演示
go run cmd/x11-capture-demo/main.go -display=:0 -duration=5s
```

## 演示使用

X11 捕获演示提供各种选项：

```bash
# 基本捕获
./x11-capture-demo -display=:0 -duration=10s

# 带指针的高质量捕获
./x11-capture-demo -display=:0 -quality=high -pointer=true

# 区域捕获
./x11-capture-demo -region=100,100,800,600

# 多显示器捕获
./x11-capture-demo -display=:0.1

# 列出可用显示
./x11-capture-demo -list
```

## 未来增强

计划的改进：

1. **特定窗口捕获**：直接窗口 ID 支持
2. **动态区域调整**：运行时区域修改
3. **硬件加速**：基于 GPU 的捕获优化
4. **高级损坏跟踪**：更高效的变化检测
5. **光标自定义**：自定义光标渲染选项

## 要求

- X11 显示服务器
- GStreamer 1.0+ 带 ximagesrc 插件
- 适当的 X11 权限
- 带有 X11 开发库的 Linux 系统

## 故障排除

### 常见问题

1. **未检测到显示**：确保设置了 DISPLAY 环境变量
2. **权限错误**：检查 X11 访问权限
3. **性能差**：启用损坏事件，降低质量/分辨率
4. **内存问题**：调整缓冲区大小，检查内存泄漏

### 调试信息

启用调试日志：

```bash
export GST_DEBUG=ximagesrc:5
./x11-capture-demo
```

这提供了详细的 GStreamer 管道信息用于故障排除。
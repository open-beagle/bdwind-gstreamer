# Wayland 桌面捕获实现

本文档描述了 BDWind-GStreamer 项目中 Wayland 桌面捕获功能的实现。

## 概述

Wayland 桌面捕获实现为基于 Wayland 的 Linux 桌面环境提供高性能屏幕捕获功能。它集成了 GStreamer 的 `waylandsrc` 元素来捕获桌面内容，并支持各种 Wayland 合成器，包括 GNOME、KDE 和 Sway。

## 功能特性

### 核心功能
- **全屏捕获**：捕获整个桌面
- **区域捕获**：捕获特定矩形区域
- **多显示器支持**：支持多个 Wayland 输出
- **帧率控制**：可配置的捕获帧率（1-120 fps）
- **分辨率控制**：可配置的捕获分辨率
- **质量预设**：低、中、高质量设置
- **指针捕获**：可选的鼠标指针包含

### Wayland 特定功能
- **合成器检测**：自动检测 Wayland 合成器
- **能力检测**：查询合成器能力
- **输出选择**：支持多个 Wayland 输出
- **缩放因子处理**：正确处理 HiDPI 显示
- **基于裁剪的区域**：使用裁剪属性的原生 Wayland 区域捕获

## 架构

### 组件

1. **桌面捕获接口** (`DesktopCapture`)
   - X11 和 Wayland 捕获的统一接口
   - 配置驱动的捕获设置
   - 统计收集和报告

2. **Wayland 源配置**
   - `waylandsrc` GStreamer 元素集成
   - Wayland 特定属性映射
   - 输出和显示管理

3. **管道管理**
   - GStreamer 管道创建和管理
   - 元素链接和配置
   - 状态管理和错误处理

4. **配置系统**
   - 基于 YAML 的配置文件
   - 环境变量支持
   - 自动检测能力

## 配置

### 基本配置

```yaml
# Wayland 桌面捕获配置
display_id: "wayland-0"   # Wayland 显示 ID
frame_rate: 30            # 捕获帧率（1-120 fps）
width: 1920               # 捕获宽度
height: 1080              # 捕获高度
use_wayland: true         # 启用 Wayland 模式
auto_detect: true         # 自动检测显示服务器
show_pointer: false       # 显示鼠标指针
quality: "medium"         # 质量预设（low、medium、high）
buffer_size: 10           # 缓冲区大小
```

### 区域捕获

```yaml
capture_region:
  x: 100                  # X 偏移（crop-x）
  y: 100                  # Y 偏移（crop-y）
  width: 800              # 区域宽度（crop-width）
  height: 600             # 区域高度（crop-height）
```

### 多显示器配置

```yaml
# 主输出
display_id: "wayland-0"

# 辅助输出
display_id: "wayland-1"

# 第三输出
display_id: "wayland-2"
```

## GStreamer 管道

### 基本管道结构

```
waylandsrc -> videoconvert -> videoscale -> videorate -> capsfilter
```

### Wayland 源属性

| 属性 | 类型 | 描述 |
|------|------|------|
| `display` | string | Wayland 显示名称 |
| `show-pointer` | boolean | 显示鼠标指针 |
| `crop-x` | int | 裁剪区域 X 偏移 |
| `crop-y` | int | 裁剪区域 Y 偏移 |
| `crop-width` | int | 裁剪区域宽度 |
| `crop-height` | int | 裁剪区域高度 |
| `output` | string | 输出选择（依赖合成器） |

### Caps 配置

caps 过滤器根据质量预设配置：

- **低质量**：`video/x-raw,format=RGB`
- **中等质量**：`video/x-raw,format=BGRx`
- **高质量**：`video/x-raw,format=BGRA`

## API 参考

### 核心函数

#### `NewDesktopCapture(cfg config.DesktopCaptureConfig) (DesktopCapture, error)`
使用指定配置创建新的桌面捕获实例。

#### `GetWaylandDisplays() ([]WaylandDisplayInfo, error)`
返回可用 Wayland 显示的信息。

#### `ValidateWaylandDisplay(displayID string) error`
验证指定的 Wayland 显示是否可用。

#### `GetOptimalWaylandCaptureSettings(displayID string) (config.DesktopCaptureConfig, error)`
返回指定 Wayland 显示的最佳捕获设置。

### Wayland 特定函数

#### `DetectWaylandCompositor() (string, error)`
检测当前运行的 Wayland 合成器。

#### `GetWaylandCompositorCapabilities() (map[string]bool, error)`
返回当前 Wayland 合成器的能力。

#### `CreateWaylandCaptureRegion(x, y, width, height int) *config.Rect`
为 Wayland 捕获创建捕获区域。

### 数据结构

#### `WaylandDisplayInfo`
```go
type WaylandDisplayInfo struct {
    DisplayName string  // 显示名称（例如 "wayland-0"）
    OutputName  string  // 输出名称（例如 "output-0"）
    Width       int     // 显示宽度
    Height      int     // 显示高度
    RefreshRate float64 // 刷新率（Hz）
    Scale       float64 // HiDPI 缩放因子
}
```

## 合成器支持

### GNOME (Mutter)
- **屏幕捕获**：✅ 完全支持
- **窗口捕获**：✅ 支持
- **区域捕获**：✅ 支持
- **多输出**：✅ 支持
- **指针捕获**：✅ 支持

### KDE (KWin)
- **屏幕捕获**：✅ 完全支持
- **窗口捕获**：✅ 支持
- **区域捕获**：✅ 支持
- **多输出**：✅ 支持
- **指针捕获**：✅ 支持

### Sway
- **屏幕捕获**：✅ 完全支持
- **窗口捕获**：⚠️ 有限支持
- **区域捕获**：✅ 支持
- **多输出**：✅ 支持
- **指针捕获**：✅ 支持

### 其他合成器
- **屏幕捕获**：✅ 基本支持
- **窗口捕获**：❌ 不支持
- **区域捕获**：❌ 不支持
- **多输出**：❌ 不支持
- **指针捕获**：✅ 支持

## 使用示例

### 基本捕获

```go
// 创建配置
cfg := config.DefaultDesktopCaptureConfig()
cfg.UseWayland = true
cfg.DisplayID = "wayland-0"

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
fmt.Printf("FPS: %.2f\n", stats.CurrentFPS)

// 停止捕获
capture.Stop()
```

### 区域捕获

```go
cfg := config.DefaultDesktopCaptureConfig()
cfg.UseWayland = true
cfg.DisplayID = "wayland-0"
cfg.CaptureRegion = &config.Rect{
    X:      100,
    Y:      100,
    Width:  800,
    Height: 600,
}

capture, err := gstreamer.NewDesktopCapture(cfg)
// ... 其余代码
```

### 多显示器捕获

```go
// 获取可用显示
displays, err := gstreamer.GetWaylandDisplays()
if err != nil {
    log.Fatal(err)
}

// 从第二个显示器捕获
cfg := config.DefaultDesktopCaptureConfig()
cfg.UseWayland = true
cfg.DisplayID = displays[1].DisplayName

capture, err := gstreamer.NewDesktopCapture(cfg)
// ... 其余代码
```

## 命令行工具

### Wayland 捕获演示

`wayland-capture-demo` 工具提供了测试 Wayland 捕获功能的命令行界面。

#### 基本使用

```bash
# 列出可用显示
./wayland-capture-demo -list-displays

# 检测合成器
./wayland-capture-demo -detect-compositor

# 显示合成器能力
./wayland-capture-demo -show-caps

# 基本捕获
./wayland-capture-demo -display wayland-0 -framerate 30

# 区域捕获
./wayland-capture-demo -region-x 100 -region-y 100 -region-width 800 -region-height 600

# 带指针的高质量捕获
./wayland-capture-demo -quality high -pointer
```

#### 配置文件

```bash
# 使用配置文件
./wayland-capture-demo -config examples/wayland_desktop_capture_config.yaml
```

## 测试

### 单元测试

实现包含全面的单元测试，涵盖：

- Wayland 显示检测和验证
- 配置解析和验证
- 捕获区域创建和验证
- 合成器检测和能力查询
- GStreamer 管道配置

### 运行测试

```bash
# 运行所有 Wayland 相关测试
go test -v ./internal/gstreamer -run "TestWayland"

# 运行特定测试
go test -v ./internal/gstreamer -run "TestWaylandCaptureConfiguration"

# 运行所有桌面捕获测试
go test -v ./internal/gstreamer -run "TestDesktopCapture"
```

## 性能考虑

### 优化提示

1. **使用硬件加速**（如果可用）
2. **根据使用场景选择适当的质量预设**
3. **限制捕获区域**以减少处理开销
4. **根据网络条件调整帧率**
5. **使用损坏事件**（如果合成器支持）

### 性能指标

实现提供详细的性能指标：

- **帧率**：当前和平均 FPS
- **帧统计**：捕获和丢弃的帧
- **延迟**：捕获延迟（毫秒）
- **资源使用**：CPU 和内存利用率

## 故障排除

### 常见问题

#### 1. 未找到 Wayland 显示
```
错误：未找到 Wayland 显示 wayland-0
```
**解决方案**：使用 `-list-displays` 标志检查可用显示。

#### 2. 合成器不支持
```
警告：设置输出属性失败（可能不支持）
```
**解决方案**：这对某些合成器是预期的。基本捕获应该仍然有效。

#### 3. 权限被拒绝
```
错误：创建 waylandsrc 元素失败
```
**解决方案**：确保应用程序具有屏幕捕获权限。

#### 4. 性能低下
```
统计 - FPS: 5.2，丢弃：150
```
**解决方案**：降低分辨率、帧率或质量预设。

### 调试信息

启用调试日志以获取详细信息：

```bash
export GST_DEBUG=waylandsrc:5
./wayland-capture-demo -display wayland-0
```

## 安全考虑

### 权限

Wayland 捕获需要适当的权限：
- 来自合成器的屏幕捕获权限
- 访问 Wayland 显示套接字
- 屏幕共享的潜在用户确认

### 隐私

实现尊重 Wayland 的安全模型：
- 没有权限不能访问其他应用程序的内容
- 合成器中介的屏幕捕获
- 活动屏幕捕获的用户通知

## 未来增强

### 计划功能

1. **特定窗口捕获**：增强的窗口捕获支持
2. **动态区域更新**：运行时区域修改
3. **合成器特定优化**：每个合成器的定制优化
4. **硬件加速**：GPU 加速捕获（如果可用）
5. **音频捕获**：同步音频捕获支持

### 贡献

要为 Wayland 桌面捕获实现做出贡献：

1. 遵循现有的代码风格和模式
2. 为新功能添加全面的测试
3. 更新 API 更改的文档
4. 使用多个 Wayland 合成器进行测试
5. 考虑安全和性能影响

## 参考资料

- [Wayland 协议文档](https://wayland.freedesktop.org/docs/html/)
- [GStreamer Wayland 插件](https://gstreamer.freedesktop.org/documentation/wayland/)
- [Wayland 屏幕捕获协议](https://github.com/wayland-project/wayland-protocols)
- [桌面捕获安全](https://wiki.gnome.org/Projects/Mutter/RemoteDesktop)
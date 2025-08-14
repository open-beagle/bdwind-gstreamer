# 桌面捕获配置

本包为桌面捕获功能提供全面的配置管理，支持环境变量、配置文件和验证。

## 功能特性

- **默认配置**：所有配置选项的合理默认值
- **环境变量支持**：从带有 `BDWIND_CAPTURE_` 前缀的环境变量加载配置
- **配置文件**：支持 YAML 配置文件
- **验证**：所有配置参数的全面验证
- **自动检测**：自动检测显示服务器（X11/Wayland）

## 配置选项

### 基本设置

- `display_id`：显示 ID（例如 ":0"）- 默认为 ":0"
- `frame_rate`：捕获帧率（1-120 fps）- 默认为 30
- `width`：捕获宽度（1-7680 像素）- 默认为 1920
- `height`：捕获高度（1-4320 像素）- 默认为 1080

### 显示服务器设置

- `use_wayland`：使用 Wayland 而不是 X11 - 默认为 false
- `auto_detect`：自动检测显示服务器 - 默认为 true

### 捕获选项

- `show_pointer`：在捕获中显示鼠标指针 - 默认为 false
- `use_damage`：使用损坏事件进行优化 - 默认为 false
- `buffer_size`：缓冲区大小（1-100）- 默认为 10
- `quality`：质量预设（"low"、"medium"、"high"）- 默认为 "medium"

### 捕获区域（可选）

- `capture_region.x`：区域捕获的 X 偏移
- `capture_region.y`：区域捕获的 Y 偏移
- `capture_region.width`：捕获区域宽度
- `capture_region.height`：捕获区域高度

## 环境变量

所有配置选项都可以通过带有 `BDWIND_CAPTURE_` 前缀的环境变量设置：

- `BDWIND_CAPTURE_DISPLAY_ID`
- `BDWIND_CAPTURE_FRAME_RATE`
- `BDWIND_CAPTURE_WIDTH`
- `BDWIND_CAPTURE_HEIGHT`
- `BDWIND_CAPTURE_USE_WAYLAND`
- `BDWIND_CAPTURE_AUTO_DETECT`
- `BDWIND_CAPTURE_SHOW_POINTER`
- `BDWIND_CAPTURE_USE_DAMAGE`
- `BDWIND_CAPTURE_BUFFER_SIZE`
- `BDWIND_CAPTURE_QUALITY`
- `BDWIND_CAPTURE_REGION`（格式："x,y,width,height"）

## 使用示例

### 使用默认配置

```go
import "github.com/open-beagle/bdwind-gstreamer/internal/config"

// 获取默认配置
cfg := config.DefaultDesktopCaptureConfig()

// 验证配置
if err := config.ValidateDesktopCaptureConfig(&cfg); err != nil {
    log.Fatal(err)
}
```

### 从环境变量加载

```go
// 从环境变量加载配置
cfg := config.LoadDesktopCaptureConfigFromEnv()
```

### 从配置文件加载

```go
// 从 YAML 文件加载配置
cfg, err := config.LoadDesktopCaptureConfigFromFile("config.yaml")
if err != nil {
    log.Fatal(err)
}
```

### 保存配置到文件

```go
// 将配置保存到 YAML 文件
cfg := config.DefaultDesktopCaptureConfig()
cfg.FrameRate = 60
cfg.Quality = "high"

err := config.SaveDesktopCaptureConfigToFile(cfg, "config.yaml")
if err != nil {
    log.Fatal(err)
}
```

### 显示服务器检测

```go
// 检测系统是否使用 Wayland
isWayland, err := config.DetectDisplayServer()
if err != nil {
    log.Fatal(err)
}

if isWayland {
    fmt.Println("使用 Wayland")
} else {
    fmt.Println("使用 X11")
}
```

## 配置文件格式

配置文件使用 YAML 格式：

```yaml
display_id: ":0"
frame_rate: 30
width: 1920
height: 1080
use_wayland: false
auto_detect: true
show_pointer: false
use_damage: false
buffer_size: 10
quality: "medium"
capture_region:
  x: 100
  y: 100
  width: 800
  height: 600
```

## 验证

配置验证确保：

- 帧率在 1 到 120 fps 之间
- 宽度在 1 到 7680 像素之间
- 高度在 1 到 4320 像素之间
- 缓冲区大小在 1 到 100 之间
- 质量是以下之一："low"、"medium"、"high"
- 捕获区域尺寸为正数
- 捕获区域位置为非负数

## 测试

包包含全面的测试，涵盖：

- 默认配置生成
- 配置验证
- 环境变量加载
- 文件加载和保存
- 显示服务器检测

运行测试：

```bash
go test -v ./internal/config
```

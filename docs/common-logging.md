# BDWind-GStreamer 日志系统

BDWind-GStreamer 使用 [logrus](https://github.com/sirupsen/logrus) 作为日志框架，提供了灵活的日志等级管理和配置选项。

## 日志等级

支持以下日志等级（从低到高）：

- **TRACE**: 最详细的日志信息，用于深度调试
- **DEBUG**: 调试信息，开发时使用
- **INFO**: 一般信息，默认等级
- **WARN**: 警告信息，需要注意但不影响正常运行
- **ERROR**: 错误信息，影响功能但不会导致程序崩溃

## 配置方式

### 1. 配置文件

在 YAML 配置文件中添加 `logging` 部分：

```yaml
logging:
  level: "info" # 日志等级
  format: "text" # 日志格式: text, json
  output: "stdout" # 输出目标: stdout, stderr, file
  file: "/var/log/app.log" # 日志文件路径 (output为file时)
  enable_timestamp: true # 启用时间戳
  enable_caller: false # 启用调用者信息
  enable_colors: true # 启用颜色输出
```

### 2. 命令行参数

```bash
# 设置日志等级
./bdwind-gstreamer --log-level debug

# 设置输出到文件
./bdwind-gstreamer --log-output file --log-file /tmp/app.log

# 设置输出到stderr
./bdwind-gstreamer --log-output stderr
```

### 3. 环境变量

```bash
# 设置日志等级
export BDWIND_LOG_LEVEL=debug

# 设置日志格式
export BDWIND_LOG_FORMAT=json

# 设置输出目标
export BDWIND_LOG_OUTPUT=file
export BDWIND_LOG_FILE=/var/log/bdwind.log

# 其他选项
export BDWIND_LOG_TIMESTAMP=true
export BDWIND_LOG_CALLER=true
export BDWIND_LOG_COLORS=false
```

## 运行时日志等级调整

日志等级可以在运行时动态调整：

```go
import "github.com/open-beagle/bdwind-gstreamer/internal/config"

// 设置全局日志等级
config.SetGlobalLogLevel("debug")

// 获取当前日志等级
level := config.GetGlobalLogLevel()
```

## 组件日志

每个组件都有自己的日志前缀，便于区分：

- `[app]`: 应用主程序
- `[webrtc]`: WebRTC 管理器
- `[gstreamer]`: GStreamer 管理器
- `[webserver]`: Web 服务器
- `[metrics]`: 监控系统

## 日志格式示例

### 文本格式 (默认)

```
2024-01-15 10:30:45.123 [INFO] [app] Starting BDWind-GStreamer v1.0.0
2024-01-15 10:30:45.124 [INFO] [webserver] Starting webserver manager...
2024-01-15 10:30:45.125 [DEBUG] [gstreamer] Initializing GStreamer pipeline
```

### JSON 格式

```json
{"level":"info","msg":"Starting BDWind-GStreamer v1.0.0","component":"app","time":"2024-01-15T10:30:45.123Z"}
{"level":"info","msg":"Starting webserver manager...","component":"webserver","time":"2024-01-15T10:30:45.124Z"}
{"level":"debug","msg":"Initializing GStreamer pipeline","component":"gstreamer","time":"2024-01-15T10:30:45.125Z"}
```

## 最佳实践

1. **生产环境**: 使用 `INFO` 或 `WARN` 等级，避免过多日志影响性能
2. **开发环境**: 使用 `DEBUG` 或 `TRACE` 等级，获取详细调试信息
3. **日志文件**: 生产环境建议输出到文件，便于日志轮转和分析
4. **JSON 格式**: 如果需要日志分析工具处理，使用 JSON 格式
5. **调用者信息**: 仅在调试时启用，会影响性能

## 故障排除

### 日志文件权限问题

```bash
# 确保应用有写入权限
sudo chown bdwind:bdwind /var/log/bdwind-gstreamer.log
sudo chmod 644 /var/log/bdwind-gstreamer.log
```

### 日志等级不生效

1. 检查配置文件语法是否正确
2. 确认命令行参数拼写正确
3. 环境变量优先级：命令行 > 配置文件 > 环境变量 > 默认值

### 性能问题

1. 避免在生产环境使用 TRACE 或 DEBUG 等级
2. 考虑使用异步日志（未来版本可能支持）
3. 定期轮转日志文件，避免文件过大

# 文件重复问题说明

## 问题描述

之前存在两个几乎相同的配置文件：
- `internal/config/desktop_capture_config.go` 
- `internal/gstreamer/desktop_capture_config.go` (已删除)

## 产生原因

### 1. CGO 编译问题
- 在实现过程中，`internal/gstreamer` 包由于包含 CGO 代码导致编译错误
- GStreamer 的 C 绑定存在头文件和函数声明问题
- 这导致整个 gstreamer 包无法正常编译和测试

### 2. 解决方案演进
1. **第一阶段**: 在 `internal/gstreamer` 包中直接实现配置功能
2. **遇到问题**: CGO 编译错误阻止了测试和验证
3. **第二阶段**: 创建独立的 `internal/config` 包，不依赖 CGO
4. **第三阶段**: 保留了两个文件，造成重复

### 3. 架构决策
最终决定采用分离架构：
- **`internal/config`**: 纯 Go 实现，负责配置管理，无 CGO 依赖
- **`internal/gstreamer`**: 专注于 GStreamer 集成，使用 config 包

## 最终解决方案

### 保留的文件
- ✅ `internal/config/desktop_capture_config.go` - 主要配置实现
- ✅ `internal/config/desktop_capture_config_test.go` - 配置测试
- ✅ `internal/gstreamer/desktop_capture.go` - 使用 config 包的桌面捕获实现

### 删除的文件
- ❌ `internal/gstreamer/desktop_capture_config.go` - 重复的配置实现

## 优势

### 1. 模块化设计
- 配置逻辑与 GStreamer 实现分离
- 可以独立测试配置功能
- 降低了代码耦合度

### 2. 避免 CGO 问题
- 配置功能不受 GStreamer CGO 问题影响
- 可以在没有 GStreamer 开发环境的机器上测试配置
- 提高了开发效率

### 3. 更好的可维护性
- 单一职责原则：config 包只负责配置
- 更容易进行单元测试
- 更清晰的依赖关系

## 当前架构

```
internal/
├── config/                          # 配置管理包 (纯 Go)
│   ├── desktop_capture_config.go    # 配置实现
│   ├── desktop_capture_config_test.go # 配置测试
│   └── README.md                    # 配置文档
└── gstreamer/                       # GStreamer 集成包
    ├── desktop_capture.go           # 桌面捕获实现 (使用 config 包)
    ├── desktop_capture_test.go      # 桌面捕获测试
    ├── gstreamer.go                 # GStreamer 绑定
    ├── element.go                   # GStreamer 元素
    ├── pipeline.go                  # GStreamer 管道
    └── events.go                    # GStreamer 事件
```

这种架构确保了配置功能的稳定性和可测试性，同时为后续的 GStreamer 集成提供了清晰的接口。
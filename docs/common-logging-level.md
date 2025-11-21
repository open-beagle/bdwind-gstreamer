# 日志等级选择指南

## 概述

本文档提供了在 Go 应用程序中选择合适日志等级的指导原则，基于对常见日志模式和最佳实践的分析。目标是确保日志提供有意义的信息，同时不在生产环境中产生噪音。

## 日志等级定义

### TRACE
- **用途**: 极其详细的诊断信息
- **使用场景**: 逐步执行流程，进入/退出函数
- **生产环境**: 通常禁用
- **性能影响**: 高

### DEBUG  
- **用途**: 用于调试的详细信息
- **使用场景**: 变量值，配置详情，决策点
- **生产环境**: 通常禁用，可能在故障排除时启用
- **性能影响**: 中等

### INFO
- **用途**: 关于应用程序流程的一般信息
- **使用场景**: 重要的状态变化，重要操作的成功完成
- **生产环境**: 启用
- **性能影响**: 低

### WARN
- **用途**: 可能有害但不阻止操作的情况
- **使用场景**: 可恢复的错误，废弃用法，配置问题
- **生产环境**: 启用
- **性能影响**: 很低

### ERROR
- **用途**: 可能仍允许应用程序继续运行的错误事件
- **使用场景**: 异常，操作失败，无效输入
- **生产环境**: 始终启用
- **性能影响**: 最小

## 按操作类型分类的指南

### 应用程序启动和初始化

#### TRACE 级别
- 单个中间件设置步骤
- 路由注册详情
- 组件初始化步骤
- 配置参数加载

**基于 routes.go 分析的示例:**
```go
logger.Trace("Setting up middleware...")
logger.Trace("Setting up basic routes...")
logger.Trace("Setting up auth routes...")
logger.Trace("Setting up static routes...")
```

#### DEBUG 级别
- 主要初始化阶段
- 组件设置完成
- 配置验证结果

**示例:**
```go
logger.Debug("Setting up webserver routes...")
logger.Debug("Component routes setup completed successfully")
```

#### INFO 级别
- 应用程序启动完成
- 服务可用性公告
- 主要功能启用

**示例:**
```go
logger.Info("Web server started on port 8080")
logger.Info("Authentication enabled with JWT")
```

### 请求处理

#### TRACE 级别
- 请求路由决策
- 中间件链执行
- 参数提取和验证

#### DEBUG 级别
- 请求处理开始/完成
- 缓存命中/未命中
- 业务逻辑决策点

#### INFO 级别
- 重要的业务事件
- 用户认证成功
- 重要状态变化

### 错误处理

#### WARN 级别
- 可恢复的错误
- 回退机制激活
- 无效但可处理的输入
- 资源约束（但不是失败）

**基于 routes.go 分析的示例:**
```go
logger.Warn("Reserved path accessed but no handler found: %s", path)
logger.Warn("Default file not found for SPA fallback, using 404")
```

#### ERROR 级别
- 操作失败
- 系统错误
- 安全违规
- 数据损坏

**示例:**
```go
logger.Error("Component routes setup failed: %v", err)
logger.Error("Permission denied for file: %s", path)
```

## 常见反模式

### INFO 级别过度记录
**问题**: 例行操作记录为 INFO 会产生噪音
```go
// 错误
logger.Info("Setting up middleware...")
logger.Info("Processing request for /api/status")

// 正确  
logger.Debug("Setting up middleware...")
logger.Trace("Processing request for /api/status")
```

### 重要事件记录不足
**问题**: 重要事件没有适当记录
```go
// 错误
logger.Debug("User authentication successful")

// 正确
logger.Info("User authentication successful for user: %s", userID)
```

### 错误级别不一致
**问题**: 类似错误在不同级别记录
```go
// 错误
logger.Error("File not found: %s", path)  // 这可能是预期的
logger.Warn("Database connection failed") // 这很严重

// 正确
logger.Debug("File not found, trying fallback: %s", path)
logger.Error("Database connection failed: %v", err)
```

## 特定上下文指南

### Web 服务器路由
- 路由设置: DEBUG 或 TRACE
- 路由匹配: TRACE
- 路由完成: DEBUG
- 路由错误: ERROR 或 WARN（取决于可恢复性）

### 文件操作
- 文件访问尝试: TRACE
- 文件未找到（预期）: DEBUG
- 文件未找到（意外）: WARN
- 权限错误: ERROR
- I/O 错误: ERROR

### 认证/授权
- 登录尝试: INFO
- 登录成功: INFO
- 登录失败: WARN
- 授权检查: DEBUG
- 授权失败: WARN
- 安全违规: ERROR

### 组件生命周期
- 组件初始化开始: DEBUG
- 组件初始化成功: INFO
- 组件初始化失败: ERROR
- 组件关闭: INFO

## 生产环境考虑

### 性能影响
- TRACE/DEBUG: 可能显著影响性能
- INFO: 影响最小但要考虑数量
- WARN/ERROR: 影响可忽略

### 日志量管理
- 使用结构化日志以便更好地过滤
- 对高频事件实施日志采样
- 对性能关键路径考虑异步日志

### 监控集成
- ERROR 日志应触发警报
- WARN 日志应监控趋势
- INFO 日志提供操作可见性
- DEBUG/TRACE 仅用于故障排除

## 实施建议

### 日志消息格式
```go
// 包含上下文和可操作信息
logger.Error("Failed to process user request: userID=%s, error=%v", userID, err)

// 避免模糊消息
logger.Error("Something went wrong") // 错误
```

### 条件日志
```go
// 对于昂贵的操作
if logger.IsDebugEnabled() {
    logger.Debug("Complex calculation result: %s", expensiveOperation())
}
```

### 结构化日志
```go
logger.WithFields(map[string]interface{}{
    "userID": userID,
    "action": "login",
    "result": "success",
}).Info("User authentication completed")
```

## 日志前缀一致性指南

### 前缀命名规范

为了确保日志的可读性和可维护性，应遵循一致的前缀命名规范：

#### 组件级前缀
- 使用组件名作为基础前缀，如 `webserver`、`gstreamer`、`metrics`
- 子模块使用连字符分隔，如 `webserver-routes`、`webserver-static`

#### 避免的反模式
```go
// 错误：前缀不明确，无法确定所属组件
logger := config.GetLoggerWithPrefix("static-router")

// 正确：明确表示属于webserver组件
logger := config.GetLoggerWithPrefix("webserver-static-router")
```

#### 推荐的前缀结构
```go
// 基础组件
config.GetLoggerWithPrefix("webserver")

// 子功能模块
config.GetLoggerWithPrefix("webserver-routes")
config.GetLoggerWithPrefix("webserver-middleware") 
config.GetLoggerWithPrefix("webserver-static")
config.GetLoggerWithPrefix("webserver-auth")

// 具体功能
config.GetLoggerWithPrefix("webserver-static-router")
config.GetLoggerWithPrefix("webserver-spa-fallback")
```

### 前缀一致性检查

定期检查代码中的日志前缀，确保：
1. 同一组件内的前缀保持一致
2. 前缀能够清楚地表示功能归属
3. 避免使用模糊或容易混淆的前缀

## 结论

正确的日志级别选择对于可维护的应用程序至关重要。关键原则是：

1. **INFO 及以上级别应在生产环境中提供价值**
2. **DEBUG 和 TRACE 用于开发和故障排除**
3. **ERROR 表示需要关注的实际问题**
4. **WARN 表示潜在问题或异常情况**
5. **考虑受众**: 运维人员需要 INFO+，开发人员需要 DEBUG+
6. **保持前缀一致性**: 使用清晰、一致的日志前缀

定期审查日志模式和前缀一致性有助于维护干净、有用的日志，既支持开发又支持运维工作。
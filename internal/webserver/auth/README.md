# 访问控制模块

本模块为 BDWind-GStreamer 项目实现了一个全面的访问控制系统，提供基于令牌的认证、会话管理和基于角色的授权功能。

## 功能特性

### 1. 令牌管理 (`token.go`)

- **令牌类型**：支持访问令牌和刷新令牌
- **令牌生成**：具有可配置 TTL 的安全令牌生成
- **令牌验证**：带有过期检查的加密验证
- **令牌撤销**：能够在过期前撤销令牌
- **令牌刷新**：使用刷新令牌刷新访问令牌
- **自动清理**：清理过期令牌

#### 使用示例

```go
// 创建令牌管理器
tokenManager := auth.NewInMemoryTokenManager(nil)

// 生成访问令牌
token, tokenString, err := tokenManager.GenerateToken("user123", auth.TokenTypeAccess)
if err != nil {
    log.Fatal(err)
}

// 验证令牌
validatedToken, err := tokenManager.ValidateToken(tokenString)
if err != nil {
    log.Fatal(err)
}

// 撤销令牌
err = tokenManager.RevokeToken(token.ID)
```

### 2. 会话管理 (`session.go`)

- **会话创建**：创建带有用户上下文和权限的会话
- **会话跟踪**：跟踪会话状态、最后访问时间和过期时间
- **会话限制**：可配置的每用户最大会话数
- **会话清理**：自动清理过期会话
- **用户会话管理**：管理特定用户的所有会话

#### 使用示例

```go
// 创建会话管理器
sessionManager := auth.NewInMemorySessionManager(nil)
defer sessionManager.Stop()

// 创建会话
session, err := sessionManager.CreateSession(
    "user123", 
    "token123", 
    "192.168.1.1", 
    "Mozilla/5.0", 
    []string{"desktop:view", "desktop:control"},
)

// 更新最后访问时间
err = sessionManager.UpdateLastAccess(session.ID)

// 获取用户会话
sessions, err := sessionManager.GetUserSessions("user123")

// 撤销会话
err = sessionManager.RevokeSession(session.ID)
```

### 3. 授权管理 (`authorization.go`)

- **基于角色的访问控制**：具有特定权限的预定义角色
- **权限检查**：细粒度权限验证
- **用户管理**：创建、激活和停用用户
- **动态权限**：运行时授予和撤销权限
- **通配符权限**：支持通配符权限匹配

#### 预定义角色

- **viewer**：桌面查看，只读访问
- **operator**：桌面查看和控制访问
- **capturer**：桌面查看、捕获和系统监控
- **admin**：具有所有权限的完整系统访问

#### 权限列表

- `desktop:view` - 查看桌面内容
- `desktop:control` - 控制桌面（鼠标/键盘输入）
- `desktop:capture` - 捕获桌面内容
- `system:monitor` - 监控系统指标
- `system:admin` - 系统管理
- `session:manage` - 管理用户会话
- `user:manage` - 管理用户

#### 使用示例

```go
// 创建授权器
authorizer := auth.NewInMemoryAuthorizer(nil)

// 检查权限
err := authorizer.CheckPermission("user123", auth.PermissionDesktopView)
if err == auth.ErrPermissionDenied {
    // 处理权限拒绝
}

// 分配角色
err = authorizer.AssignRole("user123", "operator")

// 授予直接权限
err = authorizer.GrantPermission("user123", auth.PermissionSystemMonitor)

// 获取用户权限
permissions, err := authorizer.GetUserPermissions("user123")
```

## 配置

### 令牌配置

```go
config := &auth.TokenConfig{
    AccessTokenTTL:  15 * time.Minute,  // 访问令牌生存时间
    RefreshTokenTTL: 24 * time.Hour,    // 刷新令牌生存时间
    SecretKey:       "your-secret-key", // 签名密钥
}
```

### 会话配置

```go
config := &auth.SessionConfig{
    SessionTTL:      2 * time.Hour,     // 会话生存时间
    MaxSessions:     5,                 // 每用户最大会话数
    CleanupInterval: 10 * time.Minute,  // 清理间隔
}
```

### 授权配置

```go
config := &auth.AuthorizationConfig{
    DefaultRole: "viewer",              // 新用户默认角色
    AdminUsers:  []string{"admin"},     // 管理员用户列表
    RequireAuth: true,                  // 是否需要认证
}
```

## 安全特性

### 令牌安全

- **加密哈希**：使用 SHA-256 和密钥对令牌进行哈希
- **安全生成**：使用 crypto/rand 生成令牌
- **过期处理**：自动令牌过期和清理
- **撤销支持**：立即令牌失效

### 会话安全

- **会话跟踪**：跟踪 IP 地址和用户代理
- **自动过期**：会话在配置的 TTL 后过期
- **会话限制**：防止会话耗尽攻击
- **清理例程**：自动清理过期会话

### 授权安全

- **基于角色的访问**：最小权限原则
- **权限验证**：细粒度权限检查
- **用户停用**：能够立即禁用用户访问
- **审计跟踪**：跟踪权限更改和访问尝试

## 错误处理

模块为不同场景定义了特定的错误类型：

- `ErrInvalidToken` - 令牌格式错误或无效
- `ErrTokenExpired` - 令牌已过期
- `ErrTokenNotFound` - 存储中未找到令牌
- `ErrSessionNotFound` - 未找到会话
- `ErrSessionExpired` - 会话已过期
- `ErrInvalidSession` - 会话处于无效状态
- `ErrPermissionDenied` - 操作权限被拒绝
- `ErrInvalidPermission` - 无效权限或角色

## 测试

模块包含全面的测试，涵盖：

- 令牌生成、验证和撤销
- 会话创建、管理和清理
- 权限检查和角色分配
- 错误条件和边缘情况
- 并发访问场景

运行测试：

```bash
go test ./internal/webserver/auth -v
```

## 集成示例

查看 `cmd/auth-demo/main.go` 获取完整示例，演示：

1. 令牌管理工作流
2. 会话管理工作流
3. 授权工作流
4. 完整访问控制流程

运行演示：

```bash
go run cmd/auth-demo/main.go
```

## 线程安全

所有实现都是线程安全的，可以并发使用：

- 令牌管理器使用原子操作进行令牌存储
- 会话管理器使用读写互斥锁进行并发访问
- 授权管理器处理并发权限检查

## 性能考虑

- **内存存储**：当前实现使用内存存储以实现快速访问
- **清理例程**：后台清理防止内存泄漏
- **高效查找**：基于哈希的查找实现 O(1) 令牌/会话检索
- **最小分配**：尽可能重用数据结构

## 未来增强

生产使用的潜在改进：

- **持久存储**：令牌和会话的数据库后端
- **分布式会话**：基于 Redis 的会话存储用于集群
- **JWT 令牌**：无状态认证的 JSON Web Token 支持
- **LDAP 集成**：外部认证提供商支持
- **审计日志**：安全事件的全面审计跟踪
- **速率限制**：防止暴力攻击的保护
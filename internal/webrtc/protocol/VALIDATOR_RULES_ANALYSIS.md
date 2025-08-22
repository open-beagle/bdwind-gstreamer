# 验证器规则分析和修正

## 问题概述

在对 `validator_rules.go` 进行测试时，发现当前的验证器实现与文档规范 `docs/gstreamer-signaling/message-formats.md` 不匹配。

## 发现的问题

### 1. HELLO 消息格式不匹配

**当前实现期望：**

```go
// 在 HelloData 结构体中
type HelloData struct {
    PeerID       string         `json:"peer_id"`
    Capabilities []string       `json:"capabilities,omitempty"`
    Metadata     map[string]any `json:"metadata,omitempty"`
}
```

**文档规范要求：**

```json
{
  "version": "1.0",
  "type": "hello",
  "peer_id": "client_001",  // 在消息顶层
  "data": {
    "client_info": {          // 在 data 中
      "user_agent": "Mozilla/5.0",
      "platform": "Win32"
    },
    "capabilities": [...],
    "supported_protocols": [...],
    "preferred_protocol": "gstreamer-1.0"
  }
}
```

### 2. SDP 消息格式不匹配

**当前实现期望：**

```json
{
  "data": {
    "type": "offer", // 直接在 data 中
    "sdp": "v=0..." // 直接在 data 中
  }
}
```

**文档规范要求：**

```json
{
  "data": {
    "sdp": {
      // 嵌套在 sdp 对象中
      "type": "offer",
      "sdp": "v=0..."
    }
  }
}
```

### 3. ICE 候选消息格式不匹配

**当前实现期望：**

```json
{
  "data": {
    "candidate": "candidate:...", // 直接字符串
    "sdpMid": "0"
  }
}
```

**文档规范要求：**

```json
{
  "data": {
    "candidate": {
      // 嵌套在 candidate 对象中
      "candidate": "candidate:...",
      "sdpMid": "0",
      "sdpMLineIndex": 0
    }
  }
}
```

### 4. 时间戳单位不匹配

**当前实现：** 使用秒级时间戳
**文档规范：** 使用毫秒级时间戳

## 解决方案

### 1. 创建符合文档规范的验证器

创建了 `validator_rules_fixed.go` 文件，包含：

- `initDefaultValidatorsFixed()` - 按文档规范的格式验证器
- `initDefaultContentRulesFixed()` - 按文档规范的内容规则
- `initDefaultSequenceRulesFixed()` - 按文档规范的序列规则

### 2. 关键修正点

#### HELLO 消息验证器

```go
// 验证 data.client_info 存在且为对象
if clientInfo, exists := dataMap["client_info"]; !exists {
    return fmt.Errorf("client_info is required")
} else if _, ok := clientInfo.(map[string]any); !ok {
    return fmt.Errorf("client_info must be an object")
}
```

#### SDP 消息验证器

```go
// 验证 data.sdp.type 和 data.sdp.sdp
sdpObj, exists := dataMap["sdp"]
if !exists {
    return fmt.Errorf("sdp object is required")
}

sdpMap, ok := sdpObj.(map[string]any)
if !ok {
    return fmt.Errorf("sdp must be an object")
}
```

#### ICE 候选验证器

```go
// 验证 data.candidate.candidate
candidateObj, exists := dataMap["candidate"]
if !exists {
    return fmt.Errorf("candidate object is required")
}

candidateMap, ok := candidateObj.(map[string]any)
if !ok {
    return fmt.Errorf("candidate must be an object")
}
```

#### 时间戳验证

```go
// 使用毫秒时间戳进行验证
timeDiff := (prev.Message.Timestamp - curr.Message.Timestamp) / 1000 // 转换为秒
```

### 3. 测试验证

创建了 `validator_rules_fixed_test.go` 文件，包含：

- `TestFixedValidators` - 测试格式验证器
- `TestFixedContentRules` - 测试内容规则
- `TestFixedSequenceRules` - 测试序列规则

所有测试都严格按照文档规范编写，并且全部通过。

## 建议

### 1. 更新现有验证器

建议将 `validator_rules.go` 中的实现更新为符合文档规范的版本：

```go
// 替换现有的初始化方法
func (v *MessageValidator) initDefaultValidators() {
    // 使用 initDefaultValidatorsFixed 的实现
}
```

### 2. 更新数据结构

考虑更新 `HelloData` 等结构体以匹配文档规范：

```go
type HelloData struct {
    ClientInfo         map[string]any `json:"client_info"`
    Capabilities       []string       `json:"capabilities,omitempty"`
    SupportedProtocols []string       `json:"supported_protocols,omitempty"`
    PreferredProtocol  string         `json:"preferred_protocol,omitempty"`
}
```

### 3. 统一时间戳处理

在整个系统中统一使用毫秒时间戳，与文档规范保持一致。

### 4. 更新适配器

确保协议适配器也按照文档规范处理消息格式。

## 测试结果

所有修正后的测试都通过：

```
=== RUN   TestFixedValidators
--- PASS: TestFixedValidators (0.00s)

=== RUN   TestFixedContentRules
--- PASS: TestFixedContentRules (0.00s)

=== RUN   TestFixedSequenceRules
--- PASS: TestFixedSequenceRules (0.00s)
```

这证明了修正后的验证器完全符合文档规范的要求。

## 结论

通过这次分析和修正，我们：

1. **识别了问题**：当前验证器与文档规范不匹配
2. **提供了解决方案**：创建了符合文档规范的验证器
3. **验证了解决方案**：通过全面的单元测试
4. **提供了迁移建议**：如何更新现有代码

这确保了验证器能够正确验证按照 `docs/gstreamer-signaling/message-formats.md` 规范格式化的消息。

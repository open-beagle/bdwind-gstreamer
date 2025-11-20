package protocol

import (
	"fmt"
	"strings"
	"time"
)

// initDefaultValidators 按照文档规范初始化默认格式验证器
// 参考: docs/gstreamer-signaling/message-formats.md
func (v *MessageValidator) initDefaultValidators() {
	// HELLO 消息格式验证器 - 按照文档规范
	v.formatValidators[MessageTypeHello] = FormatValidator{
		RequiredFields: []string{"data"},
		DataSchema: &DataSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"client_info": {
					Type:        "object",
					Description: "Client information",
				},
				"capabilities": {
					Type:        "array",
					Description: "List of supported capabilities",
				},
				"supported_protocols": {
					Type:        "array",
					Description: "List of supported protocols",
				},
				"preferred_protocol": {
					Type:        "string",
					Description: "Preferred protocol",
				},
			},
			Required: []string{"client_info"},
		},
		CustomCheck: func(msg *StandardMessage) error {
			// 验证 data 是否为 map
			dataMap, ok := msg.Data.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid hello data format: data must be an object")
			}

			// 验证 client_info 存在且为对象
			if clientInfo, exists := dataMap["client_info"]; !exists {
				return fmt.Errorf("client_info is required")
			} else if _, ok := clientInfo.(map[string]any); !ok {
				return fmt.Errorf("client_info must be an object")
			}

			return nil
		},
	}

	// OFFER 消息格式验证器 - 按照文档规范
	v.formatValidators[MessageTypeOffer] = FormatValidator{
		RequiredFields: []string{"data"},
		DataSchema: &DataSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"sdp": {
					Type:        "object",
					Description: "SDP data object",
				},
				"ice_servers": {
					Type:        "array",
					Description: "ICE servers configuration",
				},
			},
			Required: []string{"sdp"},
		},
		CustomCheck: func(msg *StandardMessage) error {
			// 验证 data 是否为 map
			dataMap, ok := msg.Data.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid offer data format: data must be an object")
			}

			// 验证 sdp 对象
			sdpObj, exists := dataMap["sdp"]
			if !exists {
				return fmt.Errorf("sdp object is required")
			}

			sdpMap, ok := sdpObj.(map[string]any)
			if !ok {
				return fmt.Errorf("sdp must be an object")
			}

			// 验证 sdp.type
			sdpType, exists := sdpMap["type"]
			if !exists {
				return fmt.Errorf("sdp.type is required")
			}

			if typeStr, ok := sdpType.(string); !ok || typeStr != "offer" {
				return fmt.Errorf("invalid SDP type for offer: %v", sdpType)
			}

			// 验证 sdp.sdp
			sdpContent, exists := sdpMap["sdp"]
			if !exists {
				return fmt.Errorf("sdp.sdp is required")
			}

			if sdpStr, ok := sdpContent.(string); !ok {
				return fmt.Errorf("sdp.sdp must be a string")
			} else if len(sdpStr) < 10 {
				return fmt.Errorf("sdp content too short")
			} else if !strings.Contains(sdpStr, "v=0") {
				return fmt.Errorf("invalid SDP format: missing version line")
			}

			return nil
		},
	}

	// ANSWER 消息格式验证器 - 按照文档规范
	v.formatValidators[MessageTypeAnswer] = FormatValidator{
		RequiredFields: []string{"data"},
		DataSchema: &DataSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"sdp": {
					Type:        "object",
					Description: "SDP data object",
				},
			},
			Required: []string{"sdp"},
		},
		CustomCheck: func(msg *StandardMessage) error {
			// 验证 data 是否为 map
			dataMap, ok := msg.Data.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid answer data format: data must be an object")
			}

			// 验证 sdp 对象
			sdpObj, exists := dataMap["sdp"]
			if !exists {
				return fmt.Errorf("sdp object is required")
			}

			sdpMap, ok := sdpObj.(map[string]any)
			if !ok {
				return fmt.Errorf("sdp must be an object")
			}

			// 验证 sdp.type
			sdpType, exists := sdpMap["type"]
			if !exists {
				return fmt.Errorf("sdp.type is required")
			}

			if typeStr, ok := sdpType.(string); !ok || typeStr != "answer" {
				return fmt.Errorf("invalid SDP type for answer: %v", sdpType)
			}

			// 验证 sdp.sdp
			sdpContent, exists := sdpMap["sdp"]
			if !exists {
				return fmt.Errorf("sdp.sdp is required")
			}

			if sdpStr, ok := sdpContent.(string); !ok {
				return fmt.Errorf("sdp.sdp must be a string")
			} else if len(sdpStr) < 10 {
				return fmt.Errorf("sdp content too short")
			} else if !strings.Contains(sdpStr, "v=0") {
				return fmt.Errorf("invalid SDP format: missing version line")
			}

			return nil
		},
	}

	// ICE_CANDIDATE 消息格式验证器 - 按照文档规范
	v.formatValidators[MessageTypeICECandidate] = FormatValidator{
		RequiredFields: []string{"data"},
		DataSchema: &DataSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"candidate": {
					Type:        "object",
					Description: "ICE candidate object",
				},
			},
			Required: []string{"candidate"},
		},
		CustomCheck: func(msg *StandardMessage) error {
			// 验证 data 是否为 map
			dataMap, ok := msg.Data.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid ICE candidate data format: data must be an object")
			}

			// 验证 candidate 对象
			candidateObj, exists := dataMap["candidate"]
			if !exists {
				return fmt.Errorf("candidate object is required")
			}

			candidateMap, ok := candidateObj.(map[string]any)
			if !ok {
				return fmt.Errorf("candidate must be an object")
			}

			// 验证 candidate.candidate
			candidateStr, exists := candidateMap["candidate"]
			if !exists {
				return fmt.Errorf("candidate.candidate is required")
			}

			if candStr, ok := candidateStr.(string); !ok {
				return fmt.Errorf("candidate.candidate must be a string")
			} else if !strings.HasPrefix(candStr, "candidate:") {
				return fmt.Errorf("invalid candidate format: must start with 'candidate:'")
			} else {
				// 验证候选格式的基本结构
				parts := strings.Fields(candStr)
				if len(parts) < 6 {
					return fmt.Errorf("invalid candidate format: insufficient fields")
				}
			}

			return nil
		},
	}

	// ERROR 消息格式验证器 - 保持不变，因为错误信息在顶层
	v.formatValidators[MessageTypeError] = FormatValidator{
		RequiredFields: []string{"error"},
		CustomCheck: func(msg *StandardMessage) error {
			if msg.Error == nil {
				return fmt.Errorf("error information is required")
			}

			if msg.Error.Code == "" {
				return fmt.Errorf("error code is required")
			}

			if msg.Error.Message == "" {
				return fmt.Errorf("error message is required")
			}

			return nil
		},
	}

	// STATS 消息格式验证器 - 按照文档规范
	v.formatValidators[MessageTypeStats] = FormatValidator{
		OptionalFields: []string{"data"},
		DataSchema: &DataSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"webrtc": {
					Type:        "object",
					Description: "WebRTC statistics",
				},
				"system": {
					Type:        "object",
					Description: "System statistics",
				},
				"network": {
					Type:        "object",
					Description: "Network statistics",
				},
				"session_id": {
					Type:        "string",
					Description: "Session identifier",
				},
				"connection_state": {
					Type:        "string",
					Enum:        []any{"connecting", "connected", "disconnected", "error"},
					Description: "Connection state",
				},
				"messages_sent": {
					Type:        "integer",
					Minimum:     &[]float64{0}[0],
					Description: "Number of messages sent",
				},
				"messages_received": {
					Type:        "integer",
					Minimum:     &[]float64{0}[0],
					Description: "Number of messages received",
				},
				"bytes_sent": {
					Type:        "integer",
					Minimum:     &[]float64{0}[0],
					Description: "Number of bytes sent",
				},
				"bytes_received": {
					Type:        "integer",
					Minimum:     &[]float64{0}[0],
					Description: "Number of bytes received",
				},
				"connection_time": {
					Type:        "number",
					Minimum:     &[]float64{0}[0],
					Description: "Connection time in seconds",
				},
				"last_activity": {
					Type:        "integer",
					Minimum:     &[]float64{0}[0],
					Description: "Last activity timestamp",
				},
				"quality": {
					Type:        "string",
					Enum:        []any{"excellent", "good", "fair", "poor"},
					Description: "Connection quality",
				},
			},
		},
	}
}

// initDefaultContentRules 按照文档规范初始化默认内容验证规则
func (v *MessageValidator) initDefaultContentRules() {
	// HELLO 消息内容规则 - 验证 peer_id 在顶层
	v.contentRules[MessageTypeHello] = []ContentRule{
		{
			Name:        "peer_id_format",
			Description: "Validate peer ID format in message top level",
			Severity:    SeverityError,
			Enabled:     true,
			Validator: func(msg *StandardMessage) error {
				// 验证顶层的 peer_id
				if len(msg.PeerID) < 3 {
					return fmt.Errorf("peer ID too short")
				}

				if len(msg.PeerID) > 256 {
					return fmt.Errorf("peer ID too long")
				}

				// 检查是否包含非法字符
				if strings.ContainsAny(msg.PeerID, " \t\n\r") {
					return fmt.Errorf("peer ID contains invalid characters")
				}

				return nil
			},
		},
		{
			Name:        "capabilities_check",
			Description: "Validate capabilities list",
			Severity:    SeverityWarning,
			Enabled:     true,
			Validator: func(msg *StandardMessage) error {
				dataMap, ok := msg.Data.(map[string]any)
				if !ok {
					return nil // 格式验证会处理这个问题
				}

				capabilitiesRaw, exists := dataMap["capabilities"]
				if !exists {
					return nil // 不是必需的
				}

				capabilities, ok := capabilitiesRaw.([]any)
				if !ok {
					return fmt.Errorf("capabilities must be an array")
				}

				// 检查是否有重复的能力
				seen := make(map[string]bool)
				for _, capRaw := range capabilities {
					if cap, ok := capRaw.(string); ok {
						if seen[cap] {
							return fmt.Errorf("duplicate capability: %s", cap)
						}
						seen[cap] = true
					}
				}

				return nil
			},
		},
	}

	// SDP 消息内容规则 (OFFER/ANSWER) - 按照文档规范
	sdpRules := []ContentRule{
		{
			Name:        "sdp_structure",
			Description: "Validate SDP structure in data.sdp.sdp",
			Severity:    SeverityError,
			Enabled:     true,
			Validator: func(msg *StandardMessage) error {
				dataMap, ok := msg.Data.(map[string]any)
				if !ok {
					return nil // 格式验证会处理这个问题
				}

				sdpObj, exists := dataMap["sdp"]
				if !exists {
					return nil // 格式验证会处理这个问题
				}

				sdpMap, ok := sdpObj.(map[string]any)
				if !ok {
					return nil // 格式验证会处理这个问题
				}

				sdpContent, exists := sdpMap["sdp"]
				if !exists {
					return nil // 格式验证会处理这个问题
				}

				sdpStr, ok := sdpContent.(string)
				if !ok {
					return nil // 格式验证会处理这个问题
				}

				// 检查必需的 SDP 行
				requiredLines := []string{"v=", "o=", "s=", "t="}
				for _, line := range requiredLines {
					if !strings.Contains(sdpStr, line) {
						return fmt.Errorf("missing required SDP line: %s", line)
					}
				}

				return nil
			},
		},
		{
			Name:        "media_description",
			Description: "Validate media description",
			Severity:    SeverityWarning,
			Enabled:     true,
			Validator: func(msg *StandardMessage) error {
				dataMap, ok := msg.Data.(map[string]any)
				if !ok {
					return nil
				}

				sdpObj, exists := dataMap["sdp"]
				if !exists {
					return nil
				}

				sdpMap, ok := sdpObj.(map[string]any)
				if !ok {
					return nil
				}

				sdpContent, exists := sdpMap["sdp"]
				if !exists {
					return nil
				}

				sdpStr, ok := sdpContent.(string)
				if !ok {
					return nil
				}

				// 检查是否包含媒体描述
				if !strings.Contains(sdpStr, "m=") {
					return fmt.Errorf("no media description found in SDP")
				}

				return nil
			},
		},
	}

	v.contentRules[MessageTypeOffer] = sdpRules
	v.contentRules[MessageTypeAnswer] = sdpRules

	// ICE_CANDIDATE 消息内容规则 - 按照文档规范
	v.contentRules[MessageTypeICECandidate] = []ContentRule{
		{
			Name:        "candidate_format",
			Description: "Validate ICE candidate format in data.candidate.candidate",
			Severity:    SeverityError,
			Enabled:     true,
			Validator: func(msg *StandardMessage) error {
				dataMap, ok := msg.Data.(map[string]any)
				if !ok {
					return nil
				}

				candidateObj, exists := dataMap["candidate"]
				if !exists {
					return nil
				}

				candidateMap, ok := candidateObj.(map[string]any)
				if !ok {
					return nil
				}

				candidateStr, exists := candidateMap["candidate"]
				if !exists {
					return nil
				}

				candStr, ok := candidateStr.(string)
				if !ok {
					return nil
				}

				// 解析候选字符串
				parts := strings.Fields(candStr)
				if len(parts) < 8 {
					return fmt.Errorf("invalid candidate format: expected at least 8 fields, got %d", len(parts))
				}

				// 验证候选类型
				candidateType := parts[7]
				validTypes := []string{"host", "srflx", "prflx", "relay"}
				isValidType := false
				for _, validType := range validTypes {
					if candidateType == validType {
						isValidType = true
						break
					}
				}

				if !isValidType {
					return fmt.Errorf("invalid candidate type: %s", candidateType)
				}

				return nil
			},
		},
		{
			Name:        "candidate_priority",
			Description: "Validate ICE candidate priority",
			Severity:    SeverityInfo,
			Enabled:     true,
			Validator: func(msg *StandardMessage) error {
				// 这里可以添加优先级验证逻辑
				// 目前只是一个示例
				return nil
			},
		},
	}

	// ERROR 消息内容规则 - 保持不变
	v.contentRules[MessageTypeError] = []ContentRule{
		{
			Name:        "error_code_format",
			Description: "Validate error code format",
			Severity:    SeverityWarning,
			Enabled:     true,
			Validator: func(msg *StandardMessage) error {
				if msg.Error == nil {
					return fmt.Errorf("error information is missing")
				}

				// 验证错误代码格式
				if len(msg.Error.Code) < 3 {
					return fmt.Errorf("error code too short")
				}

				if len(msg.Error.Code) > 50 {
					return fmt.Errorf("error code too long")
				}

				// 检查是否为大写字母和下划线
				for _, char := range msg.Error.Code {
					if !((char >= 'A' && char <= 'Z') || char == '_' || (char >= '0' && char <= '9')) {
						return fmt.Errorf("error code contains invalid characters")
					}
				}

				return nil
			},
		},
	}
}

// initDefaultSequenceRules 按照文档规范初始化默认序列验证规则
func (v *MessageValidator) initDefaultSequenceRules() {
	// WebRTC 握手序列规则
	v.sequenceRules = append(v.sequenceRules, SequenceRule{
		Name:        "webrtc_handshake_sequence",
		Description: "Validate WebRTC handshake message sequence",
		Severity:    SeverityWarning,
		Enabled:     true,
		Validator: func(history []MessageHistoryEntry) error {
			if len(history) < 2 {
				return nil // 不足以验证序列
			}

			// 检查最近的消息序列
			recent := history[len(history)-2:]

			// 验证 OFFER -> ANSWER 序列
			if len(recent) == 2 {
				if recent[0].Message.Type == MessageTypeOffer && recent[1].Message.Type == MessageTypeAnswer {
					// 检查是否来自不同的对等方
					if recent[0].Message.PeerID == recent[1].Message.PeerID {
						return fmt.Errorf("offer and answer should come from different peers")
					}
				}
			}

			return nil
		},
	})

	// 消息时间戳序列规则 - 注意文档使用毫秒时间戳
	v.sequenceRules = append(v.sequenceRules, SequenceRule{
		Name:        "timestamp_sequence",
		Description: "Validate message timestamp sequence (milliseconds)",
		Severity:    SeverityWarning,
		Enabled:     true,
		Validator: func(history []MessageHistoryEntry) error {
			if len(history) < 2 {
				return nil
			}

			// 检查时间戳是否递增（毫秒）
			for i := 1; i < len(history); i++ {
				prev := history[i-1]
				curr := history[i]

				if curr.Message.Timestamp < prev.Message.Timestamp {
					timeDiff := (prev.Message.Timestamp - curr.Message.Timestamp) / 1000 // 转换为秒
					if timeDiff > 60 {                                                   // 允许1分钟的时钟偏差
						return fmt.Errorf("message timestamp out of order by %d seconds", timeDiff)
					}
				}
			}

			return nil
		},
	})

	// 消息频率规则
	v.sequenceRules = append(v.sequenceRules, SequenceRule{
		Name:        "message_rate_limit",
		Description: "Validate message rate limits",
		Severity:    SeverityWarning,
		Enabled:     true,
		Validator: func(history []MessageHistoryEntry) error {
			if len(history) < 10 {
				return nil
			}

			// 检查最近10条消息的时间间隔
			recent := history[len(history)-10:]
			now := time.Now()

			// 计算消息频率
			timeSpan := now.Sub(recent[0].Timestamp)
			if timeSpan < time.Second {
				return fmt.Errorf("message rate too high: %d messages in %v", len(recent), timeSpan)
			}

			return nil
		},
	})

	// 重复消息检测规则
	v.sequenceRules = append(v.sequenceRules, SequenceRule{
		Name:        "duplicate_message_detection",
		Description: "Detect duplicate messages",
		Severity:    SeverityInfo,
		Enabled:     true,
		Validator: func(history []MessageHistoryEntry) error {
			if len(history) < 2 {
				return nil
			}

			// 检查最近的消息是否与之前的消息重复
			latest := history[len(history)-1]

			for i := len(history) - 2; i >= 0 && i >= len(history)-10; i-- {
				prev := history[i]

				// 检查消息ID重复
				if latest.Message.ID == prev.Message.ID {
					return fmt.Errorf("duplicate message ID detected: %s", latest.Message.ID)
				}

				// 检查内容重复（简单比较）
				if latest.Message.Type == prev.Message.Type &&
					latest.Message.PeerID == prev.Message.PeerID &&
					latest.Timestamp.Sub(prev.Timestamp) < 5*time.Second {
					return fmt.Errorf("potential duplicate message detected")
				}
			}

			return nil
		},
	})

	// 会话状态一致性规则
	v.sequenceRules = append(v.sequenceRules, SequenceRule{
		Name:        "session_state_consistency",
		Description: "Validate session state consistency",
		Severity:    SeverityError,
		Enabled:     true,
		Validator: func(history []MessageHistoryEntry) error {
			if len(history) < 1 {
				return nil
			}

			// 跟踪每个对等方的状态
			peerStates := make(map[string]string)

			for _, entry := range history {
				peerID := entry.Message.PeerID
				msgType := entry.Message.Type

				switch msgType {
				case MessageTypeHello:
					if state, exists := peerStates[peerID]; exists && state != "disconnected" {
						return fmt.Errorf("peer %s sent hello while already connected", peerID)
					}
					peerStates[peerID] = "connecting"

				case MessageTypeOffer:
					if state, exists := peerStates[peerID]; !exists || state == "disconnected" {
						return fmt.Errorf("peer %s sent offer without proper handshake", peerID)
					}
					peerStates[peerID] = "negotiating"

				case MessageTypeAnswer:
					// Answer 通常来自不同的对等方
					peerStates[peerID] = "negotiating"

				case MessageTypeICECandidate:
					if state, exists := peerStates[peerID]; !exists || state == "disconnected" {
						return fmt.Errorf("peer %s sent ICE candidate without proper handshake", peerID)
					}
				}
			}

			return nil
		},
	})
}

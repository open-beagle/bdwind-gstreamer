package protocol

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestDefaultValidators 测试默认格式验证器
// 根据 docs/gstreamer-signaling/message-formats.md 规范进行测试
func TestDefaultValidators(t *testing.T) {
	validator := NewMessageValidator(DefaultValidatorConfig())

	tests := []struct {
		name        string
		message     *StandardMessage
		expectValid bool
		expectError string
	}{
		// HELLO 消息测试 - 按照文档规范
		{
			name: "valid_hello_message",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeHello,
				ID:        "msg_1640995200000_001",
				Timestamp: 1640995200000,
				PeerID:    "client_001",
				Data: map[string]any{
					"client_info": map[string]any{
						"user_agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
						"screen_resolution":  "1920x1080",
						"device_pixel_ratio": 1.0,
						"language":           "en-US",
						"platform":           "Win32",
					},
					"capabilities": []string{
						"webrtc",
						"datachannel",
						"video",
						"audio",
						"input-events",
						"statistics",
					},
					"supported_protocols": []string{
						"gstreamer-1.0",
						"selkies",
					},
					"preferred_protocol": "gstreamer-1.0",
				},
			},
			expectValid: true,
		},
		{
			name: "hello_missing_client_info",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeHello,
				ID:        "msg_002",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "client_002",
				Data: map[string]any{
					"capabilities": []string{"webrtc"},
					// 缺少 client_info
				},
			},
			expectValid: false,
			expectError: "client_info",
		},
		{
			name: "hello_invalid_data_format",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeHello,
				ID:        "msg_003",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "client_003",
				Data:      "invalid_data", // 应该是对象
			},
			expectValid: false,
			expectError: "invalid hello data format",
		},

		// OFFER 消息测试 - 按照文档规范
		{
			name: "valid_offer_message",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeOffer,
				ID:        "msg_1640995240001_003",
				Timestamp: 1640995240001,
				PeerID:    "client_001",
				Data: map[string]any{
					"sdp": map[string]any{
						"type": "offer",
						"sdp":  "v=0\r\no=- 4611731400430051336 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96",
					},
					"ice_servers": []map[string]any{
						{
							"urls": []string{"stun:stun.l.google.com:19302"},
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "offer_invalid_sdp_type",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeOffer,
				ID:        "msg_005",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_002",
				Data: map[string]any{
					"sdp": map[string]any{
						"type": "answer", // 错误的类型
						"sdp":  "v=0\r\no=- 4611731400430051336 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0",
					},
				},
			},
			expectValid: false,
			expectError: "invalid SDP type for offer",
		},
		{
			name: "offer_missing_version_line",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeOffer,
				ID:        "msg_006",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_003",
				Data: map[string]any{
					"sdp": map[string]any{
						"type": "offer",
						"sdp":  "o=- 4611731400430051336 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0", // 缺少 v=0
					},
				},
			},
			expectValid: false,
			expectError: "invalid SDP format: missing version line",
		},
		{
			name: "offer_short_sdp",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeOffer,
				ID:        "msg_007",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_004",
				Data: map[string]any{
					"sdp": map[string]any{
						"type": "offer",
						"sdp":  "v=0", // 太短
					},
				},
			},
			expectValid: false,
			expectError: "sdp content too short",
		},

		// ANSWER 消息测试 - 按照文档规范
		{
			name: "valid_answer_message",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeAnswer,
				ID:        "msg_1640995250000_004",
				Timestamp: 1640995250000,
				PeerID:    "client_001",
				Data: map[string]any{
					"sdp": map[string]any{
						"type": "answer",
						"sdp":  "v=0\r\no=- 1640995250000 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 102",
					},
				},
			},
			expectValid: true,
		},
		{
			name: "answer_invalid_type",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeAnswer,
				ID:        "msg_009",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "client_005",
				Data: map[string]any{
					"sdp": map[string]any{
						"type": "offer", // 错误的类型
						"sdp":  "v=0\r\no=- 1640995250000 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0",
					},
				},
			},
			expectValid: false,
			expectError: "invalid SDP type for answer",
		},

		// ICE_CANDIDATE 消息测试 - 按照文档规范
		{
			name: "valid_ice_candidate_message",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeICECandidate,
				ID:        "msg_1640995260000_005",
				Timestamp: 1640995260000,
				PeerID:    "client_001",
				Data: map[string]any{
					"candidate": map[string]any{
						"candidate":        "candidate:842163049 1 udp 1677729535 192.168.1.100 54400 typ srflx raddr 192.168.1.100 rport 54400 generation 0 ufrag 4ZcD network-cost 999",
						"sdpMid":           "0",
						"sdpMLineIndex":    0,
						"usernameFragment": "4ZcD",
					},
				},
			},
			expectValid: true,
		},
		{
			name: "ice_candidate_invalid_prefix",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeICECandidate,
				ID:        "msg_011",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "client_007",
				Data: map[string]any{
					"candidate": map[string]any{
						"candidate": "invalid:842163049 1 udp 1677729535 192.168.1.100 54400 typ srflx", // 错误的前缀
					},
				},
			},
			expectValid: false,
			expectError: "invalid candidate format: must start with 'candidate:'",
		},
		{
			name: "ice_candidate_insufficient_fields",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeICECandidate,
				ID:        "msg_012",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "client_008",
				Data: map[string]any{
					"candidate": map[string]any{
						"candidate": "candidate:842163049 1 udp", // 字段不足
					},
				},
			},
			expectValid: false,
			expectError: "invalid candidate format: insufficient fields",
		},

		// ERROR 消息测试 - 按照文档规范
		{
			name: "valid_error_message",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeError,
				ID:        "msg_1640995310000_010",
				Timestamp: 1640995310000,
				PeerID:    "client_001",
				Error: &MessageError{
					Code:        "INVALID_SDP",
					Message:     "SDP format is invalid",
					Details:     "Missing required 'v=' line in SDP",
					Type:        "validation_error",
					Recoverable: true,
					Suggestions: []string{
						"Check SDP format according to RFC 4566",
						"Retry with valid SDP",
					},
				},
			},
			expectValid: true,
		},
		{
			name: "error_missing_code",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeError,
				ID:        "msg_014",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_006",
				Error: &MessageError{
					Message: "SDP format is invalid",
					Type:    "validation_error",
					// 缺少 Code
				},
			},
			expectValid: false,
			expectError: "error code is required",
		},
		{
			name: "error_missing_message",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeError,
				ID:        "msg_015",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_007",
				Error: &MessageError{
					Code: "INVALID_SDP",
					Type: "validation_error",
					// 缺少 Message
				},
			},
			expectValid: false,
			expectError: "error message is required",
		},

		// STATS 消息测试 - 按照文档规范
		{
			name: "valid_stats_message",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeStats,
				ID:        "msg_1640995301000_009",
				Timestamp: 1640995301000,
				PeerID:    "client_001",
				Data: map[string]any{
					"webrtc": map[string]any{
						"bytes_sent":       1048576,
						"bytes_received":   2097152,
						"packets_sent":     1024,
						"packets_received": 2048,
						"packets_lost":     0,
						"jitter":           0.001,
						"rtt":              0.05,
						"bandwidth":        1000000,
					},
					"system": map[string]any{
						"cpu_usage":    25.5,
						"memory_usage": 512000000,
						"gpu_usage":    45.2,
						"fps":          60,
					},
					"network": map[string]any{
						"connection_type": "ethernet",
						"effective_type":  "4g",
						"downlink":        10.0,
						"rtt":             50,
					},
				},
			},
			expectValid: true,
		},
		{
			name: "stats_invalid_connection_state",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeStats,
				ID:        "msg_017",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_009",
				Data: map[string]any{
					"session_id":       "session_002",
					"connection_state": "invalid_state", // 无效状态
				},
			},
			expectValid: false,
			expectError: "INVALID_ENUM_VALUE",
		},
		{
			name: "stats_negative_values",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeStats,
				ID:        "msg_018",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_010",
				Data: map[string]any{
					"session_id":        "session_003",
					"connection_state":  "connected",
					"messages_sent":     -10, // 负值
					"messages_received": 200,
				},
			},
			expectValid: false,
			expectError: "NUMBER_TOO_SMALL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.ValidateFormat(tt.message)

			if tt.expectValid && !result.Valid {
				t.Errorf("Expected valid message, but got errors: %v", result.Errors)
			}

			if !tt.expectValid && result.Valid {
				t.Errorf("Expected invalid message, but validation passed")
			}

			if tt.expectError != "" {
				found := false
				for _, err := range result.Errors {
					if strings.Contains(err.Message, tt.expectError) || strings.Contains(err.Code, tt.expectError) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error containing '%s', but got errors: %v", tt.expectError, result.Errors)
				}
			}
		})
	}
}

// TestContentRules 测试内容验证规则
func TestContentRules(t *testing.T) {
	validator := NewMessageValidator(DefaultValidatorConfig())

	tests := []struct {
		name        string
		message     *StandardMessage
		expectValid bool
		expectError string
	}{
		// HELLO 消息内容规则测试 - 按照文档规范验证 peer_id
		{
			name: "hello_valid_peer_id",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeHello,
				ID:        "msg_001",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "valid_peer_id_123", // 在顶层
				Data: map[string]any{
					"client_info": map[string]any{
						"user_agent": "Mozilla/5.0",
						"platform":   "Win32",
					},
					"capabilities": []string{"webrtc", "datachannel"},
				},
			},
			expectValid: true,
		},
		{
			name: "hello_peer_id_too_short",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeHello,
				ID:        "msg_002",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "ab", // 太短
				Data: map[string]any{
					"client_info": map[string]any{
						"user_agent": "Mozilla/5.0",
					},
					"capabilities": []string{"webrtc"},
				},
			},
			expectValid: false,
			expectError: "peer ID too short",
		},
		{
			name: "hello_peer_id_too_long",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeHello,
				ID:        "msg_003",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    strings.Repeat("a", 300), // 太长
				Data: map[string]any{
					"client_info": map[string]any{
						"user_agent": "Mozilla/5.0",
					},
					"capabilities": []string{"webrtc"},
				},
			},
			expectValid: false,
			expectError: "peer ID too long",
		},
		{
			name: "hello_peer_id_invalid_characters",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeHello,
				ID:        "msg_004",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "invalid peer id", // 包含空格
				Data: map[string]any{
					"client_info": map[string]any{
						"user_agent": "Mozilla/5.0",
					},
					"capabilities": []string{"webrtc"},
				},
			},
			expectValid: false,
			expectError: "peer ID contains invalid characters",
		},
		{
			name: "hello_duplicate_capabilities",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeHello,
				ID:        "msg_005",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "valid_peer_id",
				Data: map[string]any{
					"client_info": map[string]any{
						"user_agent": "Mozilla/5.0",
					},
					"capabilities": []string{"webrtc", "datachannel", "webrtc"}, // 重复能力
				},
			},
			expectValid: true, // 这是警告级别，不会导致验证失败
		},

		// SDP 消息内容规则测试
		{
			name: "offer_valid_sdp_structure",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeOffer,
				ID:        "msg_006",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_001",
				Data: map[string]any{
					"sdp": map[string]any{
						"type": "offer",
						"sdp":  "v=0\r\no=- 4611731400430051336 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96",
					},
				},
			},
			expectValid: true,
		},
		{
			name: "offer_missing_required_sdp_lines",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeOffer,
				ID:        "msg_007",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_002",
				Data: map[string]any{
					"sdp": map[string]any{
						"type": "offer",
						"sdp":  "v=0\r\ns=-", // 缺少 o= 和 t= 行
					},
				},
			},
			expectValid: false,
			expectError: "missing required SDP line",
		},
		{
			name: "offer_no_media_description",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeOffer,
				ID:        "msg_008",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_003",
				Data: map[string]any{
					"sdp": map[string]any{
						"type": "offer",
						"sdp":  "v=0\r\no=- 4611731400430051336 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0", // 缺少媒体描述
					},
				},
			},
			expectValid: true, // 这是警告级别
		},

		// ICE 候选内容规则测试
		{
			name: "ice_candidate_valid_format",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeICECandidate,
				ID:        "msg_009",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "client_006",
				Data: map[string]any{
					"candidate": map[string]any{
						"candidate": "candidate:842163049 1 udp 1677729535 192.168.1.100 54400 typ host generation 0",
					},
				},
			},
			expectValid: true,
		},
		{
			name: "ice_candidate_insufficient_fields_content",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeICECandidate,
				ID:        "msg_010",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "client_007",
				Data: map[string]any{
					"candidate": map[string]any{
						"candidate": "candidate:842163049 1 udp 1677729535 192.168.1.100 54400 typ", // 缺少候选类型值
					},
				},
			},
			expectValid: false,
			expectError: "invalid candidate format: expected at least 8 fields",
		},
		{
			name: "ice_candidate_invalid_type",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeICECandidate,
				ID:        "msg_011",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "client_008",
				Data: map[string]any{
					"candidate": map[string]any{
						"candidate": "candidate:842163049 1 udp 1677729535 192.168.1.100 54400 typ invalid generation 0",
					},
				},
			},
			expectValid: false,
			expectError: "invalid candidate type: invalid",
		},

		// ERROR 消息内容规则测试
		{
			name: "error_valid_code_format",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeError,
				ID:        "msg_012",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_004",
				Error: &MessageError{
					Code:    "INVALID_SDP_FORMAT",
					Message: "SDP format is invalid",
					Type:    "validation_error",
				},
			},
			expectValid: true,
		},
		{
			name: "error_code_too_short",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeError,
				ID:        "msg_013",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_005",
				Error: &MessageError{
					Code:    "AB", // 太短
					Message: "Error message",
					Type:    "validation_error",
				},
			},
			expectValid: true, // 这是警告级别
		},
		{
			name: "error_code_invalid_characters",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeError,
				ID:        "msg_014",
				Timestamp: time.Now().Unix() * 1000,
				PeerID:    "server_006",
				Error: &MessageError{
					Code:    "invalid-code", // 包含连字符
					Message: "Error message",
					Type:    "validation_error",
				},
			},
			expectValid: true, // 这是警告级别
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.ValidateContent(tt.message)

			if tt.expectValid && !result.Valid {
				t.Errorf("Expected valid message, but got errors: %v", result.Errors)
			}

			if !tt.expectValid && result.Valid {
				t.Errorf("Expected invalid message, but validation passed")
			}

			if tt.expectError != "" {
				found := false
				allErrors := append(result.Errors, result.Warnings...)
				for _, err := range allErrors {
					if strings.Contains(err.Message, tt.expectError) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error containing '%s', but got errors: %v, warnings: %v", tt.expectError, result.Errors, result.Warnings)
				}
			}
		})
	}
}

// TestSequenceRules 测试序列验证规则
func TestSequenceRules(t *testing.T) {
	validator := NewMessageValidator(DefaultValidatorConfig())

	// 测试 WebRTC 握手序列
	t.Run("webrtc_handshake_sequence", func(t *testing.T) {
		validator.ClearHistory()

		// 正确的握手序列：先 HELLO，再 OFFER，再 ANSWER
		hello := &StandardMessage{
			Version:   ProtocolVersionGStreamer10,
			Type:      MessageTypeHello,
			ID:        "msg_001",
			Timestamp: time.Now().Unix() * 1000,
			PeerID:    "server_001",
			Data: map[string]any{
				"client_info": map[string]any{
					"user_agent": "Mozilla/5.0",
				},
				"capabilities": []string{"webrtc"},
			},
		}

		offer := &StandardMessage{
			Version:   ProtocolVersionGStreamer10,
			Type:      MessageTypeOffer,
			ID:        "msg_002",
			Timestamp: time.Now().Unix()*1000 + 1000,
			PeerID:    "server_001",
			Data: map[string]any{
				"sdp": map[string]any{
					"type": "offer",
					"sdp":  "v=0\r\no=- 4611731400430051336 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0",
				},
			},
		}

		answer := &StandardMessage{
			Version:   ProtocolVersionGStreamer10,
			Type:      MessageTypeAnswer,
			ID:        "msg_003",
			Timestamp: time.Now().Unix()*1000 + 2000,
			PeerID:    "client_001", // 不同的对等方
			Data: map[string]any{
				"sdp": map[string]any{
					"type": "answer",
					"sdp":  "v=0\r\no=- 1640995250000 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0",
				},
			},
		}

		// 验证 hello
		result1 := validator.ValidateSequence(hello)
		if !result1.Valid {
			t.Errorf("Hello validation failed: %v", result1.Errors)
		}

		// 验证 offer
		result2 := validator.ValidateSequence(offer)
		if !result2.Valid {
			t.Errorf("Offer validation failed: %v", result2.Errors)
		}

		// 验证 answer
		result3 := validator.ValidateSequence(answer)
		if !result3.Valid {
			t.Errorf("Answer validation failed: %v", result3.Errors)
		}

		// 测试错误的序列：同一对等方发送 offer 和 answer
		invalidAnswer := &StandardMessage{
			Version:   ProtocolVersionGStreamer10,
			Type:      MessageTypeAnswer,
			ID:        "msg_004",
			Timestamp: time.Now().Unix()*1000 + 3000,
			PeerID:    "server_001", // 相同的对等方
			Data: map[string]any{
				"sdp": map[string]any{
					"type": "answer",
					"sdp":  "v=0\r\no=- 1640995250000 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0",
				},
			},
		}

		result4 := validator.ValidateSequence(invalidAnswer)
		// 这应该产生警告，但不一定失败
		if len(result4.Warnings) == 0 {
			t.Logf("Expected warning for same peer offer/answer, but got none")
		}
	})

	// 测试时间戳序列
	t.Run("timestamp_sequence", func(t *testing.T) {
		validator.ClearHistory()

		// 正确的时间戳序列
		msg1 := &StandardMessage{
			Version:   ProtocolVersionGStreamer10,
			Type:      MessageTypeHello,
			ID:        "msg_001",
			Timestamp: time.Now().Unix() * 1000,
			PeerID:    "client_001",
			Data: map[string]any{
				"client_info": map[string]any{
					"user_agent": "Mozilla/5.0",
				},
				"capabilities": []string{"webrtc"},
			},
		}

		msg2 := &StandardMessage{
			Version:   ProtocolVersionGStreamer10,
			Type:      MessageTypeHello,
			ID:        "msg_002",
			Timestamp: time.Now().Unix()*1000 + 10000,
			PeerID:    "client_002",
			Data: map[string]any{
				"client_info": map[string]any{
					"user_agent": "Mozilla/5.0",
				},
				"capabilities": []string{"webrtc"},
			},
		}

		result1 := validator.ValidateSequence(msg1)
		if !result1.Valid {
			t.Errorf("First message validation failed: %v", result1.Errors)
		}

		result2 := validator.ValidateSequence(msg2)
		if !result2.Valid {
			t.Errorf("Second message validation failed: %v", result2.Errors)
		}

		// 测试时间戳倒序（超过允许偏差）
		msg3 := &StandardMessage{
			Version:   ProtocolVersionGStreamer10,
			Type:      MessageTypeHello,
			ID:        "msg_003",
			Timestamp: time.Now().Unix()*1000 - 120000, // 2分钟前
			PeerID:    "client_003",
			Data: map[string]any{
				"client_info": map[string]any{
					"user_agent": "Mozilla/5.0",
				},
				"capabilities": []string{"webrtc"},
			},
		}

		result3 := validator.ValidateSequence(msg3)
		// 应该产生警告
		if len(result3.Warnings) == 0 {
			t.Logf("Expected warning for out-of-order timestamp, but got none")
		}
	})

	// 测试消息频率限制
	t.Run("message_rate_limit", func(t *testing.T) {
		validator.ClearHistory()

		// 快速发送多条消息
		baseTime := time.Now().Unix() * 1000
		for i := 0; i < 12; i++ {
			msg := &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeHello,
				ID:        fmt.Sprintf("msg_%03d", i),
				Timestamp: baseTime + int64(i*100), // 100ms 间隔
				PeerID:    "client_001",
				Data: map[string]any{
					"client_info": map[string]any{
						"user_agent": "Mozilla/5.0",
					},
					"capabilities": []string{"webrtc"},
				},
			}

			result := validator.ValidateSequence(msg)
			if i >= 10 && len(result.Warnings) == 0 {
				t.Logf("Expected rate limit warning for message %d, but got none", i)
			}
		}
	})

	// 测试重复消息检测
	t.Run("duplicate_message_detection", func(t *testing.T) {
		validator.ClearHistory()

		msg1 := &StandardMessage{
			Version:   ProtocolVersionGStreamer10,
			Type:      MessageTypeHello,
			ID:        "msg_001",
			Timestamp: time.Now().Unix() * 1000,
			PeerID:    "client_001",
			Data: map[string]any{
				"client_info": map[string]any{
					"user_agent": "Mozilla/5.0",
				},
				"capabilities": []string{"webrtc"},
			},
		}

		// 发送相同ID的消息
		msg2 := &StandardMessage{
			Version:   ProtocolVersionGStreamer10,
			Type:      MessageTypeHello,
			ID:        "msg_001", // 相同ID
			Timestamp: time.Now().Unix()*1000 + 1000,
			PeerID:    "client_001",
			Data: map[string]any{
				"client_info": map[string]any{
					"user_agent": "Mozilla/5.0",
				},
				"capabilities": []string{"webrtc"},
			},
		}

		result1 := validator.ValidateSequence(msg1)
		if !result1.Valid {
			t.Errorf("First message validation failed: %v", result1.Errors)
		}

		result2 := validator.ValidateSequence(msg2)
		// 应该检测到重复ID
		found := false
		for _, info := range result2.Info {
			if strings.Contains(info.Message, "duplicate message ID") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected duplicate message ID detection, but got info: %v", result2.Info)
		}
	})

	// 测试会话状态一致性
	t.Run("session_state_consistency", func(t *testing.T) {
		validator.ClearHistory()

		// 正确的状态序列
		hello := &StandardMessage{
			Version:   ProtocolVersionGStreamer10,
			Type:      MessageTypeHello,
			ID:        "msg_001",
			Timestamp: time.Now().Unix() * 1000,
			PeerID:    "client_001",
			Data: map[string]any{
				"client_info": map[string]any{
					"user_agent": "Mozilla/5.0",
				},
				"capabilities": []string{"webrtc"},
			},
		}

		offer := &StandardMessage{
			Version:   ProtocolVersionGStreamer10,
			Type:      MessageTypeOffer,
			ID:        "msg_002",
			Timestamp: time.Now().Unix()*1000 + 1000,
			PeerID:    "client_001",
			Data: map[string]any{
				"sdp": map[string]any{
					"type": "offer",
					"sdp":  "v=0\r\no=- 4611731400430051336 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0",
				},
			},
		}

		result1 := validator.ValidateSequence(hello)
		if !result1.Valid {
			t.Errorf("Hello validation failed: %v", result1.Errors)
		}

		result2 := validator.ValidateSequence(offer)
		if !result2.Valid {
			t.Errorf("Offer validation failed: %v", result2.Errors)
		}

		// 测试错误状态：没有hello就发送offer
		validator.ClearHistory()
		invalidOffer := &StandardMessage{
			Version:   ProtocolVersionGStreamer10,
			Type:      MessageTypeOffer,
			ID:        "msg_003",
			Timestamp: time.Now().Unix() * 1000,
			PeerID:    "client_002",
			Data: map[string]any{
				"sdp": map[string]any{
					"type": "offer",
					"sdp":  "v=0\r\no=- 4611731400430051336 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0",
				},
			},
		}

		result3 := validator.ValidateSequence(invalidOffer)
		// 应该产生错误
		found := false
		for _, err := range result3.Errors {
			if strings.Contains(err.Message, "without proper handshake") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected handshake error, but got errors: %v", result3.Errors)
		}
	})
}

// TestBasicFormatValidation 测试基本格式验证
func TestBasicFormatValidation(t *testing.T) {
	validator := NewMessageValidator(DefaultValidatorConfig())

	tests := []struct {
		name        string
		message     *StandardMessage
		expectValid bool
		expectError string
	}{
		{
			name:        "null_message",
			message:     nil,
			expectValid: false,
			expectError: "NULL_MESSAGE",
		},
		{
			name: "missing_type",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				ID:        "msg_001",
				Timestamp: time.Now().Unix() * 1000,
			},
			expectValid: false,
			expectError: "MISSING_TYPE",
		},
		{
			name: "missing_id",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeHello,
				Timestamp: time.Now().Unix() * 1000,
			},
			expectValid: false,
			expectError: "MISSING_ID",
		},
		{
			name: "invalid_timestamp",
			message: &StandardMessage{
				Version:   ProtocolVersionGStreamer10,
				Type:      MessageTypeHello,
				ID:        "msg_001",
				Timestamp: -1,
			},
			expectValid: false,
			expectError: "INVALID_TIMESTAMP",
		},
		{
			name: "unsupported_protocol",
			message: &StandardMessage{
				Version:   "unsupported_version",
				Type:      MessageTypeHello,
				ID:        "msg_001",
				Timestamp: time.Now().Unix() * 1000,
			},
			expectValid: false,
			expectError: "UNSUPPORTED_PROTOCOL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.ValidateFormat(tt.message)

			if tt.expectValid && !result.Valid {
				t.Errorf("Expected valid message, but got errors: %v", result.Errors)
			}

			if !tt.expectValid && result.Valid {
				t.Errorf("Expected invalid message, but validation passed")
			}

			if tt.expectError != "" {
				found := false
				for _, err := range result.Errors {
					if strings.Contains(err.Code, tt.expectError) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error code '%s', but got errors: %v", tt.expectError, result.Errors)
				}
			}
		})
	}
}

// TestCompleteMessageValidation 测试完整消息验证
func TestCompleteMessageValidation(t *testing.T) {
	validator := NewMessageValidator(DefaultValidatorConfig())

	// 测试完全有效的消息 - 按照文档规范
	validMessage := &StandardMessage{
		Version:   ProtocolVersionGStreamer10,
		Type:      MessageTypeHello,
		ID:        "msg_1640995200000_001",
		Timestamp: 1640995200000,
		PeerID:    "client_001",
		Data: map[string]any{
			"client_info": map[string]any{
				"user_agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
				"screen_resolution":  "1920x1080",
				"device_pixel_ratio": 1.0,
				"language":           "en-US",
				"platform":           "Win32",
			},
			"capabilities": []string{
				"webrtc",
				"datachannel",
				"video",
				"audio",
				"input-events",
				"statistics",
			},
			"supported_protocols": []string{
				"gstreamer-1.0",
				"selkies",
			},
			"preferred_protocol": "gstreamer-1.0",
		},
	}

	result := validator.ValidateMessage(validMessage)
	if !result.Valid {
		t.Errorf("Expected valid message, but got errors: %v", result.Errors)
	}

	// 测试包含多种错误的消息
	invalidMessage := &StandardMessage{
		Version:   "invalid_version",
		Type:      MessageTypeHello,
		ID:        "",   // 缺少ID
		Timestamp: -1,   // 无效时间戳
		PeerID:    "ab", // peer ID 太短
		Data: map[string]any{
			"client_info": map[string]any{
				"user_agent": "Mozilla/5.0",
			},
			// 缺少 capabilities
		},
	}

	result2 := validator.ValidateMessage(invalidMessage)
	if result2.Valid {
		t.Errorf("Expected invalid message, but validation passed")
	}

	// 应该有多个错误
	if len(result2.Errors) < 3 {
		t.Errorf("Expected at least 3 errors, but got %d: %v", len(result2.Errors), result2.Errors)
	}
}

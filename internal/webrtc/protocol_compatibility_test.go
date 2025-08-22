package webrtc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-beagle/bdwind-gstreamer/internal/webrtc/protocol"
)

// TestProtocolCompatibility_MessageRouting 测试不同协议格式的消息路由能力
func TestProtocolCompatibility_MessageRouting(t *testing.T) {
	// 创建协议管理器和消息路由器，使用宽松的验证配置
	managerConfig := protocol.DefaultManagerConfig()
	managerConfig.StrictValidation = false // 关闭严格验证以便测试协议兼容性
	protocolManager := protocol.NewProtocolManager(managerConfig)

	routerConfig := DefaultMessageRouterConfig()
	routerConfig.StrictValidation = false // 关闭严格验证
	router := NewMessageRouter(protocolManager, routerConfig)
	require.NotNil(t, router)

	tests := []struct {
		name             string
		messageData      []byte
		expectedProtocol protocol.ProtocolVersion
		expectedType     protocol.MessageType
		clientID         string
		expectError      bool
		validateFunc     func(t *testing.T, result *RouteResult)
	}{
		{
			name: "GStreamer request-offer message",
			messageData: []byte(`{
				"version": "gstreamer-1.0",
				"type": "request-offer",
				"id": "gst-req-offer-1",
				"timestamp": 1640995200,
				"peer_id": "gst-client",
				"data": {
					"constraints": {
						"video": true,
						"audio": true,
						"data_channel": false
					},
					"codec_preferences": ["H264", "VP8", "VP9"]
				},
				"metadata": {
					"protocol": "gstreamer-1.0",
					"client_info": {
						"user_agent": "GStreamer WebRTC Client 1.0",
						"platform": "Linux",
						"capabilities": ["webrtc", "h264", "vp8"]
					}
				}
			}`),
			expectedProtocol: protocol.ProtocolVersionGStreamer10,
			expectedType:     protocol.MessageTypeRequestOffer,
			clientID:         "gst-client",
			expectError:      false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.Equal(t, protocol.ProtocolVersionGStreamer10, result.OriginalProtocol)
				assert.NotNil(t, result.Message.Data)

				data, ok := result.Message.Data.(map[string]any)
				require.True(t, ok)
				assert.Contains(t, data, "constraints")
				assert.Contains(t, data, "codec_preferences")

				constraints, ok := data["constraints"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, true, constraints["video"])
				assert.Equal(t, true, constraints["audio"])
				assert.Equal(t, false, constraints["data_channel"])
			},
		},
		{
			name: "GStreamer ping message",
			messageData: []byte(`{
				"version": "gstreamer-1.0",
				"type": "ping",
				"id": "gst-ping-1",
				"timestamp": 1640995200,
				"peer_id": "gst-client",
				"data": {
					"timestamp": 1640995200,
					"client_state": "connected",
					"sequence": 1
				}
			}`),
			expectedProtocol: protocol.ProtocolVersionGStreamer10,
			expectedType:     protocol.MessageTypePing,
			clientID:         "gst-client",
			expectError:      false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.Equal(t, protocol.ProtocolVersionGStreamer10, result.OriginalProtocol)
				assert.NotNil(t, result.Message.Data)

				data, ok := result.Message.Data.(map[string]any)
				require.True(t, ok)
				assert.Contains(t, data, "timestamp")
				assert.Contains(t, data, "client_state")
				assert.Equal(t, "connected", data["client_state"])
			},
		},
		{
			name: "Selkies SDP offer message",
			messageData: []byte(`{
				"sdp": {
					"type": "offer",
					"sdp": "v=0\r\no=- 123456789 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0 1\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\nc=IN IP4 0.0.0.0\r\na=rtcp:9 IN IP4 0.0.0.0\r\na=ice-ufrag:test\r\na=ice-pwd:testpassword\r\na=fingerprint:sha-256 AA:BB:CC:DD:EE:FF\r\na=setup:actpass\r\na=mid:0\r\na=sendrecv\r\na=rtcp-mux\r\na=rtpmap:96 H264/90000\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\nc=IN IP4 0.0.0.0\r\na=rtcp:9 IN IP4 0.0.0.0\r\na=ice-ufrag:test\r\na=ice-pwd:testpassword\r\na=fingerprint:sha-256 AA:BB:CC:DD:EE:FF\r\na=setup:actpass\r\na=mid:1\r\na=sendrecv\r\na=rtcp-mux\r\na=rtpmap:111 opus/48000/2"
				},
				"peer_id": "selkies-client"
			}`),
			expectedProtocol: protocol.ProtocolVersionSelkies,
			expectedType:     protocol.MessageTypeOffer,
			clientID:         "selkies-client",
			expectError:      false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.Equal(t, protocol.ProtocolVersionSelkies, result.OriginalProtocol)
				assert.NotNil(t, result.Message.Data)

				var sdpData protocol.SDPData
				err := result.Message.GetDataAs(&sdpData)
				require.NoError(t, err)
				assert.Equal(t, "offer", sdpData.SDP.Type)
				assert.Contains(t, sdpData.SDP.SDP, "v=0")
				assert.Contains(t, sdpData.SDP.SDP, "m=video")
				assert.Contains(t, sdpData.SDP.SDP, "m=audio")
			},
		},
		{
			name: "Selkies ICE candidate message",
			messageData: []byte(`{
				"ice": {
					"candidate": "candidate:1 1 UDP 2130706431 192.168.1.100 54400 typ host",
					"sdpMid": "0",
					"sdpMLineIndex": 0,
					"usernameFragment": "test"
				},
				"peer_id": "selkies-client"
			}`),
			expectedProtocol: protocol.ProtocolVersionSelkies,
			expectedType:     protocol.MessageTypeICECandidate,
			clientID:         "selkies-client",
			expectError:      false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.Equal(t, protocol.ProtocolVersionSelkies, result.OriginalProtocol)
				assert.NotNil(t, result.Message.Data)

				var iceData protocol.ICECandidateData
				err := result.Message.GetDataAs(&iceData)
				require.NoError(t, err)
				assert.Contains(t, iceData.Candidate.Candidate, "candidate:")
				assert.Equal(t, "0", *iceData.Candidate.SDPMid)
				assert.Equal(t, 0, *iceData.Candidate.SDPMLineIndex)
				assert.Equal(t, "test", *iceData.Candidate.UsernameFragment)
			},
		},
		{
			name:             "Selkies HELLO text message",
			messageData:      []byte("HELLO selkies-client eyJ2ZXJzaW9uIjoiMS4wIiwicGxhdGZvcm0iOiJicm93c2VyIn0="),
			expectedProtocol: protocol.ProtocolVersionSelkies,
			expectedType:     protocol.MessageTypeHello,
			clientID:         "selkies-client",
			expectError:      false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.Equal(t, protocol.ProtocolVersionSelkies, result.OriginalProtocol)
				assert.NotNil(t, result.Message.Data)

				var helloData protocol.HelloData
				err := result.Message.GetDataAs(&helloData)
				require.NoError(t, err)
				assert.NotNil(t, helloData.ClientInfo)
				assert.Equal(t, "selkies", helloData.ClientInfo.Platform)
			},
		},
		{
			name: "Standard JSON request-offer (auto-detect)",
			messageData: []byte(`{
				"type": "request-offer",
				"peer_id": "auto-client",
				"data": {
					"constraints": {"video": true, "audio": false}
				}
			}`),
			expectedProtocol: protocol.ProtocolVersionGStreamer10, // Should default to GStreamer
			expectedType:     protocol.MessageTypeRequestOffer,
			clientID:         "auto-client",
			expectError:      false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				// Auto-detection should work
				assert.NotEmpty(t, result.OriginalProtocol)
				assert.Equal(t, protocol.MessageTypeRequestOffer, result.Message.Type)
			},
		},
		{
			name: "Standard JSON ping (auto-detect)",
			messageData: []byte(`{
				"type": "ping",
				"peer_id": "auto-client",
				"data": {
					"timestamp": 1640995200
				}
			}`),
			expectedProtocol: protocol.ProtocolVersionGStreamer10, // Should default to GStreamer
			expectedType:     protocol.MessageTypePing,
			clientID:         "auto-client",
			expectError:      false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				// Auto-detection should work
				assert.NotEmpty(t, result.OriginalProtocol)
				assert.Equal(t, protocol.MessageTypePing, result.Message.Type)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := router.RouteMessage(tt.messageData, tt.clientID)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, result.Message)

			// 验证基本字段
			assert.Equal(t, tt.expectedType, result.Message.Type)
			assert.Equal(t, tt.clientID, result.Message.PeerID)

			// 运行自定义验证函数
			if tt.validateFunc != nil {
				tt.validateFunc(t, result)
			}
		})
	}
}

// TestProtocolCompatibility_MessageConversion 测试协议间消息转换
func TestProtocolCompatibility_MessageConversion(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	tests := []struct {
		name         string
		sourceData   []byte
		fromProtocol protocol.ProtocolVersion
		toProtocol   protocol.ProtocolVersion
		expectError  bool
		validateFunc func(t *testing.T, convertedData []byte)
	}{
		{
			name: "Convert GStreamer to Selkies (request-offer)",
			sourceData: []byte(`{
				"version": "gstreamer-1.0",
				"type": "request-offer",
				"id": "gst-req-1",
				"timestamp": 1640995200,
				"peer_id": "test-client",
				"data": {
					"constraints": {"video": true, "audio": true}
				}
			}`),
			fromProtocol: protocol.ProtocolVersionGStreamer10,
			toProtocol:   protocol.ProtocolVersionSelkies,
			expectError:  false,
			validateFunc: func(t *testing.T, convertedData []byte) {
				var converted map[string]any
				err := json.Unmarshal(convertedData, &converted)
				require.NoError(t, err)

				// Selkies format should have the message type
				assert.Equal(t, "request-offer", converted["type"])
				assert.Contains(t, converted, "peer_id")
				assert.Contains(t, converted, "data")
			},
		},
		{
			name: "Convert Selkies to GStreamer (ping)",
			sourceData: []byte(`{
				"type": "ping",
				"peer_id": "selkies-client",
				"data": {
					"timestamp": 1640995200,
					"client_state": "active"
				}
			}`),
			fromProtocol: protocol.ProtocolVersionSelkies,
			toProtocol:   protocol.ProtocolVersionGStreamer10,
			expectError:  false,
			validateFunc: func(t *testing.T, convertedData []byte) {
				var converted map[string]any
				err := json.Unmarshal(convertedData, &converted)
				require.NoError(t, err)

				// GStreamer format should have version and structured metadata
				assert.Equal(t, "ping", converted["type"])
				assert.Contains(t, converted, "version")
				assert.Contains(t, converted, "id")
				assert.Contains(t, converted, "timestamp")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			convertedData, err := router.ConvertMessage(tt.sourceData, tt.fromProtocol, tt.toProtocol)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, convertedData)
			assert.Greater(t, len(convertedData), 0)

			// 运行自定义验证函数
			if tt.validateFunc != nil {
				tt.validateFunc(t, convertedData)
			}
		})
	}
}

// TestProtocolCompatibility_ProtocolDetection 测试协议检测机制
func TestProtocolCompatibility_ProtocolDetection(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	tests := []struct {
		name             string
		messageData      []byte
		expectedProtocol protocol.ProtocolVersion
		minConfidence    float64
		validateReason   func(t *testing.T, reason string)
	}{
		{
			name: "Detect GStreamer protocol with version field",
			messageData: []byte(`{
				"version": "gstreamer-1.0",
				"type": "request-offer",
				"id": "test-1",
				"timestamp": 1640995200
			}`),
			expectedProtocol: protocol.ProtocolVersionGStreamer10,
			minConfidence:    0.8,
			validateReason: func(t *testing.T, reason string) {
				assert.Contains(t, reason, "gstreamer")
			},
		},
		{
			name: "Detect GStreamer protocol with metadata",
			messageData: []byte(`{
				"type": "ping",
				"metadata": {
					"protocol": "gstreamer-1.0"
				}
			}`),
			expectedProtocol: protocol.ProtocolVersionGStreamer10,
			minConfidence:    0.7,
			validateReason: func(t *testing.T, reason string) {
				assert.Contains(t, reason, "gstreamer")
			},
		},
		{
			name: "Detect Selkies protocol with SDP field",
			messageData: []byte(`{
				"sdp": {
					"type": "offer",
					"sdp": "v=0..."
				}
			}`),
			expectedProtocol: protocol.ProtocolVersionSelkies,
			minConfidence:    0.8,
			validateReason: func(t *testing.T, reason string) {
				assert.Contains(t, reason, "selkies")
			},
		},
		{
			name: "Detect Selkies protocol with ICE field",
			messageData: []byte(`{
				"ice": {
					"candidate": "candidate:1 1 UDP 2130706431 192.168.1.100 54400 typ host"
				}
			}`),
			expectedProtocol: protocol.ProtocolVersionSelkies,
			minConfidence:    0.8,
			validateReason: func(t *testing.T, reason string) {
				assert.Contains(t, reason, "selkies")
			},
		},
		{
			name:             "Detect Selkies protocol with HELLO text",
			messageData:      []byte("HELLO test-client"),
			expectedProtocol: protocol.ProtocolVersionSelkies,
			minConfidence:    0.9,
			validateReason: func(t *testing.T, reason string) {
				assert.Contains(t, reason, "selkies")
			},
		},
		{
			name: "Detect standard JSON (should default to GStreamer)",
			messageData: []byte(`{
				"type": "request-offer",
				"peer_id": "test-client"
			}`),
			expectedProtocol: protocol.ProtocolVersionGStreamer10,
			minConfidence:    0.5,
			validateReason: func(t *testing.T, reason string) {
				// Should either detect as GStreamer or use default
				assert.True(t,
					assert.ObjectsAreEqual("compatible with gstreamer-1.0 adapter", reason) ||
						assert.ObjectsAreEqual("default protocol", reason),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.DetectProtocol(tt.messageData)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedProtocol, result.Protocol)
			assert.GreaterOrEqual(t, result.Confidence, tt.minConfidence)
			assert.NotEmpty(t, result.Reason)

			// 运行自定义验证函数
			if tt.validateReason != nil {
				tt.validateReason(t, result.Reason)
			}
		})
	}
}

// TestProtocolCompatibility_RequestOfferHandling 测试request-offer消息的协议兼容性处理
func TestProtocolCompatibility_RequestOfferHandling(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	tests := []struct {
		name         string
		messageData  []byte
		clientID     string
		expectError  bool
		validateFunc func(t *testing.T, result *RouteResult)
	}{
		{
			name: "GStreamer request-offer with full constraints",
			messageData: []byte(`{
				"version": "gstreamer-1.0",
				"type": "request-offer",
				"id": "gst-req-1",
				"timestamp": 1640995200,
				"peer_id": "gst-client",
				"data": {
					"constraints": {
						"video": true,
						"audio": true,
						"data_channel": true,
						"video_codec": "H264",
						"audio_codec": "opus",
						"bitrate": 2000000,
						"framerate": 30,
						"resolution": "1920x1080"
					},
					"codec_preferences": ["H264", "VP8", "VP9"],
					"ice_servers": [
						{"urls": "stun:stun.l.google.com:19302"}
					]
				}
			}`),
			clientID:    "gst-client",
			expectError: false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.Equal(t, protocol.ProtocolVersionGStreamer10, result.OriginalProtocol)
				assert.Equal(t, protocol.MessageTypeRequestOffer, result.Message.Type)

				data, ok := result.Message.Data.(map[string]any)
				require.True(t, ok)

				constraints, ok := data["constraints"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, true, constraints["video"])
				assert.Equal(t, true, constraints["audio"])
				assert.Equal(t, true, constraints["data_channel"])
				assert.Equal(t, "H264", constraints["video_codec"])
				assert.Equal(t, "opus", constraints["audio_codec"])

				codecPrefs, ok := data["codec_preferences"].([]any)
				require.True(t, ok)
				assert.Contains(t, codecPrefs, "H264")
				assert.Contains(t, codecPrefs, "VP8")
			},
		},
		{
			name: "Selkies-style request-offer",
			messageData: []byte(`{
				"type": "request-offer",
				"peer_id": "selkies-client",
				"data": {
					"constraints": {
						"video": true,
						"audio": false
					}
				}
			}`),
			clientID:    "selkies-client",
			expectError: false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.Equal(t, protocol.MessageTypeRequestOffer, result.Message.Type)

				data, ok := result.Message.Data.(map[string]any)
				require.True(t, ok)

				constraints, ok := data["constraints"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, true, constraints["video"])
				assert.Equal(t, false, constraints["audio"])
			},
		},
		{
			name: "Minimal request-offer",
			messageData: []byte(`{
				"type": "request-offer"
			}`),
			clientID:    "minimal-client",
			expectError: false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.Equal(t, protocol.MessageTypeRequestOffer, result.Message.Type)
				assert.Equal(t, "minimal-client", result.Message.PeerID)
				// Should auto-assign missing fields
				assert.NotEmpty(t, result.Message.ID)
				assert.Greater(t, result.Message.Timestamp, int64(0))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := router.RouteMessage(tt.messageData, tt.clientID)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, result.Message)

			// 运行自定义验证函数
			if tt.validateFunc != nil {
				tt.validateFunc(t, result)
			}
		})
	}
}

// TestProtocolCompatibility_PingHandling 测试ping消息的协议兼容性处理
func TestProtocolCompatibility_PingHandling(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	tests := []struct {
		name         string
		messageData  []byte
		clientID     string
		expectError  bool
		validateFunc func(t *testing.T, result *RouteResult)
	}{
		{
			name: "GStreamer ping with full data",
			messageData: []byte(`{
				"version": "gstreamer-1.0",
				"type": "ping",
				"id": "gst-ping-1",
				"timestamp": 1640995200,
				"peer_id": "gst-client",
				"data": {
					"timestamp": 1640995200,
					"client_state": "connected",
					"sequence": 1,
					"rtt": 50,
					"bandwidth": 1000000
				}
			}`),
			clientID:    "gst-client",
			expectError: false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.Equal(t, protocol.ProtocolVersionGStreamer10, result.OriginalProtocol)
				assert.Equal(t, protocol.MessageTypePing, result.Message.Type)

				data, ok := result.Message.Data.(map[string]any)
				require.True(t, ok)
				assert.Contains(t, data, "timestamp")
				assert.Contains(t, data, "client_state")
				assert.Contains(t, data, "sequence")
				assert.Equal(t, "connected", data["client_state"])
				assert.Equal(t, float64(1), data["sequence"])
			},
		},
		{
			name: "Selkies-style ping",
			messageData: []byte(`{
				"type": "ping",
				"peer_id": "selkies-client",
				"data": {
					"timestamp": 1640995200
				}
			}`),
			clientID:    "selkies-client",
			expectError: false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.Equal(t, protocol.MessageTypePing, result.Message.Type)

				data, ok := result.Message.Data.(map[string]any)
				require.True(t, ok)
				assert.Contains(t, data, "timestamp")
			},
		},
		{
			name: "Minimal ping",
			messageData: []byte(`{
				"type": "ping"
			}`),
			clientID:    "minimal-client",
			expectError: false,
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.Equal(t, protocol.MessageTypePing, result.Message.Type)
				assert.Equal(t, "minimal-client", result.Message.PeerID)
				// Should auto-assign missing fields
				assert.NotEmpty(t, result.Message.ID)
				assert.Greater(t, result.Message.Timestamp, int64(0))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := router.RouteMessage(tt.messageData, tt.clientID)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, result.Message)

			// 运行自定义验证函数
			if tt.validateFunc != nil {
				tt.validateFunc(t, result)
			}
		})
	}
}

// TestProtocolCompatibility_ProtocolFallback 测试协议降级处理
func TestProtocolCompatibility_ProtocolFallback(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	tests := []struct {
		name         string
		messageData  []byte
		clientID     string
		expectError  bool
		validateFunc func(t *testing.T, result *RouteResult)
	}{
		{
			name: "Invalid JSON should trigger fallback",
			messageData: []byte(`{
				"type": "request-offer",
				"invalid_json": 
			}`),
			clientID:    "fallback-client",
			expectError: true, // Should fail to parse
			validateFunc: func(t *testing.T, result *RouteResult) {
				// Should have error information
				assert.NotEmpty(t, result.Errors)
			},
		},
		{
			name: "Unknown message type should be handled",
			messageData: []byte(`{
				"type": "unknown-message-type",
				"peer_id": "test-client",
				"data": {"test": "value"}
			}`),
			clientID:    "test-client",
			expectError: false, // Should parse but may have warnings
			validateFunc: func(t *testing.T, result *RouteResult) {
				assert.Equal(t, protocol.MessageType("unknown-message-type"), result.Message.Type)
				// May have warnings about unknown message type
			},
		},
		{
			name: "Malformed Selkies message should fallback to standard parsing",
			messageData: []byte(`{
				"sdp": "invalid-sdp-format",
				"peer_id": "selkies-client"
			}`),
			clientID:    "selkies-client",
			expectError: true, // Should fail validation
			validateFunc: func(t *testing.T, result *RouteResult) {
				// Should have error information
				assert.NotEmpty(t, result.Errors)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := router.RouteMessage(tt.messageData, tt.clientID)

			if tt.expectError {
				assert.Error(t, err)
				// Even with errors, we might still get partial results
				if result != nil && tt.validateFunc != nil {
					tt.validateFunc(t, result)
				}
				return
			}

			assert.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, result.Message)

			// 运行自定义验证函数
			if tt.validateFunc != nil {
				tt.validateFunc(t, result)
			}
		})
	}
}

// TestProtocolCompatibility_ResponseFormatting 测试不同协议的响应格式化
func TestProtocolCompatibility_ResponseFormatting(t *testing.T) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())
	require.NotNil(t, router)

	// 创建测试响应消息
	pongMessage := router.CreateStandardResponse(protocol.MessageTypePong, "test-client", map[string]any{
		"timestamp":      time.Now().Unix(),
		"server_time":    time.Now().Unix(),
		"client_id":      "test-client",
		"ping_timestamp": 1640995200,
	})

	offerMessage := router.CreateStandardResponse(protocol.MessageTypeOffer, "test-client", map[string]any{
		"type": "offer",
		"sdp":  "v=0\r\no=- 123456789 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\nc=IN IP4 0.0.0.0\r\na=rtcp:9 IN IP4 0.0.0.0\r\na=ice-ufrag:test\r\na=ice-pwd:testpassword\r\na=fingerprint:sha-256 AA:BB:CC:DD:EE:FF\r\na=setup:actpass\r\na=mid:0\r\na=sendrecv\r\na=rtcp-mux\r\na=rtpmap:96 H264/90000",
	})

	tests := []struct {
		name           string
		message        *protocol.StandardMessage
		targetProtocol protocol.ProtocolVersion
		expectError    bool
		validateFunc   func(t *testing.T, data []byte)
	}{
		{
			name:           "Format pong for GStreamer",
			message:        pongMessage,
			targetProtocol: protocol.ProtocolVersionGStreamer10,
			expectError:    false,
			validateFunc: func(t *testing.T, data []byte) {
				var response map[string]any
				err := json.Unmarshal(data, &response)
				require.NoError(t, err)

				assert.Equal(t, "pong", response["type"])
				assert.Contains(t, response, "version")
				assert.Contains(t, response, "id")
				assert.Contains(t, response, "timestamp")
				assert.Contains(t, response, "peer_id")
				assert.Contains(t, response, "data")
			},
		},
		{
			name:           "Format pong for Selkies",
			message:        pongMessage,
			targetProtocol: protocol.ProtocolVersionSelkies,
			expectError:    false,
			validateFunc: func(t *testing.T, data []byte) {
				var response map[string]any
				err := json.Unmarshal(data, &response)
				require.NoError(t, err)

				assert.Equal(t, "pong", response["type"])
				assert.Contains(t, response, "peer_id")
				assert.Contains(t, response, "data")
				// Selkies format may be simpler
			},
		},
		{
			name:           "Format offer for GStreamer",
			message:        offerMessage,
			targetProtocol: protocol.ProtocolVersionGStreamer10,
			expectError:    false,
			validateFunc: func(t *testing.T, data []byte) {
				var response map[string]any
				err := json.Unmarshal(data, &response)
				require.NoError(t, err)

				assert.Equal(t, "offer", response["type"])
				assert.Contains(t, response, "version")
				assert.Contains(t, response, "data")

				responseData, ok := response["data"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "offer", responseData["type"])
				assert.Contains(t, responseData, "sdp")
			},
		},
		{
			name:           "Format offer for Selkies",
			message:        offerMessage,
			targetProtocol: protocol.ProtocolVersionSelkies,
			expectError:    false,
			validateFunc: func(t *testing.T, data []byte) {
				var response map[string]any
				err := json.Unmarshal(data, &response)
				require.NoError(t, err)

				// Selkies format should have SDP field
				assert.Contains(t, response, "sdp")
				sdpData, ok := response["sdp"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "offer", sdpData["type"])
				assert.Contains(t, sdpData, "sdp")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := router.FormatResponse(tt.message, tt.targetProtocol)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, data)
			assert.Greater(t, len(data), 0)

			// 运行自定义验证函数
			if tt.validateFunc != nil {
				tt.validateFunc(t, data)
			}
		})
	}
}

// BenchmarkProtocolCompatibility_MessageRouting 性能基准测试
func BenchmarkProtocolCompatibility_MessageRouting(b *testing.B) {
	// 创建协议管理器和消息路由器
	protocolManager := protocol.NewProtocolManager(protocol.DefaultManagerConfig())
	router := NewMessageRouter(protocolManager, DefaultMessageRouterConfig())

	// 测试消息
	gstreamerMessage := []byte(`{
		"version": "gstreamer-1.0",
		"type": "request-offer",
		"id": "bench-msg",
		"timestamp": 1640995200,
		"peer_id": "bench-client",
		"data": {"constraints": {"video": true, "audio": true}}
	}`)

	selkiesMessage := []byte(`{
		"type": "ping",
		"peer_id": "bench-client",
		"data": {"timestamp": 1640995200}
	}`)

	b.Run("GStreamer message routing", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := router.RouteMessage(gstreamerMessage, "bench-client")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Selkies message routing", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := router.RouteMessage(selkiesMessage, "bench-client")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Protocol detection", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			router.DetectProtocol(gstreamerMessage)
		}
	})
}

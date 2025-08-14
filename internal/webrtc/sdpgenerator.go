package webrtc

import (
	"fmt"
	"strings"
	"time"
)

// SDPConfig SDP配置
type SDPConfig struct {
	Codec string
}

// SDPGenerator SDP生成器
type SDPGenerator struct {
	config *SDPConfig
}

// NewSDPGenerator 创建SDP生成器
func NewSDPGenerator(config *SDPConfig) *SDPGenerator {
	return &SDPGenerator{config: config}
}

// GenerateOffer 生成WebRTC Offer SDP
func (g *SDPGenerator) GenerateOffer() string {
	sessionId := time.Now().Unix()

	// 根据配置的编解码器生成相应的 RTP 映射
	var rtpMap string

	if g.config != nil && g.config.Codec != "" {
		switch strings.ToLower(g.config.Codec) {
		case "h264":
			rtpMap = "a=rtpmap:96 H264/90000"
		case "vp8":
			rtpMap = "a=rtpmap:96 VP8/90000"
		case "vp9":
			rtpMap = "a=rtpmap:96 VP9/90000"
		default:
			rtpMap = "a=rtpmap:96 VP8/90000"
		}
	} else {
		rtpMap = "a=rtpmap:96 VP8/90000"
	}

	// 生成完整的SDP
	sdp := fmt.Sprintf(`v=0
o=- %d 2 IN IP4 127.0.0.1
s=-
t=0 0
a=group:BUNDLE 0
a=msid-semantic: WMS
m=video 9 UDP/TLS/RTP/SAVPF 96
c=IN IP4 0.0.0.0
a=rtcp:9 IN IP4 0.0.0.0
a=ice-ufrag:4ZcD
a=ice-pwd:2/1muCWoOi3uLifh0NuQlgbD
a=ice-options:trickle
a=fingerprint:sha-256 75:74:5A:A6:A4:E5:52:F4:A7:67:4C:01:C7:EE:91:3F:21:3D:A2:E3:53:7B:6F:30:86:F2:30:FF:A6:22:D2:04
a=setup:actpass
a=mid:0
a=sendonly
a=rtcp-mux
%s
`, sessionId, rtpMap)

	return sdp
}

// ValidateSDP 验证SDP格式
func (g *SDPGenerator) ValidateSDP(sdp string) error {
	lines := strings.Split(strings.TrimSpace(sdp), "\n")

	// 检查必需的SDP行
	requiredLines := map[string]bool{
		"v=": false, // version
		"o=": false, // origin
		"s=": false, // session name
		"t=": false, // time
		"m=": false, // media
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		for prefix := range requiredLines {
			if strings.HasPrefix(line, prefix) {
				requiredLines[prefix] = true
				break
			}
		}
	}

	// 检查是否所有必需行都存在
	for prefix, found := range requiredLines {
		if !found {
			return fmt.Errorf("missing required SDP line: %s", prefix)
		}
	}

	// 检查RTP映射行
	hasRtpMap := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "a=rtpmap:") {
			hasRtpMap = true
			// 验证RTP映射格式
			if !strings.Contains(line, "/90000") {
				return fmt.Errorf("invalid RTP mapping format: %s", line)
			}
			break
		}
	}

	if !hasRtpMap {
		return fmt.Errorf("missing RTP mapping line (a=rtpmap:)")
	}

	return nil
}

// GetCodecName 获取编解码器名称
func (g *SDPGenerator) GetCodecName() string {
	if g.config != nil && g.config.Codec != "" {
		switch strings.ToLower(g.config.Codec) {
		case "h264":
			return "H264"
		case "vp8":
			return "VP8"
		case "vp9":
			return "VP9"
		default:
			return "VP8"
		}
	}
	return "VP8"
}

package events

import "time"

// WebRTC 会话生命周期事件类型
// 命名规则：webrtc.session.{action}
const (
	// 会话建立相关
	EventWebRTCSessionStarted EventType = "webrtc.session.started" // WebRTC会话开始（客户端连接）
	EventWebRTCSessionReady   EventType = "webrtc.session.ready"   // ICE连接建立，可以传输数据

	// 会话中断相关
	EventWebRTCSessionPaused  EventType = "webrtc.session.paused"  // ICE断开，等待重连
	EventWebRTCSessionResumed EventType = "webrtc.session.resumed" // ICE重连成功
	EventWebRTCSessionFailed  EventType = "webrtc.session.failed"  // ICE连接失败

	// 会话结束相关
	EventWebRTCSessionEnded     EventType = "webrtc.session.ended"      // 客户端断开
	EventWebRTCSessionTimeout   EventType = "webrtc.session.timeout"    // 重连超时
	EventWebRTCNoActiveSessions EventType = "webrtc.no_active_sessions" // 无活跃会话
)

// WebRTCSessionStartedEvent WebRTC会话开始事件
// 触发时机：客户端连接，开始信令协商
// 期望动作：启动GStreamer推流
type WebRTCSessionStartedEvent struct {
	SessionID string    `json:"session_id"`
	ClientID  string    `json:"client_id"`
	Timestamp time.Time `json:"timestamp"`
}

// WebRTCSessionReadyEvent WebRTC会话就绪事件
// 触发时机：ICE连接建立完成
// 期望动作：数据可以传输，无需额外动作
type WebRTCSessionReadyEvent struct {
	SessionID string    `json:"session_id"`
	ClientID  string    `json:"client_id"`
	Timestamp time.Time `json:"timestamp"`
}

// WebRTCSessionPausedEvent WebRTC会话暂停事件
// 触发时机：ICE连接断开
// 期望动作：启动重连定时器，继续推流
type WebRTCSessionPausedEvent struct {
	SessionID string    `json:"session_id"`
	ClientID  string    `json:"client_id"`
	Reason    string    `json:"reason"` // disconnected, failed
	Timestamp time.Time `json:"timestamp"`
}

// WebRTCSessionResumedEvent WebRTC会话恢复事件
// 触发时机：ICE重连成功
// 期望动作：取消重连定时器，继续推流
type WebRTCSessionResumedEvent struct {
	SessionID string    `json:"session_id"`
	ClientID  string    `json:"client_id"`
	Timestamp time.Time `json:"timestamp"`
}

// WebRTCSessionFailedEvent WebRTC会话失败事件
// 触发时机：ICE连接失败
// 期望动作：记录错误
type WebRTCSessionFailedEvent struct {
	SessionID string    `json:"session_id"`
	ClientID  string    `json:"client_id"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// WebRTCSessionEndedEvent WebRTC会话结束事件
// 触发时机：客户端主动断开
// 期望动作：如果无其他会话，准备停止推流
type WebRTCSessionEndedEvent struct {
	SessionID      string    `json:"session_id"`
	ClientID       string    `json:"client_id"`
	ActiveSessions int       `json:"active_sessions"` // 剩余活跃会话数
	Timestamp      time.Time `json:"timestamp"`
}

// WebRTCSessionTimeoutEvent WebRTC会话超时事件
// 触发时机：ICE断开后重连超时
// 期望动作：停止推流，清理资源
type WebRTCSessionTimeoutEvent struct {
	SessionID string        `json:"session_id"`
	ClientID  string        `json:"client_id"`
	Duration  time.Duration `json:"duration"` // 断开持续时间
	Timestamp time.Time     `json:"timestamp"`
}

// WebRTCNoActiveSessionsEvent 无活跃会话事件
// 触发时机：所有客户端断开，且空闲超时
// 期望动作：停止GStreamer推流
type WebRTCNoActiveSessionsEvent struct {
	LastSessionID string        `json:"last_session_id"`
	IdleDuration  time.Duration `json:"idle_duration"` // 空闲持续时间
	Timestamp     time.Time     `json:"timestamp"`
}

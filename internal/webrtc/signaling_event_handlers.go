package webrtc

import (
	"context"
	"fmt"

	"github.com/pion/webrtc/v4"
	"github.com/sirupsen/logrus"

	"github.com/open-beagle/bdwind-gstreamer/internal/common/events"
	webrtcEvents "github.com/open-beagle/bdwind-gstreamer/internal/webrtc/events"
)

// SignalingEventHandlers WebRTC信令事件处理器
type SignalingEventHandlers struct {
	webrtcManager *WebRTCManager
	logger        *logrus.Entry
}

// NewSignalingEventHandlers 创建信令事件处理器
func NewSignalingEventHandlers(webrtcManager *WebRTCManager) *SignalingEventHandlers {
	return &SignalingEventHandlers{
		webrtcManager: webrtcManager,
		logger:        logrus.WithField("component", "signaling-event-handlers"),
	}
}

// RegisterHandlers 注册事件处理器到事件总线
func (h *SignalingEventHandlers) RegisterHandlers(eventBus events.EventBus) error {
	// 注册 CreateOffer 事件处理器
	if err := eventBus.Subscribe(webrtcEvents.EventCreateOffer, events.EventHandlerFunc(h.HandleCreateOffer)); err != nil {
		return fmt.Errorf("failed to subscribe to CreateOffer event: %w", err)
	}

	// 注册 ProcessAnswer 事件处理器
	if err := eventBus.Subscribe(webrtcEvents.EventProcessAnswer, events.EventHandlerFunc(h.HandleProcessAnswer)); err != nil {
		return fmt.Errorf("failed to subscribe to ProcessAnswer event: %w", err)
	}

	// 注册 AddICECandidate 事件处理器
	if err := eventBus.Subscribe(webrtcEvents.EventAddICECandidate, events.EventHandlerFunc(h.HandleAddICECandidate)); err != nil {
		return fmt.Errorf("failed to subscribe to AddICECandidate event: %w", err)
	}

	h.logger.Info("Signaling event handlers registered successfully")
	return nil
}

// HandleCreateOffer 处理创建 Offer 事件
func (h *SignalingEventHandlers) HandleCreateOffer(ctx context.Context, event events.Event) (*events.EventResult, error) {
	h.logger.Debugf("Handling CreateOffer event for session: %s", event.SessionID())

	if h.webrtcManager == nil {
		h.logger.Error("WebRTC manager not available")
		return events.ErrorResult("WebRTC manager not available", "WebRTC manager is nil"),
			fmt.Errorf("WebRTC manager not available")
	}

	// 设置当前会话ID
	h.webrtcManager.SetCurrentSessionID(event.SessionID())

	// 创建 SDP Offer
	offer, err := h.webrtcManager.CreateOffer()
	if err != nil {
		h.logger.Errorf("Failed to create offer: %v", err)
		return events.ErrorResult("Failed to create offer", err.Error()), err
	}

	h.logger.Debugf("SDP offer created successfully for session: %s", event.SessionID())

	// 返回成功结果，包含 offer 数据
	return events.SuccessResult("Offer created successfully", map[string]interface{}{
		"offer": map[string]interface{}{
			"type": offer.Type.String(),
			"sdp":  offer.SDP,
		},
	}), nil
}

// HandleProcessAnswer 处理 Answer 事件
func (h *SignalingEventHandlers) HandleProcessAnswer(ctx context.Context, event events.Event) (*events.EventResult, error) {
	h.logger.Debugf("Handling ProcessAnswer event for session: %s", event.SessionID())

	if h.webrtcManager == nil {
		h.logger.Error("WebRTC manager not available")
		return events.ErrorResult("WebRTC manager not available", "WebRTC manager is nil"),
			fmt.Errorf("WebRTC manager not available")
	}

	// 从事件数据中提取 answer
	eventData := event.Data()
	answerData, ok := eventData["answer"].(map[string]interface{})
	if !ok {
		h.logger.Error("Invalid answer data format in event")
		return events.ErrorResult("Invalid answer data", "Answer data must be a map"),
			fmt.Errorf("invalid answer data format")
	}

	sdp, ok := answerData["sdp"].(string)
	if !ok {
		h.logger.Error("Invalid SDP format in answer data")
		return events.ErrorResult("Invalid SDP format", "SDP must be a string"),
			fmt.Errorf("invalid SDP format")
	}

	// 创建 WebRTC SessionDescription
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdp,
	}

	// 设置远程描述
	if err := h.webrtcManager.SetRemoteDescription(answer); err != nil {
		h.logger.Errorf("Failed to set remote description: %v", err)
		return events.ErrorResult("Failed to set remote description", err.Error()), err
	}

	h.logger.Debugf("Answer processed successfully for session: %s", event.SessionID())

	return events.SuccessResult("Answer processed successfully", nil), nil
}

// HandleAddICECandidate 处理添加 ICE Candidate 事件
func (h *SignalingEventHandlers) HandleAddICECandidate(ctx context.Context, event events.Event) (*events.EventResult, error) {
	h.logger.Debugf("Handling AddICECandidate event for session: %s", event.SessionID())

	if h.webrtcManager == nil {
		h.logger.Error("WebRTC manager not available")
		return events.ErrorResult("WebRTC manager not available", "WebRTC manager is nil"),
			fmt.Errorf("WebRTC manager not available")
	}

	// 从事件数据中提取 candidate
	eventData := event.Data()
	candidateData, ok := eventData["candidate"].(map[string]interface{})
	if !ok {
		h.logger.Error("Invalid candidate data format in event")
		return events.ErrorResult("Invalid candidate data", "Candidate data must be a map"),
			fmt.Errorf("invalid candidate data format")
	}

	candidateStr, ok := candidateData["candidate"].(string)
	if !ok {
		h.logger.Error("Invalid candidate string format")
		return events.ErrorResult("Invalid candidate string", "Candidate must be a string"),
			fmt.Errorf("invalid candidate string format")
	}

	// 创建 WebRTC ICECandidateInit
	candidate := webrtc.ICECandidateInit{
		Candidate: candidateStr,
	}

	// 处理可选的 SDPMid
	if sdpMid, exists := candidateData["sdpMid"]; exists && sdpMid != nil {
		if sdpMidStr, ok := sdpMid.(string); ok {
			candidate.SDPMid = &sdpMidStr
		}
	}

	// 处理可选的 SDPMLineIndex
	if sdpMLineIndex, exists := candidateData["sdpMLineIndex"]; exists && sdpMLineIndex != nil {
		switch idx := sdpMLineIndex.(type) {
		case float64:
			sdpMLineIndexUint16 := uint16(idx)
			candidate.SDPMLineIndex = &sdpMLineIndexUint16
		case int:
			sdpMLineIndexUint16 := uint16(idx)
			candidate.SDPMLineIndex = &sdpMLineIndexUint16
		case *int:
			if idx != nil {
				sdpMLineIndexUint16 := uint16(*idx)
				candidate.SDPMLineIndex = &sdpMLineIndexUint16
			}
		}
	}

	// 添加 ICE candidate
	if err := h.webrtcManager.AddICECandidate(candidate); err != nil {
		h.logger.Errorf("Failed to add ICE candidate: %v", err)
		return events.ErrorResult("Failed to add ICE candidate", err.Error()), err
	}

	h.logger.Debugf("ICE candidate added successfully for session: %s", event.SessionID())

	return events.SuccessResult("ICE candidate added successfully", nil), nil
}

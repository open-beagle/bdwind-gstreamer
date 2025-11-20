package webrtc

import (
	"errors"
	"fmt"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/common/protocol"
)

// Message size limits
const (
	MaxMessageSize = 64 * 1024 // 64KB maximum message size
)

// Error constants used by SignalingClient
const (
	ErrorCodeConnectionFailed  = protocol.ErrorCodeConnectionFailed
	ErrorCodeConnectionTimeout = protocol.ErrorCodeConnectionTimeout
	ErrorCodeInvalidMessage    = protocol.ErrorCodeInvalidMessage
	ErrorCodeMessageTooLarge   = protocol.ErrorCodeMessageTooLarge
	ErrorCodeInternalError     = protocol.ErrorCodeInternalError
)

// Errors
var (
	ErrSignalingSendChannelFull = errors.New("signaling send channel is full")
)

// Type aliases for backward compatibility
type SignalingError = protocol.MessageError

// Utility functions
func generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

func generateSignalingClientID() string {
	return fmt.Sprintf("client_%d", time.Now().UnixNano())
}

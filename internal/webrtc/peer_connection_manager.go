package webrtc

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// PeerConnectionManager 管理WebRTC对等连接
type PeerConnectionManager struct {
	config      *webrtc.Configuration
	mediaStream MediaStream
	connections map[string]*PeerConnectionInfo
	mutex       sync.RWMutex
	logger      *log.Logger
}

// PeerConnectionInfo 对等连接信息
type PeerConnectionInfo struct {
	ID         string
	PC         *webrtc.PeerConnection
	CreatedAt  time.Time
	LastActive time.Time
	State      webrtc.PeerConnectionState
}

// NewPeerConnectionManager 创建对等连接管理器
func NewPeerConnectionManager(mediaStream MediaStream, iceServers []webrtc.ICEServer, logger *log.Logger) *PeerConnectionManager {
	// 如果没有提供ICE服务器，使用默认的
	if len(iceServers) == 0 {
		iceServers = []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
					"stun:stun1.l.google.com:19302",
				},
			},
		}
	}

	config := &webrtc.Configuration{
		ICEServers: iceServers,
		// 为WSL2环境优化ICE配置
		ICETransportPolicy: webrtc.ICETransportPolicyAll,
		BundlePolicy:       webrtc.BundlePolicyMaxBundle,
		RTCPMuxPolicy:      webrtc.RTCPMuxPolicyRequire,
	}

	return &PeerConnectionManager{
		config:      config,
		mediaStream: mediaStream,
		connections: make(map[string]*PeerConnectionInfo),
		logger:      logger,
	}
}

// CreatePeerConnection 为客户端创建新的对等连接
func (pcm *PeerConnectionManager) CreatePeerConnection(clientID string) (*webrtc.PeerConnection, error) {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()

	// 如果已存在连接，先关闭
	if existing, exists := pcm.connections[clientID]; exists {
		existing.PC.Close()
		delete(pcm.connections, clientID)
	}

	// 创建新的PeerConnection
	pc, err := webrtc.NewPeerConnection(*pcm.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	// 添加媒体轨道
	if pcm.mediaStream != nil {
		videoTrack := pcm.mediaStream.GetVideoTrack()
		if videoTrack != nil {
			_, err = pc.AddTrack(videoTrack)
			if err != nil {
				pc.Close()
				return nil, fmt.Errorf("failed to add video track: %w", err)
			}
			pcm.logger.Printf("Video track added to peer connection for client %s", clientID)
		}

		audioTrack := pcm.mediaStream.GetAudioTrack()
		if audioTrack != nil {
			_, err = pc.AddTrack(audioTrack)
			if err != nil {
				pc.Close()
				return nil, fmt.Errorf("failed to add audio track: %w", err)
			}
			pcm.logger.Printf("Audio track added to peer connection for client %s", clientID)
		}
	}

	// 设置事件处理器
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		pcm.logger.Printf("Client %s connection state changed: %s", clientID, state.String())
		pcm.updateConnectionState(clientID, state)
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		pcm.logger.Printf("Client %s ICE connection state changed: %s", clientID, state.String())
	})

	// ICE候选处理将在信令服务器中设置

	// 处理数据通道
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		pcm.logger.Printf("Data channel created for client %s: %s", clientID, dc.Label())
	})

	// 存储连接信息
	pcm.connections[clientID] = &PeerConnectionInfo{
		ID:         clientID,
		PC:         pc,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
		State:      webrtc.PeerConnectionStateNew,
	}

	pcm.logger.Printf("Created peer connection for client %s", clientID)
	return pc, nil
}

// GetPeerConnection 获取客户端的对等连接
func (pcm *PeerConnectionManager) GetPeerConnection(clientID string) (*webrtc.PeerConnection, bool) {
	pcm.mutex.RLock()
	defer pcm.mutex.RUnlock()

	if info, exists := pcm.connections[clientID]; exists {
		return info.PC, true
	}
	return nil, false
}

// RemovePeerConnection 移除客户端的对等连接
func (pcm *PeerConnectionManager) RemovePeerConnection(clientID string) error {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()

	if info, exists := pcm.connections[clientID]; exists {
		info.PC.Close()
		delete(pcm.connections, clientID)
		pcm.logger.Printf("Removed peer connection for client %s", clientID)
		return nil
	}

	return fmt.Errorf("peer connection not found for client %s", clientID)
}

// updateConnectionState 更新连接状态
func (pcm *PeerConnectionManager) updateConnectionState(clientID string, state webrtc.PeerConnectionState) {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()

	if info, exists := pcm.connections[clientID]; exists {
		info.State = state
		info.LastActive = time.Now()

		// 如果连接失败或关闭，清理连接
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			delete(pcm.connections, clientID)
			pcm.logger.Printf("Cleaned up peer connection for client %s due to state: %s", clientID, state.String())
		}
	}
}

// GetAllConnections 获取所有连接信息
func (pcm *PeerConnectionManager) GetAllConnections() map[string]*PeerConnectionInfo {
	pcm.mutex.RLock()
	defer pcm.mutex.RUnlock()

	result := make(map[string]*PeerConnectionInfo)
	for id, info := range pcm.connections {
		result[id] = &PeerConnectionInfo{
			ID:         info.ID,
			PC:         info.PC,
			CreatedAt:  info.CreatedAt,
			LastActive: info.LastActive,
			State:      info.State,
		}
	}
	return result
}

// CleanupInactiveConnections 清理不活跃的连接
func (pcm *PeerConnectionManager) CleanupInactiveConnections(timeout time.Duration) {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()

	now := time.Now()
	for clientID, info := range pcm.connections {
		if now.Sub(info.LastActive) > timeout {
			pcm.logger.Printf("Cleaning up inactive connection for client %s", clientID)
			info.PC.Close()
			delete(pcm.connections, clientID)
		}
	}
}

// Close 关闭所有连接
func (pcm *PeerConnectionManager) Close() error {
	pcm.mutex.Lock()
	defer pcm.mutex.Unlock()

	for clientID, info := range pcm.connections {
		info.PC.Close()
		pcm.logger.Printf("Closed peer connection for client %s", clientID)
	}

	pcm.connections = make(map[string]*PeerConnectionInfo)
	return nil
}

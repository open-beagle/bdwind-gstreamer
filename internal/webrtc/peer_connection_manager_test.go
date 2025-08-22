package webrtc

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockMediaStream 模拟媒体流
type MockMediaStream struct {
	mock.Mock
}

func (m *MockMediaStream) AddTrack(track *webrtc.TrackLocalStaticSample) error {
	args := m.Called(track)
	return args.Error(0)
}

func (m *MockMediaStream) RemoveTrack(trackID string) error {
	args := m.Called(trackID)
	return args.Error(0)
}

func (m *MockMediaStream) WriteVideo(sample media.Sample) error {
	args := m.Called(sample)
	return args.Error(0)
}

func (m *MockMediaStream) WriteAudio(sample media.Sample) error {
	args := m.Called(sample)
	return args.Error(0)
}

func (m *MockMediaStream) GetVideoTrack() *webrtc.TrackLocalStaticSample {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*webrtc.TrackLocalStaticSample)
}

func (m *MockMediaStream) GetAudioTrack() *webrtc.TrackLocalStaticSample {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*webrtc.TrackLocalStaticSample)
}

func (m *MockMediaStream) GetTrackState(trackID string) (TrackState, error) {
	args := m.Called(trackID)
	return args.Get(0).(TrackState), args.Error(1)
}

func (m *MockMediaStream) GetAllTracks() []TrackInfo {
	args := m.Called()
	return args.Get(0).([]TrackInfo)
}

func (m *MockMediaStream) StartTrackMonitoring() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockMediaStream) StopTrackMonitoring() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockMediaStream) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockMediaStream) GetStats() MediaStreamStats {
	args := m.Called()
	return args.Get(0).(MediaStreamStats)
}

// TestPeerConnectionManager_CreatePeerConnection 测试创建PeerConnection
func TestPeerConnectionManager_CreatePeerConnection(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)

	// 测试用例
	tests := []struct {
		name     string
		clientID string
	}{
		{
			name:     "Create first PeerConnection",
			clientID: "client-1",
		},
		{
			name:     "Create second PeerConnection",
			clientID: "client-2",
		},
		{
			name:     "Create third PeerConnection",
			clientID: "client-3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建PeerConnection
			pc, err := pcManager.CreatePeerConnection(tt.clientID)
			assert.NoError(t, err)
			require.NotNil(t, pc)

			// 验证PeerConnection状态
			assert.Equal(t, webrtc.PeerConnectionStateNew, pc.ConnectionState())

			// 验证PeerConnection在管理器中存在
			retrievedPC, exists := pcManager.GetPeerConnection(tt.clientID)
			assert.True(t, exists)
			assert.Equal(t, pc, retrievedPC)

			// 验证连接信息
			allConnections := pcManager.GetAllConnections()
			assert.Contains(t, allConnections, tt.clientID)

			connInfo := allConnections[tt.clientID]
			assert.Equal(t, tt.clientID, connInfo.ID)
			assert.Equal(t, pc, connInfo.PC)
			assert.Equal(t, webrtc.PeerConnectionStateNew, connInfo.State)
			assert.WithinDuration(t, time.Now(), connInfo.CreatedAt, 1*time.Second)
			assert.WithinDuration(t, time.Now(), connInfo.LastActive, 1*time.Second)
		})
	}

	// 清理
	err := pcManager.Close()
	assert.NoError(t, err)
}

// TestPeerConnectionManager_CreateOfferFlow 测试完整的offer创建流程
func TestPeerConnectionManager_CreateOfferFlow(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)

	clientID := "offer-test-client"

	// 创建PeerConnection
	pc, err := pcManager.CreatePeerConnection(clientID)
	assert.NoError(t, err)
	require.NotNil(t, pc)

	// 测试创建offer
	offer, err := pc.CreateOffer(nil)
	assert.NoError(t, err)
	require.NotNil(t, offer)

	// 验证offer属性
	assert.Equal(t, webrtc.SDPTypeOffer, offer.Type)
	assert.NotEmpty(t, offer.SDP)
	assert.Contains(t, offer.SDP, "v=0") // SDP版本行
	assert.Contains(t, offer.SDP, "o=")  // Origin行

	// 测试设置本地描述
	err = pc.SetLocalDescription(offer)
	assert.NoError(t, err)

	// 验证本地描述已设置
	localDesc := pc.LocalDescription()
	require.NotNil(t, localDesc)
	assert.Equal(t, webrtc.SDPTypeOffer, localDesc.Type)
	assert.Equal(t, offer.SDP, localDesc.SDP)

	// 清理
	err = pcManager.Close()
	assert.NoError(t, err)
}

// TestPeerConnectionManager_RecreatePeerConnection 测试重新创建PeerConnection
func TestPeerConnectionManager_RecreatePeerConnection(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)

	clientID := "recreate-test-client"

	// 创建第一个PeerConnection
	pc1, err := pcManager.CreatePeerConnection(clientID)
	assert.NoError(t, err)
	require.NotNil(t, pc1)

	// 验证第一个连接存在
	retrievedPC1, exists := pcManager.GetPeerConnection(clientID)
	assert.True(t, exists)
	assert.Equal(t, pc1, retrievedPC1)

	// 重新创建PeerConnection（应该替换现有的）
	pc2, err := pcManager.CreatePeerConnection(clientID)
	assert.NoError(t, err)
	require.NotNil(t, pc2)

	// 验证新连接替换了旧连接
	retrievedPC2, exists := pcManager.GetPeerConnection(clientID)
	assert.True(t, exists)
	assert.Equal(t, pc2, retrievedPC2)
	assert.NotEqual(t, pc1, pc2) // 应该是不同的实例

	// 验证只有一个连接存在
	allConnections := pcManager.GetAllConnections()
	assert.Len(t, allConnections, 1)
	assert.Contains(t, allConnections, clientID)

	// 清理
	err = pcManager.Close()
	assert.NoError(t, err)
}

// TestPeerConnectionManager_RemovePeerConnection 测试移除PeerConnection
func TestPeerConnectionManager_RemovePeerConnection(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)

	clientID := "remove-test-client"

	// 创建PeerConnection
	pc, err := pcManager.CreatePeerConnection(clientID)
	assert.NoError(t, err)
	require.NotNil(t, pc)

	// 验证连接存在
	_, exists := pcManager.GetPeerConnection(clientID)
	assert.True(t, exists)

	// 移除连接
	err = pcManager.RemovePeerConnection(clientID)
	assert.NoError(t, err)

	// 验证连接已被移除
	_, exists = pcManager.GetPeerConnection(clientID)
	assert.False(t, exists)

	// 验证连接不在所有连接列表中
	allConnections := pcManager.GetAllConnections()
	assert.NotContains(t, allConnections, clientID)

	// 测试移除不存在的连接
	err = pcManager.RemovePeerConnection("non-existent-client")
	assert.Error(t, err)

	// 清理
	err = pcManager.Close()
	assert.NoError(t, err)
}

// TestPeerConnectionManager_GetAllConnections 测试获取所有连接
func TestPeerConnectionManager_GetAllConnections(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)

	// 初始状态应该没有连接
	allConnections := pcManager.GetAllConnections()
	assert.Empty(t, allConnections)

	// 创建多个连接
	clientIDs := []string{"client-1", "client-2", "client-3"}
	createdPCs := make(map[string]*webrtc.PeerConnection)

	for _, clientID := range clientIDs {
		pc, err := pcManager.CreatePeerConnection(clientID)
		assert.NoError(t, err)
		require.NotNil(t, pc)
		createdPCs[clientID] = pc
	}

	// 验证所有连接都存在
	allConnections = pcManager.GetAllConnections()
	assert.Len(t, allConnections, len(clientIDs))

	for _, clientID := range clientIDs {
		assert.Contains(t, allConnections, clientID)

		connInfo := allConnections[clientID]
		assert.Equal(t, clientID, connInfo.ID)
		assert.Equal(t, createdPCs[clientID], connInfo.PC)
		assert.Equal(t, webrtc.PeerConnectionStateNew, connInfo.State)
		assert.WithinDuration(t, time.Now(), connInfo.CreatedAt, 1*time.Second)
		assert.WithinDuration(t, time.Now(), connInfo.LastActive, 1*time.Second)
	}

	// 移除一个连接
	err := pcManager.RemovePeerConnection(clientIDs[0])
	assert.NoError(t, err)

	// 验证连接数量减少
	allConnections = pcManager.GetAllConnections()
	assert.Len(t, allConnections, len(clientIDs)-1)
	assert.NotContains(t, allConnections, clientIDs[0])

	// 清理
	err = pcManager.Close()
	assert.NoError(t, err)
}

// TestPeerConnectionManager_CleanupInactiveConnections 测试清理不活跃连接
func TestPeerConnectionManager_CleanupInactiveConnections(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)

	// 创建连接
	activeClientID := "active-client"
	inactiveClientID := "inactive-client"

	activePC, err := pcManager.CreatePeerConnection(activeClientID)
	assert.NoError(t, err)
	require.NotNil(t, activePC)

	inactivePC, err := pcManager.CreatePeerConnection(inactiveClientID)
	assert.NoError(t, err)
	require.NotNil(t, inactivePC)

	// 验证两个连接都存在
	allConnections := pcManager.GetAllConnections()
	assert.Len(t, allConnections, 2)

	// 模拟不活跃连接（通过直接修改LastActive时间）
	pcManager.mutex.Lock()
	if connInfo, exists := pcManager.connections[inactiveClientID]; exists {
		connInfo.LastActive = time.Now().Add(-2 * time.Hour) // 2小时前
	}
	pcManager.mutex.Unlock()

	// 清理不活跃连接（超时时间设为1小时）
	pcManager.CleanupInactiveConnections(1 * time.Hour)

	// 验证不活跃连接被清理
	allConnections = pcManager.GetAllConnections()
	assert.Len(t, allConnections, 1)
	assert.Contains(t, allConnections, activeClientID)
	assert.NotContains(t, allConnections, inactiveClientID)

	// 验证活跃连接仍然存在
	_, exists := pcManager.GetPeerConnection(activeClientID)
	assert.True(t, exists)

	// 验证不活跃连接已被移除
	_, exists = pcManager.GetPeerConnection(inactiveClientID)
	assert.False(t, exists)

	// 清理
	err = pcManager.Close()
	assert.NoError(t, err)
}

// TestPeerConnectionManager_Close 测试关闭管理器
func TestPeerConnectionManager_Close(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)

	// 创建多个连接
	clientIDs := []string{"client-1", "client-2", "client-3"}

	for _, clientID := range clientIDs {
		pc, err := pcManager.CreatePeerConnection(clientID)
		assert.NoError(t, err)
		require.NotNil(t, pc)
	}

	// 验证连接存在
	allConnections := pcManager.GetAllConnections()
	assert.Len(t, allConnections, len(clientIDs))

	// 关闭管理器
	err := pcManager.Close()
	assert.NoError(t, err)

	// 验证所有连接都被清理
	allConnections = pcManager.GetAllConnections()
	assert.Empty(t, allConnections)

	// 验证无法获取任何连接
	for _, clientID := range clientIDs {
		_, exists := pcManager.GetPeerConnection(clientID)
		assert.False(t, exists)
	}
}

// TestPeerConnectionManager_WithMediaTracks 测试带媒体轨道的PeerConnection创建
func TestPeerConnectionManager_WithMediaTracks(t *testing.T) {
	// 创建模拟视频轨道
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
	require.NoError(t, err)

	// 创建模拟音频轨道
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
	require.NoError(t, err)

	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(videoTrack)
	mockMediaStream.On("GetAudioTrack").Return(audioTrack)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)

	clientID := "media-test-client"

	// 创建PeerConnection
	pc, err := pcManager.CreatePeerConnection(clientID)
	assert.NoError(t, err)
	require.NotNil(t, pc)

	// 验证轨道被添加（通过创建offer来间接验证）
	offer, err := pc.CreateOffer(nil)
	assert.NoError(t, err)
	require.NotNil(t, offer)

	// SDP应该包含媒体描述
	assert.Contains(t, offer.SDP, "m=video") // 视频媒体行
	assert.Contains(t, offer.SDP, "m=audio") // 音频媒体行

	// 验证mock被调用
	mockMediaStream.AssertExpectations(t)

	// 清理
	err = pcManager.Close()
	assert.NoError(t, err)
}

// TestPeerConnectionManager_ConnectionStateHandling 测试连接状态处理
func TestPeerConnectionManager_ConnectionStateHandling(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, []webrtc.ICEServer{}, logger)
	require.NotNil(t, pcManager)

	clientID := "state-test-client"

	// 创建PeerConnection
	pc, err := pcManager.CreatePeerConnection(clientID)
	assert.NoError(t, err)
	require.NotNil(t, pc)

	// 验证初始状态
	allConnections := pcManager.GetAllConnections()
	assert.Contains(t, allConnections, clientID)
	assert.Equal(t, webrtc.PeerConnectionStateNew, allConnections[clientID].State)

	// 模拟状态变化到连接中
	pcManager.updateConnectionState(clientID, webrtc.PeerConnectionStateConnecting)

	// 验证状态更新
	allConnections = pcManager.GetAllConnections()
	assert.Contains(t, allConnections, clientID)
	assert.Equal(t, webrtc.PeerConnectionStateConnecting, allConnections[clientID].State)

	// 模拟状态变化到已连接
	pcManager.updateConnectionState(clientID, webrtc.PeerConnectionStateConnected)

	// 验证状态更新
	allConnections = pcManager.GetAllConnections()
	assert.Contains(t, allConnections, clientID)
	assert.Equal(t, webrtc.PeerConnectionStateConnected, allConnections[clientID].State)

	// 模拟状态变化到失败（应该自动清理）
	pcManager.updateConnectionState(clientID, webrtc.PeerConnectionStateFailed)

	// 验证连接被自动清理
	allConnections = pcManager.GetAllConnections()
	assert.NotContains(t, allConnections, clientID)

	// 验证连接不再存在
	_, exists := pcManager.GetPeerConnection(clientID)
	assert.False(t, exists)

	// 清理
	err = pcManager.Close()
	assert.NoError(t, err)
}

// TestPeerConnectionManager_DefaultICEServers 测试默认ICE服务器
func TestPeerConnectionManager_DefaultICEServers(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 创建PeerConnection管理器（不提供ICE服务器）
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, nil, logger)
	require.NotNil(t, pcManager)

	// 验证默认ICE服务器被设置
	assert.NotNil(t, pcManager.config)
	assert.NotEmpty(t, pcManager.config.ICEServers)

	// 验证包含Google STUN服务器
	foundGoogleSTUN := false
	for _, server := range pcManager.config.ICEServers {
		for _, url := range server.URLs {
			if url == "stun:stun.l.google.com:19302" {
				foundGoogleSTUN = true
				break
			}
		}
		if foundGoogleSTUN {
			break
		}
	}
	assert.True(t, foundGoogleSTUN, "Should include Google STUN server by default")

	// 清理
	err := pcManager.Close()
	assert.NoError(t, err)
}

// TestPeerConnectionManager_CustomICEServers 测试自定义ICE服务器
func TestPeerConnectionManager_CustomICEServers(t *testing.T) {
	// 创建模拟媒体流
	mockMediaStream := &MockMediaStream{}
	mockMediaStream.On("GetVideoTrack").Return(nil)
	mockMediaStream.On("GetAudioTrack").Return(nil)

	// 自定义ICE服务器
	customICEServers := []webrtc.ICEServer{
		{
			URLs: []string{"stun:custom-stun.example.com:3478"},
		},
		{
			URLs:       []string{"turn:custom-turn.example.com:3478"},
			Username:   "testuser",
			Credential: "testpass",
		},
	}

	// 创建PeerConnection管理器
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	pcManager := NewPeerConnectionManager(mockMediaStream, customICEServers, logger)
	require.NotNil(t, pcManager)

	// 验证自定义ICE服务器被设置
	assert.NotNil(t, pcManager.config)
	assert.Equal(t, customICEServers, pcManager.config.ICEServers)

	// 清理
	err := pcManager.Close()
	assert.NoError(t, err)
}

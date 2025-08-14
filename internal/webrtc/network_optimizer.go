package webrtc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// NetworkOptimizerConfig 网络优化器配置
type NetworkOptimizerConfig struct {
	// 拥塞控制配置
	CongestionControlEnabled bool          `yaml:"congestion_control_enabled"`
	CongestionThreshold      float64       `yaml:"congestion_threshold"`     // 丢包率阈值
	CongestionRecoveryTime   time.Duration `yaml:"congestion_recovery_time"` // 拥塞恢复时间

	// 自适应比特率配置
	AdaptiveBitrateEnabled bool    `yaml:"adaptive_bitrate_enabled"`
	MinBitrate             int     `yaml:"min_bitrate"`       // 最小比特率 (kbps)
	MaxBitrate             int     `yaml:"max_bitrate"`       // 最大比特率 (kbps)
	BitrateStepSize        int     `yaml:"bitrate_step_size"` // 比特率调整步长 (kbps)
	QualityThreshold       float64 `yaml:"quality_threshold"` // 质量阈值

	// 丢包恢复配置
	PacketLossRecoveryEnabled bool          `yaml:"packet_loss_recovery_enabled"`
	RetransmissionTimeout     time.Duration `yaml:"retransmission_timeout"`
	MaxRetransmissions        int           `yaml:"max_retransmissions"`
	FECEnabled                bool          `yaml:"fec_enabled"` // Forward Error Correction

	// 监控配置
	StatsUpdateInterval time.Duration `yaml:"stats_update_interval"`
	RTTSmoothingFactor  float64       `yaml:"rtt_smoothing_factor"`
}

// DefaultNetworkOptimizerConfig 返回默认网络优化器配置
func DefaultNetworkOptimizerConfig() *NetworkOptimizerConfig {
	return &NetworkOptimizerConfig{
		CongestionControlEnabled: true,
		CongestionThreshold:      0.05, // 5% 丢包率
		CongestionRecoveryTime:   time.Second * 5,

		AdaptiveBitrateEnabled: true,
		MinBitrate:             500,  // 500 kbps
		MaxBitrate:             5000, // 5 Mbps
		BitrateStepSize:        200,  // 200 kbps
		QualityThreshold:       0.8,

		PacketLossRecoveryEnabled: true,
		RetransmissionTimeout:     time.Millisecond * 100,
		MaxRetransmissions:        3,
		FECEnabled:                true,

		StatsUpdateInterval: time.Second * 2,
		RTTSmoothingFactor:  0.125, // RFC 6298
	}
}

// NetworkOptimizer 网络优化器接口
type NetworkOptimizer interface {
	// Start 启动网络优化器
	Start() error

	// Stop 停止网络优化器
	Stop() error

	// SetPeerConnection 设置 WebRTC 连接
	SetPeerConnection(pc *webrtc.PeerConnection) error

	// UpdateNetworkStats 更新网络统计信息
	UpdateNetworkStats(stats NetworkStats) error

	// GetOptimizationStats 获取优化统计信息
	GetOptimizationStats() OptimizationStats

	// OnBitrateChange 比特率变化回调
	OnBitrateChange(callback func(bitrate int)) error
}

// NetworkStats 网络统计信息
type NetworkStats struct {
	PacketsSent int64
	PacketsLost int64
	BytesSent   int64
	RTT         time.Duration
	Jitter      time.Duration
	Bandwidth   int64 // bps
	Timestamp   time.Time
}

// OptimizationStats 优化统计信息
type OptimizationStats struct {
	CurrentBitrate       int
	TargetBitrate        int
	PacketLossRate       float64
	CongestionDetected   bool
	BitrateAdjustments   int64
	RetransmissionCount  int64
	FECPacketsSent       int64
	AverageRTT           time.Duration
	NetworkQualityScore  float64
	LastOptimizationTime time.Time
}

// networkOptimizerImpl 网络优化器实现
type networkOptimizerImpl struct {
	config         *NetworkOptimizerConfig
	peerConnection *webrtc.PeerConnection

	// 状态管理
	running bool
	mutex   sync.RWMutex

	// 统计信息
	currentStats  NetworkStats
	optStats      OptimizationStats
	statsHistory  []NetworkStats
	maxHistoryLen int

	// 拥塞控制状态
	congestionState    CongestionState
	lastCongestionTime time.Time

	// 自适应比特率状态
	currentBitrate int
	targetBitrate  int
	bitrateHistory []int

	// 回调函数
	bitrateCallback func(int)

	// 上下文和控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// CongestionState 拥塞状态
type CongestionState int

const (
	CongestionStateNormal CongestionState = iota
	CongestionStateDetected
	CongestionStateRecovering
)

// String returns the string representation of CongestionState
func (cs CongestionState) String() string {
	switch cs {
	case CongestionStateNormal:
		return "normal"
	case CongestionStateDetected:
		return "detected"
	case CongestionStateRecovering:
		return "recovering"
	default:
		return "unknown"
	}
}

// NewNetworkOptimizer 创建新的网络优化器
func NewNetworkOptimizer(config *NetworkOptimizerConfig) NetworkOptimizer {
	if config == nil {
		config = DefaultNetworkOptimizerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &networkOptimizerImpl{
		config:          config,
		maxHistoryLen:   100, // 保留最近100个统计记录
		congestionState: CongestionStateNormal,
		currentBitrate:  (config.MinBitrate + config.MaxBitrate) / 2, // 初始比特率设为中间值
		targetBitrate:   (config.MinBitrate + config.MaxBitrate) / 2,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// Start 启动网络优化器
func (no *networkOptimizerImpl) Start() error {
	no.mutex.Lock()
	defer no.mutex.Unlock()

	if no.running {
		return fmt.Errorf("network optimizer already running")
	}

	if no.peerConnection == nil {
		return fmt.Errorf("peer connection not set")
	}

	no.running = true

	// 启动统计监控协程
	no.wg.Add(1)
	go no.monitorNetworkStats()

	// 启动优化协程
	no.wg.Add(1)
	go no.optimizeNetwork()

	return nil
}

// Stop 停止网络优化器
func (no *networkOptimizerImpl) Stop() error {
	no.mutex.Lock()
	if !no.running {
		no.mutex.Unlock()
		return nil
	}
	no.running = false
	no.mutex.Unlock()

	// 取消上下文
	no.cancel()

	// 等待所有协程结束
	no.wg.Wait()

	return nil
}

// SetPeerConnection 设置 WebRTC 连接
func (no *networkOptimizerImpl) SetPeerConnection(pc *webrtc.PeerConnection) error {
	no.mutex.Lock()
	defer no.mutex.Unlock()

	no.peerConnection = pc
	return nil
}

// UpdateNetworkStats 更新网络统计信息
func (no *networkOptimizerImpl) UpdateNetworkStats(stats NetworkStats) error {
	no.mutex.Lock()
	defer no.mutex.Unlock()

	no.currentStats = stats

	// 添加到历史记录
	no.statsHistory = append(no.statsHistory, stats)
	if len(no.statsHistory) > no.maxHistoryLen {
		no.statsHistory = no.statsHistory[1:]
	}

	// 更新优化统计信息
	no.updateOptimizationStats()

	return nil
}

// GetOptimizationStats 获取优化统计信息
func (no *networkOptimizerImpl) GetOptimizationStats() OptimizationStats {
	no.mutex.RLock()
	defer no.mutex.RUnlock()

	// Update current stats with latest values
	stats := no.optStats
	stats.CurrentBitrate = no.currentBitrate
	stats.TargetBitrate = no.targetBitrate

	return stats
}

// OnBitrateChange 比特率变化回调
func (no *networkOptimizerImpl) OnBitrateChange(callback func(bitrate int)) error {
	no.mutex.Lock()
	defer no.mutex.Unlock()

	no.bitrateCallback = callback
	return nil
}

// monitorNetworkStats 监控网络统计信息的协程
func (no *networkOptimizerImpl) monitorNetworkStats() {
	defer no.wg.Done()

	ticker := time.NewTicker(no.config.StatsUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			no.collectNetworkStats()
		case <-no.ctx.Done():
			return
		}
	}
}

// optimizeNetwork 网络优化的协程
func (no *networkOptimizerImpl) optimizeNetwork() {
	defer no.wg.Done()

	ticker := time.NewTicker(no.config.StatsUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			no.performOptimization()
		case <-no.ctx.Done():
			return
		}
	}
}

// collectNetworkStats 收集网络统计信息
func (no *networkOptimizerImpl) collectNetworkStats() {
	if no.peerConnection == nil {
		return
	}

	// 获取 WebRTC 统计信息
	stats := no.peerConnection.GetStats()

	// 处理统计信息并更新
	networkStats := no.processWebRTCStats(stats)
	no.UpdateNetworkStats(networkStats)
}

// processWebRTCStats 处理 WebRTC 统计信息
func (no *networkOptimizerImpl) processWebRTCStats(stats webrtc.StatsReport) NetworkStats {
	var networkStats NetworkStats
	networkStats.Timestamp = time.Now()

	// 遍历统计报告
	for _, stat := range stats {
		switch s := stat.(type) {
		case *webrtc.OutboundRTPStreamStats:
			networkStats.PacketsSent = int64(s.PacketsSent)
			networkStats.BytesSent = int64(s.BytesSent)
		case *webrtc.RemoteInboundRTPStreamStats:
			networkStats.PacketsLost = int64(s.PacketsLost)
			if s.RoundTripTime > 0 {
				networkStats.RTT = time.Duration(s.RoundTripTime * float64(time.Second))
			}
			if s.Jitter > 0 {
				networkStats.Jitter = time.Duration(s.Jitter * float64(time.Second))
			}
		}
	}

	return networkStats
}

// performOptimization 执行网络优化
func (no *networkOptimizerImpl) performOptimization() {
	no.mutex.Lock()
	defer no.mutex.Unlock()

	// 拥塞控制
	if no.config.CongestionControlEnabled {
		no.performCongestionControl()
	}

	// 自适应比特率
	if no.config.AdaptiveBitrateEnabled {
		no.performAdaptiveBitrate()
	}

	// 丢包恢复
	if no.config.PacketLossRecoveryEnabled {
		no.performPacketLossRecovery()
	}

	no.optStats.LastOptimizationTime = time.Now()
}

// performCongestionControl 执行拥塞控制
func (no *networkOptimizerImpl) performCongestionControl() {
	if len(no.statsHistory) < 2 {
		return
	}

	// 计算丢包率
	recent := no.statsHistory[len(no.statsHistory)-1]
	previous := no.statsHistory[len(no.statsHistory)-2]

	packetsSent := recent.PacketsSent - previous.PacketsSent
	packetsLost := recent.PacketsLost - previous.PacketsLost

	var lossRate float64
	if packetsSent > 0 {
		lossRate = float64(packetsLost) / float64(packetsSent)
	}

	no.optStats.PacketLossRate = lossRate

	// 拥塞检测
	if lossRate > no.config.CongestionThreshold {
		if no.congestionState == CongestionStateNormal {
			no.congestionState = CongestionStateDetected
			no.lastCongestionTime = time.Now()
			no.optStats.CongestionDetected = true

			// 降低比特率
			newBitrate := int(float64(no.currentBitrate) * 0.8) // 降低20%
			if newBitrate < no.config.MinBitrate {
				newBitrate = no.config.MinBitrate
			}
			no.adjustBitrate(newBitrate)
		}
	} else {
		// 拥塞恢复
		if no.congestionState == CongestionStateDetected {
			if time.Since(no.lastCongestionTime) > no.config.CongestionRecoveryTime {
				no.congestionState = CongestionStateRecovering
				no.optStats.CongestionDetected = false
			}
		} else if no.congestionState == CongestionStateRecovering {
			// 逐渐恢复比特率
			if no.currentBitrate < no.targetBitrate {
				newBitrate := no.currentBitrate + no.config.BitrateStepSize
				if newBitrate > no.targetBitrate {
					newBitrate = no.targetBitrate
				}
				no.adjustBitrate(newBitrate)

				if newBitrate >= no.targetBitrate {
					no.congestionState = CongestionStateNormal
				}
			}
		}
	}
}

// performAdaptiveBitrate 执行自适应比特率
func (no *networkOptimizerImpl) performAdaptiveBitrate() {
	if len(no.statsHistory) < 5 {
		return
	}

	// 计算网络质量分数
	qualityScore := no.calculateNetworkQuality()
	no.optStats.NetworkQualityScore = qualityScore

	// 根据质量分数调整目标比特率
	if qualityScore > no.config.QualityThreshold {
		// 网络质量好，可以提高比特率
		newTarget := no.targetBitrate + no.config.BitrateStepSize
		if newTarget <= no.config.MaxBitrate {
			no.targetBitrate = newTarget
		}
	} else if qualityScore < no.config.QualityThreshold*0.7 {
		// 网络质量差，降低比特率
		newTarget := no.targetBitrate - no.config.BitrateStepSize
		if newTarget >= no.config.MinBitrate {
			no.targetBitrate = newTarget
		}
	}

	no.optStats.TargetBitrate = no.targetBitrate

	// 如果没有拥塞，逐步调整到目标比特率
	if no.congestionState == CongestionStateNormal {
		if no.currentBitrate < no.targetBitrate {
			newBitrate := no.currentBitrate + no.config.BitrateStepSize/2
			if newBitrate > no.targetBitrate {
				newBitrate = no.targetBitrate
			}
			no.adjustBitrate(newBitrate)
		} else if no.currentBitrate > no.targetBitrate {
			newBitrate := no.currentBitrate - no.config.BitrateStepSize/2
			if newBitrate < no.targetBitrate {
				newBitrate = no.targetBitrate
			}
			no.adjustBitrate(newBitrate)
		}
	}
}

// performPacketLossRecovery 执行丢包恢复
func (no *networkOptimizerImpl) performPacketLossRecovery() {
	if no.optStats.PacketLossRate > 0.01 { // 1% 丢包率以上启用恢复
		// 这里可以实现重传逻辑或FEC
		// 由于WebRTC的复杂性，这里只是更新统计信息
		no.optStats.RetransmissionCount++

		if no.config.FECEnabled {
			no.optStats.FECPacketsSent++
		}
	}
}

// calculateNetworkQuality 计算网络质量分数
func (no *networkOptimizerImpl) calculateNetworkQuality() float64 {
	if len(no.statsHistory) < 5 {
		return 0.5 // 默认中等质量
	}

	// 获取最近的统计信息
	recent := no.statsHistory[len(no.statsHistory)-5:]

	// 计算平均RTT
	var totalRTT time.Duration
	var rttCount int
	for _, stat := range recent {
		if stat.RTT > 0 {
			totalRTT += stat.RTT
			rttCount++
		}
	}

	var avgRTT time.Duration
	if rttCount > 0 {
		avgRTT = totalRTT / time.Duration(rttCount)
		no.optStats.AverageRTT = avgRTT
	}

	// 计算质量分数 (0-1)
	qualityScore := 1.0

	// RTT影响 (RTT越低质量越好)
	if avgRTT > 0 {
		rttScore := 1.0 - float64(avgRTT.Milliseconds())/1000.0 // 1秒RTT为0分
		if rttScore < 0 {
			rttScore = 0
		}
		qualityScore *= rttScore
	}

	// 丢包率影响
	lossScore := 1.0 - no.optStats.PacketLossRate*10 // 10%丢包率为0分
	if lossScore < 0 {
		lossScore = 0
	}
	qualityScore *= lossScore

	// 抖动影响
	if len(recent) > 0 && recent[len(recent)-1].Jitter > 0 {
		jitterScore := 1.0 - float64(recent[len(recent)-1].Jitter.Milliseconds())/100.0 // 100ms抖动为0分
		if jitterScore < 0 {
			jitterScore = 0
		}
		qualityScore *= jitterScore
	}

	return qualityScore
}

// adjustBitrate 调整比特率
func (no *networkOptimizerImpl) adjustBitrate(newBitrate int) {
	if newBitrate == no.currentBitrate {
		return
	}

	no.currentBitrate = newBitrate
	no.optStats.CurrentBitrate = newBitrate
	no.optStats.BitrateAdjustments++

	// 记录比特率历史
	no.bitrateHistory = append(no.bitrateHistory, newBitrate)
	if len(no.bitrateHistory) > 50 { // 保留最近50次调整
		no.bitrateHistory = no.bitrateHistory[1:]
	}

	// 调用回调函数
	if no.bitrateCallback != nil {
		go no.bitrateCallback(newBitrate)
	}
}

// updateOptimizationStats 更新优化统计信息
func (no *networkOptimizerImpl) updateOptimizationStats() {
	// 这个方法在UpdateNetworkStats中被调用，用于更新基本统计信息
	// 具体的优化统计信息在各个优化方法中更新
}

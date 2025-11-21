package interfaces

import "time"

// VideoDataSink 视频数据接收器接口
// 用于解耦 GStreamer 和 WebRTC 组件
type VideoDataSink interface {
	// SendVideoData 发送视频数据
	// data: H.264 编码的视频数据
	// timestamp: 视频帧时间戳
	SendVideoData(data []byte, timestamp time.Duration) error
}

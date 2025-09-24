# Stream Publisher Implementation

## Overview

The Stream Publisher is a core component of the GStreamer Go-GST refactor that implements a publish-subscribe pattern to replace complex callback chains. This component provides asynchronous media stream distribution to multiple subscribers while avoiding blocking of media processing.

## Key Features

### 1. Publish-Subscribe Pattern
- **Replaces complex callback chains** with a simple pub-sub model
- **Asynchronous publishing** to avoid blocking media processing
- **Multiple subscriber support** for video, audio, and combined media streams

### 2. Standardized Data Structures
- **EncodedVideoFrame**: Complete video frame metadata including codec, dimensions, bitrate, timestamps
- **EncodedAudioFrame**: Complete audio frame metadata including sample rate, channels, codec
- **Backward compatibility** with legacy Sample-based interface

### 3. Subscriber Management
- **VideoStreamSubscriber**: Interface for video-only subscribers
- **AudioStreamSubscriber**: Interface for audio-only subscribers  
- **MediaStreamSubscriber**: Interface for combined video+audio subscribers
- **Automatic subscriber health monitoring** and error handling

### 4. Robust Error Handling
- **Timeout protection** for slow subscribers
- **Panic recovery** from subscriber errors
- **Error notification** to subscribers
- **Automatic inactive subscriber detection**

### 5. Comprehensive Statistics
- **Frame publishing metrics** (published, dropped, errors)
- **Subscriber statistics** (count, activity, errors)
- **Performance metrics** (average frame sizes, timestamps)
- **Real-time status monitoring**

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Stream Publisher                         │
├─────────────────────────────────────────────────────────────┤
│  Video Channel ──┐                                          │
│                  ├─► Video Publish Loop ──┐                 │
│  Audio Channel ──┘                        │                 │
│                                           ▼                 │
│  ┌─────────────────────────────────────────────────────────┐│
│  │              Frame Distribution                         ││
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     ││
│  │  │   Video     │  │   Audio     │  │   Media     │     ││
│  │  │Subscribers  │  │Subscribers  │  │Subscribers  │     ││
│  │  └─────────────┘  └─────────────┘  └─────────────┘     ││
│  └─────────────────────────────────────────────────────────┘│
│                                                             │
│  ┌─────────────────────────────────────────────────────────┐│
│  │              Background Services                        ││
│  │  • Metrics Collection    • Subscriber Monitoring       ││
│  │  • Health Checking       • Error Handling              ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

## Usage Examples

### Basic Usage

```go
// Create stream publisher
ctx := context.Background()
config := DefaultStreamPublisherConfig()
logger := logrus.NewEntry(logrus.StandardLogger())

publisher := NewStreamPublisher(ctx, config, logger)

// Start the publisher
err := publisher.Start()
if err != nil {
    log.Fatal("Failed to start publisher:", err)
}
defer publisher.Stop()

// Publish video frame
videoFrame := &EncodedVideoFrame{
    Data:        encodedData,
    Timestamp:   time.Now(),
    Duration:    33 * time.Millisecond,
    KeyFrame:    true,
    Width:       1920,
    Height:      1080,
    Codec:       "h264",
    Bitrate:     2000000,
}

err = publisher.PublishVideoFrame(videoFrame)
if err != nil {
    log.Error("Failed to publish video frame:", err)
}
```

### Implementing a Subscriber

```go
type MyVideoSubscriber struct {
    id     uint64
    active bool
}

func (s *MyVideoSubscriber) OnVideoFrame(frame *EncodedVideoFrame) error {
    // Process the video frame
    log.Printf("Received video frame: %d bytes, codec: %s", 
               len(frame.Data), frame.Codec)
    return nil
}

func (s *MyVideoSubscriber) OnVideoError(err error) error {
    log.Printf("Video error: %v", err)
    return nil
}

func (s *MyVideoSubscriber) GetSubscriberID() uint64 {
    return s.id
}

func (s *MyVideoSubscriber) IsActive() bool {
    return s.active
}

// Add subscriber to publisher
subscriber := &MyVideoSubscriber{id: 1, active: true}
subscriberID, err := publisher.AddVideoSubscriber(subscriber)
if err != nil {
    log.Error("Failed to add subscriber:", err)
}
```

### Media Subscriber (Video + Audio)

```go
type MyMediaSubscriber struct {
    id     uint64
    active bool
}

func (s *MyMediaSubscriber) OnVideoFrame(frame *EncodedVideoFrame) error {
    // Handle video frame
    return nil
}

func (s *MyMediaSubscriber) OnAudioFrame(frame *EncodedAudioFrame) error {
    // Handle audio frame
    return nil
}

func (s *MyMediaSubscriber) OnVideoError(err error) error {
    return nil
}

func (s *MyMediaSubscriber) OnAudioError(err error) error {
    return nil
}

func (s *MyMediaSubscriber) OnStreamStart() error {
    log.Println("Media stream started")
    return nil
}

func (s *MyMediaSubscriber) OnStreamStop() error {
    log.Println("Media stream stopped")
    return nil
}

func (s *MyMediaSubscriber) GetSubscriberID() uint64 {
    return s.id
}

func (s *MyMediaSubscriber) IsActive() bool {
    return s.active
}
```

## Configuration

```go
type StreamPublisherConfig struct {
    VideoBufferSize int           // Size of video frame buffer (default: 100)
    AudioBufferSize int           // Size of audio frame buffer (default: 100)
    MaxSubscribers  int           // Maximum number of subscribers (default: 10)
    PublishTimeout  time.Duration // Timeout for subscriber operations (default: 1s)
    EnableMetrics   bool          // Enable metrics collection (default: true)
    MetricsInterval time.Duration // Metrics update interval (default: 30s)
}
```

## Data Structures

### EncodedVideoFrame
```go
type EncodedVideoFrame struct {
    Data        []byte        // Encoded video data
    Timestamp   time.Time     // Frame timestamp
    Duration    time.Duration // Frame duration
    KeyFrame    bool          // Whether this is a key frame
    Width       int           // Video width in pixels
    Height      int           // Video height in pixels
    Codec       string        // Video codec (h264, vp8, vp9)
    Bitrate     int           // Current bitrate
    FrameNumber uint64        // Sequential frame number
    PTS         uint64        // Presentation timestamp
    DTS         uint64        // Decode timestamp
}
```

### EncodedAudioFrame
```go
type EncodedAudioFrame struct {
    Data        []byte        // Encoded audio data
    Timestamp   time.Time     // Frame timestamp
    Duration    time.Duration // Frame duration
    SampleRate  int           // Audio sample rate
    Channels    int           // Number of audio channels
    Codec       string        // Audio codec (opus, pcm)
    Bitrate     int           // Current bitrate
    FrameNumber uint64        // Sequential frame number
    PTS         uint64        // Presentation timestamp
}
```

## Error Handling

The Stream Publisher implements comprehensive error handling:

1. **Subscriber Timeouts**: If a subscriber takes too long to process a frame, it receives a timeout error
2. **Panic Recovery**: Panics in subscriber code are caught and converted to errors
3. **Error Notification**: Subscribers are notified of errors through their error callback methods
4. **Health Monitoring**: Inactive subscribers are automatically detected and marked
5. **Statistics Tracking**: All errors are tracked in detailed statistics

## Performance Characteristics

- **Asynchronous Processing**: Frame publishing doesn't block the caller
- **Concurrent Distribution**: Frames are sent to subscribers concurrently
- **Buffer Management**: Configurable buffer sizes prevent memory issues
- **Timeout Protection**: Prevents slow subscribers from affecting others
- **Memory Efficient**: Frames are shared between subscribers (read-only)

## Integration with GStreamer Components

The Stream Publisher is designed to integrate seamlessly with other GStreamer components:

1. **Desktop Capture Component**: Publishes raw video frames
2. **Video Encoder Component**: Publishes encoded video frames
3. **Audio Processor Component**: Publishes encoded audio frames
4. **WebRTC Transport**: Subscribes to receive media streams

## Backward Compatibility

The Stream Publisher maintains backward compatibility with the legacy Sample-based interface:

```go
// Legacy sample publishing
sample := &Sample{
    Data:      encodedData,
    Timestamp: time.Now(),
    Format: SampleFormat{
        Width:  1920,
        Height: 1080,
        Codec:  "h264",
    },
}

err := publisher.PublishVideoSample(sample)
```

## Testing

The implementation includes comprehensive tests covering:

- Basic lifecycle (start/stop)
- Subscriber management (add/remove)
- Frame publishing and distribution
- Error handling and recovery
- Timeout handling
- Statistics and monitoring
- Backward compatibility

## Requirements Satisfied

This implementation satisfies the following requirements from the specification:

- **Requirement 4.3**: Replaces complex callback chains with publish-subscribe pattern
- **Requirement 7.1**: Implements standardized media stream publishing
- **Requirement 7.2**: Provides subscriber management interfaces
- **Requirement 7.3**: Creates asynchronous publishing mechanism

## Future Enhancements

Potential future enhancements include:

1. **Quality Adaptation**: Dynamic quality adjustment based on subscriber feedback
2. **Load Balancing**: Distribute subscribers across multiple publisher instances
3. **Persistent Subscriptions**: Maintain subscriptions across restarts
4. **Advanced Filtering**: Content-based frame filtering for subscribers
5. **Compression**: Optional frame compression for network efficiency
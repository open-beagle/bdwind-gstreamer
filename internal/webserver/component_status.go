package webserver

import (
	"fmt"
	"sync"
	"time"
)

// ComponentStatus 组件状态枚举
type ComponentStatus int

const (
	// ComponentStatusStopped 组件已停止
	ComponentStatusStopped ComponentStatus = iota
	// ComponentStatusStarting 组件正在启动
	ComponentStatusStarting
	// ComponentStatusRunning 组件正在运行
	ComponentStatusRunning
	// ComponentStatusStopping 组件正在停止
	ComponentStatusStopping
	// ComponentStatusError 组件出现错误
	ComponentStatusError
	// ComponentStatusDisabled 组件已禁用
	ComponentStatusDisabled
)

// String 返回组件状态的字符串表示
func (s ComponentStatus) String() string {
	switch s {
	case ComponentStatusStopped:
		return "stopped"
	case ComponentStatusStarting:
		return "starting"
	case ComponentStatusRunning:
		return "running"
	case ComponentStatusStopping:
		return "stopping"
	case ComponentStatusError:
		return "error"
	case ComponentStatusDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}

// ComponentState 组件状态信息
type ComponentState struct {
	// Name 组件名称
	Name string `json:"name"`
	// Status 组件状态
	Status ComponentStatus `json:"status"`
	// Enabled 组件是否启用
	Enabled bool `json:"enabled"`
	// StartTime 启动时间
	StartTime *time.Time `json:"start_time,omitempty"`
	// StopTime 停止时间
	StopTime *time.Time `json:"stop_time,omitempty"`
	// LastError 最后一次错误
	LastError string `json:"last_error,omitempty"`
	// ErrorTime 错误发生时间
	ErrorTime *time.Time `json:"error_time,omitempty"`
	// Stats 组件统计信息
	Stats map[string]interface{} `json:"stats,omitempty"`
}

// ComponentStatusTracker 组件状态跟踪器
type ComponentStatusTracker struct {
	mu         sync.RWMutex
	components map[string]*ComponentState
}

// NewComponentStatusTracker 创建新的组件状态跟踪器
func NewComponentStatusTracker() *ComponentStatusTracker {
	return &ComponentStatusTracker{
		components: make(map[string]*ComponentState),
	}
}

// RegisterComponent 注册组件
func (t *ComponentStatusTracker) RegisterComponent(name string, enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	status := ComponentStatusStopped
	if !enabled {
		status = ComponentStatusDisabled
	}

	t.components[name] = &ComponentState{
		Name:    name,
		Status:  status,
		Enabled: enabled,
	}
}

// UpdateStatus 更新组件状态
func (t *ComponentStatusTracker) UpdateStatus(name string, status ComponentStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, exists := t.components[name]
	if !exists {
		return
	}

	state.Status = status
	now := time.Now()

	switch status {
	case ComponentStatusRunning:
		state.StartTime = &now
		state.LastError = ""
		state.ErrorTime = nil
	case ComponentStatusStopped:
		state.StopTime = &now
	case ComponentStatusError:
		state.ErrorTime = &now
	}
}

// SetError 设置组件错误
func (t *ComponentStatusTracker) SetError(name string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, exists := t.components[name]
	if !exists {
		return
	}

	state.Status = ComponentStatusError
	state.LastError = err.Error()
	now := time.Now()
	state.ErrorTime = &now
}

// UpdateStats 更新组件统计信息
func (t *ComponentStatusTracker) UpdateStats(name string, stats map[string]interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, exists := t.components[name]
	if !exists {
		return
	}

	state.Stats = stats
}

// GetComponentState 获取组件状态
func (t *ComponentStatusTracker) GetComponentState(name string) (*ComponentState, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state, exists := t.components[name]
	if !exists {
		return nil, false
	}

	// 返回状态的副本以避免并发修改
	stateCopy := *state
	if state.Stats != nil {
		stateCopy.Stats = make(map[string]interface{})
		for k, v := range state.Stats {
			stateCopy.Stats[k] = v
		}
	}

	return &stateCopy, true
}

// GetAllComponentStates 获取所有组件状态
func (t *ComponentStatusTracker) GetAllComponentStates() map[string]*ComponentState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]*ComponentState)
	for name, state := range t.components {
		// 返回状态的副本以避免并发修改
		stateCopy := *state
		if state.Stats != nil {
			stateCopy.Stats = make(map[string]interface{})
			for k, v := range state.Stats {
				stateCopy.Stats[k] = v
			}
		}
		result[name] = &stateCopy
	}

	return result
}

// GetRunningComponents 获取正在运行的组件列表
func (t *ComponentStatusTracker) GetRunningComponents() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var running []string
	for name, state := range t.components {
		if state.Status == ComponentStatusRunning {
			running = append(running, name)
		}
	}

	return running
}

// GetFailedComponents 获取失败的组件列表
func (t *ComponentStatusTracker) GetFailedComponents() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var failed []string
	for name, state := range t.components {
		if state.Status == ComponentStatusError {
			failed = append(failed, name)
		}
	}

	return failed
}

// IsHealthy 检查所有组件是否健康
func (t *ComponentStatusTracker) IsHealthy() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, state := range t.components {
		if state.Enabled {
			// 启用的组件必须处于运行状态才算健康
			if state.Status != ComponentStatusRunning {
				return false
			}
		}
	}

	return true
}

// GetHealthSummary 获取健康状态摘要
func (t *ComponentStatusTracker) GetHealthSummary() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	summary := map[string]interface{}{
		"healthy":    true,
		"total":      len(t.components),
		"running":    0,
		"stopped":    0,
		"disabled":   0,
		"error":      0,
		"components": make(map[string]string),
	}

	for name, state := range t.components {
		summary["components"].(map[string]string)[name] = state.Status.String()

		switch state.Status {
		case ComponentStatusRunning:
			summary["running"] = summary["running"].(int) + 1
		case ComponentStatusStopped:
			summary["stopped"] = summary["stopped"].(int) + 1
			// 启用的组件如果停止，则认为不健康
			if state.Enabled {
				summary["healthy"] = false
			}
		case ComponentStatusDisabled:
			summary["disabled"] = summary["disabled"].(int) + 1
		case ComponentStatusError:
			summary["error"] = summary["error"].(int) + 1
			if state.Enabled {
				summary["healthy"] = false
			}
		case ComponentStatusStarting, ComponentStatusStopping:
			// 启用的组件如果在过渡状态，则认为不健康
			if state.Enabled {
				summary["healthy"] = false
			}
		}
	}

	return summary
}

// ComponentStartupError 组件启动错误
type ComponentStartupError struct {
	ComponentName     string
	Err               error
	StartedComponents []string
}

// Error 实现error接口
func (e *ComponentStartupError) Error() string {
	return fmt.Sprintf("failed to start component '%s': %v", e.ComponentName, e.Err)
}

// Unwrap 返回原始错误
func (e *ComponentStartupError) Unwrap() error {
	return e.Err
}

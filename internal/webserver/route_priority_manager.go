package webserver

import (
	"strings"
)

// RoutePriorityManager 管理路由优先级，确保API路由不被静态文件覆盖
type RoutePriorityManager struct {
	// apiPrefixes API路径前缀列表，这些路径具有最高优先级
	apiPrefixes []string
	// reservedPaths 保留路径列表，这些路径不应该被静态文件处理
	reservedPaths []string
}

// NewRoutePriorityManager 创建新的路由优先级管理器
func NewRoutePriorityManager() *RoutePriorityManager {
	return &RoutePriorityManager{
		apiPrefixes: []string{
			"/api/",
			"/auth/",
		},
		reservedPaths: []string{
			"/health",
			"/test/",
		},
	}
}

// NewRoutePriorityManagerWithConfig 使用自定义配置创建路由优先级管理器
func NewRoutePriorityManagerWithConfig(apiPrefixes, reservedPaths []string) *RoutePriorityManager {
	return &RoutePriorityManager{
		apiPrefixes:   apiPrefixes,
		reservedPaths: reservedPaths,
	}
}

// IsReservedPath 检查给定路径是否为保留路径
// 保留路径包括API前缀和其他系统路径
func (rpm *RoutePriorityManager) IsReservedPath(path string) bool {
	// 检查API前缀
	for _, prefix := range rpm.apiPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	// 检查保留路径
	for _, reservedPath := range rpm.reservedPaths {
		if strings.HasPrefix(path, reservedPath) {
			return true
		}
	}

	return false
}

// ShouldServeAsStatic 判断给定路径是否应该作为静态文件处理
// 如果路径是保留路径，则不应该作为静态文件处理
func (rpm *RoutePriorityManager) ShouldServeAsStatic(path string) bool {
	return !rpm.IsReservedPath(path)
}

// IsAPIPath 检查给定路径是否为API路径
func (rpm *RoutePriorityManager) IsAPIPath(path string) bool {
	for _, prefix := range rpm.apiPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// GetAPIPrefix 获取所有API前缀
func (rpm *RoutePriorityManager) GetAPIPrefix() []string {
	return append([]string{}, rpm.apiPrefixes...)
}

// GetReservedPaths 获取所有保留路径
func (rpm *RoutePriorityManager) GetReservedPaths() []string {
	return append([]string{}, rpm.reservedPaths...)
}

// AddAPIPrefix 添加新的API前缀
func (rpm *RoutePriorityManager) AddAPIPrefix(prefix string) {
	// 确保前缀以/开头和结尾
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	// 检查是否已存在
	for _, existing := range rpm.apiPrefixes {
		if existing == prefix {
			return
		}
	}

	rpm.apiPrefixes = append(rpm.apiPrefixes, prefix)
}

// AddReservedPath 添加新的保留路径
func (rpm *RoutePriorityManager) AddReservedPath(path string) {
	// 确保路径以/开头
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// 检查是否已存在
	for _, existing := range rpm.reservedPaths {
		if existing == path {
			return
		}
	}

	rpm.reservedPaths = append(rpm.reservedPaths, path)
}

// RemoveAPIPrefix 移除API前缀
func (rpm *RoutePriorityManager) RemoveAPIPrefix(prefix string) {
	// 确保前缀以/开头和结尾
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	for i, existing := range rpm.apiPrefixes {
		if existing == prefix {
			rpm.apiPrefixes = append(rpm.apiPrefixes[:i], rpm.apiPrefixes[i+1:]...)
			return
		}
	}
}

// RemoveReservedPath 移除保留路径
func (rpm *RoutePriorityManager) RemoveReservedPath(path string) {
	// 确保路径以/开头
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	for i, existing := range rpm.reservedPaths {
		if existing == path {
			rpm.reservedPaths = append(rpm.reservedPaths[:i], rpm.reservedPaths[i+1:]...)
			return
		}
	}
}

// DetectPathConflict 检测路径冲突
// 返回冲突类型和详细信息
func (rpm *RoutePriorityManager) DetectPathConflict(staticPath string) (hasConflict bool, conflictType string, details string) {
	if rpm.IsAPIPath(staticPath) {
		return true, "api_conflict", "Path conflicts with API prefix"
	}

	if rpm.IsReservedPath(staticPath) {
		return true, "reserved_conflict", "Path conflicts with reserved path"
	}

	return false, "", ""
}

// ResolvePathConflict 解决路径冲突
// 根据优先级规则决定如何处理冲突的路径
func (rpm *RoutePriorityManager) ResolvePathConflict(path string) (shouldServeStatic bool, reason string) {
	hasConflict, conflictType, details := rpm.DetectPathConflict(path)

	if !hasConflict {
		return true, "no_conflict"
	}

	switch conflictType {
	case "api_conflict":
		return false, "api_priority: " + details
	case "reserved_conflict":
		return false, "reserved_priority: " + details
	default:
		return false, "unknown_conflict: " + details
	}
}

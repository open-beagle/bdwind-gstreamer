package auth

import (
	"errors"
	"strings"
)

var (
	// ErrPermissionDenied 权限拒绝错误
	ErrPermissionDenied = errors.New("auth: permission denied")
	// ErrInvalidPermission 无效权限错误
	ErrInvalidPermission = errors.New("auth: invalid permission")
)

// Permission 权限常量
const (
	// PermissionDesktopCapture 桌面捕获权限
	PermissionDesktopCapture = "desktop:capture"
	// PermissionDesktopView 桌面查看权限
	PermissionDesktopView = "desktop:view"
	// PermissionDesktopControl 桌面控制权限
	PermissionDesktopControl = "desktop:control"
	// PermissionSystemMonitor 系统监控权限
	PermissionSystemMonitor = "system:monitor"
	// PermissionSystemAdmin 系统管理权限
	PermissionSystemAdmin = "system:admin"
	// PermissionSessionManage 会话管理权限
	PermissionSessionManage = "session:manage"
	// PermissionUserManage 用户管理权限
	PermissionUserManage = "user:manage"
)

// Role 角色定义
type Role struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// User 用户结构
type User struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	Active      bool     `json:"active"`
	CreatedAt   string   `json:"created_at"`
}

// AuthorizationConfig 授权配置
type AuthorizationConfig struct {
	// DefaultRole 默认角色
	DefaultRole string
	// AdminUsers 管理员用户列表
	AdminUsers []string
	// RequireAuth 是否需要认证
	RequireAuth bool
}

// DefaultAuthorizationConfig 默认授权配置
func DefaultAuthorizationConfig() *AuthorizationConfig {
	return &AuthorizationConfig{
		DefaultRole: "viewer",
		AdminUsers:  []string{"admin"},
		RequireAuth: true,
	}
}

// Authorizer 授权器接口
type Authorizer interface {
	// CheckPermission 检查权限
	CheckPermission(userID string, permission string) error

	// GetUserPermissions 获取用户权限
	GetUserPermissions(userID string) ([]string, error)

	// HasRole 检查用户是否有指定角色
	HasRole(userID string, role string) (bool, error)

	// GrantPermission 授予权限
	GrantPermission(userID string, permission string) error

	// RevokePermission 撤销权限
	RevokePermission(userID string, permission string) error

	// AssignRole 分配角色
	AssignRole(userID string, role string) error

	// RemoveRole 移除角色
	RemoveRole(userID string, role string) error
}

// InMemoryAuthorizer 内存授权器
type InMemoryAuthorizer struct {
	config *AuthorizationConfig
	users  map[string]*User
	roles  map[string]*Role
}

// NewInMemoryAuthorizer 创建内存授权器
func NewInMemoryAuthorizer(config *AuthorizationConfig) *InMemoryAuthorizer {
	if config == nil {
		config = DefaultAuthorizationConfig()
	}

	auth := &InMemoryAuthorizer{
		config: config,
		users:  make(map[string]*User),
		roles:  make(map[string]*Role),
	}

	// 初始化默认角色
	auth.initializeDefaultRoles()

	// 初始化管理员用户
	auth.initializeAdminUsers()

	return auth
}

// initializeDefaultRoles 初始化默认角色
func (a *InMemoryAuthorizer) initializeDefaultRoles() {
	// 查看者角色
	a.roles["viewer"] = &Role{
		Name:        "viewer",
		Description: "Desktop viewer with read-only access",
		Permissions: []string{
			PermissionDesktopView,
		},
	}

	// 操作者角色
	a.roles["operator"] = &Role{
		Name:        "operator",
		Description: "Desktop operator with control access",
		Permissions: []string{
			PermissionDesktopView,
			PermissionDesktopControl,
		},
	}

	// 捕获者角色
	a.roles["capturer"] = &Role{
		Name:        "capturer",
		Description: "Desktop capturer with capture access",
		Permissions: []string{
			PermissionDesktopView,
			PermissionDesktopCapture,
			PermissionSystemMonitor,
		},
	}

	// 管理员角色
	a.roles["admin"] = &Role{
		Name:        "admin",
		Description: "System administrator with full access",
		Permissions: []string{
			PermissionDesktopView,
			PermissionDesktopControl,
			PermissionDesktopCapture,
			PermissionSystemMonitor,
			PermissionSystemAdmin,
			PermissionSessionManage,
			PermissionUserManage,
		},
	}
}

// initializeAdminUsers 初始化管理员用户
func (a *InMemoryAuthorizer) initializeAdminUsers() {
	for _, adminUser := range a.config.AdminUsers {
		a.users[adminUser] = &User{
			ID:       adminUser,
			Username: adminUser,
			Email:    adminUser + "@localhost",
			Roles:    []string{"admin"},
			Active:   true,
		}
	}
}

// CheckPermission 检查权限
func (a *InMemoryAuthorizer) CheckPermission(userID string, permission string) error {
	if !a.config.RequireAuth {
		return nil
	}

	user, exists := a.users[userID]
	if !exists {
		// 创建默认用户
		user = &User{
			ID:       userID,
			Username: userID,
			Roles:    []string{a.config.DefaultRole},
			Active:   true,
		}
		a.users[userID] = user
	}

	if !user.Active {
		return ErrPermissionDenied
	}

	// 检查直接权限
	for _, perm := range user.Permissions {
		if a.matchPermission(perm, permission) {
			return nil
		}
	}

	// 检查角色权限
	for _, roleName := range user.Roles {
		if role, exists := a.roles[roleName]; exists {
			for _, perm := range role.Permissions {
				if a.matchPermission(perm, permission) {
					return nil
				}
			}
		}
	}

	return ErrPermissionDenied
}

// matchPermission 匹配权限（支持通配符）
func (a *InMemoryAuthorizer) matchPermission(granted, required string) bool {
	// 完全匹配
	if granted == required {
		return true
	}

	// 通配符匹配
	if strings.HasSuffix(granted, "*") {
		prefix := strings.TrimSuffix(granted, "*")
		return strings.HasPrefix(required, prefix)
	}

	return false
}

// GetUserPermissions 获取用户权限
func (a *InMemoryAuthorizer) GetUserPermissions(userID string) ([]string, error) {
	user, exists := a.users[userID]
	if !exists {
		return []string{}, nil
	}

	permissions := make(map[string]bool)

	// 添加直接权限
	for _, perm := range user.Permissions {
		permissions[perm] = true
	}

	// 添加角色权限
	for _, roleName := range user.Roles {
		if role, exists := a.roles[roleName]; exists {
			for _, perm := range role.Permissions {
				permissions[perm] = true
			}
		}
	}

	result := make([]string, 0, len(permissions))
	for perm := range permissions {
		result = append(result, perm)
	}

	return result, nil
}

// HasRole 检查用户是否有指定角色
func (a *InMemoryAuthorizer) HasRole(userID string, role string) (bool, error) {
	user, exists := a.users[userID]
	if !exists {
		return false, nil
	}

	for _, userRole := range user.Roles {
		if userRole == role {
			return true, nil
		}
	}

	return false, nil
}

// GrantPermission 授予权限
func (a *InMemoryAuthorizer) GrantPermission(userID string, permission string) error {
	user, exists := a.users[userID]
	if !exists {
		user = &User{
			ID:       userID,
			Username: userID,
			Roles:    []string{a.config.DefaultRole},
			Active:   true,
		}
		a.users[userID] = user
	}

	// 检查权限是否已存在
	for _, perm := range user.Permissions {
		if perm == permission {
			return nil // 权限已存在
		}
	}

	user.Permissions = append(user.Permissions, permission)
	return nil
}

// RevokePermission 撤销权限
func (a *InMemoryAuthorizer) RevokePermission(userID string, permission string) error {
	user, exists := a.users[userID]
	if !exists {
		return nil // 用户不存在，无需撤销
	}

	// 移除权限
	for i, perm := range user.Permissions {
		if perm == permission {
			user.Permissions = append(user.Permissions[:i], user.Permissions[i+1:]...)
			break
		}
	}

	return nil
}

// AssignRole 分配角色
func (a *InMemoryAuthorizer) AssignRole(userID string, role string) error {
	if _, exists := a.roles[role]; !exists {
		return ErrInvalidPermission
	}

	user, exists := a.users[userID]
	if !exists {
		user = &User{
			ID:       userID,
			Username: userID,
			Active:   true,
		}
		a.users[userID] = user
	}

	// 检查角色是否已存在
	for _, userRole := range user.Roles {
		if userRole == role {
			return nil // 角色已存在
		}
	}

	user.Roles = append(user.Roles, role)
	return nil
}

// RemoveRole 移除角色
func (a *InMemoryAuthorizer) RemoveRole(userID string, role string) error {
	user, exists := a.users[userID]
	if !exists {
		return nil // 用户不存在，无需移除
	}

	// 移除角色
	for i, userRole := range user.Roles {
		if userRole == role {
			user.Roles = append(user.Roles[:i], user.Roles[i+1:]...)
			break
		}
	}

	return nil
}

// GetUser 获取用户信息
func (a *InMemoryAuthorizer) GetUser(userID string) (*User, error) {
	user, exists := a.users[userID]
	if !exists {
		return nil, errors.New("user not found")
	}

	return user, nil
}

// CreateUser 创建用户
func (a *InMemoryAuthorizer) CreateUser(userID, username, email string) (*User, error) {
	if _, exists := a.users[userID]; exists {
		return nil, errors.New("user already exists")
	}

	user := &User{
		ID:       userID,
		Username: username,
		Email:    email,
		Roles:    []string{a.config.DefaultRole},
		Active:   true,
	}

	a.users[userID] = user
	return user, nil
}

// DeactivateUser 停用用户
func (a *InMemoryAuthorizer) DeactivateUser(userID string) error {
	user, exists := a.users[userID]
	if !exists {
		return errors.New("user not found")
	}

	user.Active = false
	return nil
}

// ActivateUser 激活用户
func (a *InMemoryAuthorizer) ActivateUser(userID string) error {
	user, exists := a.users[userID]
	if !exists {
		return errors.New("user not found")
	}

	user.Active = true
	return nil
}

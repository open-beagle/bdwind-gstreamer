package auth

import (
	"testing"
)

func TestInMemoryAuthorizer_CheckPermission(t *testing.T) {
	config := &AuthorizationConfig{
		DefaultRole: "viewer",
		AdminUsers:  []string{"admin"},
		RequireAuth: true,
	}

	auth := NewInMemoryAuthorizer(config)

	// 测试管理员权限
	err := auth.CheckPermission("admin", PermissionSystemAdmin)
	if err != nil {
		t.Errorf("Admin should have system admin permission: %v", err)
	}

	// 测试默认用户权限
	err = auth.CheckPermission("user123", PermissionDesktopView)
	if err != nil {
		t.Errorf("Default user should have desktop view permission: %v", err)
	}

	// 测试权限拒绝
	err = auth.CheckPermission("user123", PermissionSystemAdmin)
	if err != ErrPermissionDenied {
		t.Errorf("Expected ErrPermissionDenied for system admin permission, got %v", err)
	}

	// 测试不需要认证的情况
	config.RequireAuth = false
	auth2 := NewInMemoryAuthorizer(config)
	err = auth2.CheckPermission("anyone", PermissionSystemAdmin)
	if err != nil {
		t.Errorf("Should allow any permission when auth is disabled: %v", err)
	}
}

func TestInMemoryAuthorizer_GetUserPermissions(t *testing.T) {
	auth := NewInMemoryAuthorizer(nil)

	// 测试管理员权限
	permissions, err := auth.GetUserPermissions("admin")
	if err != nil {
		t.Fatalf("Failed to get admin permissions: %v", err)
	}

	expectedPermissions := map[string]bool{
		PermissionDesktopView:    true,
		PermissionDesktopControl: true,
		PermissionDesktopCapture: true,
		PermissionSystemMonitor:  true,
		PermissionSystemAdmin:    true,
		PermissionSessionManage:  true,
		PermissionUserManage:     true,
	}

	if len(permissions) != len(expectedPermissions) {
		t.Errorf("Expected %d permissions for admin, got %d", len(expectedPermissions), len(permissions))
	}

	for _, perm := range permissions {
		if !expectedPermissions[perm] {
			t.Errorf("Unexpected permission for admin: %s", perm)
		}
	}

	// 测试默认用户权限（需要先触发用户创建）
	err = auth.CheckPermission("user123", PermissionDesktopView)
	if err != nil {
		t.Fatalf("Default user should have desktop view permission: %v", err)
	}

	permissions, err = auth.GetUserPermissions("user123")
	if err != nil {
		t.Fatalf("Failed to get user permissions: %v", err)
	}

	if len(permissions) != 1 || permissions[0] != PermissionDesktopView {
		t.Errorf("Expected [%s] for default user, got %v", PermissionDesktopView, permissions)
	}
}

func TestInMemoryAuthorizer_HasRole(t *testing.T) {
	auth := NewInMemoryAuthorizer(nil)

	// 测试管理员角色
	hasRole, err := auth.HasRole("admin", "admin")
	if err != nil {
		t.Fatalf("Failed to check admin role: %v", err)
	}
	if !hasRole {
		t.Error("Admin should have admin role")
	}

	// 测试不存在的角色
	hasRole, err = auth.HasRole("admin", "non-existent-role")
	if err != nil {
		t.Fatalf("Failed to check non-existent role: %v", err)
	}
	if hasRole {
		t.Error("Admin should not have non-existent role")
	}

	// 测试不存在的用户
	hasRole, err = auth.HasRole("non-existent-user", "admin")
	if err != nil {
		t.Fatalf("Failed to check role for non-existent user: %v", err)
	}
	if hasRole {
		t.Error("Non-existent user should not have any role")
	}
}

func TestInMemoryAuthorizer_GrantPermission(t *testing.T) {
	auth := NewInMemoryAuthorizer(nil)

	// 授予权限
	err := auth.GrantPermission("user123", PermissionDesktopControl)
	if err != nil {
		t.Fatalf("Failed to grant permission: %v", err)
	}

	// 验证权限已授予
	err = auth.CheckPermission("user123", PermissionDesktopControl)
	if err != nil {
		t.Errorf("User should have granted permission: %v", err)
	}

	// 重复授予相同权限（应该不报错）
	err = auth.GrantPermission("user123", PermissionDesktopControl)
	if err != nil {
		t.Errorf("Granting duplicate permission should not error: %v", err)
	}

	// 验证权限列表
	permissions, err := auth.GetUserPermissions("user123")
	if err != nil {
		t.Fatalf("Failed to get user permissions: %v", err)
	}

	hasDesktopView := false
	hasDesktopControl := false
	for _, perm := range permissions {
		if perm == PermissionDesktopView {
			hasDesktopView = true
		}
		if perm == PermissionDesktopControl {
			hasDesktopControl = true
		}
	}

	if !hasDesktopView {
		t.Error("User should have default desktop view permission")
	}

	if !hasDesktopControl {
		t.Error("User should have granted desktop control permission")
	}
}

func TestInMemoryAuthorizer_RevokePermission(t *testing.T) {
	auth := NewInMemoryAuthorizer(nil)

	// 先授予权限
	err := auth.GrantPermission("user123", PermissionDesktopControl)
	if err != nil {
		t.Fatalf("Failed to grant permission: %v", err)
	}

	// 验证权限存在
	err = auth.CheckPermission("user123", PermissionDesktopControl)
	if err != nil {
		t.Fatalf("User should have granted permission: %v", err)
	}

	// 撤销权限
	err = auth.RevokePermission("user123", PermissionDesktopControl)
	if err != nil {
		t.Fatalf("Failed to revoke permission: %v", err)
	}

	// 验证权限已撤销
	err = auth.CheckPermission("user123", PermissionDesktopControl)
	if err != ErrPermissionDenied {
		t.Errorf("Expected ErrPermissionDenied after revoking permission, got %v", err)
	}

	// 撤销不存在用户的权限（应该不报错）
	err = auth.RevokePermission("non-existent-user", PermissionDesktopControl)
	if err != nil {
		t.Errorf("Revoking permission from non-existent user should not error: %v", err)
	}
}

func TestInMemoryAuthorizer_AssignRole(t *testing.T) {
	auth := NewInMemoryAuthorizer(nil)

	// 分配角色
	err := auth.AssignRole("user123", "operator")
	if err != nil {
		t.Fatalf("Failed to assign role: %v", err)
	}

	// 验证角色已分配
	hasRole, err := auth.HasRole("user123", "operator")
	if err != nil {
		t.Fatalf("Failed to check assigned role: %v", err)
	}
	if !hasRole {
		t.Error("User should have assigned role")
	}

	// 验证角色权限
	err = auth.CheckPermission("user123", PermissionDesktopControl)
	if err != nil {
		t.Errorf("User should have operator role permissions: %v", err)
	}

	// 重复分配相同角色（应该不报错）
	err = auth.AssignRole("user123", "operator")
	if err != nil {
		t.Errorf("Assigning duplicate role should not error: %v", err)
	}

	// 分配无效角色
	err = auth.AssignRole("user123", "invalid-role")
	if err != ErrInvalidPermission {
		t.Errorf("Expected ErrInvalidPermission for invalid role, got %v", err)
	}
}

func TestInMemoryAuthorizer_RemoveRole(t *testing.T) {
	auth := NewInMemoryAuthorizer(nil)

	// 先分配角色
	err := auth.AssignRole("user123", "operator")
	if err != nil {
		t.Fatalf("Failed to assign role: %v", err)
	}

	// 验证角色存在
	hasRole, err := auth.HasRole("user123", "operator")
	if err != nil {
		t.Fatalf("Failed to check role: %v", err)
	}
	if !hasRole {
		t.Fatalf("User should have assigned role")
	}

	// 移除角色
	err = auth.RemoveRole("user123", "operator")
	if err != nil {
		t.Fatalf("Failed to remove role: %v", err)
	}

	// 验证角色已移除
	hasRole, err = auth.HasRole("user123", "operator")
	if err != nil {
		t.Fatalf("Failed to check role after removal: %v", err)
	}
	if hasRole {
		t.Error("User should not have removed role")
	}

	// 验证角色权限已失效
	err = auth.CheckPermission("user123", PermissionDesktopControl)
	if err != ErrPermissionDenied {
		t.Errorf("Expected ErrPermissionDenied after removing role, got %v", err)
	}

	// 移除不存在用户的角色（应该不报错）
	err = auth.RemoveRole("non-existent-user", "operator")
	if err != nil {
		t.Errorf("Removing role from non-existent user should not error: %v", err)
	}
}

func TestInMemoryAuthorizer_CreateUser(t *testing.T) {
	auth := NewInMemoryAuthorizer(nil)

	// 创建用户
	user, err := auth.CreateUser("user123", "testuser", "test@example.com")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if user.ID != "user123" {
		t.Errorf("Expected user ID 'user123', got '%s'", user.ID)
	}

	if user.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", user.Username)
	}

	if user.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%s'", user.Email)
	}

	if !user.Active {
		t.Error("New user should be active")
	}

	if len(user.Roles) != 1 || user.Roles[0] != "viewer" {
		t.Errorf("Expected default role ['viewer'], got %v", user.Roles)
	}

	// 尝试创建重复用户
	_, err = auth.CreateUser("user123", "duplicate", "duplicate@example.com")
	if err == nil {
		t.Error("Creating duplicate user should return error")
	}
}

func TestInMemoryAuthorizer_GetUser(t *testing.T) {
	auth := NewInMemoryAuthorizer(nil)

	// 创建用户
	originalUser, err := auth.CreateUser("user123", "testuser", "test@example.com")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// 获取用户
	retrievedUser, err := auth.GetUser("user123")
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if retrievedUser.ID != originalUser.ID {
		t.Errorf("Expected user ID %s, got %s", originalUser.ID, retrievedUser.ID)
	}

	if retrievedUser.Username != originalUser.Username {
		t.Errorf("Expected username %s, got %s", originalUser.Username, retrievedUser.Username)
	}

	// 获取不存在的用户
	_, err = auth.GetUser("non-existent-user")
	if err == nil {
		t.Error("Getting non-existent user should return error")
	}
}

func TestInMemoryAuthorizer_DeactivateUser(t *testing.T) {
	auth := NewInMemoryAuthorizer(nil)

	// 创建用户
	_, err := auth.CreateUser("user123", "testuser", "test@example.com")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// 验证用户有权限
	err = auth.CheckPermission("user123", PermissionDesktopView)
	if err != nil {
		t.Fatalf("Active user should have permissions: %v", err)
	}

	// 停用用户
	err = auth.DeactivateUser("user123")
	if err != nil {
		t.Fatalf("Failed to deactivate user: %v", err)
	}

	// 验证用户权限被拒绝
	err = auth.CheckPermission("user123", PermissionDesktopView)
	if err != ErrPermissionDenied {
		t.Errorf("Expected ErrPermissionDenied for deactivated user, got %v", err)
	}

	// 停用不存在的用户
	err = auth.DeactivateUser("non-existent-user")
	if err == nil {
		t.Error("Deactivating non-existent user should return error")
	}
}

func TestInMemoryAuthorizer_ActivateUser(t *testing.T) {
	auth := NewInMemoryAuthorizer(nil)

	// 创建并停用用户
	_, err := auth.CreateUser("user123", "testuser", "test@example.com")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	err = auth.DeactivateUser("user123")
	if err != nil {
		t.Fatalf("Failed to deactivate user: %v", err)
	}

	// 验证用户权限被拒绝
	err = auth.CheckPermission("user123", PermissionDesktopView)
	if err != ErrPermissionDenied {
		t.Fatalf("Deactivated user should not have permissions: %v", err)
	}

	// 激活用户
	err = auth.ActivateUser("user123")
	if err != nil {
		t.Fatalf("Failed to activate user: %v", err)
	}

	// 验证用户权限恢复
	err = auth.CheckPermission("user123", PermissionDesktopView)
	if err != nil {
		t.Errorf("Activated user should have permissions: %v", err)
	}

	// 激活不存在的用户
	err = auth.ActivateUser("non-existent-user")
	if err == nil {
		t.Error("Activating non-existent user should return error")
	}
}

func TestPermissionMatching(t *testing.T) {
	auth := NewInMemoryAuthorizer(nil)

	// 测试通配符权限
	err := auth.GrantPermission("user123", "desktop:*")
	if err != nil {
		t.Fatalf("Failed to grant wildcard permission: %v", err)
	}

	// 验证通配符匹配
	err = auth.CheckPermission("user123", "desktop:view")
	if err != nil {
		t.Errorf("Wildcard permission should match desktop:view: %v", err)
	}

	err = auth.CheckPermission("user123", "desktop:control")
	if err != nil {
		t.Errorf("Wildcard permission should match desktop:control: %v", err)
	}

	err = auth.CheckPermission("user123", "desktop:capture")
	if err != nil {
		t.Errorf("Wildcard permission should match desktop:capture: %v", err)
	}

	// 验证通配符不匹配其他前缀
	err = auth.CheckPermission("user123", "system:admin")
	if err != ErrPermissionDenied {
		t.Errorf("Wildcard permission should not match system:admin, got %v", err)
	}
}

func TestDefaultAuthorizationConfig(t *testing.T) {
	config := DefaultAuthorizationConfig()

	if config.DefaultRole != "viewer" {
		t.Errorf("Expected default role 'viewer', got '%s'", config.DefaultRole)
	}

	if len(config.AdminUsers) != 1 || config.AdminUsers[0] != "admin" {
		t.Errorf("Expected admin users ['admin'], got %v", config.AdminUsers)
	}

	if !config.RequireAuth {
		t.Error("Expected require auth to be true")
	}
}

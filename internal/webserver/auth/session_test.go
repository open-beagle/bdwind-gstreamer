package auth

import (
	"testing"
	"time"
)

func TestInMemorySessionManager_CreateSession(t *testing.T) {
	config := &SessionConfig{
		SessionTTL:      2 * time.Hour,
		MaxSessions:     3,
		CleanupInterval: 10 * time.Minute,
	}

	sm := NewInMemorySessionManager(config)
	defer sm.Stop()

	// 创建会话
	session, err := sm.CreateSession("user123", "token123", "192.168.1.1", "Mozilla/5.0", []string{"desktop:view"})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if session.UserID != "user123" {
		t.Errorf("Expected user ID 'user123', got '%s'", session.UserID)
	}

	if session.TokenID != "token123" {
		t.Errorf("Expected token ID 'token123', got '%s'", session.TokenID)
	}

	if session.Status != SessionStatusActive {
		t.Errorf("Expected session status %s, got %s", SessionStatusActive, session.Status)
	}

	if session.IPAddress != "192.168.1.1" {
		t.Errorf("Expected IP address '192.168.1.1', got '%s'", session.IPAddress)
	}

	if len(session.Permissions) != 1 || session.Permissions[0] != "desktop:view" {
		t.Errorf("Expected permissions ['desktop:view'], got %v", session.Permissions)
	}
}

func TestInMemorySessionManager_GetSession(t *testing.T) {
	sm := NewInMemorySessionManager(nil)
	defer sm.Stop()

	// 创建会话
	originalSession, err := sm.CreateSession("user123", "token123", "192.168.1.1", "Mozilla/5.0", []string{"desktop:view"})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// 获取会话
	retrievedSession, err := sm.GetSession(originalSession.ID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if retrievedSession.ID != originalSession.ID {
		t.Errorf("Expected session ID %s, got %s", originalSession.ID, retrievedSession.ID)
	}

	if retrievedSession.UserID != originalSession.UserID {
		t.Errorf("Expected user ID %s, got %s", originalSession.UserID, retrievedSession.UserID)
	}

	// 测试获取不存在的会话
	_, err = sm.GetSession("non-existent-session")
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound, got %v", err)
	}
}

func TestInMemorySessionManager_UpdateLastAccess(t *testing.T) {
	sm := NewInMemorySessionManager(nil)
	defer sm.Stop()

	// 创建会话
	session, err := sm.CreateSession("user123", "token123", "192.168.1.1", "Mozilla/5.0", []string{"desktop:view"})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	originalLastAccess := session.LastAccess
	originalExpiresAt := session.ExpiresAt

	// 等待一小段时间
	time.Sleep(10 * time.Millisecond)

	// 更新最后访问时间
	err = sm.UpdateLastAccess(session.ID)
	if err != nil {
		t.Fatalf("Failed to update last access: %v", err)
	}

	// 获取更新后的会话
	updatedSession, err := sm.GetSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to get updated session: %v", err)
	}

	if !updatedSession.LastAccess.After(originalLastAccess) {
		t.Error("Last access time should be updated")
	}

	if !updatedSession.ExpiresAt.After(originalExpiresAt) {
		t.Error("Expires at time should be extended")
	}

	// 测试更新不存在的会话
	err = sm.UpdateLastAccess("non-existent-session")
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound, got %v", err)
	}
}

func TestInMemorySessionManager_RevokeSession(t *testing.T) {
	sm := NewInMemorySessionManager(nil)
	defer sm.Stop()

	// 创建会话
	session, err := sm.CreateSession("user123", "token123", "192.168.1.1", "Mozilla/5.0", []string{"desktop:view"})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// 验证会话存在
	_, err = sm.GetSession(session.ID)
	if err != nil {
		t.Fatalf("Session should exist before revocation: %v", err)
	}

	// 撤销会话
	err = sm.RevokeSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to revoke session: %v", err)
	}

	// 验证会话已被撤销
	_, err = sm.GetSession(session.ID)
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound after revocation, got %v", err)
	}

	// 测试撤销不存在的会话
	err = sm.RevokeSession("non-existent-session")
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound for non-existent session, got %v", err)
	}
}

func TestInMemorySessionManager_RevokeUserSessions(t *testing.T) {
	sm := NewInMemorySessionManager(nil)
	defer sm.Stop()

	// 为同一用户创建多个会话
	session1, err := sm.CreateSession("user123", "token1", "192.168.1.1", "Mozilla/5.0", []string{"desktop:view"})
	if err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}

	session2, err := sm.CreateSession("user123", "token2", "192.168.1.2", "Chrome", []string{"desktop:control"})
	if err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}

	// 为不同用户创建会话
	session3, err := sm.CreateSession("user456", "token3", "192.168.1.3", "Safari", []string{"desktop:view"})
	if err != nil {
		t.Fatalf("Failed to create session 3: %v", err)
	}

	// 撤销 user123 的所有会话
	err = sm.RevokeUserSessions("user123")
	if err != nil {
		t.Fatalf("Failed to revoke user sessions: %v", err)
	}

	// 验证 user123 的会话已被撤销
	_, err = sm.GetSession(session1.ID)
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound for session 1, got %v", err)
	}

	_, err = sm.GetSession(session2.ID)
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound for session 2, got %v", err)
	}

	// 验证 user456 的会话仍然存在
	_, err = sm.GetSession(session3.ID)
	if err != nil {
		t.Errorf("Session 3 should still exist: %v", err)
	}
}

func TestInMemorySessionManager_GetUserSessions(t *testing.T) {
	sm := NewInMemorySessionManager(nil)
	defer sm.Stop()

	// 为用户创建多个会话
	session1, err := sm.CreateSession("user123", "token1", "192.168.1.1", "Mozilla/5.0", []string{"desktop:view"})
	if err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}

	session2, err := sm.CreateSession("user123", "token2", "192.168.1.2", "Chrome", []string{"desktop:control"})
	if err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}

	// 获取用户会话
	sessions, err := sm.GetUserSessions("user123")
	if err != nil {
		t.Fatalf("Failed to get user sessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(sessions))
	}

	// 验证会话ID
	sessionIDs := make(map[string]bool)
	for _, session := range sessions {
		sessionIDs[session.ID] = true
	}

	if !sessionIDs[session1.ID] {
		t.Error("Session 1 should be in user sessions")
	}

	if !sessionIDs[session2.ID] {
		t.Error("Session 2 should be in user sessions")
	}

	// 测试获取不存在用户的会话
	sessions, err = sm.GetUserSessions("non-existent-user")
	if err != nil {
		t.Fatalf("Failed to get sessions for non-existent user: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions for non-existent user, got %d", len(sessions))
	}
}

func TestInMemorySessionManager_MaxSessions(t *testing.T) {
	config := &SessionConfig{
		SessionTTL:      2 * time.Hour,
		MaxSessions:     2, // 限制最大会话数为2
		CleanupInterval: 10 * time.Minute,
	}

	sm := NewInMemorySessionManager(config)
	defer sm.Stop()

	// 创建第一个会话
	session1, err := sm.CreateSession("user123", "token1", "192.168.1.1", "Mozilla/5.0", []string{"desktop:view"})
	if err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}

	// 创建第二个会话
	session2, err := sm.CreateSession("user123", "token2", "192.168.1.2", "Chrome", []string{"desktop:control"})
	if err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}

	// 创建第三个会话（应该移除最旧的会话）
	session3, err := sm.CreateSession("user123", "token3", "192.168.1.3", "Safari", []string{"desktop:capture"})
	if err != nil {
		t.Fatalf("Failed to create session 3: %v", err)
	}

	// 验证最旧的会话已被移除
	_, err = sm.GetSession(session1.ID)
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound for oldest session, got %v", err)
	}

	// 验证其他会话仍然存在
	_, err = sm.GetSession(session2.ID)
	if err != nil {
		t.Errorf("Session 2 should still exist: %v", err)
	}

	_, err = sm.GetSession(session3.ID)
	if err != nil {
		t.Errorf("Session 3 should exist: %v", err)
	}

	// 验证用户会话数量
	sessions, err := sm.GetUserSessions("user123")
	if err != nil {
		t.Fatalf("Failed to get user sessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions after max limit enforcement, got %d", len(sessions))
	}
}

func TestInMemorySessionManager_CleanupExpiredSessions(t *testing.T) {
	config := &SessionConfig{
		SessionTTL:      100 * time.Millisecond, // 很短的过期时间用于测试
		MaxSessions:     10,
		CleanupInterval: 10 * time.Minute,
	}

	sm := NewInMemorySessionManager(config)
	defer sm.Stop()

	// 创建一些会话
	session1, err := sm.CreateSession("user1", "token1", "192.168.1.1", "Mozilla/5.0", []string{"desktop:view"})
	if err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}

	session2, err := sm.CreateSession("user2", "token2", "192.168.1.2", "Chrome", []string{"desktop:control"})
	if err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}

	// 验证会话存在
	_, err = sm.GetSession(session1.ID)
	if err != nil {
		t.Fatalf("Session 1 should exist: %v", err)
	}

	_, err = sm.GetSession(session2.ID)
	if err != nil {
		t.Fatalf("Session 2 should exist: %v", err)
	}

	// 等待会话过期
	time.Sleep(150 * time.Millisecond)

	// 清理过期会话
	err = sm.CleanupExpiredSessions()
	if err != nil {
		t.Fatalf("Failed to cleanup expired sessions: %v", err)
	}

	// 验证会话已被清理
	_, err = sm.GetSession(session1.ID)
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound for expired session 1, got %v", err)
	}

	_, err = sm.GetSession(session2.ID)
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound for expired session 2, got %v", err)
	}
}

func TestInMemorySessionManager_GetActiveSessions(t *testing.T) {
	sm := NewInMemorySessionManager(nil)
	defer sm.Stop()

	// 初始活跃会话数应为0
	activeCount := sm.GetActiveSessions()
	if activeCount != 0 {
		t.Errorf("Expected 0 active sessions initially, got %d", activeCount)
	}

	// 创建一些会话
	_, err := sm.CreateSession("user1", "token1", "192.168.1.1", "Mozilla/5.0", []string{"desktop:view"})
	if err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}

	_, err = sm.CreateSession("user2", "token2", "192.168.1.2", "Chrome", []string{"desktop:control"})
	if err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}

	// 验证活跃会话数
	activeCount = sm.GetActiveSessions()
	if activeCount != 2 {
		t.Errorf("Expected 2 active sessions, got %d", activeCount)
	}
}

func TestDefaultSessionConfig(t *testing.T) {
	config := DefaultSessionConfig()

	if config.SessionTTL != 2*time.Hour {
		t.Errorf("Expected session TTL 2 hours, got %v", config.SessionTTL)
	}

	if config.MaxSessions != 5 {
		t.Errorf("Expected max sessions 5, got %d", config.MaxSessions)
	}

	if config.CleanupInterval != 10*time.Minute {
		t.Errorf("Expected cleanup interval 10 minutes, got %v", config.CleanupInterval)
	}
}

func TestSessionExpiration(t *testing.T) {
	config := &SessionConfig{
		SessionTTL:      50 * time.Millisecond, // 很短的过期时间用于测试
		MaxSessions:     10,
		CleanupInterval: 10 * time.Minute,
	}

	sm := NewInMemorySessionManager(config)
	defer sm.Stop()

	// 创建会话
	session, err := sm.CreateSession("user123", "token123", "192.168.1.1", "Mozilla/5.0", []string{"desktop:view"})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// 验证会话有效
	_, err = sm.GetSession(session.ID)
	if err != nil {
		t.Fatalf("Session should be valid: %v", err)
	}

	// 等待会话过期
	time.Sleep(60 * time.Millisecond)

	// 验证会话已过期
	_, err = sm.GetSession(session.ID)
	if err != ErrSessionExpired {
		t.Errorf("Expected ErrSessionExpired, got %v", err)
	}
}

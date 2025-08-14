package auth

import (
	"errors"
	"sync"
	"time"
)

var (
	// ErrSessionNotFound 会话未找到错误
	ErrSessionNotFound = errors.New("auth: session not found")
	// ErrSessionExpired 会话过期错误
	ErrSessionExpired = errors.New("auth: session expired")
	// ErrInvalidSession 无效会话错误
	ErrInvalidSession = errors.New("auth: invalid session")
)

// SessionStatus 会话状态
type SessionStatus string

const (
	// SessionStatusActive 活跃状态
	SessionStatusActive SessionStatus = "active"
	// SessionStatusExpired 过期状态
	SessionStatusExpired SessionStatus = "expired"
	// SessionStatusRevoked 撤销状态
	SessionStatusRevoked SessionStatus = "revoked"
)

// Session 会话结构
type Session struct {
	ID          string        `json:"id"`
	UserID      string        `json:"user_id"`
	TokenID     string        `json:"token_id"`
	Status      SessionStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	LastAccess  time.Time     `json:"last_access"`
	ExpiresAt   time.Time     `json:"expires_at"`
	IPAddress   string        `json:"ip_address"`
	UserAgent   string        `json:"user_agent"`
	Permissions []string      `json:"permissions"`
}

// SessionConfig 会话配置
type SessionConfig struct {
	// SessionTTL 会话生存时间
	SessionTTL time.Duration
	// MaxSessions 每个用户最大会话数
	MaxSessions int
	// CleanupInterval 清理间隔
	CleanupInterval time.Duration
}

// DefaultSessionConfig 默认会话配置
func DefaultSessionConfig() *SessionConfig {
	return &SessionConfig{
		SessionTTL:      2 * time.Hour,
		MaxSessions:     5,
		CleanupInterval: 10 * time.Minute,
	}
}

// SessionManager 会话管理器接口
type SessionManager interface {
	// CreateSession 创建会话
	CreateSession(userID, tokenID, ipAddress, userAgent string, permissions []string) (*Session, error)

	// GetSession 获取会话
	GetSession(sessionID string) (*Session, error)

	// UpdateLastAccess 更新最后访问时间
	UpdateLastAccess(sessionID string) error

	// RevokeSession 撤销会话
	RevokeSession(sessionID string) error

	// RevokeUserSessions 撤销用户所有会话
	RevokeUserSessions(userID string) error

	// GetUserSessions 获取用户会话列表
	GetUserSessions(userID string) ([]*Session, error)

	// CleanupExpiredSessions 清理过期会话
	CleanupExpiredSessions() error

	// GetActiveSessions 获取活跃会话数量
	GetActiveSessions() int
}

// InMemorySessionManager 内存会话管理器
type InMemorySessionManager struct {
	config       *SessionConfig
	sessions     map[string]*Session
	userSessions map[string][]string // userID -> sessionIDs
	mutex        sync.RWMutex
	stopCh       chan struct{}
}

// NewInMemorySessionManager 创建内存会话管理器
func NewInMemorySessionManager(config *SessionConfig) *InMemorySessionManager {
	if config == nil {
		config = DefaultSessionConfig()
	}

	sm := &InMemorySessionManager{
		config:       config,
		sessions:     make(map[string]*Session),
		userSessions: make(map[string][]string),
		stopCh:       make(chan struct{}),
	}

	// 启动清理协程
	go sm.cleanupRoutine()

	return sm
}

// CreateSession 创建会话
func (sm *InMemorySessionManager) CreateSession(userID, tokenID, ipAddress, userAgent string, permissions []string) (*Session, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// 检查用户会话数量限制
	if userSessions, exists := sm.userSessions[userID]; exists {
		if len(userSessions) >= sm.config.MaxSessions {
			// 移除最旧的会话
			oldestSessionID := userSessions[0]
			sm.revokeSessionLocked(oldestSessionID)
		}
	}

	sessionID := generateTokenID()
	now := time.Now()

	session := &Session{
		ID:          sessionID,
		UserID:      userID,
		TokenID:     tokenID,
		Status:      SessionStatusActive,
		CreatedAt:   now,
		LastAccess:  now,
		ExpiresAt:   now.Add(sm.config.SessionTTL),
		IPAddress:   ipAddress,
		UserAgent:   userAgent,
		Permissions: permissions,
	}

	sm.sessions[sessionID] = session

	// 更新用户会话列表
	if _, exists := sm.userSessions[userID]; !exists {
		sm.userSessions[userID] = make([]string, 0)
	}
	sm.userSessions[userID] = append(sm.userSessions[userID], sessionID)

	return session, nil
}

// GetSession 获取会话
func (sm *InMemorySessionManager) GetSession(sessionID string) (*Session, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	if session.Status != SessionStatusActive {
		return nil, ErrInvalidSession
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	return session, nil
}

// UpdateLastAccess 更新最后访问时间
func (sm *InMemorySessionManager) UpdateLastAccess(sessionID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	session.LastAccess = time.Now()
	// 延长会话过期时间
	session.ExpiresAt = time.Now().Add(sm.config.SessionTTL)

	return nil
}

// RevokeSession 撤销会话
func (sm *InMemorySessionManager) RevokeSession(sessionID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	return sm.revokeSessionLocked(sessionID)
}

// revokeSessionLocked 撤销会话（需要持有锁）
func (sm *InMemorySessionManager) revokeSessionLocked(sessionID string) error {
	session, exists := sm.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	session.Status = SessionStatusRevoked

	// 从用户会话列表中移除
	if userSessions, exists := sm.userSessions[session.UserID]; exists {
		for i, id := range userSessions {
			if id == sessionID {
				sm.userSessions[session.UserID] = append(userSessions[:i], userSessions[i+1:]...)
				break
			}
		}

		// 如果用户没有其他会话，删除用户记录
		if len(sm.userSessions[session.UserID]) == 0 {
			delete(sm.userSessions, session.UserID)
		}
	}

	delete(sm.sessions, sessionID)

	return nil
}

// RevokeUserSessions 撤销用户所有会话
func (sm *InMemorySessionManager) RevokeUserSessions(userID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	userSessions, exists := sm.userSessions[userID]
	if !exists {
		return nil
	}

	// 撤销所有用户会话
	for _, sessionID := range userSessions {
		sm.revokeSessionLocked(sessionID)
	}

	return nil
}

// GetUserSessions 获取用户会话列表
func (sm *InMemorySessionManager) GetUserSessions(userID string) ([]*Session, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	userSessions, exists := sm.userSessions[userID]
	if !exists {
		return []*Session{}, nil
	}

	sessions := make([]*Session, 0, len(userSessions))
	for _, sessionID := range userSessions {
		if session, exists := sm.sessions[sessionID]; exists {
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// CleanupExpiredSessions 清理过期会话
func (sm *InMemorySessionManager) CleanupExpiredSessions() error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	now := time.Now()
	expiredSessions := make([]string, 0)

	for sessionID, session := range sm.sessions {
		if now.After(session.ExpiresAt) {
			expiredSessions = append(expiredSessions, sessionID)
		}
	}

	for _, sessionID := range expiredSessions {
		sm.revokeSessionLocked(sessionID)
	}

	return nil
}

// GetActiveSessions 获取活跃会话数量
func (sm *InMemorySessionManager) GetActiveSessions() int {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	count := 0
	now := time.Now()

	for _, session := range sm.sessions {
		if session.Status == SessionStatusActive && now.Before(session.ExpiresAt) {
			count++
		}
	}

	return count
}

// cleanupRoutine 清理协程
func (sm *InMemorySessionManager) cleanupRoutine() {
	ticker := time.NewTicker(sm.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.CleanupExpiredSessions()
		case <-sm.stopCh:
			return
		}
	}
}

// Stop 停止会话管理器
func (sm *InMemorySessionManager) Stop() {
	close(sm.stopCh)
}

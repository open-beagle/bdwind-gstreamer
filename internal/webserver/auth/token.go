package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidToken 无效令牌错误
	ErrInvalidToken = errors.New("auth: invalid token")
	// ErrTokenExpired 令牌过期错误
	ErrTokenExpired = errors.New("auth: token expired")
	// ErrTokenNotFound 令牌未找到错误
	ErrTokenNotFound = errors.New("auth: token not found")
)

// TokenType 令牌类型
type TokenType string

const (
	// TokenTypeAccess 访问令牌
	TokenTypeAccess TokenType = "access"
	// TokenTypeRefresh 刷新令牌
	TokenTypeRefresh TokenType = "refresh"
)

// Token 令牌结构
type Token struct {
	ID        string    `json:"id"`
	Type      TokenType `json:"type"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	Hash      string    `json:"hash"`
}

// TokenConfig 令牌配置
type TokenConfig struct {
	// AccessTokenTTL 访问令牌生存时间
	AccessTokenTTL time.Duration
	// RefreshTokenTTL 刷新令牌生存时间
	RefreshTokenTTL time.Duration
	// SecretKey 密钥
	SecretKey string
}

// DefaultTokenConfig 默认令牌配置
func DefaultTokenConfig() *TokenConfig {
	return &TokenConfig{
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
		SecretKey:       generateRandomKey(),
	}
}

// TokenManager 令牌管理器接口
type TokenManager interface {
	// GenerateToken 生成令牌
	GenerateToken(userID string, tokenType TokenType) (*Token, string, error)

	// ValidateToken 验证令牌
	ValidateToken(tokenString string) (*Token, error)

	// RevokeToken 撤销令牌
	RevokeToken(tokenID string) error

	// RefreshToken 刷新令牌
	RefreshToken(refreshTokenString string) (*Token, string, error)

	// CleanupExpiredTokens 清理过期令牌
	CleanupExpiredTokens() error
}

// InMemoryTokenManager 内存令牌管理器
type InMemoryTokenManager struct {
	config *TokenConfig
	tokens map[string]*Token
}

// NewInMemoryTokenManager 创建内存令牌管理器
func NewInMemoryTokenManager(config *TokenConfig) *InMemoryTokenManager {
	if config == nil {
		config = DefaultTokenConfig()
	}

	return &InMemoryTokenManager{
		config: config,
		tokens: make(map[string]*Token),
	}
}

// GenerateToken 生成令牌
func (tm *InMemoryTokenManager) GenerateToken(userID string, tokenType TokenType) (*Token, string, error) {
	tokenID := generateTokenID()
	tokenString := generateTokenString()

	var expiresAt time.Time
	switch tokenType {
	case TokenTypeAccess:
		expiresAt = time.Now().Add(tm.config.AccessTokenTTL)
	case TokenTypeRefresh:
		expiresAt = time.Now().Add(tm.config.RefreshTokenTTL)
	default:
		return nil, "", fmt.Errorf("auth: unsupported token type: %s", tokenType)
	}

	hash := tm.hashToken(tokenString)

	token := &Token{
		ID:        tokenID,
		Type:      tokenType,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
		Hash:      hash,
	}

	tm.tokens[tokenID] = token

	return token, tokenString, nil
}

// ValidateToken 验证令牌
func (tm *InMemoryTokenManager) ValidateToken(tokenString string) (*Token, error) {
	hash := tm.hashToken(tokenString)

	for _, token := range tm.tokens {
		if token.Hash == hash {
			if time.Now().After(token.ExpiresAt) {
				return nil, ErrTokenExpired
			}
			return token, nil
		}
	}

	return nil, ErrTokenNotFound
}

// RevokeToken 撤销令牌
func (tm *InMemoryTokenManager) RevokeToken(tokenID string) error {
	if _, exists := tm.tokens[tokenID]; !exists {
		return ErrTokenNotFound
	}

	delete(tm.tokens, tokenID)
	return nil
}

// RefreshToken 刷新令牌
func (tm *InMemoryTokenManager) RefreshToken(refreshTokenString string) (*Token, string, error) {
	refreshToken, err := tm.ValidateToken(refreshTokenString)
	if err != nil {
		return nil, "", err
	}

	if refreshToken.Type != TokenTypeRefresh {
		return nil, "", ErrInvalidToken
	}

	// 撤销旧的刷新令牌
	if err := tm.RevokeToken(refreshToken.ID); err != nil {
		return nil, "", err
	}

	// 生成新的访问令牌
	return tm.GenerateToken(refreshToken.UserID, TokenTypeAccess)
}

// CleanupExpiredTokens 清理过期令牌
func (tm *InMemoryTokenManager) CleanupExpiredTokens() error {
	now := time.Now()

	for tokenID, token := range tm.tokens {
		if now.After(token.ExpiresAt) {
			delete(tm.tokens, tokenID)
		}
	}

	return nil
}

// hashToken 计算令牌哈希
func (tm *InMemoryTokenManager) hashToken(tokenString string) string {
	h := sha256.New()
	h.Write([]byte(tokenString + tm.config.SecretKey))
	return hex.EncodeToString(h.Sum(nil))
}

// generateTokenID 生成令牌ID
func generateTokenID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// generateTokenString 生成令牌字符串
func generateTokenString() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// generateRandomKey 生成随机密钥
func generateRandomKey() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

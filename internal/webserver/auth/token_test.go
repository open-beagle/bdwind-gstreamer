package auth

import (
	"testing"
	"time"
)

func TestInMemoryTokenManager_GenerateToken(t *testing.T) {
	config := &TokenConfig{
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
		SecretKey:       "test-secret-key",
	}

	tm := NewInMemoryTokenManager(config)

	// 测试生成访问令牌
	token, tokenString, err := tm.GenerateToken("user123", TokenTypeAccess)
	if err != nil {
		t.Fatalf("Failed to generate access token: %v", err)
	}

	if token.Type != TokenTypeAccess {
		t.Errorf("Expected token type %s, got %s", TokenTypeAccess, token.Type)
	}

	if token.UserID != "user123" {
		t.Errorf("Expected user ID 'user123', got '%s'", token.UserID)
	}

	if tokenString == "" {
		t.Error("Token string should not be empty")
	}

	// 测试生成刷新令牌
	refreshToken, refreshTokenString, err := tm.GenerateToken("user123", TokenTypeRefresh)
	if err != nil {
		t.Fatalf("Failed to generate refresh token: %v", err)
	}

	if refreshToken.Type != TokenTypeRefresh {
		t.Errorf("Expected token type %s, got %s", TokenTypeRefresh, refreshToken.Type)
	}

	if refreshTokenString == "" {
		t.Error("Refresh token string should not be empty")
	}

	// 测试无效令牌类型
	_, _, err = tm.GenerateToken("user123", "invalid")
	if err == nil {
		t.Error("Expected error for invalid token type")
	}
}

func TestInMemoryTokenManager_ValidateToken(t *testing.T) {
	config := &TokenConfig{
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
		SecretKey:       "test-secret-key",
	}

	tm := NewInMemoryTokenManager(config)

	// 生成令牌
	originalToken, tokenString, err := tm.GenerateToken("user123", TokenTypeAccess)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// 验证有效令牌
	validatedToken, err := tm.ValidateToken(tokenString)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	if validatedToken.ID != originalToken.ID {
		t.Errorf("Expected token ID %s, got %s", originalToken.ID, validatedToken.ID)
	}

	if validatedToken.UserID != originalToken.UserID {
		t.Errorf("Expected user ID %s, got %s", originalToken.UserID, validatedToken.UserID)
	}

	// 测试无效令牌
	_, err = tm.ValidateToken("invalid-token")
	if err != ErrTokenNotFound {
		t.Errorf("Expected ErrTokenNotFound, got %v", err)
	}
}

func TestInMemoryTokenManager_RevokeToken(t *testing.T) {
	tm := NewInMemoryTokenManager(nil)

	// 生成令牌
	token, tokenString, err := tm.GenerateToken("user123", TokenTypeAccess)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// 验证令牌存在
	_, err = tm.ValidateToken(tokenString)
	if err != nil {
		t.Fatalf("Token should be valid before revocation: %v", err)
	}

	// 撤销令牌
	err = tm.RevokeToken(token.ID)
	if err != nil {
		t.Fatalf("Failed to revoke token: %v", err)
	}

	// 验证令牌已被撤销
	_, err = tm.ValidateToken(tokenString)
	if err != ErrTokenNotFound {
		t.Errorf("Expected ErrTokenNotFound after revocation, got %v", err)
	}

	// 测试撤销不存在的令牌
	err = tm.RevokeToken("non-existent-token")
	if err != ErrTokenNotFound {
		t.Errorf("Expected ErrTokenNotFound for non-existent token, got %v", err)
	}
}

func TestInMemoryTokenManager_RefreshToken(t *testing.T) {
	tm := NewInMemoryTokenManager(nil)

	// 生成刷新令牌
	refreshToken, refreshTokenString, err := tm.GenerateToken("user123", TokenTypeRefresh)
	if err != nil {
		t.Fatalf("Failed to generate refresh token: %v", err)
	}

	// 使用刷新令牌获取新的访问令牌
	newToken, newTokenString, err := tm.RefreshToken(refreshTokenString)
	if err != nil {
		t.Fatalf("Failed to refresh token: %v", err)
	}

	if newToken.Type != TokenTypeAccess {
		t.Errorf("Expected new token type %s, got %s", TokenTypeAccess, newToken.Type)
	}

	if newToken.UserID != refreshToken.UserID {
		t.Errorf("Expected user ID %s, got %s", refreshToken.UserID, newToken.UserID)
	}

	if newTokenString == "" {
		t.Error("New token string should not be empty")
	}

	// 验证旧的刷新令牌已被撤销
	_, err = tm.ValidateToken(refreshTokenString)
	if err != ErrTokenNotFound {
		t.Errorf("Expected ErrTokenNotFound for revoked refresh token, got %v", err)
	}

	// 测试使用访问令牌刷新（应该失败）
	_, accessTokenString, err := tm.GenerateToken("user123", TokenTypeAccess)
	if err != nil {
		t.Fatalf("Failed to generate access token: %v", err)
	}

	_, _, err = tm.RefreshToken(accessTokenString)
	if err != ErrInvalidToken {
		t.Errorf("Expected ErrInvalidToken when using access token for refresh, got %v", err)
	}
}

func TestInMemoryTokenManager_CleanupExpiredTokens(t *testing.T) {
	config := &TokenConfig{
		AccessTokenTTL:  100 * time.Millisecond, // 很短的过期时间用于测试
		RefreshTokenTTL: 200 * time.Millisecond,
		SecretKey:       "test-secret-key",
	}

	tm := NewInMemoryTokenManager(config)

	// 生成一些令牌
	_, tokenString1, err := tm.GenerateToken("user1", TokenTypeAccess)
	if err != nil {
		t.Fatalf("Failed to generate token 1: %v", err)
	}

	_, tokenString2, err := tm.GenerateToken("user2", TokenTypeAccess)
	if err != nil {
		t.Fatalf("Failed to generate token 2: %v", err)
	}

	// 验证令牌存在
	_, err = tm.ValidateToken(tokenString1)
	if err != nil {
		t.Fatalf("Token 1 should be valid: %v", err)
	}

	_, err = tm.ValidateToken(tokenString2)
	if err != nil {
		t.Fatalf("Token 2 should be valid: %v", err)
	}

	// 等待令牌过期
	time.Sleep(150 * time.Millisecond)

	// 清理过期令牌
	err = tm.CleanupExpiredTokens()
	if err != nil {
		t.Fatalf("Failed to cleanup expired tokens: %v", err)
	}

	// 验证令牌已过期
	_, err = tm.ValidateToken(tokenString1)
	if err != ErrTokenNotFound {
		t.Errorf("Expected ErrTokenNotFound for expired token 1, got %v", err)
	}

	_, err = tm.ValidateToken(tokenString2)
	if err != ErrTokenNotFound {
		t.Errorf("Expected ErrTokenNotFound for expired token 2, got %v", err)
	}
}

func TestTokenExpiration(t *testing.T) {
	config := &TokenConfig{
		AccessTokenTTL:  50 * time.Millisecond, // 很短的过期时间用于测试
		RefreshTokenTTL: 100 * time.Millisecond,
		SecretKey:       "test-secret-key",
	}

	tm := NewInMemoryTokenManager(config)

	// 生成令牌
	_, tokenString, err := tm.GenerateToken("user123", TokenTypeAccess)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// 验证令牌有效
	_, err = tm.ValidateToken(tokenString)
	if err != nil {
		t.Fatalf("Token should be valid: %v", err)
	}

	// 等待令牌过期
	time.Sleep(60 * time.Millisecond)

	// 验证令牌已过期
	_, err = tm.ValidateToken(tokenString)
	if err != ErrTokenExpired {
		t.Errorf("Expected ErrTokenExpired, got %v", err)
	}
}

func TestDefaultTokenConfig(t *testing.T) {
	config := DefaultTokenConfig()

	if config.AccessTokenTTL != 15*time.Minute {
		t.Errorf("Expected access token TTL 15 minutes, got %v", config.AccessTokenTTL)
	}

	if config.RefreshTokenTTL != 24*time.Hour {
		t.Errorf("Expected refresh token TTL 24 hours, got %v", config.RefreshTokenTTL)
	}

	if config.SecretKey == "" {
		t.Error("Secret key should not be empty")
	}
}

func TestGenerateTokenID(t *testing.T) {
	id1 := generateTokenID()
	id2 := generateTokenID()

	if id1 == id2 {
		t.Error("Generated token IDs should be unique")
	}

	if len(id1) != 32 { // 16 bytes * 2 (hex encoding)
		t.Errorf("Expected token ID length 32, got %d", len(id1))
	}
}

func TestGenerateTokenString(t *testing.T) {
	token1 := generateTokenString()
	token2 := generateTokenString()

	if token1 == token2 {
		t.Error("Generated token strings should be unique")
	}

	if len(token1) != 64 { // 32 bytes * 2 (hex encoding)
		t.Errorf("Expected token string length 64, got %d", len(token1))
	}
}

package webserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
	"github.com/open-beagle/bdwind-gstreamer/internal/webserver/auth"
)

// WebServer Web服务器
type WebServer struct {
	config     *config.WebServerConfig
	server     *http.Server
	router     *mux.Router
	authorizer auth.Authorizer // 认证组件
	mutex      sync.RWMutex
	running    bool
	startTime  time.Time
	components map[string]ComponentManager // 注册的组件
}

// NewWebServer 创建Web服务器
func NewWebServer(cfg *config.WebServerConfig) (*WebServer, error) {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	router := mux.NewRouter()

	ws := &WebServer{
		config:     cfg,
		router:     router,
		startTime:  time.Now(),
		components: make(map[string]ComponentManager),
	}

	// 创建HTTP服务器
	ws.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      ws.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return ws, nil
}

// SetupRoutes 允许外部组件注册路由
// 这个方法应该在启动服务器之前调用
func (ws *WebServer) SetupRoutes(setupFunc func(*mux.Router) error) error {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()

	if ws.running {
		return fmt.Errorf("cannot setup routes while server is running")
	}

	return setupFunc(ws.router)
}

// GetRouter 获取路由器实例（用于外部路由注册）
func (ws *WebServer) GetRouter() *mux.Router {
	return ws.router
}

// GetHandler 获取HTTP处理器
func (ws *WebServer) GetHandler() http.Handler {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()

	// 设置路由
	ws.setupRoutes()

	return ws.router
}

// SetupRoutesForTesting 为测试设置路由
func (ws *WebServer) SetupRoutesForTesting() {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()
	ws.setupRoutes()
}

// API处理器
func (ws *WebServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	ws.mutex.RLock()
	defer ws.mutex.RUnlock()

	status := map[string]any{
		"status":     "running",
		"timestamp":  time.Now().Unix(),
		"uptime":     time.Since(ws.startTime).Seconds(),
		"components": make(map[string]interface{}), // 空的组件状态，由App层面管理
		"version":    "1.0.0",
		"build_time": "2024-01-01T00:00:00Z",
	}

	ws.writeJSON(w, status)
}

func (ws *WebServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	version := map[string]any{
		"version":    "1.0.0",
		"build_time": "2024-01-01T00:00:00Z",
		"git_commit": "unknown",
		"go_version": "go1.21",
	}

	ws.writeJSON(w, version)
}

func (ws *WebServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	ws.mutex.RLock()
	defer ws.mutex.RUnlock()

	// WebServer自身的健康状态
	health := map[string]any{
		"status": "healthy",
		"checks": map[string]any{
			"webserver": ws.running,
		},
	}

	ws.writeJSON(w, health)
}

func (ws *WebServer) handleStats(w http.ResponseWriter, r *http.Request) {
	ws.mutex.RLock()
	defer ws.mutex.RUnlock()

	// 基本统计信息
	stats := map[string]any{
		"server": map[string]any{
			"uptime":                time.Since(ws.startTime).Seconds(),
			"start_time":            ws.startTime.Unix(),
			"registered_components": len(ws.components),
			"running":               ws.running,
		},
		"system": map[string]any{
			"timestamp": time.Now().Unix(),
			"version":   "1.0.0",
		},
		"components": make(map[string]any),
	}

	// 添加组件统计信息（如果有的话）
	componentStats := make(map[string]any)
	for name := range ws.components {
		componentStats[name] = map[string]any{
			"status": "registered",
		}
	}
	stats["components"] = componentStats

	ws.writeJSON(w, stats)
}

// handleComponentList 处理组件列表请求
func (ws *WebServer) handleComponentList(w http.ResponseWriter, r *http.Request) {
	// 组件管理由App层负责，这里返回提示信息
	http.Error(w, "Component management is handled at application level", http.StatusNotImplemented)
}

// handleComponentStatus 处理单个组件状态请求
func (ws *WebServer) handleComponentStatus(w http.ResponseWriter, r *http.Request) {
	// 组件管理由App层负责，这里返回提示信息
	http.Error(w, "Component management is handled at application level", http.StatusNotImplemented)
}

// handleComponentStats 处理组件统计信息请求
func (ws *WebServer) handleComponentStats(w http.ResponseWriter, r *http.Request) {
	// 组件管理由App层负责，这里返回提示信息
	http.Error(w, "Component management is handled at application level", http.StatusNotImplemented)
}

// 工具方法
func (ws *WebServer) writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// Start 启动Web服务器
func (ws *WebServer) Start() error {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()

	// 设置路由（包括组件路由）
	ws.setupRoutes()

	ws.running = true

	log.Printf("Starting web server on %s", ws.server.Addr)

	if ws.config.EnableTLS {
		return ws.server.ListenAndServeTLS(ws.config.TLS.CertFile, ws.config.TLS.KeyFile)
	}

	return ws.server.ListenAndServe()
}

// Stop 停止Web服务器
func (ws *WebServer) Stop(ctx context.Context) error {
	ws.mutex.Lock()
	ws.running = false
	ws.mutex.Unlock()

	log.Println("Stopping web server...")

	// 只停止HTTP服务器，其他组件由main管理
	return ws.server.Shutdown(ctx)
}

// IsRunning 检查服务器是否运行中
func (ws *WebServer) IsRunning() bool {
	ws.mutex.RLock()
	defer ws.mutex.RUnlock()
	return ws.running
}

// RegisterComponent 注册组件
func (ws *WebServer) RegisterComponent(name string, component ComponentManager) error {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()

	if _, exists := ws.components[name]; exists {
		return fmt.Errorf("component %s already registered", name)
	}

	ws.components[name] = component
	log.Printf("Component %s registered successfully", name)

	// 如果服务器已经运行，立即设置组件路由
	if ws.running {
		if err := component.SetupRoutes(ws.router); err != nil {
			// 如果路由设置失败，移除组件注册
			delete(ws.components, name)
			return fmt.Errorf("failed to setup routes for component %s: %w", name, err)
		}
		log.Printf("Routes for component %s setup successfully", name)
	}

	return nil
}

// UnregisterComponent 注销组件
func (ws *WebServer) UnregisterComponent(name string) error {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()

	if _, exists := ws.components[name]; !exists {
		return fmt.Errorf("component %s not found", name)
	}

	delete(ws.components, name)
	log.Printf("Component %s unregistered successfully", name)

	// 注意：路由无法动态移除，需要重启服务器才能完全移除组件路由
	return nil
}

// GetComponent 获取已注册的组件
func (ws *WebServer) GetComponent(name string) (ComponentManager, bool) {
	ws.mutex.RLock()
	defer ws.mutex.RUnlock()

	component, exists := ws.components[name]
	return component, exists
}

// ListComponents 列出所有已注册的组件名称
func (ws *WebServer) ListComponents() []string {
	ws.mutex.RLock()
	defer ws.mutex.RUnlock()

	names := make([]string, 0, len(ws.components))
	for name := range ws.components {
		names = append(names, name)
	}
	return names
}

// setupComponentRoutes 设置组件路由
// 注意：调用此方法的函数必须已经持有mutex锁
func (ws *WebServer) setupComponentRoutes() error {
	log.Printf("Setting up routes for %d registered components", len(ws.components))

	// 为每个已注册的组件设置路由
	for name, component := range ws.components {
		log.Printf("Setting up routes for component: %s", name)

		// 先检查组件是否为nil
		if component == nil {
			log.Printf("Component %s is nil, skipping", name)
			continue
		}

		if err := component.SetupRoutes(ws.router); err != nil {
			log.Printf("Failed to setup routes for component %s: %v", name, err)
			return fmt.Errorf("failed to setup routes for component %s: %w", name, err)
		}
		log.Printf("Routes for component %s setup successfully", name)
	}

	log.Println("All component routes setup completed")
	return nil
}

// handleIndex 处理主页请求
func (s *WebServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	indexPath := filepath.Join(s.config.StaticDir, "index.html")

	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		// 返回内置的HTML界面
		s.serveBuiltinHTML(w, r)
		return
	}

	http.ServeFile(w, r, indexPath)
}

// handleWebSocketTest 处理WebSocket测试页面请求
func (s *WebServer) handleWebSocketTest(w http.ResponseWriter, r *http.Request) {
	testPath := filepath.Join(s.config.StaticDir, "websocket-test.html")

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		// 返回内置的测试页面
		s.serveBuiltinTestHTML(w, r)
		return
	}

	http.ServeFile(w, r, testPath)
}

// handleSimpleTest 处理简单测试页面请求
func (s *WebServer) handleSimpleTest(w http.ResponseWriter, r *http.Request) {
	testPath := filepath.Join(s.config.StaticDir, "simple-test.html")

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		// 返回内置的简单测试页面
		s.serveBuiltinSimpleTestHTML(w, r)
		return
	}

	http.ServeFile(w, r, testPath)
}

// serveBuiltinHTML 提供内置HTML界面
func (s *WebServer) serveBuiltinHTML(w http.ResponseWriter, r *http.Request) {
	// 尝试从嵌入的静态文件中获取HTML内容
	htmlContent, err := GetStaticFileContent("index.html")
	if err != nil {
		http.Error(w, "Failed to load embedded HTML file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(htmlContent)
}

// serveBuiltinTestHTML 提供内置测试页面
func (s *WebServer) serveBuiltinTestHTML(w http.ResponseWriter, r *http.Request) {
	// 尝试从嵌入的静态文件中获取测试页面内容
	htmlContent, err := GetStaticFileContent("websocket-test.html")
	if err != nil {
		http.Error(w, "Failed to load embedded test HTML file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(htmlContent)
}

// serveBuiltinSimpleTestHTML 提供内置简单测试页面
func (s *WebServer) serveBuiltinSimpleTestHTML(w http.ResponseWriter, r *http.Request) {
	// 尝试从嵌入的静态文件中获取简单测试页面内容
	htmlContent, err := GetStaticFileContent("simple-test.html")
	if err != nil {
		http.Error(w, "Failed to load embedded simple test HTML file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(htmlContent)
}

// SetAuthorizer 设置认证组件
func (ws *WebServer) SetAuthorizer(authorizer auth.Authorizer) {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()
	ws.authorizer = authorizer
}

// setupAuthRoutes 设置认证路由
func (ws *WebServer) setupAuthRoutes() {
	// 只有在认证启用且 authorizer 存在时才注册认证路由
	if !ws.config.Auth.Enabled || ws.authorizer == nil {
		return
	}

	authAPI := ws.router.PathPrefix("/auth").Subrouter()
	authAPI.HandleFunc("/login", ws.handleLogin).Methods("POST")
	authAPI.HandleFunc("/logout", ws.handleLogout).Methods("POST")
	authAPI.HandleFunc("/status", ws.handleAuthStatus).Methods("GET")
	authAPI.HandleFunc("/refresh", ws.handleRefresh).Methods("POST")
	authAPI.HandleFunc("/users", ws.handleUsers).Methods("GET")
	authAPI.HandleFunc("/users/{id}", ws.handleUserInfo).Methods("GET")
	authAPI.HandleFunc("/users/{id}/permissions", ws.handleUserPermissions).Methods("GET")
	authAPI.HandleFunc("/users/{id}/roles", ws.handleUserRoles).Methods("GET", "POST", "DELETE")
	authAPI.HandleFunc("/permissions/check", ws.handlePermissionCheck).Methods("POST")
}

// authMiddleware 认证中间件
func (ws *WebServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 跳过认证相关的路径
		if strings.HasPrefix(r.URL.Path, "/auth/") ||
			strings.HasPrefix(r.URL.Path, "/health") ||
			strings.HasPrefix(r.URL.Path, "/static/") ||
			strings.HasPrefix(r.URL.Path, "/assets/") ||
			strings.HasPrefix(r.URL.Path, "/css/") ||
			strings.HasPrefix(r.URL.Path, "/js/") {
			next.ServeHTTP(w, r)
			return
		}

		// 从请求头获取用户信息（简化实现）
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			userID = "anonymous"
		}

		// 检查基本权限
		if err := ws.authorizer.CheckPermission(userID, auth.PermissionDesktopView); err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// 将用户信息添加到请求上下文中
		r.Header.Set("X-Authenticated-User", userID)
		next.ServeHTTP(w, r)
	})
}

// Auth handler methods (moved from auth package)

// handleLogin 处理登录请求
func (ws *WebServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !ws.config.Auth.Enabled || ws.authorizer == nil {
		http.Error(w, "Auth not enabled", http.StatusServiceUnavailable)
		return
	}

	var loginReq struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// TODO: 实现实际的用户认证逻辑
	// 这里应该验证用户名和密码，检查用户状态等

	// 简化实现：检查用户是否存在于授权器中
	if inMemoryAuth, ok := ws.authorizer.(*auth.InMemoryAuthorizer); ok {
		if user, err := inMemoryAuth.GetUser(loginReq.Username); err == nil && user.Active {
			// 用户存在且活跃
			response := map[string]interface{}{
				"success": true,
				"token":   "dummy-token-" + fmt.Sprintf("%d", time.Now().Unix()),
				"user": map[string]interface{}{
					"id":       user.ID,
					"username": user.Username,
					"email":    user.Email,
					"roles":    user.Roles,
					"active":   user.Active,
				},
			}
			ws.writeJSON(w, response)
			return
		}
	}

	// 创建新用户（如果不存在）
	response := map[string]interface{}{
		"success": true,
		"token":   "dummy-token-" + fmt.Sprintf("%d", time.Now().Unix()),
		"user": map[string]interface{}{
			"id":       "user_" + loginReq.Username,
			"username": loginReq.Username,
			"role":     ws.config.Auth.DefaultRole,
		},
	}

	ws.writeJSON(w, response)
}

// handleLogout 处理登出请求
func (ws *WebServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !ws.config.Auth.Enabled || ws.authorizer == nil {
		http.Error(w, "Auth not enabled", http.StatusServiceUnavailable)
		return
	}

	// TODO: 实现实际的登出逻辑，如清除会话、撤销令牌等

	ws.writeJSON(w, map[string]interface{}{
		"status":  "success",
		"message": "Logged out successfully",
	})
}

// handleAuthStatus 处理认证状态请求
func (ws *WebServer) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "anonymous"
	}

	status := map[string]interface{}{
		"authenticated": ws.config.Auth.Enabled,
		"user_id":       userID,
		"auth_enabled":  ws.config.Auth.Enabled,
		"default_role":  ws.config.Auth.DefaultRole,
	}

	// 如果有具体用户，获取用户信息
	if userID != "anonymous" && ws.authorizer != nil {
		if inMemoryAuth, ok := ws.authorizer.(*auth.InMemoryAuthorizer); ok {
			if user, err := inMemoryAuth.GetUser(userID); err == nil {
				status["user"] = map[string]interface{}{
					"id":       user.ID,
					"username": user.Username,
					"email":    user.Email,
					"roles":    user.Roles,
					"active":   user.Active,
				}
			}
		}
	}

	ws.writeJSON(w, status)
}

// handleRefresh 处理令牌刷新请求
func (ws *WebServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if !ws.config.Auth.Enabled || ws.authorizer == nil {
		http.Error(w, "Auth not enabled", http.StatusServiceUnavailable)
		return
	}

	// TODO: 实现令牌刷新逻辑
	// 这里应该验证现有令牌，生成新令牌等

	response := map[string]interface{}{
		"success": true,
		"token":   "refreshed-token-" + fmt.Sprintf("%d", time.Now().Unix()),
	}

	ws.writeJSON(w, response)
}

// handleUsers 处理用户列表请求
func (ws *WebServer) handleUsers(w http.ResponseWriter, r *http.Request) {
	if !ws.config.Auth.Enabled || ws.authorizer == nil {
		http.Error(w, "Auth not enabled", http.StatusServiceUnavailable)
		return
	}

	// 检查用户管理权限
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "anonymous"
	}

	if err := ws.authorizer.CheckPermission(userID, auth.PermissionUserManage); err != nil {
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}

	// TODO: 实现用户管理
	// 这里应该从授权器获取用户列表
	users := []map[string]interface{}{
		{
			"id":       "admin",
			"username": "admin",
			"roles":    []string{"admin"},
			"active":   true,
		},
		{
			"id":       "demo",
			"username": "demo",
			"roles":    []string{ws.config.Auth.DefaultRole},
			"active":   true,
		},
	}

	ws.writeJSON(w, users)
}

// handleUserInfo 处理单个用户信息请求
func (ws *WebServer) handleUserInfo(w http.ResponseWriter, r *http.Request) {
	if !ws.config.Auth.Enabled || ws.authorizer == nil {
		http.Error(w, "Auth not enabled", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	targetUserID := vars["id"]

	// 检查权限：用户只能查看自己的信息，或者有用户管理权限
	currentUserID := r.Header.Get("X-User-ID")
	if currentUserID == "" {
		currentUserID = "anonymous"
	}

	if currentUserID != targetUserID {
		if err := ws.authorizer.CheckPermission(currentUserID, auth.PermissionUserManage); err != nil {
			http.Error(w, "Permission denied", http.StatusForbidden)
			return
		}
	}

	// 获取用户信息
	if inMemoryAuth, ok := ws.authorizer.(*auth.InMemoryAuthorizer); ok {
		if user, err := inMemoryAuth.GetUser(targetUserID); err == nil {
			ws.writeJSON(w, user)
			return
		}
	}

	http.Error(w, "User not found", http.StatusNotFound)
}

// handleUserPermissions 处理用户权限请求
func (ws *WebServer) handleUserPermissions(w http.ResponseWriter, r *http.Request) {
	if !ws.config.Auth.Enabled || ws.authorizer == nil {
		http.Error(w, "Auth not enabled", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	targetUserID := vars["id"]

	// 检查权限
	currentUserID := r.Header.Get("X-User-ID")
	if currentUserID == "" {
		currentUserID = "anonymous"
	}

	if currentUserID != targetUserID {
		if err := ws.authorizer.CheckPermission(currentUserID, auth.PermissionUserManage); err != nil {
			http.Error(w, "Permission denied", http.StatusForbidden)
			return
		}
	}

	// 获取用户权限
	permissions, err := ws.authorizer.GetUserPermissions(targetUserID)
	if err != nil {
		http.Error(w, "Failed to get user permissions", http.StatusInternalServerError)
		return
	}

	ws.writeJSON(w, map[string]interface{}{
		"user_id":     targetUserID,
		"permissions": permissions,
	})
}

// handleUserRoles 处理用户角色请求
func (ws *WebServer) handleUserRoles(w http.ResponseWriter, r *http.Request) {
	if !ws.config.Auth.Enabled || ws.authorizer == nil {
		http.Error(w, "Auth not enabled", http.StatusServiceUnavailable)
		return
	}

	vars := mux.Vars(r)
	targetUserID := vars["id"]

	// 检查权限
	currentUserID := r.Header.Get("X-User-ID")
	if currentUserID == "" {
		currentUserID = "anonymous"
	}

	if err := ws.authorizer.CheckPermission(currentUserID, auth.PermissionUserManage); err != nil {
		http.Error(w, "Permission denied", http.StatusForbidden)
		return
	}

	switch r.Method {
	case "GET":
		// 获取用户角色
		if inMemoryAuth, ok := ws.authorizer.(*auth.InMemoryAuthorizer); ok {
			if user, err := inMemoryAuth.GetUser(targetUserID); err == nil {
				ws.writeJSON(w, map[string]interface{}{
					"user_id": targetUserID,
					"roles":   user.Roles,
				})
				return
			}
		}
		http.Error(w, "User not found", http.StatusNotFound)

	case "POST":
		// 添加角色
		var req struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if err := ws.authorizer.AssignRole(targetUserID, req.Role); err != nil {
			http.Error(w, "Failed to assign role", http.StatusInternalServerError)
			return
		}

		ws.writeJSON(w, map[string]interface{}{
			"status":  "success",
			"message": fmt.Sprintf("Role %s assigned to user %s", req.Role, targetUserID),
		})

	case "DELETE":
		// 移除角色
		var req struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if err := ws.authorizer.RemoveRole(targetUserID, req.Role); err != nil {
			http.Error(w, "Failed to remove role", http.StatusInternalServerError)
			return
		}

		ws.writeJSON(w, map[string]interface{}{
			"status":  "success",
			"message": fmt.Sprintf("Role %s removed from user %s", req.Role, targetUserID),
		})
	}
}

// handlePermissionCheck 处理权限检查请求
func (ws *WebServer) handlePermissionCheck(w http.ResponseWriter, r *http.Request) {
	if !ws.config.Auth.Enabled || ws.authorizer == nil {
		http.Error(w, "Auth not enabled", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		UserID     string `json:"user_id"`
		Permission string `json:"permission"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// 检查权限
	err := ws.authorizer.CheckPermission(req.UserID, req.Permission)
	hasPermission := err == nil

	response := map[string]interface{}{
		"user_id":        req.UserID,
		"permission":     req.Permission,
		"has_permission": hasPermission,
	}

	if !hasPermission {
		response["error"] = err.Error()
	}

	ws.writeJSON(w, response)
}

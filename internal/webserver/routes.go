package webserver

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

func (ws *WebServer) setupRoutes() {
	log.Println("Setting up webserver routes...")
	ws.router = mux.NewRouter()

	// 只有在server存在时才设置Handler
	if ws.server != nil {
		ws.server.Handler = ws.router
	}

	log.Println("Setting up middleware...")
	if ws.config.EnableCORS {
		ws.router.Use(ws.corsMiddleware)
	}
	ws.router.Use(ws.loggingMiddleware)

	if ws.config.Auth.Enabled && ws.authorizer != nil {
		ws.router.Use(ws.authMiddleware)
	}

	// 设置高优先级路由（API、认证、健康检查等）
	log.Println("Setting up basic routes...")
	ws.setupBasicRoutes()
	log.Println("Setting up auth routes...")
	ws.setupAuthRoutes()

	// Setup component routes
	log.Println("Setting up component routes...")
	if err := ws.setupComponentRoutes(); err != nil {
		log.Printf("Component routes setup failed: %v", err)
		// Log error but don't fail - this allows the server to start even if some components fail
		ws.router.HandleFunc("/api/component-routes-error", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, fmt.Sprintf("Component routes setup failed: %v", err), http.StatusInternalServerError)
		}).Methods("GET")
	} else {
		log.Println("Component routes setup completed successfully")
	}

	// 设置测试路由
	ws.router.HandleFunc("/test/websocket", ws.handleWebSocketTest).Methods("GET")
	ws.router.HandleFunc("/test/simple", ws.handleSimpleTest).Methods("GET")

	// 最后设置静态文件路由（最低优先级）
	log.Println("Setting up static routes...")
	ws.setupStaticRoutes()
}

func (ws *WebServer) setupStaticRoutes() {
	// 创建静态文件路由器和优先级管理器
	staticFileRouter := CreateStaticFileRouterFromConfig(ws.config.StaticDir, ws.config.DefaultFile)
	priorityManager := NewRoutePriorityManager()

	// 创建新的静态文件处理器，支持根路径直接访问
	staticHandler := ws.createStaticFileHandler(staticFileRouter, priorityManager)

	// 保持向后兼容性：继续支持/static/路径前缀
	if ws.config.StaticDir != "" {
		if _, err := os.Stat(ws.config.StaticDir); err == nil {
			legacyFS := http.FileServer(http.Dir(ws.config.StaticDir))
			ws.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", legacyFS))
		}
	} else {
		legacyHandler := GetStaticFileHandler()
		ws.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", legacyHandler))
	}

	// 设置NotFoundHandler来处理所有未匹配的路径（包括静态文件和SPA回退）
	ws.router.NotFoundHandler = staticHandler
}

func (ws *WebServer) setupBasicRoutes() {
	ws.router.HandleFunc("/api/status", ws.handleStatus).Methods("GET")
	ws.router.HandleFunc("/api/version", ws.handleVersion).Methods("GET")
	ws.router.HandleFunc("/api/system/stats", ws.handleStats).Methods("GET")
	ws.router.HandleFunc("/health", ws.handleHealth).Methods("GET")
	ws.router.HandleFunc("/api/components", ws.handleComponentList).Methods("GET")
	ws.router.HandleFunc("/api/components/{name}", ws.handleComponentStatus).Methods("GET")
	ws.router.HandleFunc("/api/components/{name}/stats", ws.handleComponentStats).Methods("GET")
}

// createStaticFileHandler 创建新的静态文件处理器，集成StaticFileRouter和RoutePriorityManager
func (ws *WebServer) createStaticFileHandler(staticRouter *StaticFileRouter, priorityManager *RoutePriorityManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// 检查路径优先级：如果是保留路径，返回404
		if !priorityManager.ShouldServeAsStatic(path) {
			// 这是保留路径（API、认证等），应该已经被其他处理器处理
			// 如果到达这里，说明没有找到对应的处理器，返回404
			log.Printf("Reserved path accessed but no handler found: %s", path)
			http.NotFound(w, r)
			return
		}

		// 尝试提供静态文件
		err := staticRouter.ServeStaticFile(w, r)
		if err != nil {
			// 处理不同类型的错误
			ws.handleStaticFileError(w, r, err, staticRouter, priorityManager)
		}
	})
}

// handleStaticFileError 处理静态文件服务错误
func (ws *WebServer) handleStaticFileError(w http.ResponseWriter, r *http.Request, err error, staticRouter *StaticFileRouter, priorityManager *RoutePriorityManager) {
	path := r.URL.Path

	// 检查错误类型
	if staticErr, ok := err.(*StaticFileError); ok {
		log.Printf("Static file error: %v", staticErr)

		switch staticErr.Type {
		case ErrorTypeFileNotFound:
			// 文件不存在，尝试SPA回退
			ws.handleSPAFallback(w, r, staticRouter, priorityManager)
			return

		case ErrorTypePermissionDenied:
			log.Printf("Permission denied for file: %s", staticErr.Path)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return

		case ErrorTypeReadError:
			log.Printf("Read error for file: %s - %v", staticErr.Path, staticErr.Err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return

		case ErrorTypeInvalidPath:
			log.Printf("Invalid path rejected: %s", staticErr.Path)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return

		case ErrorTypeStatError:
			log.Printf("Stat error for file: %s - %v", staticErr.Path, staticErr.Err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return

		default:
			log.Printf("Unknown static file error type: %s for path: %s", staticErr.Type, staticErr.Path)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}

	// 未知错误类型，记录并尝试SPA回退
	log.Printf("Unknown error serving static file %s: %v", path, err)
	ws.handleSPAFallback(w, r, staticRouter, priorityManager)
}

// handleSPAFallback 处理SPA回退机制
//
// 实现要求：
// - Requirement 4.1: 当静态文件不存在时返回 index.html
// - Requirement 4.2: 非API路径回退到主页
// - Requirement 4.3: API路径返回正确的404错误，而非回退到主页
func (ws *WebServer) handleSPAFallback(w http.ResponseWriter, r *http.Request, staticRouter *StaticFileRouter, priorityManager *RoutePriorityManager) {
	path := r.URL.Path

	log.Printf("Attempting SPA fallback for path: %s", path)

	// Requirement 4.3: 如果是API路径，返回404而不是回退到index.html
	if priorityManager.IsAPIPath(path) {
		log.Printf("API path not found, returning 404: %s", path)
		http.NotFound(w, r)
		return
	}

	// Requirements 4.1 & 4.2: 对于非API路径，尝试返回默认文件（SPA回退）
	defaultFileRequest := &http.Request{
		Method: "GET",
		URL:    r.URL,
	}
	defaultFileRequest.URL.Path = "/" + staticRouter.GetDefaultFile()

	log.Printf("Serving default file for SPA fallback: %s", defaultFileRequest.URL.Path)

	// 尝试提供默认文件
	err := staticRouter.ServeStaticFile(w, defaultFileRequest)
	if err != nil {
		// 如果连默认文件都不存在，返回404
		log.Printf("Default file not found for SPA fallback, returning 404: %v", err)
		http.NotFound(w, r)
		return
	}

	log.Printf("SPA fallback successful for path: %s", path)
}

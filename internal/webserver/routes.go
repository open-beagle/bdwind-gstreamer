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

	log.Println("Setting up basic routes...")
	ws.setupBasicRoutes()
	log.Println("Setting up auth routes...")
	ws.setupAuthRoutes()
	log.Println("Setting up static routes...")
	ws.setupStaticRoutes()

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

	ws.router.HandleFunc("/test/websocket", ws.handleWebSocketTest).Methods("GET")
	ws.router.HandleFunc("/test/simple", ws.handleSimpleTest).Methods("GET")
	ws.router.HandleFunc("/", ws.handleIndex).Methods("GET")
	ws.router.NotFoundHandler = http.HandlerFunc(ws.handleIndex)
}

func (ws *WebServer) setupStaticRoutes() {
	staticDir := ws.config.StaticDir
	if staticDir != "" {
		if _, err := os.Stat(staticDir); err == nil {
			fs := http.FileServer(http.Dir(staticDir))
			ws.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", fs))
			ws.router.PathPrefix("/assets/").Handler(http.StripPrefix("/assets/", fs))
			ws.router.PathPrefix("/css/").Handler(http.StripPrefix("/css/", fs))
			ws.router.PathPrefix("/js/").Handler(http.StripPrefix("/js/", fs))
			return
		}
	}

	staticHandler := GetStaticFileHandler()
	ws.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", staticHandler))
	ws.router.PathPrefix("/assets/").Handler(http.StripPrefix("/assets/", staticHandler))
	ws.router.PathPrefix("/css/").Handler(http.StripPrefix("/css/", staticHandler))
	ws.router.PathPrefix("/js/").Handler(http.StripPrefix("/js/", staticHandler))
}

func (ws *WebServer) setupBasicRoutes() {
	ws.router.HandleFunc("/api/status", ws.handleStatus).Methods("GET")
	ws.router.HandleFunc("/api/version", ws.handleVersion).Methods("GET")
	ws.router.HandleFunc("/health", ws.handleHealth).Methods("GET")
	ws.router.HandleFunc("/api/components", ws.handleComponentList).Methods("GET")
	ws.router.HandleFunc("/api/components/{name}", ws.handleComponentStatus).Methods("GET")
	ws.router.HandleFunc("/api/components/{name}/stats", ws.handleComponentStats).Methods("GET")
}

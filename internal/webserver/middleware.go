package webserver

import (
	"net/http"
	"time"

	"github.com/open-beagle/bdwind-gstreamer/internal/config"
)

// 中间件
func (ws *WebServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (ws *WebServer) loggingMiddleware(next http.Handler) http.Handler {
	logger := config.GetLoggerWithPrefix("webserver-middleware")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Debugf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

package webserver

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// StaticFileError 静态文件服务错误类型
type StaticFileError struct {
	Type    string // 错误类型：file_not_found, permission_denied, read_error, invalid_path
	Path    string // 相关路径
	Message string // 错误消息
	Err     error  // 原始错误
}

func (e *StaticFileError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (path: %s): %v", e.Type, e.Message, e.Path, e.Err)
	}
	return fmt.Sprintf("%s: %s (path: %s)", e.Type, e.Message, e.Path)
}

// 错误类型常量
const (
	ErrorTypeFileNotFound     = "file_not_found"
	ErrorTypePermissionDenied = "permission_denied"
	ErrorTypeReadError        = "read_error"
	ErrorTypeInvalidPath      = "invalid_path"
	ErrorTypeStatError        = "stat_error"
)

// FileExistenceCache 文件存在性缓存条目
type FileExistenceCache struct {
	exists     bool
	isExternal bool
	lastCheck  time.Time
	etag       string
	size       int64
	modTime    time.Time
}

// StaticFileRouter 静态文件路由器
type StaticFileRouter struct {
	externalDir string
	embeddedFS  fs.FS
	defaultFile string
	mimeTypes   map[string]string
	logger      *log.Logger

	// 性能优化相关字段
	existenceCache map[string]*FileExistenceCache
	cacheMutex     sync.RWMutex
	cacheTimeout   time.Duration
	enableETag     bool
	maxCacheSize   int
	bufferPool     sync.Pool
}

// StaticFileContext 静态文件请求上下文
type StaticFileContext struct {
	RequestPath  string    // 原始请求路径
	FilePath     string    // 实际文件路径
	IsExternal   bool      // 是否来自外部目录
	ContentType  string    // MIME类型
	LastModified time.Time // 文件修改时间
	Size         int64     // 文件大小
	ETag         string    // ETag值
}

// NewStaticFileRouter 创建静态文件路由器
func NewStaticFileRouter(externalDir string, embeddedFS fs.FS, defaultFile string) *StaticFileRouter {
	if defaultFile == "" {
		defaultFile = "index.html"
	}

	// 创建专用的日志器
	logger := log.New(log.Writer(), "[STATIC-ROUTER] ", log.LstdFlags)

	router := &StaticFileRouter{
		externalDir: externalDir,
		embeddedFS:  embeddedFS,
		defaultFile: defaultFile,
		mimeTypes:   getDefaultMimeTypes(),
		logger:      logger,

		// 性能优化设置
		existenceCache: make(map[string]*FileExistenceCache),
		cacheTimeout:   5 * time.Minute, // 缓存5分钟
		enableETag:     true,
		maxCacheSize:   1000, // 最多缓存1000个文件信息
		bufferPool: sync.Pool{
			New: func() interface{} {
				// 创建64KB的缓冲区用于文件传输
				return make([]byte, 64*1024)
			},
		},
	}

	// 记录初始化信息
	logger.Printf("Static file router initialized - External dir: %s, Default file: %s", externalDir, defaultFile)
	logger.Printf("Performance features enabled - ETag: %v, Cache timeout: %v, Max cache size: %d",
		router.enableETag, router.cacheTimeout, router.maxCacheSize)

	if externalDir != "" {
		if _, err := os.Stat(externalDir); err != nil {
			logger.Printf("Warning: External directory not accessible: %v", err)
		} else {
			logger.Printf("External directory verified: %s", externalDir)
		}
	}

	return router
}

// getDefaultMimeTypes 获取默认MIME类型映射
func getDefaultMimeTypes() map[string]string {
	return map[string]string{
		".html":  "text/html; charset=utf-8",
		".htm":   "text/html; charset=utf-8",
		".css":   "text/css",
		".js":    "application/javascript",
		".json":  "application/json",
		".xml":   "application/xml",
		".png":   "image/png",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".gif":   "image/gif",
		".svg":   "image/svg+xml",
		".ico":   "image/x-icon",
		".woff":  "font/woff",
		".woff2": "font/woff2",
		".ttf":   "font/ttf",
		".eot":   "application/vnd.ms-fontobject",
		".txt":   "text/plain; charset=utf-8",
		".pdf":   "application/pdf",
		".zip":   "application/zip",
	}
}

// ServeStaticFile 处理静态文件请求
func (sfr *StaticFileRouter) ServeStaticFile(w http.ResponseWriter, r *http.Request) error {
	startTime := time.Now()
	originalPath := r.URL.Path

	sfr.logger.Printf("Serving static file request: %s %s", r.Method, originalPath)

	// 清理和验证路径
	cleanPath := sfr.sanitizePath(originalPath)
	if cleanPath == "" {
		err := &StaticFileError{
			Type:    ErrorTypeInvalidPath,
			Path:    originalPath,
			Message: "Path contains invalid characters or directory traversal attempts",
		}
		sfr.logger.Printf("Invalid path rejected: %s - %v", originalPath, err)
		return err
	}

	sfr.logger.Printf("Path sanitized: %s -> %s", originalPath, cleanPath)

	// 检查文件是否存在
	exists, isExternal := sfr.FileExists(cleanPath)
	if !exists {
		err := &StaticFileError{
			Type:    ErrorTypeFileNotFound,
			Path:    cleanPath,
			Message: "File not found in external directory or embedded filesystem",
		}
		sfr.logger.Printf("File not found: %s", cleanPath)
		return err
	}

	source := "embedded"
	if isExternal {
		source = "external"
	}
	sfr.logger.Printf("File found in %s filesystem: %s", source, cleanPath)

	// 获取缓存的文件信息
	cachedInfo := sfr.getCachedFileInfo(cleanPath)

	// 创建文件上下文
	ctx := &StaticFileContext{
		RequestPath: originalPath,
		FilePath:    cleanPath,
		IsExternal:  isExternal,
		ContentType: sfr.GetContentType(cleanPath),
	}

	// 如果有缓存信息，使用缓存的元数据
	if cachedInfo != nil && cachedInfo.exists {
		ctx.LastModified = cachedInfo.modTime
		ctx.Size = cachedInfo.size
		ctx.ETag = cachedInfo.etag
		sfr.logger.Printf("Using cached metadata for file: %s (size: %d, etag: %s)",
			cleanPath, ctx.Size, ctx.ETag)
	}

	sfr.logger.Printf("Content type detected: %s for file: %s", ctx.ContentType, cleanPath)

	// 检查条件请求（If-None-Match, If-Modified-Since）
	if sfr.checkConditionalRequest(r, ctx) {
		sfr.logger.Printf("Conditional request matched, returning 304 Not Modified for: %s", cleanPath)
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	// 提供文件内容（响应头在各自的serve方法中设置）
	var err error
	if isExternal {
		err = sfr.serveExternalFile(w, r, ctx)
	} else {
		err = sfr.serveEmbeddedFile(w, r, ctx)
	}

	duration := time.Since(startTime)
	if err != nil {
		sfr.logger.Printf("Failed to serve file %s from %s filesystem: %v (took %v)", cleanPath, source, err, duration)
		return err
	}

	sfr.logger.Printf("Successfully served file %s from %s filesystem (took %v)", cleanPath, source, duration)
	return nil
}

// FileExists 检查文件是否存在（带缓存）
func (sfr *StaticFileRouter) FileExists(path string) (bool, bool) {
	sfr.logger.Printf("Checking file existence: %s", path)

	// 检查缓存
	if cached := sfr.getCachedFileInfo(path); cached != nil {
		sfr.logger.Printf("File existence found in cache: %s (exists: %v, external: %v)",
			path, cached.exists, cached.isExternal)
		return cached.exists, cached.isExternal
	}

	// 缓存未命中，执行实际检查
	exists, isExternal, cacheInfo := sfr.checkFileExistence(path)

	// 更新缓存
	sfr.setCachedFileInfo(path, cacheInfo)

	return exists, isExternal
}

// checkFileExistence 实际检查文件是否存在
func (sfr *StaticFileRouter) checkFileExistence(path string) (bool, bool, *FileExistenceCache) {
	cacheInfo := &FileExistenceCache{
		lastCheck: time.Now(),
	}

	// 首先检查外部目录
	if sfr.externalDir != "" {
		externalPath := filepath.Join(sfr.externalDir, path)
		sfr.logger.Printf("Checking external path: %s", externalPath)

		if info, err := os.Stat(externalPath); err == nil {
			if info.IsDir() {
				sfr.logger.Printf("Path is directory (skipping): %s", externalPath)
			} else {
				sfr.logger.Printf("File found in external directory: %s (size: %d bytes)", externalPath, info.Size())
				cacheInfo.exists = true
				cacheInfo.isExternal = true
				cacheInfo.size = info.Size()
				cacheInfo.modTime = info.ModTime()
				cacheInfo.etag = sfr.generateETag(externalPath, info.ModTime(), info.Size())
				return true, true, cacheInfo
			}
		} else {
			sfr.logger.Printf("External file not found or error: %s - %v", externalPath, err)
		}
	}

	// 然后检查嵌入文件系统
	if sfr.embeddedFS != nil {
		sfr.logger.Printf("Checking embedded filesystem: %s", path)

		if info, err := fs.Stat(sfr.embeddedFS, path); err == nil {
			if info.IsDir() {
				sfr.logger.Printf("Path is directory in embedded FS (skipping): %s", path)
			} else {
				sfr.logger.Printf("File found in embedded filesystem: %s (size: %d bytes)", path, info.Size())
				cacheInfo.exists = true
				cacheInfo.isExternal = false
				cacheInfo.size = info.Size()
				cacheInfo.modTime = info.ModTime()
				cacheInfo.etag = sfr.generateETag(path, info.ModTime(), info.Size())
				return true, false, cacheInfo
			}
		} else {
			sfr.logger.Printf("Embedded file not found or error: %s - %v", path, err)
		}
	}

	sfr.logger.Printf("File not found in any filesystem: %s", path)
	cacheInfo.exists = false
	return false, false, cacheInfo
}

// getCachedFileInfo 获取缓存的文件信息
func (sfr *StaticFileRouter) getCachedFileInfo(path string) *FileExistenceCache {
	sfr.cacheMutex.RLock()
	defer sfr.cacheMutex.RUnlock()

	cached, exists := sfr.existenceCache[path]
	if !exists {
		return nil
	}

	// 检查缓存是否过期
	if time.Since(cached.lastCheck) > sfr.cacheTimeout {
		sfr.logger.Printf("Cache expired for file: %s", path)
		return nil
	}

	return cached
}

// setCachedFileInfo 设置缓存的文件信息
func (sfr *StaticFileRouter) setCachedFileInfo(path string, info *FileExistenceCache) {
	sfr.cacheMutex.Lock()
	defer sfr.cacheMutex.Unlock()

	// 检查缓存大小限制
	if len(sfr.existenceCache) >= sfr.maxCacheSize {
		sfr.evictOldestCacheEntry()
	}

	sfr.existenceCache[path] = info
	sfr.logger.Printf("Cached file info for: %s (exists: %v, external: %v)",
		path, info.exists, info.isExternal)
}

// evictOldestCacheEntry 清除最旧的缓存条目
func (sfr *StaticFileRouter) evictOldestCacheEntry() {
	var oldestPath string
	var oldestTime time.Time

	for path, info := range sfr.existenceCache {
		if oldestPath == "" || info.lastCheck.Before(oldestTime) {
			oldestPath = path
			oldestTime = info.lastCheck
		}
	}

	if oldestPath != "" {
		delete(sfr.existenceCache, oldestPath)
		sfr.logger.Printf("Evicted oldest cache entry: %s", oldestPath)
	}
}

// generateETag 生成ETag
func (sfr *StaticFileRouter) generateETag(path string, modTime time.Time, size int64) string {
	if !sfr.enableETag {
		return ""
	}

	// 使用文件路径、修改时间和大小生成ETag
	h := md5.New()
	h.Write([]byte(path))
	h.Write([]byte(modTime.Format(time.RFC3339Nano)))
	h.Write([]byte(strconv.FormatInt(size, 10)))

	etag := fmt.Sprintf(`"%x"`, h.Sum(nil))
	sfr.logger.Printf("Generated ETag for %s: %s", path, etag)
	return etag
}

// ClearCache 清除所有缓存
func (sfr *StaticFileRouter) ClearCache() {
	sfr.cacheMutex.Lock()
	defer sfr.cacheMutex.Unlock()

	count := len(sfr.existenceCache)
	sfr.existenceCache = make(map[string]*FileExistenceCache)
	sfr.logger.Printf("Cleared %d cache entries", count)
}

// GetCacheStats 获取缓存统计信息
func (sfr *StaticFileRouter) GetCacheStats() (size int, maxSize int, timeout time.Duration) {
	sfr.cacheMutex.RLock()
	defer sfr.cacheMutex.RUnlock()

	return len(sfr.existenceCache), sfr.maxCacheSize, sfr.cacheTimeout
}

// SetCacheTimeout 设置缓存超时时间
func (sfr *StaticFileRouter) SetCacheTimeout(timeout time.Duration) {
	sfr.cacheMutex.Lock()
	defer sfr.cacheMutex.Unlock()

	sfr.cacheTimeout = timeout
	sfr.logger.Printf("Cache timeout set to: %v", timeout)
}

// SetETagEnabled 启用或禁用ETag
func (sfr *StaticFileRouter) SetETagEnabled(enabled bool) {
	sfr.enableETag = enabled
	sfr.logger.Printf("ETag support %s", map[bool]string{true: "enabled", false: "disabled"}[enabled])
}

// GetContentType 获取文件的Content-Type
func (sfr *StaticFileRouter) GetContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	// 首先检查自定义MIME类型
	if contentType, exists := sfr.mimeTypes[ext]; exists {
		return contentType
	}

	// 使用标准库检测
	contentType := mime.TypeByExtension(ext)
	if contentType != "" {
		return contentType
	}

	// 默认类型
	return "application/octet-stream"
}

// sanitizePath 清理和验证路径，防止目录遍历攻击
func (sfr *StaticFileRouter) sanitizePath(requestPath string) string {
	originalPath := requestPath
	sfr.logger.Printf("Sanitizing path: %s", originalPath)

	// 移除查询参数和片段
	if idx := strings.Index(requestPath, "?"); idx != -1 {
		requestPath = requestPath[:idx]
		sfr.logger.Printf("Removed query parameters: %s", requestPath)
	}
	if idx := strings.Index(requestPath, "#"); idx != -1 {
		requestPath = requestPath[:idx]
		sfr.logger.Printf("Removed fragment: %s", requestPath)
	}

	// 检查原始路径是否包含目录遍历尝试
	if strings.Contains(requestPath, "..") {
		sfr.logger.Printf("Directory traversal attempt detected in path: %s", originalPath)
		return ""
	}

	// 检查是否包含空字节（安全检查）
	if strings.Contains(requestPath, "\x00") {
		sfr.logger.Printf("Null byte detected in path: %s", originalPath)
		return ""
	}

	// 检查路径长度限制
	if len(requestPath) > 1024 {
		sfr.logger.Printf("Path too long (%d characters): %s", len(requestPath), originalPath)
		return ""
	}

	// 清理路径
	cleanPath := path.Clean(requestPath)
	sfr.logger.Printf("Path cleaned: %s -> %s", requestPath, cleanPath)

	// 移除前导斜杠
	cleanPath = strings.TrimPrefix(cleanPath, "/")

	// 再次检查清理后的路径是否包含目录遍历
	if strings.Contains(cleanPath, "..") {
		sfr.logger.Printf("Directory traversal attempt detected after cleaning: %s", cleanPath)
		return ""
	}

	// 检查是否为空或只包含斜杠
	if cleanPath == "" || cleanPath == "." {
		sfr.logger.Printf("Empty or root path, using default file: %s", sfr.defaultFile)
		return sfr.defaultFile
	}

	// 检查文件名是否包含非法字符（Windows兼容性）
	invalidChars := []string{"<", ">", ":", "\"", "|", "?", "*"}
	for _, char := range invalidChars {
		if strings.Contains(cleanPath, char) {
			sfr.logger.Printf("Invalid character '%s' detected in path: %s", char, cleanPath)
			return ""
		}
	}

	sfr.logger.Printf("Path sanitization successful: %s -> %s", originalPath, cleanPath)
	return cleanPath
}

// setResponseHeaders 设置HTTP响应头
func (sfr *StaticFileRouter) setResponseHeaders(w http.ResponseWriter, ctx *StaticFileContext) {
	sfr.logger.Printf("Setting response headers for file: %s", ctx.FilePath)

	// 设置Content-Type
	w.Header().Set("Content-Type", ctx.ContentType)
	sfr.logger.Printf("Content-Type set to: %s", ctx.ContentType)

	// 设置缓存头
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// 如果有修改时间，设置Last-Modified
	if !ctx.LastModified.IsZero() {
		lastModified := ctx.LastModified.UTC().Format(http.TimeFormat)
		w.Header().Set("Last-Modified", lastModified)
		sfr.logger.Printf("Last-Modified set to: %s", lastModified)
	}

	// 设置ETag（如果启用）
	if sfr.enableETag && ctx.ETag != "" {
		w.Header().Set("ETag", ctx.ETag)
		sfr.logger.Printf("ETag set to: %s", ctx.ETag)
	}

	// 设置Content-Length（如果已知）
	if ctx.Size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", ctx.Size))
		sfr.logger.Printf("Content-Length set to: %d", ctx.Size)
	}

	// 设置Accept-Ranges头以支持范围请求（用于大文件优化）
	w.Header().Set("Accept-Ranges", "bytes")

	sfr.logger.Printf("Response headers set successfully for file: %s", ctx.FilePath)
}

// checkConditionalRequest 检查条件请求
func (sfr *StaticFileRouter) checkConditionalRequest(r *http.Request, ctx *StaticFileContext) bool {
	// 检查If-None-Match (ETag)
	if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != "" && ctx.ETag != "" {
		if ifNoneMatch == ctx.ETag || ifNoneMatch == "*" {
			sfr.logger.Printf("ETag matched: %s", ifNoneMatch)
			return true
		}
	}

	// 检查If-Modified-Since
	if ifModifiedSince := r.Header.Get("If-Modified-Since"); ifModifiedSince != "" && !ctx.LastModified.IsZero() {
		if t, err := time.Parse(http.TimeFormat, ifModifiedSince); err == nil {
			if !ctx.LastModified.After(t.Add(1 * time.Second)) {
				sfr.logger.Printf("File not modified since %s", t.Format(time.RFC3339))
				return true
			}
		}
	}

	return false
}

// serveExternalFile 提供外部文件
func (sfr *StaticFileRouter) serveExternalFile(w http.ResponseWriter, r *http.Request, ctx *StaticFileContext) error {
	externalPath := filepath.Join(sfr.externalDir, ctx.FilePath)
	sfr.logger.Printf("Serving external file: %s", externalPath)

	// 如果上下文中没有文件信息，获取文件信息
	if ctx.Size == 0 || ctx.LastModified.IsZero() {
		info, err := os.Stat(externalPath)
		if err != nil {
			staticErr := &StaticFileError{
				Type:    ErrorTypeStatError,
				Path:    externalPath,
				Message: "Failed to get file information",
				Err:     err,
			}
			sfr.logger.Printf("Stat error for external file: %v", staticErr)
			return staticErr
		}

		// 更新上下文
		ctx.LastModified = info.ModTime()
		ctx.Size = info.Size()
		ctx.ETag = sfr.generateETag(externalPath, ctx.LastModified, ctx.Size)
	}

	sfr.logger.Printf("External file info - Size: %d bytes, Modified: %s, ETag: %s",
		ctx.Size, ctx.LastModified.Format(time.RFC3339), ctx.ETag)

	// 设置响应头
	sfr.setResponseHeaders(w, ctx)

	// 优化大文件传输
	if ctx.Size > 1024*1024 { // 大于1MB的文件使用流式传输
		return sfr.serveExternalFileStream(w, r, ctx, externalPath)
	}

	// 小文件直接读取到内存
	content, err := os.ReadFile(externalPath)
	if err != nil {
		var staticErr *StaticFileError

		// 检查是否是权限错误
		if os.IsPermission(err) {
			staticErr = &StaticFileError{
				Type:    ErrorTypePermissionDenied,
				Path:    externalPath,
				Message: "Permission denied when reading file",
				Err:     err,
			}
		} else {
			staticErr = &StaticFileError{
				Type:    ErrorTypeReadError,
				Path:    externalPath,
				Message: "Failed to read file content",
				Err:     err,
			}
		}

		sfr.logger.Printf("Read error for external file: %v", staticErr)
		return staticErr
	}

	sfr.logger.Printf("Successfully read external file: %s (%d bytes)", externalPath, len(content))

	// 写入内容
	bytesWritten, err := w.Write(content)
	if err != nil {
		sfr.logger.Printf("Error writing response for external file %s: %v", externalPath, err)
		return &StaticFileError{
			Type:    ErrorTypeReadError,
			Path:    externalPath,
			Message: "Failed to write response",
			Err:     err,
		}
	}

	sfr.logger.Printf("Successfully wrote %d bytes for external file: %s", bytesWritten, externalPath)
	return nil
}

// serveExternalFileStream 流式传输外部文件（用于大文件优化）
func (sfr *StaticFileRouter) serveExternalFileStream(w http.ResponseWriter, r *http.Request, ctx *StaticFileContext, externalPath string) error {
	sfr.logger.Printf("Using stream transfer for large external file: %s (%d bytes)", externalPath, ctx.Size)

	file, err := os.Open(externalPath)
	if err != nil {
		var staticErr *StaticFileError
		if os.IsPermission(err) {
			staticErr = &StaticFileError{
				Type:    ErrorTypePermissionDenied,
				Path:    externalPath,
				Message: "Permission denied when opening file",
				Err:     err,
			}
		} else {
			staticErr = &StaticFileError{
				Type:    ErrorTypeReadError,
				Path:    externalPath,
				Message: "Failed to open file for streaming",
				Err:     err,
			}
		}
		sfr.logger.Printf("Open error for external file: %v", staticErr)
		return staticErr
	}
	defer file.Close()

	// 使用缓冲池中的缓冲区进行流式传输
	buffer := sfr.bufferPool.Get().([]byte)
	defer sfr.bufferPool.Put(buffer)

	bytesWritten, err := io.CopyBuffer(w, file, buffer)
	if err != nil {
		sfr.logger.Printf("Error streaming external file %s: %v", externalPath, err)
		return &StaticFileError{
			Type:    ErrorTypeReadError,
			Path:    externalPath,
			Message: "Failed to stream file content",
			Err:     err,
		}
	}

	sfr.logger.Printf("Successfully streamed %d bytes for external file: %s", bytesWritten, externalPath)
	return nil
}

// serveEmbeddedFile 提供嵌入文件
func (sfr *StaticFileRouter) serveEmbeddedFile(w http.ResponseWriter, r *http.Request, ctx *StaticFileContext) error {
	sfr.logger.Printf("Serving embedded file: %s", ctx.FilePath)

	// 如果上下文中没有文件信息，获取文件信息
	if ctx.Size == 0 || ctx.LastModified.IsZero() {
		if info, err := fs.Stat(sfr.embeddedFS, ctx.FilePath); err == nil {
			ctx.LastModified = info.ModTime()
			ctx.Size = info.Size()
			ctx.ETag = sfr.generateETag(ctx.FilePath, ctx.LastModified, ctx.Size)
			sfr.logger.Printf("Embedded file info - Size: %d bytes, Modified: %s, ETag: %s",
				ctx.Size, ctx.LastModified.Format(time.RFC3339), ctx.ETag)
		} else {
			sfr.logger.Printf("Warning: Could not get embedded file info: %v", err)
		}
	}

	// 设置响应头
	sfr.setResponseHeaders(w, ctx)

	// 优化大文件传输
	if ctx.Size > 1024*1024 { // 大于1MB的文件使用流式传输
		return sfr.serveEmbeddedFileStream(w, r, ctx)
	}

	// 小文件直接读取到内存
	content, err := fs.ReadFile(sfr.embeddedFS, ctx.FilePath)
	if err != nil {
		staticErr := &StaticFileError{
			Type:    ErrorTypeReadError,
			Path:    ctx.FilePath,
			Message: "Failed to read embedded file content",
			Err:     err,
		}
		sfr.logger.Printf("Read error for embedded file: %v", staticErr)
		return staticErr
	}

	sfr.logger.Printf("Successfully read embedded file: %s (%d bytes)", ctx.FilePath, len(content))

	// 写入内容
	bytesWritten, err := w.Write(content)
	if err != nil {
		sfr.logger.Printf("Error writing response for embedded file %s: %v", ctx.FilePath, err)
		return &StaticFileError{
			Type:    ErrorTypeReadError,
			Path:    ctx.FilePath,
			Message: "Failed to write response",
			Err:     err,
		}
	}

	sfr.logger.Printf("Successfully wrote %d bytes for embedded file: %s", bytesWritten, ctx.FilePath)
	return nil
}

// serveEmbeddedFileStream 流式传输嵌入文件（用于大文件优化）
func (sfr *StaticFileRouter) serveEmbeddedFileStream(w http.ResponseWriter, r *http.Request, ctx *StaticFileContext) error {
	sfr.logger.Printf("Using stream transfer for large embedded file: %s (%d bytes)", ctx.FilePath, ctx.Size)

	file, err := sfr.embeddedFS.Open(ctx.FilePath)
	if err != nil {
		staticErr := &StaticFileError{
			Type:    ErrorTypeReadError,
			Path:    ctx.FilePath,
			Message: "Failed to open embedded file for streaming",
			Err:     err,
		}
		sfr.logger.Printf("Open error for embedded file: %v", staticErr)
		return staticErr
	}
	defer file.Close()

	// 使用缓冲池中的缓冲区进行流式传输
	buffer := sfr.bufferPool.Get().([]byte)
	defer sfr.bufferPool.Put(buffer)

	bytesWritten, err := io.CopyBuffer(w, file, buffer)
	if err != nil {
		sfr.logger.Printf("Error streaming embedded file %s: %v", ctx.FilePath, err)
		return &StaticFileError{
			Type:    ErrorTypeReadError,
			Path:    ctx.FilePath,
			Message: "Failed to stream embedded file content",
			Err:     err,
		}
	}

	sfr.logger.Printf("Successfully streamed %d bytes for embedded file: %s", bytesWritten, ctx.FilePath)
	return nil
}

// GetEmbeddedStaticFS 获取嵌入的静态文件系统（用于向后兼容）
func GetEmbeddedStaticFS() fs.FS {
	return GetStaticFS()
}

// GetDefaultFile 获取默认文件名
func (sfr *StaticFileRouter) GetDefaultFile() string {
	return sfr.defaultFile
}

// CreateStaticFileRouterFromConfig 从配置创建静态文件路由器
func CreateStaticFileRouterFromConfig(externalDir string, defaultFile string) *StaticFileRouter {
	embeddedFS := GetEmbeddedStaticFS()
	return NewStaticFileRouter(externalDir, embeddedFS, defaultFile)
}

// SetLogger 设置自定义日志器
func (sfr *StaticFileRouter) SetLogger(logger *log.Logger) {
	if logger != nil {
		sfr.logger = logger
		sfr.logger.Printf("Custom logger set for static file router")
	}
}

// GetLogger 获取当前日志器
func (sfr *StaticFileRouter) GetLogger() *log.Logger {
	return sfr.logger
}

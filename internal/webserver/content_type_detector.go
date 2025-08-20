package webserver

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

// ContentTypeDetector 内容类型检测器
type ContentTypeDetector struct {
	mimeTypes map[string]string
}

// NewContentTypeDetector 创建内容类型检测器
func NewContentTypeDetector() *ContentTypeDetector {
	return &ContentTypeDetector{
		mimeTypes: getContentTypeDetectorDefaultMimeTypes(),
	}
}

// NewContentTypeDetectorWithCustomTypes 创建带自定义类型映射的内容类型检测器
func NewContentTypeDetectorWithCustomTypes(customTypes map[string]string) *ContentTypeDetector {
	detector := &ContentTypeDetector{
		mimeTypes: getContentTypeDetectorDefaultMimeTypes(),
	}

	// 合并自定义类型
	for ext, contentType := range customTypes {
		detector.mimeTypes[ext] = contentType
	}

	return detector
}

// getContentTypeDetectorDefaultMimeTypes 获取内容类型检测器的默认MIME类型映射
func getContentTypeDetectorDefaultMimeTypes() map[string]string {
	return map[string]string{
		// HTML 文件
		".html": "text/html; charset=utf-8",
		".htm":  "text/html; charset=utf-8",

		// 样式表
		".css": "text/css",

		// JavaScript 文件
		".js":   "application/javascript",
		".mjs":  "application/javascript",
		".json": "application/json",

		// XML 文件
		".xml":   "application/xml",
		".xhtml": "application/xhtml+xml",

		// 图片文件
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".ico":  "image/x-icon",
		".webp": "image/webp",
		".bmp":  "image/bmp",
		".tiff": "image/tiff",
		".tif":  "image/tiff",

		// 字体文件
		".woff":  "font/woff",
		".woff2": "font/woff2",
		".ttf":   "font/ttf",
		".otf":   "font/otf",
		".eot":   "application/vnd.ms-fontobject",

		// 文本文件
		".txt": "text/plain; charset=utf-8",
		".md":  "text/markdown; charset=utf-8",
		".csv": "text/csv; charset=utf-8",

		// 文档文件
		".pdf":  "application/pdf",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".ppt":  "application/vnd.ms-powerpoint",
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",

		// 压缩文件
		".zip": "application/zip",
		".rar": "application/vnd.rar",
		".7z":  "application/x-7z-compressed",
		".tar": "application/x-tar",
		".gz":  "application/gzip",

		// 音频文件
		".mp3":  "audio/mpeg",
		".wav":  "audio/wav",
		".ogg":  "audio/ogg",
		".m4a":  "audio/mp4",
		".flac": "audio/flac",

		// 视频文件
		".mp4":  "video/mp4",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
		".wmv":  "video/x-ms-wmv",
		".webm": "video/webm",
		".mkv":  "video/x-matroska",

		// Web 相关
		".manifest":    "text/cache-manifest",
		".webmanifest": "application/manifest+json",
		".map":         "application/json",
	}
}

// DetectContentType 根据文件名检测内容类型
func (ctd *ContentTypeDetector) DetectContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	// 首先检查自定义MIME类型映射
	if contentType, exists := ctd.mimeTypes[ext]; exists {
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

// SetResponseHeaders 设置HTTP响应头，包括Content-Type
func (ctd *ContentTypeDetector) SetResponseHeaders(w http.ResponseWriter, filename string) {
	contentType := ctd.DetectContentType(filename)
	w.Header().Set("Content-Type", contentType)
}

// SetResponseHeadersWithContentType 设置HTTP响应头，使用指定的Content-Type
func (ctd *ContentTypeDetector) SetResponseHeadersWithContentType(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
}

// AddCustomType 添加自定义MIME类型映射
func (ctd *ContentTypeDetector) AddCustomType(extension, contentType string) {
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	ctd.mimeTypes[strings.ToLower(extension)] = contentType
}

// RemoveCustomType 移除自定义MIME类型映射
func (ctd *ContentTypeDetector) RemoveCustomType(extension string) {
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	delete(ctd.mimeTypes, strings.ToLower(extension))
}

// GetSupportedExtensions 获取所有支持的文件扩展名
func (ctd *ContentTypeDetector) GetSupportedExtensions() []string {
	extensions := make([]string, 0, len(ctd.mimeTypes))
	for ext := range ctd.mimeTypes {
		extensions = append(extensions, ext)
	}
	return extensions
}

// IsTextType 检查给定的内容类型是否为文本类型
func (ctd *ContentTypeDetector) IsTextType(contentType string) bool {
	textTypes := []string{
		"text/",
		"application/javascript",
		"application/json",
		"application/xml",
		"application/xhtml+xml",
		"image/svg+xml",
	}

	contentType = strings.ToLower(contentType)
	for _, textType := range textTypes {
		if strings.HasPrefix(contentType, textType) {
			return true
		}
	}

	return false
}

// IsImageType 检查给定的内容类型是否为图片类型
func (ctd *ContentTypeDetector) IsImageType(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(contentType), "image/")
}

// IsVideoType 检查给定的内容类型是否为视频类型
func (ctd *ContentTypeDetector) IsVideoType(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(contentType), "video/")
}

// IsAudioType 检查给定的内容类型是否为音频类型
func (ctd *ContentTypeDetector) IsAudioType(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(contentType), "audio/")
}

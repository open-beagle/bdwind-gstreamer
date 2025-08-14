package webserver

import (
	"embed"
	"io/fs"
	"net/http"
)

// 嵌入静态文件
//
//go:embed static/*
var staticFiles embed.FS

// GetStaticFS 返回静态文件系统
func GetStaticFS() fs.FS {
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return staticFS
}

// GetStaticFileHandler 返回静态文件处理器
func GetStaticFileHandler() http.Handler {
	return http.FileServer(http.FS(GetStaticFS()))
}

// GetStaticFileContent 获取静态文件内容
func GetStaticFileContent(filename string) ([]byte, error) {
	return staticFiles.ReadFile("static/" + filename)
}

package server

import (
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"graycrow/internal/httpx"
)

// serveStatic 单二进制托管前端：STATIC_DIR 优先（本地开发用磁盘目录），否则用 go:embed 的构建产物。
// 未命中文件时回落到 index.html，交给前端路由处理。
func serveStatic(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") || c.Request.URL.Path == "/healthz" {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "接口不存在: "+c.Request.Method+" "+c.Request.URL.Path)
		return
	}
	static := staticFS()
	if static == nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "前端资源未构建：请执行 make build-web 或设置 STATIC_DIR")
		return
	}
	path := strings.TrimPrefix(c.Request.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if path != "index.html" {
		if file, err := static.Open(path); err == nil {
			file.Close()
			if strings.HasPrefix(path, "assets/") {
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			}
			c.FileFromFS(path, http.FS(static))
			return
		}
	}
	// 直接写字节：FileFromFS 对 /index.html 会 301 到 ./，把单页路由的请求打回根路径
	index, err := fs.ReadFile(static, "index.html")
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "前端资源未构建：请执行 make build-web 或设置 STATIC_DIR")
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", index)
}

func staticFS() fs.FS {
	if dir := strings.TrimSpace(os.Getenv("STATIC_DIR")); dir != "" {
		return os.DirFS(dir)
	}
	return embeddedFS
}

package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS 返回前端构建产物；未构建时只有占位文件，由 STATIC_DIR 或外部静态服务器兜底。
func FS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}

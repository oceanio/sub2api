//go:build embed

// Fork: 嵌入 dist 静态资源的预压缩 + Cache-Control 投递。
// 与 vite-plugin-compression2 配套：构建期生成同名 .br / .gz；
// 这里按客户端 Accept-Encoding 挑预压缩文件直接吐，零 CPU 开销。
//
// 触发顺序见 embed_on.go 的 Middleware()：
//   override 文件 → tryServePrecompressed → 原 fileServer.ServeHTTP（兜底）
//
// 命中预压缩时设 Content-Encoding + Vary + Cache-Control + 原 Content-Type。
// 未命中时（小文件、客户端不支持压缩、SVG/原图等）仍由原 fileServer 处理，
// 但通过 setStaticCacheControl 给浏览器加长缓存。
package web

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// tryServePrecompressed 尝试用预压缩文件响应。命中返回 true，未命中返回 false。
// cleanPath 已去掉前导 /，例：assets/index-xxx.js。
func (s *FrontendServer) tryServePrecompressed(c *gin.Context, cleanPath string) bool {
	if s == nil || s.distFS == nil {
		return false
	}
	// 原文件必须存在（否则就不是合法静态资源，让原 fileServer 走 SPA fallback）。
	if !s.fileExists(cleanPath) {
		return false
	}

	accept := c.GetHeader("Accept-Encoding")
	if accept == "" {
		setStaticCacheControl(c, cleanPath)
		return false
	}

	// Brotli 优先，Gzip 兜底。
	if strings.Contains(accept, "br") {
		if served := serveEncodedFromFS(c, s.distFS, cleanPath, ".br", "br"); served {
			return true
		}
	}
	if strings.Contains(accept, "gzip") {
		if served := serveEncodedFromFS(c, s.distFS, cleanPath, ".gz", "gzip"); served {
			return true
		}
	}

	// 客户端支持压缩但本资源没有预压缩版本（小于阈值或不可压缩格式）：
	// 让原 fileServer 处理，但顺手设上 Cache-Control。
	setStaticCacheControl(c, cleanPath)
	return false
}

// serveEncodedFromFS 从 distFS 读 cleanPath+suffix 文件并以指定 encoding 投递。
func serveEncodedFromFS(c *gin.Context, distFS fs.FS, cleanPath, suffix, encoding string) bool {
	encodedPath := cleanPath + suffix
	f, err := distFS.Open(encodedPath)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return false
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return false
	}

	contentType := resolveStaticContentType(cleanPath)

	h := c.Writer.Header()
	h.Set("Content-Type", contentType)
	h.Set("Content-Encoding", encoding)
	h.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	// Vary 让代理/CDN 按 Accept-Encoding 分别缓存，避免给不支持压缩的客户端误送压缩体。
	h.Set("Vary", "Accept-Encoding")
	setStaticCacheControlHeader(h, cleanPath)

	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(data)
	c.Abort()
	return true
}

// setStaticCacheControl 给当前响应加合适的 Cache-Control。
// 仅供 *静态资源* 调用，index.html 走 no-cache（由 serveIndexHTML 单独管理）。
func setStaticCacheControl(c *gin.Context, cleanPath string) {
	setStaticCacheControlHeader(c.Writer.Header(), cleanPath)
}

// resolveStaticContentType 给 dist 资源选 Content-Type。
// 优先用显式映射保证跨 OS 一致（mime.TypeByExtension 行为依赖 /etc/mime.types），
// 命不中时退回 stdlib，再退回 octet-stream。
func resolveStaticContentType(cleanPath string) string {
	switch strings.ToLower(path.Ext(cleanPath)) {
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".map":
		return "application/json; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	}
	if ct := mime.TypeByExtension(path.Ext(cleanPath)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func setStaticCacheControlHeader(h http.Header, cleanPath string) {
	// /assets/* 都是带 hash 的，永久缓存最优。
	if strings.HasPrefix(cleanPath, "assets/") {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	// logo / favicon 等品牌资源——一天足够，便于更换 logo 后第二天生效。
	switch path.Ext(cleanPath) {
	case ".png", ".svg", ".ico", ".jpg", ".jpeg", ".webp":
		h.Set("Cache-Control", "public, max-age=86400")
		return
	}
	// 其他 dist 静态文件——1 小时兜底。
	h.Set("Cache-Control", "public, max-age=3600")
}

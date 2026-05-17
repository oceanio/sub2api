//go:build embed

package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

const (
	// NonceHTMLPlaceholder is the placeholder for nonce in HTML script tags
	NonceHTMLPlaceholder = "__CSP_NONCE_VALUE__"
)

//go:embed all:dist
var frontendFS embed.FS

// PublicSettingsProvider is an interface to fetch public settings
type PublicSettingsProvider interface {
	GetPublicSettingsForInjection(ctx context.Context) (any, error)
}

// FrontendServer serves the embedded frontend with settings injection
type FrontendServer struct {
	distFS       fs.FS
	embeddedBase []byte // immutable embedded index.html
	fileServer   http.Handler
	cache        *HTMLCache
	settings     PublicSettingsProvider
	overrideDir  string // local file override directory

	baseMu        sync.Mutex
	baseHTML      []byte    // currently active index.html (embedded or override)
	overrideMTime time.Time // mtime of last loaded override index.html (zero if not loaded)
}

// NewFrontendServer creates a new frontend server with settings injection
func NewFrontendServer(settingsProvider PublicSettingsProvider) (*FrontendServer, error) {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return nil, err
	}

	// Read base HTML once
	file, err := distFS.Open("index.html")
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	baseHTML, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	cache := NewHTMLCache()
	cache.SetBaseHTML(baseHTML)

	return &FrontendServer{
		distFS:       distFS,
		embeddedBase: baseHTML,
		fileServer:   http.FileServer(http.FS(distFS)),
		baseHTML:     baseHTML,
		cache:        cache,
		settings:     settingsProvider,
		overrideDir:  filepath.Join("data", "public"),
	}, nil
}

// refreshBaseHTML checks if data/public/index.html exists and is newer than
// the currently loaded copy. When it changes (or appears/disappears) the
// in-memory base HTML and the rendered HTML cache are refreshed so that the
// next request reflects the new content.
//
// Returns the active base HTML to use for rendering this request.
func (s *FrontendServer) refreshBaseHTML() []byte {
	s.baseMu.Lock()
	defer s.baseMu.Unlock()

	overridePath := s.overrideIndexPath()
	if overridePath == "" {
		return s.baseHTML
	}

	info, err := os.Stat(overridePath)
	if err != nil || info.IsDir() {
		// Override removed → fall back to embedded version.
		if !s.overrideMTime.IsZero() {
			s.baseHTML = s.embeddedBase
			s.cache.SetBaseHTML(s.embeddedBase)
			s.cache.Invalidate()
			s.overrideMTime = time.Time{}
		}
		return s.baseHTML
	}

	mtime := info.ModTime()
	if mtime.Equal(s.overrideMTime) {
		return s.baseHTML
	}

	content, readErr := os.ReadFile(overridePath)
	if readErr != nil {
		return s.baseHTML
	}

	s.baseHTML = content
	s.overrideMTime = mtime
	s.cache.SetBaseHTML(content)
	s.cache.Invalidate()
	return s.baseHTML
}

// overrideIndexPath returns the on-disk path of the override index.html,
// or "" if override is disabled.
func (s *FrontendServer) overrideIndexPath() string {
	if s.overrideDir == "" {
		return ""
	}
	return filepath.Join(s.overrideDir, "index.html")
}

// overrideFileExists reports whether the named relative path exists in the
// override directory as a regular file.
func (s *FrontendServer) overrideFileExists(cleanPath string) bool {
	if s.overrideDir == "" {
		return false
	}
	filePath := filepath.Join(s.overrideDir, filepath.Clean("/"+cleanPath))
	info, err := os.Stat(filePath)
	return err == nil && !info.IsDir()
}

// InvalidateCache invalidates the HTML cache (call when settings change)
func (s *FrontendServer) InvalidateCache() {
	if s != nil && s.cache != nil {
		s.cache.Invalidate()
	}
}

// Middleware returns the Gin middleware handler.
//
// Resolution order for a request path:
//  1. API/bypass routes → next handler.
//  2. "/" or "/index.html" → serve injected index.html (from override if present, else embedded).
//  3. data/public/<path> regular file → serve from override (covers files not in dist).
//  4. embedded dist/<path> file → serve from embedded FS.
//  5. otherwise → SPA fallback to injected index.html.
func (s *FrontendServer) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip API routes
		if shouldBypassEmbeddedFrontend(path) {
			c.Next()
			return
		}

		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "" || cleanPath == "index.html" {
			s.serveIndexHTML(c)
			return
		}

		// Override directory takes precedence — covers both replacements of
		// embedded files AND brand-new files (e.g. site-verification files).
		if s.tryServeOverride(c, cleanPath) {
			return
		}

		// Embedded static asset.
		if s.fileExists(cleanPath) {
			s.fileServer.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}

		// SPA fallback.
		s.serveIndexHTML(c)
	}
}

func (s *FrontendServer) fileExists(path string) bool {
	file, err := s.distFS.Open(path)
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}

// tryServeOverride checks if a local override file exists and serves it.
// Files in overrideDir take precedence over embedded files.
//
// index.html is intentionally skipped here — it is served via serveIndexHTML
// so that __APP_CONFIG__, site title and CSP nonce can be injected even when
// the HTML comes from disk.
func (s *FrontendServer) tryServeOverride(c *gin.Context, cleanPath string) bool {
	if s.overrideDir == "" {
		return false
	}
	if cleanPath == "index.html" {
		return false
	}
	filePath := filepath.Join(s.overrideDir, filepath.Clean("/"+cleanPath))
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return false
	}
	c.File(filePath)
	c.Abort()
	return true
}

func (s *FrontendServer) serveIndexHTML(c *gin.Context) {
	// Get nonce from context (generated by SecurityHeaders middleware)
	nonce := middleware.GetNonceFromContext(c)

	// Pick up disk override (if any) before consulting the rendered cache —
	// refreshBaseHTML invalidates the cache when the override file changes.
	baseHTML := s.refreshBaseHTML()

	// Check cache first
	cached := s.cache.Get()
	if cached != nil {
		// Check If-None-Match for 304 response
		if match := c.GetHeader("If-None-Match"); match == cached.ETag {
			c.Status(http.StatusNotModified)
			c.Abort()
			return
		}

		// Replace nonce placeholder with actual nonce before serving
		content := replaceNoncePlaceholder(cached.Content, nonce)

		c.Header("ETag", cached.ETag)
		c.Header("Cache-Control", "no-cache") // Must revalidate
		c.Data(http.StatusOK, "text/html; charset=utf-8", content)
		c.Abort()
		return
	}

	// Cache miss - fetch settings and render
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	settings, err := s.settings.GetPublicSettingsForInjection(ctx)
	if err != nil {
		// Fallback: serve without injection
		c.Data(http.StatusOK, "text/html; charset=utf-8", baseHTML)
		c.Abort()
		return
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		// Fallback: serve without injection
		c.Data(http.StatusOK, "text/html; charset=utf-8", baseHTML)
		c.Abort()
		return
	}

	rendered := s.injectSettingsInto(baseHTML, settingsJSON)
	s.cache.Set(rendered, settingsJSON)

	// Replace nonce placeholder with actual nonce before serving
	content := replaceNoncePlaceholder(rendered, nonce)

	cached = s.cache.Get()
	if cached != nil {
		c.Header("ETag", cached.ETag)
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", content)
	c.Abort()
}

// injectSettings renders the currently active base HTML with the given
// settings. Kept for backward compatibility with existing tests.
func (s *FrontendServer) injectSettings(settingsJSON []byte) []byte {
	s.baseMu.Lock()
	base := s.baseHTML
	s.baseMu.Unlock()
	return s.injectSettingsInto(base, settingsJSON)
}

// injectSettingsInto injects the settings script and site title into the
// provided base HTML. Stateless: callers pass in the base they want rendered.
func (s *FrontendServer) injectSettingsInto(baseHTML, settingsJSON []byte) []byte {
	// Create the script tag to inject with nonce placeholder
	// The placeholder will be replaced with actual nonce at request time
	script := []byte(`<script nonce="` + NonceHTMLPlaceholder + `">window.__APP_CONFIG__=` + string(settingsJSON) + `;</script>`)

	// Inject before </head>
	headClose := []byte("</head>")
	result := bytes.Replace(baseHTML, headClose, append(script, headClose...), 1)

	// Replace <title> with custom site name so the browser tab shows it immediately
	result = injectSiteTitle(result, settingsJSON)

	return result
}

// injectSiteTitle replaces the static <title> in HTML with the configured site name.
// This ensures the browser tab shows the correct title before JS executes.
func injectSiteTitle(html, settingsJSON []byte) []byte {
	var cfg struct {
		SiteName string `json:"site_name"`
	}
	if err := json.Unmarshal(settingsJSON, &cfg); err != nil || cfg.SiteName == "" {
		return html
	}

	// Find and replace the existing <title>...</title>
	titleStart := bytes.Index(html, []byte("<title>"))
	titleEnd := bytes.Index(html, []byte("</title>"))
	if titleStart == -1 || titleEnd == -1 || titleEnd <= titleStart {
		return html
	}

	newTitle := []byte("<title>" + cfg.SiteName + " - AI API Gateway</title>")
	var buf bytes.Buffer
	buf.Write(html[:titleStart])
	buf.Write(newTitle)
	buf.Write(html[titleEnd+len("</title>"):])
	return buf.Bytes()
}

// replaceNoncePlaceholder replaces the nonce placeholder with actual nonce value
func replaceNoncePlaceholder(html []byte, nonce string) []byte {
	return bytes.ReplaceAll(html, []byte(NonceHTMLPlaceholder), []byte(nonce))
}

// ServeEmbeddedFrontend returns a middleware for serving embedded frontend
// This is the legacy function for backward compatibility when no settings provider is available.
//
// Resolution order matches FrontendServer.Middleware (minus the injection step):
//  1. data/public/<path>  (regular file; covers replacements AND new files)
//  2. embedded dist/<path>
//  3. SPA fallback to index.html (override wins over embedded)
func ServeEmbeddedFrontend() gin.HandlerFunc {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		panic("failed to get dist subdirectory: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(distFS))
	overrideDir := filepath.Join("data", "public")

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		if shouldBypassEmbeddedFrontend(path) {
			c.Next()
			return
		}

		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "" || cleanPath == "index.html" {
			serveIndexHTML(c, overrideDir, distFS)
			return
		}

		if tryServeOverrideFile(c, overrideDir, cleanPath) {
			return
		}

		if file, err := distFS.Open(cleanPath); err == nil {
			_ = file.Close()
			fileServer.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}

		serveIndexHTML(c, overrideDir, distFS)
	}
}

// tryServeOverrideFile is a standalone version of tryServeOverride for legacy usage.
// Skips index.html so that serveIndexHTML can handle it consistently.
func tryServeOverrideFile(c *gin.Context, overrideDir, cleanPath string) bool {
	if overrideDir == "" || cleanPath == "index.html" {
		return false
	}
	filePath := filepath.Join(overrideDir, filepath.Clean("/"+cleanPath))
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return false
	}
	c.File(filePath)
	c.Abort()
	return true
}

func shouldBypassEmbeddedFrontend(path string) bool {
	trimmed := strings.TrimSpace(path)
	return strings.HasPrefix(trimmed, "/api/") ||
		strings.HasPrefix(trimmed, "/v1/") ||
		strings.HasPrefix(trimmed, "/v1beta/") ||
		strings.HasPrefix(trimmed, "/backend-api/") ||
		strings.HasPrefix(trimmed, "/antigravity/") ||
		strings.HasPrefix(trimmed, "/setup/") ||
		trimmed == "/health" ||
		trimmed == "/responses" ||
		strings.HasPrefix(trimmed, "/responses/") ||
		strings.HasPrefix(trimmed, "/images/")
}

// serveIndexHTML serves index.html for the legacy middleware. It prefers
// data/public/index.html when present, falling back to the embedded copy.
// No __APP_CONFIG__/title injection is performed — callers wanting injection
// should use FrontendServer.Middleware.
func serveIndexHTML(c *gin.Context, overrideDir string, fsys fs.FS) {
	if overrideDir != "" {
		overridePath := filepath.Join(overrideDir, "index.html")
		if info, err := os.Stat(overridePath); err == nil && !info.IsDir() {
			c.File(overridePath)
			c.Abort()
			return
		}
	}

	file, err := fsys.Open("index.html")
	if err != nil {
		c.String(http.StatusNotFound, "Frontend not found")
		c.Abort()
		return
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to read index.html")
		c.Abort()
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", content)
	c.Abort()
}

func HasEmbeddedFrontend() bool {
	_, err := frontendFS.ReadFile("dist/index.html")
	return err == nil
}

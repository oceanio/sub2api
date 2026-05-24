//go:build embed

package web

import (
	"bytes"
	"os"
	"path/filepath"
	"time"
)

// Fork addition: data/public/ takeover of the embedded frontend. The legacy
// behavior (override single files in data/public) is preserved by upstream's
// embed_on.go. This file adds the "fully replace the SPA shell" capability
// so an ops-shipped index.html and brand-new assets (site-verification
// files, custom static pages) override the embedded bundle without rebuilding.
//
// Helpers live here so upstream-tracking sees embed_on.go's diff focused on
// the constructor + middleware wiring, not the implementation details.

// refreshBaseHTML hot-reloads the override index.html from data/public/.
// Called on every serveIndexHTML request; the mtime check makes the common
// "no change" path a single os.Stat with no allocations beyond the mutex.
// When the override file appears, changes, or disappears, the in-memory base
// HTML and the rendered HTML cache are refreshed so the next request reflects
// the new content.
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
// or "" if override is disabled (overrideDir empty).
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

// injectSettingsInto injects the settings script and site title into the
// provided base HTML. Stateless: callers pass in the base they want rendered.
// The stateful injectSettings (in embed_on.go) delegates here so legacy
// callers/tests keep working while serveIndexHTML uses the explicit version.
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

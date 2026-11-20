package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves static assets and falls back to index.html for client routes.
func spaHandler(static fs.FS) http.Handler {
	files := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(static, p); err != nil {
			// Unknown path: let the client router handle it.
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFileFS(w, r, static, "index.html")
			return
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

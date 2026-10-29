package httpapi

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// WithWeb serves the web build in dir from the same origin as the API:
// its files as they are, and index.html for any other path so the app's
// own routes load. Empty serves the API alone.
func (s *Server) WithWeb(dir string) *Server {
	s.webDir = dir
	return s
}

// web serves the build. /api stays the API's: an unknown API path is a
// 404, never the app. Vite's hashed assets/ never change, so they cache
// for a year; everything else revalidates so a release shows at once.
func (s *Server) web() http.Handler {
	fsys := os.DirFS(s.webDir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		cache := "no-cache"
		if strings.HasPrefix(name, "assets/") {
			cache = "public, max-age=31536000, immutable"
		}
		if name != "" && serveFile(w, r, fsys, name, cache) {
			return
		}
		if strings.HasPrefix(name, "assets/") {
			http.NotFound(w, r)
			return
		}
		if !serveFile(w, r, fsys, "index.html", "no-cache") {
			http.NotFound(w, r)
		}
	})
}

// serveFile writes name when it is a regular file of fsys, and says
// whether it did. fs.FS refuses names that leave its root.
func serveFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, name, cache string) bool {
	f, err := fsys.Open(name)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	rs, seekable := f.(io.ReadSeeker)
	if err != nil || !info.Mode().IsRegular() || !seekable {
		return false
	}
	w.Header().Set("Cache-Control", cache)
	http.ServeContent(w, r, name, info.ModTime(), rs)
	return true
}

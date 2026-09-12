package handlers

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// frontend serves the single-page app: its files as they are, and index.html
// for any other path, so that loading a client-side route such as /profile
// directly works.
func (s *Server) frontend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Dotfiles (the .gitkeep that holds the directory in git) are not served.
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" && !strings.HasPrefix(name, ".") && !strings.Contains(name, "/.") {
		if info, err := fs.Stat(s.Frontend, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				// Vite puts a content hash in every asset's name, so a changed
				// file is a new URL and the old one can be kept forever.
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, s.Frontend, name)
			return
		}
	}

	// A path with an extension asks for a file that isn't there, e.g. an
	// asset from before the last deploy. Answering with the page would only
	// hide that.
	if path.Ext(name) != "" {
		http.NotFound(w, r)
		return
	}
	if _, err := fs.Stat(s.Frontend, "index.html"); err != nil {
		http.Error(w, "frontend not built: see make build", http.StatusNotFound)
		return
	}
	// Revalidate every time, so a deploy reaches open phones on their next load.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.Frontend, "index.html")
}

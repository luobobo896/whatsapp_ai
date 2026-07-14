package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist
var embedded embed.FS

// Handler serves compiled frontend assets and falls back to index.html for
// client-side routes.
func Handler() http.Handler {
	dist, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic("open embedded frontend distribution")
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		panic("read embedded frontend index")
	}
	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		assetPath := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
		if assetPath != "." && assetPath != "" {
			if info, statErr := fs.Stat(dist, assetPath); statErr == nil && !info.IsDir() {
				if strings.HasPrefix(assetPath, "assets/") {
					response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(response, request)
				return
			}
			if strings.HasPrefix(assetPath, "assets/") || path.Ext(assetPath) != "" {
				http.NotFound(response, request)
				return
			}
		}

		response.Header().Set("Cache-Control", "no-cache")
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write(index)
	})
}

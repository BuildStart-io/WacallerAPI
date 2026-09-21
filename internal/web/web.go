package web

import (
	"embed"
	"net/http"
)

//go:embed index.html
var content embed.FS

// Handler returns an http.Handler that serves the embedded web dashboard and documentation.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := content.ReadFile("index.html")
		if err != nil {
			http.Error(w, "dashboard asset not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}

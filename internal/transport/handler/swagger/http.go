package swagger

import (
	"net/http"
	"os"
)

type HTTPHandler struct {
	openapiPath string
	htmlPath    string
}

func NewHTTPHandler(openapiPath, htmlPath string) *HTTPHandler {
	return &HTTPHandler{
		openapiPath: openapiPath,
		htmlPath:    htmlPath,
	}
}

func (h *HTTPHandler) OpenAPI(w http.ResponseWriter, r *http.Request) {
	if h.serveFile(w, r, h.openapiPath, "application/yaml; charset=utf-8") {
		return
	}
	http.Error(w, "openapi file not found", http.StatusNotFound)
}

func (h *HTTPHandler) UI(w http.ResponseWriter, r *http.Request) {
	if h.serveFile(w, r, h.htmlPath, "text/html; charset=utf-8") {
		return
	}
	http.Error(w, "swagger ui not found", http.StatusNotFound)
}

func (h *HTTPHandler) serveFile(w http.ResponseWriter, r *http.Request, filename, contentType string) bool {
	if filename == "" {
		return false
	}
	if _, err := os.Stat(filename); err != nil {
		return false
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeFile(w, r, filename)
	return true
}

package handler

import "net/http"

func NewRouter(links *LinkHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", links.onlyMethod(http.MethodGet, Health))
	mux.HandleFunc("/api/v1/links", links.onlyMethod(http.MethodPost, links.Create))
	mux.HandleFunc("/api/v1/links/{id}", links.onlyMethod(http.MethodGet, links.GetByID))
	mux.HandleFunc("/{id}", links.onlyMethod(http.MethodGet, links.Redirect))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		links.writeError(w, http.StatusNotFound, "route not found")
	})
	return mux
}

func (h *LinkHandler) onlyMethod(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		next(w, r)
	}
}

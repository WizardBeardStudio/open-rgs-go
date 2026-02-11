package server

import (
	"net/http"
)

type SystemHandler struct{}

func (h SystemHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.health)
}

func (h SystemHandler) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

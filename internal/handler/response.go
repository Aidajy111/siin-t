package handler

import (
	"encoding/json"
	"net/http"

	"github.com/Aidajy111/siin.git/internal/dto"
)

func (h *LinkHandler) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Cache-Control", "no-store")
	h.respond(w, status, dto.ErrorResponse{Error: message})
}

func (h *LinkHandler) respond(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error().Err(err).Msg("encode HTTP response")
		status = http.StatusInternalServerError
		data = []byte(`{"error":"internal server error"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(append(data, '\n')); err != nil {
		h.logger.Warn().Err(err).Msg("write HTTP response")
	}
}

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Aidajy111/siin.git/internal/dto"
	"github.com/Aidajy111/siin.git/internal/model"
	"github.com/Aidajy111/siin.git/internal/service"
	"github.com/rs/zerolog"
)

const maxBodyBytes = 1 << 20

type LinkService interface {
	Create(ctx context.Context, originalURL string) (model.Link, error)
	GetByID(ctx context.Context, id string) (model.Link, error)
	ResolveAndIncrement(ctx context.Context, id string) (string, error)
	ShortURL(id string) string
}

type LinkHandler struct {
	service LinkService
	logger  zerolog.Logger
}

func NewLinkHandler(links LinkService, logger zerolog.Logger) *LinkHandler {
	return &LinkHandler{
		service: links,
		logger:  logger.With().Str("component", "link_handler").Logger(),
	}
}

func (h *LinkHandler) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			h.writeError(w, http.StatusRequestEntityTooLarge, "request body must not exceed 1 MiB")
		} else {
			h.writeError(w, http.StatusBadRequest, "could not read request body")
		}
		return
	}

	var request *dto.CreateLinkRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request == nil {
		h.writeError(w, http.StatusBadRequest, "body must be a JSON object with a string url field")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		h.writeError(w, http.StatusBadRequest, "body must contain exactly one JSON object")
		return
	}

	link, err := h.service.Create(r.Context(), request.URL)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	h.respond(w, http.StatusCreated, dto.CreateLinkResponse{
		ID:       link.ID,
		ShortURL: h.service.ShortURL(link.ID),
	})
}

func (h *LinkHandler) Redirect(w http.ResponseWriter, r *http.Request) {
	originalURL, err := h.service.ResolveAndIncrement(r.Context(), r.PathValue("id"))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	// Every visit must reach the service so cached redirects do not bypass counting.
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, originalURL, http.StatusFound)
}

func (h *LinkHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	link, err := h.service.GetByID(r.Context(), r.PathValue("id"))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.respond(w, http.StatusOK, dto.LinkResponse{
		ID:        link.ID,
		URL:       link.URL,
		Clicks:    link.Clicks,
		CreatedAt: link.CreatedAt.UTC(),
	})
}

func (h *LinkHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidURL):
		h.writeError(w, http.StatusBadRequest, service.ErrInvalidURL.Error())
	case errors.Is(err, service.ErrLinkNotFound):
		h.writeError(w, http.StatusNotFound, "link not found")
	default:
		h.logger.Error().Err(err).Str("method", r.Method).Str("route", r.Pattern).Msg("request failed")
		h.writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

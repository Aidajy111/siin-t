package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/Aidajy111/siin.git/internal/model"
	"github.com/Aidajy111/siin.git/internal/repository"
)

const maxIDAttempts = 5

type LinkRepository interface {
	Create(ctx context.Context, id, originalURL string) (model.Link, error)
	GetByID(ctx context.Context, id string) (model.Link, error)
	ResolveAndIncrement(ctx context.Context, id string) (string, error)
}

type LinkService struct {
	repository LinkRepository
	baseURL    string
	generateID func() (string, error)
}

func NewLinkService(repo LinkRepository, baseURL string) *LinkService {
	return &LinkService{
		repository: repo,
		baseURL:    strings.TrimRight(baseURL, "/"),
		generateID: newID,
	}
}

func (s *LinkService) Create(ctx context.Context, originalURL string) (model.Link, error) {
	if !validURL(originalURL) {
		return model.Link{}, ErrInvalidURL
	}
	for attempt := 0; attempt < maxIDAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return model.Link{}, fmt.Errorf("create link: %w", err)
		}
		id, err := s.generateID()
		if err != nil {
			return model.Link{}, fmt.Errorf("generate link ID: %w", err)
		}
		link, err := s.repository.Create(ctx, id, originalURL)
		if errors.Is(err, repository.ErrIDExists) {
			continue
		}
		if err != nil {
			return model.Link{}, fmt.Errorf("create link: %w", err)
		}
		return link, nil
	}
	return model.Link{}, ErrIDGenerationExhausted
}

func (s *LinkService) GetByID(ctx context.Context, id string) (model.Link, error) {
	link, err := s.repository.GetByID(ctx, id)
	if errors.Is(err, repository.ErrLinkNotFound) {
		return model.Link{}, ErrLinkNotFound
	}
	if err != nil {
		return model.Link{}, fmt.Errorf("get link statistics: %w", err)
	}
	return link, nil
}

func (s *LinkService) ResolveAndIncrement(ctx context.Context, id string) (string, error) {
	originalURL, err := s.repository.ResolveAndIncrement(ctx, id)
	if errors.Is(err, repository.ErrLinkNotFound) {
		return "", ErrLinkNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve link: %w", err)
	}
	return originalURL, nil
}

func (s *LinkService) ShortURL(id string) string {
	return s.baseURL + "/" + id
}

func newID() (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(random[:]), nil
}

func validURL(raw string) bool {
	if strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		return err == nil && number > 0 && number <= 65535
	}
	return !strings.HasSuffix(u.Host, ":")
}

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Aidajy111/siin.git/internal/dto"
	"github.com/Aidajy111/siin.git/internal/handler"
	"github.com/Aidajy111/siin.git/internal/model"
	"github.com/Aidajy111/siin.git/internal/repository"
	"github.com/Aidajy111/siin.git/internal/service"
	"github.com/rs/zerolog"
)

type serviceStub struct {
	create  func(context.Context, string) (model.Link, error)
	get     func(context.Context, string) (model.Link, error)
	resolve func(context.Context, string) (string, error)
}

func (s serviceStub) Create(ctx context.Context, originalURL string) (model.Link, error) {
	return s.create(ctx, originalURL)
}
func (s serviceStub) GetByID(ctx context.Context, id string) (model.Link, error) {
	return s.get(ctx, id)
}
func (s serviceStub) ResolveAndIncrement(ctx context.Context, id string) (string, error) {
	return s.resolve(ctx, id)
}
func (s serviceStub) ShortURL(id string) string {
	return "https://short.example/" + id
}

func newRouter(s handler.LinkService) http.Handler {
	return handler.NewRouter(handler.NewLinkHandler(s, zerolog.Nop()))
}

func request(router http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func assertError(t *testing.T, response *httptest.ResponseRecorder, status int) dto.ErrorResponse {
	t.Helper()
	if response.Code != status {
		t.Fatalf("expected HTTP %d, got %d: %s", status, response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatal("error response must be JSON")
	}
	var result dto.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Error == "" {
		t.Fatalf("invalid error response: %s", response.Body.String())
	}
	return result
}

func TestCreate(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	calls := 0
	router := newRouter(serviceStub{create: func(got context.Context, originalURL string) (model.Link, error) {
		calls++
		if got != ctx || originalURL != "https://example.com/path" {
			t.Fatal("request data or context was not passed to service")
		}
		return model.Link{ID: "abc123", URL: originalURL}, nil
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(`{"url":"https://example.com/path"}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)

	var result dto.CreateLinkResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusCreated || response.Header().Get("Content-Type") != "application/json" ||
		result.ID != "abc123" || result.ShortURL != "https://short.example/abc123" || calls != 1 {
		t.Fatalf("unexpected create response: %d %s", response.Code, response.Body.String())
	}
}

func TestInvalidCreateRequests(t *testing.T) {
	for _, body := range []string{
		"", "{", "null", "[]", `"text"`, "{}",
		`{"url":null}`, `{"url":123}`, `{"url":""}`,
		`{"url":"/relative"}`, `{"url":"ftp://example.com"}`,
		`{"url":"https://example.com","unknown":1}`,
		`{"url":"https://example.com"}{}`,
		`{"url":"https://example.com"} trailing`,
	} {
		t.Run(body, func(t *testing.T) {
			repo := &memoryRepository{links: make(map[string]model.Link)}
			router := newRouter(service.NewLinkService(repo, "http://localhost:8080"))
			assertError(t, request(router, http.MethodPost, "/api/v1/links", body), http.StatusBadRequest)
			if len(repo.links) != 0 {
				t.Fatal("invalid request created a link")
			}
		})
	}
}

func TestCreateBodyLimit(t *testing.T) {
	const limit = 1 << 20
	validJSON := `{"url":"https://example.com"}`
	for _, tt := range []struct {
		name          string
		size          int
		contentLength int64
		status        int
	}{
		{"exact limit", limit, limit, http.StatusCreated},
		{"over limit", limit + 1, limit + 1, http.StatusRequestEntityTooLarge},
		{"unknown length over limit", limit + 1, -1, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			router := newRouter(serviceStub{create: func(context.Context, string) (model.Link, error) {
				calls++
				return model.Link{ID: "abc123"}, nil
			}})
			body := validJSON + strings.Repeat(" ", tt.size-len(validJSON))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
			req.ContentLength = tt.contentLength
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if tt.status == http.StatusCreated {
				if response.Code != tt.status || calls != 1 {
					t.Fatalf("body at limit was rejected: HTTP %d, calls=%d", response.Code, calls)
				}
			} else {
				assertError(t, response, tt.status)
				if calls != 0 {
					t.Fatal("oversized body reached the service")
				}
			}
		})
	}
}

func TestRedirect(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	const originalURL = "https://example.com/path?q=1#section"
	calls := 0
	router := newRouter(serviceStub{resolve: func(got context.Context, id string) (string, error) {
		calls++
		if got != ctx || id != "abc123" {
			t.Fatal("context or path ID was not passed to service")
		}
		return originalURL, nil
	}})
	req := httptest.NewRequest(http.MethodGet, "/abc123", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusFound || response.Header().Get("Location") != originalURL || calls != 1 {
		t.Fatalf("unexpected redirect: status=%d, location=%s, calls=%d", response.Code, response.Header().Get("Location"), calls)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("redirect must not be cached")
	}
}

func TestStatistics(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	createdAt := time.Date(2026, 10, 8, 15, 30, 0, 0, time.FixedZone("UTC+3", 3*60*60))
	router := newRouter(serviceStub{get: func(got context.Context, id string) (model.Link, error) {
		if got != ctx || id != "abc123" {
			t.Fatal("context or path ID was not passed to service")
		}
		return model.Link{ID: id, URL: "https://example.com", Clicks: 42, CreatedAt: createdAt}, nil
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/links/abc123", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	var result dto.LinkResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result.ID != "abc123" || result.URL != "https://example.com" ||
		result.Clicks != 42 || !result.CreatedAt.Equal(createdAt) || result.CreatedAt.Location() != time.UTC {
		t.Fatalf("unexpected statistics: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("statistics must not be cached")
	}
}

func TestServiceErrors(t *testing.T) {
	failure := errors.New("database unavailable: private diagnostic")
	for _, tt := range []struct {
		name   string
		method string
		target string
		err    error
		status int
	}{
		{"invalid URL", http.MethodPost, "/api/v1/links", fmt.Errorf("validate: %w", service.ErrInvalidURL), 400},
		{"create failure", http.MethodPost, "/api/v1/links", failure, 500},
		{"ID exhaustion", http.MethodPost, "/api/v1/links", service.ErrIDGenerationExhausted, 500},
		{"statistics missing", http.MethodGet, "/api/v1/links/missing", service.ErrLinkNotFound, 404},
		{"redirect missing", http.MethodGet, "/missing", service.ErrLinkNotFound, 404},
		{"statistics failure", http.MethodGet, "/api/v1/links/abc123", failure, 500},
		{"redirect failure", http.MethodGet, "/abc123", failure, 500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stub := serviceStub{
				create:  func(context.Context, string) (model.Link, error) { return model.Link{}, tt.err },
				get:     func(context.Context, string) (model.Link, error) { return model.Link{}, tt.err },
				resolve: func(context.Context, string) (string, error) { return "", tt.err },
			}
			var logs bytes.Buffer
			router := handler.NewRouter(handler.NewLinkHandler(stub, zerolog.New(&logs)))
			response := request(router, tt.method, tt.target, `{"url":"https://example.com"}`)
			result := assertError(t, response, tt.status)
			if tt.status == 500 {
				if result.Error != "internal server error" || strings.Contains(response.Body.String(), tt.err.Error()) {
					t.Fatal("internal error details were exposed to the client")
				}
				var event map[string]any
				if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
					t.Fatalf("expected a structured error log: %v", err)
				}
				if event["level"] != "error" || event["error"] != tt.err.Error() || event["method"] != tt.method {
					t.Fatalf("incomplete error log: %s", logs.String())
				}
			} else if logs.Len() != 0 {
				t.Fatal("expected client errors must not be logged as internal failures")
			}
		})
	}
}

func TestMethodsAndUnknownRoutes(t *testing.T) {
	router := newRouter(serviceStub{})
	for _, tt := range []struct{ method, target, allow string }{
		{http.MethodGet, "/api/v1/links", http.MethodPost},
		{http.MethodPut, "/api/v1/links", http.MethodPost},
		{http.MethodDelete, "/api/v1/links/abc123", http.MethodGet},
		{http.MethodPost, "/abc123", http.MethodGet},
		{http.MethodHead, "/abc123", http.MethodGet},
		{http.MethodPost, "/health", http.MethodGet},
	} {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			response := request(router, tt.method, tt.target, "")
			assertError(t, response, http.StatusMethodNotAllowed)
			if response.Header().Get("Allow") != tt.allow {
				t.Fatalf("incorrect Allow header: %s", response.Header().Get("Allow"))
			}
		})
	}
	for _, target := range []string{"/", "/api/v1/links/", "/unknown/path"} {
		assertError(t, request(router, http.MethodGet, target, ""), http.StatusNotFound)
	}
	health := request(router, http.MethodGet, "/health", "")
	if health.Code != http.StatusOK || strings.TrimSpace(health.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("health route was not preserved: %d %s", health.Code, health.Body.String())
	}
}

type memoryRepository struct {
	links map[string]model.Link
}

func (r *memoryRepository) Create(_ context.Context, id, originalURL string) (model.Link, error) {
	if _, exists := r.links[id]; exists {
		return model.Link{}, repository.ErrIDExists
	}
	link := model.Link{ID: id, URL: originalURL, CreatedAt: time.Now().UTC()}
	r.links[id] = link
	return link, nil
}
func (r *memoryRepository) GetByID(_ context.Context, id string) (model.Link, error) {
	link, exists := r.links[id]
	if !exists {
		return model.Link{}, repository.ErrLinkNotFound
	}
	return link, nil
}
func (r *memoryRepository) ResolveAndIncrement(ctx context.Context, id string) (string, error) {
	link, err := r.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	link.Clicks++
	r.links[id] = link
	return link.URL, nil
}

func TestLinkLifecycle(t *testing.T) {
	repo := &memoryRepository{links: make(map[string]model.Link)}
	router := newRouter(service.NewLinkService(repo, "https://short.example"))
	var created []dto.CreateLinkResponse
	for i := 0; i < 2; i++ {
		response := request(router, http.MethodPost, "/api/v1/links", `{"url":"https://example.com/long/path"}`)
		if response.Code != http.StatusCreated {
			t.Fatalf("create failed: %s", response.Body.String())
		}
		var link dto.CreateLinkResponse
		if err := json.Unmarshal(response.Body.Bytes(), &link); err != nil {
			t.Fatal(err)
		}
		if link.ShortURL != "https://short.example/"+link.ID || len(link.ID) != 11 {
			t.Fatalf("invalid short URL: %+v", link)
		}
		created = append(created, link)
	}
	if created[0].ID == created[1].ID {
		t.Fatal("repeated original URL must produce a new link")
	}
	id := created[0].ID
	for i := 0; i < 3; i++ {
		redirect := request(router, http.MethodGet, "/"+id, "")
		if redirect.Code != http.StatusFound || redirect.Header().Get("Location") != "https://example.com/long/path" {
			t.Fatalf("redirect failed: %d %s", redirect.Code, redirect.Body.String())
		}
	}
	for i := 0; i < 2; i++ {
		response := request(router, http.MethodGet, "/api/v1/links/"+id, "")
		var stats dto.LinkResponse
		if err := json.Unmarshal(response.Body.Bytes(), &stats); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || stats.Clicks != 3 {
			t.Fatalf("statistics changed the counter: %d %s", response.Code, response.Body.String())
		}
	}
	if repo.links[created[1].ID].Clicks != 0 {
		t.Fatal("redirect changed another link's counter")
	}
	assertError(t, request(router, http.MethodGet, "/unknown", ""), http.StatusNotFound)
}

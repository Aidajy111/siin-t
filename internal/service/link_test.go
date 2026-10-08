package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Aidajy111/siin.git/internal/model"
	"github.com/Aidajy111/siin.git/internal/repository"
)

type repositoryStub struct {
	create  func(context.Context, string, string) (model.Link, error)
	get     func(context.Context, string) (model.Link, error)
	resolve func(context.Context, string) (string, error)
}

func (r repositoryStub) Create(ctx context.Context, id, originalURL string) (model.Link, error) {
	return r.create(ctx, id, originalURL)
}

func (r repositoryStub) GetByID(ctx context.Context, id string) (model.Link, error) {
	return r.get(ctx, id)
}

func (r repositoryStub) ResolveAndIncrement(ctx context.Context, id string) (string, error) {
	return r.resolve(ctx, id)
}

func TestCreateURLValidation(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		valid bool
	}{
		{"https", "https://example.com/path?q=1#section", true},
		{"http", "http://example.com", true},
		{"encoded path", "https://example.com/some%20path", true},
		{"localhost", "http://localhost:8080/path", true},
		{"IPv6", "http://[::1]:8080/path", true},
		{"empty", "", false},
		{"relative", "/path", false},
		{"missing scheme", "example.com/path", false},
		{"scheme relative", "//example.com/path", false},
		{"ftp", "ftp://example.com/file", false},
		{"javascript", "javascript:alert(1)", false},
		{"missing host", "https:///path", false},
		{"missing hostname", "http://:8080", false},
		{"leading whitespace", " https://example.com", false},
		{"path whitespace", "https://example.com/some path", false},
		{"line break", "https://example.com/\r\nLocation: bad", false},
		{"bad escape", "https://example.com/%zz", false},
		{"invalid port", "https://example.com:invalid", false},
		{"zero port", "https://example.com:0", false},
		{"large port", "https://example.com:65536", false},
		{"empty port", "https://example.com:", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			repo := repositoryStub{create: func(ctx context.Context, id, originalURL string) (model.Link, error) {
				calls++
				if originalURL != tt.url {
					t.Fatal("original URL was changed")
				}
				return model.Link{ID: id, URL: originalURL}, nil
			}}
			s := NewLinkService(repo, "https://short.example")
			s.generateID = func() (string, error) { return "generatedID", nil }
			link, err := s.Create(context.Background(), tt.url)
			if tt.valid {
				if err != nil || calls != 1 || link.URL != tt.url {
					t.Fatalf("valid URL rejected or not saved: %v", err)
				}
			} else if !errors.Is(err, ErrInvalidURL) || calls != 0 {
				t.Fatalf("invalid URL must be rejected before saving: calls=%d, error=%v", calls, err)
			}
		})
	}
}

func TestCreateRetriesIDCollisions(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	createdAt := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	calls := 0
	s := NewLinkService(repositoryStub{create: func(got context.Context, id, originalURL string) (model.Link, error) {
		calls++
		if got != ctx || id != fmt.Sprintf("id-%d", calls) {
			t.Fatal("context or generated ID was not passed to the repository")
		}
		if calls < 3 {
			return model.Link{}, fmt.Errorf("insert: %w", repository.ErrIDExists)
		}
		return model.Link{ID: id, URL: originalURL, CreatedAt: createdAt}, nil
	}}, "https://short.example/")
	generated := 0
	s.generateID = func() (string, error) {
		generated++
		return fmt.Sprintf("id-%d", generated), nil
	}
	link, err := s.Create(ctx, "https://example.com")
	if err != nil || calls != 3 || generated != 3 || !link.CreatedAt.Equal(createdAt) {
		t.Fatalf("collision retry failed: calls=%d, generated=%d, error=%v", calls, generated, err)
	}
	if got := s.ShortURL(link.ID); got != "https://short.example/id-3" {
		t.Fatalf("unexpected short URL: %s", got)
	}
}

func TestCreateStopsAfterFiveCollisions(t *testing.T) {
	calls := 0
	s := NewLinkService(repositoryStub{create: func(context.Context, string, string) (model.Link, error) {
		calls++
		return model.Link{}, repository.ErrIDExists
	}}, "http://localhost:8080")
	s.generateID = func() (string, error) { return "collision", nil }
	_, err := s.Create(context.Background(), "https://example.com")
	if !errors.Is(err, ErrIDGenerationExhausted) || calls != 5 {
		t.Fatalf("expected five attempts and ErrIDGenerationExhausted, calls=%d, error=%v", calls, err)
	}
}

func TestCreateDoesNotRetryOtherFailures(t *testing.T) {
	failure := errors.New("database unavailable")
	calls := 0
	s := NewLinkService(repositoryStub{create: func(context.Context, string, string) (model.Link, error) {
		calls++
		return model.Link{}, failure
	}}, "http://localhost:8080")
	_, err := s.Create(context.Background(), "https://example.com")
	if !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("unexpected retry: calls=%d, error=%v", calls, err)
	}
}

func TestCreateGeneratorFailure(t *testing.T) {
	failure := errors.New("random source unavailable")
	s := NewLinkService(repositoryStub{}, "http://localhost:8080")
	s.generateID = func() (string, error) { return "", failure }
	_, err := s.Create(context.Background(), "https://example.com")
	if !errors.Is(err, failure) {
		t.Fatalf("expected generator error, got %v", err)
	}
}

func TestCreateCancellation(t *testing.T) {
	t.Run("before generation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s := NewLinkService(repositoryStub{}, "http://localhost:8080")
		s.generateID = func() (string, error) {
			t.Fatal("generation must not run after cancellation")
			return "", nil
		}
		_, err := s.Create(ctx, "https://example.com")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	})
	t.Run("between retries", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		s := NewLinkService(repositoryStub{create: func(context.Context, string, string) (model.Link, error) {
			calls++
			cancel()
			return model.Link{}, repository.ErrIDExists
		}}, "http://localhost:8080")
		_, err := s.Create(ctx, "https://example.com")
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("retry continued after cancellation: calls=%d, error=%v", calls, err)
		}
	})
}

func TestGetByIDDoesNotIncrementClicks(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	want := model.Link{ID: "abc123", URL: "https://example.com", Clicks: 42}
	s := NewLinkService(repositoryStub{get: func(got context.Context, id string) (model.Link, error) {
		if got != ctx || id != want.ID {
			t.Fatal("context or ID was not passed to the repository")
		}
		return want, nil
	}}, "http://localhost:8080")
	got, err := s.GetByID(ctx, want.ID)
	if err != nil || got != want {
		t.Fatalf("unexpected statistics: %+v, %v", got, err)
	}
}

func TestResolveCallsRepositoryOnce(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	calls := 0
	s := NewLinkService(repositoryStub{resolve: func(got context.Context, id string) (string, error) {
		calls++
		if got != ctx || id != "abc123" {
			t.Fatal("context or ID was not passed to the repository")
		}
		return "https://example.com", nil
	}}, "http://localhost:8080")
	got, err := s.ResolveAndIncrement(ctx, "abc123")
	if err != nil || got != "https://example.com" || calls != 1 {
		t.Fatalf("unexpected resolution: URL=%s, calls=%d, error=%v", got, calls, err)
	}
}

func TestLookupErrors(t *testing.T) {
	failure := errors.New("database unavailable")
	for _, tt := range []struct {
		name  string
		input error
		want  error
	}{
		{"not found", fmt.Errorf("lookup: %w", repository.ErrLinkNotFound), ErrLinkNotFound},
		{"database failure", failure, failure},
		{"cancelled", context.Canceled, context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := NewLinkService(repositoryStub{
				get:     func(context.Context, string) (model.Link, error) { return model.Link{}, tt.input },
				resolve: func(context.Context, string) (string, error) { return "", tt.input },
			}, "http://localhost:8080")
			if _, err := s.GetByID(context.Background(), "abc123"); !errors.Is(err, tt.want) {
				t.Fatalf("unexpected statistics error: %v", err)
			}
			if _, err := s.ResolveAndIncrement(context.Background(), "abc123"); !errors.Is(err, tt.want) {
				t.Fatalf("unexpected redirect error: %v", err)
			}
		})
	}
}

func TestNewIDFormat(t *testing.T) {
	id, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil || len(decoded) != 8 || len(id) != 11 || strings.ContainsAny(id, "+/=") {
		t.Fatalf("expected 8 random bytes as unpadded URL-safe Base64, got %q", id)
	}
}

//go:build integration

package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Aidajy111/siin.git/internal/dto"
	"github.com/Aidajy111/siin.git/internal/repository/postgres"
	"github.com/Aidajy111/siin.git/internal/service"
	"github.com/Aidajy111/siin.git/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

const integrationBaseURL = "https://short.example"

func TestLinkAPIIntegrationLifecycle(t *testing.T) {
	db := testutil.NewDatabase(t)
	ctx := apiTestContext(t)
	router := postgresRouter(db.Pool)
	const originalURL = "https://example.com/long/path?q=one%20two#section"

	created := createAPILink(t, ctx, router, originalURL)
	stats := getAPIStats(t, ctx, router, created.ID)
	if stats.ID != created.ID || stats.URL != originalURL || stats.Clicks != 0 || stats.CreatedAt.IsZero() {
		t.Fatalf("unexpected initial statistics: %+v", stats)
	}

	response := apiRequest(ctx, router, http.MethodGet, "/"+created.ID, "")
	if response.Code != http.StatusFound || response.Header().Get("Location") != originalURL {
		t.Fatalf("unexpected redirect: HTTP %d, Location=%q", response.Code, response.Header().Get("Location"))
	}
	for i := 0; i < 2; i++ {
		current := getAPIStats(t, ctx, router, created.ID)
		if current.ID != stats.ID || current.URL != stats.URL ||
			current.Clicks != 1 || !current.CreatedAt.Equal(stats.CreatedAt) {
			t.Fatalf("unexpected statistics after redirect: %+v", current)
		}
	}
	assertDatabaseState(t, ctx, db.Pool, 1, 1)

	// Rebuild the complete API stack with new connections to the same schema.
	db.Pool.Close()
	freshPool := db.NewPool(t)
	freshRouter := postgresRouter(freshPool)
	persisted := getAPIStats(t, ctx, freshRouter, created.ID)
	if persisted.ID != stats.ID || persisted.URL != stats.URL ||
		persisted.Clicks != 1 || !persisted.CreatedAt.Equal(stats.CreatedAt) {
		t.Fatalf("link was not persisted across pool recreation: %+v", persisted)
	}
	assertDatabaseState(t, ctx, freshPool, 1, 1)
}

func TestLinkAPIIntegrationConcurrentRedirects(t *testing.T) {
	db := testutil.NewDatabase(t)
	ctx := apiTestContext(t)
	router := postgresRouter(db.Pool)
	const originalURL = "https://example.com/concurrent"
	created := createAPILink(t, ctx, router, originalURL)

	const redirects = 100
	const reads = 100
	responses := parallelAPIRequests(redirects+reads, func(index int) *httptest.ResponseRecorder {
		if index < redirects {
			return apiRequest(ctx, router, http.MethodGet, "/"+created.ID, "")
		}
		return apiRequest(ctx, router, http.MethodGet, "/api/v1/links/"+created.ID, "")
	})
	for index, response := range responses {
		if index < redirects {
			if response.Code != http.StatusFound || response.Header().Get("Location") != originalURL {
				t.Errorf("redirect %d: HTTP %d, Location=%q", index, response.Code, response.Header().Get("Location"))
			}
			continue
		}
		stats := decodeAPIResponse[dto.LinkResponse](t, response, http.StatusOK)
		if stats.ID != created.ID || stats.URL != originalURL || stats.Clicks < 0 || stats.Clicks > redirects {
			t.Errorf("concurrent statistics returned invalid data: %+v", stats)
		}
	}

	final := getAPIStats(t, ctx, router, created.ID)
	if final.Clicks != redirects {
		t.Fatalf("expected %d clicks after concurrent requests, got %d", redirects, final.Clicks)
	}
	assertDatabaseState(t, ctx, db.Pool, 1, redirects)

	again := getAPIStats(t, ctx, router, created.ID)
	if again.Clicks != redirects {
		t.Fatalf("reading statistics changed the counter to %d", again.Clicks)
	}
	assertDatabaseState(t, ctx, db.Pool, 1, redirects)
}

func TestLinkAPIIntegrationConcurrentCreation(t *testing.T) {
	db := testutil.NewDatabase(t)
	ctx := apiTestContext(t)
	router := postgresRouter(db.Pool)
	const originalURL = "https://example.com/same-original"
	const requests = 50
	body := `{"url":"https://example.com/same-original"}`

	responses := parallelAPIRequests(requests, func(int) *httptest.ResponseRecorder {
		return apiRequest(ctx, router, http.MethodPost, "/api/v1/links", body)
	})
	ids := make(map[string]bool, requests)
	for _, response := range responses {
		link := decodeAPIResponse[dto.CreateLinkResponse](t, response, http.StatusCreated)
		assertShortURL(t, link)
		if ids[link.ID] {
			t.Fatalf("concurrent creation returned duplicate ID %q", link.ID)
		}
		ids[link.ID] = true
	}

	rows, err := db.Pool.Query(ctx, "SELECT id, url, clicks, created_at FROM links")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	stored := 0
	for rows.Next() {
		var id, original string
		var clicks int64
		var createdAt time.Time
		if err := rows.Scan(&id, &original, &clicks, &createdAt); err != nil {
			t.Fatal(err)
		}
		if !ids[id] || original != originalURL || clicks != 0 || createdAt.IsZero() {
			t.Fatalf("unexpected database row: id=%q, url=%q, clicks=%d", id, original, clicks)
		}
		delete(ids, id)
		stored++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if stored != requests || len(ids) != 0 {
		t.Fatalf("expected %d stored links, found %d; missing IDs: %v", requests, stored, ids)
	}
}

func TestLinkAPIIntegrationRejectedRequests(t *testing.T) {
	db := testutil.NewDatabase(t)
	ctx := apiTestContext(t)
	router := postgresRouter(db.Pool)
	created := createAPILink(t, ctx, router, "https://example.com/unchanged")
	before := getAPIStats(t, ctx, router, created.ID)
	const validBody = `{"url":"https://example.com/new"}`

	tests := []struct {
		name   string
		method string
		target string
		body   string
		status int
		allow  string
	}{
		{"invalid URL", http.MethodPost, "/api/v1/links", `{"url":"/relative"}`, 400, ""},
		{"missing URL", http.MethodPost, "/api/v1/links", "{}", 400, ""},
		{"malformed JSON", http.MethodPost, "/api/v1/links", `{"url":`, 400, ""},
		{"null", http.MethodPost, "/api/v1/links", "null", 400, ""},
		{"trailing object", http.MethodPost, "/api/v1/links", validBody + "{}", 400, ""},
		{"oversized", http.MethodPost, "/api/v1/links", validBody + strings.Repeat(" ", 1<<20), 413, ""},
		{"unknown redirect", http.MethodGet, "/missing", "", 404, ""},
		{"unknown statistics", http.MethodGet, "/api/v1/links/missing", "", 404, ""},
		{"unsupported create method", http.MethodGet, "/api/v1/links", "", 405, http.MethodPost},
		{"unsupported redirect method", http.MethodPost, "/" + created.ID, validBody, 405, http.MethodGet},
		{"unsupported statistics method", http.MethodDelete, "/api/v1/links/" + created.ID, "", 405, http.MethodGet},
		{"HEAD does not count", http.MethodHead, "/" + created.ID, "", 405, http.MethodGet},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := apiRequest(ctx, router, tt.method, tt.target, tt.body)
			assertError(t, response, tt.status)
			if response.Header().Get("Allow") != tt.allow {
				t.Fatalf("unexpected Allow header: %q", response.Header().Get("Allow"))
			}
			assertDatabaseState(t, ctx, db.Pool, 1, 0)
			after := getAPIStats(t, ctx, router, created.ID)
			if after.ID != before.ID || after.URL != before.URL ||
				after.Clicks != before.Clicks || !after.CreatedAt.Equal(before.CreatedAt) {
				t.Fatalf("rejected request changed an existing link: %+v", after)
			}
			assertDatabaseState(t, ctx, db.Pool, 1, 0)
		})
	}
}

func postgresRouter(pool *pgxpool.Pool) http.Handler {
	repo := postgres.NewLinkRepository(pool)
	links := service.NewLinkService(repo, integrationBaseURL)
	return newRouter(links)
}

func apiTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func apiRequest(ctx context.Context, router http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func parallelAPIRequests(count int, run func(int) *httptest.ResponseRecorder) []*httptest.ResponseRecorder {
	responses := make([]*httptest.ResponseRecorder, count)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			<-start
			responses[index] = run(index)
		}(i)
	}
	close(start)
	workers.Wait()
	return responses
}

func createAPILink(t *testing.T, ctx context.Context, router http.Handler, originalURL string) dto.CreateLinkResponse {
	t.Helper()
	body, err := json.Marshal(dto.CreateLinkRequest{URL: originalURL})
	if err != nil {
		t.Fatal(err)
	}
	response := apiRequest(ctx, router, http.MethodPost, "/api/v1/links", string(body))
	link := decodeAPIResponse[dto.CreateLinkResponse](t, response, http.StatusCreated)
	assertShortURL(t, link)
	return link
}

func assertShortURL(t *testing.T, link dto.CreateLinkResponse) {
	t.Helper()
	if len(link.ID) != 11 || strings.ContainsAny(link.ID, "/+=") ||
		link.ShortURL != integrationBaseURL+"/"+link.ID {
		t.Fatalf("unexpected short link: %+v", link)
	}
}

func getAPIStats(t *testing.T, ctx context.Context, router http.Handler, id string) dto.LinkResponse {
	t.Helper()
	response := apiRequest(ctx, router, http.MethodGet, "/api/v1/links/"+id, "")
	return decodeAPIResponse[dto.LinkResponse](t, response, http.StatusOK)
}

func decodeAPIResponse[T any](t *testing.T, response *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if response.Code != status {
		t.Fatalf("expected HTTP %d, got %d: %s", status, response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatal("API response must be JSON")
	}
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode API response: %v", err)
	}
	return value
}

func assertDatabaseState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, expectedRows, expectedClicks int64) {
	t.Helper()
	var rows, clicks int64
	err := pool.QueryRow(ctx, "SELECT count(*), COALESCE(sum(clicks), 0)::bigint FROM links").Scan(&rows, &clicks)
	if err != nil {
		t.Fatal(err)
	}
	if rows != expectedRows || clicks != expectedClicks {
		t.Fatalf("unexpected database state: rows=%d, clicks=%d; want rows=%d, clicks=%d",
			rows, clicks, expectedRows, expectedClicks)
	}
}

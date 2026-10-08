//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Aidajy111/siin.git/internal/repository"
	"github.com/Aidajy111/siin.git/internal/repository/postgres"
	"github.com/Aidajy111/siin.git/internal/testutil"
)

func TestLinkRepository(t *testing.T) {
	pool := testutil.NewDatabase(t).Pool
	repo := postgres.NewLinkRepository(pool)

	t.Run("create and read", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		const id = "id'--"
		const originalURL = "https://example.com/path?q='value'&page=1"
		created, err := repo.Create(ctx, id, originalURL)
		if err != nil {
			t.Fatal(err)
		}
		if created.ID != id || created.URL != originalURL || created.Clicks != 0 || created.CreatedAt.IsZero() {
			t.Fatalf("unexpected created link: %+v", created)
		}
		found, err := repo.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if found.ID != created.ID || found.URL != created.URL ||
			found.Clicks != created.Clicks || !found.CreatedAt.Equal(created.CreatedAt) {
			t.Fatalf("stored link differs from created link: %+v", found)
		}
	})

	t.Run("duplicate ID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		const originalURL = "https://example.com/first"
		if _, err := repo.Create(ctx, "duplicate", originalURL); err != nil {
			t.Fatal(err)
		}
		_, err := repo.Create(ctx, "duplicate", "https://example.com/second")
		if !errors.Is(err, repository.ErrIDExists) {
			t.Fatalf("expected ErrIDExists, got %v", err)
		}
		link, err := repo.GetByID(ctx, "duplicate")
		if err != nil || link.URL != originalURL || link.Clicks != 0 {
			t.Fatalf("duplicate insert changed the original link: %v", err)
		}
	})

	t.Run("unknown ID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if _, err := repo.GetByID(ctx, "missing"); !errors.Is(err, repository.ErrLinkNotFound) {
			t.Fatalf("expected ErrLinkNotFound from GetByID, got %v", err)
		}
		if _, err := repo.ResolveAndIncrement(ctx, "missing"); !errors.Is(err, repository.ErrLinkNotFound) {
			t.Fatalf("expected ErrLinkNotFound from ResolveAndIncrement, got %v", err)
		}
	})

	t.Run("concurrent clicks", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		const originalURL = "https://example.com/concurrent"
		if _, err := repo.Create(ctx, "concurrent", originalURL); err != nil {
			t.Fatal(err)
		}

		const requests = 100
		start := make(chan struct{})
		errs := make(chan error, requests)
		var workers sync.WaitGroup
		for i := 0; i < requests; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-start
				resolved, err := repo.ResolveAndIncrement(ctx, "concurrent")
				if err != nil {
					errs <- err
				} else if resolved != originalURL {
					errs <- fmt.Errorf("unexpected resolved URL: %s", resolved)
				}
			}()
		}
		close(start)
		workers.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}

		link, err := repo.GetByID(ctx, "concurrent")
		if err != nil {
			t.Fatal(err)
		}
		if link.Clicks != requests {
			t.Fatalf("expected %d clicks, got %d", requests, link.Clicks)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if _, err := repo.Create(ctx, "cancelled", "https://example.com"); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation from Create, got %v", err)
		}
		if _, err := repo.GetByID(ctx, "cancelled"); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation from GetByID, got %v", err)
		}
		if _, err := repo.ResolveAndIncrement(ctx, "cancelled"); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation from ResolveAndIncrement, got %v", err)
		}
	})
}

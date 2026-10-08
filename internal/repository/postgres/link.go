package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/Aidajy111/siin.git/internal/model"
	"github.com/Aidajy111/siin.git/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LinkRepository struct {
	pool *pgxpool.Pool
}

func NewLinkRepository(pool *pgxpool.Pool) *LinkRepository {
	return &LinkRepository{pool: pool}
}

func (r *LinkRepository) Create(ctx context.Context, id, originalURL string) (model.Link, error) {
	const query = `
		INSERT INTO links (id, url)
		VALUES ($1, $2)
		RETURNING id, url, clicks, created_at`

	link, err := scanLink(r.pool.QueryRow(ctx, query, id, originalURL))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "links_pkey" {
			return model.Link{}, repository.ErrIDExists
		}
		return model.Link{}, fmt.Errorf("create link: %w", err)
	}
	return link, nil
}

func (r *LinkRepository) GetByID(ctx context.Context, id string) (model.Link, error) {
	const query = `
		SELECT id, url, clicks, created_at
		FROM links
		WHERE id = $1`

	link, err := scanLink(r.pool.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Link{}, repository.ErrLinkNotFound
	}
	if err != nil {
		return model.Link{}, fmt.Errorf("get link: %w", err)
	}
	return link, nil
}

func (r *LinkRepository) ResolveAndIncrement(ctx context.Context, id string) (string, error) {
	const query = `
		UPDATE links
		SET clicks = clicks + 1
		WHERE id = $1
		RETURNING url`

	var originalURL string
	err := r.pool.QueryRow(ctx, query, id).Scan(&originalURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", repository.ErrLinkNotFound
	}
	if err != nil {
		return "", fmt.Errorf("increment link clicks: %w", err)
	}
	return originalURL, nil
}

func scanLink(row pgx.Row) (model.Link, error) {
	var link model.Link
	err := row.Scan(&link.ID, &link.URL, &link.Clicks, &link.CreatedAt)
	return link, err
}

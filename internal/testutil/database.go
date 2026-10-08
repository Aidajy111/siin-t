//go:build integration

package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/Aidajy111/siin.git/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Database struct {
	Pool   *pgxpool.Pool
	config *pgxpool.Config
}

// NewDatabase creates an isolated schema and applies the project's migrations.
// Cleanup closes every test pool before dropping only this schema.
func NewDatabase(t *testing.T) *Database {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests; point it to a running PostgreSQL database")
	}
	baseConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid TEST_DATABASE_URL")
	}
	baseConfig.ConnConfig.ConnectTimeout = 5 * time.Second
	baseConfig.MinConns = 0
	baseConfig.MaxConns = 1

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	adminPool, err := pgxpool.NewWithConfig(ctx, baseConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adminPool.Close)

	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	schema := "siin_test_" + hex.EncodeToString(random[:])
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := adminPool.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("remove test schema: %v", err)
		}
	})

	testConfig := baseConfig.Copy()
	testConfig.ConnConfig.RuntimeParams["search_path"] = schema
	testConfig.MaxConns = 10
	db := &Database{config: testConfig}
	db.Pool = db.NewPool(t)

	names, err := fs.Glob(migrations.Files, "*.up.sql")
	if err != nil || len(names) == 0 {
		t.Fatal("no SQL migrations found")
	}
	for _, name := range names {
		sql, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	return db
}

// NewPool opens another pool in the same schema without reapplying migrations.
// Register it on the test that owns the Database or on one of its subtests.
func (db *Database) NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, db.config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	return pool
}

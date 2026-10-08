package database

import (
	"strings"
	"testing"
	"time"

	"github.com/Aidajy111/siin.git/internal/config"
)

func TestPoolConfigPreservesCredentials(t *testing.T) {
	cfg := config.PostgresConfig{
		Host:           "::1",
		Port:           6543,
		User:           "user@name",
		Password:       "p@ss:/?#%+$'\\ word",
		Database:       "links/test db",
		SSLMode:        "disable",
		ConnectTimeout: 3 * time.Second,
		MaxConns:       7,
	}
	poolConfig, err := newPoolConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	conn := poolConfig.ConnConfig
	if conn.Host != cfg.Host || conn.Port != cfg.Port || conn.User != cfg.User ||
		conn.Password != cfg.Password || conn.Database != cfg.Database {
		t.Fatal("connection URL did not preserve its fields")
	}
	if poolConfig.MaxConns != cfg.MaxConns || conn.ConnectTimeout != cfg.ConnectTimeout {
		t.Fatal("pool limits were not applied")
	}
}

func TestPoolConfigDoesNotExposePassword(t *testing.T) {
	cfg := config.PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "siin",
		Password: "private-test-password",
		Database: "siin",
		SSLMode:  "invalid",
	}
	_, err := newPoolConfig(cfg)
	if err == nil {
		t.Fatal("expected an invalid SSL mode error")
	}
	if strings.Contains(err.Error(), cfg.Password) || strings.Contains(err.Error(), "postgres://") {
		t.Fatal("connection error exposed credentials")
	}
}

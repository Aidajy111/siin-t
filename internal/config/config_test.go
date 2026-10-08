package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Aidajy111/siin.git/internal/config"
)

func prepareEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"HTTP_ADDR", "HTTP_SHUTDOWN_TIMEOUT", "BASE_URL",
		"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_USER", "POSTGRES_PASSWORD",
		"POSTGRES_DB", "POSTGRES_SSLMODE", "POSTGRES_CONNECT_TIMEOUT", "POSTGRES_MAX_CONNS",
	} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("POSTGRES_PASSWORD", "test-password")
}

func TestLoadDefaults(t *testing.T) {
	prepareEnv(t)
	settings, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	httpConfig, ok := settings.Get(config.HTTPKey).(config.HTTPConfig)
	if !ok || httpConfig.Addr != ":8080" || httpConfig.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected HTTP configuration: %+v", httpConfig)
	}
	appConfig, ok := settings.Get(config.AppKey).(config.AppConfig)
	if !ok || appConfig.BaseURL != "http://localhost:8080" {
		t.Fatalf("unexpected application configuration: %+v", appConfig)
	}
	pg, ok := settings.Get(config.PostgresKey).(config.PostgresConfig)
	if !ok || pg.Host != "localhost" || pg.Port != 5432 || pg.User != "siin" ||
		pg.Database != "siin" || pg.SSLMode != "disable" || pg.MaxConns != 10 ||
		pg.ConnectTimeout != 5*time.Second || pg.Password != "test-password" {
		t.Fatal("unexpected PostgreSQL configuration")
	}
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	prepareEnv(t)
	for key, value := range map[string]string{
		"HTTP_ADDR":                "127.0.0.1:9090",
		"HTTP_SHUTDOWN_TIMEOUT":    "12s",
		"BASE_URL":                 "https://short.example/",
		"POSTGRES_HOST":            "db.example",
		"POSTGRES_PORT":            "6543",
		"POSTGRES_USER":            "links-user",
		"POSTGRES_PASSWORD":        "password@with:/?#% special characters",
		"POSTGRES_DB":              "links",
		"POSTGRES_SSLMODE":         "require",
		"POSTGRES_CONNECT_TIMEOUT": "2s",
		"POSTGRES_MAX_CONNS":       "20",
	} {
		t.Setenv(key, value)
	}
	settings, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	httpConfig := settings.Get(config.HTTPKey).(config.HTTPConfig)
	if httpConfig.Addr != "127.0.0.1:9090" || httpConfig.ShutdownTimeout != 12*time.Second {
		t.Fatalf("unexpected HTTP configuration: %+v", httpConfig)
	}
	if settings.Get(config.AppKey).(config.AppConfig).BaseURL != "https://short.example" {
		t.Fatal("BASE_URL was not normalized")
	}
	pg := settings.Get(config.PostgresKey).(config.PostgresConfig)
	if pg.Host != "db.example" || pg.Port != 6543 || pg.User != "links-user" ||
		pg.Database != "links" || pg.SSLMode != "require" || pg.MaxConns != 20 ||
		pg.ConnectTimeout != 2*time.Second || pg.Password != os.Getenv("POSTGRES_PASSWORD") {
		t.Fatal("environment overrides were not applied")
	}

	t.Setenv("POSTGRES_PASSWORD", "changed-password")
	other, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings == other || settings.Get(config.PostgresKey).(config.PostgresConfig).Password == "changed-password" {
		t.Fatal("configuration instances must be independent snapshots")
	}
}

func TestLoadRejectsInvalidEnvironment(t *testing.T) {
	tests := []struct{ key, value string }{
		{"HTTP_ADDR", ""},
		{"HTTP_ADDR", "8080"},
		{"HTTP_ADDR", ":0"},
		{"HTTP_ADDR", ":65536"},
		{"HTTP_SHUTDOWN_TIMEOUT", "invalid"},
		{"HTTP_SHUTDOWN_TIMEOUT", "0s"},
		{"BASE_URL", ""},
		{"BASE_URL", "example.com"},
		{"BASE_URL", "http://localhost:65536"},
		{"BASE_URL", "http://localhost:0"},
		{"BASE_URL", "http://localhost:"},
		{"BASE_URL", "ftp://example.com"},
		{"BASE_URL", "https://user:password@example.com"},
		{"BASE_URL", "https://example.com/path"},
		{"BASE_URL", "https://example.com?x=1"},
		{"BASE_URL", "https://example.com#fragment"},
		{"POSTGRES_HOST", ""},
		{"POSTGRES_USER", ""},
		{"POSTGRES_PASSWORD", ""},
		{"POSTGRES_DB", " "},
		{"POSTGRES_PORT", "abc"},
		{"POSTGRES_PORT", "-1"},
		{"POSTGRES_PORT", "0"},
		{"POSTGRES_PORT", "65536"},
		{"POSTGRES_SSLMODE", "invalid"},
		{"POSTGRES_CONNECT_TIMEOUT", "-1s"},
		{"POSTGRES_CONNECT_TIMEOUT", "5"},
		{"POSTGRES_MAX_CONNS", "0"},
		{"POSTGRES_MAX_CONNS", "1001"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"/"+tt.value, func(t *testing.T) {
			prepareEnv(t)
			t.Setenv(tt.key, tt.value)
			_, err := config.Load()
			if err == nil || !strings.Contains(err.Error(), tt.key) {
				t.Fatalf("expected an error naming %s, got %v", tt.key, err)
			}
			if strings.Contains(err.Error(), "test-password") {
				t.Fatal("configuration error exposed the database password")
			}
		})
	}
}

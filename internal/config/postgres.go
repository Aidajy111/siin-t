package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type PostgresConfig struct {
	Host           string
	Port           uint16
	User           string
	Password       string
	Database       string
	SSLMode        string
	ConnectTimeout time.Duration
	MaxConns       int32
}

func loadPostgres(v *viper.Viper) (PostgresConfig, error) {
	cfg := PostgresConfig{
		Host:     v.GetString("POSTGRES_HOST"),
		User:     v.GetString("POSTGRES_USER"),
		Password: v.GetString("POSTGRES_PASSWORD"),
		Database: v.GetString("POSTGRES_DB"),
		SSLMode:  v.GetString("POSTGRES_SSLMODE"),
	}
	for _, field := range []struct{ name, value string }{
		{"POSTGRES_HOST", cfg.Host},
		{"POSTGRES_USER", cfg.User},
		{"POSTGRES_DB", cfg.Database},
	} {
		if strings.TrimSpace(field.value) == "" {
			return PostgresConfig{}, fmt.Errorf("%s must not be empty", field.name)
		}
	}
	if cfg.Password == "" {
		return PostgresConfig{}, errors.New("POSTGRES_PASSWORD is required")
	}
	switch cfg.SSLMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return PostgresConfig{}, errors.New("POSTGRES_SSLMODE must be disable, allow, prefer, require, verify-ca or verify-full")
	}

	port, err := readInt(v, "POSTGRES_PORT", 1, 65535)
	if err != nil {
		return PostgresConfig{}, err
	}
	maxConns, err := readInt(v, "POSTGRES_MAX_CONNS", 1, 1000)
	if err != nil {
		return PostgresConfig{}, err
	}
	cfg.ConnectTimeout, err = readDuration(v, "POSTGRES_CONNECT_TIMEOUT")
	if err != nil {
		return PostgresConfig{}, err
	}
	cfg.Port = uint16(port)
	cfg.MaxConns = int32(maxConns)
	return cfg, nil
}

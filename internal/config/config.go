package config

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/viper"
)

const (
	HTTPKey     = "http"
	AppKey      = "app"
	PostgresKey = "postgres"
)

func Load() (*viper.Viper, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.AllowEmptyEnv(true)

	for key, value := range map[string]string{
		"HTTP_ADDR":                ":8080",
		"HTTP_SHUTDOWN_TIMEOUT":    "10s",
		"BASE_URL":                 "http://localhost:8080",
		"POSTGRES_HOST":            "localhost",
		"POSTGRES_PORT":            "5432",
		"POSTGRES_USER":            "siin",
		"POSTGRES_DB":              "siin",
		"POSTGRES_SSLMODE":         "disable",
		"POSTGRES_CONNECT_TIMEOUT": "5s",
		"POSTGRES_MAX_CONNS":       "10",
	} {
		v.SetDefault(key, value)
	}

	httpConfig, err := loadHTTP(v)
	if err != nil {
		return nil, err
	}
	appConfig, err := loadApp(v)
	if err != nil {
		return nil, err
	}
	postgresConfig, err := loadPostgres(v)
	if err != nil {
		return nil, err
	}

	v.Set(HTTPKey, httpConfig)
	v.Set(AppKey, appConfig)
	v.Set(PostgresKey, postgresConfig)
	return v, nil
}

func readInt(v *viper.Viper, key string, min, max int) (int, error) {
	value, err := strconv.Atoi(v.GetString(key))
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, min, max)
	}
	return value, nil
}

func readDuration(v *viper.Viper, key string) (time.Duration, error) {
	value, err := time.ParseDuration(v.GetString(key))
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration, for example 5s", key)
	}
	return value, nil
}

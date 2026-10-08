package config

import (
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/spf13/viper"
)

type HTTPConfig struct {
	Addr            string
	ShutdownTimeout time.Duration
}

func loadHTTP(v *viper.Viper) (HTTPConfig, error) {
	cfg := HTTPConfig{Addr: v.GetString("HTTP_ADDR")}
	_, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return HTTPConfig{}, errors.New("HTTP_ADDR must have the form host:port or :port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return HTTPConfig{}, errors.New("HTTP_ADDR port must be between 1 and 65535")
	}
	cfg.ShutdownTimeout, err = readDuration(v, "HTTP_SHUTDOWN_TIMEOUT")
	if err != nil {
		return HTTPConfig{}, err
	}
	return cfg, nil
}

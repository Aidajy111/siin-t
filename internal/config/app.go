package config

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

type AppConfig struct {
	BaseURL string
}

func loadApp(v *viper.Viper) (AppConfig, error) {
	baseURL := v.GetString("BASE_URL")
	u, err := url.Parse(baseURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") {
		return AppConfig{}, errors.New("BASE_URL must be an absolute HTTP(S) origin without credentials, path, query or fragment")
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return AppConfig{}, errors.New("BASE_URL port must be between 1 and 65535")
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return AppConfig{}, errors.New("BASE_URL port must not be empty")
	}
	return AppConfig{BaseURL: strings.TrimRight(baseURL, "/")}, nil
}

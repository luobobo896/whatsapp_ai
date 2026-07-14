package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var cookieNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type Config struct {
	Environment       string
	Production        bool
	Host              string
	Port              int
	AppOrigin         *url.URL
	DatabaseURL       string
	SessionCookieName string
	SessionTTL        time.Duration
	LoginRateLimit    int
	LoginRateWindow   time.Duration
}

func Parse(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, fmt.Errorf("environment getter is required")
	}

	environment := strings.TrimSpace(getenv("APP_ENV"))
	switch environment {
	case "development", "test", "production":
	default:
		return Config{}, invalid("APP_ENV")
	}

	host := strings.TrimSpace(getenv("HTTP_HOST"))
	if host == "" {
		return Config{}, invalid("HTTP_HOST")
	}
	port, err := parseInt(getenv, "PORT", 1, 65535)
	if err != nil {
		return Config{}, err
	}

	origin, err := url.Parse(strings.TrimSpace(getenv("APP_ORIGIN")))
	if err != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") || origin.User != nil {
		return Config{}, invalid("APP_ORIGIN")
	}

	databaseURL := strings.TrimSpace(getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, invalid("DATABASE_URL")
	}

	cookieName := strings.TrimSpace(getenv("SESSION_COOKIE_NAME"))
	if !cookieNamePattern.MatchString(cookieName) {
		return Config{}, invalid("SESSION_COOKIE_NAME")
	}
	ttlHours, err := parseInt(getenv, "SESSION_TTL_HOURS", 1, 168)
	if err != nil {
		return Config{}, err
	}
	rateLimit, err := parseInt(getenv, "LOGIN_RATE_LIMIT", 1, int(^uint(0)>>1))
	if err != nil {
		return Config{}, err
	}
	rateWindowSeconds, err := parseInt(getenv, "LOGIN_RATE_WINDOW_SECONDS", 1, int(^uint(0)>>1))
	if err != nil {
		return Config{}, err
	}

	return Config{
		Environment:       environment,
		Production:        environment == "production",
		Host:              host,
		Port:              port,
		AppOrigin:         origin,
		DatabaseURL:       databaseURL,
		SessionCookieName: cookieName,
		SessionTTL:        time.Duration(ttlHours) * time.Hour,
		LoginRateLimit:    rateLimit,
		LoginRateWindow:   time.Duration(rateWindowSeconds) * time.Second,
	}, nil
}

func parseInt(getenv func(string) string, key string, minimum, maximum int) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(getenv(key)))
	if err != nil || value < minimum || value > maximum {
		return 0, invalid(key)
	}
	return value, nil
}

func invalid(key string) error {
	return fmt.Errorf("invalid or missing %s", key)
}

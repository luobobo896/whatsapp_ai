package config

import (
	"testing"
	"time"
)

func TestParseAcceptsRuntimeContract(t *testing.T) {
	cfg, err := Parse(envGetter(validTestEnv()))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Environment != "test" || cfg.Production {
		t.Fatalf("unexpected environment: %#v", cfg)
	}
	if cfg.Host != "127.0.0.1" || cfg.Port != 8790 {
		t.Fatalf("unexpected listener: %#v", cfg)
	}
	if cfg.AppOrigin.String() != "http://localhost:8790" {
		t.Fatalf("unexpected origin: %s", cfg.AppOrigin)
	}
	if cfg.SessionTTL != 12*time.Hour || cfg.LoginRateWindow != time.Minute {
		t.Fatalf("unexpected durations: %#v", cfg)
	}
}

func TestParseRejectsUnsafeValues(t *testing.T) {
	tests := map[string]string{
		"missing database URL":       "DATABASE_URL",
		"missing application origin": "APP_ORIGIN",
		"missing cookie name":        "SESSION_COOKIE_NAME",
	}
	for name, key := range tests {
		t.Run(name, func(t *testing.T) {
			env := validTestEnv()
			env[key] = ""
			if _, err := Parse(envGetter(env)); err == nil {
				t.Fatalf("expected %s error", key)
			}
		})
	}
}

func TestParseRejectsInvalidRuntimeValues(t *testing.T) {
	tests := []struct {
		key, value string
	}{
		{"APP_ENV", "staging"},
		{"PORT", "0"},
		{"PORT", "65536"},
		{"APP_ORIGIN", "ftp://localhost"},
		{"APP_ORIGIN", "http://user:pass@localhost"},
		{"SESSION_COOKIE_NAME", "not-valid"},
		{"SESSION_TTL_HOURS", "169"},
		{"LOGIN_RATE_LIMIT", "0"},
		{"LOGIN_RATE_WINDOW_SECONDS", "-1"},
	}
	for _, test := range tests {
		t.Run(test.key+"="+test.value, func(t *testing.T) {
			env := validTestEnv()
			env[test.key] = test.value
			if _, err := Parse(envGetter(env)); err == nil {
				t.Fatalf("expected invalid %s to fail", test.key)
			}
		})
	}
}

func validTestEnv() map[string]string {
	return map[string]string{
		"APP_ENV":                   "test",
		"HTTP_HOST":                 "127.0.0.1",
		"PORT":                      "8790",
		"APP_ORIGIN":                "http://localhost:8790",
		"DATABASE_URL":              "postgres://whatsapp_app@localhost:5432/whatsapp_ai_test",
		"SESSION_COOKIE_NAME":       "wa_session",
		"SESSION_TTL_HOURS":         "12",
		"LOGIN_RATE_LIMIT":          "5",
		"LOGIN_RATE_WINDOW_SECONDS": "60",
	}
}

func envGetter(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

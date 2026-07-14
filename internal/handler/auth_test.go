package handler

import "testing"

func TestSessionCookieSecureDefaultsToTrue(t *testing.T) {
	t.Setenv("COOKIE_SECURE", "")
	if !sessionCookieSecure() {
		t.Fatal("session cookies must be secure by default")
	}
}

func TestSessionCookieSecureAllowsExplicitLocalOverride(t *testing.T) {
	t.Setenv("COOKIE_SECURE", "false")
	if sessionCookieSecure() {
		t.Fatal("COOKIE_SECURE=false must support local HTTP development")
	}
}

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

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

func TestLoginRateLimitLocksOutAfterThreshold(t *testing.T) {
	// Reset shared limiter state so prior tests do not influence this one.
	loginLimiter.mu.Lock()
	loginLimiter.failures = map[string][]time.Time{}
	loginLimiter.mu.Unlock()

	now := time.Now()
	// Pre-populate failures directly against the IP-derived subject that
	// enforceLoginRateLimit will compute from RemoteAddr below.
	subject := "ip:1.2.3.4"
	for i := 0; i < loginFailureThreshold; i++ {
		loginLimiter.recordLoginFailure(subject, now)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.RemoteAddr = "1.2.3.4:1234"
	// When the threshold has been reached enforceLoginRateLimit must return
	// false AND write a 429 so the LB sees the lockout.
	if enforceLoginRateLimit(c, "attacker@example.com") {
		t.Fatal("expected rate limit to engage once threshold reached")
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 once threshold reached, got %d", w.Code)
	}
}

func TestLoginRateLimitClearsOnSuccess(t *testing.T) {
	loginLimiter.mu.Lock()
	loginLimiter.failures = map[string][]time.Time{}
	loginLimiter.mu.Unlock()

	subject := "email:user@example.com"
	loginLimiter.recordLoginFailure(subject, time.Now())
	if got := loginLimiter.recentFailureCount(subject, time.Now()); got != 1 {
		t.Fatalf("expected 1 failure, got %d", got)
	}
	loginLimiter.clearLoginFailures(subject)
	if got := loginLimiter.recentFailureCount(subject, time.Now()); got != 0 {
		t.Fatalf("expected 0 failures after clear, got %d", got)
	}
}

func TestLoginRateLimitPrunesOldFailures(t *testing.T) {
	loginLimiter.mu.Lock()
	loginLimiter.failures = map[string][]time.Time{}
	loginLimiter.mu.Unlock()

	subject := "ip:stale"
	// An old failure outside the sliding window must not count.
	loginLimiter.recordLoginFailure(subject, time.Now().Add(-2*loginFailureWindow))
	loginLimiter.recordLoginFailure(subject, time.Now().Add(-2*loginFailureWindow))
	if got := loginLimiter.recentFailureCount(subject, time.Now()); got != 0 {
		t.Fatalf("expected stale failures to be pruned, got %d", got)
	}
}

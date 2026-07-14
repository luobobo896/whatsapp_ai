package auth_test

import (
	"strings"
	"testing"

	"whatsapp-ai-poc/internal/auth"
)

func TestPasswordRoundTripAndMalformedHash(t *testing.T) {
	hash, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "scrypt$N=16384,r=8,p=1$") {
		t.Fatalf("unexpected password encoding: %s", hash)
	}
	if !auth.VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("correct password did not match")
	}
	if auth.VerifyPassword(hash, "wrong") {
		t.Fatal("wrong password matched")
	}
	for _, malformed := range []string{"", "broken", "scrypt$N=1,r=1,p=1$bad$bad", hash + "$extra"} {
		if auth.VerifyPassword(malformed, "correct horse battery staple") {
			t.Fatalf("malformed hash matched: %q", malformed)
		}
	}
}

func TestPasswordUsesUniqueSalt(t *testing.T) {
	first, err := auth.HashPassword("same password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := auth.HashPassword("same password")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("password hashes reused a salt")
	}
}

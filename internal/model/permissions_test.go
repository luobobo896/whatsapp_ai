package model

import (
	"slices"
	"testing"
)

func TestAgentCanReplyWithoutManagingAccountsOrKnowledge(t *testing.T) {
	permissions := PermissionsForRole("agent")
	if !slices.Contains(permissions, "conversations:reply") {
		t.Fatalf("agent permissions = %#v, want conversations:reply", permissions)
	}
	for _, forbidden := range []string{"accounts:manage", "knowledge:manage", "members:manage"} {
		if slices.Contains(permissions, forbidden) {
			t.Fatalf("agent permissions = %#v, must not contain %q", permissions, forbidden)
		}
	}
}

func TestTenantManagersCanReply(t *testing.T) {
	for _, role := range []string{"owner", "admin"} {
		if permissions := PermissionsForRole(role); !slices.Contains(permissions, "conversations:reply") {
			t.Fatalf("%s permissions = %#v, want conversations:reply", role, permissions)
		}
	}
}

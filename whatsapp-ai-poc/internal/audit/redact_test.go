package audit_test

import (
	"reflect"
	"testing"

	"whatsapp-ai-poc/internal/audit"
)

func TestRedactSecretsNested(t *testing.T) {
	input := map[string]any{
		"label": "Model",
		"credentials": map[string]any{
			"apiKey": "real-key",
			"nested": []any{
				map[string]any{"refresh_token": "real-token", "region": "cn"},
			},
		},
		"passwordConfirmation": "real-password",
	}
	want := map[string]any{
		"label": "Model",
		"credentials": map[string]any{
			"apiKey": "[REDACTED]",
			"nested": []any{
				map[string]any{"refresh_token": "[REDACTED]", "region": "cn"},
			},
		},
		"passwordConfirmation": "[REDACTED]",
	}
	if got := audit.Redact(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("redacted value mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestRedactDoesNotMutateInput(t *testing.T) {
	nested := map[string]any{"apiKey": "real-key"}
	input := map[string]any{"credentials": nested}
	_ = audit.Redact(input)
	if nested["apiKey"] != "real-key" {
		t.Fatal("redaction mutated caller input")
	}
}

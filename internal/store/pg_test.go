package store

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeAccountDeletionExecutor struct {
	calls       []string
	accountRows int64
}

func (f *fakeAccountDeletionExecutor) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	f.calls = append(f.calls, sql)
	if strings.Contains(sql, "DELETE FROM accounts") {
		return pgconn.NewCommandTag("DELETE " + strconv.FormatInt(f.accountRows, 10)), nil
	}
	return pgconn.NewCommandTag("DELETE 1"), nil
}

func TestDeleteAccountRowsRemovesAccountScopedData(t *testing.T) {
	executor := &fakeAccountDeletionExecutor{accountRows: 1}
	if err := deleteAccountRows(context.Background(), executor, "tenant-1", "account-1"); err != nil {
		t.Fatal(err)
	}
	if len(executor.calls) != 3 {
		t.Fatalf("delete calls = %d, want 3", len(executor.calls))
	}
	if !strings.Contains(executor.calls[0], "conversation_messages") ||
		!strings.Contains(executor.calls[1], "conversations") ||
		!strings.Contains(executor.calls[2], "accounts") {
		t.Fatalf("unexpected delete order: %#v", executor.calls)
	}
}

func TestDeleteAccountRowsRejectsUnknownTenantAccount(t *testing.T) {
	executor := &fakeAccountDeletionExecutor{accountRows: 0}
	err := deleteAccountRows(context.Background(), executor, "other-tenant", "account-1")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("delete error = %v, want pgx.ErrNoRows", err)
	}
}

func TestSplitQueryAddsBigramsWithoutSingletonNoise(t *testing.T) {
	got := splitQuery("羽绒服怎么洗？")
	want := []string{"羽绒服怎么洗", "羽绒", "绒服", "服怎", "怎么", "么洗"}
	if !slices.Equal(got, want) {
		t.Fatalf("splitQuery() = %#v, want %#v", got, want)
	}
}

func TestSplitQueryDoesNotCreateBigramsForEnglishWords(t *testing.T) {
	got := splitQuery("return policy")
	want := []string{"return", "policy"}
	if !slices.Equal(got, want) {
		t.Fatalf("splitQuery() = %#v, want %#v", got, want)
	}
}

func TestDailyReplyLimitReached(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		count int
		want  bool
	}{
		{name: "unlimited", limit: 0, count: 10000, want: false},
		{name: "below limit", limit: 3, count: 2, want: false},
		{name: "at limit", limit: 3, count: 3, want: true},
		{name: "over limit", limit: 3, count: 4, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dailyReplyLimitReached(tt.limit, tt.count); got != tt.want {
				t.Fatalf("dailyReplyLimitReached(%d, %d) = %t, want %t", tt.limit, tt.count, got, tt.want)
			}
		})
	}
}

func TestDedupMessageIDIsDeterministicAndRoleScoped(t *testing.T) {
	// Same inputs must produce the same key so retries collapse atomically.
	a := dedupMessageID("customer", "hi")
	b := dedupMessageID("customer", "hi")
	if a != b {
		t.Fatalf("dedupMessageID not deterministic: %q vs %q", a, b)
	}
	// Different role or content must produce different keys so legitimate
	// distinct messages are not collapsed together.
	if dedupMessageID("customer", "hi") == dedupMessageID("assistant", "hi") {
		t.Fatal("dedupMessageID should differ when role differs")
	}
	if dedupMessageID("customer", "hi") == dedupMessageID("customer", "hello") {
		t.Fatal("dedupMessageID should differ when content differs")
	}
	// Output is hex-encoded (no NUL/control bytes) so it is safe to store and
	// matches against the partial unique index predicate.
	for _, r := range a {
		if r < 0x20 || r > 0x7e {
			t.Fatalf("dedupMessageID contains non-printable byte: %q", a)
		}
	}
}

func TestEscapeILIKEPattern(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain text unchanged", in: "hello", want: "hello"},
		{name: "percent escaped", in: "50%", want: `50\%`},
		{name: "underscore escaped", in: "user_id", want: `user\_id`},
		{name: "backslash doubled", in: `a\b`, want: `a\\b`},
		{name: "all wildcards together", in: `%_\`, want: `\%\_\\`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeILIKEPattern(tt.in); got != tt.want {
				t.Fatalf("escapeILIKEPattern(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

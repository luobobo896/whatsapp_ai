package legacy_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"

	"whatsapp-ai-poc/internal/legacy"
)

func TestInspectProducesDeterministicSafeReport(t *testing.T) {
	paths := fixturePaths(t)
	want := legacy.Report{
		Accounts: legacy.Count{Valid: 1, Invalid: 1},
		Roles:    legacy.Count{Valid: 3, Invalid: 0},
		Entries:  legacy.Count{Valid: 2, Invalid: 1},
		Conflicts: []legacy.Conflict{
			{Code: "DUPLICATE_ENTRY_ID", Source: "knowledge.json", Value: "product-1"},
			{Code: "UNKNOWN_ACCOUNT_ROLE", Source: "accounts.json", Value: "missing-role"},
		},
	}

	first, err := legacy.Inspect(t.Context(), paths)
	if err != nil {
		t.Fatal(err)
	}
	second, err := legacy.Inspect(t.Context(), paths)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("report mismatch\n got: %#v\nwant: %#v", first, want)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reports are nondeterministic\nfirst: %#v\nsecond: %#v", first, second)
	}
	for _, conflict := range first.Conflicts {
		if filepath.IsAbs(conflict.Source) {
			t.Fatalf("report leaked absolute path: %#v", conflict)
		}
	}
}

func TestInspectDoesNotModifySourcesAndOpensSQLiteReadOnly(t *testing.T) {
	paths := fixturePaths(t)
	sqlitePath := filepath.Join(t.TempDir(), "legacy.db")
	if err := createSQLiteFixture(sqlitePath); err != nil {
		t.Fatal(err)
	}
	paths.SQLite = sqlitePath
	before, err := os.ReadFile(sqlitePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Inspect(context.Background(), paths); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(sqlitePath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("inventory modified the SQLite source")
	}
}

func fixturePaths(t *testing.T) legacy.Paths {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return legacy.Paths{
		AccountsJSON:  filepath.Join(root, "test", "fixtures", "legacy", "accounts.json"),
		KnowledgeJSON: filepath.Join(root, "test", "fixtures", "legacy", "knowledge.json"),
	}
}

func createSQLiteFixture(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	if _, err := db.Exec("CREATE TABLE sample (id integer primary key)"); err != nil {
		_ = db.Close()
		return err
	}
	return db.Close()
}

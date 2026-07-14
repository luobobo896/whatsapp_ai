package legacy

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type Paths struct {
	AccountsJSON  string
	KnowledgeJSON string
	SQLite        string
}

type Count struct {
	Valid   int `json:"valid"`
	Invalid int `json:"invalid"`
}

type Conflict struct {
	Code   string `json:"code"`
	Source string `json:"source"`
	Value  string `json:"value"`
}

type Report struct {
	Accounts  Count      `json:"accounts"`
	Roles     Count      `json:"roles"`
	Entries   Count      `json:"entries"`
	Conflicts []Conflict `json:"conflicts"`
}

type accountWire struct {
	Label           string   `json:"label"`
	AllowedProducts []string `json:"allowedProducts"`
}

type knowledgeWire struct {
	Roles []json.RawMessage `json:"roles"`
}

type roleWire struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Products []json.RawMessage `json:"products"`
}

type entryWire struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func Inspect(ctx context.Context, paths Paths) (Report, error) {
	knowledgeData, err := safeRead(paths.KnowledgeJSON, "knowledge.json")
	if err != nil {
		return Report{}, err
	}
	accountsData, err := safeRead(paths.AccountsJSON, "accounts.json")
	if err != nil {
		return Report{}, err
	}

	var report Report
	report.Conflicts = make([]Conflict, 0)
	roleIDs, err := inspectKnowledge(knowledgeData, filepath.Base(paths.KnowledgeJSON), &report)
	if err != nil {
		return Report{}, err
	}
	if err := inspectAccounts(accountsData, filepath.Base(paths.AccountsJSON), roleIDs, &report); err != nil {
		return Report{}, err
	}
	if paths.SQLite != "" {
		if err := inspectSQLite(ctx, paths.SQLite); err != nil {
			return Report{}, err
		}
	}

	sort.Slice(report.Conflicts, func(i, j int) bool {
		left, right := report.Conflicts[i], report.Conflicts[j]
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		return left.Value < right.Value
	})
	return report, nil
}

func inspectKnowledge(data []byte, source string, report *Report) (map[string]struct{}, error) {
	var wire knowledgeWire
	if err := json.Unmarshal(data, &wire); err != nil || wire.Roles == nil {
		return nil, fmt.Errorf("decode %s failed", source)
	}
	roleIDs := make(map[string]struct{})
	entryIDs := make(map[string]struct{})
	for _, rawRole := range wire.Roles {
		var role roleWire
		if err := json.Unmarshal(rawRole, &role); err != nil || blank(role.ID) || blank(role.Name) || role.Products == nil {
			report.Roles.Invalid++
			continue
		}
		role.ID = strings.TrimSpace(role.ID)
		report.Roles.Valid++
		if _, exists := roleIDs[role.ID]; exists {
			report.Conflicts = append(report.Conflicts, Conflict{Code: "DUPLICATE_ROLE_ID", Source: source, Value: role.ID})
		} else {
			roleIDs[role.ID] = struct{}{}
		}
		for _, rawEntry := range role.Products {
			var entry entryWire
			if err := json.Unmarshal(rawEntry, &entry); err != nil || blank(entry.ID) || blank(entry.Name) {
				report.Entries.Invalid++
				continue
			}
			entry.ID = strings.TrimSpace(entry.ID)
			report.Entries.Valid++
			if _, exists := entryIDs[entry.ID]; exists {
				report.Conflicts = append(report.Conflicts, Conflict{Code: "DUPLICATE_ENTRY_ID", Source: source, Value: entry.ID})
			} else {
				entryIDs[entry.ID] = struct{}{}
			}
		}
	}
	return roleIDs, nil
}

func inspectAccounts(data []byte, source string, roleIDs map[string]struct{}, report *Report) error {
	var accounts map[string]json.RawMessage
	if err := json.Unmarshal(data, &accounts); err != nil || accounts == nil {
		return fmt.Errorf("decode %s failed", source)
	}
	keys := make([]string, 0, len(accounts))
	for key := range accounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var account accountWire
		if err := json.Unmarshal(accounts[key], &account); err != nil || blank(key) || blank(account.Label) || len(account.AllowedProducts) == 0 {
			report.Accounts.Invalid++
			continue
		}
		valid := true
		for _, roleID := range account.AllowedProducts {
			roleID = strings.TrimSpace(roleID)
			if roleID == "" {
				valid = false
				continue
			}
			if _, exists := roleIDs[roleID]; !exists {
				report.Conflicts = append(report.Conflicts, Conflict{Code: "UNKNOWN_ACCOUNT_ROLE", Source: source, Value: roleID})
			}
		}
		if valid {
			report.Accounts.Valid++
		} else {
			report.Accounts.Invalid++
		}
	}
	return nil
}

func inspectSQLite(ctx context.Context, path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve SQLite source failed")
	}
	dsn := (&url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open SQLite source failed")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var tables int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type = 'table'").Scan(&tables); err != nil {
		return fmt.Errorf("inspect SQLite source failed")
	}
	return nil
}

func safeRead(path, source string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%s path is required", source)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s failed", source)
	}
	return data, nil
}

func blank(value string) bool { return strings.TrimSpace(value) == "" }

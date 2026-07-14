package main

// kbload: loads knowledge-base articles from JSON files into the database.
// It contains NO hardcoded article content — every article comes from the
// JSON files in the seed directory. Each file describes one knowledge base:
//
//   {
//     "kb": "服装服饰知识库",
//     "description": "...",
//     "articles": [
//       {"title": "...", "content": "...", "category": "...",
//        "attributes": {"任意": "键值"}}
//     ]
//   }
//
// Config via env (with sensible fallbacks):
//   KBLOAD_DSN         postgres DSN
//   KBLOAD_TENANT      tenant name to attach the knowledge bases to
//   KBLOAD_ADMIN_EMAIL admin user to set as tenant owner
//   KBLOAD_DIR         directory holding the *.json seed files
//
// For each KB it REPLACES existing articles (delete-then-insert) so re-runs
// are idempotent, then re-chunks each article via the store's own pipeline.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/store"
)

type seedArticle struct {
	Title      string         `json:"title"`
	Content    string         `json:"content"`
	Category   string         `json:"category"`
	Attributes map[string]any `json:"attributes"`
}

type seedFile struct {
	KB          string        `json:"kb"`
	Description string        `json:"description"`
	Articles    []seedArticle `json:"articles"`
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	dsn := env("KBLOAD_DSN", os.Getenv("DATABASE_URL"))
	if dsn == "" {
		fmt.Println("KBLOAD_DSN or DATABASE_URL must be set")
		os.Exit(1)
	}
	tenantName := env("KBLOAD_TENANT", "智能客服知识库中心")
	adminEmail := env("KBLOAD_ADMIN_EMAIL", "admin@whatsapp-ai.local")
	dir := env("KBLOAD_DIR", "/tmp/kb_seed")

	ctx := context.Background()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		fmt.Println("open err:", err)
		os.Exit(1)
	}
	defer st.Close()

	// Raw pool for delete + orphan cleanup (store has no delete-by-KB helper).
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Println("pool err:", err)
		os.Exit(1)
	}
	defer pool.Close()

	// 1. Find or create the tenant.
	tenants, err := st.AllTenants()
	if err != nil {
		fmt.Println("tenants err:", err)
		os.Exit(1)
	}
	var tenantID string
	for _, t := range tenants {
		if t.Name == tenantName {
			tenantID = t.ID
			break
		}
	}
	if tenantID == "" {
		t, err := st.CreateTenant(tenantName)
		if err != nil {
			fmt.Println("create tenant err:", err)
			os.Exit(1)
		}
		tenantID = t.ID
		fmt.Printf("created tenant %q id=%s\n", tenantName, tenantID)
	} else {
		fmt.Printf("reusing tenant %q id=%s\n", tenantName, tenantID)
	}

	// 2. Ensure admin is owner.
	if admin, err := st.UserByEmail(adminEmail); err == nil {
		st.AddTenantMember(tenantID, admin.ID, "owner")
	}

	// 3. Load seed files.
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) == 0 {
		fmt.Printf("no seed files in %s (err=%v)\n", dir, err)
		os.Exit(1)
	}
	sort.Strings(files)

	existing, _ := st.KnowledgeBasesByTenant(tenantID)
	byName := map[string]string{}
	for _, k := range existing {
		byName[k.Name] = k.ID
	}

	total := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			fmt.Printf("read %s err: %v\n", f, err)
			continue
		}
		var sf seedFile
		if err := json.Unmarshal(raw, &sf); err != nil {
			fmt.Printf("parse %s err: %v\n", f, err)
			continue
		}
		if sf.KB == "" || len(sf.Articles) == 0 {
			fmt.Printf("skip %s: empty kb/articles\n", f)
			continue
		}

		kbID, ok := byName[sf.KB]
		if !ok {
			kb, err := st.CreateKnowledgeBase(tenantID, sf.KB, sf.Description)
			if err != nil {
				fmt.Printf("create KB %q err: %v\n", sf.KB, err)
				continue
			}
			kbID = kb.ID
			byName[sf.KB] = kbID
		} else {
			// Replace: delete existing articles (chunks cleaned below).
			pool.Exec(ctx, `DELETE FROM knowledge_articles WHERE knowledge_base_id=$1`, kbID)
		}

		count := 0
		for _, a := range sf.Articles {
			if a.Title == "" || a.Content == "" {
				continue
			}
			attrs := "{}"
			if len(a.Attributes) > 0 {
				if b, err := json.Marshal(a.Attributes); err == nil {
					attrs = string(b)
				}
			}
			art, err := st.CreateArticle(kbID, a.Title, a.Content, a.Category, attrs)
			if err != nil {
				fmt.Printf("  article err (%s): %v\n", a.Title, err)
				continue
			}
			st.ChunkArticle(art.ID, a.Content)
			count++
		}
		total += count
		fmt.Printf("loaded KB %q id=%s articles=%d (from %s)\n", sf.KB, kbID, count, filepath.Base(f))
	}

	// 4. Remove chunks orphaned by the deletes above.
	pool.Exec(ctx, `DELETE FROM knowledge_chunks c WHERE NOT EXISTS (SELECT 1 FROM knowledge_articles a WHERE a.id = c.article_id)`)

	fmt.Printf("\nDONE. tenant=%s total articles loaded=%d\n", tenantID, total)
}

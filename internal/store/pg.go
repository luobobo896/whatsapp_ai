package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/model"
)

// ChunkInfo is used by GetChunksWithoutEmbeddings
type ChunkInfo struct {
	ID      string
	Content string
}

type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 20
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect pg: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping pg: %w", err)
	}
	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() { s.pool.Close() }

func genID() string  { b := make([]byte, 12); rand.Read(b); return hex.EncodeToString(b) }
func genToken() string { b := make([]byte, 32); rand.Read(b); return hex.EncodeToString(b) }

// ---- migrations ----

func (s *Store) migrate(ctx context.Context) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY, email TEXT UNIQUE NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL, platform_role TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY, name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_members (
			tenant_id TEXT NOT NULL REFERENCES tenants(id),
			user_id TEXT NOT NULL REFERENCES users(id),
			role TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'active',
			PRIMARY KEY (tenant_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL,
			csrf_token TEXT NOT NULL, active_tenant_id TEXT,
			expires_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS invitations (
			id TEXT PRIMARY KEY, token TEXT UNIQUE NOT NULL,
			tenant_id TEXT NOT NULL, email TEXT NOT NULL,
			role TEXT NOT NULL, expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL,
			name TEXT NOT NULL, account_key TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			daily_limit INTEGER NOT NULL DEFAULT 30,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS knowledge_bases (
			id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL,
			name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS knowledge_articles (
			id TEXT PRIMARY KEY, knowledge_base_id TEXT NOT NULL,
			title TEXT NOT NULL, content TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT '',
			attributes TEXT NOT NULL DEFAULT '{}',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS knowledge_chunks (
				id TEXT PRIMARY KEY, article_id TEXT NOT NULL,
				content TEXT NOT NULL, chunk_index INTEGER NOT NULL DEFAULT 0,
				embedding TEXT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_article ON knowledge_chunks(article_id)`,
		`CREATE TABLE IF NOT EXISTS conversation_messages (
			id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL, customer_name TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL, content TEXT NOT NULL,
			knowledge_ids TEXT NOT NULL DEFAULT '[]',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_conv_msg_lookup ON conversation_messages(tenant_id, conversation_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL,
			account_id TEXT NOT NULL, customer TEXT NOT NULL,
			last_message TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			last_message_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	}
	for _, d := range ddl {
		if _, err := s.pool.Exec(context.Background(), d); err != nil {
			return fmt.Errorf("ddl: %w\n%s", err, d)
		}
	}
	// Extensions
	s.pool.Exec(context.Background(), `CREATE EXTENSION IF NOT EXISTS pg_trgm`)
	// Migrations
	s.pool.Exec(context.Background(), `ALTER TABLE knowledge_articles ADD COLUMN IF NOT EXISTS attributes TEXT NOT NULL DEFAULT '{}'`)
	// Performance indexes
	s.pool.Exec(context.Background(), `CREATE INDEX IF NOT EXISTS idx_articles_title_trgm ON knowledge_articles USING GIN (title gin_trgm_ops)`)
	s.pool.Exec(context.Background(), `CREATE INDEX IF NOT EXISTS idx_articles_content_trgm ON knowledge_articles USING GIN (content gin_trgm_ops)`)
	s.pool.Exec(context.Background(), `CREATE INDEX IF NOT EXISTS idx_articles_status ON knowledge_articles(status)`)
	s.pool.Exec(context.Background(), `CREATE INDEX IF NOT EXISTS idx_articles_kbid ON knowledge_articles(knowledge_base_id)`)
	s.pool.Exec(context.Background(), `CREATE INDEX IF NOT EXISTS idx_kb_tenant_status ON knowledge_bases(tenant_id, status)`)
	s.pool.Exec(context.Background(), `CREATE INDEX IF NOT EXISTS idx_chunks_embedding ON knowledge_chunks(article_id) WHERE embedding IS NOT NULL`)
	s.pool.Exec(context.Background(), `ALTER TABLE knowledge_articles ADD COLUMN IF NOT EXISTS search_vector tsvector`)
	s.pool.Exec(context.Background(), `CREATE INDEX IF NOT EXISTS idx_articles_search ON knowledge_articles USING GIN (search_vector)`)
	s.pool.Exec(context.Background(), `CREATE OR REPLACE FUNCTION articles_search_update() RETURNS trigger AS $$ BEGIN NEW.search_vector := setweight(to_tsvector('simple', COALESCE(NEW.title, '')), 'A') || setweight(to_tsvector('simple', COALESCE(NEW.content, '')), 'B') || setweight(to_tsvector('simple', COALESCE(NEW.category, '')), 'C') || setweight(to_tsvector('simple', COALESCE(NEW.attributes, '')), 'D'); RETURN NEW; END; $$ LANGUAGE plpgsql`)
	s.pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS trg_articles_search ON knowledge_articles`)
	s.pool.Exec(context.Background(), `CREATE TRIGGER trg_articles_search BEFORE INSERT OR UPDATE ON knowledge_articles FOR EACH ROW EXECUTE FUNCTION articles_search_update()`)
	s.pool.Exec(context.Background(), `UPDATE knowledge_articles SET search_vector = setweight(to_tsvector('simple', COALESCE(title, '')), 'A') || setweight(to_tsvector('simple', COALESCE(content, '')), 'B') || setweight(to_tsvector('simple', COALESCE(category, '')), 'C') || setweight(to_tsvector('simple', COALESCE(attributes, '')), 'D') WHERE search_vector IS NULL`)
	return nil
}

// ---- users ----

func (s *Store) CreateUser( email, displayName, passwordHash, platformRole string) (*model.UserRow, error) {
	u := &model.UserRow{
		ID: genID(), Email: email, DisplayName: displayName,
		PasswordHash: passwordHash, PlatformRole: platformRole,
	}
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO users (id,email,display_name,password_hash,platform_role) VALUES ($1,$2,$3,$4,$5)
		 RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')`,
		u.ID, u.Email, u.DisplayName, u.PasswordHash, u.PlatformRole,
	).Scan(&u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) UserByEmail( email string) (*model.UserRow, error) {
	u := &model.UserRow{}
	err := s.pool.QueryRow(context.Background(),
		`SELECT id,email,display_name,password_hash,platform_role,
		        to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.PlatformRole, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) UserByID( id string) (*model.UserRow, error) {
	u := &model.UserRow{}
	err := s.pool.QueryRow(context.Background(),
		`SELECT id,email,display_name,password_hash,platform_role,
		        to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM users WHERE id=$1`, id,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.PlatformRole, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ---- sessions ----

func (s *Store) CreateSession( userID string) (*model.SessionRow, error) {
	sess := &model.SessionRow{
		ID: genToken(), UserID: userID, CSRFToken: genToken(),
		ExpiresAt: time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05"),
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO sessions (id,user_id,csrf_token,active_tenant_id,expires_at)
		 VALUES ($1,$2,$3,NULL,$4)`,
		sess.ID, sess.UserID, sess.CSRFToken, sess.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) SessionByID( id string) (*model.SessionRow, error) {
	sess := &model.SessionRow{}
	var activeTenant *string
	err := s.pool.QueryRow(context.Background(),
		`SELECT id,user_id,csrf_token,active_tenant_id,
		        to_char(expires_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM sessions WHERE id=$1`, id,
	).Scan(&sess.ID, &sess.UserID, &sess.CSRFToken, &activeTenant, &sess.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if activeTenant != nil {
		sess.ActiveTenantID = *activeTenant
	}
	if expires, err := time.Parse("2006-01-02 15:04:05", sess.ExpiresAt); err != nil || time.Now().After(expires) {
		s.pool.Exec(context.Background(), `DELETE FROM sessions WHERE id=$1`, id)
		return nil, fmt.Errorf("session expired")
	}
	return sess, nil
}

func (s *Store) UpdateSessionTenant( sessionID, tenantID string) error {
	_, err := s.pool.Exec(context.Background(), `UPDATE sessions SET active_tenant_id=$1 WHERE id=$2`, tenantID, sessionID)
	return err
}

func (s *Store) DeleteSession( id string) error {
	_, err := s.pool.Exec(context.Background(), `DELETE FROM sessions WHERE id=$1`, id)
	return err
}

// ---- tenants ----

func (s *Store) CreateTenant( name string) (*model.TenantRow, error) {
	t := &model.TenantRow{ID: genID(), Name: name, Status: "active"}
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO tenants (id,name) VALUES ($1,$2) RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')`,
		t.ID, t.Name,
	).Scan(&t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) TenantByID( id string) (*model.TenantRow, error) {
	t := &model.TenantRow{}
	err := s.pool.QueryRow(context.Background(),
		`SELECT id,name,status,to_char(created_at, 'YYYY-MM-DD HH24:MI:SS') FROM tenants WHERE id=$1`, id,
	).Scan(&t.ID, &t.Name, &t.Status, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) UpdateTenantStatus( id, status string) error {
	_, err := s.pool.Exec(context.Background(), `UPDATE tenants SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (s *Store) TenantsForUser( userID string) ([]model.TenantWithMembership, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT t.id, t.name, t.status, tm.role, tm.status
		FROM tenants t
		JOIN tenant_members tm ON t.id = tm.tenant_id
		WHERE tm.user_id = $1
		ORDER BY t.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.TenantWithMembership
	for rows.Next() {
		var m model.TenantWithMembership
		if err := rows.Scan(&m.ID, &m.Name, &m.Status, &m.Role, &m.MembershipStatus); err != nil {
			return nil, err
		}
		m.Permissions = model.PermissionsForRole(m.Role)
		list = append(list, m)
	}
	return list, rows.Err()
}

func (s *Store) AllTenants() ([]model.TenantWithMembership, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT id, name, status FROM tenants ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.TenantWithMembership
	for rows.Next() {
		var m model.TenantWithMembership
		if err := rows.Scan(&m.ID, &m.Name, &m.Status); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// ---- tenant members ----

func (s *Store) AddTenantMember( tenantID, userID, role string) error {
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO tenant_members (tenant_id,user_id,role,status) VALUES ($1,$2,$3,'active')
		 ON CONFLICT (tenant_id, user_id) DO UPDATE SET role=$3, status='active'`,
		tenantID, userID, role,
	)
	return err
}

func (s *Store) TenantMember( tenantID, userID string) (*model.TenantMemberRow, error) {
	m := &model.TenantMemberRow{}
	err := s.pool.QueryRow(context.Background(),
		`SELECT tenant_id,user_id,role,status FROM tenant_members WHERE tenant_id=$1 AND user_id=$2`,
		tenantID, userID,
	).Scan(&m.TenantID, &m.UserID, &m.Role, &m.Status)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Store) UpdateMember( tenantID, userID, role, status string) error {
	q := `UPDATE tenant_members SET `
	args := []any{}
	argIdx := 1
	if role != "" {
		q += fmt.Sprintf(`role=$%d, `, argIdx)
		args = append(args, role)
		argIdx++
	}
	if status != "" {
		q += fmt.Sprintf(`status=$%d, `, argIdx)
		args = append(args, status)
		argIdx++
	}
	q = q[:len(q)-2] + fmt.Sprintf(` WHERE tenant_id=$%d AND user_id=$%d`, argIdx, argIdx+1)
	args = append(args, tenantID, userID)
	_, err := s.pool.Exec(context.Background(), q, args...)
	return err
}

func (s *Store) TenantMembers( tenantID string) ([]model.Member, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT u.id, u.email, u.display_name, tm.role, tm.status,
		       to_char(u.created_at, 'YYYY-MM-DD HH24:MI:SS')
		FROM tenant_members tm
		JOIN users u ON u.id = tm.user_id
		WHERE tm.tenant_id = $1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Member
	for rows.Next() {
		var m model.Member
		if err := rows.Scan(&m.UserID, &m.Email, &m.DisplayName, &m.Role, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// ---- invitations ----

func (s *Store) CreateInvitation( tenantID, email, role string) (*model.InvitationRow, error) {
	inv := &model.InvitationRow{
		ID: genID(), Token: genToken(), TenantID: tenantID,
		Email: email, Role: role,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour).Format("2006-01-02 15:04:05"),
	}
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO invitations (id,token,tenant_id,email,role,expires_at) VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')`,
		inv.ID, inv.Token, inv.TenantID, inv.Email, inv.Role, inv.ExpiresAt,
	).Scan(&inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (s *Store) InvitationByToken( token string) (*model.InvitationRow, error) {
	inv := &model.InvitationRow{}
	err := s.pool.QueryRow(context.Background(),
		`SELECT id,token,tenant_id,email,role,
		        to_char(expires_at, 'YYYY-MM-DD HH24:MI:SS'),
		        to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM invitations WHERE token=$1`, token,
	).Scan(&inv.ID, &inv.Token, &inv.TenantID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	if expires, err := time.Parse("2006-01-02 15:04:05", inv.ExpiresAt); err != nil || time.Now().After(expires) {
		s.pool.Exec(context.Background(), `DELETE FROM invitations WHERE id=$1`, inv.ID)
		return nil, fmt.Errorf("invitation expired")
	}
	return inv, nil
}

func (s *Store) DeleteInvitation( id string) error {
	_, err := s.pool.Exec(context.Background(), `DELETE FROM invitations WHERE id=$1`, id)
	return err
}

// ---- accounts ----

func (s *Store) AccountsByTenant( tenantID string) ([]model.Account, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id,name,account_key,status,daily_limit,
		        to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM accounts WHERE tenant_id=$1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Account
	for rows.Next() {
		var a model.Account
		if err := rows.Scan(&a.ID, &a.Name, &a.AccountKey, &a.Status, &a.DailyLimit, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *Store) CreateAccount( tenantID, name string, dailyLimit int) (*model.AccountRow, error) {
	a := &model.AccountRow{
		ID: genID(), TenantID: tenantID, Name: name,
		AccountKey: "wa_" + genID()[:8], Status: "pending", DailyLimit: dailyLimit,
	}
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO accounts (id,tenant_id,name,account_key,status,daily_limit)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')`,
		a.ID, a.TenantID, a.Name, a.AccountKey, a.Status, a.DailyLimit,
	).Scan(&a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ---- knowledge bases ----

func (s *Store) KnowledgeBasesByTenant( tenantID string) ([]model.KnowledgeBase, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id,name,description,status,
		        to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM knowledge_bases WHERE tenant_id=$1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.KnowledgeBase
	for rows.Next() {
		var k model.KnowledgeBase
		if err := rows.Scan(&k.ID, &k.Name, &k.Description, &k.Status, &k.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, k)
	}
	return list, rows.Err()
}

func (s *Store) CreateKnowledgeBase( tenantID, name, description string) (*model.KnowledgeRow, error) {
	k := &model.KnowledgeRow{
		ID: genID(), TenantID: tenantID, Name: name,
		Description: description, Status: "active",
	}
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO knowledge_bases (id,tenant_id,name,description,status)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')`,
		k.ID, k.TenantID, k.Name, k.Description, k.Status,
	).Scan(&k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return k, nil
}

// ---- conversations ----

func (s *Store) SearchKnowledge( tenantID, query string, embedding []float32, limit int) ([]model.SearchResultItem, error) {
	if limit <= 0 { limit = 5 }
		// Vector search via Go cosine similarity if embedding provided
		if len(embedding) > 0 {
			rows, err := s.pool.Query(context.Background(),
				"SELECT a.id, a.title, a.content, a.category, a.attributes, k.name AS kb_name, c.id, c.embedding FROM knowledge_chunks c JOIN knowledge_articles a ON c.article_id = a.id JOIN knowledge_bases k ON a.knowledge_base_id = k.id WHERE k.tenant_id=$1 AND a.status='active' AND k.status='active' AND c.embedding IS NOT NULL", tenantID)
			if err == nil {
				defer rows.Close()
				type row struct {
					artID, title, content, category, attrs, kbName, chunkID, emb string
				}
				var chunks []row
				for rows.Next() {
					var r row
					if err := rows.Scan(&r.artID, &r.title, &r.content, &r.category, &r.attrs, &r.kbName, &r.chunkID, &r.emb); err != nil {
						continue
					}
					chunks = append(chunks, r)
				}
				rows.Close()
				type scored struct {
					item  model.SearchResultItem
					score float64
				}
				var results []scored
				for _, ck := range chunks {
					var embVec []float64
					if json.Unmarshal([]byte(ck.emb), &embVec) == nil && len(embVec) > 0 {
						queryVec := make([]float64, len(embedding))
						for i_, v := range embedding { queryVec[i_] = float64(v) }
						sim := cosineSimilarity(queryVec, embVec)
						results = append(results, scored{
							item:  model.SearchResultItem{ID: ck.artID, Title: ck.title, Content: ck.content, Category: ck.category, Attributes: ck.attrs, KnowledgeBaseName: ck.kbName, Score: sim},
							score: sim,
						})
					}
				}
				sort.Slice(results, func(i_, j_ int) bool { return results[i_].score > results[j_].score })
				seen := map[string]bool{}
				var list []model.SearchResultItem
				for _, s := range results {
					if !seen[s.item.ID] {
						seen[s.item.ID] = true
						list = append(list, s.item)
						if len(list) >= limit { break }
					}
				}
				if len(list) > 0 { return list, nil }
			}
		}
	// ILIKE fallback for Chinese text
	words := splitQuery(query)
	// Cap at 10 tokens to avoid SQL explosion; pg_trgm GIN indexes handle ILIKE efficiently
	if len(words) > 10 { words = words[:10] }
	if len(words) == 0 { return []model.SearchResultItem{}, nil }
	scoreParts := make([]string, len(words))
	likeParts := make([]string, len(words))
	args := []any{tenantID, limit}
	argIdx := 3
	for i, w := range words {
		p := fmt.Sprintf("$%d", argIdx)
		argIdx++
		scoreParts[i] = fmt.Sprintf("(CASE WHEN a.title ILIKE '%%%%' || %s || '%%%%' THEN 3 WHEN a.content ILIKE '%%%%' || %s || '%%%%' THEN 2 WHEN a.category ILIKE '%%%%' || %s || '%%%%' THEN 2 WHEN a.attributes ILIKE '%%%%' || %s || '%%%%' THEN 1 ELSE 0 END)", p, p, p, p)
		likeParts[i] = fmt.Sprintf("(a.title ILIKE '%%%%' || %s || '%%%%' OR a.content ILIKE '%%%%' || %s || '%%%%' OR a.category ILIKE '%%%%' || %s || '%%%%' OR a.attributes ILIKE '%%%%' || %s || '%%%%')", p, p, p, p)
		args = append(args, w)
	}
	sql := fmt.Sprintf("SELECT a.id, a.title, a.content, a.category, a.attributes, k.name AS kb_name, (%s) AS score FROM knowledge_articles a JOIN knowledge_bases k ON a.knowledge_base_id = k.id WHERE k.tenant_id=$1 AND a.status='active' AND k.status='active' AND (%s) ORDER BY score DESC LIMIT $2", strings.Join(scoreParts, " + "), strings.Join(likeParts, " AND "))
	rows, err := s.pool.Query(context.Background(), sql, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var list []model.SearchResultItem
	for rows.Next() {
		var item model.SearchResultItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Content, &item.Category, &item.Attributes, &item.KnowledgeBaseName, &item.Score); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	if list == nil { list = []model.SearchResultItem{} }
	return list, rows.Err()
}

// ChunkArticle splits article content into chunks and stores them.
func (s *Store) ChunkArticle( articleID, content string) error {
	s.pool.Exec(context.Background(), "DELETE FROM knowledge_chunks WHERE article_id=$1", articleID)
	chunks := splitContent(content, 500)
	for i, c := range chunks {
		id := genID()
		s.pool.Exec(context.Background(),
			"INSERT INTO knowledge_chunks (id,article_id,content,chunk_index) VALUES ($1,$2,$3,$4)",
			id, articleID, c, i)
	}
	return nil
}

// UpdateChunkEmbedding sets the embedding vector for a chunk.
func (s *Store) UpdateChunkEmbedding( chunkID string, embedding []float32) error {
	b, _ := json.Marshal(embedding)
	_, err := s.pool.Exec(context.Background(),
		"UPDATE knowledge_chunks SET embedding=$1 WHERE id=$2", string(b), chunkID)
	return err
}

// GetChunksWithoutEmbeddings returns chunks that need embedding.
func (s *Store) GetChunksWithoutEmbeddings( tenantID string, limit int) ([]ChunkInfo, error) {
	rows, err := s.pool.Query(context.Background(),
		"SELECT c.id, c.content FROM knowledge_chunks c JOIN knowledge_articles a ON c.article_id = a.id JOIN knowledge_bases k ON a.knowledge_base_id = k.id WHERE k.tenant_id=$1 AND c.embedding IS NULL LIMIT $2",
		tenantID, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	var list []ChunkInfo
	for rows.Next() {
		var item ChunkInfo
		if err := rows.Scan(&item.ID, &item.Content); err != nil { return nil, err }
		list = append(list, item)
	}
	return list, rows.Err()
}

func splitContent(text string, maxLen int) []string {
	if len([]rune(text)) <= maxLen {
		return []string{text}
	}
	var chunks []string
	runes := []rune(text)
	for i := 0; i < len(runes); i += maxLen {
		end := i + maxLen
		if end > len(runes) { end = len(runes) }
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}


func splitQuery(q string) []string {
	sep := func(r rune) bool {
		return r == ' ' || r == '、' || r == '。' || r == '，' || r == '？' || r == '！' || r == '；' || r == '：' ||
			r == ',' || r == '.' || r == '?' || r == '!'
	}
	parts := strings.FieldsFunc(q, sep)
	seen := map[string]bool{}
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) == 0 { continue }
		if !seen[p] { result = append(result, p); seen[p] = true }
		// For Chinese text (>2 chars), add bigrams and unigrams for fuzzy matching
		runes := []rune(p)
		if len(runes) > 2 {
			for i := 0; i < len(runes)-1; i++ {
				bigram := string(runes[i : i+2])
				if !seen[bigram] { result = append(result, bigram); seen[bigram] = true }
			}
			for _, r := range runes {
				ch := string(r)
				if !seen[ch] { result = append(result, ch); seen[ch] = true }
			}
		}
	}
	return result
}

func (s *Store) SaveMessage( tenantID, conversationID, customerName, role, content, knowledgeIDs string) (*model.ConversationMessage, error) {
	m := &model.ConversationMessage{
		ID: genID(), ConversationID: conversationID, CustomerName: customerName,
		Role: role, Content: content, KnowledgeIDs: knowledgeIDs,
	}
	err := s.pool.QueryRow(context.Background(),
		"INSERT INTO conversation_messages (id,tenant_id,conversation_id,customer_name,role,content,knowledge_ids) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')",
		m.ID, tenantID, m.ConversationID, m.CustomerName, m.Role, m.Content, m.KnowledgeIDs,
	).Scan(&m.CreatedAt)
	if err != nil { return nil, err }
	return m, nil
}

func (s *Store) LoadHistory( tenantID, conversationID string, limit int) ([]model.ConversationMessage, error) {
	if limit <= 0 { limit = 20 }
	rows, err := s.pool.Query(context.Background(),
		"SELECT id,conversation_id,customer_name,role,content,knowledge_ids,to_char(created_at, 'YYYY-MM-DD HH24:MI:SS') FROM conversation_messages WHERE tenant_id=$1 AND conversation_id=$2 ORDER BY created_at DESC LIMIT $3",
		tenantID, conversationID, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	var list []model.ConversationMessage
	for rows.Next() {
		var m model.ConversationMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.CustomerName, &m.Role, &m.Content, &m.KnowledgeIDs, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	if list == nil { list = []model.ConversationMessage{} }
	return list, rows.Err()
}

func (s *Store) ListConversationSummaries( tenantID string) ([]model.ConversationSummary, error) {
	rows, err := s.pool.Query(context.Background(),
		"SELECT conversation_id, customer_name, (SELECT content FROM conversation_messages cm2 WHERE cm2.tenant_id=$1 AND cm2.conversation_id=cm.conversation_id ORDER BY created_at DESC LIMIT 1) AS last_message, (SELECT to_char(created_at, 'YYYY-MM-DD HH24:MI:SS') FROM conversation_messages cm3 WHERE cm3.tenant_id=$1 AND cm3.conversation_id=cm.conversation_id ORDER BY created_at DESC LIMIT 1) AS last_message_at, COUNT(*) AS message_count FROM conversation_messages cm WHERE tenant_id=$1 GROUP BY conversation_id, customer_name ORDER BY last_message_at DESC", tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	var list []model.ConversationSummary
	for rows.Next() {
		var s model.ConversationSummary
		var lastAt *string
		if err := rows.Scan(&s.ConversationID, &s.CustomerName, &s.LastMessage, &lastAt, &s.MessageCount); err != nil {
			return nil, err
		}
		if lastAt != nil { s.LastMessageAt = *lastAt }
		list = append(list, s)
	}
	if list == nil { list = []model.ConversationSummary{} }
	return list, rows.Err()
}




func (s *Store) KnowledgeBaseByID( id, tenantID string) (*model.KnowledgeRow, error) {
	k := &model.KnowledgeRow{}
	err := s.pool.QueryRow(context.Background(),
		`SELECT id,tenant_id,name,description,status,
		        to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM knowledge_bases WHERE id=$1 AND tenant_id=$2`, id, tenantID,
	).Scan(&k.ID, &k.TenantID, &k.Name, &k.Description, &k.Status, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return k, nil
}

func (s *Store) UpdateKnowledgeBase( id, tenantID string, name, description, status *string) (*model.KnowledgeRow, error) {
	q := `UPDATE knowledge_bases SET `
	args := []any{}
	idx := 1
	if name != nil { q += fmt.Sprintf(`name=$%d, `, idx); args = append(args, *name); idx++ }
	if description != nil { q += fmt.Sprintf(`description=$%d, `, idx); args = append(args, *description); idx++ }
	if status != nil { q += fmt.Sprintf(`status=$%d, `, idx); args = append(args, *status); idx++ }
	q = q[:len(q)-2] + fmt.Sprintf(` WHERE id=$%d AND tenant_id=$%d RETURNING id,tenant_id,name,description,status,to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')`, idx, idx+1)
	args = append(args, id, tenantID)
	k := &model.KnowledgeRow{}
	err := s.pool.QueryRow(context.Background(), q, args...).Scan(&k.ID, &k.TenantID, &k.Name, &k.Description, &k.Status, &k.CreatedAt)
	if err != nil { return nil, err }
	return k, nil
}

func (s *Store) DeleteKnowledgeBase( id, tenantID string) error {
	_, err := s.pool.Exec(context.Background(), `DELETE FROM knowledge_bases WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	return err
}

func (s *Store) ArticlesByKnowledgeBase( kbID, tenantID string) ([]model.KnowledgeArticle, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT a.id,a.knowledge_base_id,a.title,a.content,a.category,a.attributes,a.status,
		        to_char(a.created_at, 'YYYY-MM-DD HH24:MI:SS'),
		        to_char(a.updated_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM knowledge_articles a
		 JOIN knowledge_bases k ON a.knowledge_base_id = k.id
		 WHERE a.knowledge_base_id=$1 AND k.tenant_id=$2
		 ORDER BY a.created_at`, kbID, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	var list []model.KnowledgeArticle
	for rows.Next() {
		var a model.KnowledgeArticle
		if err := rows.Scan(&a.ID, &a.KnowledgeBaseID, &a.Title, &a.Content, &a.Category, &a.Attributes, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	if list == nil { list = []model.KnowledgeArticle{} }
	return list, rows.Err()
}

func (s *Store) CreateArticle( kbID, title, content, category, attributes string) (*model.KnowledgeArticleRow, error) {
	a := &model.KnowledgeArticleRow{
		ID: genID(), KnowledgeBaseID: kbID, Title: title,
		Content: content, Category: category, Attributes: attributes, Status: "active",
	}
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO knowledge_articles (id,knowledge_base_id,title,content,category,attributes)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS'),
		           to_char(updated_at, 'YYYY-MM-DD HH24:MI:SS')`,
		a.ID, a.KnowledgeBaseID, a.Title, a.Content, a.Category, a.Attributes,
	).Scan(&a.CreatedAt, &a.UpdatedAt)
	if err != nil { return nil, err }
	return a, nil
}

func (s *Store) UpdateArticle( id string, title, content, category, attributes, status *string) (*model.KnowledgeArticleRow, error) {
	q := `UPDATE knowledge_articles SET updated_at=NOW(), `
	args := []any{}
	idx := 1
	if title != nil { q += fmt.Sprintf(`title=$%d, `, idx); args = append(args, *title); idx++ }
	if content != nil { q += fmt.Sprintf(`content=$%d, `, idx); args = append(args, *content); idx++ }
	if category != nil { q += fmt.Sprintf(`category=$%d, `, idx); args = append(args, *category); idx++ }
	if attributes != nil { q += fmt.Sprintf(`attributes=$%d, `, idx); args = append(args, *attributes); idx++ }
	if status != nil { q += fmt.Sprintf(`status=$%d, `, idx); args = append(args, *status); idx++ }
	q = q[:len(q)-2] + fmt.Sprintf(` WHERE id=$%d RETURNING id,knowledge_base_id,title,content,category,attributes,status,to_char(created_at, 'YYYY-MM-DD HH24:MI:SS'),to_char(updated_at, 'YYYY-MM-DD HH24:MI:SS')`, idx)
	args = append(args, id)
	a := &model.KnowledgeArticleRow{}
	err := s.pool.QueryRow(context.Background(), q, args...).Scan(&a.ID, &a.KnowledgeBaseID, &a.Title, &a.Content, &a.Category, &a.Attributes, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil { return nil, err }
	return a, nil
}

func (s *Store) DeleteArticle( id string) error {
	_, err := s.pool.Exec(context.Background(), `DELETE FROM knowledge_articles WHERE id=$1`, id)
	return err
}


func (s *Store) ConversationsByTenant( tenantID string) ([]model.Conversation, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id,account_id,customer,last_message,status,
		        to_char(last_message_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM conversations WHERE tenant_id=$1 ORDER BY last_message_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Conversation
	for rows.Next() {
		var c model.Conversation
		if err := rows.Scan(&c.ID, &c.AccountID, &c.Customer, &c.LastMessage, &c.Status, &c.LastMessageAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 { return 0 }
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 { return 0 }
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

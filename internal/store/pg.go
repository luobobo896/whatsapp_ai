package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/model"
)

// ChunkInfo is used by GetChunksWithoutEmbeddings
type ChunkInfo struct {
	ID      string
	Content string
}

type Store struct{ pool *pgxpool.Pool }

var ErrDailyReplyLimitReached = errors.New("daily reply limit reached")

func dailyReplyLimitReached(limit, count int) bool {
	return limit > 0 && count >= limit
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 20
	cfg.ConnConfig.RuntimeParams["timezone"] = "Asia/Shanghai"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect pg: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping pg: %w", err)
	}
	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() { s.pool.Close() }

func genID() string    { b := make([]byte, 12); rand.Read(b); return hex.EncodeToString(b) }
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
		if _, err := s.pool.Exec(ctx, d); err != nil {
			return fmt.Errorf("ddl: %w\n%s", err, d)
		}
	}
	migrations := []string{
		`CREATE EXTENSION IF NOT EXISTS pg_trgm`,
		`ALTER TABLE knowledge_articles ADD COLUMN IF NOT EXISTS attributes TEXT NOT NULL DEFAULT '{}'`,
		`CREATE INDEX IF NOT EXISTS idx_articles_title_trgm ON knowledge_articles USING GIN (title gin_trgm_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_articles_content_trgm ON knowledge_articles USING GIN (content gin_trgm_ops)`,
		`CREATE INDEX IF NOT EXISTS idx_articles_status ON knowledge_articles(status)`,
		`CREATE INDEX IF NOT EXISTS idx_articles_kbid ON knowledge_articles(knowledge_base_id)`,
		`CREATE INDEX IF NOT EXISTS idx_kb_tenant_status ON knowledge_bases(tenant_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_embedding ON knowledge_chunks(article_id) WHERE embedding IS NOT NULL`,
		`ALTER TABLE knowledge_articles ADD COLUMN IF NOT EXISTS search_vector tsvector`,
		`CREATE INDEX IF NOT EXISTS idx_articles_search ON knowledge_articles USING GIN (search_vector)`,
		`CREATE OR REPLACE FUNCTION articles_search_update() RETURNS trigger AS $$ BEGIN NEW.search_vector := setweight(to_tsvector('simple', COALESCE(NEW.title, '')), 'A') || setweight(to_tsvector('simple', COALESCE(NEW.content, '')), 'B') || setweight(to_tsvector('simple', COALESCE(NEW.category, '')), 'C') || setweight(to_tsvector('simple', COALESCE(NEW.attributes, '')), 'D'); RETURN NEW; END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS trg_articles_search ON knowledge_articles`,
		`CREATE TRIGGER trg_articles_search BEFORE INSERT OR UPDATE ON knowledge_articles FOR EACH ROW EXECUTE FUNCTION articles_search_update()`,
		`UPDATE knowledge_articles SET search_vector = setweight(to_tsvector('simple', COALESCE(title, '')), 'A') || setweight(to_tsvector('simple', COALESCE(content, '')), 'B') || setweight(to_tsvector('simple', COALESCE(category, '')), 'C') || setweight(to_tsvector('simple', COALESCE(attributes, '')), 'D') WHERE search_vector IS NULL`,
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS kb_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN IF NOT EXISTS reply_limit INTEGER NOT NULL DEFAULT 30`,
		`ALTER TABLE conversation_messages ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_conv_msg_account ON conversation_messages(tenant_id, account_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_conv_msg_daily_reply ON conversation_messages(tenant_id, account_id, created_at) WHERE role='assistant'`,
		`ALTER TABLE conversation_messages ADD COLUMN IF NOT EXISTS message_id TEXT`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_conv_msg_dedup ON conversation_messages(tenant_id, conversation_id, message_id) WHERE message_id IS NOT NULL`,
	}
	for _, statement := range migrations {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return fmt.Errorf("migration: %w\n%s", err, statement)
		}
	}
	return nil
}

// ---- users ----

func (s *Store) CreateUser(email, displayName, passwordHash, platformRole string) (*model.UserRow, error) {
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

func (s *Store) UserByEmail(email string) (*model.UserRow, error) {
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

func (s *Store) UserByID(id string) (*model.UserRow, error) {
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

func (s *Store) CreateSession(userID string) (*model.SessionRow, error) {
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

func (s *Store) SessionByID(id string) (*model.SessionRow, error) {
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

func (s *Store) UpdateSessionTenant(sessionID, tenantID string) error {
	_, err := s.pool.Exec(context.Background(), `UPDATE sessions SET active_tenant_id=$1 WHERE id=$2`, tenantID, sessionID)
	return err
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.pool.Exec(context.Background(), `DELETE FROM sessions WHERE id=$1`, id)
	return err
}

// ---- tenants ----

func (s *Store) CreateTenant(name string) (*model.TenantRow, error) {
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

// CreateTenantWithOwner creates the tenant, its first owner, and the creating
// platform administrator's membership in one transaction.
func (s *Store) CreateTenantWithOwner(name, passwordHash, platformAdminID string) (*model.TenantRow, *model.UserRow, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	tenant := &model.TenantRow{ID: genID(), Name: name, Status: "active"}
	if err := tx.QueryRow(ctx,
		`INSERT INTO tenants (id,name) VALUES ($1,$2) RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')`,
		tenant.ID, tenant.Name,
	).Scan(&tenant.CreatedAt); err != nil {
		return nil, nil, err
	}

	owner := &model.UserRow{
		ID:           genID(),
		Email:        fmt.Sprintf("admin@%s.local", tenant.ID[:8]),
		DisplayName:  "管理员",
		PasswordHash: passwordHash,
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO users (id,email,display_name,password_hash,platform_role) VALUES ($1,$2,$3,$4,'') RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')`,
		owner.ID, owner.Email, owner.DisplayName, owner.PasswordHash,
	).Scan(&owner.CreatedAt); err != nil {
		return nil, nil, err
	}

	if _, err := tx.Exec(ctx, `INSERT INTO tenant_members (tenant_id,user_id,role,status) VALUES ($1,$2,'owner','active')`, tenant.ID, owner.ID); err != nil {
		return nil, nil, err
	}
	if platformAdminID != "" && platformAdminID != owner.ID {
		if _, err := tx.Exec(ctx,
			`INSERT INTO tenant_members (tenant_id,user_id,role,status) VALUES ($1,$2,'owner','active') ON CONFLICT (tenant_id,user_id) DO UPDATE SET role='owner', status='active'`,
			tenant.ID, platformAdminID,
		); err != nil {
			return nil, nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return tenant, owner, nil
}

func (s *Store) TenantByID(id string) (*model.TenantRow, error) {
	t := &model.TenantRow{}
	err := s.pool.QueryRow(context.Background(),
		`SELECT id,name,status,to_char(created_at, 'YYYY-MM-DD HH24:MI:SS') FROM tenants WHERE id=$1`, id,
	).Scan(&t.ID, &t.Name, &t.Status, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) UpdateTenantStatus(id, status string) error {
	_, err := s.pool.Exec(context.Background(), `UPDATE tenants SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (s *Store) TenantsForUser(userID string) ([]model.TenantWithMembership, error) {
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

func (s *Store) AddTenantMember(tenantID, userID, role string) error {
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO tenant_members (tenant_id,user_id,role,status) VALUES ($1,$2,$3,'active')
		 ON CONFLICT (tenant_id, user_id) DO UPDATE SET role=$3, status='active'`,
		tenantID, userID, role,
	)
	return err
}

func (s *Store) TenantMember(tenantID, userID string) (*model.TenantMemberRow, error) {
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

func (s *Store) UpdateMember(tenantID, userID, role, status string) error {
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

func (s *Store) TenantMembers(tenantID string) ([]model.Member, error) {
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

func (s *Store) CreateInvitation(tenantID, email, role string) (*model.InvitationRow, error) {
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

func (s *Store) InvitationByToken(token string) (*model.InvitationRow, error) {
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

func (s *Store) DeleteInvitation(id string) error {
	_, err := s.pool.Exec(context.Background(), `DELETE FROM invitations WHERE id=$1`, id)
	return err
}

// AcceptInvitationForUser atomically grants membership, creates a tenant-scoped
// session, and consumes the invitation.
func (s *Store) AcceptInvitationForUser(invitationID, userID string) (*model.SessionRow, string, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx)

	var tenantID, role string
	var expiresAt time.Time
	if err := tx.QueryRow(ctx, `SELECT tenant_id,role,expires_at FROM invitations WHERE id=$1 FOR UPDATE`, invitationID).Scan(&tenantID, &role, &expiresAt); err != nil {
		return nil, "", err
	}
	if time.Now().After(expiresAt) {
		if _, err := tx.Exec(ctx, `DELETE FROM invitations WHERE id=$1`, invitationID); err != nil {
			return nil, "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, "", err
		}
		return nil, "", pgx.ErrNoRows
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO tenant_members (tenant_id,user_id,role,status) VALUES ($1,$2,$3,'active')
		 ON CONFLICT (tenant_id, user_id) DO UPDATE SET role=$3, status='active'`,
		tenantID, userID, role,
	); err != nil {
		return nil, "", err
	}

	sess := &model.SessionRow{
		ID: genToken(), UserID: userID, CSRFToken: genToken(), ActiveTenantID: tenantID,
		ExpiresAt: time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05"),
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO sessions (id,user_id,csrf_token,active_tenant_id,expires_at) VALUES ($1,$2,$3,$4,$5)`,
		sess.ID, sess.UserID, sess.CSRFToken, sess.ActiveTenantID, sess.ExpiresAt,
	); err != nil {
		return nil, "", err
	}
	command, err := tx.Exec(ctx, `DELETE FROM invitations WHERE id=$1`, invitationID)
	if err != nil {
		return nil, "", err
	}
	if command.RowsAffected() == 0 {
		return nil, "", pgx.ErrNoRows
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, "", err
	}
	return sess, tenantID, nil
}

// ---- accounts ----

// AllAccounts returns all accounts across all tenants (internal use only).
func (s *Store) AllAccounts() ([]model.AccountRow, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id,tenant_id,name,account_key,status,daily_limit,kb_id,reply_limit,
		        to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM accounts ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.AccountRow
	for rows.Next() {
		var a model.AccountRow
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Name, &a.AccountKey, &a.Status, &a.DailyLimit, &a.KbID, &a.ReplyLimit, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *Store) AccountsByTenant(tenantID string) ([]model.Account, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT a.id,a.name,a.account_key,a.status,a.daily_limit,
		        (SELECT COUNT(*) FROM conversation_messages cm WHERE cm.tenant_id=$1 AND cm.account_id=a.id AND cm.role='assistant' AND cm.created_at >= date_trunc('day', CURRENT_TIMESTAMP)),
		        a.kb_id,a.reply_limit,
		        to_char(a.created_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM accounts a WHERE a.tenant_id=$1 ORDER BY a.created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Account
	for rows.Next() {
		var a model.Account
		var kbIDRaw string
		if err := rows.Scan(&a.ID, &a.Name, &a.AccountKey, &a.Status, &a.DailyLimit, &a.DailyReplies, &kbIDRaw, &a.ReplyLimit, &a.CreatedAt); err != nil {
			return nil, err
		}
		if kbIDRaw != "" {
			json.Unmarshal([]byte(kbIDRaw), &a.KbID)
		}
		if a.KbID == nil {
			a.KbID = []string{}
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *Store) CreateAccount(tenantID, name, kbID string, dailyLimit, replyLimit int) (*model.AccountRow, error) {
	a := &model.AccountRow{
		ID: genID(), TenantID: tenantID, Name: name, KbID: kbID,
		AccountKey: "wa_" + genID()[:8], Status: "pending", DailyLimit: dailyLimit, ReplyLimit: replyLimit,
	}
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO accounts (id,tenant_id,name,account_key,status,daily_limit,kb_id,reply_limit)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')`,
		a.ID, a.TenantID, a.Name, a.AccountKey, a.Status, a.DailyLimit, a.KbID, a.ReplyLimit,
	).Scan(&a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) AccountByID(tenantID, accountID string) (*model.AccountRow, error) {
	a := &model.AccountRow{}
	err := s.pool.QueryRow(context.Background(),
		`SELECT id,tenant_id,name,account_key,status,daily_limit,kb_id,reply_limit,
		        to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM accounts WHERE id=$1 AND tenant_id=$2`, accountID, tenantID,
	).Scan(&a.ID, &a.TenantID, &a.Name, &a.AccountKey, &a.Status, &a.DailyLimit, &a.KbID, &a.ReplyLimit, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// TenantIDByAccountID returns the tenant that owns the given account.
func (s *Store) TenantIDByAccountID(accountID string) (string, error) {
	var tenantID string
	err := s.pool.QueryRow(context.Background(),
		`SELECT tenant_id FROM accounts WHERE id=$1`, accountID,
	).Scan(&tenantID)
	return tenantID, err
}

// ActiveTenantIDByAccountID resolves an account only while both its tenant and
// the account itself are enabled for service-to-service traffic.
func (s *Store) ActiveTenantIDByAccountID(accountID string) (string, error) {
	var tenantID string
	err := s.pool.QueryRow(context.Background(), `
		SELECT a.tenant_id
		FROM accounts a
		JOIN tenants t ON t.id=a.tenant_id
		WHERE a.id=$1 AND a.status <> 'disabled' AND t.status='active'`, accountID,
	).Scan(&tenantID)
	return tenantID, err
}

func (s *Store) UpdateAccount(tenantID, accountID, name, status string, kbID *string, dailyLimit, replyLimit *int) (*model.AccountRow, error) {
	q := `UPDATE accounts SET `
	args := []any{}
	idx := 1
	if name != "" {
		q += fmt.Sprintf(`name=$%d, `, idx)
		args = append(args, name)
		idx++
	}
	if kbID != nil {
		q += fmt.Sprintf(`kb_id=$%d, `, idx)
		args = append(args, *kbID)
		idx++
	}
	if status != "" {
		q += fmt.Sprintf(`status=$%d, `, idx)
		args = append(args, status)
		idx++
	}
	if dailyLimit != nil {
		q += fmt.Sprintf(`daily_limit=$%d, `, idx)
		args = append(args, *dailyLimit)
		idx++
	}
	if replyLimit != nil {
		q += fmt.Sprintf(`reply_limit=$%d, `, idx)
		args = append(args, *replyLimit)
		idx++
	}
	q = q[:len(q)-2] + fmt.Sprintf(` WHERE id=$%d AND tenant_id=$%d RETURNING id,tenant_id,name,account_key,status,daily_limit,kb_id,reply_limit,to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')`, idx, idx+1)
	args = append(args, accountID, tenantID)
	a := &model.AccountRow{}
	err := s.pool.QueryRow(context.Background(), q, args...).Scan(&a.ID, &a.TenantID, &a.Name, &a.AccountKey, &a.Status, &a.DailyLimit, &a.KbID, &a.ReplyLimit, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

type accountDeletionExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func deleteAccountRows(ctx context.Context, tx accountDeletionExecutor, tenantID, accountID string) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM conversation_messages WHERE tenant_id=$1 AND account_id=$2`,
		tenantID, accountID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM conversations WHERE tenant_id=$1 AND account_id=$2`,
		tenantID, accountID); err != nil {
		return err
	}
	command, err := tx.Exec(ctx,
		`DELETE FROM accounts WHERE id=$1 AND tenant_id=$2`,
		accountID, tenantID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteAccount(tenantID, accountID string) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := deleteAccountRows(ctx, tx, tenantID, accountID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---- knowledge bases ----

func (s *Store) KnowledgeBasesByTenant(tenantID string) ([]model.KnowledgeBase, error) {
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

func (s *Store) CreateKnowledgeBase(tenantID, name, description string) (*model.KnowledgeRow, error) {
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

func (s *Store) SearchKnowledge(tenantID, query string, embedding []float32, limit int) ([]model.SearchResultItem, error) {
	return s.searchKnowledge(tenantID, nil, query, embedding, limit)
}

// SearchKnowledgeForBases limits a search to the account's explicitly bound
// knowledge bases. An account without bindings cannot read tenant-wide content.
func (s *Store) SearchKnowledgeForBases(tenantID string, baseIDs []string, query string, embedding []float32, limit int) ([]model.SearchResultItem, error) {
	if len(baseIDs) == 0 {
		return []model.SearchResultItem{}, nil
	}
	return s.searchKnowledge(tenantID, baseIDs, query, embedding, limit)
}

func (s *Store) searchKnowledge(tenantID string, baseIDs []string, query string, embedding []float32, limit int) ([]model.SearchResultItem, error) {
	if limit <= 0 {
		limit = 5
	}
	// Vector search via Go cosine similarity if embedding provided.
	if len(embedding) > 0 {
		vectorArgs := []any{tenantID}
		baseFilter := ""
		argIdx := 2
		if len(baseIDs) > 0 {
			baseFilter = fmt.Sprintf(" AND k.id = ANY($%d)", argIdx)
			vectorArgs = append(vectorArgs, baseIDs)
			argIdx++
		}
		// Cap the candidate set so we do not pull every chunk+embedding for the
		// tenant into Go heap. For very large corpora a server-side ANN/pgvector
		// index would be the right long-term fix.
		const vectorCandidateLimit = 500
		vectorArgs = append(vectorArgs, vectorCandidateLimit)
		limitArgIdx := argIdx
		rows, err := s.pool.Query(context.Background(),
			"SELECT a.id, a.title, a.content, a.category, a.attributes, k.name AS kb_name, c.id, c.embedding FROM knowledge_chunks c JOIN knowledge_articles a ON c.article_id = a.id JOIN knowledge_bases k ON a.knowledge_base_id = k.id WHERE k.tenant_id=$1 AND a.status='active' AND k.status='active' AND c.embedding IS NOT NULL"+baseFilter+fmt.Sprintf(" LIMIT $%d", limitArgIdx), vectorArgs...)
		if err != nil {
			slog.Warn("knowledge vector query failed; falling back to ILIKE", "tenant_id", tenantID, "err", err)
		} else {
			type row struct {
				artID, title, content, category, attrs, kbName, chunkID, emb string
			}
			var chunks []row
			for rows.Next() {
				var r row
				if err := rows.Scan(&r.artID, &r.title, &r.content, &r.category, &r.attrs, &r.kbName, &r.chunkID, &r.emb); err != nil {
					slog.Warn("knowledge vector row scan failed; skipping chunk", "err", err)
					continue
				}
				chunks = append(chunks, r)
			}
			rows.Close()
			queryVec := make([]float64, len(embedding))
			for i, v := range embedding {
				queryVec[i] = float64(v)
			}
			// Drop near-zero matches; cosine similarity below this threshold is
			// treated as "no relevant knowledge" so the caller falls back to ILIKE
			// or the canned "I don't know" reply instead of surfacing noise.
			const minVectorScore = 0.2
			type scored struct {
				item  model.SearchResultItem
				score float64
			}
			var results []scored
			for _, ck := range chunks {
				var embVec []float64
				if json.Unmarshal([]byte(ck.emb), &embVec) != nil || len(embVec) == 0 {
					continue
				}
				if len(embVec) != len(queryVec) {
					slog.Warn("embedding dimension mismatch; skipping chunk", "chunk_id", ck.chunkID, "want", len(queryVec), "got", len(embVec))
					continue
				}
				sim := cosineSimilarity(queryVec, embVec)
				if sim < minVectorScore {
					continue
				}
				results = append(results, scored{
					item:  model.SearchResultItem{ID: ck.artID, Title: ck.title, Content: ck.content, Category: ck.category, Attributes: ck.attrs, KnowledgeBaseName: ck.kbName, Score: sim},
					score: sim,
				})
			}
			sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })
			seen := map[string]bool{}
			var list []model.SearchResultItem
			for _, s := range results {
				if !seen[s.item.ID] {
					seen[s.item.ID] = true
					list = append(list, s.item)
					if len(list) >= limit {
						break
					}
				}
			}
			if len(list) > 0 {
				return list, nil
			}
		}
	}
	// ILIKE fallback for Chinese text
	words := splitQuery(query)
	// Cap at 10 tokens to avoid SQL explosion; pg_trgm GIN indexes handle ILIKE efficiently
	if len(words) > 10 {
		words = words[:10]
	}
	if len(words) == 0 {
		return []model.SearchResultItem{}, nil
	}
	scoreParts := make([]string, len(words))
	args := []any{tenantID, limit}
	baseFilter := ""
	argIdx := 3
	if len(baseIDs) > 0 {
		baseFilter = fmt.Sprintf(" AND k.id = ANY($%d)", argIdx)
		args = append(args, baseIDs)
		argIdx++
	}
	for i, w := range words {
		p := fmt.Sprintf("$%d", argIdx)
		argIdx++
		scoreParts[i] = fmt.Sprintf("(CASE WHEN a.title ILIKE '%%%%' || %s || '%%%%' THEN 3 WHEN a.content ILIKE '%%%%' || %s || '%%%%' THEN 2 WHEN a.category ILIKE '%%%%' || %s || '%%%%' THEN 2 WHEN a.attributes ILIKE '%%%%' || %s || '%%%%' THEN 1 ELSE 0 END)", p, p, p, p)
		args = append(args, escapeILIKEPattern(w))
	}
	minimumScore := 2
	if len(words) > 1 {
		minimumScore = 4
	}
	scoreExpression := strings.Join(scoreParts, " + ")
	sql := fmt.Sprintf("SELECT a.id, a.title, a.content, a.category, a.attributes, k.name AS kb_name, (%s) AS score FROM knowledge_articles a JOIN knowledge_bases k ON a.knowledge_base_id = k.id WHERE k.tenant_id=$1 AND a.status='active' AND k.status='active'%s AND ((%s) >= %d) ORDER BY score DESC LIMIT $2", scoreExpression, baseFilter, scoreExpression, minimumScore)
	rows, err := s.pool.Query(context.Background(), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.SearchResultItem
	for rows.Next() {
		var item model.SearchResultItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Content, &item.Category, &item.Attributes, &item.KnowledgeBaseName, &item.Score); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	if list == nil {
		list = []model.SearchResultItem{}
	}
	return list, rows.Err()
}

// maxArticleRunes caps the amount of content chunkArticleTx will process in
// a single transaction. Articles larger than this are truncated so one big
// article cannot hold a long-open transaction with thousands of chunk INSERTs.
const maxArticleRunes = 50000

func chunkArticleTx(ctx context.Context, tx pgx.Tx, articleID, content string) error {
	// Bound per-article work; callers that need full indexing for larger docs
	// should split the input themselves.
	if r := []rune(content); len(r) > maxArticleRunes {
		content = string(r[:maxArticleRunes])
	}
	chunks := splitContent(content, 500)
	// Preserve embeddings of unchanged chunks (keyed by exact content) so that
	// editing title/category (or minor content tweaks) does not zero out the
	// article's vector index and silently take it out of retrieval until the
	// background embedder catches up.
	existingRows, err := tx.Query(ctx,
		"SELECT content, embedding FROM knowledge_chunks WHERE article_id=$1", articleID)
	if err != nil {
		return err
	}
	preservedEmbeddings := map[string]string{}
	for existingRows.Next() {
		var c string
		var emb *string
		if err := existingRows.Scan(&c, &emb); err != nil {
			existingRows.Close()
			return err
		}
		if emb != nil && *emb != "" {
			preservedEmbeddings[c] = *emb
		}
	}
	existingRows.Close()
	if err := existingRows.Err(); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM knowledge_chunks WHERE article_id=$1", articleID); err != nil {
		return err
	}
	for i, c := range chunks {
		var emb any
		if v, ok := preservedEmbeddings[c]; ok {
			emb = v
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO knowledge_chunks (id,article_id,content,chunk_index,embedding) VALUES ($1,$2,$3,$4,$5)",
			genID(), articleID, c, i, emb); err != nil {
			return err
		}
	}
	return nil
}

// ChunkArticle splits article content into chunks and stores them atomically.
func (s *Store) ChunkArticle(articleID, content string) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := chunkArticleTx(ctx, tx, articleID, content); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UpdateChunkEmbedding sets the embedding vector for a chunk.
func (s *Store) UpdateChunkEmbedding(chunkID string, embedding []float32) error {
	b, err := json.Marshal(embedding)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(),
		"UPDATE knowledge_chunks SET embedding=$1 WHERE id=$2", string(b), chunkID)
	return err
}

// GetChunksWithoutEmbeddings returns chunks that need embedding.
func (s *Store) GetChunksWithoutEmbeddings(tenantID string, limit int) ([]ChunkInfo, error) {
	rows, err := s.pool.Query(context.Background(),
		"SELECT c.id, c.content FROM knowledge_chunks c JOIN knowledge_articles a ON c.article_id = a.id JOIN knowledge_bases k ON a.knowledge_base_id = k.id WHERE k.tenant_id=$1 AND c.embedding IS NULL LIMIT $2",
		tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ChunkInfo
	for rows.Next() {
		var item ChunkInfo
		if err := rows.Scan(&item.ID, &item.Content); err != nil {
			return nil, err
		}
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
		if end > len(runes) {
			end = len(runes)
		}
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
		if len(p) == 0 {
			continue
		}
		if !seen[p] {
			result = append(result, p)
			seen[p] = true
		}
		// For Chinese text (>2 chars), add bigrams for fuzzy matching.
		runes := []rune(p)
		hasHan := false
		for _, r := range runes {
			if unicode.Is(unicode.Han, r) {
				hasHan = true
				break
			}
		}
		if len(runes) > 2 && hasHan {
			for i := 0; i < len(runes)-1; i++ {
				bigram := string(runes[i : i+2])
				if !seen[bigram] {
					result = append(result, bigram)
					seen[bigram] = true
				}
			}
		}
	}
	return result
}

func (s *Store) SaveMessage(tenantID, accountID, conversationID, customerName, role, content, knowledgeIDs string) (*model.ConversationMessage, error) {
	m := &model.ConversationMessage{
		ID: genID(), ConversationID: conversationID, CustomerName: customerName,
		Role: role, Content: content, KnowledgeIDs: knowledgeIDs,
	}
	err := s.pool.QueryRow(context.Background(),
		"INSERT INTO conversation_messages (id,tenant_id,account_id,conversation_id,customer_name,role,content,knowledge_ids) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')",
		m.ID, tenantID, accountID, m.ConversationID, m.CustomerName, m.Role, m.Content, m.KnowledgeIDs,
	).Scan(&m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Store) SaveAssistantReply(tenantID, accountID, conversationID, customerName, content, knowledgeIDs string) (*model.ConversationMessage, error) {
	return s.saveAssistantReply(tenantID, accountID, conversationID, customerName, content, knowledgeIDs, nil)
}

// DeliverAndSaveAssistantReply reserves daily quota and persists the reply in
// a short transaction, then invokes deliver() to send the WhatsApp message
// AFTER the transaction commits. If deliver fails, the reserved row is deleted
// so the daily-limit counter stays accurate. Signature is preserved so the
// handler call site is unchanged.
func (s *Store) DeliverAndSaveAssistantReply(tenantID, accountID, conversationID, customerName, content, knowledgeIDs string, deliver func() error) (*model.ConversationMessage, error) {
	return s.saveAssistantReply(tenantID, accountID, conversationID, customerName, content, knowledgeIDs, deliver)
}

// saveAssistantReply atomically reserves daily quota and persists the reply,
// then optionally delivers it OUTSIDE the database transaction. Running
// deliver() after commit is required: the previous implementation invoked
// deliver() between the account lock and INSERT/commit, so a commit failure
// after a successful WhatsApp send produced duplicate sends plus an
// off-by-N daily limit counter.
//
// Sequence:
//  1. tx A (short, account row lock): SELECT daily_limit + COUNT today + INSERT message + commit.
//     The committed message is what subsequent callers count against the limit.
//  2. deliver() runs with no DB lock held.
//  3. On deliver failure, delete the reserved row to release the quota. If the
//     delete also fails the quota stays consumed (fail-closed for rate limit,
//     still no duplicate WhatsApp send).
func (s *Store) saveAssistantReply(tenantID, accountID, conversationID, customerName, content, knowledgeIDs string, deliver func() error) (*model.ConversationMessage, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var dailyLimit int
	if err := tx.QueryRow(ctx, `SELECT daily_limit FROM accounts WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, accountID, tenantID).Scan(&dailyLimit); err != nil {
		return nil, err
	}
	if dailyLimit > 0 {
		var repliesToday int
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM conversation_messages WHERE tenant_id=$1 AND account_id=$2 AND role='assistant' AND created_at >= date_trunc('day', CURRENT_TIMESTAMP)`,
			tenantID, accountID,
		).Scan(&repliesToday); err != nil {
			return nil, err
		}
		if dailyReplyLimitReached(dailyLimit, repliesToday) {
			return nil, ErrDailyReplyLimitReached
		}
	}

	m := &model.ConversationMessage{
		ID: genID(), ConversationID: conversationID, CustomerName: customerName,
		Role: "assistant", Content: content, KnowledgeIDs: knowledgeIDs,
	}
	if err := tx.QueryRow(ctx,
		"INSERT INTO conversation_messages (id,tenant_id,account_id,conversation_id,customer_name,role,content,knowledge_ids) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')",
		m.ID, tenantID, accountID, m.ConversationID, m.CustomerName, m.Role, m.Content, m.KnowledgeIDs,
	).Scan(&m.CreatedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	if deliver != nil {
		if derr := deliver(); derr != nil {
			if delErr := s.deleteMessageByID(ctx, m.ID); delErr != nil {
				slog.Warn("failed to release undelivered assistant reply quota",
					"message_id", m.ID, "tenant_id", tenantID, "account_id", accountID, "err", delErr)
			}
			return nil, derr
		}
	}
	return m, nil
}

// deleteMessageByID removes a conversation_messages row by primary key. Used by
// saveAssistantReply to release the daily-quota slot reserved for an assistant
// reply whose out-of-transaction delivery failed.
func (s *Store) deleteMessageByID(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM conversation_messages WHERE id=$1", id)
	return err
}

// SaveMessageIfAbsent saves a message idempotently. The previous SELECT-then-
// INSERT had a TOCTOU race and the trailing ON CONFLICT DO NOTHING never matched
// because the table had no unique constraint to conflict on. We now derive a
// deterministic message_id from (role, content) and rely on the partial unique
// index idx_conv_msg_dedup to collapse concurrent/retried calls atomically.
//
// Trade-off: identical (role, content) within the same conversation collapse to
// one row, so legitimate back-to-back duplicate customer messages are deduped
// too. A future API can take an explicit external idempotency key to lift this
// restriction; the index already supports it via the message_id column.
func (s *Store) SaveMessageIfAbsent(tenantID, accountID, conversationID, customerName, role, content, knowledgeIDs string) error {
	messageID := dedupMessageID(role, content)
	_, err := s.pool.Exec(context.Background(),
		"INSERT INTO conversation_messages (id,tenant_id,account_id,conversation_id,customer_name,role,content,knowledge_ids,message_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (tenant_id, conversation_id, message_id) WHERE message_id IS NOT NULL DO NOTHING",
		genID(), tenantID, accountID, conversationID, customerName, role, content, knowledgeIDs, messageID,
	)
	return err
}

// dedupMessageID derives a deterministic idempotency key for SaveMessageIfAbsent
// from role+content. It is deliberately content-derived so retries of the exact
// same request collapse; callers with a true external idempotency key should
// populate message_id directly in a future API extension.
func dedupMessageID(role, content string) string {
	h := sha256.Sum256([]byte(role + "\x00" + content))
	return hex.EncodeToString(h[:16])
}

// escapeILIKEPattern escapes user-supplied text so % and _ are treated as
// literals when concatenated into an ILIKE pattern. Backslash is the default
// ESCAPE character in PostgreSQL's ILIKE, so it must be escaped first.
func escapeILIKEPattern(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

func (s *Store) LoadHistory(tenantID, accountID, conversationID string, limit int) ([]model.ConversationMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(context.Background(),
		"SELECT id,conversation_id,account_id,customer_name,role,content,knowledge_ids,to_char(created_at, 'YYYY-MM-DD HH24:MI:SS') FROM conversation_messages WHERE tenant_id=$1 AND account_id=$2 AND conversation_id=$3 ORDER BY created_at DESC, id LIMIT $4",
		tenantID, accountID, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.ConversationMessage
	for rows.Next() {
		var m model.ConversationMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.AccountID, &m.CustomerName, &m.Role, &m.Content, &m.KnowledgeIDs, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	if list == nil {
		list = []model.ConversationMessage{}
	}
	return list, rows.Err()
}

// ListConversationSummaries returns a page of conversation summaries. limit is
// clamped by the caller (handler) to a sane max; offset is 0-based. Without the
// LIMIT the previous implementation streamed the whole tenant's conversations,
// which degraded as data accumulated.
func (s *Store) ListConversationSummaries(tenantID, accountID string, limit, offset int) ([]model.ConversationSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := `SELECT cm.conversation_id, (SELECT customer_name FROM conversation_messages cm4 WHERE cm4.tenant_id=$1 AND cm4.account_id=cm.account_id AND cm4.conversation_id=cm.conversation_id ORDER BY created_at DESC LIMIT 1) AS customer_name, cm.account_id, (SELECT content FROM conversation_messages cm2 WHERE cm2.tenant_id=$1 AND cm2.account_id=cm.account_id AND cm2.conversation_id=cm.conversation_id ORDER BY created_at DESC LIMIT 1) AS last_message, (SELECT to_char(created_at, 'YYYY-MM-DD HH24:MI:SS') FROM conversation_messages cm3 WHERE cm3.tenant_id=$1 AND cm3.account_id=cm.account_id AND cm3.conversation_id=cm.conversation_id ORDER BY created_at DESC LIMIT 1) AS last_message_at, COUNT(*) AS message_count FROM conversation_messages cm WHERE tenant_id=$1`
	args := []any{tenantID}
	limitArg := 2
	if accountID != "" {
		q += ` AND cm.account_id=$2`
		args = append(args, accountID)
		limitArg = 3
	}
	offsetArg := limitArg + 1
	args = append(args, limit, offset)
	q += fmt.Sprintf(` GROUP BY cm.conversation_id, cm.account_id ORDER BY last_message_at DESC, cm.conversation_id LIMIT $%d OFFSET $%d`, limitArg, offsetArg)
	rows, err := s.pool.Query(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.ConversationSummary
	for rows.Next() {
		var s model.ConversationSummary
		var lastAt *string
		if err := rows.Scan(&s.ConversationID, &s.CustomerName, &s.AccountID, &s.LastMessage, &lastAt, &s.MessageCount); err != nil {
			return nil, err
		}
		if lastAt != nil {
			s.LastMessageAt = *lastAt
		}
		list = append(list, s)
	}
	if list == nil {
		list = []model.ConversationSummary{}
	}
	return list, rows.Err()
}

func (s *Store) DeleteConversation(tenantID, accountID, conversationID string) error {
	_, err := s.pool.Exec(context.Background(),
		"DELETE FROM conversation_messages WHERE tenant_id=$1 AND account_id=$2 AND conversation_id=$3",
		tenantID, accountID, conversationID)
	return err
}

func (s *Store) KnowledgeBaseByID(id, tenantID string) (*model.KnowledgeRow, error) {
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

func (s *Store) UpdateKnowledgeBase(id, tenantID string, name, description, status *string) (*model.KnowledgeRow, error) {
	q := `UPDATE knowledge_bases SET `
	args := []any{}
	idx := 1
	if name != nil {
		q += fmt.Sprintf(`name=$%d, `, idx)
		args = append(args, *name)
		idx++
	}
	if description != nil {
		q += fmt.Sprintf(`description=$%d, `, idx)
		args = append(args, *description)
		idx++
	}
	if status != nil {
		q += fmt.Sprintf(`status=$%d, `, idx)
		args = append(args, *status)
		idx++
	}
	q = q[:len(q)-2] + fmt.Sprintf(` WHERE id=$%d AND tenant_id=$%d RETURNING id,tenant_id,name,description,status,to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')`, idx, idx+1)
	args = append(args, id, tenantID)
	k := &model.KnowledgeRow{}
	err := s.pool.QueryRow(context.Background(), q, args...).Scan(&k.ID, &k.TenantID, &k.Name, &k.Description, &k.Status, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return k, nil
}

func (s *Store) DeleteKnowledgeBase(id, tenantID string) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM knowledge_chunks c USING knowledge_articles a, knowledge_bases k WHERE c.article_id=a.id AND a.knowledge_base_id=k.id AND k.id=$1 AND k.tenant_id=$2`, id, tenantID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM knowledge_articles a USING knowledge_bases k WHERE a.knowledge_base_id=k.id AND k.id=$1 AND k.tenant_id=$2`, id, tenantID); err != nil {
		return err
	}
	command, err := tx.Exec(ctx, `DELETE FROM knowledge_bases WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}

func (s *Store) ArticlesByKnowledgeBase(kbID, tenantID string) ([]model.KnowledgeArticle, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT a.id,a.knowledge_base_id,a.title,a.content,a.category,a.attributes,a.status,
		        to_char(a.created_at, 'YYYY-MM-DD HH24:MI:SS'),
		        to_char(a.updated_at, 'YYYY-MM-DD HH24:MI:SS')
		 FROM knowledge_articles a
		 JOIN knowledge_bases k ON a.knowledge_base_id = k.id
		 WHERE a.knowledge_base_id=$1 AND k.tenant_id=$2
		 ORDER BY a.created_at`, kbID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.KnowledgeArticle
	for rows.Next() {
		var a model.KnowledgeArticle
		if err := rows.Scan(&a.ID, &a.KnowledgeBaseID, &a.Title, &a.Content, &a.Category, &a.Attributes, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	if list == nil {
		list = []model.KnowledgeArticle{}
	}
	return list, rows.Err()
}

func (s *Store) CreateArticle(kbID, title, content, category, attributes string) (*model.KnowledgeArticleRow, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	a := &model.KnowledgeArticleRow{
		ID: genID(), KnowledgeBaseID: kbID, Title: title,
		Content: content, Category: category, Attributes: attributes, Status: "active",
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO knowledge_articles (id,knowledge_base_id,title,content,category,attributes)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING to_char(created_at, 'YYYY-MM-DD HH24:MI:SS'),
		           to_char(updated_at, 'YYYY-MM-DD HH24:MI:SS')`,
		a.ID, a.KnowledgeBaseID, a.Title, a.Content, a.Category, a.Attributes,
	).Scan(&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := chunkArticleTx(ctx, tx, a.ID, content); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

// CreateArticles imports a fully validated batch in sub-batches of 50 articles
// per transaction. The previous version held a single transaction open for the
// whole import (N articles × M chunk INSERTs each), which on large uploads
// produced long-running transactions that bloated WAL, held row locks, and
// degraded replication. Trade-off: a mid-batch failure now leaves the prior
// sub-batch committed; callers that need all-or-nothing semantics should
// validate before calling.
func (s *Store) CreateArticles(kbID string, articles []model.CreateArticleRequest) error {
	if len(articles) == 0 {
		return nil
	}
	const batchSize = 50
	ctx := context.Background()
	for start := 0; start < len(articles); start += batchSize {
		end := start + batchSize
		if end > len(articles) {
			end = len(articles)
		}
		if err := s.createArticlesBatch(ctx, kbID, articles[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) createArticlesBatch(ctx context.Context, kbID string, articles []model.CreateArticleRequest) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, article := range articles {
		id := genID()
		if _, err := tx.Exec(ctx,
			`INSERT INTO knowledge_articles (id,knowledge_base_id,title,content,category,attributes)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			id, kbID, article.Title, article.Content, article.Category, article.Attributes,
		); err != nil {
			return err
		}
		if err := chunkArticleTx(ctx, tx, id, article.Content); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) UpdateArticle(id, kbID, tenantID string, title, content, category, attributes, status *string) (*model.KnowledgeArticleRow, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := `UPDATE knowledge_articles SET updated_at=NOW(), `
	args := []any{}
	idx := 1
	if title != nil {
		q += fmt.Sprintf(`title=$%d, `, idx)
		args = append(args, *title)
		idx++
	}
	if content != nil {
		q += fmt.Sprintf(`content=$%d, `, idx)
		args = append(args, *content)
		idx++
	}
	if category != nil {
		q += fmt.Sprintf(`category=$%d, `, idx)
		args = append(args, *category)
		idx++
	}
	if attributes != nil {
		q += fmt.Sprintf(`attributes=$%d, `, idx)
		args = append(args, *attributes)
		idx++
	}
	if status != nil {
		q += fmt.Sprintf(`status=$%d, `, idx)
		args = append(args, *status)
		idx++
	}
	q = q[:len(q)-2] + fmt.Sprintf(` WHERE id=$%d AND knowledge_base_id=$%d AND EXISTS (SELECT 1 FROM knowledge_bases WHERE id=$%d AND tenant_id=$%d) RETURNING id,knowledge_base_id,title,content,category,attributes,status,to_char(created_at, 'YYYY-MM-DD HH24:MI:SS'),to_char(updated_at, 'YYYY-MM-DD HH24:MI:SS')`, idx, idx+1, idx+1, idx+2)
	args = append(args, id, kbID, tenantID)
	a := &model.KnowledgeArticleRow{}
	err = tx.QueryRow(ctx, q, args...).Scan(&a.ID, &a.KnowledgeBaseID, &a.Title, &a.Content, &a.Category, &a.Attributes, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if content != nil {
		if err := chunkArticleTx(ctx, tx, a.ID, *content); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) DeleteArticle(id, kbID, tenantID string) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM knowledge_chunks c USING knowledge_articles a, knowledge_bases k WHERE c.article_id=a.id AND a.id=$1 AND a.knowledge_base_id=$2 AND k.id=a.knowledge_base_id AND k.tenant_id=$3`, id, kbID, tenantID); err != nil {
		return err
	}
	command, err := tx.Exec(ctx, `DELETE FROM knowledge_articles a USING knowledge_bases k WHERE a.id=$1 AND a.knowledge_base_id=$2 AND k.id=a.knowledge_base_id AND k.tenant_id=$3`, id, kbID, tenantID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}

func (s *Store) ConversationsByTenant(tenantID string) ([]model.Conversation, error) {
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
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

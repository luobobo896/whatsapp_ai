package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"whatsapp-ai-poc/internal/model"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func genID() string  { b := make([]byte, 12); rand.Read(b); return hex.EncodeToString(b) }
func genToken() string { b := make([]byte, 32); rand.Read(b); return hex.EncodeToString(b) }

// ---- migrations ----

func (s *Store) migrate() error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY, email TEXT UNIQUE NOT NULL, display_name TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL, platform_role TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_members (
			tenant_id TEXT NOT NULL, user_id TEXT NOT NULL, role TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			PRIMARY KEY (tenant_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, csrf_token TEXT NOT NULL,
			active_tenant_id TEXT, expires_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS invitations (
			id TEXT PRIMARY KEY, token TEXT UNIQUE NOT NULL, tenant_id TEXT NOT NULL,
			email TEXT NOT NULL, role TEXT NOT NULL,
			expires_at TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, name TEXT NOT NULL,
			account_key TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
			daily_limit INTEGER NOT NULL DEFAULT 30,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS knowledge_bases (
			id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, account_id TEXT NOT NULL,
			customer TEXT NOT NULL, last_message TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			last_message_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
	}
	for _, d := range ddl {
		if _, err := s.db.Exec(d); err != nil {
			return err
		}
	}
	return nil
}

// ---- users ----

func (s *Store) CreateUser(email, displayName, passwordHash, platformRole string) (*model.UserRow, error) {
	u := &model.UserRow{
		ID: genID(), Email: email, DisplayName: displayName,
		PasswordHash: passwordHash, PlatformRole: platformRole,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	_, err := s.db.Exec(
		`INSERT INTO users (id,email,display_name,password_hash,platform_role) VALUES (?,?,?,?,?)`,
		u.ID, u.Email, u.DisplayName, u.PasswordHash, u.PlatformRole,
	)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) UserByEmail(email string) (*model.UserRow, error) {
	u := &model.UserRow{}
	err := s.db.QueryRow(
		`SELECT id,email,display_name,password_hash,platform_role,created_at FROM users WHERE email=?`,
		email,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.PlatformRole, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) UserByID(id string) (*model.UserRow, error) {
	u := &model.UserRow{}
	err := s.db.QueryRow(
		`SELECT id,email,display_name,password_hash,platform_role,created_at FROM users WHERE id=?`,
		id,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.PlatformRole, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ---- sessions ----

func (s *Store) CreateSession(userID string) (*model.SessionRow, error) {
	sess := &model.SessionRow{
		ID:        genToken(),
		UserID:    userID,
		CSRFToken: genToken(),
		ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (id,user_id,csrf_token,active_tenant_id,expires_at) VALUES (?,?,?,NULL,?)`,
		sess.ID, sess.UserID, sess.CSRFToken, sess.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) SessionByID(id string) (*model.SessionRow, error) {
	sess := &model.SessionRow{}
	var activeTenant sql.NullString
	err := s.db.QueryRow(
		`SELECT id,user_id,csrf_token,active_tenant_id,expires_at FROM sessions WHERE id=?`,
		id,
	).Scan(&sess.ID, &sess.UserID, &sess.CSRFToken, &activeTenant, &sess.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if activeTenant.Valid {
		sess.ActiveTenantID = activeTenant.String
	}
	if expires, err := time.Parse(time.RFC3339, sess.ExpiresAt); err != nil || time.Now().After(expires) {
		s.db.Exec(`DELETE FROM sessions WHERE id=?`, id)
		return nil, fmt.Errorf("session expired")
	}
	return sess, nil
}

func (s *Store) UpdateSessionTenant(sessionID, tenantID string) error {
	_, err := s.db.Exec(`UPDATE sessions SET active_tenant_id=? WHERE id=?`, tenantID, sessionID)
	return err
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id=?`, id)
	return err
}

// ---- tenants ----

func (s *Store) CreateTenant(name string) (*model.TenantRow, error) {
	t := &model.TenantRow{ID: genID(), Name: name, Status: "active", CreatedAt: time.Now().Format(time.RFC3339)}
	_, err := s.db.Exec(`INSERT INTO tenants (id,name,status) VALUES (?,?,?)`, t.ID, t.Name, t.Status)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) TenantByID(id string) (*model.TenantRow, error) {
	t := &model.TenantRow{}
	err := s.db.QueryRow(`SELECT id,name,status,created_at FROM tenants WHERE id=?`, id).
		Scan(&t.ID, &t.Name, &t.Status, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) UpdateTenantStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE tenants SET status=? WHERE id=?`, status, id)
	return err
}

func (s *Store) TenantsForUser(userID string) ([]model.TenantWithMembership, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.name, t.status, tm.role, tm.status
		FROM tenants t
		JOIN tenant_members tm ON t.id = tm.tenant_id
		WHERE tm.user_id = ?
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
	rows, err := s.db.Query(`SELECT id, name, status FROM tenants ORDER BY created_at`)
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
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO tenant_members (tenant_id,user_id,role,status) VALUES (?,?,?,'active')`,
		tenantID, userID, role,
	)
	return err
}

func (s *Store) TenantMember(tenantID, userID string) (*model.TenantMemberRow, error) {
	m := &model.TenantMemberRow{}
	err := s.db.QueryRow(
		`SELECT tenant_id,user_id,role,status FROM tenant_members WHERE tenant_id=? AND user_id=?`,
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
	if role != "" {
		q += `role=?, `
		args = append(args, role)
	}
	if status != "" {
		q += `status=?, `
		args = append(args, status)
	}
	q = q[:len(q)-2] + ` WHERE tenant_id=? AND user_id=?`
	args = append(args, tenantID, userID)
	_, err := s.db.Exec(q, args...)
	return err
}

func (s *Store) TenantMembers(tenantID string) ([]model.Member, error) {
	rows, err := s.db.Query(`
		SELECT u.id, u.email, u.display_name, tm.role, tm.status,
			COALESCE((SELECT created_at FROM tenant_members WHERE tenant_id=tm.tenant_id AND user_id=u.id), u.created_at)
		FROM tenant_members tm
		JOIN users u ON u.id = tm.user_id
		WHERE tm.tenant_id = ?`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Member
	for rows.Next() {
		var m model.Member
		var createdAt string
		if err := rows.Scan(&m.UserID, &m.Email, &m.DisplayName, &m.Role, &m.Status, &createdAt); err != nil {
			return nil, err
		}
		m.CreatedAt = createdAt
		list = append(list, m)
	}
	return list, rows.Err()
}

// ---- invitations ----

func (s *Store) CreateInvitation(tenantID, email, role string) (*model.InvitationRow, error) {
	inv := &model.InvitationRow{
		ID: genID(), Token: genToken(), TenantID: tenantID,
		Email: email, Role: role,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339),
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	_, err := s.db.Exec(
		`INSERT INTO invitations (id,token,tenant_id,email,role,expires_at) VALUES (?,?,?,?,?,?)`,
		inv.ID, inv.Token, inv.TenantID, inv.Email, inv.Role, inv.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (s *Store) InvitationByToken(token string) (*model.InvitationRow, error) {
	inv := &model.InvitationRow{}
	err := s.db.QueryRow(
		`SELECT id,token,tenant_id,email,role,expires_at,created_at FROM invitations WHERE token=?`,
		token,
	).Scan(&inv.ID, &inv.Token, &inv.TenantID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	if expires, err := time.Parse(time.RFC3339, inv.ExpiresAt); err != nil || time.Now().After(expires) {
		s.db.Exec(`DELETE FROM invitations WHERE id=?`, inv.ID)
		return nil, fmt.Errorf("invitation expired")
	}
	return inv, nil
}

func (s *Store) DeleteInvitation(id string) error {
	_, err := s.db.Exec(`DELETE FROM invitations WHERE id=?`, id)
	return err
}

// ---- accounts ----

func (s *Store) AccountsByTenant(tenantID string) ([]model.Account, error) {
	rows, err := s.db.Query(
		`SELECT id,tenant_id,name,account_key,status,daily_limit,created_at FROM accounts WHERE tenant_id=? ORDER BY created_at`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAccounts(rows)
}

func (s *Store) CreateAccount(tenantID, name string, dailyLimit int) (*model.AccountRow, error) {
	a := &model.AccountRow{
		ID: genID(), TenantID: tenantID, Name: name,
		AccountKey: "wa_" + genID()[:8], Status: "pending",
		DailyLimit: dailyLimit, CreatedAt: time.Now().Format(time.RFC3339),
	}
	_, err := s.db.Exec(
		`INSERT INTO accounts (id,tenant_id,name,account_key,status,daily_limit) VALUES (?,?,?,?,?,?)`,
		a.ID, a.TenantID, a.Name, a.AccountKey, a.Status, a.DailyLimit,
	)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func scanAccounts(rows *sql.Rows) ([]model.Account, error) {
	var list []model.Account
	for rows.Next() {
		var a model.Account
		var createdAt, dummy string
		if err := rows.Scan(&a.ID, &dummy, &a.Name, &a.AccountKey, &a.Status, &a.DailyLimit, &createdAt); err != nil {
			return nil, err
		}
		a.CreatedAt = createdAt
		list = append(list, a)
	}
	return list, rows.Err()
}

// ---- knowledge bases ----

func (s *Store) KnowledgeBasesByTenant(tenantID string) ([]model.KnowledgeBase, error) {
	rows, err := s.db.Query(
		`SELECT id,tenant_id,name,description,status,created_at FROM knowledge_bases WHERE tenant_id=? ORDER BY created_at`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.KnowledgeBase
	for rows.Next() {
		var k model.KnowledgeBase
		var createdAt, dummy string
		if err := rows.Scan(&k.ID, &dummy, &k.Name, &k.Description, &k.Status, &createdAt); err != nil {
			return nil, err
		}
		k.CreatedAt = createdAt
		list = append(list, k)
	}
	return list, rows.Err()
}

func (s *Store) CreateKnowledgeBase(tenantID, name, description string) (*model.KnowledgeRow, error) {
	k := &model.KnowledgeRow{
		ID: genID(), TenantID: tenantID, Name: name,
		Description: description, Status: "active", CreatedAt: time.Now().Format(time.RFC3339),
	}
	_, err := s.db.Exec(
		`INSERT INTO knowledge_bases (id,tenant_id,name,description,status) VALUES (?,?,?,?,?)`,
		k.ID, k.TenantID, k.Name, k.Description, k.Status,
	)
	if err != nil {
		return nil, err
	}
	return k, nil
}

// ---- conversations ----

func (s *Store) ConversationsByTenant(tenantID string) ([]model.Conversation, error) {
	rows, err := s.db.Query(
		`SELECT id,tenant_id,account_id,customer,last_message,status,last_message_at FROM conversations WHERE tenant_id=? ORDER BY last_message_at DESC`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Conversation
	for rows.Next() {
		var c model.Conversation
		var lastMsgAt, dummy string
		if err := rows.Scan(&c.ID, &dummy, &c.AccountID, &c.Customer, &c.LastMessage, &c.Status, &lastMsgAt); err != nil {
			return nil, err
		}
		c.LastMessageAt = lastMsgAt
		list = append(list, c)
	}
	return list, rows.Err()
}

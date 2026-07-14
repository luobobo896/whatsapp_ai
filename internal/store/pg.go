package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/model"
)

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
		 RETURNING to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`,
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
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
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
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
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
		ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
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
		        to_char(expires_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM sessions WHERE id=$1`, id,
	).Scan(&sess.ID, &sess.UserID, &sess.CSRFToken, &activeTenant, &sess.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if activeTenant != nil {
		sess.ActiveTenantID = *activeTenant
	}
	if expires, err := time.Parse(time.RFC3339, sess.ExpiresAt); err != nil || time.Now().After(expires) {
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
		`INSERT INTO tenants (id,name) VALUES ($1,$2) RETURNING to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`,
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
		`SELECT id,name,status,to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM tenants WHERE id=$1`, id,
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
		       to_char(u.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
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
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339),
	}
	err := s.pool.QueryRow(context.Background(),
		`INSERT INTO invitations (id,token,tenant_id,email,role,expires_at) VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`,
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
		        to_char(expires_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM invitations WHERE token=$1`, token,
	).Scan(&inv.ID, &inv.Token, &inv.TenantID, &inv.Email, &inv.Role, &inv.ExpiresAt, &inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	if expires, err := time.Parse(time.RFC3339, inv.ExpiresAt); err != nil || time.Now().After(expires) {
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
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
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
		 RETURNING to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`,
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
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
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
		 RETURNING to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`,
		k.ID, k.TenantID, k.Name, k.Description, k.Status,
	).Scan(&k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return k, nil
}

// ---- conversations ----

func (s *Store) ConversationsByTenant( tenantID string) ([]model.Conversation, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id,account_id,customer,last_message,status,
		        to_char(last_message_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
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

CREATE TABLE support_accounts (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name text NOT NULL,
  account_key text NOT NULL,
  status text NOT NULL CHECK (status IN ('pending', 'connected', 'disabled')),
  daily_limit integer NOT NULL DEFAULT 30 CHECK (daily_limit >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, account_key)
);
CREATE INDEX support_accounts_tenant_created_idx ON support_accounts (tenant_id, created_at DESC);

CREATE TABLE knowledge_bases (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  status text NOT NULL CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX knowledge_bases_tenant_created_idx ON knowledge_bases (tenant_id, created_at DESC);

CREATE TABLE conversations (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  account_id uuid REFERENCES support_accounts(id) ON DELETE SET NULL,
  customer text NOT NULL,
  last_message text NOT NULL DEFAULT '',
  status text NOT NULL CHECK (status IN ('open', 'closed')),
  last_message_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX conversations_tenant_activity_idx ON conversations (tenant_id, last_message_at DESC);

GRANT SELECT, INSERT, UPDATE, DELETE ON support_accounts, knowledge_bases, conversations TO whatsapp_app;

ALTER TABLE support_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE support_accounts FORCE ROW LEVEL SECURITY;
CREATE POLICY support_accounts_tenant ON support_accounts
  USING (tenant_id = app_current_tenant_id())
  WITH CHECK (tenant_id = app_current_tenant_id());

ALTER TABLE knowledge_bases ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_bases FORCE ROW LEVEL SECURITY;
CREATE POLICY knowledge_bases_tenant ON knowledge_bases
  USING (tenant_id = app_current_tenant_id())
  WITH CHECK (tenant_id = app_current_tenant_id());

ALTER TABLE conversations ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversations FORCE ROW LEVEL SECURITY;
CREATE POLICY conversations_tenant ON conversations
  USING (tenant_id = app_current_tenant_id())
  WITH CHECK (tenant_id = app_current_tenant_id());

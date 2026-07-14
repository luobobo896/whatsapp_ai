CREATE FUNCTION app_current_tenant_id() RETURNS uuid
LANGUAGE sql STABLE AS $$
  SELECT NULLIF(current_setting('app.tenant_id', true), '')::uuid
$$;

ALTER TABLE tenant_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_memberships_tenant ON tenant_memberships
  USING (tenant_id = app_current_tenant_id())
  WITH CHECK (tenant_id = app_current_tenant_id());

ALTER TABLE member_invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE member_invitations FORCE ROW LEVEL SECURITY;
CREATE POLICY member_invitations_tenant ON member_invitations
  USING (tenant_id = app_current_tenant_id())
  WITH CHECK (tenant_id = app_current_tenant_id());
CREATE POLICY member_invitations_token_lookup ON member_invitations
  FOR SELECT USING (
    token_hash = NULLIF(current_setting('app.invitation_token_hash', true), '')
  );

ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs FORCE ROW LEVEL SECURITY;
CREATE POLICY audit_logs_tenant ON audit_logs
  USING (tenant_id = app_current_tenant_id())
  WITH CHECK (tenant_id = app_current_tenant_id());
CREATE POLICY audit_logs_platform_insert ON audit_logs
  FOR INSERT WITH CHECK (
    tenant_id IS NULL AND app_current_tenant_id() IS NULL
  );
CREATE POLICY audit_logs_platform_select ON audit_logs
  FOR SELECT USING (
    tenant_id IS NULL AND app_current_tenant_id() IS NULL
  );

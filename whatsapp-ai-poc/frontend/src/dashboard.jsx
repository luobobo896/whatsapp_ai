import { useEffect, useState } from "react";
import {
  Building2,
  Check,
  Copy,
  Database,
  LayoutDashboard,
  LogOut,
  Plus,
  RefreshCw,
  Server,
  ShieldCheck,
  UserPlus,
  Users,
} from "lucide-react";
import { APIError, get, messageForError, patch, post } from "./api.js";
import { Badge, Brand, Button, Dialog, EmptyState, Field, IconButton, Panel, Toast } from "./components.jsx";

const ROLE_LABELS = {
  owner: "所有者",
  admin: "管理员",
  agent: "客服",
  viewer: "只读成员",
};

const NAV_ITEMS = [
  { id: "overview", label: "工作台", hint: "Overview", icon: LayoutDashboard },
  { id: "tenants", label: "租户管理", hint: "Tenants", icon: Building2 },
  { id: "members", label: "成员管理", hint: "Members", icon: Users },
];

export function Dashboard({ session, onSessionChange, onSignedOut }) {
  const [view, setView] = useState("overview");
  const [tenants, setTenants] = useState([]);
  const [platformRole, setPlatformRole] = useState("");
  const [members, setMembers] = useState([]);
  const [health, setHealth] = useState(null);
  const [loading, setLoading] = useState(true);
  const [selectingTenant, setSelectingTenant] = useState(false);
  const [signingOut, setSigningOut] = useState(false);
  const [refreshVersion, setRefreshVersion] = useState(0);
  const [toast, setToast] = useState(null);
  const [createTenantOpen, setCreateTenantOpen] = useState(false);
  const [inviteMemberOpen, setInviteMemberOpen] = useState(false);
  const [invitation, setInvitation] = useState(null);

  const activeTenant = tenants.find((tenant) => tenant.id === session.activeTenantId) || null;
  const selectableTenants = tenants.filter((tenant) => tenant.status === "active" && tenant.membershipStatus === "active");
  const canManageMembers = activeTenant?.permissions?.includes("members:manage") || false;

  useEffect(() => {
    let cancelled = false;
    const healthRequest = get("/health");
    const tenantsRequest = get("/api/tenants");
    const membersRequest = session.activeTenantId ? get("/api/members") : Promise.resolve({ members: [] });
    Promise.all([healthRequest, tenantsRequest, membersRequest])
      .then(([healthResult, tenantResult, memberResult]) => {
        if (cancelled) return;
        setHealth(healthResult);
        setTenants(tenantResult.tenants || []);
        setPlatformRole(tenantResult.platformRole || "");
        setMembers(memberResult.members || []);
      })
      .catch((requestError) => {
        if (cancelled) return;
        if (requestError instanceof APIError && requestError.status === 401) {
          onSignedOut();
          return;
        }
        setToast({ tone: "error", message: messageForError(requestError) });
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [session.activeTenantId, refreshVersion, onSignedOut]);

  useEffect(() => {
    if (!toast) return undefined;
    const timeout = window.setTimeout(() => setToast(null), 3600);
    return () => window.clearTimeout(timeout);
  }, [toast]);

  async function refresh() {
    onSessionChange(await get("/api/auth/me"));
    setRefreshVersion((value) => value + 1);
  }

  async function selectTenant(tenantId) {
    if (selectingTenant) return;
    setSelectingTenant(true);
    try {
      await post("/api/auth/select-tenant", { tenantId }, session.csrfToken);
      onSessionChange(await get("/api/auth/me"));
      setView("overview");
      setRefreshVersion((value) => value + 1);
      setToast({ tone: "success", message: "工作区已切换" });
    } catch (requestError) {
      setToast({ tone: "error", message: messageForError(requestError) });
    } finally {
      setSelectingTenant(false);
    }
  }

  async function signOut() {
    if (signingOut) return;
    setSigningOut(true);
    try {
      await post("/api/auth/logout", {}, session.csrfToken);
    } finally {
      onSignedOut();
    }
  }

  function navigate(nextView) {
    if (nextView === "members" && !session.activeTenantId) {
      setToast({ tone: "info", message: "请先选择一个租户工作区" });
      return;
    }
    setView(nextView);
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="sidebar-desktop-brand"><Brand /></div>
        <div className="sidebar-horizontal-brand"><Brand compact /></div>
        <nav aria-label="主导航">
          <span className="nav-section-label">管理中心</span>
          {NAV_ITEMS.map((item) => (
            <button key={item.id} className={`nav-item ${view === item.id ? "active" : ""}`} onClick={() => navigate(item.id)}>
              <item.icon aria-hidden="true" size={18} />
              <span>{item.label}<small>{item.hint}</small></span>
            </button>
          ))}
        </nav>
        <div className="sidebar-foot">
          <ShieldCheck aria-hidden="true" size={17} />
          <span>租户隔离已启用<small>所有权限由服务端会话校验</small></span>
        </div>
      </aside>

      <main className="workspace">
        <header className="topbar">
          <div className="topbar-left">
            <div>
              <h1>{NAV_ITEMS.find((item) => item.id === view)?.label}</h1>
              <p>{activeTenant ? activeTenant.name : platformRole ? "平台管理空间" : "尚未选择租户工作区"}</p>
            </div>
          </div>
          <div className="topbar-actions">
            {selectableTenants.length ? (
              <label className="tenant-switcher">
                <Building2 aria-hidden="true" size={16} />
                <select aria-label="切换租户" value={session.activeTenantId || ""} disabled={selectingTenant} onChange={(event) => selectTenant(event.target.value)}>
                  <option value="" disabled>选择工作区</option>
                  {selectableTenants.map((tenant) => <option key={tenant.id} value={tenant.id}>{tenant.name}</option>)}
                </select>
              </label>
            ) : null}
            <IconButton label="刷新" icon={RefreshCw} onClick={() => refresh().catch((error) => setToast({ tone: "error", message: messageForError(error) }))} />
            <span className="user-chip"><span>{(session.user.displayName || session.user.email).slice(0, 1).toUpperCase()}</span><small>{session.user.displayName || session.user.email}</small></span>
            <IconButton label="退出登录" icon={LogOut} disabled={signingOut} onClick={signOut} />
          </div>
        </header>

        {loading ? <div className="content-loading">加载中...</div> : null}
        {!loading && view === "overview" ? <Overview health={health} tenants={tenants} activeTenant={activeTenant} members={members} platformRole={platformRole} onNavigate={navigate} /> : null}
        {!loading && view === "tenants" ? <TenantsView tenants={tenants} platformRole={platformRole} activeTenantId={session.activeTenantId} onSelect={selectTenant} onCreate={() => setCreateTenantOpen(true)} onStatusChanged={() => setRefreshVersion((value) => value + 1)} csrfToken={session.csrfToken} setToast={setToast} /> : null}
        {!loading && view === "members" ? <MembersView members={members} canManage={canManageMembers} onInvite={() => setInviteMemberOpen(true)} onChanged={() => setRefreshVersion((value) => value + 1)} csrfToken={session.csrfToken} setToast={setToast} /> : null}
      </main>

      {createTenantOpen ? <CreateTenantDialog csrfToken={session.csrfToken} onClose={() => setCreateTenantOpen(false)} onCreated={(created) => { setCreateTenantOpen(false); setInvitation(created.invitation); setRefreshVersion((value) => value + 1); }} setToast={setToast} /> : null}
      {inviteMemberOpen ? <InviteMemberDialog csrfToken={session.csrfToken} onClose={() => setInviteMemberOpen(false)} onInvited={(created) => { setInviteMemberOpen(false); setInvitation(created.invitation); }} setToast={setToast} /> : null}
      {invitation ? <InvitationResult invitation={invitation} onClose={() => setInvitation(null)} setToast={setToast} /> : null}
      <Toast toast={toast} />
    </div>
  );
}

function Overview({ health, tenants, activeTenant, members, platformRole, onNavigate }) {
  const metrics = [
    { label: "服务状态", value: health?.status === "ok" ? "正常" : "异常", note: "Go API", icon: Server, tone: health?.status === "ok" ? "success" : "error" },
    { label: "数据库", value: health?.database === "up" ? "已连接" : "断开", note: "PostgreSQL", icon: Database, tone: health?.database === "up" ? "success" : "error" },
    { label: "可用租户", value: String(tenants.length), note: platformRole ? "平台范围" : "账号权限范围", icon: Building2, tone: "info" },
    { label: "当前成员", value: activeTenant ? String(members.length) : "-", note: activeTenant?.name || "未选择工作区", icon: Users, tone: "muted" },
  ];
  return (
    <div className="view-stack">
      <section className="metric-grid">
        {metrics.map((metric) => (
          <div className="metric" key={metric.label}>
            <span className={`metric-icon metric-icon-${metric.tone}`}><metric.icon aria-hidden="true" size={19} /></span>
            <div><small>{metric.label}</small><strong>{metric.value}</strong><span>{metric.note}</span></div>
          </div>
        ))}
      </section>
      <div className="overview-grid">
        <Panel title="当前访问范围" description="登录会话中的平台与租户上下文">
          <dl className="detail-list">
            <div><dt>平台权限</dt><dd>{platformRole ? <Badge tone="success">平台管理员</Badge> : <Badge>普通成员</Badge>}</dd></div>
            <div><dt>当前租户</dt><dd>{activeTenant ? activeTenant.name : "未选择"}</dd></div>
            <div><dt>租户角色</dt><dd>{activeTenant?.role ? ROLE_LABELS[activeTenant.role] : "-"}</dd></div>
            <div><dt>租户状态</dt><dd>{activeTenant ? <Badge tone={activeTenant.status === "active" ? "success" : "warning"}>{activeTenant.status === "active" ? "运行中" : "已暂停"}</Badge> : "-"}</dd></div>
          </dl>
        </Panel>
        <Panel title="管理入口" description="当前后端已开放的运营能力">
          <div className="action-list">
            <button onClick={() => onNavigate("tenants")}><span><Building2 aria-hidden="true" size={18} /><span><strong>租户管理</strong><small>{platformRole ? "接入与维护租户" : "切换可访问的工作区"}</small></span></span><span>进入</span></button>
            <button onClick={() => onNavigate("members")} disabled={!activeTenant}><span><Users aria-hidden="true" size={18} /><span><strong>成员管理</strong><small>查看角色与成员状态</small></span></span><span>{activeTenant ? "进入" : "需选择租户"}</span></button>
          </div>
        </Panel>
      </div>
    </div>
  );
}

function TenantsView({ tenants, platformRole, activeTenantId, onSelect, onCreate, onStatusChanged, csrfToken, setToast }) {
  const [pendingTenantId, setPendingTenantId] = useState("");

  async function toggleStatus(tenant) {
    if (pendingTenantId) return;
    const nextStatus = tenant.status === "active" ? "suspended" : "active";
    setPendingTenantId(tenant.id);
    try {
      await patch(`/api/platform/tenants/${tenant.id}/status`, { status: nextStatus, reason: nextStatus === "suspended" ? "由平台管理台暂停" : "" }, csrfToken);
      setToast({ tone: "success", message: nextStatus === "active" ? "租户已恢复" : "租户已暂停" });
      onStatusChanged();
    } catch (error) {
      setToast({ tone: "error", message: messageForError(error) });
    } finally {
      setPendingTenantId("");
    }
  }

  async function enterTenant(tenantId) {
    if (pendingTenantId) return;
    setPendingTenantId(tenantId);
    try {
      await onSelect(tenantId);
    } finally {
      setPendingTenantId("");
    }
  }

  return (
    <Panel title={platformRole ? "平台租户" : "我的工作区"} description={platformRole ? "创建租户并管理服务状态" : "选择当前会话使用的租户"} action={platformRole ? <Button icon={Plus} onClick={onCreate}>新建租户</Button> : null}>
      {!tenants.length ? <EmptyState icon={Building2} title="暂无租户" description={platformRole ? "创建首个租户后，系统将生成所有者邀请。" : "当前账号还没有可访问的租户。"} action={platformRole ? <Button icon={Plus} onClick={onCreate}>新建租户</Button> : null} /> : (
        <div className="table-wrap">
          <table>
            <thead><tr><th>租户</th><th>标识</th><th>状态</th><th>成员身份</th><th><span className="sr-only">操作</span></th></tr></thead>
            <tbody>{tenants.map((tenant) => {
              const membershipActive = tenant.membershipStatus === "active";
              const selectable = membershipActive && tenant.status === "active";
              const pending = pendingTenantId === tenant.id;
              return (
                <tr key={tenant.id}>
                  <td><strong>{tenant.name}</strong>{tenant.id === activeTenantId ? <small className="table-subline">当前工作区</small> : null}</td>
                  <td><code>{tenant.slug}</code></td>
                  <td><Badge tone={tenant.status === "active" ? "success" : "warning"}>{tenant.status === "active" ? "运行中" : "已暂停"}</Badge></td>
                  <td>{tenant.role ? <>{ROLE_LABELS[tenant.role] || tenant.role}{!membershipActive ? <small className="table-subline">成员身份已停用</small> : null}</> : "-"}</td>
                  <td className="table-actions"><span className="table-action-group">
                    {selectable ? <Button variant={tenant.id === activeTenantId ? "secondary" : "primary"} disabled={tenant.id === activeTenantId || pending} onClick={() => enterTenant(tenant.id)}>{tenant.id === activeTenantId ? "已选择" : pending ? "正在进入..." : "进入"}</Button> : null}
                    {platformRole ? <Button variant="secondary" disabled={pending} onClick={() => toggleStatus(tenant)}>{pending ? "处理中..." : tenant.status === "active" ? "暂停" : "恢复"}</Button> : null}
                  </span></td>
                </tr>
              );
            })}</tbody>
          </table>
        </div>
      )}
    </Panel>
  );
}

function MembersView({ members, canManage, onInvite, onChanged, csrfToken, setToast }) {
  const [pendingUserId, setPendingUserId] = useState("");

  async function updateMember(member, changes) {
    if (pendingUserId) return;
    setPendingUserId(member.userId);
    try {
      await patch(`/api/members/${member.userId}`, { role: changes.role || member.role, status: changes.status || member.status }, csrfToken);
      setToast({ tone: "success", message: "成员信息已更新" });
      onChanged();
    } catch (error) {
      setToast({ tone: "error", message: messageForError(error) });
    } finally {
      setPendingUserId("");
    }
  }

  return (
    <Panel title="租户成员" description="成员角色和访问状态由服务端权限控制" action={canManage ? <Button icon={UserPlus} onClick={onInvite}>邀请成员</Button> : null}>
      {!members.length ? <EmptyState icon={Users} title="暂无成员" description="当前租户还没有可显示的成员。" action={canManage ? <Button icon={UserPlus} onClick={onInvite}>邀请成员</Button> : null} /> : (
        <div className="table-wrap">
          <table>
            <thead><tr><th>成员</th><th>角色</th><th>状态</th><th>加入时间</th>{canManage ? <th><span className="sr-only">操作</span></th> : null}</tr></thead>
            <tbody>{members.map((member) => {
              const pending = pendingUserId === member.userId;
              return (
                <tr key={member.userId}>
                  <td><strong>{member.displayName}</strong><small className="table-subline">{member.email}</small></td>
                  <td>{canManage ? <select className="table-select" value={member.role} disabled={pending} onChange={(event) => updateMember(member, { role: event.target.value })}>{Object.entries(ROLE_LABELS).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select> : ROLE_LABELS[member.role]}</td>
                  <td><Badge tone={member.status === "active" ? "success" : "warning"}>{member.status === "active" ? "正常" : "已停用"}</Badge></td>
                  <td>{new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium" }).format(new Date(member.createdAt))}</td>
                  {canManage ? <td className="table-actions"><Button variant="secondary" disabled={pending} onClick={() => updateMember(member, { status: member.status === "active" ? "disabled" : "active" })}>{pending ? "处理中..." : member.status === "active" ? "停用" : "启用"}</Button></td> : null}
                </tr>
              );
            })}</tbody>
          </table>
        </div>
      )}
    </Panel>
  );
}

function CreateTenantDialog({ csrfToken, onClose, onCreated, setToast }) {
  const [form, setForm] = useState({ name: "", slug: "", ownerEmail: "", ownerDisplayName: "" });
  const [submitting, setSubmitting] = useState(false);
  function update(key, value) { setForm((current) => ({ ...current, [key]: value })); }
  async function submit(event) {
    event.preventDefault();
    setSubmitting(true);
    try {
      onCreated(await post("/api/platform/tenants", form, csrfToken));
      setToast({ tone: "success", message: "租户已创建" });
    } catch (error) {
      setToast({ tone: "error", message: messageForError(error) });
    } finally { setSubmitting(false); }
  }
  return (
    <Dialog title="新建租户" description="创建后会生成首位所有者的邀请链接" onClose={onClose} footer={<><Button variant="secondary" type="button" onClick={onClose}>取消</Button><Button form="create-tenant" type="submit" icon={Plus} disabled={submitting}>{submitting ? "创建中..." : "创建租户"}</Button></>}>
      <form id="create-tenant" className="dialog-form" onSubmit={submit}>
        <div className="form-grid"><Field label="租户名称"><input value={form.name} onChange={(event) => update("name", event.target.value)} required /></Field><Field label="租户标识" hint="小写字母、数字和连字符"><input value={form.slug} pattern="[a-z0-9]+(?:-[a-z0-9]+)*" onChange={(event) => update("slug", event.target.value)} required /></Field></div>
        <div className="form-grid"><Field label="所有者邮箱"><input type="email" value={form.ownerEmail} onChange={(event) => update("ownerEmail", event.target.value)} required /></Field><Field label="所有者名称"><input value={form.ownerDisplayName} onChange={(event) => update("ownerDisplayName", event.target.value)} required /></Field></div>
      </form>
    </Dialog>
  );
}

function InviteMemberDialog({ csrfToken, onClose, onInvited, setToast }) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("agent");
  const [submitting, setSubmitting] = useState(false);
  async function submit(event) {
    event.preventDefault();
    setSubmitting(true);
    try {
      onInvited(await post("/api/members/invitations", { email, role }, csrfToken));
      setToast({ tone: "success", message: "邀请已创建" });
    } catch (error) {
      setToast({ tone: "error", message: messageForError(error) });
    } finally { setSubmitting(false); }
  }
  return (
    <Dialog title="邀请成员" description="邀请链接将在 7 天后失效" onClose={onClose} footer={<><Button variant="secondary" type="button" onClick={onClose}>取消</Button><Button form="invite-member" type="submit" icon={UserPlus} disabled={submitting}>{submitting ? "创建中..." : "生成邀请"}</Button></>}>
      <form id="invite-member" className="dialog-form" onSubmit={submit}>
        <Field label="成员邮箱"><input type="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></Field>
        <Field label="租户角色"><select value={role} onChange={(event) => setRole(event.target.value)}>{Object.entries(ROLE_LABELS).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></Field>
      </form>
    </Dialog>
  );
}

function InvitationResult({ invitation, onClose, setToast }) {
  const link = `${window.location.origin}/invitations/${encodeURIComponent(invitation.token)}/accept`;
  async function copy() {
    await navigator.clipboard.writeText(link);
    setToast({ tone: "success", message: "邀请链接已复制" });
  }
  return (
    <Dialog title="邀请已生成" description={`发送给 ${invitation.email}`} onClose={onClose} footer={<Button onClick={onClose}>完成</Button>}>
      <div className="invitation-result"><span><Check aria-hidden="true" size={22} /></span><p>该链接只显示一次，请立即发送给受邀成员。</p></div>
      <div className="copy-field"><input readOnly value={link} aria-label="邀请链接" /><IconButton label="复制邀请链接" icon={Copy} onClick={() => copy().catch(() => setToast({ tone: "error", message: "复制失败，请手动复制" }))} /></div>
    </Dialog>
  );
}

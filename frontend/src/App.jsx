import { useEffect, useState } from "react";
import { AlertTriangle, Check, Eye, EyeOff, KeyRound, MessageCircleMore, ShieldCheck, UserPlus } from "lucide-react";
import { APIError, get, messageForError, post } from "./api.js";
import { Brand, Button, Field, IconButton } from "./components.jsx";
import { Dashboard } from "./dashboard.jsx";

function invitationTokenFromPath() {
  const match = window.location.pathname.match(/^\/invitations\/([^/]+)\/accept\/?$/);
  return match ? decodeURIComponent(match[1]) : "";
}

export default function App() {
  const [session, setSession] = useState(null);
  const [loading, setLoading] = useState(true);
  const inviteToken = invitationTokenFromPath();

  useEffect(() => {
    let cancelled = false;
    get("/api/auth/me")
      .then((result) => {
        if (!cancelled) setSession(result);
      })
      .catch((error) => {
        if (!cancelled && error instanceof APIError && error.status === 401) setSession(null);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, []);

  if (inviteToken) {
    return <InvitationPage token={inviteToken} onAccepted={setSession} />;
  }
  if (loading) return <LoadingScreen />;
  if (!session) return <LoginPage onLogin={setSession} />;
  return <Dashboard session={session} onSessionChange={setSession} onSignedOut={() => setSession(null)} />;
}

function LoadingScreen() {
  return (
    <main className="loading-screen">
      <span className="loading-mark"><MessageCircleMore aria-hidden="true" size={25} /></span>
      <span>加载中...</span>
    </main>
  );
}

function LoginPage({ onLogin }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(event) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      await post("/api/auth/login", { email, password });
      onLogin(await get("/api/auth/me"));
      window.history.replaceState(null, "", "/");
    } catch (requestError) {
      setError(messageForError(requestError));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="auth-layout">
      <section className="auth-brand-panel">
        <Brand />
        <div className="auth-brand-status">
          <span><ShieldCheck aria-hidden="true" size={18} /></span>
          <div><strong>安全运营入口</strong><small>会话与租户权限由服务端统一管理</small></div>
        </div>
        <p className="auth-footnote">企业内部管理系统</p>
      </section>
      <section className="auth-form-area">
        <form className="auth-form" onSubmit={handleSubmit}>
          <header>
            <h1>登录管理台</h1>
            <p>使用管理员或租户成员账号继续</p>
          </header>
          {error ? <div className="form-alert"><AlertTriangle aria-hidden="true" size={17} />{error}</div> : null}
          <Field label="邮箱地址">
            <input type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="name@company.com" required autoFocus />
          </Field>
          <Field label="密码">
            <span className="password-field">
              <input type={showPassword ? "text" : "password"} autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="输入登录密码" required />
              <IconButton label={showPassword ? "隐藏密码" : "显示密码"} icon={showPassword ? EyeOff : Eye} type="button" onClick={() => setShowPassword((value) => !value)} />
            </span>
          </Field>
          <Button className="auth-submit" type="submit" icon={KeyRound} disabled={submitting}>
            {submitting ? "正在登录..." : "登录"}
          </Button>
        </form>
      </section>
    </main>
  );
}

function InvitationPage({ token, onAccepted }) {
  const [form, setForm] = useState({ email: "", displayName: "", password: "" });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  function update(key, value) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  async function handleSubmit(event) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      const accepted = await post(`/api/invitations/${encodeURIComponent(token)}/accept`, form);
      await post("/api/auth/select-tenant", { tenantId: accepted.tenantId }, accepted.csrfToken);
      onAccepted(await get("/api/auth/me"));
      window.history.replaceState(null, "", "/");
    } catch (requestError) {
      setError(messageForError(requestError));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="invitation-layout">
      <header><Brand compact /></header>
      <form className="invitation-form" onSubmit={handleSubmit}>
        <span className="page-icon"><UserPlus aria-hidden="true" size={24} /></span>
        <h1>接受成员邀请</h1>
        <p>完成账号信息后进入所属租户工作区</p>
        {error ? <div className="form-alert"><AlertTriangle aria-hidden="true" size={17} />{error}</div> : null}
        <div className="form-grid">
          <Field label="受邀邮箱"><input type="email" value={form.email} onChange={(event) => update("email", event.target.value)} required /></Field>
          <Field label="显示名称"><input value={form.displayName} onChange={(event) => update("displayName", event.target.value)} required /></Field>
        </div>
        <Field label="设置密码" hint="至少 12 个字符">
          <input type="password" autoComplete="new-password" minLength={12} value={form.password} onChange={(event) => update("password", event.target.value)} required />
        </Field>
        <Button type="submit" icon={Check} disabled={submitting}>{submitting ? "正在加入..." : "确认并进入"}</Button>
      </form>
    </main>
  );
}

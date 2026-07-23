import { setSession } from "../composables/useSession";

export class APIError extends Error {
  constructor(message, code, status, requestId) {
    super(message);
    this.name = "APIError";
    this.code = code;
    this.status = status;
    this.requestId = requestId;
  }
}

const ERROR_MESSAGES = {
  AUTH_REQUIRED: "登录状态已失效，请重新登录。",
  SESSION_EXPIRED: "登录状态已失效，请重新登录。",
  AUTH_INVALID: "邮箱或密码不正确。",
  FORBIDDEN: "当前账号没有执行此操作的权限。",
  TENANT_SUSPENDED: "当前租户已暂停服务。",
  RATE_LIMITED: "操作过于频繁，请稍后重试。",
  DAILY_LIMIT_REACHED: "今日回复已达到账号上限，请明日再试或调整账号配额。",
  OPENCLAW_ERROR: "OpenClaw 服务异常，请检查 OpenClaw 是否正常运行。",
};

// Distinguish network failures (DNS/down/offline) from server-side 5xx so views
// can show actionable messages instead of relying on each one to interpret status.
const NETWORK_ERROR_MESSAGE = "网络连接失败，请检查网络后重试。";
const SERVER_ERROR_MESSAGE = "服务暂时不可用，请稍后重试。";

// Endpoints that authenticate themselves (login / invitation acceptance) must not
// trigger the global "session expired" redirect when they return 401.
const SESSION_AUTH_PATH_PREFIXES = ["/api/auth/login", "/api/invitations/"];

// Module-level guard so concurrent in-flight 401s only redirect once.
let sessionExpiryHandling = false;

function isSessionAuthPath(path) {
  return SESSION_AUTH_PATH_PREFIXES.some((prefix) => path.startsWith(prefix));
}

function handleSessionExpired() {
  if (sessionExpiryHandling) return;
  sessionExpiryHandling = true;
  try {
    setSession(null);
  } catch {
    // setSession may fail in non-browser contexts; ignore — the redirect below still handles it.
  }
  if (typeof window !== "undefined" && window.location) {
    const current = window.location.pathname || "";
    const loginPath = import.meta.env.BASE_URL + "login";
    if (current !== loginPath && !current.startsWith(import.meta.env.BASE_URL + "invitations/")) {
      window.location.assign(import.meta.env.BASE_URL + "login");
    }
  }
}

export function messageForError(error) {
  if (error instanceof APIError) {
    if (error.code === "NETWORK_ERROR") return NETWORK_ERROR_MESSAGE;
    if (error.status >= 500) return SERVER_ERROR_MESSAGE;
    return ERROR_MESSAGES[error.code] || error.message;
  }
  return "暂时无法完成请求，请稍后重试。";
}

async function request(path, options = {}) {
  let response;
  try {
    response = await fetch(path, {
      credentials: "same-origin",
      ...options,
      headers: {
        ...(options.body && !(options.body instanceof FormData) ? { "Content-Type": "application/json" } : {}),
        ...options.headers,
      },
    });
  } catch {
    // fetch() throws TypeError on network-level failures (DNS, connection refused,
    // offline). Surface as a dedicated code so callers can distinguish from HTTP 5xx.
    throw new APIError(NETWORK_ERROR_MESSAGE, "NETWORK_ERROR", 0, undefined);
  }

  if (!response.ok) {
    let payload = null;
    try {
      payload = await response.json();
    } catch {
      // The fallback message below is used for non-JSON failures.
    }
    const details = payload?.error;
    const status = response.status;
    // Centralized 401 handling: clear session + redirect to /login. Skip for login/
    // invitation endpoints so wrong-credentials on those pages stays actionable.
    if (status === 401 && !isSessionAuthPath(path)) {
      handleSessionExpired();
    }
    throw new APIError(
      details?.message || (status >= 500 ? SERVER_ERROR_MESSAGE : `请求失败 (${status})`),
      details?.code || (status >= 500 ? "SERVER_ERROR" : "REQUEST_FAILED"),
      status,
      details?.requestId,
    );
  }
  if (response.status === 204) return null;
  return response.json();
}

export function get(path) {
  return request(path);
}

export function post(path, body, csrfToken) {
  return request(path, {
    method: "POST",
    body: JSON.stringify(body),
    headers: csrfToken
      ? { "X-CSRF-Token": csrfToken }
      : undefined,
  });
}

export function postForm(path, body, csrfToken) {
  return request(path, {
    method: "POST",
    body,
    headers: csrfToken ? { "X-CSRF-Token": csrfToken } : undefined,
  });
}

export function del(path, csrfToken) {
  return request(path, {
    method: "DELETE",
    headers: { "X-CSRF-Token": csrfToken },
  });
}

export function patch(path, body, csrfToken) {
  return request(path, {
    method: "PATCH",
    body: JSON.stringify(body),
    headers: { "X-CSRF-Token": csrfToken },
  });
}

export function put(path, body, csrfToken) {
  return request(path, {
    method: "PUT",
    body: JSON.stringify(body),
    headers: { "X-CSRF-Token": csrfToken },
  });
}

// Members invitations — list pending and revoke. Backend contract assumed:
//   GET    /api/members/invitations          -> { invitations: [...] }
//   DELETE /api/members/invitations/:id      -> 204
export function listInvitations() {
  return get("/api/members/invitations").then((resp) => resp?.invitations || []);
}

export function revokeInvitation(invitationId, csrfToken) {
  return del(`/api/members/invitations/${encodeURIComponent(invitationId)}`, csrfToken);
}

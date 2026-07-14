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
};

export function messageForError(error) {
  if (error instanceof APIError) return ERROR_MESSAGES[error.code] || error.message;
  return "暂时无法完成请求，请稍后重试。";
}

async function request(path, options = {}) {
  const response = await fetch(path, {
    credentials: "same-origin",
    ...options,
    headers: {
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...options.headers,
    },
  });

  if (!response.ok) {
    let payload = null;
    try {
      payload = await response.json();
    } catch {
      // The fallback message below is used for non-JSON failures.
    }
    const details = payload?.error;
    throw new APIError(
      details?.message || `请求失败 (${response.status})`,
      details?.code || "REQUEST_FAILED",
      response.status,
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

export function patch(path, body, csrfToken) {
  return request(path, {
    method: "PATCH",
    body: JSON.stringify(body),
    headers: { "X-CSRF-Token": csrfToken },
  });
}

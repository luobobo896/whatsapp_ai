import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import App from "./App.jsx";

function jsonResponse(status, body) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: vi.fn().mockResolvedValue(body),
  };
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.history.replaceState(null, "", "/");
});

describe("App", () => {
  it("renders the login page when there is no authenticated session", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(401, {
      error: { code: "AUTH_REQUIRED", message: "Authentication is required." },
    })));

    render(<App />);

    expect(await screen.findByRole("heading", { name: "登录管理台" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "登录" })).toBeTruthy();
  });

  it("loads the authenticated platform workspace and opens tenant management", async () => {
    const session = {
      csrfToken: "csrf-token",
      activeTenantId: null,
      user: { id: "user-1", email: "admin@example.com", displayName: "Admin" },
    };
    vi.stubGlobal("fetch", vi.fn().mockImplementation((input) => {
      if (input === "/api/auth/me") return Promise.resolve(jsonResponse(200, session));
      if (input === "/health") return Promise.resolve(jsonResponse(200, { status: "ok", database: "up" }));
      if (input === "/api/tenants") return Promise.resolve(jsonResponse(200, { platformRole: "platform_admin", tenants: [] }));
      return Promise.reject(new Error(`unexpected request: ${input}`));
    }));

    render(<App />);

    expect(await screen.findByRole("heading", { name: "工作台" })).toBeTruthy();
    expect(await screen.findByText("已连接")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /租户管理.*Tenants/ }));
    expect(await screen.findByRole("heading", { name: "平台租户" })).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "新建租户" })).toHaveLength(2);
  });
});

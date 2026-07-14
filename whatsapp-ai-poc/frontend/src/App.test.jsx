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

    expect(await screen.findByRole("heading", { name: "总览" })).toBeTruthy();
    expect(screen.getByRole("button", { name: /客服账号.*Accounts/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /知识库.*Knowledge/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /会话.*Conversations/ })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /租户管理.*Tenants/ }));
    expect(await screen.findByRole("heading", { name: "平台租户" })).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "新建租户" })).toHaveLength(2);
    fireEvent.click(screen.getAllByRole("button", { name: "新建租户" })[0]);
    expect(screen.getByRole("heading", { name: "新建租户" })).toBeTruthy();
    expect(screen.getByLabelText("租户名称")).toBeTruthy();
    expect(screen.queryByLabelText("租户标识")).toBeNull();
    expect(screen.queryByLabelText("所有者邮箱")).toBeNull();
  });

  it("shows tenant operations and their persisted records", async () => {
    const session = {
      csrfToken: "csrf-token",
      activeTenantId: "tenant-1",
      user: { id: "user-1", email: "owner@example.com", displayName: "Owner" },
    };
    const tenant = {
      id: "tenant-1",
      name: "Acme",
      status: "active",
      role: "owner",
      membershipStatus: "active",
      permissions: ["accounts:manage", "knowledge:manage", "members:manage"],
    };
    vi.stubGlobal("fetch", vi.fn().mockImplementation((input) => {
      if (input === "/api/auth/me") return Promise.resolve(jsonResponse(200, session));
      if (input === "/health") return Promise.resolve(jsonResponse(200, { status: "ok", database: "up" }));
      if (input === "/api/tenants") return Promise.resolve(jsonResponse(200, { platformRole: "", tenants: [tenant] }));
      if (input === "/api/members") return Promise.resolve(jsonResponse(200, { members: [] }));
      if (input === "/api/accounts") return Promise.resolve(jsonResponse(200, { accounts: [{ id: "account-1", name: "售前客服", accountKey: "wa-account", status: "pending", dailyLimit: 30, createdAt: "2026-07-14T04:00:00Z" }] }));
      if (input === "/api/knowledge/bases") return Promise.resolve(jsonResponse(200, { bases: [{ id: "base-1", name: "商品知识库", description: "商品与售后", status: "active", createdAt: "2026-07-14T04:00:00Z" }] }));
      if (input === "/api/conversations") return Promise.resolve(jsonResponse(200, { conversations: [] }));
      return Promise.reject(new Error(`unexpected request: ${input}`));
    }));

    render(<App />);

    expect(await screen.findByRole("heading", { name: "总览" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /客服账号.*Accounts/ }));
    expect(await screen.findByText("售前客服")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /知识库.*Knowledge/ }));
    expect(await screen.findByText("商品知识库")).toBeTruthy();
  });
});

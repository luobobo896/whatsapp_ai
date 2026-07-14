import { afterEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";
import { mount } from "@vue/test-utils";
import { createRouter, createWebHistory } from "vue-router";
import App from "./App.vue";

function jsonResponse(status, body) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: vi.fn().mockResolvedValue(body),
  };
}

async function mountApp() {
  const router = createRouter({
    history: createWebHistory(),
    routes: [
      { path: "/", component: { template: "<div>Dashboard</div>" } },
      { path: "/login", component: { template: "<div>LoginPage</div>" } },
    ],
  });
  router.push("/");
  await router.isReady();

  const wrapper = mount(App, {
    global: { plugins: [router] },
  });

  // Wait for onMounted async operations to complete
  await nextTick();
  await new Promise((r) => setTimeout(r, 50));
  await nextTick();

  return { wrapper, router };
}

afterEach(() => {
  vi.restoreAllMocks();
  window.history.replaceState(null, "", "/");
});

describe("App", () => {
  it("redirects to login page when there is no authenticated session", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(401, {
      error: { code: "AUTH_REQUIRED", message: "Authentication is required." },
    })));

    const { wrapper } = await mountApp();

    expect(wrapper.text()).toContain("LoginPage");
  });

  it("shows the dashboard when authenticated", async () => {
    const session = {
      csrfToken: "csrf-token",
      activeTenantId: null,
      user: { id: "user-1", email: "admin@example.com", displayName: "Admin" },
    };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(200, session)));

    const { wrapper } = await mountApp();

    expect(wrapper.text()).toContain("Dashboard");
  });
});

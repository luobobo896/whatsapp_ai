import { afterEach, describe, expect, it, vi } from "vitest";
import { flushPromises, shallowMount } from "@vue/test-utils";
import Dashboard from "./Dashboard.vue";

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  setSession: vi.fn(),
  routerReplace: vi.fn(),
  session: { __v_isRef: true, value: null },
}));

vi.mock("../api", () => ({
  APIError: class APIError extends Error {},
  get: mocks.get,
  post: mocks.post,
  messageForError: (error) => error.message,
}));

vi.mock("../composables/useSession", () => ({
  useSession: () => ({ session: mocks.session }),
  setSession: mocks.setSession,
}));

vi.mock("vue-router", () => ({
  useRouter: () => ({ replace: mocks.routerReplace }),
}));

function mountDashboard(showToast) {
  return shallowMount(Dashboard, {
    global: {
      provide: { showToast },
      stubs: {
        ElButton: { template: "<button v-bind='$attrs' @click='$emit(\"click\")'><slot /></button>" },
        ElMenu: { template: "<nav><slot /></nav>" },
        ElMenuItem: { template: "<div><slot /></div>" },
        ElMenuItemGroup: { template: "<div><slot /></div>" },
        ElSelect: { template: "<select><slot /></select>" },
        ElOption: { template: "<option />" },
      },
    },
  });
}

afterEach(() => {
  mocks.get.mockReset();
  mocks.post.mockReset();
  mocks.setSession.mockReset();
  mocks.routerReplace.mockReset();
  mocks.session.value = null;
});

describe("Dashboard sign out", () => {
  it("keeps the local session when server-side session revocation fails", async () => {
    mocks.session.value = {
      csrfToken: "csrf-token",
      activeTenantId: null,
      user: { email: "admin@example.com", displayName: "Admin" },
    };
    mocks.get.mockResolvedValue({ status: "ok", tenants: [] });
    mocks.post.mockRejectedValue(new Error("Failed to end session."));
    const showToast = vi.fn();
    const wrapper = mountDashboard(showToast);

    await flushPromises();
    mocks.setSession.mockClear();
    mocks.routerReplace.mockClear();
    await wrapper.find('[aria-label="退出登录"]').trigger("click");
    await flushPromises();

    expect(mocks.setSession).not.toHaveBeenCalled();
    expect(mocks.routerReplace).not.toHaveBeenCalled();
    expect(showToast).toHaveBeenCalledWith({ tone: "error", message: "Failed to end session." });
  });
});

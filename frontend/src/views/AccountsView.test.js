import { afterEach, describe, expect, it, vi } from "vitest";
import { shallowMount } from "@vue/test-utils";
import AccountsView from "./AccountsView.vue";
import { del, get } from "../api";
import { ElMessageBox } from "element-plus";

vi.mock("../api", () => ({
  del: vi.fn(),
  get: vi.fn(),
  messageForError: vi.fn((error) => error.message),
}));

vi.mock("element-plus", () => ({
  ElMessageBox: { confirm: vi.fn() },
}));

const account = {
  id: "account-1",
  name: "售前客服",
  accountKey: "wa_sales",
  status: "pending",
  kbId: [],
};

afterEach(() => {
  del.mockReset();
  get.mockReset();
  ElMessageBox.confirm.mockReset();
});

describe("AccountsView", () => {
  it("confirms permanent account deletion before refreshing the list", async () => {
    ElMessageBox.confirm.mockResolvedValue();
    del.mockResolvedValue(null);
    const wrapper = shallowMount(AccountsView, {
      props: {
        accounts: [],
        canManage: true,
        csrfToken: "csrf-token",
        knowledgeBases: [],
      },
      global: { provide: { showToast: vi.fn() } },
    });

    await wrapper.vm.removeAccount(account);

    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      expect.stringContaining("会话记录"),
      "确认删除",
      expect.objectContaining({ confirmButtonText: "删除" }),
    );
    expect(del).toHaveBeenCalledWith("/api/accounts/account-1", "csrf-token");
    expect(wrapper.emitted("changed")).toHaveLength(1);
  });

  it("reports slow deletion as background work", async () => {
    ElMessageBox.confirm.mockResolvedValue();
    del.mockResolvedValue({ status: "deleting" });
    get.mockResolvedValue({ accounts: [] });
    const showToast = vi.fn();
    const wrapper = shallowMount(AccountsView, {
      props: {
        accounts: [],
        canManage: true,
        csrfToken: "csrf-token",
        knowledgeBases: [],
      },
      global: { provide: { showToast } },
    });

    await wrapper.vm.removeAccount(account);

    expect(showToast).toHaveBeenCalledWith({ tone: "info", message: "客服账号正在后台删除" });
    expect(showToast).toHaveBeenCalledWith({ tone: "success", message: "客服账号已删除" });
    expect(get).toHaveBeenCalledWith("/api/accounts");
    expect(wrapper.emitted("changed")).toHaveLength(1);
  });
});

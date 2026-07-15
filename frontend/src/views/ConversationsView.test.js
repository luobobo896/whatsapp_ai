import { describe, expect, it, vi } from "vitest";
import { mount } from "@vue/test-utils";
import ConversationsView from "./ConversationsView.vue";

vi.mock("../api", () => ({ del: vi.fn() }));

const conversation = {
  conversationId: "+8613800000000",
  accountId: "account-1",
  customerName: "客户一号",
  lastMessage: "需要帮助",
  lastMessageAt: "2026-07-15 10:00:00",
  messageCount: 1,
};

const TableStub = {
  name: "TableStub",
  emits: ["row-click"],
  template: "<div><slot /></div>",
};

function mountConversations() {
  return mount(ConversationsView, {
    props: {
      conversations: [conversation],
      accounts: [{ id: "account-1", name: "售前客服" }],
      canManage: true,
      csrfToken: "csrf",
    },
    global: {
      provide: { showToast: vi.fn() },
      stubs: {
        ElCard: { template: "<section><slot name='header' /><slot /></section>" },
        ElTable: TableStub,
        ElTableColumn: { template: "<div />" },
        ElTag: { template: "<span><slot /></span>" },
        ElSelect: { template: "<select><slot /></select>" },
        ElOption: { template: "<option />" },
        ElEmpty: { template: "<div />" },
        ElPopconfirm: { template: "<div><slot name='reference' /></div>" },
        ElButton: { template: "<button><slot /></button>" },
      },
    },
  });
}

describe("ConversationsView", () => {
  it("opens a conversation in the shared chat workspace", async () => {
    const wrapper = mountConversations();

    await wrapper.findComponent(TableStub).vm.$emit("row-click", conversation);

    expect(wrapper.emitted("chat")).toEqual([[conversation]]);
    expect(wrapper.findComponent({ name: "ElDialog" }).exists()).toBe(false);
  });
});

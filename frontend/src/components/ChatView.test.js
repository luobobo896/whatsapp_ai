import { afterEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import ChatView from "./ChatView.vue";

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}));

vi.mock("../api", () => ({
  get: mocks.get,
  post: mocks.post,
  messageForError: (error) => error.message,
}));

function mountChat(initialConversationId = "") {
  return mount(ChatView, {
    props: {
      account: { id: "account-1", name: "售前客服", replyLimit: 30 },
      csrfToken: "csrf",
      initialConversationId,
    },
    global: {
      provide: { showToast: vi.fn() },
      stubs: {
        ElButton: { template: "<button @click='$emit(\"click\")'><slot /></button>" },
        ElTag: { template: "<span><slot /></span>" },
        ElInput: {
          props: ["modelValue"],
          template: "<textarea :value='modelValue' @input='$emit(\"update:modelValue\", $event.target.value)' />",
        },
        ElIcon: { template: "<span />" },
      },
      directives: { loading: {} },
    },
  });
}

afterEach(() => {
  mocks.get.mockReset();
  mocks.post.mockReset();
});

describe("ChatView", () => {
  it("opens the requested conversation when arriving from the conversation workspace", async () => {
    mocks.get.mockImplementation((path) => {
      if (path.startsWith("/api/conversations?")) {
        return Promise.resolve({ conversations: [{
          conversationId: "conv-1",
          customerName: "客户一号",
          lastMessage: "需要帮助",
          lastMessageAt: "2026-07-15 10:00:00",
          messageCount: 1,
        }] });
      }
      if (path.includes("/messages?")) {
        return Promise.resolve({ messages: [{
          id: "message-1",
          role: "customer",
          customerName: "客户一号",
          content: "需要帮助",
          createdAt: "2026-07-15 10:00:00",
        }] });
      }
      throw new Error(`unexpected path: ${path}`);
    });

    const wrapper = mountChat("conv-1");
    await flushPromises();

    expect(mocks.get).toHaveBeenCalledWith("/api/conversations/conv-1/messages?accountId=account-1&limit=30");
    expect(wrapper.text()).toContain("需要帮助");
    wrapper.unmount();
  });

  it("sends a manual reply through the account-scoped delivery endpoint", async () => {
    mocks.get.mockImplementation((path) => {
      if (path.startsWith("/api/conversations?")) {
        return Promise.resolve({ conversations: [{
          conversationId: "+8613800000000",
          customerName: "客户一号",
          lastMessage: "需要帮助",
          lastMessageAt: "2026-07-15 10:00:00",
          messageCount: 1,
        }] });
      }
      return Promise.resolve({ messages: [] });
    });
    mocks.post.mockResolvedValue({ id: "message-1" });

    const wrapper = mountChat("+8613800000000");
    await flushPromises();
    await wrapper.find("textarea").setValue("您好，我来协助您");
    await wrapper.findAll("button").find((button) => button.text() === "发送").trigger("click");
    await flushPromises();

    expect(mocks.post).toHaveBeenCalledWith(
      "/api/conversations/%2B8613800000000/send",
      {
        accountId: "account-1",
        customerName: "客户一号",
        content: "您好，我来协助您",
      },
      "csrf",
    );
    wrapper.unmount();
  });
});

import { afterEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import QrLoginCard from "./QrLoginCard.vue";
import { get, post } from "../api";

vi.mock("../api", () => ({
  get: vi.fn(),
  post: vi.fn(),
  messageForError: vi.fn((error) => error.message),
}));

const pngDataUrl = "data:image/png;base64,ZmFrZQ==";

afterEach(() => {
  vi.clearAllMocks();
});

describe("QrLoginCard", () => {
  it("renders the native PNG returned by OpenClaw without rebuilding it on canvas", async () => {
    post.mockResolvedValue({
      qrData: pngDataUrl,
      expiresAt: new Date(Date.now() + 30000).toISOString(),
    });
    get.mockResolvedValue({ status: "qr_pending" });

    const wrapper = mount(QrLoginCard, {
      props: {
        account: { id: "account-1", name: "客服一号", status: "pending" },
        csrfToken: "csrf-token",
      },
      global: {
        provide: { showToast: vi.fn() },
        stubs: {
          ElCard: { template: "<section><slot name='header' /><slot /></section>" },
          ElTag: { template: "<span><slot /></span>" },
          ElButton: { template: "<button @click='$emit(\"click\")'><slot /></button>" },
          ElProgress: { template: "<div />" },
        },
      },
    });

    await wrapper.find("button").trigger("click");
    await flushPromises();

    expect(wrapper.find("img").attributes("src")).toBe(pngDataUrl);
    expect(wrapper.find("canvas").exists()).toBe(false);
    wrapper.unmount();
  });
});

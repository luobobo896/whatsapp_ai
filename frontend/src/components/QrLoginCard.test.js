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
  vi.useRealTimers();
  get.mockReset();
  post.mockReset();
  vi.clearAllMocks();
});

function mountCard() {
  return mount(QrLoginCard, {
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
}

describe("QrLoginCard", () => {
  it("renders the native PNG returned by OpenClaw without rebuilding it on canvas", async () => {
    post.mockResolvedValue({
      qrData: pngDataUrl,
      expiresAt: new Date(Date.now() + 30000).toISOString(),
    });
    get.mockResolvedValue({ status: "qr_pending" });

    const wrapper = mountCard();

    await wrapper.find("button").trigger("click");
    await flushPromises();

    expect(wrapper.find("img").attributes("src")).toBe(pngDataUrl);
    expect(wrapper.find("canvas").exists()).toBe(false);
    wrapper.unmount();
  });

  it("reuses an already connected OpenClaw account without polling for a QR code", async () => {
    post.mockResolvedValue({ status: "connected", accountId: "account-1" });
    const wrapper = mountCard();

    await wrapper.find("button").trigger("click");
    await flushPromises();

    expect(wrapper.text()).toContain("WhatsApp 已连接");
    expect(wrapper.emitted("connected")?.length).toBeGreaterThan(0);
    expect(get).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("stops the QR countdown after scanning and keeps polling until connected", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-14T12:00:00Z"));
    post.mockResolvedValue({
      qrData: pngDataUrl,
      expiresAt: "2026-07-14T12:00:30Z",
    });
    get
      .mockResolvedValueOnce({ status: "connecting" })
      .mockResolvedValueOnce({ status: "connected" });

    const wrapper = mountCard();
    await wrapper.find("button").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("0:30");

    await vi.advanceTimersByTimeAsync(3000);
    await flushPromises();
    expect(wrapper.text()).toContain("正在连接 WhatsApp");
    expect(wrapper.text()).not.toContain("二维码有效期剩余");

    await vi.advanceTimersByTimeAsync(3000);
    await flushPromises();
    expect(wrapper.emitted("connected")).toHaveLength(1);
    expect(wrapper.text()).toContain("WhatsApp 已连接");
    wrapper.unmount();
  });

  it("waits up to one minute for connection after scanning", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-14T12:00:00Z"));
    post.mockResolvedValue({
      qrData: pngDataUrl,
      expiresAt: "2026-07-14T12:00:30Z",
    });
    get.mockResolvedValue({ status: "connecting" });

    const wrapper = mountCard();
    await wrapper.find("button").trigger("click");
    await flushPromises();

    await vi.advanceTimersByTimeAsync(3000);
    await flushPromises();
    await vi.advanceTimersByTimeAsync(59000);
    await flushPromises();
    expect(wrapper.text()).toContain("正在连接 WhatsApp");

    await vi.advanceTimersByTimeAsync(1000);
    await flushPromises();
    expect(wrapper.text()).toContain("连接超时，请重新获取二维码");
    wrapper.unmount();
  });

  it("shows the bridge failure returned by QR status polling", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-14T12:00:00Z"));
    post.mockResolvedValue({
      qrData: pngDataUrl,
      expiresAt: "2026-07-14T12:00:30Z",
    });
    get.mockResolvedValue({
      status: "expired",
      error: "OpenClaw gateway restart failed",
    });

    const wrapper = mountCard();
    await wrapper.find("button").trigger("click");
    await flushPromises();
    await vi.advanceTimersByTimeAsync(3000);
    await flushPromises();

    expect(wrapper.text()).toContain("OpenClaw gateway restart failed");
    wrapper.unmount();
  });

  it("does not overlap QR status requests when a poll is still pending", async () => {
    vi.useFakeTimers();
    post.mockResolvedValue({
      qrData: pngDataUrl,
      expiresAt: new Date(Date.now() + 30000).toISOString(),
    });
    let resolvePoll;
    get.mockImplementation(() => new Promise((resolve) => { resolvePoll = resolve; }));

    const wrapper = mountCard();
    await wrapper.find("button").trigger("click");
    await flushPromises();

    await vi.advanceTimersByTimeAsync(12000);
    expect(get).toHaveBeenCalledTimes(1);

    resolvePoll({ status: "qr_pending" });
    await flushPromises();
    await vi.advanceTimersByTimeAsync(3000);
    expect(get).toHaveBeenCalledTimes(2);
    wrapper.unmount();
  });

  it("ignores a pending QR status response after starting a new QR session", async () => {
    vi.useFakeTimers();
    const replacementQr = "data:image/png;base64,bmV3";
    post
      .mockResolvedValueOnce({ qrData: pngDataUrl, expiresAt: new Date(Date.now() + 30000).toISOString() })
      .mockResolvedValueOnce({ qrData: replacementQr, expiresAt: new Date(Date.now() + 30000).toISOString() });
    let resolvePoll;
    get.mockImplementation(() => new Promise((resolve) => { resolvePoll = resolve; }));

    const wrapper = mountCard();
    await wrapper.find("button").trigger("click");
    await flushPromises();
    await vi.advanceTimersByTimeAsync(3000);

    await wrapper.vm.fetchQr();
    await flushPromises();

    resolvePoll({ status: "connected" });
    await flushPromises();
    expect(wrapper.emitted("connected")).toBeUndefined();
    expect(wrapper.find("img").attributes("src")).toBe(replacementQr);
    wrapper.unmount();
  });
});

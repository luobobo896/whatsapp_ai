import { afterEach, describe, expect, it, vi } from "vitest";
import { flushPromises, shallowMount } from "@vue/test-utils";
import CreateAccountDialog from "./CreateAccountDialog.vue";
import EditAccountDialog from "./EditAccountDialog.vue";

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  patch: vi.fn(),
}));

vi.mock("../api", () => ({
  get: mocks.get,
  post: mocks.post,
  patch: mocks.patch,
  messageForError: (error) => error.message,
}));

function mountDialog(component, props, showToast) {
  return shallowMount(component, {
    props,
    global: { provide: { showToast } },
  });
}

afterEach(() => {
  mocks.get.mockReset();
  mocks.post.mockReset();
  mocks.patch.mockReset();
});

describe("account dialogs", () => {
  it("shows an error when the knowledge-base list cannot load during account creation", async () => {
    mocks.get.mockRejectedValue(new Error("Knowledge service unavailable."));
    const showToast = vi.fn();
    mountDialog(CreateAccountDialog, { csrfToken: "csrf" }, showToast);

    await flushPromises();

    expect(showToast).toHaveBeenCalledWith({ tone: "error", message: "Knowledge service unavailable." });
  });

  it("shows an error when the knowledge-base list cannot load during account editing", async () => {
    mocks.get.mockRejectedValue(new Error("Knowledge service unavailable."));
    const showToast = vi.fn();
    mountDialog(EditAccountDialog, {
      csrfToken: "csrf",
      account: { id: "account-1", name: "Support", dailyLimit: 30, replyLimit: 30, kbId: [] },
    }, showToast);

    await flushPromises();

    expect(showToast).toHaveBeenCalledWith({ tone: "error", message: "Knowledge service unavailable." });
  });
});

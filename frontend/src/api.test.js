import { afterEach, describe, expect, it, vi } from "vitest";
import { APIError, messageForError, post } from "./api";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("API mutations", () => {
  it("sends the CSRF token with same-origin credentials", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 204 });
    vi.stubGlobal("fetch", fetchMock);

    await post("/api/auth/logout", {}, "csrf-token");

    expect(fetchMock).toHaveBeenCalledWith("/api/auth/logout", {
      credentials: "same-origin",
      method: "POST",
      body: "{}",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": "csrf-token",
      },
    });
  });

  it("maps permission failures to an actionable Chinese message", () => {
    const error = new APIError("You do not have permission.", "FORBIDDEN", 403, "req_1");
    expect(messageForError(error)).toBe("当前账号没有执行此操作的权限。");
  });
});

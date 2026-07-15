import { afterEach, describe, expect, it, vi } from "vitest";
import { APIError, messageForError, post, postForm } from "./api";

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

  it("uploads form data without overriding its multipart boundary", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 204 });
    vi.stubGlobal("fetch", fetchMock);
    const body = new FormData();
    body.append("files", new File(["title,content\n示例,内容"], "knowledge.csv", { type: "text/csv" }));

    await postForm("/api/knowledge/bases/kb-1/import", body, "csrf-token");

    expect(fetchMock).toHaveBeenCalledWith("/api/knowledge/bases/kb-1/import", {
      credentials: "same-origin",
      method: "POST",
      body,
      headers: { "X-CSRF-Token": "csrf-token" },
    });
  });
});

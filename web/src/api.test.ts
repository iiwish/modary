import { afterEach, describe, expect, it, vi } from "vitest";
import { APIError, executeAction, getSession, newIdempotencyKey } from "./api";

afterEach(() => vi.unstubAllGlobals());

describe("action API client", () => {
  it("keeps the CSRF token for governed writes", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        actor: { id: "user_admin", type: "user", display_name: "Admin", workspace_id: "ws_default", roles: [], permissions: [] },
        csrf_token: "csrf-test",
        expires_at: "2030-01-01T00:00:00Z",
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ result: { ok: true }, summary: "ok", request_id: "req_1" }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    await getSession();
    await executeAction("test.execute", { value: 1 });
    const init = fetchMock.mock.calls[1][1] as RequestInit;
    expect(new Headers(init.headers).get("X-CSRF-Token")).toBe("csrf-test");
  });

  it("returns structured action errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { error_code: "AUTHZ_DENIED", human_readable_reason: "permission is missing" },
    }), { status: 403, headers: { "Content-Type": "application/json" } })));
    await expect(executeAction("test.execute", {})).rejects.toEqual(expect.any(APIError));
  });

  it("creates unique idempotency keys", () => {
    expect(newIdempotencyKey("run")).not.toBe(newIdempotencyKey("run"));
  });
});

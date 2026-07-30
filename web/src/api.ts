export type Actor = {
  id: string;
  type: "user" | "agent";
  display_name: string;
  workspace_id: string;
  roles: string[];
  permissions: string[];
};

export type Session = {
  actor: Actor;
  csrf_token: string;
  expires_at: string;
};

export type ActionError = {
  error_code: string;
  human_readable_reason: string;
  action_id?: string;
  required_permission?: string;
  request_id?: string;
};

let csrfToken = "";

export class APIError extends Error {
  readonly detail: ActionError;

  constructor(detail: ActionError) {
    super(detail.human_readable_reason);
    this.name = "APIError";
    this.detail = detail;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body) headers.set("Content-Type", "application/json");
  if (init?.method && init.method !== "GET" && csrfToken) {
    headers.set("X-CSRF-Token", csrfToken);
  }
  const response = await fetch(path, { ...init, headers, credentials: "same-origin" });
  if (response.status === 204) return undefined as T;
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const detail = payload.error ?? {
      error_code: "NETWORK_ERROR",
      human_readable_reason: `Request failed with status ${response.status}`,
    };
    throw new APIError(detail);
  }
  return payload as T;
}

export async function login(username: string, password: string): Promise<Session> {
  const session = await request<Session>("/api/auth/login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });
  csrfToken = session.csrf_token;
  return session;
}

export async function getSession(): Promise<Session> {
  const session = await request<Session>("/api/auth/session");
  csrfToken = session.csrf_token;
  return session;
}

export async function logout(): Promise<void> {
  await request<void>("/api/auth/logout", { method: "POST" });
  csrfToken = "";
}

type ExecuteEnvelope<T> = {
  result: T;
  summary: string;
  request_id: string;
};

export async function executeAction<T>(
  id: string,
  input: unknown,
  options: { planHash?: string; idempotencyKey?: string } = {},
): Promise<ExecuteEnvelope<T>> {
  return request<ExecuteEnvelope<T>>(`/api/actions/${id}/execute`, {
    method: "POST",
    body: JSON.stringify({
      input,
      plan_hash: options.planHash ?? "",
      idempotency_key: options.idempotencyKey ?? "",
    }),
  });
}

export type Preview<T> = {
  plan_hash: string;
  summary: T;
  impact: { rows?: number; resources?: string[] };
  expires_at: string;
};

export async function previewAction<T>(id: string, input: unknown): Promise<Preview<T>> {
  const response = await request<{ preview: Preview<T> }>(`/api/actions/${id}/preview`, {
    method: "POST",
    body: JSON.stringify({ input }),
  });
  return response.preview;
}

export async function queryAudit(filters: URLSearchParams): Promise<{ events: AuditEvent[] }> {
  return request<{ events: AuditEvent[] }>(`/api/audit?${filters.toString()}`);
}

export type AuditEvent = {
  request_id: string;
  actor_id: string;
  actor_type: string;
  channel: string;
  action_id: string;
  workspace_id: string;
  plan_hash?: string;
  decision: string;
  result_summary?: string;
  error_code?: string;
  reason?: string;
  started_at: string;
  finished_at: string;
};

export function newIdempotencyKey(prefix: string): string {
  return `${prefix}-${crypto.randomUUID()}`;
}

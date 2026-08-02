export type Actor = {
  id: string
  type: string
  display_name: string
  scope: { kind: string; id: string }
}

export type Session = {
  actor: Actor
  csrf_token: string
  expires_at: string
  request_id: string
}

export class APIError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly requestID = ''
  ) {
    super(message)
    this.name = 'APIError'
  }
}

let csrfToken = ''
let authenticationExpiredHandler: (() => void) | null = null

export function setCSRFToken(token: string) {
  csrfToken = token
}

export function setAuthenticationExpiredHandler(handler: (() => void) | null) {
  authenticationExpiredHandler = handler
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers)
  headers.set('Accept', 'application/json')
  if (options.body !== undefined) headers.set('Content-Type', 'application/json')
  if (csrfToken && options.method && !['GET', 'HEAD'].includes(options.method)) {
    headers.set('X-CSRF-Token', csrfToken)
  }
  const response = await fetch(path, { ...options, headers, credentials: 'same-origin' })
  if (response.status === 204) return undefined as T
  const payload = await response.json().catch(() => ({})) as any
  if (!response.ok) {
    const error = new APIError(response.status, payload.error?.code ?? 'REQUEST_FAILED', payload.error?.message ?? 'Request failed', payload.request_id ?? '')
    if (response.status === 401 && path !== '/api/auth/login' && path !== '/api/auth/session') {
      authenticationExpiredHandler?.()
    }
    throw error
  }
  return payload as T
}

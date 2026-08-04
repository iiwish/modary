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

export type AdminDescriptor = {
  id: string
  label: string
  path: `/${string}`
  icon: string
  order: number
  permissions?: string[]
  requiredPermissions?: string[]
}

export type AdminContext = {
  modules: AdminDescriptor[]
  grants: string[]
}

export type DecimalID = string

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

const localizedErrorMessages: Readonly<Record<string, string>> = {
  VALIDATION_FAILED: '请求内容无效，请检查后重试。',
  INVALID_QUERY: '筛选条件无效，请检查后重试。',
  AUTHENTICATION_FAILED: '用户名或密码错误。',
  AUTHENTICATION_REQUIRED: '请先登录。',
  CSRF_REQUIRED: '页面凭据已失效，请刷新后重试。',
  FORBIDDEN: '没有执行此操作的权限。',
  AUTHORIZATION_DENIED: '没有执行此操作的权限。',
  AUTHORIZATION_FAILED: '暂时无法完成权限校验。',
  NOT_FOUND: '请求的内容不存在。',
  VERSION_CONFLICT: '数据已发生变化，请刷新后重试。',
  TASKS_UNAVAILABLE: '暂时无法加载任务数据。',
  AUDIT_UNAVAILABLE: '暂时无法加载审计数据。',
  INTERNAL: '操作失败，请稍后重试。',
  REQUEST_FAILED: '请求失败，请稍后重试。',
}

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
    const code = payload.error?.code ?? 'REQUEST_FAILED'
    const message = localizedErrorMessages[code] ?? payload.error?.message ?? '请求失败，请稍后重试。'
    const error = new APIError(response.status, code, message, payload.request_id ?? '')
    if (response.status === 401 && path !== '/api/auth/login' && path !== '/api/auth/session') {
      authenticationExpiredHandler?.()
    }
    throw error
  }
  return payload as T
}

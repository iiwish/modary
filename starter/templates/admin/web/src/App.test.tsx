import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { describe, expect, it, vi } from 'vitest'
import { AdminApp, AdminRoutes } from './App'

const session = {
  actor: { id: 'admin', type: 'human', display_name: '管理员', scope: { kind: 'workspace', id: 'default' } },
  csrf_token: 'csrf-token', expires_at: '2030-01-01T00:00:00Z', request_id: 'req_test',
}
const adminContext = {
  modules: [{ id: 'records', label: '记录', path: '/records', icon: 'database', order: 100, permissions: ['records.create', 'records.delete', 'records.list', 'records.update'], requiredPermissions: ['records.list'] }],
  grants: ['records.create', 'records.delete', 'records.list', 'records.update'],
}

function json(status: number, value: unknown) {
  return new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })
}

function renderAt(path: string) {
  return render(<AdminApp router={<MemoryRouter initialEntries={[path]}><AdminRoutes /></MemoryRouter>} />)
}

describe('Admin routes', () => {
  it('redirects an unauthenticated protected route to sign in', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/meta') return json(200, { name: 'Example', version: '0.1.0' })
      if (path === '/api/auth/session') return json(401, { error: { code: 'AUTHENTICATION_REQUIRED', message: 'authentication is required' } })
      throw new Error(`unexpected request ${path}`)
    }))
    renderAt('/records')
    expect(await screen.findByRole('heading', { name: '登录' })).toBeTruthy()
  })

  it('restores an authenticated session before rendering a protected module', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: false,
      media: '(max-width: 800px)',
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })))
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/meta') return json(200, { name: 'Example', version: '0.1.0' })
      if (path === '/api/auth/session') return json(200, session)
      if (path === '/api/admin/context') return json(200, adminContext)
      if (path === '/api/records') return json(200, { records: [] })
      throw new Error(`unexpected request ${path}`)
    }))
    renderAt('/login')
    expect(await screen.findByRole('heading', { name: '记录' })).toBeTruthy()
    expect(screen.queryByRole('heading', { name: '登录' })).toBeNull()
  })

  it('moves focus to the selected page after mobile navigation closes', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: true,
      media: '(max-width: 800px)',
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })))
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { callback(0); return 1 })
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/meta') return json(200, { name: 'Example', version: '0.1.0' })
      if (path === '/api/auth/session') return json(200, session)
      if (path === '/api/admin/context') return json(200, adminContext)
      if (path === '/api/records') return json(200, { records: [] })
      throw new Error(`unexpected request ${path}`)
    }))
    const user = userEvent.setup()
    renderAt('/records')
    const heading = await screen.findByRole('heading', { name: '记录' })
    await user.click(screen.getByRole('button', { name: '打开导航' }))
    await user.click(screen.getByRole('link', { name: '记录' }))
    await waitFor(() => expect(document.activeElement).toBe(heading))
    expect(document.getElementById('primary-sidebar')?.getAttribute('aria-hidden')).toBe('true')
  })

  it('returns to sign in when an authenticated API session expires', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: false,
      media: '(max-width: 800px)',
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })))
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/meta') return json(200, { name: 'Example', version: '0.1.0' })
      if (path === '/api/auth/session') return json(200, session)
      if (path === '/api/admin/context') return json(200, adminContext)
      if (path === '/api/records') return json(401, { error: { code: 'AUTHENTICATION_REQUIRED', message: 'session is invalid or expired' } })
      throw new Error(`unexpected request ${path}`)
    }))
    renderAt('/records')
    expect(await screen.findByRole('heading', { name: '登录' })).toBeTruthy()
  })

  it('completes sign in and returns to the requested protected route', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: false,
      media: '(max-width: 800px)',
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })))
    const loginRequests: RequestInit[] = []
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/meta') return json(200, { name: 'Example', version: '0.1.0' })
      if (path === '/api/auth/session') return json(401, { error: { code: 'AUTHENTICATION_REQUIRED', message: 'authentication is required' } })
      if (path === '/api/auth/login') { loginRequests.push(init ?? {}); return json(200, session) }
      if (path === '/api/admin/context') return json(200, adminContext)
      if (path === '/api/records') return json(200, { records: [] })
      throw new Error(`unexpected request ${path}`)
    }))
    const user = userEvent.setup()
    renderAt('/records?status=active')
    await user.type(await screen.findByLabelText('用户名'), 'admin')
    await user.type(screen.getByLabelText('密码'), 'development-password')
    await user.click(screen.getByRole('button', { name: '登录' }))
    expect(await screen.findByRole('heading', { name: '记录' })).toBeTruthy()
    expect(loginRequests).toHaveLength(1)
    expect(loginRequests[0]?.method).toBe('POST')
    expect(loginRequests[0]?.body).toBe(JSON.stringify({ username: 'admin', password: 'development-password' }))
  })

  it('shows and retries an unexpected session initialization failure', async () => {
    let sessionRequests = 0
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/meta') return json(200, { name: 'Example', version: '0.1.0' })
      if (path === '/api/auth/session') {
        sessionRequests += 1
        if (sessionRequests === 1) return json(503, { error: { code: 'UNAVAILABLE', message: 'session service is unavailable' } })
        return json(401, { error: { code: 'AUTHENTICATION_REQUIRED', message: 'authentication is required' } })
      }
      throw new Error(`unexpected request ${path}`)
    }))
    const user = userEvent.setup()
    renderAt('/records')
    expect(await screen.findByRole('heading', { name: '应用暂时不可用' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByRole('heading', { name: '登录' })).toBeTruthy()
    expect(sessionRequests).toBe(2)
  })

  it('retries application metadata after an initialization failure', async () => {
    let metadataRequests = 0
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/meta') {
        metadataRequests += 1
        if (metadataRequests === 1) return json(503, { error: { code: 'UNAVAILABLE', message: 'metadata is unavailable' } })
        return json(200, { name: 'Example', version: '0.1.0' })
      }
      if (path === '/api/auth/session') return json(401, { error: { code: 'AUTHENTICATION_REQUIRED', message: 'authentication is required' } })
      throw new Error(`unexpected request ${path}`)
    }))
    const user = userEvent.setup()
    renderAt('/records')
    expect(await screen.findByRole('heading', { name: '应用暂时不可用' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByRole('heading', { name: '登录' })).toBeTruthy()
    expect(metadataRequests).toBe(2)
  })

  it('keeps the authenticated workspace visible when server-side logout fails', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: false,
      media: '(max-width: 800px)',
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })))
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/meta') return json(200, { name: 'Example', version: '0.1.0' })
      if (path === '/api/auth/session') return json(200, session)
      if (path === '/api/admin/context') return json(200, adminContext)
      if (path === '/api/records') return json(200, { records: [] })
      if (path === '/api/auth/logout') return json(500, { error: { code: 'INTERNAL', message: 'logout is unavailable' } })
      throw new Error(`unexpected request ${path}`)
    }))
    const user = userEvent.setup()
    renderAt('/records')
    expect(await screen.findByRole('heading', { name: '记录' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: '退出登录' }))
    expect((await screen.findByRole('status')).textContent).toContain('操作失败，请稍后重试。')
    expect(screen.getByRole('heading', { name: '记录' })).toBeTruthy()
  })
})

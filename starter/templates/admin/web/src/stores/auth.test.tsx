import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AppProvider } from './app'
import { AuthProvider, useAuth } from './auth'

const session = {
  actor: { id: 'admin', type: 'human', display_name: 'Administrator', scope: { kind: 'workspace', id: 'default' } },
  csrf_token: 'csrf-token', expires_at: '2030-01-01T00:00:00Z', request_id: 'req_test',
}

function response(status: number, value: unknown) {
  return new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })
}

function wrapper({ children }: { children: React.ReactNode }) {
  return <AppProvider><AuthProvider>{children}</AuthProvider></AppProvider>
}

describe('Auth provider', () => {
  it('restores a server-side session and sends its CSRF token on logout', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      if (path === '/api/meta') return response(200, { name: 'Example', version: '0.1.0' })
      if (path === '/api/auth/session') return response(200, session)
      if (path === '/api/auth/logout') {
        expect(new Headers(init?.headers).get('X-CSRF-Token')).toBe('csrf-token')
        return new Response(null, { status: 204 })
      }
      throw new Error(`unexpected request ${path}`)
    }))
    const { result } = renderHook(() => useAuth(), { wrapper })
    await act(() => result.current.initialize())
    expect(result.current.actor?.id).toBe('admin')
    await act(() => result.current.logout())
    expect(result.current.authenticated).toBe(false)
  })

  it('keeps an expected missing session unauthenticated and can sign in', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/meta') return response(200, { name: 'Example', version: '0.1.0' })
      if (path === '/api/auth/session') return response(401, { error: { code: 'AUTHENTICATION_REQUIRED', message: 'authentication is required' } })
      if (path === '/api/auth/login') return response(200, session)
      throw new Error(`unexpected request ${path}`)
    }))
    const { result } = renderHook(() => useAuth(), { wrapper })
    await act(() => result.current.initialize())
    expect(result.current.authenticated).toBe(false)
    await act(() => result.current.login('admin', 'development-password'))
    expect(result.current.actor?.display_name).toBe('Administrator')
  })

  it('keeps the authenticated state when server-side logout fails', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const path = String(input)
      if (path === '/api/meta') return response(200, { name: 'Example', version: '0.1.0' })
      if (path === '/api/auth/session') return response(200, session)
      if (path === '/api/auth/logout') return response(500, { error: { code: 'INTERNAL', message: 'logout is unavailable' } })
      throw new Error(`unexpected request ${path}`)
    }))
    const { result } = renderHook(() => useAuth(), { wrapper })
    await act(() => result.current.initialize())
    let logoutError: unknown
    await act(async () => {
      try {
        await result.current.logout()
      } catch (cause) {
        logoutError = cause
      }
    })
    expect(logoutError).toBeInstanceOf(Error)
    expect(result.current.authenticated).toBe(true)
    expect(result.current.actor?.id).toBe('admin')
  })
})

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { APIError, api, setAuthenticationExpiredHandler, setCSRFToken, type Actor, type AdminDescriptor, type AdminContext, type Session } from '@/api/client'
import { useApp } from './app'

type AuthContextValue = {
  actor: Actor | null
  modules: readonly AdminDescriptor[]
  grants: ReadonlySet<string>
  initialized: boolean
  initializationError: string
  busy: boolean
  authenticated: boolean
  can: (permission: string) => boolean
  initialize: () => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const { load } = useApp()
  const [actor, setActor] = useState<Actor | null>(null)
  const [modules, setModules] = useState<readonly AdminDescriptor[]>([])
  const [grants, setGrants] = useState<ReadonlySet<string>>(new Set())
  const [initialized, setInitialized] = useState(false)
  const [initializationError, setInitializationError] = useState('')
  const [busy, setBusy] = useState(false)
  const initializePromise = useRef<Promise<void> | null>(null)

  const accept = useCallback(async (session: Session) => {
    setCSRFToken(session.csrf_token)
    let admin: AdminContext
    try {
      admin = await api<AdminContext>('/api/admin/context')
    } catch (cause) {
      setCSRFToken('')
      throw cause
    }
    setActor(session.actor)
    setModules(admin.modules)
    setGrants(new Set(admin.grants))
  }, [])

  const expire = useCallback(() => {
    setActor(null)
    setModules([])
    setGrants(new Set())
    setCSRFToken('')
  }, [])

  useEffect(() => {
    setAuthenticationExpiredHandler(expire)
    return () => setAuthenticationExpiredHandler(null)
  }, [expire])

  const initialize = useCallback(() => {
    if (initializePromise.current) return initializePromise.current
    setInitialized(false)
    setInitializationError('')
    const request = Promise.all([
      load(),
      api<Session>('/api/auth/session').catch((error: unknown) => {
        if (!(error instanceof APIError) || error.status !== 401) throw error
        return null
      }),
    ]).then(async ([, session]) => {
      if (session) await accept(session)
    }).catch((cause: unknown) => {
      expire()
      setInitializationError(cause instanceof APIError ? cause.message : '应用服务暂时不可用，请稍后重试。')
    }).finally(() => {
      setInitialized(true)
      if (initializePromise.current === request) initializePromise.current = null
    })
    initializePromise.current = request
    return request
  }, [accept, expire, load])

  const logout = useCallback(async () => {
    setBusy(true)
    try {
      await api<void>('/api/auth/logout', { method: 'POST', body: '{}' })
      expire()
    } finally {
      setBusy(false)
    }
  }, [expire])

  const can = useCallback((permission: string) => grants.has(permission), [grants])
  const value = useMemo<AuthContextValue>(() => ({
    actor, modules, grants, initialized, initializationError, busy,
    authenticated: actor !== null, can, initialize, logout,
  }), [actor, busy, can, grants, initializationError, initialize, initialized, logout, modules])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) throw new Error('useAuth must be used inside AuthProvider')
  return value
}

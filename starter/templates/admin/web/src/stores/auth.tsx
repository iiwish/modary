import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { APIError, api, setAuthenticationExpiredHandler, setCSRFToken, type Actor, type Session } from '@/api/client'
import { useApp } from './app'

type AuthContextValue = {
  actor: Actor | null
  initialized: boolean
  initializationError: string
  busy: boolean
  authenticated: boolean
  initialize: () => Promise<void>
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const { load } = useApp()
  const [actor, setActor] = useState<Actor | null>(null)
  const [initialized, setInitialized] = useState(false)
  const [initializationError, setInitializationError] = useState('')
  const [busy, setBusy] = useState(false)
  const initializePromise = useRef<Promise<void> | null>(null)

  const accept = useCallback((session: Session) => {
    setActor(session.actor)
    setCSRFToken(session.csrf_token)
  }, [])

  const expire = useCallback(() => {
    setActor(null)
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
    ]).then(([, session]) => {
      if (session) accept(session)
    }).catch((cause: unknown) => {
      expire()
      setInitializationError(cause instanceof APIError
        ? cause.message
        : 'Application services are temporarily unavailable')
    }).finally(() => {
      setInitialized(true)
      if (initializePromise.current === request) initializePromise.current = null
    })
    initializePromise.current = request
    return request
  }, [accept, expire, load])

  const login = useCallback(async (username: string, password: string) => {
    setBusy(true)
    try {
      accept(await api<Session>('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password }),
      }))
    } finally {
      setBusy(false)
    }
  }, [accept])

  const logout = useCallback(async () => {
    setBusy(true)
    try {
      await api<void>('/api/auth/logout', { method: 'POST', body: '{}' })
      setActor(null)
      setCSRFToken('')
    } finally {
      setBusy(false)
    }
  }, [])

  const value = useMemo<AuthContextValue>(() => ({
    actor,
    initialized,
    initializationError,
    busy,
    authenticated: actor !== null,
    initialize,
    login,
    logout,
  }), [actor, busy, initializationError, initialize, initialized, login, logout])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) throw new Error('useAuth must be used inside AuthProvider')
  return value
}

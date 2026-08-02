import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { api } from '@/api/client'

type AppMetadata = { name: string; version: string }
type AppContextValue = AppMetadata & { loaded: boolean; load: () => Promise<void> }

const AppContext = createContext<AppContextValue | null>(null)

export function AppProvider({ children }: { children: ReactNode }) {
  const [metadata, setMetadata] = useState<AppMetadata>({ name: 'Admin', version: '' })
  const [loaded, setLoaded] = useState(false)
  const loadPromise = useRef<Promise<void> | null>(null)

  const load = useCallback(() => {
    if (loadPromise.current) return loadPromise.current
    const request = api<AppMetadata>('/api/meta')
      .then((result) => {
        const name = result.name || 'Admin'
        setMetadata({ name, version: result.version })
        document.title = `${name} Admin`
        setLoaded(true)
      })
      .catch((cause: unknown) => {
        loadPromise.current = null
        throw cause
      })
    loadPromise.current = request
    return request
  }, [])

  const value = useMemo(() => ({ ...metadata, loaded, load }), [loaded, load, metadata])
  return <AppContext.Provider value={value}>{children}</AppContext.Provider>
}

export function useApp() {
  const value = useContext(AppContext)
  if (!value) throw new Error('useApp must be used inside AppProvider')
  return value
}

import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { APIError, api } from '@/api/client'

export type RecordStatus = 'draft' | 'active' | 'archived'
export type RecordItem = {
  id: string
  title: string
  status: RecordStatus
  version: number
  created_at: string
  updated_at: string
}
export type RecordInput = { title: string; status: RecordStatus }

type RecordsContextValue = {
  items: readonly RecordItem[]
  loading: boolean
  loaded: boolean
  error: string
  errorStatus: number | null
  count: number
  load: () => Promise<void>
  create: (input: RecordInput) => Promise<RecordItem>
  update: (record: RecordItem, input: RecordInput) => Promise<RecordItem>
  remove: (record: RecordItem) => Promise<void>
}

const RecordsContext = createContext<RecordsContextValue | null>(null)

export function RecordsProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<RecordItem[]>([])
  const [loading, setLoading] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState('')
  const [errorStatus, setErrorStatus] = useState<number | null>(null)
  const loadPromise = useRef<Promise<void> | null>(null)

  const load = useCallback(() => {
    if (loadPromise.current) return loadPromise.current
    setLoading(true)
    setError('')
    setErrorStatus(null)
    loadPromise.current = api<{ records: RecordItem[] }>('/api/records')
      .then((result) => {
        setItems(result.records)
        setLoaded(true)
      })
      .catch((cause: unknown) => {
        setError(cause instanceof Error ? cause.message : 'Records could not be loaded')
        setErrorStatus(cause instanceof APIError ? cause.status : null)
        throw cause
      })
      .finally(() => {
        setLoading(false)
        loadPromise.current = null
      })
    return loadPromise.current
  }, [])

  const create = useCallback(async (input: RecordInput) => {
    const record = await api<RecordItem>('/api/records', { method: 'POST', body: JSON.stringify(input) })
    setItems((current) => [record, ...current])
    return record
  }, [])

  const update = useCallback(async (record: RecordItem, input: RecordInput) => {
    const response = await api<RecordItem>(`/api/records/${record.id}`, {
      method: 'PATCH',
      body: JSON.stringify({ ...input, version: record.version }),
    })
    setItems((current) => current.map((item) => item.id === record.id ? response : item))
    return response
  }, [])

  const remove = useCallback(async (record: RecordItem) => {
    await api<void>(`/api/records/${record.id}`, { method: 'DELETE', body: '{}' })
    setItems((current) => current.filter((item) => item.id !== record.id))
  }, [])

  const value = useMemo<RecordsContextValue>(() => ({
    items,
    loading,
    loaded,
    error,
    errorStatus,
    count: items.length,
    load,
    create,
    update,
    remove,
  }), [create, error, errorStatus, items, load, loaded, loading, remove, update])

  return <RecordsContext.Provider value={value}>{children}</RecordsContext.Provider>
}

export function useRecords() {
  const value = useContext(RecordsContext)
  if (!value) throw new Error('useRecords must be used inside RecordsProvider')
  return value
}

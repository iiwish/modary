import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { setCSRFToken } from '@/api/client'
import { RecordsProvider, useRecords, type RecordItem } from './store'

const record = { id: 'rec_000000000000000000000001', title: 'First', status: 'draft' as const, version: 1, created_at: '2026-08-02T00:00:00Z', updated_at: '2026-08-02T00:00:00Z' }

describe('Records provider', () => {
  it('loads and completes create, update, and delete with CSRF', async () => {
    setCSRFToken('csrf')
    const calls: Array<{ path: string; method: string; csrf: string | null }> = []
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      const method = init?.method ?? 'GET'
      calls.push({ path, method, csrf: new Headers(init?.headers).get('X-CSRF-Token') })
      if (method === 'GET') return new Response(JSON.stringify({ records: [record] }), { status: 200 })
      if (method === 'POST') return new Response(JSON.stringify({ ...record, id: 'rec_000000000000000000000002' }), { status: 201 })
      if (method === 'PATCH') return new Response(JSON.stringify({ ...record, id: 'rec_000000000000000000000002', title: 'Updated', status: 'active', version: 2, created_at: '2026-08-01T00:00:00Z' }), { status: 200 })
      return new Response(null, { status: 204 })
    }))
    const { result } = renderHook(() => useRecords(), { wrapper: RecordsProvider })
    await act(() => result.current.load())
    let created: RecordItem = record
    await act(async () => { created = await result.current.create({ title: 'Second', status: 'draft' }) })
    let updated: RecordItem = record
    await act(async () => { updated = await result.current.update(created, { title: 'Updated', status: 'active' }) })
    expect(updated.created_at).toBe('2026-08-01T00:00:00Z')
    await act(() => result.current.remove(result.current.items[0]))
    expect(calls.filter((call) => call.method !== 'GET').every((call) => call.csrf === 'csrf')).toBe(true)
    expect(result.current.items).toHaveLength(1)
  })
})

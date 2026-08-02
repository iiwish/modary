import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import axe from 'axe-core'
import { StrictMode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import ToastRegion from '@/components/ToastRegion'
import { ToastProvider } from '@/stores/toast'
import RecordsView from './RecordsView'

const record = { id: 'rec_000000000000000000000001', title: 'Access review', status: 'active', version: 1, created_at: '2026-08-02T00:00:00Z', updated_at: '2026-08-02T00:00:00Z' }

function renderRecords() {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ records: [record] }), { status: 200 })))
  return render(<StrictMode><ToastProvider><RecordsView /></ToastProvider></StrictMode>)
}

describe('Records view', () => {
  it('renders an accessible work table and complete row commands', async () => {
    const { container } = renderRecords()
    expect(await screen.findByRole('button', { name: 'Edit Access review' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Delete Access review' })).toBeTruthy()
    const result = await axe.run(container)
    expect(result.violations).toEqual([])
  })

  it('dismisses editor and delete dialogs with Escape', async () => {
    const user = userEvent.setup()
    renderRecords()
    const editTrigger = await screen.findByRole('button', { name: 'Access review' })
    await user.click(editTrigger)
    const editor = screen.getByRole('dialog', { name: 'Edit record' })
    expect(document.activeElement).toBe(screen.getByLabelText('Title'))
    fireEvent.keyDown(editor, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Edit record' })).toBeNull())
    expect(document.activeElement).toBe(editTrigger)

    await user.click(screen.getByRole('button', { name: 'Delete Access review' }))
    const deletion = screen.getByRole('dialog', { name: 'Delete record?' })
    fireEvent.keyDown(deletion, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Delete record?' })).toBeNull())
  })

  it('renders a dedicated forbidden state', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      error: { code: 'AUTHORIZATION_DENIED', message: 'permission is not granted' },
    }), { status: 403 })))
    render(<ToastProvider><RecordsView /><ToastRegion /></ToastProvider>)
    expect(await screen.findByRole('heading', { name: 'Access denied' })).toBeTruthy()
    expect(screen.getByText('permission is not granted')).toBeTruthy()
  })

  it('completes create, edit, filter, and delete through visible commands', async () => {
    const records = [record]
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const path = String(input)
      const method = init?.method ?? 'GET'
      if (path === '/api/records' && method === 'GET') return new Response(JSON.stringify({ records }), { status: 200 })
      if (path === '/api/records' && method === 'POST') {
        const created = { ...record, id: 'rec_000000000000000000000002', title: 'Second', status: 'draft', version: 1 }
        return new Response(JSON.stringify(created), { status: 201 })
      }
      if (path.endsWith('000000000000000000000002') && method === 'PATCH') {
        const updated = { ...record, id: 'rec_000000000000000000000002', title: 'Updated', status: 'active', version: 2 }
        return new Response(JSON.stringify(updated), { status: 200 })
      }
      if (path.endsWith('000000000000000000000002') && method === 'DELETE') return new Response(null, { status: 204 })
      throw new Error(`unexpected request ${method} ${path}`)
    }))
    const user = userEvent.setup()
    render(<ToastProvider><RecordsView /><ToastRegion /></ToastProvider>)
    await screen.findByRole('button', { name: 'Access review' })

    await user.click(screen.getByRole('button', { name: 'New record' }))
    await user.type(screen.getByLabelText('Title'), 'Second')
    await user.click(screen.getByRole('button', { name: 'Save record' }))
    expect(await screen.findByRole('button', { name: 'Second' })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Second' }))
    const title = screen.getByLabelText('Title')
    await user.clear(title)
    await user.type(title, 'Updated')
    await user.click(screen.getByLabelText('active'))
    await user.click(screen.getByRole('button', { name: 'Save record' }))
    expect(await screen.findByRole('button', { name: 'Updated' })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'archived' }))
    expect(screen.getByRole('heading', { name: 'No matching records' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'all' }))

    await user.click(screen.getByRole('button', { name: 'Delete Updated' }))
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Updated' })).toBeNull())
    expect(screen.getByText('Record deleted')).toBeTruthy()
  })
})

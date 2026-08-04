import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import axe from 'axe-core'
import { describe, expect, it, vi } from 'vitest'
import AuditView from './AuditView'

describe('Audit view', () => {
  it('renders scope-bound operational metadata without sensitive detail fields', async () => {
    const requests: string[] = []
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      requests.push(String(input))
      return new Response(JSON.stringify({ events: [{
        id: '9223372036854775807', request_id: 'req_123', actor_id: 'admin', actor_type: 'human', channel: 'http',
        action_id: 'record.update', decision: 'allowed', started_at: '2026-08-02T00:00:00Z', finished_at: '2026-08-02T00:00:01Z',
      }], next_before_id: '9223372036854775807' }), { status: 200 })
    }))
    const user = userEvent.setup()
    const { container } = render(<AuditView />)
    expect(await screen.findByRole('table', { name: '审计事件' })).toBeTruthy()
    expect(screen.getByText('record.update')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: '下一页' }))
    await waitFor(() => expect(requests.at(-1)).toContain('before_id=9223372036854775807'))
    expect(container.textContent).not.toMatch(/result summary|reason|input|resources/i)
    expect((await axe.run(container)).violations).toEqual([])
  })

  it('renders a dedicated forbidden state without a retry command', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      error: { code: 'FORBIDDEN', message: 'Audit access is not granted' },
    }), { status: 403 })))
    render(<AuditView />)
    expect(await screen.findByText('无权访问')).toBeTruthy()
    expect(screen.getByText('没有执行此操作的权限。')).toBeTruthy()
    expect(screen.queryByRole('button', { name: '重试' })).toBeNull()
  })

  it('renders a recoverable dependency failure', async () => {
    let requests = 0
    vi.stubGlobal('fetch', vi.fn(async () => {
      requests += 1
      return requests === 1
        ? new Response(JSON.stringify({ error: { code: 'AUDIT_UNAVAILABLE', message: 'Audit metadata could not be loaded' } }), { status: 503 })
        : new Response(JSON.stringify({ events: [] }), { status: 200 })
    }))
    const user = userEvent.setup()
    render(<AuditView />)
    expect(await screen.findByText('暂时无法加载审计数据。')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText('暂无审计事件')).toBeTruthy()
  })
})

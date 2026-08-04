import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import axe from 'axe-core'
import { describe, expect, it, vi } from 'vitest'
import TasksView from './TasksView'

describe('Tasks view', () => {
  it('renders bounded metadata without payloads and applies filters', async () => {
    const requests: string[] = []
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      requests.push(String(input))
      return new Response(JSON.stringify({ tasks: [{
        id: '9223372036854775807', kind: 'report.generate', queue: 'priority', state: 'queued', attempt: 0,
        max_attempts: 3, scheduled_at: '2026-08-02T00:00:00Z', created_at: '2026-08-02T00:00:00Z',
      }], next_before_id: '9223372036854775807' }), { status: 200 })
    }))
    const user = userEvent.setup()
    const { container } = render(<TasksView />)
    expect(await screen.findByRole('table', { name: '持久化任务' })).toBeTruthy()
    expect(screen.getByText('report.generate')).toBeTruthy()
    expect(screen.getByText('9223372036854775807')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: '下一页' }))
    await waitFor(() => expect(requests.at(-1)).toContain('before_id=9223372036854775807'))
    expect(container.textContent).not.toContain('payload')
    await user.type(screen.getByLabelText('队列'), 'priority')
    expect(screen.getByRole('option', { name: '等待中' })).toBeTruthy()
    await user.selectOptions(screen.getByLabelText('状态'), 'queued')
    await user.click(screen.getByRole('button', { name: '应用筛选' }))
    expect(requests.at(-1)).toContain('queue=priority')
    expect(requests.at(-1)).toContain('state=queued')
    expect((await axe.run(container)).violations).toEqual([])
  })

  it('renders a recoverable failure state', async () => {
    let requests = 0
    vi.stubGlobal('fetch', vi.fn(async () => {
      requests += 1
      return requests === 1
        ? new Response(JSON.stringify({ error: { code: 'TASKS_UNAVAILABLE', message: 'Task metadata could not be loaded' } }), { status: 503 })
        : new Response(JSON.stringify({ tasks: [] }), { status: 200 })
    }))
    const user = userEvent.setup()
    render(<TasksView />)
    expect(await screen.findByText('暂时无法加载任务数据。')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText('没有找到任务')).toBeTruthy()
  })

  it('renders a dedicated forbidden state without a retry command', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      error: { code: 'FORBIDDEN', message: 'Task access is not granted' },
    }), { status: 403 })))
    render(<TasksView />)
    expect(await screen.findByText('无权访问')).toBeTruthy()
    expect(screen.getByText('没有执行此操作的权限。')).toBeTruthy()
    expect(screen.queryByRole('button', { name: '重试' })).toBeNull()
  })

  it('ignores a stale refresh response after filters change', async () => {
    type Pending = { path: string; resolve: (response: Response) => void }
    const pending: Pending[] = []
    let requests = 0
    vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
      requests += 1
      if (requests === 1) {
        return Promise.resolve(new Response(JSON.stringify({ tasks: [{
          id: '3', kind: 'initial.task', queue: 'default', state: 'queued', attempt: 0,
          max_attempts: 3, scheduled_at: '2026-08-02T00:00:00Z', created_at: '2026-08-02T00:00:00Z',
        }] }), { status: 200 }))
      }
      return new Promise<Response>((resolve) => pending.push({ path: String(input), resolve }))
    }))
    const user = userEvent.setup()
    render(<TasksView />)
    expect(await screen.findByText('initial.task')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: '刷新任务' }))
    await waitFor(() => expect(pending).toHaveLength(1))
    await user.type(screen.getByLabelText('队列'), 'priority')
    await user.click(screen.getByRole('button', { name: '应用筛选' }))
    await waitFor(() => expect(pending).toHaveLength(2))
    expect(pending[1]?.path).toContain('queue=priority')

    await act(async () => pending[1]?.resolve(new Response(JSON.stringify({ tasks: [{
      id: '2', kind: 'filtered.task', queue: 'priority', state: 'queued', attempt: 0,
      max_attempts: 3, scheduled_at: '2026-08-02T00:00:00Z', created_at: '2026-08-02T00:00:00Z',
    }] }), { status: 200 })))
    expect(await screen.findByText('filtered.task')).toBeTruthy()

    await act(async () => pending[0]?.resolve(new Response(JSON.stringify({ tasks: [{
      id: '1', kind: 'stale.task', queue: 'default', state: 'queued', attempt: 0,
      max_attempts: 3, scheduled_at: '2026-08-02T00:00:00Z', created_at: '2026-08-02T00:00:00Z',
    }] }), { status: 200 })))
    await waitFor(() => expect(screen.queryByText('stale.task')).toBeNull())
    expect(screen.getByText('filtered.task')).toBeTruthy()
  })
})

import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { RefreshCw } from 'lucide-react'
import { APIError, api, type DecimalID } from '@/api/client'
import AdminPage from '@/components/admin/AdminPage'
import DataTable from '@/components/admin/DataTable'
import Pagination from '@/components/admin/Pagination'
import { EmptyState, ErrorState, ForbiddenState, LoadingState } from '@/components/admin/States'
import useLatestRequest from '@/components/admin/useLatestRequest'

type TaskSummary = {
  id: DecimalID; kind: string; queue: string; state: TaskState; attempt: number; max_attempts: number
  scheduled_at: string; created_at: string; finalized_at?: string
}
type TaskPage = { tasks: TaskSummary[]; next_before_id?: DecimalID }
type LoadFailure = { message: string; status?: number } | null
type TaskState = 'queued' | 'pending' | 'scheduled' | 'running' | 'retrying' | 'succeeded' | 'failed' | 'cancelled'
const taskStates: ReadonlyArray<{ value: TaskState; label: string }> = [
  { value: 'queued', label: '已入队' },
  { value: 'pending', label: '等待中' },
  { value: 'scheduled', label: '已计划' },
  { value: 'running', label: '运行中' },
  { value: 'retrying', label: '重试中' },
  { value: 'succeeded', label: '已成功' },
  { value: 'failed', label: '已失败' },
  { value: 'cancelled', label: '已取消' },
]
const taskStateLabels = Object.fromEntries(taskStates.map(({ value, label }) => [value, label])) as Record<TaskState, string>

export default function TasksView() {
  const [page, setPage] = useState<TaskPage>({ tasks: [] })
  const [loading, setLoading] = useState(true)
  const [failure, setFailure] = useState<LoadFailure>(null)
  const [queueInput, setQueueInput] = useState('')
  const [stateInput, setStateInput] = useState<'' | TaskState>('')
  const [filters, setFilters] = useState({ queue: '', state: '' })
  const [before, setBefore] = useState<DecimalID | ''>('')
  const [history, setHistory] = useState<Array<DecimalID | ''>>([])
  const { begin, cancel } = useLatestRequest()

  const load = useCallback(async () => {
    const request = begin()
    setLoading(true); setFailure(null)
    const query = new URLSearchParams({ limit: '50' })
    if (before) query.set('before_id', String(before))
    if (filters.queue) query.set('queue', filters.queue)
    if (filters.state) query.set('state', filters.state)
    try {
      const result = await api<TaskPage>(`/api/tasks?${query}`, { signal: request.signal })
      if (request.isCurrent()) setPage(result)
    }
    catch (cause) {
      if (!request.isCurrent()) return
      setFailure({
        message: cause instanceof APIError ? cause.message : '任务元数据暂时不可用。',
        status: cause instanceof APIError ? cause.status : undefined,
      })
    } finally {
      if (request.isCurrent()) {
        setLoading(false)
        request.release()
      }
    }
  }, [before, filters, begin])

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0)
    return () => { window.clearTimeout(timer); cancel() }
  }, [cancel, load])

  function applyFilters(event: FormEvent) {
    event.preventDefault(); setHistory([]); setBefore(''); setFilters({ queue: queueInput.trim(), state: stateInput })
  }

  return <AdminPage title="任务" summary="查看持久化任务的只读运行元数据" actions={
    <button className="icon-button" type="button" aria-label="刷新任务" title="刷新" disabled={loading} onClick={() => void load()}><RefreshCw size={17} className={loading ? 'spinning' : undefined} aria-hidden="true" /></button>
  }>
    <form className="operations-filter" onSubmit={applyFilters}>
      <label>队列<input value={queueInput} onChange={(event) => setQueueInput(event.target.value)} placeholder="全部队列" /></label>
      <label>状态<select value={stateInput} onChange={(event) => setStateInput(event.target.value as '' | TaskState)}><option value="">全部状态</option>{taskStates.map((state) => <option key={state.value} value={state.value}>{state.label}</option>)}</select></label>
      <button className="secondary-button" type="submit">应用筛选</button>
    </form>
    {loading && !page.tasks.length ? <LoadingState label="正在加载任务" /> : failure?.status === 403 ? <ForbiddenState message={failure.message} /> : failure ? <ErrorState message={failure.message} retry={() => void load()} /> : !page.tasks.length ? <EmptyState title="没有找到任务" message="当前筛选条件下没有持久化任务。" /> : <>
      <DataTable label="持久化任务" headings={['ID', '类型', '队列', '状态', '执行次数', '计划时间']}>
        {page.tasks.map((item) => <tr key={item.id}><td className="numeric-cell">{item.id}</td><td><strong>{item.kind}</strong></td><td>{item.queue}</td><td><span className={`status-chip status-${item.state}`}>{taskStateLabels[item.state]}</span></td><td>{item.attempt} / {item.max_attempts}</td><td>{formatDate(item.scheduled_at)}</td></tr>)}
      </DataTable>
      <Pagination busy={loading} canPrevious={history.length > 0} canNext={Boolean(page.next_before_id)} previous={() => { const next = [...history]; setBefore(next.pop() ?? ''); setHistory(next) }} next={() => { if (page.next_before_id) { setHistory((value) => [...value, before]); setBefore(page.next_before_id) } }} />
    </>}
  </AdminPage>
}

function formatDate(value: string) { const date = new Date(value); return Number.isNaN(date.valueOf()) ? '--' : new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(date) }

import { useCallback, useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { APIError, api, type DecimalID } from '@/api/client'
import AdminPage from '@/components/admin/AdminPage'
import DataTable from '@/components/admin/DataTable'
import Pagination from '@/components/admin/Pagination'
import { EmptyState, ErrorState, ForbiddenState, LoadingState } from '@/components/admin/States'
import useLatestRequest from '@/components/admin/useLatestRequest'

type AuditSummary = {
  id: DecimalID; request_id: string; actor_id?: string; actor_type?: string; channel?: string
  action_id: string; decision: string; error_code?: string; started_at: string; finished_at: string
}
type AuditPage = { events: AuditSummary[]; next_before_id?: DecimalID }
type LoadFailure = { message: string; status?: number } | null

export default function AuditView() {
  const [page, setPage] = useState<AuditPage>({ events: [] })
  const [loading, setLoading] = useState(true)
  const [failure, setFailure] = useState<LoadFailure>(null)
  const [before, setBefore] = useState<DecimalID | ''>('')
  const [history, setHistory] = useState<Array<DecimalID | ''>>([])
  const { begin, cancel } = useLatestRequest()
  const load = useCallback(async () => {
    const request = begin()
    setLoading(true); setFailure(null)
    const query = new URLSearchParams({ limit: '50' }); if (before) query.set('before_id', String(before))
    try {
      const result = await api<AuditPage>(`/api/audit?${query}`, { signal: request.signal })
      if (request.isCurrent()) setPage(result)
    }
    catch (cause) {
      if (!request.isCurrent()) return
      setFailure({
        message: cause instanceof APIError ? cause.message : '审计元数据暂时不可用。',
        status: cause instanceof APIError ? cause.status : undefined,
      })
    } finally {
      if (request.isCurrent()) {
        setLoading(false)
        request.release()
      }
    }
  }, [before, begin])
  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0)
    return () => { window.clearTimeout(timer); cancel() }
  }, [cancel, load])

  return <AdminPage title="审计日志" summary="查看当前作用域内的决策与执行结果" actions={
    <button className="icon-button" type="button" aria-label="刷新审计日志" title="刷新" disabled={loading} onClick={() => void load()}><RefreshCw size={17} className={loading ? 'spinning' : undefined} aria-hidden="true" /></button>
  }>
    {loading && !page.events.length ? <LoadingState label="正在加载审计日志" /> : failure?.status === 403 ? <ForbiddenState message={failure.message} /> : failure ? <ErrorState message={failure.message} retry={() => void load()} /> : !page.events.length ? <EmptyState title="暂无审计事件" message="当前作用域内受治理的操作会显示在这里。" /> : <>
      <DataTable label="审计事件" headings={['时间', '操作', '决策', '执行主体', '请求 ID']}>
        {page.events.map((event) => <tr key={event.id}><td>{formatDate(event.finished_at)}</td><td><strong>{event.action_id}</strong>{event.channel && <small>{event.channel}</small>}</td><td><span className={`status-chip status-${event.decision}`}>{event.decision}</span>{event.error_code && <small>{event.error_code}</small>}</td><td>{event.actor_id || '系统'}{event.actor_type && <small>{event.actor_type}</small>}</td><td className="request-cell" title={event.request_id}>{event.request_id}</td></tr>)}
      </DataTable>
      <Pagination busy={loading} canPrevious={history.length > 0} canNext={Boolean(page.next_before_id)} previous={() => { const next = [...history]; setBefore(next.pop() ?? ''); setHistory(next) }} next={() => { if (page.next_before_id) { setHistory((value) => [...value, before]); setBefore(page.next_before_id) } }} />
    </>}
  </AdminPage>
}

function formatDate(value: string) { const date = new Date(value); return Number.isNaN(date.valueOf()) ? '--' : new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(date) }

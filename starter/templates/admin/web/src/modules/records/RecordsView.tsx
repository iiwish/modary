import { useEffect, useMemo, useState } from 'react'
import { Archive, LockKeyhole, MoreHorizontal, Pencil, Plus, RefreshCw, Search, Trash2 } from 'lucide-react'
import { APIError } from '@/api/client'
import { useToast } from '@/stores/toast'
import { useAuth } from '@/stores/auth'
import DeleteDialog from './DeleteDialog'
import { recordFilterLabels } from './labels'
import RecordDialog from './RecordDialog'
import StatusBadge from './StatusBadge'
import { RecordsProvider, useRecords, type RecordInput, type RecordItem, type RecordStatus } from './store'

const filters: ReadonlyArray<'all' | RecordStatus> = ['all', 'draft', 'active', 'archived']

export function RecordsWorkspace() {
  const records = useRecords()
  const toast = useToast()
  const auth = useAuth()
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState<'all' | RecordStatus>('all')
  const [editing, setEditing] = useState<RecordItem | null>(null)
  const [deleting, setDeleting] = useState<RecordItem | null>(null)
  const [editorOpen, setEditorOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const forbidden = records.errorStatus === 403

  const { load, loaded } = records
  useEffect(() => {
    if (!loaded) void load().catch(() => undefined)
  }, [load, loaded])

  const visibleRecords = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase()
    return records.items.filter((record) => (
      (status === 'all' || record.status === status)
      && (!needle || record.title.toLocaleLowerCase().includes(needle))
    ))
  }, [query, records.items, status])

  function openCreate() {
    setEditing(null)
    setEditorOpen(true)
  }

  function openEdit(record: RecordItem) {
    setEditing(record)
    setEditorOpen(true)
  }

  async function save(input: RecordInput) {
    setBusy(true)
    try {
      if (editing) {
        await records.update(editing, input)
        toast.show('记录已更新')
      } else {
        await records.create(input)
        toast.show('记录已创建')
      }
      setEditorOpen(false)
    } catch (cause) {
      toast.show(cause instanceof APIError ? cause.message : '记录保存失败，请重试。', 'error')
    } finally {
      setBusy(false)
    }
  }

  async function remove() {
    if (!deleting) return
    setBusy(true)
    try {
      await records.remove(deleting)
      setDeleting(null)
      toast.show('记录已删除')
    } catch (cause) {
      toast.show(cause instanceof APIError ? cause.message : '记录删除失败，请重试。', 'error')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="records-page" aria-labelledby="records-title">
      <header className="page-header">
        <div><p className="eyebrow">工作区</p><h1 id="records-title" tabIndex={-1}>记录</h1><p className="page-summary">共 {records.count} 条</p></div>
        {!forbidden && auth.can('records.create') && <button className="primary-button" type="button" onClick={openCreate}><Plus size={17} aria-hidden="true" />新建记录</button>}
      </header>
      {!forbidden && <div className="records-toolbar">
        <div className="search-field">
          <Search size={17} aria-hidden="true" />
          <label className="sr-only" htmlFor="record-search">搜索记录</label>
          <input id="record-search" value={query} onChange={(event) => setQuery(event.target.value)} type="search" placeholder="搜索记录" />
        </div>
        <div className="filter-tabs" aria-label="按状态筛选记录">
          {filters.map((value) => (
            <button key={value} type="button" className={status === value ? 'active' : undefined} aria-pressed={status === value} onClick={() => setStatus(value)}>{recordFilterLabels[value]}</button>
          ))}
        </div>
        <button className="icon-button refresh-button" type="button" aria-label="刷新记录" title="刷新" disabled={records.loading} onClick={() => void records.load().catch(() => undefined)}>
          <RefreshCw size={17} className={records.loading ? 'spinning' : undefined} aria-hidden="true" />
        </button>
      </div>}

      {forbidden ? (
        <div className="empty-state forbidden-state" role="alert">
          <span className="empty-icon"><LockKeyhole size={24} aria-hidden="true" /></span>
          <h2>无权访问</h2>
          <p>{records.error}</p>
        </div>
      ) : records.error && !records.loading ? (
        <div className="inline-error" role="alert"><span>{records.error}</span><button type="button" onClick={() => void records.load().catch(() => undefined)}>重试</button></div>
      ) : records.loading && !records.loaded ? (
        <div className="loading-table" aria-label="正在加载记录">{Array.from({ length: 5 }, (_, index) => <span key={index} />)}</div>
      ) : visibleRecords.length ? (
        <div className="table-wrap">
          <table>
            <thead><tr><th>标题</th><th>状态</th><th>更新时间</th><th><span className="sr-only">操作</span></th></tr></thead>
            <tbody>
              {visibleRecords.map((record) => (
                <tr key={record.id}>
                  <td>{auth.can('records.update') ? <button className="record-title" type="button" onClick={() => openEdit(record)}>{record.title}</button> : <span className="record-title-static">{record.title}</span>}<small>{record.id}</small></td>
                  <td><StatusBadge status={record.status} /></td>
                  <td>{formatDate(record.updated_at)}</td>
                  <td className="row-actions">
                    {auth.can('records.update') && <button className="icon-button" type="button" aria-label={`编辑“${record.title}”`} title="编辑" onClick={() => openEdit(record)}><Pencil size={16} aria-hidden="true" /></button>}
                    {auth.can('records.delete') && <button className="icon-button danger-text" type="button" aria-label={`删除“${record.title}”`} title="删除" onClick={() => setDeleting(record)}><Trash2 size={16} aria-hidden="true" /></button>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="empty-state">
          <span className="empty-icon">{records.items.length ? <Archive size={24} aria-hidden="true" /> : <MoreHorizontal size={24} aria-hidden="true" />}</span>
          <h2>{records.items.length ? '没有符合条件的记录' : '暂无记录'}</h2>
          <p>{records.items.length ? '请调整搜索内容或状态筛选条件。' : '为当前工作区创建第一条记录。'}</p>
          {!records.items.length && auth.can('records.create') && <button className="secondary-button" type="button" onClick={openCreate}><Plus size={16} aria-hidden="true" />新建记录</button>}
        </div>
      )}

      {editorOpen && <RecordDialog record={editing} busy={busy} onClose={() => setEditorOpen(false)} onSubmit={(input) => void save(input)} />}
      {deleting && <DeleteDialog record={deleting} busy={busy} onClose={() => setDeleting(null)} onConfirm={() => void remove()} />}
    </section>
  )
}

export default function RecordsView() {
  return <RecordsProvider><RecordsWorkspace /></RecordsProvider>
}

function formatDate(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.valueOf()) ? '--' : new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium' }).format(date)
}

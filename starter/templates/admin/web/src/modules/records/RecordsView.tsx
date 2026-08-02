import { useEffect, useMemo, useState } from 'react'
import { Archive, LockKeyhole, MoreHorizontal, Pencil, Plus, RefreshCw, Search, Trash2 } from 'lucide-react'
import { APIError } from '@/api/client'
import { useToast } from '@/stores/toast'
import DeleteDialog from './DeleteDialog'
import RecordDialog from './RecordDialog'
import StatusBadge from './StatusBadge'
import { RecordsProvider, useRecords, type RecordInput, type RecordItem, type RecordStatus } from './store'

const filters: ReadonlyArray<'all' | RecordStatus> = ['all', 'draft', 'active', 'archived']

export function RecordsWorkspace() {
  const records = useRecords()
  const toast = useToast()
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
        toast.show('Record updated')
      } else {
        await records.create(input)
        toast.show('Record created')
      }
      setEditorOpen(false)
    } catch (cause) {
      toast.show(cause instanceof APIError ? cause.message : 'Record could not be saved', 'error')
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
      toast.show('Record deleted')
    } catch (cause) {
      toast.show(cause instanceof APIError ? cause.message : 'Record could not be deleted', 'error')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="records-page" aria-labelledby="records-title">
      <header className="page-header">
        <div><p className="eyebrow">Workspace</p><h1 id="records-title">Records</h1><p className="page-summary">{records.count} total</p></div>
        {!forbidden && <button className="primary-button" type="button" onClick={openCreate}><Plus size={17} aria-hidden="true" />New record</button>}
      </header>
      {!forbidden && <div className="records-toolbar">
        <div className="search-field">
          <Search size={17} aria-hidden="true" />
          <label className="sr-only" htmlFor="record-search">Search records</label>
          <input id="record-search" value={query} onChange={(event) => setQuery(event.target.value)} type="search" placeholder="Search records" />
        </div>
        <div className="filter-tabs" aria-label="Filter records by status">
          {filters.map((value) => (
            <button key={value} type="button" className={status === value ? 'active' : undefined} aria-pressed={status === value} onClick={() => setStatus(value)}>{value}</button>
          ))}
        </div>
        <button className="icon-button refresh-button" type="button" aria-label="Refresh records" title="Refresh" disabled={records.loading} onClick={() => void records.load().catch(() => undefined)}>
          <RefreshCw size={17} className={records.loading ? 'spinning' : undefined} aria-hidden="true" />
        </button>
      </div>}

      {forbidden ? (
        <div className="empty-state forbidden-state" role="alert">
          <span className="empty-icon"><LockKeyhole size={24} aria-hidden="true" /></span>
          <h2>Access denied</h2>
          <p>{records.error}</p>
        </div>
      ) : records.error && !records.loading ? (
        <div className="inline-error" role="alert"><span>{records.error}</span><button type="button" onClick={() => void records.load().catch(() => undefined)}>Try again</button></div>
      ) : records.loading && !records.loaded ? (
        <div className="loading-table" aria-label="Loading records">{Array.from({ length: 5 }, (_, index) => <span key={index} />)}</div>
      ) : visibleRecords.length ? (
        <div className="table-wrap">
          <table>
            <thead><tr><th>Title</th><th>Status</th><th>Updated</th><th><span className="sr-only">Actions</span></th></tr></thead>
            <tbody>
              {visibleRecords.map((record) => (
                <tr key={record.id}>
                  <td><button className="record-title" type="button" onClick={() => openEdit(record)}>{record.title}</button><small>{record.id}</small></td>
                  <td><StatusBadge status={record.status} /></td>
                  <td>{formatDate(record.updated_at)}</td>
                  <td className="row-actions">
                    <button className="icon-button" type="button" aria-label={`Edit ${record.title}`} title="Edit" onClick={() => openEdit(record)}><Pencil size={16} aria-hidden="true" /></button>
                    <button className="icon-button danger-text" type="button" aria-label={`Delete ${record.title}`} title="Delete" onClick={() => setDeleting(record)}><Trash2 size={16} aria-hidden="true" /></button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="empty-state">
          <span className="empty-icon">{records.items.length ? <Archive size={24} aria-hidden="true" /> : <MoreHorizontal size={24} aria-hidden="true" />}</span>
          <h2>{records.items.length ? 'No matching records' : 'No records yet'}</h2>
          <p>{records.items.length ? 'Adjust the search or status filter.' : 'Create the first record for this workspace.'}</p>
          {!records.items.length && <button className="secondary-button" type="button" onClick={openCreate}><Plus size={16} aria-hidden="true" />New record</button>}
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
  return Number.isNaN(date.valueOf()) ? '--' : new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(date)
}

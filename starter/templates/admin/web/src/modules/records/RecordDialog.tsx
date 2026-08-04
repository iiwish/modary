import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { X } from 'lucide-react'
import { recordStatusLabels } from './labels'
import type { RecordInput, RecordItem, RecordStatus } from './store'

type Props = {
  record: RecordItem | null
  busy: boolean
  onClose: () => void
  onSubmit: (value: RecordInput) => void
}

const statuses: readonly RecordStatus[] = ['draft', 'active', 'archived']

export default function RecordDialog({ record, busy, onClose, onSubmit }: Props) {
  const [title, setTitle] = useState(record?.title ?? '')
  const [status, setStatus] = useState<RecordStatus>(record?.status ?? 'draft')
  const dialog = useRef<HTMLDialogElement>(null)
  const titleInput = useRef<HTMLInputElement>(null)
  const returnFocus = useRef<HTMLElement | null>(null)

  useEffect(() => {
    returnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    if (dialog.current && !dialog.current.open) dialog.current.showModal()
    titleInput.current?.focus()
    return () => returnFocus.current?.focus()
  }, [])

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const normalized = title.trim()
    if (normalized) onSubmit({ title: normalized, status })
  }

  function handleKeyDown(event: KeyboardEvent<HTMLDialogElement>) {
    if (event.key !== 'Escape') return
    event.preventDefault()
    event.stopPropagation()
    onClose()
  }

  return (
    <dialog ref={dialog} className="modal" aria-labelledby="record-dialog-title" onCancel={(event) => { event.preventDefault(); onClose() }} onKeyDown={handleKeyDown}>
      <form className="modal-content" onSubmit={submit}>
        <header className="modal-header">
          <div><p className="eyebrow">记录</p><h2 id="record-dialog-title">{record ? '编辑记录' : '新建记录'}</h2></div>
          <button className="icon-button" type="button" aria-label="关闭对话框" onClick={onClose}><X size={19} aria-hidden="true" /></button>
        </header>
        <div className="form-field">
          <label htmlFor="record-title">标题</label>
          <input id="record-title" ref={titleInput} value={title} onChange={(event) => setTitle(event.target.value)} maxLength={200} required />
        </div>
        <fieldset className="form-field">
          <legend>状态</legend>
          <div className="segmented-control">
            {statuses.map((value) => (
              <label key={value}>
                <input checked={status === value} onChange={() => setStatus(value)} type="radio" name="record-status" value={value} />
                <span>{recordStatusLabels[value]}</span>
              </label>
            ))}
          </div>
        </fieldset>
        <footer className="modal-footer">
          <button className="secondary-button" type="button" onClick={onClose}>取消</button>
          <button className="primary-button" type="submit" disabled={busy || !title.trim()}>{busy ? '正在保存...' : '保存记录'}</button>
        </footer>
      </form>
    </dialog>
  )
}

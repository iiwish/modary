import { useEffect, useRef, type KeyboardEvent } from 'react'
import { Trash2, X } from 'lucide-react'
import type { RecordItem } from './store'

type Props = {
  record: RecordItem
  busy: boolean
  onClose: () => void
  onConfirm: () => void
}

export default function DeleteDialog({ record, busy, onClose, onConfirm }: Props) {
  const dialog = useRef<HTMLDialogElement>(null)
  const cancelButton = useRef<HTMLButtonElement>(null)
  const returnFocus = useRef<HTMLElement | null>(null)

  useEffect(() => {
    returnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    if (dialog.current && !dialog.current.open) dialog.current.showModal()
    cancelButton.current?.focus()
    return () => returnFocus.current?.focus()
  }, [])

  function handleKeyDown(event: KeyboardEvent<HTMLDialogElement>) {
    if (event.key !== 'Escape') return
    event.preventDefault()
    event.stopPropagation()
    onClose()
  }

  return (
    <dialog ref={dialog} className="modal compact-modal" aria-labelledby="delete-title" onCancel={(event) => { event.preventDefault(); onClose() }} onKeyDown={handleKeyDown}>
      <div className="modal-content">
        <header className="modal-header">
          <span className="danger-icon"><Trash2 size={20} aria-hidden="true" /></span>
          <button className="icon-button" type="button" aria-label="关闭对话框" onClick={onClose}><X size={19} aria-hidden="true" /></button>
        </header>
        <h2 id="delete-title">删除这条记录？</h2>
        <p className="modal-copy">记录 <strong>{record.title}</strong> 将被永久删除。</p>
        <footer className="modal-footer">
          <button ref={cancelButton} className="secondary-button" type="button" onClick={onClose}>取消</button>
          <button className="danger-button" type="button" disabled={busy} onClick={onConfirm}>{busy ? '正在删除...' : '删除'}</button>
        </footer>
      </div>
    </dialog>
  )
}

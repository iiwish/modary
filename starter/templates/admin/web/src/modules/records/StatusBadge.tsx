import type { RecordStatus } from './store'

export default function StatusBadge({ status }: { status: RecordStatus }) {
  return <span className={`status-badge ${status}`}><span aria-hidden="true" />{status}</span>
}

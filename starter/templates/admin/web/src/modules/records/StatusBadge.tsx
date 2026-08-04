import type { RecordStatus } from './store'
import { recordStatusLabels } from './labels'

export default function StatusBadge({ status }: { status: RecordStatus }) {
  return <span className={`status-badge ${status}`}><span aria-hidden="true" />{recordStatusLabels[status]}</span>
}

import { ChevronLeft, ChevronRight } from 'lucide-react'

export default function Pagination({ canPrevious, canNext, busy, previous, next }: {
  canPrevious: boolean
  canNext: boolean
  busy: boolean
  previous: () => void
  next: () => void
}) {
  if (!canPrevious && !canNext) return null
  return <nav className="table-pagination" aria-label="表格分页">
    <button type="button" disabled={!canPrevious || busy} onClick={previous}><ChevronLeft size={16} aria-hidden="true" />上一页</button>
    <button type="button" disabled={!canNext || busy} onClick={next}>下一页<ChevronRight size={16} aria-hidden="true" /></button>
  </nav>
}

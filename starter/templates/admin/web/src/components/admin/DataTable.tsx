import type { ReactNode } from 'react'

export default function DataTable({ label, headings, children }: {
  label: string
  headings: readonly ReactNode[]
  children: ReactNode
}) {
  return (
    <div className="table-wrap operations-table">
      <table aria-label={label}>
        <thead><tr>{headings.map((heading, index) => <th key={index}>{heading}</th>)}</tr></thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  )
}

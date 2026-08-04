import { useId, type ReactNode } from 'react'

export default function AdminPage({ title, summary, actions, children }: {
  title: string
  summary: string
  actions?: ReactNode
  children: ReactNode
}) {
  const id = `admin-page-${useId().replaceAll(':', '')}`
  return (
    <section className="admin-page" aria-labelledby={id}>
      <header className="page-header">
        <div><p className="eyebrow">运行管理</p><h1 id={id} tabIndex={-1}>{title}</h1><p className="page-summary">{summary}</p></div>
        {actions && <div className="page-actions">{actions}</div>}
      </header>
      {children}
    </section>
  )
}

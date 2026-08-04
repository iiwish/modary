import { AlertTriangle, Inbox, LoaderCircle, LockKeyhole } from 'lucide-react'

export function LoadingState({ label }: { label: string }) {
  return <div className="operation-state" aria-label={label}><LoaderCircle className="spinning" size={22} aria-hidden="true" /><span>{label}</span></div>
}

export function ErrorState({ message, retry }: { message: string; retry: () => void }) {
  return <div className="operation-state error-state" role="alert"><AlertTriangle size={22} aria-hidden="true" /><strong>数据加载失败</strong><span>{message}</span><button type="button" onClick={retry}>重试</button></div>
}

export function ForbiddenState({ message }: { message: string }) {
  return <div className="operation-state forbidden-state" role="alert"><LockKeyhole size={22} aria-hidden="true" /><strong>无权访问</strong><span>{message}</span></div>
}

export function EmptyState({ title, message }: { title: string; message: string }) {
  return <div className="operation-state"><Inbox size={23} aria-hidden="true" /><strong>{title}</strong><span>{message}</span></div>
}

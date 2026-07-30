import { AlertTriangle, CheckCircle2, X } from "lucide-react";
import type { ReactNode } from "react";
import { APIError } from "./api";

export function PageHeader({ eyebrow, title, meta, actions }: { eyebrow?: string; title: string; meta?: ReactNode; actions?: ReactNode }) {
  return <header className="page-header"><div>{eyebrow && <span className="eyebrow">{eyebrow}</span>}<h1>{title}</h1>{meta && <div className="page-meta">{meta}</div>}</div>{actions && <div className="header-actions">{actions}</div>}</header>;
}

export function StatusBadge({ status }: { status: string }) {
  const normalized = status.toLowerCase();
  return <span className={`status-badge ${normalized}`}><span />{status}</span>;
}

export function ErrorNotice({ error }: { error: unknown }) {
  const detail = error instanceof APIError ? error.detail : null;
  const message = error instanceof Error ? error.message : "The operation failed";
  return <div className="notice error" role="alert"><AlertTriangle size={18} /><div><strong>{detail?.error_code ?? "ERROR"}</strong><span>{message}</span>{detail?.request_id && <code>{detail.request_id}</code>}</div></div>;
}

export function SuccessNotice({ children }: { children: ReactNode }) {
  return <div className="notice success"><CheckCircle2 size={18} /><span>{children}</span></div>;
}

export function Modal({ title, children, onClose, footer }: { title: string; children: ReactNode; onClose: () => void; footer?: ReactNode }) {
  return <div className="modal-backdrop" role="presentation" onMouseDown={onClose}><section className="modal" role="dialog" aria-modal="true" aria-label={title} onMouseDown={(event) => event.stopPropagation()}><header><h2>{title}</h2><button className="icon-button" onClick={onClose} aria-label="Close"><X size={18} /></button></header><div className="modal-body">{children}</div>{footer && <footer>{footer}</footer>}</section></div>;
}

export function EmptyState({ icon, title, action }: { icon: ReactNode; title: string; action?: ReactNode }) {
  return <div className="empty-state"><div>{icon}</div><h2>{title}</h2>{action}</div>;
}

export function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

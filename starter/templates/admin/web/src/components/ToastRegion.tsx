import { CheckCircle2, CircleAlert, X } from 'lucide-react'
import { useToast } from '@/stores/toast'

export default function ToastRegion() {
  const toast = useToast()
  return (
    <div className="toast-region" aria-live="polite" aria-atomic="false">
      {toast.items.map((item) => (
        <div key={item.id} className={`toast ${item.tone}`} role="status">
          {item.tone === 'success'
            ? <CheckCircle2 size={18} aria-hidden="true" />
            : <CircleAlert size={18} aria-hidden="true" />}
          <span>{item.message}</span>
          <button className="icon-button compact" type="button" aria-label="Dismiss notification" onClick={() => toast.dismiss(item.id)}>
            <X size={15} aria-hidden="true" />
          </button>
        </div>
      ))}
    </div>
  )
}

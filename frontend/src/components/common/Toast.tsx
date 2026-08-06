import { Icon } from './Icon'

export interface ToastMessage {
  id: number
  message: string
  tone: 'success' | 'info' | 'warning'
}

export function ToastStack({ toasts }: { toasts: ToastMessage[] }) {
  return (
    <div aria-live="polite" className="toast-stack">
      {toasts.map((toast) => (
        <div className={`toast toast--${toast.tone}`} key={toast.id}>
          <span className="toast__icon">
            <Icon name={toast.tone === 'warning' ? 'alert' : 'check'} size={16} />
          </span>
          <span>{toast.message}</span>
        </div>
      ))}
    </div>
  )
}

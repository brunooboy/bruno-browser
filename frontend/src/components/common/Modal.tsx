import { useEffect, type ReactNode } from 'react'
import { Icon } from './Icon'

interface ModalProps {
  children: ReactNode
  description?: string
  onClose: () => void
  open: boolean
  title: string
  width?: 'md' | 'lg'
}

export function Modal({
  children,
  description,
  onClose,
  open,
  title,
  width = 'md',
}: ModalProps) {
  useEffect(() => {
    if (!open) return
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.body.style.overflow = 'hidden'
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      document.body.style.overflow = ''
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [onClose, open])

  if (!open) return null

  return (
    <div className="modal-shell" role="presentation">
      <button aria-label="Fechar modal" className="modal-backdrop" onClick={onClose} />
      <section
        aria-modal="true"
        className={`modal-panel ${width === 'lg' ? 'modal-panel--lg' : ''}`}
        role="dialog"
      >
        <header className="modal-header">
          <div>
            <span className="eyebrow">CONFIGURAÇÃO LOCAL</span>
            <h2>{title}</h2>
            {description && <p>{description}</p>}
          </div>
          <button aria-label="Fechar" className="icon-button" onClick={onClose} type="button">
            <Icon name="x" />
          </button>
        </header>
        {children}
      </section>
    </div>
  )
}

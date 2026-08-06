import { useState } from 'react'
import type { ProfileTag } from '../../types/profile'
import { Icon } from '../common/Icon'
import { Modal } from '../common/Modal'
import { TagBadge } from '../tags/TagBadge'

const colors = ['#36f58b', '#70a5ff', '#30d5c8', '#f5b942', '#ff8a4c', '#bd7cff', '#ff5f6d']

interface TagManagerModalProps {
  onAdd: (label: string, color: string) => void
  onClose: () => void
  onRemove: (id: string) => void
  open: boolean
  tags: ProfileTag[]
}

export function TagManagerModal({ onAdd, onClose, onRemove, open, tags }: TagManagerModalProps) {
  const [label, setLabel] = useState('')
  const [color, setColor] = useState(colors[0])
  const [error, setError] = useState('')

  const handleAdd = (event: React.FormEvent) => {
    event.preventDefault()
    const trimmed = label.trim()
    if (trimmed.length < 2) {
      setError('Digite pelo menos 2 caracteres.')
      return
    }
    if (tags.some((tag) => tag.label.toLocaleLowerCase('pt-BR') === trimmed.toLocaleLowerCase('pt-BR'))) {
      setError('Já existe uma tag com este nome.')
      return
    }
    onAdd(trimmed, color)
    setLabel('')
    setError('')
  }

  return (
    <Modal description="Crie classificações reutilizáveis para o inventário." onClose={onClose} open={open} title="Central de tags">
      <div className="tag-manager">
        <section className="tag-manager__section">
          <div className="tag-manager__heading"><div><span className="eyebrow">BIBLIOTECA</span><h3>Tags disponíveis</h3></div><b>{tags.length.toString().padStart(2, '0')}</b></div>
          <div className="tag-manager__list">
            {tags.map((tag) => (
              <div key={tag.id}>
                <TagBadge tag={tag} />
                <span>{tag.kind === 'status' ? 'PREDEFINIDA' : 'PERSONALIZADA'}</span>
                {tag.kind === 'custom' ? (
                  <button aria-label={`Excluir ${tag.label}`} onClick={() => onRemove(tag.id)} type="button"><Icon name="trash" size={14} /></button>
                ) : (
                  <Icon className="tag-manager__lock" name="shield" size={14} />
                )}
              </div>
            ))}
          </div>
        </section>

        <form className="tag-creator" onSubmit={handleAdd}>
          <div className="tag-manager__heading"><div><span className="eyebrow">NOVA CLASSIFICAÇÃO</span><h3>Criar tag personalizada</h3></div><Icon name="plus" /></div>
          <label className="field-control">
            <span>Nome da tag</span>
            <input maxLength={24} onChange={(event) => setLabel(event.target.value)} placeholder="Ex.: Cliente premium" value={label} />
            <small>{label.length}/24</small>
          </label>
          <div>
            <span className="field-label">Cor do sinal</span>
            <div className="color-picker color-picker--wide">
              {colors.map((item) => (
                <button
                  aria-label={`Selecionar cor ${item}`}
                  className={color === item ? 'active' : ''}
                  key={item}
                  onClick={() => setColor(item)}
                  style={{ backgroundColor: item }}
                  type="button"
                >
                  {color === item && <Icon name="check" size={13} />}
                </button>
              ))}
            </div>
          </div>
          <div className="tag-creator__preview">
            <span>PRÉVIA</span>
            <TagBadge tag={{ id: 'preview', label: label || 'Nova tag', color, kind: 'custom' }} />
          </div>
          {error && <span className="form-error form-error--visible"><Icon name="alert" size={14} /> {error}</span>}
          <button className="primary-button primary-button--full" type="submit"><Icon name="plus" size={16} /> Adicionar à biblioteca</button>
        </form>
      </div>
      <footer className="modal-footer"><span>Tags predefinidas representam o ciclo de maturidade.</span><button className="secondary-button" onClick={onClose} type="button">Concluir</button></footer>
    </Modal>
  )
}

import { useEffect, useMemo, useState } from 'react'
import { statusLabels } from '../../lib/profile'
import type { BrowserProfile, Platform, ProfileDraft, ProfileStatus, ProfileTag } from '../../types/profile'
import { Icon } from '../common/Icon'
import { Modal } from '../common/Modal'
import { allPlatforms, PlatformIcon } from '../profiles/PlatformIcon'
import { TagBadge } from '../tags/TagBadge'

const accentColors = ['#36f58b', '#70a5ff', '#30d5c8', '#f5b942', '#ff8a4c', '#bd7cff', '#ff5f6d']
const statuses = Object.keys(statusLabels) as ProfileStatus[]

const emptyDraft: ProfileDraft = {
  name: '',
  color: accentColors[0],
  platforms: [],
  status: 'starting',
  tagIds: [],
  notes: '',
  startUrl: '',
}

interface ProfileModalProps {
  onClose: () => void
  onManageTags: () => void
  onSave: (draft: ProfileDraft) => void
  open: boolean
  profile: BrowserProfile | null
  tags: ProfileTag[]
}

export function ProfileModal({ onClose, onManageTags, onSave, open, profile, tags }: ProfileModalProps) {
  const [draft, setDraft] = useState<ProfileDraft>(emptyDraft)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    setError('')
    setDraft(profile ? {
      name: profile.name,
      color: profile.color,
      platforms: profile.platforms,
      status: profile.status,
      tagIds: profile.tags.filter((tag) => tag.kind === 'custom').map((tag) => tag.id),
      notes: profile.notes,
      startUrl: profile.startUrl,
    } : emptyDraft)
  }, [open, profile])

  const customTags = useMemo(() => tags.filter((tag) => tag.kind === 'custom'), [tags])

  const togglePlatform = (platform: Platform) => {
    setDraft((current) => ({
      ...current,
      platforms: current.platforms.includes(platform)
        ? current.platforms.filter((item) => item !== platform)
        : [...current.platforms, platform],
    }))
  }

  const toggleTag = (tagId: string) => {
    setDraft((current) => ({
      ...current,
      tagIds: current.tagIds.includes(tagId)
        ? current.tagIds.filter((id) => id !== tagId)
        : [...current.tagIds, tagId],
    }))
  }

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault()
    if (draft.name.trim().length < 3) {
      setError('Informe um nome com pelo menos 3 caracteres.')
      return
    }
    if (draft.platforms.length === 0) {
      setError('Selecione pelo menos uma plataforma.')
      return
    }
    onSave({
      ...draft,
      name: draft.name.trim(),
      notes: draft.notes.trim(),
      startUrl: draft.startUrl.trim(),
    })
  }

  return (
    <Modal
      description="Identidade visual, plataformas e classificação operacional."
      onClose={onClose}
      open={open}
      title={profile ? 'Editar perfil' : 'Criar novo perfil'}
      width="lg"
    >
      <form className="profile-form" onSubmit={handleSubmit}>
        <div className="profile-form__main">
          <div className="form-section">
            <div className="form-section__heading"><span>01</span><div><h3>Identidade</h3><p>Como este perfil aparece no radar.</p></div></div>
            <label className="field-control">
              <span>Nome do perfil</span>
              <input
                autoFocus
                maxLength={48}
                onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
                placeholder="Ex.: IG • Conteúdo BR 02"
                value={draft.name}
              />
              <small>{draft.name.length}/48</small>
            </label>
            <div className="field-group">
              <div>
                <span className="field-label">Cor de identificação</span>
                <div className="color-picker">
                  {accentColors.map((color) => (
                    <button
                      aria-label={`Selecionar cor ${color}`}
                      aria-pressed={draft.color === color}
                      className={draft.color === color ? 'active' : ''}
                      key={color}
                      onClick={() => setDraft((current) => ({ ...current, color }))}
                      style={{ backgroundColor: color }}
                      type="button"
                    >
                      {draft.color === color && <Icon name="check" size={13} />}
                    </button>
                  ))}
                </div>
              </div>
              <div className="profile-preview" style={{ '--preview-color': draft.color } as React.CSSProperties}>
                <i /> <span>PRÉVIA</span><b>{draft.name || 'Novo perfil'}</b>
              </div>
            </div>
            <label className="field-control">
              <span>Página de acesso da conta</span>
              <input
                inputMode="url"
                onChange={(event) => setDraft((current) => ({ ...current, startUrl: event.target.value }))}
                placeholder="https://accounts.google.com/"
                type="url"
                value={draft.startUrl}
              />
              <small className="field-control__hint">Usada no primeiro acesso; depois o Chromium restaura a última sessão.</small>
            </label>
          </div>

          <div className="form-section">
            <div className="form-section__heading"><span>02</span><div><h3>Placas vinculadas</h3><p>Selecione as plataformas deste perfil.</p></div></div>
            <div className="platform-picker">
              {allPlatforms.map((platform) => (
                <button
                  aria-pressed={draft.platforms.includes(platform)}
                  className={draft.platforms.includes(platform) ? 'active' : ''}
                  key={platform}
                  onClick={() => togglePlatform(platform)}
                  type="button"
                >
                  <PlatformIcon platform={platform} />
                  <span>{platform === 'x' ? 'X / Twitter' : platform[0].toUpperCase() + platform.slice(1)}</span>
                  <i><Icon name="check" size={12} /></i>
                </button>
              ))}
            </div>
          </div>

          <div className="form-section">
            <div className="form-section__heading"><span>03</span><div><h3>Notas operacionais</h3><p>Instruções rápidas visíveis no card.</p></div></div>
            <label className="field-control">
              <span>Notas</span>
              <textarea
                maxLength={180}
                onChange={(event) => setDraft((current) => ({ ...current, notes: event.target.value }))}
                placeholder="Registre rotina, restrições ou próximos passos..."
                rows={3}
                value={draft.notes}
              />
              <small>{draft.notes.length}/180</small>
            </label>
          </div>
        </div>

        <aside className="profile-form__side">
          <div className="form-section form-section--side">
            <div className="form-section__heading"><span>04</span><div><h3>Status atual</h3><p>Fase operacional do perfil.</p></div></div>
            <div className="status-picker">
              {statuses.map((status) => (
                <button
                  className={draft.status === status ? `active status-picker--${status}` : ''}
                  key={status}
                  onClick={() => setDraft((current) => ({ ...current, status }))}
                  type="button"
                >
                  <i /> <span>{statusLabels[status]}</span><Icon name="check" size={14} />
                </button>
              ))}
            </div>
          </div>

          <div className="form-section form-section--side">
            <div className="form-section__heading form-section__heading--actions">
              <div><h3>Tags personalizadas</h3><p>Organize campanhas e rotinas.</p></div>
              <button aria-label="Gerenciar tags" className="icon-button icon-button--small" onClick={onManageTags} type="button"><Icon name="settings" size={14} /></button>
            </div>
            <div className="tag-picker">
              {customTags.length > 0 ? customTags.map((tag) => (
                <button
                  aria-pressed={draft.tagIds.includes(tag.id)}
                  className={draft.tagIds.includes(tag.id) ? 'active' : ''}
                  key={tag.id}
                  onClick={() => toggleTag(tag.id)}
                  type="button"
                >
                  <TagBadge tag={tag} />
                  <i><Icon name="check" size={12} /></i>
                </button>
              )) : <p className="tag-picker__empty">Nenhuma tag personalizada.</p>}
            </div>
          </div>

          <div className="disk-notice">
            <Icon name="shield" size={18} />
            <div><strong>Persistência em disco</strong><span>O diretório físico será criado pelo núcleo local ao salvar.</span></div>
          </div>
        </aside>

        <footer className="modal-footer profile-form__footer">
          <span className={error ? 'form-error form-error--visible' : 'form-error'}><Icon name="alert" size={14} /> {error}</span>
          <div>
            <button className="ghost-button" onClick={onClose} type="button">Cancelar</button>
            <button className="primary-button" type="submit"><Icon name={profile ? 'check' : 'plus'} size={16} /> {profile ? 'Salvar alterações' : 'Criar perfil'}</button>
          </div>
        </footer>
      </form>
    </Modal>
  )
}

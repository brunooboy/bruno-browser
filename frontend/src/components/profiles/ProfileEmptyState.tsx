import { Icon } from '../common/Icon'

export function ProfileEmptyState({ onReset }: { onReset: () => void }) {
  return (
    <div className="empty-state">
      <span><Icon name="search" size={24} /></span>
      <h3>Nenhum perfil neste radar</h3>
      <p>Ajuste os filtros ou limpe a busca para visualizar seus perfis.</p>
      <button className="secondary-button" onClick={onReset} type="button">Limpar filtros</button>
    </div>
  )
}

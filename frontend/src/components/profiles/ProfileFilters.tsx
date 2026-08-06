import type { Platform, ProfileStatus } from '../../types/profile'
import { Icon } from '../common/Icon'
import { allPlatforms, PlatformIcon } from './PlatformIcon'

interface ProfileFiltersProps {
  count: number
  onManageTags: () => void
  onPlatformChange: (platform: Platform | 'all') => void
  onSortChange: (sort: 'recent' | 'mature' | 'latency') => void
  onStatusChange: (status: ProfileStatus | 'all') => void
  onViewChange: (view: 'grid' | 'list') => void
  platform: Platform | 'all'
  sort: 'recent' | 'mature' | 'latency'
  status: ProfileStatus | 'all'
  view: 'grid' | 'list'
}

export function ProfileFilters({
  count,
  onManageTags,
  onPlatformChange,
  onSortChange,
  onStatusChange,
  onViewChange,
  platform,
  sort,
  status,
  view,
}: ProfileFiltersProps) {
  return (
    <div className="profile-toolbar">
      <div className="profile-toolbar__title">
        <div>
          <span className="eyebrow">INVENTÁRIO LOCAL</span>
          <h2>Perfis operacionais <span>{count.toString().padStart(2, '0')}</span></h2>
        </div>
        <button className="secondary-button" onClick={onManageTags} type="button">
          <Icon name="tag" size={15} /> Gerenciar tags
        </button>
      </div>
      <div className="profile-toolbar__filters">
        <label className="select-control">
          <span>Status</span>
          <select onChange={(event) => onStatusChange(event.target.value as ProfileStatus | 'all')} value={status}>
            <option value="all">Todos</option>
            <option value="starting">Iniciando</option>
            <option value="warming">Aquecendo</option>
            <option value="fixed">Operação fixa</option>
            <option value="farm">Farm</option>
          </select>
          <Icon name="chevronDown" size={13} />
        </label>

        <div aria-label="Filtrar por plataforma" className="platform-filter" role="group">
          <button
            className={platform === 'all' ? 'platform-filter__all platform-filter__active' : 'platform-filter__all'}
            onClick={() => onPlatformChange('all')}
            type="button"
          >
            TODAS
          </button>
          {allPlatforms.map((item) => (
            <button
              aria-pressed={platform === item}
              className={platform === item ? 'platform-filter__active' : ''}
              key={item}
              onClick={() => onPlatformChange(item)}
              type="button"
            >
              <PlatformIcon platform={item} size="sm" />
            </button>
          ))}
        </div>

        <label className="select-control select-control--sort">
          <Icon name="sliders" size={14} />
          <select onChange={(event) => onSortChange(event.target.value as typeof sort)} value={sort}>
            <option value="recent">Mais recentes</option>
            <option value="mature">Mais maduros</option>
            <option value="latency">Menor latência</option>
          </select>
          <Icon name="chevronDown" size={13} />
        </label>

        <div aria-label="Modo de visualização" className="view-toggle" role="group">
          <button aria-label="Grade" className={view === 'grid' ? 'active' : ''} onClick={() => onViewChange('grid')} type="button"><Icon name="grid" size={15} /></button>
          <button aria-label="Lista" className={view === 'list' ? 'active' : ''} onClick={() => onViewChange('list')} type="button"><Icon name="list" size={16} /></button>
        </div>
      </div>
    </div>
  )
}

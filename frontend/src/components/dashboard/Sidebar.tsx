import { Icon, type IconName } from '../common/Icon'
import { PixelLogo } from '../common/PixelLogo'

interface SidebarProps {
  active: string
  collapsed: boolean
  onNavigate: (item: string) => void
  onToggle: () => void
  profileCount: number
  isAdmin?: boolean
  desktopMode?: boolean
}

const primaryItems: { id: string; label: string; icon: IconName }[] = [
  { id: 'overview', label: 'Visão geral', icon: 'grid' },
  { id: 'profiles', label: 'Perfis', icon: 'layers' },
  { id: 'proxies', label: 'Proxies', icon: 'globe' },
  { id: 'tags', label: 'Tags', icon: 'tag' },
  { id: 'extensions', label: 'Extensões', icon: 'extensions' },
	{ id: 'backups', label: 'Backups', icon: 'download' },
]

export function Sidebar({ active, collapsed, desktopMode = false, isAdmin = false, onNavigate, onToggle, profileCount }: SidebarProps) {
  return (
    <aside className={`sidebar ${collapsed ? 'sidebar--collapsed' : ''}`}>
      <div className="sidebar__brand">
        <PixelLogo compact={collapsed} />
      </div>

      <div className="sidebar__section-label">OPERAÇÕES</div>
      <nav aria-label="Navegação principal" className="sidebar__nav">
        {primaryItems.map((item) => (
          <button
            className={active === item.id ? 'sidebar__item sidebar__item--active' : 'sidebar__item'}
            key={item.id}
            onClick={() => onNavigate(item.id)}
            title={collapsed ? item.label : undefined}
            type="button"
          >
            <Icon name={item.icon} size={19} />
            <span className="sidebar__item-label">{item.label}</span>
            {item.id === 'profiles' && <span className="sidebar__count">{profileCount}</span>}
          </button>
        ))}
      </nav>

      <div className="sidebar__section-label sidebar__section-label--system">SISTEMA</div>
      <nav aria-label="Navegação do sistema" className="sidebar__nav">
        <button className={active === 'updates' ? 'sidebar__item sidebar__item--active' : 'sidebar__item'} onClick={() => onNavigate('updates')} type="button">
          <Icon name="refresh" size={19} />
          <span className="sidebar__item-label">Atualizações</span>
          <span className="status-dot status-dot--ok" />
        </button>
        <button className={active === 'settings' ? 'sidebar__item sidebar__item--active' : 'sidebar__item'} onClick={() => onNavigate('settings')} type="button">
          <Icon name="settings" size={19} />
          <span className="sidebar__item-label">Configurações</span>
        </button>
        {isAdmin && <button className={active === 'admin' ? 'sidebar__item sidebar__item--active' : 'sidebar__item'} onClick={() => onNavigate('admin')} type="button">
          <Icon name="shield" size={19} />
          <span className="sidebar__item-label">Admin</span>
          <span className="status-dot status-dot--danger" />
        </button>}
      </nav>

      <div className="sidebar__terminal">
        <div className="sidebar__terminal-head">
          <span className="status-dot status-dot--ok" />
          <span>CORE LOCAL</span>
          <strong>{desktopMode ? 'ONLINE' : 'PREVIEW'}</strong>
        </div>
        <div className="sidebar__terminal-row">
          <span>DISK</span>
          <div><i style={{ width: '62%' }} /></div>
          <b>62%</b>
        </div>
        <div className="sidebar__terminal-row">
          <span>CDP</span>
          <div><i style={{ width: '100%' }} /></div>
          <b>{desktopMode ? 'OK' : '—'}</b>
        </div>
      </div>

      <button aria-label={collapsed ? 'Expandir menu' : 'Recolher menu'} className="sidebar__collapse" onClick={onToggle} type="button">
        <Icon name="chevronDown" size={16} />
      </button>
    </aside>
  )
}

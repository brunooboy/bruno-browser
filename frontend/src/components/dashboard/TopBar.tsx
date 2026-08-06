import { useState } from 'react'
import type { AppNotification } from '../../types/system'
import { Icon } from '../common/Icon'

interface TopBarProps {
  actionLabel?: string
  contextLabel?: string
  notifications: AppNotification[]
  onClearNotifications: () => void
  onCreate?: () => void
  onReadNotifications: () => void
  onSearch?: (value: string) => void
  searchPlaceholder?: string
  search: string
  statusLabel?: string
  title?: string
}

export function TopBar({
  actionLabel = 'Novo perfil', contextLabel = 'CENTRAL DE OPERAÇÕES', notifications,
  onClearNotifications, onCreate, onReadNotifications, onSearch,
  searchPlaceholder = 'Buscar perfil ou ID...', search,
  statusLabel = 'NÚCLEO LOCAL ONLINE', title = 'Radar de perfis',
}: TopBarProps) {
  const [notificationsOpen, setNotificationsOpen] = useState(false)
  const unread = notifications.filter((item) => !item.read).length
  const toggleNotifications = () => {
    setNotificationsOpen((current) => {
      if (!current) onReadNotifications()
      return !current
    })
  }
  return <header className="topbar">
    <div className="topbar__context"><span className="eyebrow">{contextLabel}</span><div className="topbar__title-row"><h1>{title}</h1><span className="market-status"><i /> {statusLabel}</span></div></div>
    <div className="topbar__actions">
      {onSearch && <label className="topbar__search"><Icon name="search" size={17} /><input aria-label="Buscar perfis" onChange={(event) => onSearch(event.target.value)} placeholder={searchPlaceholder} type="search" value={search} /><kbd>CTRL K</kbd></label>}
      <div className="notification-center">
        <button aria-expanded={notificationsOpen} aria-label={`Notificações (${unread} não lidas)`} className="icon-button topbar__notification" onClick={toggleNotifications} type="button">
          <Icon name="bell" />{unread > 0 && <span className="notification-count">{unread}</span>}
        </button>
        {notificationsOpen && <div className="notification-panel">
          <header><div><span className="eyebrow">HISTÓRICO LOCAL</span><strong>Notificações</strong></div>{notifications.length > 0 && <button onClick={onClearNotifications} type="button">Limpar</button>}</header>
          <div className="notification-list">
            {notifications.length === 0 ? <div className="notification-empty"><Icon name="bell" size={20} /><span>Nenhuma notificação armazenada.</span></div> : notifications.map((item) => <article className={`notification-item notification-item--${item.tone}`} key={item.id}>
              <i /><div><p>{item.message}</p><time dateTime={item.createdAt}>{new Intl.DateTimeFormat('pt-BR', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(item.createdAt))}</time></div>
            </article>)}
          </div>
          <footer>ÚLTIMAS {notifications.length}/10 · LOCALSTORAGE</footer>
        </div>}
      </div>
      {onCreate && <button className="primary-button" onClick={onCreate} type="button"><Icon name="plus" size={17} />{actionLabel}</button>}
    </div>
  </header>
}

import { formatAge, maturityFor, statusLabels } from '../../lib/profile'
import type { BrowserProfile } from '../../types/profile'
import { Icon } from '../common/Icon'
import { TagBadge } from '../tags/TagBadge'
import { PlatformIcon } from './PlatformIcon'

export type ProfileAction = 'edit' | 'delete' | 'cache' | 'cookies'

interface ProfileCardProps {
  now: number
  onAction: (action: ProfileAction, profile: BrowserProfile) => void
  onLaunch: (profile: BrowserProfile) => void
  premiumActive: boolean
  profile: BrowserProfile
  view: 'grid' | 'list'
}

const riskLabel = {
  low: 'Risco baixo',
  medium: 'Atenção',
  high: 'Risco alto',
}

export function ProfileCard({ now, onAction, onLaunch, premiumActive, profile, view }: ProfileCardProps) {
  const maturity = maturityFor(profile.createdAt, now)
  const age = formatAge(profile.createdAt, now)

  return (
    <article
      className={`profile-card profile-card--${view}`}
      style={{ '--profile-accent': profile.color } as React.CSSProperties}
    >
      <div className="profile-card__accent" />
      <header className="profile-card__header">
        <div className="profile-card__platforms">
          {profile.platforms.map((platform) => <PlatformIcon key={platform} platform={platform} />)}
        </div>
        <div className="profile-card__identity">
          <div className="profile-card__name-line">
            <h3>{profile.name}</h3>
            {profile.running && <span className="running-pill"><i /> EM EXECUÇÃO</span>}
            <span className={`risk-pill risk-pill--${profile.risk}`} title={profile.riskReasons?.join(' · ') || 'Nenhum alerta operacional'}><i /> {riskLabel[profile.risk]}</span>
          </div>
          <span className="profile-card__id">{profile.id} <i /> {profile.lastSeen}</span>
        </div>
        <details className="profile-menu">
          <summary aria-label="Opções avançadas"><Icon name="more" /></summary>
          <div className="profile-menu__popover">
            <span>AÇÕES DO PERFIL</span>
            <button onClick={() => onAction('edit', profile)} type="button"><Icon name="edit" size={15} /> Editar perfil</button>
            <button onClick={() => onAction('cache', profile)} type="button"><Icon name="broom" size={15} /> Limpar histórico e cache</button>
            <button onClick={() => onAction('cookies', profile)} type="button"><Icon name="cookie" size={15} /> Limpar cookies / sessão</button>
            <hr />
            <button className="danger" onClick={() => onAction('delete', profile)} type="button"><Icon name="trash" size={15} /> Excluir perfil</button>
          </div>
        </details>
      </header>

      <div className="profile-card__tags">
        <span className={`status-tag status-tag--${profile.status}`}><i /> {statusLabels[profile.status]}</span>
        {profile.tags.filter((tag) => tag.id !== profile.status).map((tag) => <TagBadge key={tag.id} tag={tag} />)}
      </div>

      <div className="profile-card__metrics">
        <div>
          <span><Icon name="globe" size={14} /> Proxy</span>
          {profile.proxy ? (
            <><strong>{profile.proxy.location}</strong><small>{profile.proxy.endpoint}</small></>
          ) : (
            <><strong className="metric-danger">Não configurado</strong><small>Defina uma rota dedicada</small></>
          )}
        </div>
        <div>
          <span><Icon name="shield" size={14} /> Fingerprint</span>
          <strong className={profile.fingerprintScore < 100 ? 'metric-danger' : ''}>{profile.fingerprintScore === 100 ? '100% verificado' : 'Pendente'}</strong>
          <small>{profile.fingerprintLabel ?? 'Sem leitura'} · {profile.engine ?? 'motor desconhecido'} · {profile.sessions} aberturas</small>
        </div>
      </div>

      <div className="profile-card__maturity">
        <div className="maturity-head">
          <span><Icon name="clock" size={14} /> Maturidade</span>
          <b>{maturity.label}</b>
          <strong>{age}</strong>
        </div>
        <div className="maturity-track"><i style={{ width: `${maturity.percentage}%` }} /></div>
        <div className="maturity-caption"><span>CRIADO</span><span>{maturity.percentage}% DO CICLO DE 30 DIAS</span></div>
      </div>

      <p className="profile-card__notes">“{profile.notes}”</p>

      <footer className="profile-card__footer">
        <div className="latency">
          <span className={`status-dot ${profile.proxy ? 'status-dot--ok' : 'status-dot--danger'}`} />
          <span>{profile.proxy ? `${profile.proxy.latencyMs} ms` : 'sem rota'}</span>
        </div>
        <button className={`launch-button ${profile.running ? 'launch-button--running' : ''}`} disabled={!profile.running && !premiumActive} onClick={() => onLaunch(profile)} title={!profile.running && !premiumActive ? 'Ative uma key para abrir este perfil' : undefined} type="button">
          <Icon name={profile.running ? 'x' : premiumActive ? 'play' : 'shield'} size={15} /> {profile.running ? 'Fechar perfil' : premiumActive ? 'Abrir perfil' : 'Key necessária'} <span>{profile.running ? '■' : premiumActive ? '↗' : '◆'}</span>
        </button>
      </footer>
    </article>
  )
}

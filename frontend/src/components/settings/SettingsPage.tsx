import { useState, type CSSProperties } from 'react'
import type { AppPreferences, DiscordAccount, PlanStatus } from '../../types/system'
import { Icon } from '../common/Icon'

interface SettingsPageProps {
  account?: DiscordAccount
  plan: PlanStatus
  preferences: AppPreferences
  oauthConfigured: boolean
  settingsPath: string
  desktopMode: boolean
  busy: boolean
  onActivate: (key: string) => Promise<void>
  onDeactivate: () => Promise<void>
  onLogin: () => Promise<void>
  onLogout: () => Promise<void>
  onSaveTheme: (color: string) => Promise<void>
}

const themeColors = ['#42ff91', '#30d5c8', '#70a5ff', '#bd7cff', '#f5b942', '#ff5f6d']

const formatDate = (timestamp?: number) => timestamp ? new Date(timestamp * 1000).toLocaleString('pt-BR') : '—'
const planLabels: Record<NonNullable<PlanStatus['plan']>, string> = {
  VITALICIO: 'Vitalício',
  '30': '30 dias',
  '7': '7 dias',
  '1': '1 dia',
}
const planLabel = (plan?: PlanStatus['plan']) => plan ? planLabels[plan] : 'Sem plano'

export function SettingsPage(props: SettingsPageProps) {
  const [accent, setAccent] = useState(props.preferences.accentColor)
  const [key, setKey] = useState('')
  const statusLabel = props.plan.status === 'active' ? 'ATIVO' : props.plan.status === 'expired' ? 'EXPIRADO' : 'NENHUM'

  return (
    <div className="system-page settings-page">
      <div className="settings-grid">
        <section className="system-panel settings-card">
          <header><div><span>APARÊNCIA</span><h3>Cor de operação</h3></div><Icon name="sliders" /></header>
          <p>A cor é persistida no disco e aplicada imediatamente em toda a interface.</p>
          <div className="theme-picker">
            {themeColors.map((color) => <button aria-label={`Usar cor ${color}`} className={accent === color ? 'active' : ''} key={color} onClick={() => setAccent(color)} style={{ '--swatch': color } as CSSProperties} type="button"><Icon name="check" size={14} /></button>)}
            <label><span>PERSONALIZADA</span><input onChange={(event) => setAccent(event.target.value)} type="color" value={accent} /></label>
          </div>
          <button className="primary-button" disabled={props.busy || !props.desktopMode} onClick={() => props.onSaveTheme(accent)} type="button">Salvar tema</button>
        </section>

        <section className="system-panel settings-card account-card">
          <header><div><span>CONTA</span><h3>Discord OAuth2</h3></div><Icon name="shield" /></header>
          {props.account ? <div className="account-identity">
            {props.account.avatarUrl ? <img alt="Avatar do Discord" src={props.account.avatarUrl} /> : <span>{props.account.username.slice(0, 1).toUpperCase()}</span>}
            <div><strong>{props.account.globalName || props.account.username}</strong><small>@{props.account.username} • {props.account.id}</small></div>
            {props.account.isAdmin && <em>ADMIN</em>}
          </div> : <div className="account-offline"><Icon name="alert" /><span><b>Nenhuma conta conectada</b><small>O primeiro login exige internet. Depois, a identidade fica disponível offline.</small></span></div>}
          {!props.oauthConfigured && <div className="config-hint"><b>OAuth ainda não configurado</b><span>Preencha Client ID, Client Secret e Admin ID em:</span><code>{props.settingsPath || 'appconfig.json'}</code></div>}
          <button className={props.account ? 'ghost-button' : 'primary-button'} disabled={props.busy || !props.desktopMode || (!props.oauthConfigured && !props.account)} onClick={props.account ? props.onLogout : props.onLogin} type="button">
            <Icon name={props.account ? 'x' : 'globe'} size={16} /> {props.account ? 'Sair da conta' : 'Entrar com Discord'}
          </button>
        </section>
      </div>

      <section className="system-panel license-panel">
        <header><div><span>PLANO LOCAL</span><h3>Licença criptografada</h3></div><span className={`plan-status plan-status--${props.plan.status}`}><i /> {statusLabel}</span></header>
        <div className="plan-details">
          <div><span>TIPO</span><strong>{planLabel(props.plan.plan)}</strong></div>
          <div><span>EXPIRAÇÃO</span><strong>{props.plan.plan === 'VITALICIO' ? 'Nunca' : formatDate(props.plan.expires_at)}</strong></div>
          <div><span>ATIVAÇÃO</span><strong>{formatDate(props.plan.activated_at)}</strong></div>
          <div><span>KEY ID</span><strong>{props.plan.key_id || '—'}</strong></div>
        </div>
        {props.plan.status === 'active' ? <button className="ghost-button danger-button" disabled={props.busy || !props.desktopMode} onClick={props.onDeactivate} type="button"><Icon name="trash" size={15} /> Remover / desativar key</button> : <div className="activation-form">
          <label><span>KEY DE ATIVAÇÃO</span><textarea onChange={(event) => setKey(event.target.value)} placeholder="Cole aqui a key Base64URL..." rows={3} value={key} /></label>
          <button className="primary-button" disabled={props.busy || !props.desktopMode || !props.account || !key.trim()} onClick={() => props.onActivate(key)} type="button"><Icon name="shield" size={16} /> Ativar key</button>
        </div>}
      </section>
    </div>
  )
}

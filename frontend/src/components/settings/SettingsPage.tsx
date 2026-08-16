import { useState, type CSSProperties } from 'react'
import type { AppPreferences, DiscordAccount, PlanStatus, SystemDiagnostics } from '../../types/system'
import { Icon } from '../common/Icon'

interface SettingsPageProps {
  account?: DiscordAccount
  plan: PlanStatus
  preferences: AppPreferences
  oauthConfigured: boolean
  settingsPath: string
  desktopMode: boolean
  busy: boolean
  diagnostics: SystemDiagnostics
  onActivate: (key: string) => Promise<void>
  onDeactivate: () => Promise<void>
  onClearDiagnostics: () => Promise<void>
  onLogin: () => Promise<void>
  onLogout: () => Promise<void>
  onRunDiagnostics: () => Promise<void>
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
  const diagnosticsLabel = props.diagnostics.status === 'ready' ? 'PRONTO' : props.diagnostics.status === 'attention' ? 'ATENÇÃO' : 'BLOQUEADO'

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

      <section className="system-panel diagnostics-panel">
        <header><div><span>HOMOLOGAÇÃO LOCAL</span><h3>Diagnóstico do aplicativo</h3></div><span className={`diagnostics-status diagnostics-status--${props.diagnostics.status}`}><i /> {diagnosticsLabel}</span></header>
        <div className="diagnostics-summary">
          {props.diagnostics.checks.length ? props.diagnostics.checks.map((check) => <article className={`diagnostic-check diagnostic-check--${check.status}`} key={check.id}>
            <span>{check.status === 'pass' ? <Icon name="check" size={14} /> : <Icon name="alert" size={14} />}</span>
            <div><strong>{check.label}</strong><small>{check.detail}</small></div>
          </article>) : <div className="diagnostics-empty"><Icon name="activity" /><span>Execute o diagnóstico no aplicativo desktop.</span></div>}
        </div>
        <div className="diagnostics-actions">
          <button className="primary-button" disabled={props.busy || !props.desktopMode} onClick={props.onRunDiagnostics} type="button"><Icon name="activity" size={15} /> Executar diagnóstico</button>
          <small>Última leitura: {props.diagnostics.generatedAt.startsWith('1970-') ? 'ainda não executado' : new Date(props.diagnostics.generatedAt).toLocaleString('pt-BR')}</small>
        </div>
        <div className="diagnostics-log">
          <header><span>FALHAS RECENTES</span><button className="ghost-button" disabled={props.busy || !props.desktopMode || props.diagnostics.incidents.length === 0} onClick={props.onClearDiagnostics} type="button"><Icon name="trash" size={13} /> Limpar registro</button></header>
          {props.diagnostics.incidents.length ? props.diagnostics.incidents.slice(0, 8).map((incident, index) => <article key={`${incident.at}-${incident.scope}-${index}`}>
            <time>{new Date(incident.at).toLocaleString('pt-BR')}</time><b>{incident.scope.replaceAll('_', ' ')}</b><span>{incident.message}</span>
          </article>) : <p>Nenhuma falha operacional registrada neste dispositivo.</p>}
        </div>
      </section>
    </div>
  )
}

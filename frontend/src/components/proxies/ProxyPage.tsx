import { useMemo } from 'react'
import type { BrowserProfile } from '../../types/profile'
import { Icon } from '../common/Icon'
import { PlatformIcon } from '../profiles/PlatformIcon'

interface ProxyPageProps {
  profiles: BrowserProfile[]
  premiumActive: boolean
  testingId: string | null
  onConfigure: (profile: BrowserProfile) => void
  onTest: (profile: BrowserProfile) => void
}

export function ProxyPage({ profiles, premiumActive, testingId, onConfigure, onTest }: ProxyPageProps) {
  const routed = useMemo(() => profiles.filter((profile) => profile.proxy), [profiles])
  const medianLatency = useMemo(() => {
    const values = routed.map((profile) => profile.proxy?.latencyMs ?? 0).filter((value) => value > 0).sort((a, b) => a - b)
    return values.length ? values[Math.floor(values.length / 2)] : 0
  }, [routed])

  return (
    <div className="proxy-page">
      <section aria-label="Resumo de rede" className="proxy-stats">
        <article>
          <div className="proxy-stats__icon proxy-stats__icon--green"><Icon name="network" size={18} /></div>
          <span>ROTAS ATIVAS</span>
          <strong>{String(routed.length).padStart(2, '0')}<small> / {String(profiles.length).padStart(2, '0')}</small></strong>
          <p><i /> isoladas por perfil</p>
        </article>
        <article>
          <div className="proxy-stats__icon"><Icon name="globe" size={18} /></div>
          <span>CONEXÃO DIRETA</span>
          <strong>{String(profiles.length - routed.length).padStart(2, '0')}</strong>
          <p><i /> DNS automático ativo</p>
        </article>
        <article>
          <div className="proxy-stats__icon proxy-stats__icon--cyan"><Icon name="zap" size={18} /></div>
          <span>LATÊNCIA MEDIANA</span>
          <strong>{medianLatency || '—'}<small>{medianLatency ? ' ms' : ''}</small></strong>
          <p><i /> ponte local Go</p>
        </article>
        <article>
          <div className="proxy-stats__icon proxy-stats__icon--blue"><Icon name="shield" size={18} /></div>
          <span>PROTEÇÃO DE IP</span>
          <strong>LOCK<small>ED</small></strong>
          <p><i /> WebRTC restrito</p>
        </article>
      </section>

      <section className="dns-route-panel">
        <div className="dns-route-panel__head">
          <div><span>DNS CONTROL PLANE</span><h2>Resolução automática por rota</h2></div>
          <span className="dns-route-panel__live"><i /> ATIVO</span>
        </div>
        <div className="dns-route-panel__flow">
          <div><Icon name="globe" size={19} /><span>PERFIL</span><strong>Bruno Engine isolado</strong></div>
          <i><span /></i>
          <div><Icon name="shield" size={19} /><span>POLÍTICA</span><strong>Anti-vazamento</strong></div>
          <i><span /></i>
          <div><Icon name="network" size={19} /><span>RESOLUÇÃO</span><strong>DoH ou proxy remoto</strong></div>
          <i><span /></i>
          <div><Icon name="zap" size={19} /><span>DESTINO</span><strong>Rota verificada</strong></div>
        </div>
        <p>Sem proxy: DNS-over-HTTPS automático com redundância e fallback do sistema. Com proxy: pré-resolução local bloqueada e domínio encaminhado pela rota configurada.</p>
      </section>

      <section className="proxy-inventory">
        <header>
          <div><span>INVENTÁRIO DE REDE</span><h2>Rotas por perfil <b>{String(profiles.length).padStart(2, '0')}</b></h2></div>
          <div className="proxy-inventory__legend"><span><i className="online" /> ROTA</span><span><i /> DIRETO</span></div>
        </header>
        <div className="proxy-table" role="table" aria-label="Configurações de proxy por perfil">
          <div className="proxy-table__header" role="row">
            <span>PERFIL</span><span>PROTOCOLO</span><span>ENDPOINT</span><span>DNS</span><span>LATÊNCIA</span><span>AÇÕES</span>
          </div>
          {profiles.map((profile) => (
            <article className="proxy-table__row" key={profile.id} role="row">
              <div className="proxy-table__profile">
                <div className="proxy-table__platforms">
                  {profile.platforms.slice(0, 2).map((platform) => <PlatformIcon key={platform} platform={platform} />)}
                </div>
                <div><strong>{profile.name}</strong><span>{profile.id}</span></div>
              </div>
              <div><span className={profile.proxy ? `route-badge route-badge--${profile.proxy.mode}` : 'route-badge route-badge--direct'}>{profile.proxy?.mode.toUpperCase() ?? 'DIRECT'}</span></div>
              <div className="proxy-table__endpoint">
                <strong>{profile.proxy ? `${profile.proxy.host}:${profile.proxy.port}` : 'Conexão do sistema'}</strong>
                <span>{profile.proxy?.username ? `AUTH • ${profile.proxy.hasPassword ? 'PROTEGIDA' : 'SEM SENHA'}` : 'SEM AUTENTICAÇÃO'}</span>
              </div>
              <div className="proxy-table__dns">
                <Icon name="shield" size={14} />
                <div><strong>{profile.proxy ? 'REMOTO' : 'AUTO DoH'}</strong><span>WebRTC LOCK</span></div>
              </div>
              <div className="proxy-table__latency">
                <i className={!profile.proxy || (profile.proxy.latencyMs > 0 && profile.proxy.latencyMs < 90) ? 'good' : 'warn'} />
                <strong>{profile.proxy ? (profile.proxy.latencyMs ? `${profile.proxy.latencyMs} ms` : 'NÃO TESTADA') : 'NATIVO'}</strong>
              </div>
              <div className="proxy-table__actions">
                <button className="ghost-button" disabled={!premiumActive || !profile.proxy || testingId === profile.id} onClick={() => onTest(profile)} type="button">
                  <Icon className={testingId === profile.id ? 'spin' : ''} name="refresh" size={14} />
                  {testingId === profile.id ? 'Testando' : 'Testar'}
                </button>
                <button aria-label={`Configurar rede de ${profile.name}`} className="icon-button icon-button--small" disabled={!premiumActive} onClick={() => onConfigure(profile)} type="button"><Icon name="settings" size={15} /></button>
              </div>
            </article>
          ))}
        </div>
      </section>
    </div>
  )
}

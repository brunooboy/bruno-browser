import type { UpdateStatus } from '../../types/system'
import { Icon } from '../common/Icon'

interface UpdatesPageProps {
  status: UpdateStatus
  busy: boolean
  desktopMode: boolean
  onCheck: () => Promise<void>
  onInstall: () => Promise<void>
}

export function UpdatesPage({ status, busy, desktopMode, onCheck, onInstall }: UpdatesPageProps) {
  return (
    <div className="system-page updates-page">
      <section className={status.updateAvailable ? 'update-banner update-banner--available' : 'update-banner'}>
        <div className="system-hero__icon"><Icon name="refresh" size={25} /></div>
        <div>
          <span>RELEASE CHANNEL // STABLE</span>
          <h2>{status.updateAvailable ? `Versão ${status.latestVersion} disponível` : 'Seu aplicativo está atualizado'}</h2>
          <p>Versão instalada <b>v{status.currentVersion}</b> • última verificação {new Date(status.checkedAt).toLocaleString('pt-BR')}</p>
        </div>
        <div className="update-actions">
          <button className="ghost-button" disabled={busy || !desktopMode} onClick={onCheck} type="button">
            <Icon className={busy ? 'spin' : ''} name="refresh" size={16} /> Verificar agora
          </button>
          {status.updateAvailable && <button className="primary-button" disabled={busy || !desktopMode || !status.installAvailable} onClick={onInstall} title={status.installReason} type="button">
            <Icon className={busy ? 'spin' : ''} name="download" size={16} /> {busy ? 'Baixando e verificando...' : 'Baixar e instalar'}
          </button>}
        </div>
      </section>

      {status.updateAvailable && !status.installAvailable && <div className="system-notice system-notice--warning"><Icon name="alert" size={15} /> {status.installReason || 'Instalador automático indisponível para esta release.'}</div>}

      <section className="system-panel changelog-panel">
        <header><div><span>CHANGELOG LOCAL</span><h3>Histórico de versões</h3></div><em>{status.source === 'local' ? 'MANIFEST LOCAL' : 'ENDPOINT REMOTO'}</em></header>
        <div className="release-timeline">
          {status.changelog.map((entry, index) => (
            <article key={`${entry.version}-${entry.date}`}>
              <i className={index === 0 ? 'release-dot release-dot--current' : 'release-dot'} />
              <div className="release-meta"><b>v{entry.version}</b><time>{new Date(`${entry.date}T00:00:00`).toLocaleDateString('pt-BR')}</time></div>
              <p>{entry.description}</p>
              {index === 0 && <span>ATUAL</span>}
            </article>
          ))}
        </div>
      </section>
    </div>
  )
}

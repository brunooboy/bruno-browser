import { useMemo, useState } from 'react'
import { backupPasswordValid, eligibleProfileIds, exportBackupReady } from '../../lib/backups'
import type { BrowserProfile } from '../../types/profile'
import type { BackupHistoryEntry, BackupImportResult } from '../../types/system'
import { Icon } from '../common/Icon'

interface BackupsPageProps {
  profiles: BrowserProfile[]
  history: BackupHistoryEntry[]
  busy: boolean
  desktopMode: boolean
  premiumActive: boolean
  onExport: (profileIds: string[], password: string) => Promise<void>
  onImport: (password: string) => Promise<BackupImportResult | undefined>
}

const formatBytes = (bytes: number) => {
  if (!bytes) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit += 1 }
  return `${value.toLocaleString('pt-BR', { maximumFractionDigits: unit > 1 ? 1 : 0 })} ${units[unit]}`
}

export function BackupsPage({ profiles, history, busy, desktopMode, premiumActive, onExport, onImport }: BackupsPageProps) {
  const [selected, setSelected] = useState<string[]>([])
  const [exportPassword, setExportPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [importPassword, setImportPassword] = useState('')
  const [lastImport, setLastImport] = useState<BackupImportResult>()
  const eligibleProfiles = useMemo(() => profiles.filter((profile) => !profile.running), [profiles])
  const selectedEligible = eligibleProfileIds(profiles, selected)
  const exportReady = desktopMode && premiumActive && exportBackupReady(profiles, selected, exportPassword, confirmation)
  const importReady = desktopMode && premiumActive && backupPasswordValid(importPassword)

  const toggle = (profileId: string) => setSelected((current) => current.includes(profileId) ? current.filter((id) => id !== profileId) : [...current, profileId])
  const selectAll = () => setSelected(selectedEligible.length === eligibleProfiles.length ? [] : eligibleProfiles.map((profile) => profile.id))

  const handleExport = async () => {
    await onExport(selectedEligible, exportPassword)
    setExportPassword('')
    setConfirmation('')
  }

  const handleImport = async () => {
    const result = await onImport(importPassword)
    if (result && !result.cancelled) {
      setLastImport(result)
      setImportPassword('')
    }
  }

  return <div className="system-page backup-page">
    <section className="system-hero backup-hero">
      <div className="system-hero__icon"><Icon name="lock" size={22} /></div>
      <div><span>COFRE DE MIGRAÇÃO LOCAL</span><h2>Backup criptografado de perfis</h2><p>Transfira sessões, cookies, fingerprint, DNS, proxy e extensões sem enviar dados para a nuvem.</p></div>
      <div className="backup-hero__status"><i /><strong>AES-256-GCM</strong><small>Proteção por senha + scrypt</small></div>
    </section>

    {!premiumActive && <div className="backup-warning"><Icon name="shield" size={17} /><span><strong>Plano necessário</strong><small>Ative uma key válida para exportar ou restaurar perfis.</small></span></div>}

    <section className="backup-grid">
      <article className="settings-panel backup-panel">
        <header className="panel-header"><div><span>EXPORTAÇÃO</span><h3>Criar pacote seguro</h3></div><Icon name="download" size={18} /></header>
        <div className="backup-profile-head"><span>{selectedEligible.length} PERFIS SELECIONADOS</span><button className="ghost-button" disabled={!eligibleProfiles.length} onClick={selectAll} type="button">{selectedEligible.length === eligibleProfiles.length && eligibleProfiles.length ? 'Limpar' : 'Selecionar todos'}</button></div>
        <div className="backup-profile-list">
          {profiles.length ? profiles.map((profile) => <label className={profile.running ? 'backup-profile backup-profile--disabled' : selected.includes(profile.id) ? 'backup-profile backup-profile--selected' : 'backup-profile'} key={profile.id}>
            <input checked={selected.includes(profile.id)} disabled={profile.running} onChange={() => toggle(profile.id)} type="checkbox" />
            <i style={{ background: profile.color }} />
            <span><strong>{profile.name}</strong><small>{profile.id.slice(0, 8)} · {profile.running ? 'feche antes de exportar' : `${profile.sessions} aberturas`}</small></span>
            {profile.running ? <em>EM EXECUÇÃO</em> : <Icon name={selected.includes(profile.id) ? 'check' : 'layers'} size={14} />}
          </label>) : <p className="backup-empty">Crie um perfil antes de gerar um backup.</p>}
        </div>
        <div className="backup-passwords">
          <label className="field-control"><span>SENHA DO ARQUIVO</span><input autoComplete="new-password" onChange={(event) => setExportPassword(event.target.value)} placeholder="Mínimo de 8 caracteres" type="password" value={exportPassword} /></label>
          <label className="field-control"><span>CONFIRMAR SENHA</span><input autoComplete="new-password" onChange={(event) => setConfirmation(event.target.value)} placeholder="Digite novamente" type="password" value={confirmation} /></label>
        </div>
        {confirmation && exportPassword !== confirmation && <p className="backup-form-error">As senhas não coincidem.</p>}
        <button className="primary-button primary-button--full" disabled={busy || !exportReady} onClick={handleExport} type="button"><Icon name="lock" size={15} /> Exportar .bruno-profile</button>
      </article>

      <article className="settings-panel backup-panel backup-restore">
        <header className="panel-header"><div><span>RESTAURAÇÃO</span><h3>Migrar para este computador</h3></div><Icon name="upload" size={18} /></header>
        <div className="backup-dropzone"><span><Icon name="upload" size={28} /></span><strong>Selecione um arquivo .bruno-profile</strong><small>O conteúdo só será gravado após autenticação e validação completa.</small></div>
        <label className="field-control"><span>SENHA DO BACKUP</span><input autoComplete="current-password" onChange={(event) => setImportPassword(event.target.value)} placeholder="Senha usada na exportação" type="password" value={importPassword} /></label>
        <button className="primary-button primary-button--full" disabled={busy || !importReady} onClick={handleImport} type="button"><Icon name="upload" size={15} /> Escolher e restaurar</button>
        <div className="backup-security-list">
          <span><Icon name="check" size={13} /><b>Integridade</b><small>Bloqueia arquivos alterados ou incompletos</small></span>
          <span><Icon name="check" size={13} /><b>UUID seguro</b><small>Colisões criam um novo perfil sem sobrescrever</small></span>
          <span><Icon name="check" size={13} /><b>Credenciais</b><small>Proxy é protegido novamente pelo Windows</small></span>
          <span><Icon name="shield" size={13} /><b>Escopo local</b><small>Discord, licença e keys nunca são exportados</small></span>
        </div>
        {lastImport && <div className="backup-import-result"><Icon name="check" size={17} /><span><strong>{lastImport.profiles.length} perfil(is) restaurado(s)</strong><small>{lastImport.profiles.map((profile) => profile.rekeyed ? `${profile.name} (novo UUID)` : profile.name).join(' · ')}</small></span></div>}
      </article>
    </section>

    <section className="settings-panel backup-history">
      <header className="panel-header"><div><span>HISTÓRICO LOCAL</span><h3>Operações recentes</h3></div><strong>{history.length.toString().padStart(2, '0')}</strong></header>
      <div className="backup-history__table">
        {history.length ? history.map((entry) => <article key={entry.id}>
          <span className={entry.status === 'success' ? 'backup-operation backup-operation--success' : 'backup-operation backup-operation--failed'}><Icon name={entry.operation === 'export' ? 'download' : 'upload'} size={14} /><b>{entry.operation === 'export' ? 'EXPORTAÇÃO' : 'RESTAURAÇÃO'}</b></span>
          <span><b>{entry.profileCount} perfil(is)</b><small>{entry.profileNames?.join(' · ') || 'Sem nomes registrados'}</small></span>
          <span><b>{formatBytes(entry.bytes)}</b><small title={entry.archivePath}>{entry.archivePath.split(/[\\/]/).pop()}</small></span>
          <time>{new Date(entry.createdAt).toLocaleString('pt-BR')}</time>
          <em>{entry.status === 'success' ? 'CONCLUÍDO' : 'FALHOU'}</em>
        </article>) : <p className="backup-empty">Nenhuma operação de backup registrada neste dispositivo.</p>}
      </div>
    </section>
  </div>
}

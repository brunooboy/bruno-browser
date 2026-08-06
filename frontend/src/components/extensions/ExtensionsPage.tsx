import { useEffect, useMemo, useState } from 'react'
import type { BrowserProfile } from '../../types/profile'
import type { InstalledExtension } from '../../types/system'
import { Icon } from '../common/Icon'
import { PlatformIcon } from '../profiles/PlatformIcon'

interface ExtensionsPageProps {
  extensions: InstalledExtension[]
  profiles: BrowserProfile[]
  desktopMode: boolean
  premiumActive: boolean
  busy: boolean
  onInstall: () => Promise<void>
  onRemove: (extensionId: string) => Promise<void>
  onSaveAssignments: (extensionId: string, profileIds: string[]) => Promise<void>
}

export function ExtensionsPage({ extensions, profiles, desktopMode, premiumActive, busy, onInstall, onRemove, onSaveAssignments }: ExtensionsPageProps) {
  const [selectedId, setSelectedId] = useState<string | null>(extensions[0]?.id ?? null)
  const selected = useMemo(() => extensions.find((item) => item.id === selectedId) ?? null, [extensions, selectedId])
  const [assignments, setAssignments] = useState<string[]>(selected?.assignedProfileIds ?? [])

  useEffect(() => {
    if (!selectedId && extensions[0]) setSelectedId(extensions[0].id)
    if (selectedId && !extensions.some((item) => item.id === selectedId)) setSelectedId(extensions[0]?.id ?? null)
  }, [extensions, selectedId])

  useEffect(() => setAssignments(selected?.assignedProfileIds ?? []), [selected])

  const toggleProfile = (profileId: string) => {
    setAssignments((current) => current.includes(profileId)
      ? current.filter((id) => id !== profileId)
      : [...current, profileId])
  }

  return (
    <div className="system-page extensions-page">
      <section className="system-hero">
        <div className="system-hero__icon"><Icon name="extensions" size={24} /></div>
        <div>
          <span>EXTENSION VAULT</span>
          <h2>Biblioteca global de extensões</h2>
          <p>Instale o CRX uma única vez. A associação aos perfis é uma operação separada e só entra em vigor na próxima abertura do perfil.</p>
        </div>
        <button className="primary-button" disabled={busy || !desktopMode || !premiumActive} onClick={onInstall} title={!premiumActive ? 'Ative uma key para instalar extensões' : undefined} type="button">
          <Icon name="plus" size={17} /> Instalar CRX
        </button>
      </section>

      {!desktopMode && <div className="system-notice system-notice--warning"><Icon name="alert" /> Abra esta tela no aplicativo Wails para selecionar um arquivo CRX do disco.</div>}

      <div className="extensions-layout">
        <section className="system-panel extension-library">
          <header><div><span>INSTALADAS</span><h3>{extensions.length} extensões no cofre</h3></div></header>
          {extensions.length === 0 ? (
            <div className="system-empty"><Icon name="extensions" size={28} /><b>Nenhuma extensão instalada</b><span>Use “Instalar CRX” para importar e validar um pacote.</span></div>
          ) : extensions.map((extension) => (
            <button className={selectedId === extension.id ? 'extension-row extension-row--active' : 'extension-row'} key={extension.id} onClick={() => setSelectedId(extension.id)} type="button">
              <span className="extension-row__glyph">{extension.name.slice(0, 1).toUpperCase()}</span>
              <span><strong>{extension.name}</strong><small>v{extension.version} • MV{extension.manifestVersion}</small></span>
              <em>{extension.assignedProfileIds.length} PERFIS</em>
            </button>
          ))}
        </section>

        <section className="system-panel extension-assignment">
          {selected ? <>
            <header>
              <div><span>ATRIBUIÇÃO POR PERFIL</span><h3>{selected.name}</h3></div>
              <button className="ghost-button danger-button" disabled={busy || !desktopMode || !premiumActive} onClick={() => onRemove(selected.id)} type="button"><Icon name="trash" size={15} /> Desinstalar</button>
            </header>
            <p className="extension-description">{selected.description || 'A extensão não forneceu descrição no manifest.json.'}</p>
            <div className="assignment-list">
              {profiles.map((profile) => {
                const checked = assignments.includes(profile.id)
                return <label className={checked ? 'assignment-row assignment-row--active' : 'assignment-row'} key={profile.id}>
                  <input checked={checked} disabled={busy} onChange={() => toggleProfile(profile.id)} type="checkbox" />
                  <span className="proxy-table__platforms">{profile.platforms.slice(0, 2).map((platform) => <PlatformIcon key={platform} platform={platform} />)}</span>
                  <span><strong>{profile.name}</strong><small>{profile.id}</small></span>
                  <i><Icon name="check" size={13} /></i>
                </label>
              })}
            </div>
            <footer className="system-panel__footer">
              <span><b>{assignments.length}</b> perfil(is) selecionado(s)</span>
              <button className="primary-button" disabled={busy || !desktopMode || !premiumActive} onClick={() => onSaveAssignments(selected.id, assignments)} type="button"><Icon name="check" size={16} /> Salvar associação</button>
            </footer>
          </> : <div className="system-empty"><Icon name="layers" size={28} /><b>Selecione uma extensão</b><span>As opções de perfil aparecerão aqui.</span></div>}
        </section>
      </div>
    </div>
  )
}

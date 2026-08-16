import { useEffect, useMemo, useRef, useState } from 'react'
import type { BrowserProfile, DNSPreset, ProxyDraft, ProxyMode } from '../../types/profile'
import { Icon } from '../common/Icon'
import { Modal } from '../common/Modal'
import { PlatformIcon } from '../profiles/PlatformIcon'

interface ProxyModalProps {
  initialProfile: BrowserProfile | null
  open: boolean
  profiles: BrowserProfile[]
  onClose: () => void
  onSave: (draft: ProxyDraft) => void
}

const emptyDraft: ProxyDraft = {
  profileId: '', mode: 'direct', dnsPreset: 'normal', host: '', port: '', username: '', password: '', clearPassword: false, bypassList: '',
}

const dnsPresets: { id: DNSPreset; label: string; provider: string; detail: string }[] = [
  { id: 'light', label: 'LEVE', provider: 'Automático', detail: 'DoH quando disponível, com fallback do sistema' },
  { id: 'normal', label: 'NORMAL', provider: 'Cloudflare', detail: 'DoH privado e sem filtro de conteúdo' },
  { id: 'high', label: 'ALTO', provider: 'Quad9', detail: 'DoH com bloqueio de domínios maliciosos' },
  { id: 'pro', label: 'PRO', provider: 'AdGuard', detail: 'Bloqueia anúncios, rastreadores e phishing' },
  { id: 'pro_plus', label: 'PRO +', provider: 'AdGuard Family', detail: 'PRO com filtro adulto e busca segura' },
]

export function ProxyModal({ initialProfile, open, profiles, onClose, onSave }: ProxyModalProps) {
  const [draft, setDraft] = useState<ProxyDraft>(emptyDraft)
  const [error, setError] = useState('')
  const initializedForCurrentOpen = useRef(false)
  const selectedProfile = useMemo(() => profiles.find((profile) => profile.id === draft.profileId) ?? null, [draft.profileId, profiles])

  useEffect(() => {
    if (!open) {
      initializedForCurrentOpen.current = false
      return
    }
    // Profiles are refreshed from the Go backend every few seconds. Hydrate
    // the form only once per opening so a refresh cannot overwrite edits that
    // the user is currently making.
    if (initializedForCurrentOpen.current) return
    const selected = initialProfile ?? profiles[0] ?? null
    if (!selected) return
    const proxy = selected?.proxy
    initializedForCurrentOpen.current = true
    setError('')
    setDraft({
      profileId: selected.id,
      mode: proxy?.mode ?? 'direct',
      dnsPreset: selected.dnsPreset ?? 'normal',
      host: proxy?.host ?? '',
      port: proxy ? String(proxy.port) : '',
      username: proxy?.username ?? '',
      password: '',
      clearPassword: false,
      bypassList: proxy?.bypassList.join('\n') ?? '',
    })
  }, [initialProfile, open, profiles])

  const selectProfile = (profileId: string) => {
    const profile = profiles.find((item) => item.id === profileId)
    const proxy = profile?.proxy
    setError('')
    setDraft({
      profileId,
      mode: proxy?.mode ?? 'direct',
      dnsPreset: profile?.dnsPreset ?? 'normal',
      host: proxy?.host ?? '',
      port: proxy ? String(proxy.port) : '',
      username: proxy?.username ?? '',
      password: '',
      clearPassword: false,
      bypassList: proxy?.bypassList.join('\n') ?? '',
    })
  }

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault()
    if (!draft.profileId) {
      setError('Selecione um perfil.')
      return
    }
    if (draft.mode !== 'direct') {
      const port = Number(draft.port)
      if (!draft.host.trim()) {
        setError('Informe o endereço do proxy.')
        return
      }
      if (!Number.isInteger(port) || port < 1 || port > 65_535) {
        setError('Informe uma porta entre 1 e 65535.')
        return
      }
      if (draft.password && !draft.username.trim()) {
        setError('Informe o usuário correspondente à senha.')
        return
      }
    }
    onSave({ ...draft, host: draft.host.trim(), username: draft.username.trim() })
  }

  const setMode = (mode: ProxyMode) => setDraft((current) => ({ ...current, mode }))

  return (
    <Modal description="Rota independente, DNS automático e proteção WebRTC." onClose={onClose} open={open} title="Configurar rede do perfil" width="lg">
      <form className="proxy-form" onSubmit={handleSubmit}>
        <div className="proxy-form__body">
          <section className="form-section">
            <div className="form-section__heading"><span>01</span><div><h3>Perfil de destino</h3><p>A configuração será aplicada somente ao perfil escolhido.</p></div></div>
            <label className="field-control">
              <span>Perfil</span>
              <select onChange={(event) => selectProfile(event.target.value)} value={draft.profileId}>
                {profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.name} • {profile.id}</option>)}
              </select>
            </label>
            {selectedProfile && <div className="proxy-selected-profile">
              <div>{selectedProfile.platforms.slice(0, 3).map((platform) => <PlatformIcon key={platform} platform={platform} />)}</div>
              <span><strong>{selectedProfile.name}</strong><small>{selectedProfile.proxy ? 'Rota existente será atualizada' : 'Atualmente em conexão direta'}</small></span>
              <Icon name="shield" size={19} />
            </div>}
          </section>

          <section className="form-section">
            <div className="form-section__heading"><span>02</span><div><h3>Modo de conexão</h3><p>Escolha conexão direta ou um proxy dedicado.</p></div></div>
            <div className="proxy-mode-picker">
              {(['direct', 'http', 'socks5'] as ProxyMode[]).map((mode) => (
                <button className={draft.mode === mode ? 'active' : ''} key={mode} onClick={() => setMode(mode)} type="button">
                  <Icon name={mode === 'direct' ? 'globe' : mode === 'http' ? 'network' : 'shield'} size={18} />
                  <span><strong>{mode === 'direct' ? 'Direto' : mode.toUpperCase()}</strong><small>{mode === 'direct' ? 'DNS automático' : mode === 'http' ? 'HTTP / HTTPS' : 'DNS remoto'}</small></span>
                  <i><Icon name="check" size={12} /></i>
                </button>
              ))}
            </div>
          </section>

          <section className="form-section">
            <div className="form-section__heading"><span>03</span><div><h3>Nível de DNS</h3><p>Escolha uma política DoH real. DNS melhora privacidade e filtragem; não altera o IP público nem substitui proxy.</p></div></div>
            <div className={`dns-preset-picker ${draft.mode !== 'direct' ? 'dns-preset-picker--disabled' : ''}`}>
              {dnsPresets.map((preset) => (
                <button className={draft.dnsPreset === preset.id ? 'active' : ''} disabled={draft.mode !== 'direct'} key={preset.id} onClick={() => setDraft((current) => ({ ...current, dnsPreset: preset.id }))} type="button">
                  <span>{preset.label}</span><strong>{preset.provider}</strong><small>{preset.detail}</small><i><Icon name="check" size={11} /></i>
                </button>
              ))}
            </div>
            {draft.mode !== 'direct' && <p className="dns-proxy-notice"><Icon name="shield" size={14} /> Com proxy, a resolução local fica desligada e os domínios seguem pela rota remota. O nível escolhido fica salvo para quando o perfil voltar ao modo direto.</p>}
          </section>

          {draft.mode !== 'direct' ? <section className="form-section">
            <div className="form-section__heading"><span>04</span><div><h3>Endpoint e autenticação</h3><p>A senha será cifrada e nunca aparecerá nas flags do Chromium.</p></div></div>
            <div className="proxy-endpoint-fields">
              <label className="field-control"><span>Host ou IP</span><input onChange={(event) => setDraft((current) => ({ ...current, host: event.target.value }))} placeholder="proxy.example.com" value={draft.host} /></label>
              <label className="field-control"><span>Porta</span><input inputMode="numeric" onChange={(event) => setDraft((current) => ({ ...current, port: event.target.value.replace(/\D/g, '') }))} placeholder={draft.mode === 'socks5' ? '1080' : '8080'} value={draft.port} /></label>
            </div>
            <div className="proxy-auth-fields">
              <label className="field-control"><span>Usuário opcional</span><input autoComplete="off" onChange={(event) => setDraft((current) => ({ ...current, username: event.target.value }))} placeholder="operator" value={draft.username} /></label>
              <label className="field-control"><span>{selectedProfile?.proxy?.hasPassword ? 'Nova senha (vazio mantém a atual)' : 'Senha opcional'}</span><input autoComplete="new-password" onChange={(event) => setDraft((current) => ({ ...current, password: event.target.value, clearPassword: false }))} placeholder="••••••••••••" type="password" value={draft.password} /></label>
            </div>
            {selectedProfile?.proxy?.hasPassword && <label className="proxy-clear-password"><input checked={draft.clearPassword} onChange={(event) => setDraft((current) => ({ ...current, clearPassword: event.target.checked, password: '' }))} type="checkbox" /><span>Remover a senha armazenada</span></label>}
            <label className="field-control">
              <span>Exceções opcionais — uma por linha</span>
              <textarea onChange={(event) => setDraft((current) => ({ ...current, bypassList: event.target.value }))} placeholder={'localhost\n*.internal.example'} rows={3} value={draft.bypassList} />
            </label>
          </section> : <section className="direct-dns-card">
            <Icon name="globe" size={25} />
            <div><span>CONEXÃO DIRETA PROTEGIDA</span><strong>{dnsPresets.find((preset) => preset.id === draft.dnsPreset)?.provider} ativo</strong><p>{dnsPresets.find((preset) => preset.id === draft.dnsPreset)?.detail}. O WebRTC permanece impedido de usar rotas UDP paralelas.</p></div>
          </section>}
        </div>

        <aside className="proxy-form__security">
          <span>NETWORK GUARD</span>
          <h3>Políticas aplicadas</h3>
          <div><Icon name="shield" size={17} /><span><strong>WebRTC LOCK</strong><small>UDP fora da rota bloqueado</small></span><i /></div>
          <div><Icon name="globe" size={17} /><span><strong>{draft.mode === 'direct' ? `DNS ${draft.dnsPreset.replace('_', ' ').toUpperCase()}` : 'DNS REMOTO'}</strong><small>{draft.mode === 'direct' ? 'Política DoH por perfil' : 'Pré-resolução local bloqueada'}</small></span><i /></div>
          <div><Icon name="network" size={17} /><span><strong>SEM DIRECT FALLBACK</strong><small>Falha fechada quando há proxy</small></span><i /></div>
          <div><Icon name="shield" size={17} /><span><strong>CREDENCIAL CIFRADA</strong><small>Fora das flags e metadados</small></span><i /></div>
          <p>Você pode salvar a rota a qualquer momento. Se o perfil estiver aberto, ela será aplicada automaticamente na próxima abertura.</p>
        </aside>

        <footer className="modal-footer proxy-form__footer">
          <span className={error ? 'form-error form-error--visible' : 'form-error'}><Icon name="alert" size={14} /> {error}</span>
          <div><button className="ghost-button" onClick={onClose} type="button">Cancelar</button><button className="primary-button" type="submit"><Icon name="check" size={16} /> Salvar configuração</button></div>
        </footer>
      </form>
    </Modal>
  )
}

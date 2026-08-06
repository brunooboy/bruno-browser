import { useEffect, useMemo, useState } from 'react'
import { invoke, isDesktop, onRuntimeEvent } from './bridge/desktop'
import { AdminPage } from './components/admin/AdminPage'
import { ToastStack, type ToastMessage } from './components/common/Toast'
import { ActivityChart } from './components/dashboard/ActivityChart'
import { Sidebar } from './components/dashboard/Sidebar'
import { StatsGrid } from './components/dashboard/StatsGrid'
import { TopBar } from './components/dashboard/TopBar'
import { ExtensionsPage } from './components/extensions/ExtensionsPage'
import { ProfileModal } from './components/modals/ProfileModal'
import { TagManagerModal } from './components/modals/TagManagerModal'
import { ProfileCard, type ProfileAction } from './components/profiles/ProfileCard'
import { ProfileEmptyState } from './components/profiles/ProfileEmptyState'
import { ProfileFilters } from './components/profiles/ProfileFilters'
import { ProxyModal } from './components/proxies/ProxyModal'
import { ProxyPage } from './components/proxies/ProxyPage'
import { SettingsPage } from './components/settings/SettingsPage'
import { UpdatesPage } from './components/updates/UpdatesPage'
import { initialProfiles, initialTags } from './data/demoProfiles'
import { useNow } from './hooks/useNow'
import { createProfileId, sortProfiles } from './lib/profile'
import { loadNotifications, persistNotifications, prependNotification } from './lib/notifications'
import type { BrowserProfile, Platform, ProfileDraft, ProfileStatus, ProfileTag, ProxyDraft } from './types/profile'
import type { AppNotification, BootstrapState, DashboardTelemetry, DiscordAccount, InstalledExtension, KeyClaims, KeyHistoryEntry, LicensePlan, NativeProfile, PlanStatus, UpdateStatus } from './types/system'

let toastSequence = 0

const fallbackUpdates: UpdateStatus = {
  currentVersion: '0.8.0', latestVersion: '0.8.0', updateAvailable: false,
  checkedAt: new Date().toISOString(), source: 'local',
  changelog: [
    { version: '0.8.0', date: '2026-08-06', description: 'Licenciamento estrito, identidade 8-bit, instalador Windows e publicação de releases.' },
    { version: '0.7.0', date: '2026-08-05', description: 'Aplicativo Wails, conta Discord, licenças locais, biblioteca de extensões e central de atualizações.' },
    { version: '0.6.0', date: '2026-08-04', description: 'Fingerprint persistente e proteção CDP aplicada antes da navegação.' },
    { version: '0.5.0', date: '2026-08-03', description: 'Proxy por perfil, DNS automático e bloqueio de vazamento WebRTC.' },
  ],
}

const emptyPlan: PlanStatus = { activated: false, status: 'none' }
const isPlanActive = (plan: PlanStatus) => plan.activated
  && plan.status === 'active'
  && (!plan.expires_at || plan.expires_at * 1000 > Date.now())
const emptyTelemetry: DashboardTelemetry = {
  generatedAt: '1970-01-01T00:00:00.000Z',
  summary: { totalProfiles: 0, newProfilesThisMonth: 0, runningProfiles: 0, successfulLaunches24h: 0, configuredProxies: 0, healthyProxies: 0, proxyHealthPercent: 0, medianProxyLatencyMs: 0, attentionProfiles: 0 },
  signals: { overall: 0, fingerprint: 0, network: 0, sessions: 0, label: 'Aguardando dados', detail: 'O núcleo local ainda não enviou uma leitura' },
  activity: [], profiles: [],
}

const toBrowserProfile = (profile: NativeProfile): BrowserProfile => ({
  id: profile.id,
  name: profile.name,
  color: profile.color,
  createdAt: profile.createdAt,
  platforms: profile.platforms,
  status: profile.status,
  tags: profile.tags,
  notes: profile.notes,
  startUrl: profile.startUrl,
  proxy: profile.proxy ? {
    mode: profile.proxy.mode,
    host: profile.proxy.host,
    port: profile.proxy.port,
    username: profile.proxy.username,
    hasPassword: profile.proxy.hasPassword,
    bypassList: profile.proxy.bypassList,
    location: `${profile.proxy.mode.toUpperCase()} configurado`,
    countryCode: '--',
    endpoint: profile.proxy.endpoint,
    latencyMs: profile.proxy.latencyMs,
  } : null,
  fingerprintScore: profile.fingerprintScore,
  sessions: profile.launchCount,
  lastSeen: profile.lastLaunchedAt ? new Date(profile.lastLaunchedAt).toLocaleString('pt-BR') : 'nunca aberto',
  risk: profile.risk,
  running: profile.running,
  engine: profile.engine,
  fingerprintLabel: profile.fingerprintLabel,
  riskReasons: profile.riskReasons,
})

const errorText = (error: unknown) => {
  const message = error instanceof Error ? error.message : String(error)
  if (message.includes('an active plan is required')) return 'Ative uma key em Configurações para abrir recursos premium.'
  if (message.includes('Discord login is required')) return 'Entre com Discord em Configurações antes de continuar.'
  if (message.includes('Chromium executable was not found')) return 'Chrome, Brave ou Edge não foi encontrado. Configure o caminho do navegador em config.json.'
  return message
}

export default function App() {
  const desktopMode = useMemo(isDesktop, [])
  const [profiles, setProfiles] = useState<BrowserProfile[]>(desktopMode ? [] : initialProfiles)
  const [tags, setTags] = useState<ProfileTag[]>(initialTags)
  const [activeSection, setActiveSection] = useState('profiles')
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [search, setSearch] = useState('')
  const [status, setStatus] = useState<ProfileStatus | 'all'>('all')
  const [platform, setPlatform] = useState<Platform | 'all'>('all')
  const [sort, setSort] = useState<'recent' | 'mature' | 'latency'>('recent')
  const [view, setView] = useState<'grid' | 'list'>('grid')
  const [profileModalOpen, setProfileModalOpen] = useState(false)
  const [tagModalOpen, setTagModalOpen] = useState(false)
  const [proxyModalOpen, setProxyModalOpen] = useState(false)
  const [proxyProfile, setProxyProfile] = useState<BrowserProfile | null>(null)
  const [testingProxyId, setTestingProxyId] = useState<string | null>(null)
  const [editingProfile, setEditingProfile] = useState<BrowserProfile | null>(null)
  const [toasts, setToasts] = useState<ToastMessage[]>([])
  const [busy, setBusy] = useState(false)
  const [account, setAccount] = useState<DiscordAccount | undefined>()
  const [plan, setPlan] = useState<PlanStatus>(emptyPlan)
  const [accentColor, setAccentColor] = useState('#42ff91')
  const [updates, setUpdates] = useState<UpdateStatus>(fallbackUpdates)
  const [extensions, setExtensions] = useState<InstalledExtension[]>([])
  const [oauthConfigured, setOAuthConfigured] = useState(false)
  const [settingsPath, setSettingsPath] = useState('')
  const [keyHistory, setKeyHistory] = useState<KeyHistoryEntry[]>([])
  const [telemetry, setTelemetry] = useState<DashboardTelemetry>(emptyTelemetry)
  const [notifications, setNotifications] = useState<AppNotification[]>(() => loadNotifications(localStorage))
  const [coreOnline, setCoreOnline] = useState(false)
  const now = useNow()
  const premiumActive = !desktopMode || isPlanActive(plan)

  const notify = (message: string, tone: ToastMessage['tone'] = 'success') => {
    const id = ++toastSequence
    setToasts((current) => [...current, { id, message, tone }])
    setNotifications((current) => prependNotification(current, {
      id: crypto.randomUUID(), message, tone, createdAt: new Date().toISOString(), read: false,
    }))
    window.setTimeout(() => setToasts((current) => current.filter((toast) => toast.id !== id)), 3_600)
  }

  const refreshTelemetry = () => {
    if (!desktopMode) return
    invoke<DashboardTelemetry>('GetTelemetry').then(setTelemetry).catch(() => undefined)
  }

  const applyAccent = (color: string) => {
    document.documentElement.style.setProperty('--green', color)
    document.documentElement.style.setProperty('--accent-rgb', color)
    setAccentColor(color)
  }

  const applyBootstrap = (state: BootstrapState) => {
    setProfiles(state.profiles.map(toBrowserProfile))
    const persistedTags = state.profiles.flatMap((profile) => profile.tags)
    setTags((current) => [...current, ...persistedTags.filter((tag) => !current.some((item) => item.id === tag.id))])
    setAccount(state.account)
    setPlan(state.plan)
    applyAccent(state.preferences.accentColor)
    setUpdates(state.updates)
    setExtensions(state.extensions)
    setOAuthConfigured(state.oauthConfigured)
    setSettingsPath(state.settingsPath)
    setTelemetry(state.telemetry)
  }

  useEffect(() => {
    persistNotifications(localStorage, notifications)
  }, [notifications])

  useEffect(() => {
    if (!desktopMode) return
    setBusy(true)
    invoke<BootstrapState>('Bootstrap')
      .then((state) => { applyBootstrap(state); setCoreOnline(true) })
      .catch((error) => { setCoreOnline(false); notify(`Falha ao iniciar o núcleo: ${errorText(error)}`, 'warning') })
      .finally(() => setBusy(false))
  }, [desktopMode])

  useEffect(() => onRuntimeEvent<UpdateStatus>('update:status', (status) => {
    setUpdates(status)
    if (status.updateAvailable) notify(`Atualização ${status.latestVersion} disponível.`, 'info')
  }), [])

  useEffect(() => {
    if (!desktopMode) applyAccent(accentColor)
  }, [])

  useEffect(() => {
    if (!desktopMode || !account?.isAdmin) return
    invoke<KeyHistoryEntry[]>('KeyHistory').then(setKeyHistory).catch(() => setKeyHistory([]))
  }, [account?.isAdmin, desktopMode])

  useEffect(() => {
    if (!desktopMode) return
    const timer = window.setInterval(() => {
      Promise.all([invoke<NativeProfile[]>('ListProfiles'), invoke<DashboardTelemetry>('GetTelemetry')])
        .then(([items, currentTelemetry]) => { setProfiles(items.map(toBrowserProfile)); setTelemetry(currentTelemetry); setCoreOnline(true) })
        .catch(() => setCoreOnline(false))
    }, 3_000)
    return () => window.clearInterval(timer)
  }, [desktopMode])

  useEffect(() => {
    if (!desktopMode || !account?.id) return
    let cancelled = false
    const refreshLicense = () => invoke<PlanStatus>('LicenseStatus')
      .then((status) => { if (!cancelled) setPlan(status) })
      .catch(() => undefined)
    refreshLicense()
    const timer = window.setInterval(refreshLicense, 5_000)
    return () => { cancelled = true; window.clearInterval(timer) }
  }, [account?.id, desktopMode])

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        document.querySelector<HTMLInputElement>('[aria-label="Buscar perfis"]')?.focus()
      }
    }
    window.addEventListener('keydown', handleShortcut)
    return () => window.removeEventListener('keydown', handleShortcut)
  }, [])

  const visibleProfiles = useMemo(() => {
    const term = search.trim().toLocaleLowerCase('pt-BR')
    return sortProfiles(profiles.filter((profileItem) => {
      const matchesSearch = !term || [profileItem.name, profileItem.id, profileItem.notes].some((value) => value.toLocaleLowerCase('pt-BR').includes(term))
      return matchesSearch && (status === 'all' || profileItem.status === status) && (platform === 'all' || profileItem.platforms.includes(platform))
    }), sort, now)
  }, [now, platform, profiles, search, sort, status])

  const openCreateModal = () => { setEditingProfile(null); setProfileModalOpen(true) }

  const handleSaveProfile = async (draft: ProfileDraft) => {
    const statusTag = tags.find((tag) => tag.id === draft.status)
    const selectedTags = tags.filter((tag) => draft.tagIds.includes(tag.id))
    const profileTags = statusTag ? [statusTag, ...selectedTags] : selectedTags
    if (desktopMode) {
      setBusy(true)
      try {
        const input = { ...draft, tags: profileTags }
        const saved = editingProfile
          ? await invoke<NativeProfile>('UpdateProfile', editingProfile.id, input)
          : await invoke<NativeProfile>('CreateProfile', input)
        const viewProfile = toBrowserProfile(saved)
        setProfiles((current) => editingProfile ? current.map((item) => item.id === viewProfile.id ? viewProfile : item) : [viewProfile, ...current])
        notify(`${draft.name} salvo no disco local.`)
      } catch (error) { notify(errorText(error), 'warning'); return } finally { setBusy(false) }
    } else if (editingProfile) {
      setProfiles((current) => current.map((item) => item.id === editingProfile.id ? { ...item, ...draft, tags: profileTags } : item))
      notify(`${draft.name} atualizado na prévia.`)
    } else {
      setProfiles((current) => [{ id: createProfileId(), ...draft, createdAt: new Date().toISOString(), tags: profileTags, notes: draft.notes || 'Nenhuma nota operacional registrada.', proxy: null, fingerprintScore: 0, sessions: 0, lastSeen: 'nunca aberto', risk: 'high' }, ...current])
      notify(`${draft.name} criado na prévia.`)
    }
    setProfileModalOpen(false); setEditingProfile(null)
  }

  const handleProfileAction = async (action: ProfileAction, profileItem: BrowserProfile) => {
    if (action === 'edit') { setEditingProfile(profileItem); setProfileModalOpen(true); return }
    if (!desktopMode) { notify('A manutenção física exige o aplicativo desktop.', 'info'); return }
    const confirmations = { cache: `Limpar histórico e cache de ${profileItem.name}?`, cookies: `Remover cookies e sessões de ${profileItem.name}?`, delete: `Excluir permanentemente o perfil ${profileItem.name} e sua pasta física?` }
    if (!window.confirm(confirmations[action])) return
    setBusy(true)
    try {
      const method = action === 'cache' ? 'ClearProfileCache' : action === 'cookies' ? 'ClearProfileSession' : 'DeleteProfile'
      await invoke(method, profileItem.id)
      if (action === 'delete') setProfiles((current) => current.filter((item) => item.id !== profileItem.id))
      notify(action === 'delete' ? 'Perfil excluído do disco.' : 'Manutenção concluída.')
    } catch (error) { notify(errorText(error), 'warning') } finally { setBusy(false) }
  }

  const handleLaunch = async (profileItem: BrowserProfile) => {
    if (!desktopMode) { notify('Use o aplicativo desktop para iniciar o Chromium real.', 'info'); return }
    if (!profileItem.running && !isPlanActive(plan)) {
      invoke<PlanStatus>('LicenseStatus').then(setPlan).catch(() => undefined)
      notify('Ative uma key válida em Configurações antes de abrir perfis.', 'warning')
      return
    }
    setBusy(true)
    try {
      if (profileItem.running) {
        await invoke('StopProfile', profileItem.id)
        setProfiles((current) => current.map((item) => item.id === profileItem.id ? { ...item, running: false } : item))
        notify(`${profileItem.name} fechado e salvo no disco.`, 'info')
      } else {
        const process = await invoke<{ pid: number }>('LaunchProfile', profileItem.id, '')
        setProfiles((current) => current.map((item) => item.id === profileItem.id ? {
          ...item, running: true, sessions: item.sessions + 1, lastSeen: 'agora',
        } : item))
        notify(`${profileItem.name} aberto em uma janela própria (PID ${process.pid}).`)
      }
      refreshTelemetry()
    }
    catch (error) {
      notify(errorText(error), 'warning')
      invoke<PlanStatus>('LicenseStatus').then(setPlan).catch(() => undefined)
      refreshTelemetry()
    } finally { setBusy(false) }
  }

  const openProxyModal = (profileItem: BrowserProfile | null = null) => {
    if (desktopMode && !premiumActive) { notify('Ative uma key válida para configurar a rede.', 'warning'); return }
    setProxyProfile(profileItem); setProxyModalOpen(true)
  }

  const handleSaveProxy = async (draft: ProxyDraft) => {
    const target = profiles.find((item) => item.id === draft.profileId)
    if (!target) return
    if (desktopMode) {
      setBusy(true)
      try {
        await invoke('SaveNetwork', draft.profileId, {
          mode: draft.mode, host: draft.host, port: Number(draft.port), username: draft.username,
          password: draft.password, clearPassword: draft.clearPassword,
          bypassList: draft.bypassList.split(/\r?\n|,/).map((rule) => rule.trim()).filter(Boolean),
        })
        const refreshed = await invoke<NativeProfile[]>('ListProfiles')
        setProfiles(refreshed.map(toBrowserProfile))
        notify(`Rede de ${target.name} salva no disco.`)
      } catch (error) { notify(errorText(error), 'warning'); return } finally { setBusy(false) }
    } else {
      setProfiles((current) => current.map((item) => item.id !== draft.profileId ? item : draft.mode === 'direct' ? { ...item, proxy: null } : { ...item, proxy: { mode: draft.mode, host: draft.host, port: Number(draft.port), username: draft.username, hasPassword: Boolean(draft.password) || Boolean(item.proxy?.hasPassword), bypassList: draft.bypassList.split(/\r?\n|,/).filter(Boolean), location: 'Rota personalizada', countryCode: '--', endpoint: `${draft.host}:${draft.port}`, latencyMs: 0 } }))
      notify('Rota alterada somente na prévia.', 'info')
    }
    setProxyModalOpen(false); setProxyProfile(null)
  }

  const handleTestProxy = async (profileItem: BrowserProfile) => {
    if (!desktopMode) { notify('O teste de rede real exige o aplicativo desktop.', 'info'); return }
    setTestingProxyId(profileItem.id)
    try {
      const result = await invoke<{ latencyMs: number }>('TestNetwork', profileItem.id)
      setProfiles((current) => current.map((item) => item.id === profileItem.id && item.proxy ? { ...item, proxy: { ...item.proxy, latencyMs: result.latencyMs } } : item))
      notify(`Rota respondeu em ${result.latencyMs} ms.`, 'info')
      refreshTelemetry()
    } catch (error) { notify(errorText(error), 'warning'); refreshTelemetry() } finally { setTestingProxyId(null) }
  }

  const handleAddTag = (label: string, color: string) => {
    setTags((current) => [...current, { id: `custom-${crypto.randomUUID().split('-')[0]}`, label, color, kind: 'custom' }])
    notify(`Tag “${label}” adicionada.`)
  }
  const handleRemoveTag = (id: string) => { setTags((current) => current.filter((item) => item.id !== id)); setProfiles((current) => current.map((item) => ({ ...item, tags: item.tags.filter((tag) => tag.id !== id) }))) }

  const handleCheckUpdates = async () => {
    setBusy(true)
    try { const result = await invoke<UpdateStatus>('CheckForUpdates'); setUpdates(result); notify(result.updateAvailable ? `Atualização ${result.latestVersion} disponível.` : 'Você já está na versão mais recente.', 'info') }
    catch (error) { notify(errorText(error), 'warning') } finally { setBusy(false) }
  }

  const handleSaveTheme = async (color: string) => {
    setBusy(true)
    try { await invoke('SavePreferences', { accentColor: color }); applyAccent(color); notify('Tema salvo no disco.') }
    catch (error) { notify(errorText(error), 'warning') } finally { setBusy(false) }
  }

  const handleLogin = async () => {
    setBusy(true)
    try { const user = await invoke<DiscordAccount>('LoginDiscord'); setAccount(user); setPlan(await invoke<PlanStatus>('LicenseStatus')); notify(`Discord conectado como ${user.username}.`) }
    catch (error) { notify(errorText(error), 'warning') } finally { setBusy(false) }
  }
  const handleLogout = async () => { setBusy(true); try { await invoke('LogoutDiscord'); setAccount(undefined); setPlan(emptyPlan); notify('Conta desconectada.') } catch (error) { notify(errorText(error), 'warning') } finally { setBusy(false) } }
  const handleActivate = async (key: string) => { setBusy(true); try { setPlan(await invoke<PlanStatus>('ActivateKey', key)); notify('Key ativada e validada localmente.') } catch (error) { notify(errorText(error), 'warning') } finally { setBusy(false) } }
  const handleDeactivate = async () => { setBusy(true); try { await invoke('DeactivateKey'); setPlan(emptyPlan); notify('Key removida deste dispositivo.', 'info') } catch (error) { notify(errorText(error), 'warning') } finally { setBusy(false) } }

  const handleInstallExtension = async () => { setBusy(true); try { await invoke('InstallExtension'); setExtensions(await invoke<InstalledExtension[]>('ListExtensions')); notify('CRX validado e instalado no cofre.') } catch (error) { if (!errorText(error).includes('cancelled')) notify(errorText(error), 'warning') } finally { setBusy(false) } }
  const handleAssignExtension = async (extensionId: string, profileIds: string[]) => { setBusy(true); try { const saved = await invoke<InstalledExtension>('SetExtensionAssignments', extensionId, profileIds); setExtensions((current) => current.map((item) => item.id === saved.id ? saved : item)); notify('Associação salva. Será aplicada na próxima abertura.') } catch (error) { notify(errorText(error), 'warning') } finally { setBusy(false) } }
  const handleRemoveExtension = async (extensionId: string) => { if (!window.confirm('Desinstalar esta extensão e removê-la de todos os perfis associados?')) return; setBusy(true); try { await invoke('RemoveExtension', extensionId); setExtensions((current) => current.filter((item) => item.id !== extensionId)); notify('Extensão desassociada dos perfis e desinstalada.') } catch (error) { notify(errorText(error), 'warning') } finally { setBusy(false) } }

  const handleGenerateKey = async (discordId: string, keyPlan: LicensePlan) => {
    setBusy(true)
    try {
      const entry = await invoke<KeyHistoryEntry>('GenerateKey', discordId, keyPlan)
      setKeyHistory((current) => [entry, ...current]); notify('Key gerada e salva no histórico local.')
      return entry
    } catch (error) { notify(errorText(error), 'warning'); throw error } finally { setBusy(false) }
  }
  const handleInspectKey = async (key: string) => {
    setBusy(true)
    try { return await invoke<KeyClaims>('InspectKey', key) }
    catch (error) { notify(errorText(error), 'warning'); throw error } finally { setBusy(false) }
  }
  const handleCopy = async (value: string) => {
    try {
      if (desktopMode) await invoke('CopyText', value)
      else await navigator.clipboard.writeText(value)
      notify('Key copiada para a área de transferência.', 'info')
    } catch (error) { notify(errorText(error), 'warning') }
  }

  const handleNavigate = (section: string) => { if (section === 'tags') { setTagModalOpen(true); return }; setActiveSection(section); setSearch('') }
  const resetFilters = () => { setSearch(''); setStatus('all'); setPlatform('all') }

  const topbar = activeSection === 'proxies'
    ? { contextLabel: 'NETWORK OPERATIONS', title: 'Roteamento & DNS', statusLabel: coreOnline ? 'CONFIGURAÇÃO LOCAL' : 'NÚCLEO INDISPONÍVEL', actionLabel: 'Configurar rota', onCreate: () => openProxyModal(), onSearch: setSearch, searchPlaceholder: 'Buscar rota ou perfil...' }
    : activeSection === 'extensions' ? { contextLabel: 'EXTENSION OPERATIONS', title: 'Extensões', statusLabel: 'COFRE LOCAL' }
    : activeSection === 'updates' ? { contextLabel: 'SYSTEM RELEASES', title: 'Atualizações', statusLabel: 'CANAL ESTÁVEL' }
    : activeSection === 'settings' ? { contextLabel: 'LOCAL CONTROL', title: 'Configurações', statusLabel: desktopMode ? 'APP DESKTOP' : 'MODO PRÉVIA' }
    : activeSection === 'admin' ? { contextLabel: 'LICENSE CONTROL', title: 'Painel Admin', statusLabel: 'ACESSO RESTRITO' }
    : { contextLabel: 'CENTRAL DE OPERAÇÕES', title: 'Radar de perfis', statusLabel: desktopMode ? (coreOnline ? 'NÚCLEO LOCAL ONLINE' : 'NÚCLEO INDISPONÍVEL') : 'MODO PRÉVIA', actionLabel: 'Novo perfil', onCreate: openCreateModal, onSearch: setSearch, searchPlaceholder: 'Buscar perfil ou ID...' }

  return <div className={`app-shell ${sidebarCollapsed ? 'app-shell--collapsed' : ''}`}>
    <Sidebar active={activeSection} collapsed={sidebarCollapsed} desktopMode={desktopMode} isAdmin={account?.isAdmin} onNavigate={handleNavigate} onToggle={() => setSidebarCollapsed((current) => !current)} profileCount={profiles.length} />
    <main className="main-content">
      <TopBar {...topbar} notifications={notifications} onClearNotifications={() => setNotifications([])} onReadNotifications={() => setNotifications((current) => current.map((item) => ({ ...item, read: true })))} search={search} />
      {activeSection === 'proxies' ? <ProxyPage onConfigure={openProxyModal} onTest={handleTestProxy} premiumActive={premiumActive} profiles={profiles.filter((item) => !search.trim() || [item.name, item.id, item.proxy?.host ?? 'direto'].some((value) => value.toLowerCase().includes(search.toLowerCase())))} testingId={testingProxyId} />
      : activeSection === 'extensions' ? <ExtensionsPage busy={busy} desktopMode={desktopMode} extensions={extensions} onInstall={handleInstallExtension} onRemove={handleRemoveExtension} onSaveAssignments={handleAssignExtension} premiumActive={premiumActive} profiles={profiles} />
      : activeSection === 'updates' ? <UpdatesPage busy={busy} desktopMode={desktopMode} onCheck={handleCheckUpdates} status={updates} />
      : activeSection === 'settings' ? <SettingsPage account={account} busy={busy} desktopMode={desktopMode} oauthConfigured={oauthConfigured} onActivate={handleActivate} onDeactivate={handleDeactivate} onLogin={handleLogin} onLogout={handleLogout} onSaveTheme={handleSaveTheme} plan={plan} preferences={{ accentColor }} settingsPath={settingsPath} />
      : activeSection === 'admin' && account?.isAdmin ? <AdminPage busy={busy} history={keyHistory} onCopy={handleCopy} onGenerate={handleGenerateKey} onInspect={handleInspectKey} />
      : <div className="dashboard-content">
        <StatsGrid telemetry={telemetry} /><ActivityChart telemetry={telemetry} />
        <ProfileFilters count={visibleProfiles.length} onManageTags={() => setTagModalOpen(true)} onPlatformChange={setPlatform} onSortChange={setSort} onStatusChange={setStatus} onViewChange={setView} platform={platform} sort={sort} status={status} view={view} />
        {visibleProfiles.length ? <section aria-label="Lista de perfis" className={`profiles-layout profiles-layout--${view}`}>{visibleProfiles.map((item) => <ProfileCard key={item.id} now={now} onAction={handleProfileAction} onLaunch={handleLaunch} premiumActive={premiumActive} profile={item} view={view} />)}</section> : <ProfileEmptyState onReset={resetFilters} />}
        <footer className="dashboard-footer"><span>BRUNO BROWSER // LOCAL OPERATIONS TERMINAL</span><span><i /> DADOS DO PERFIL: DISCO LOCAL</span><span>BUILD 0.8.0</span></footer>
      </div>}
    </main>
    <ProfileModal onClose={() => { setProfileModalOpen(false); setEditingProfile(null) }} onManageTags={() => setTagModalOpen(true)} onSave={handleSaveProfile} open={profileModalOpen} profile={editingProfile} tags={tags} />
    <TagManagerModal onAdd={handleAddTag} onClose={() => setTagModalOpen(false)} onRemove={handleRemoveTag} open={tagModalOpen} tags={tags} />
    <ProxyModal initialProfile={proxyProfile} onClose={() => { setProxyModalOpen(false); setProxyProfile(null) }} onSave={handleSaveProxy} open={proxyModalOpen} profiles={profiles} />
    <ToastStack toasts={toasts} />
  </div>
}

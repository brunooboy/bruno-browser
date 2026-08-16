import type { DNSPreset, Platform, ProfileStatus, ProfileTag, ProxyMode } from './profile'

export interface NativeProxy {
  mode: Exclude<ProxyMode, 'direct'>
  host: string
  port: number
  username: string
  hasPassword: boolean
  bypassList: string[]
  endpoint: string
  latencyMs: number
}

export interface NativeProfile {
  id: string
  name: string
  color: string
  createdAt: string
  updatedAt: string
  lastLaunchedAt?: string
  launchCount: number
  platforms: Platform[]
  status: ProfileStatus
  tags: ProfileTag[]
  notes: string
  startUrl: string
  lastUrl: string
  extensionPaths: string[]
  dnsPreset: DNSPreset
  proxy: NativeProxy | null
  running: boolean
  engine: 'bruno' | 'chromium' | 'unavailable'
  fingerprintScore: number
  fingerprintLabel: string
  risk: 'low' | 'medium' | 'high'
  riskReasons: string[]
}

export interface TelemetryActivityBucket {
  startedAt: string
  launches: number
  uniqueProfiles: number
  profilesCreated: number
  proxyTests: number
  failures: number
}

export interface TelemetryProfileMetric {
  profileId: string
  engine: string
  fingerprintScore: number
  fingerprintLabel: string
  risk: 'low' | 'medium' | 'high'
  riskReasons: string[]
  proxyLatencyMs: number
  proxyTested: boolean
  proxyHealthy: boolean
  lastProxyTestAt?: string
}

export interface DashboardTelemetry {
  generatedAt: string
  summary: {
    totalProfiles: number
    newProfilesThisMonth: number
    runningProfiles: number
    successfulLaunches24h: number
    configuredProxies: number
    healthyProxies: number
    proxyHealthPercent: number
    medianProxyLatencyMs: number
    attentionProfiles: number
  }
  signals: {
    overall: number
    fingerprint: number
    network: number
    sessions: number
    label: string
    detail: string
  }
  activity: TelemetryActivityBucket[]
  profiles: TelemetryProfileMetric[]
}

export interface AppNotification {
  id: string
  message: string
  tone: 'success' | 'info' | 'warning'
  createdAt: string
  read: boolean
}

export interface DiscordAccount {
  id: string
  username: string
  globalName?: string
  avatar?: string
  avatarUrl?: string
  loggedInAt: string
  isAdmin: boolean
}

export type LicensePlan = 'VITALICIO' | '30' | '7' | '1'

export interface PlanStatus {
  activated: boolean
  plan?: LicensePlan
  expires_at?: number
  key_id?: string
  activated_at?: number
  status: 'active' | 'expired' | 'none'
}

export interface ChangelogEntry {
  version: string
  date: string
  description: string
}

export interface UpdateStatus {
  currentVersion: string
  latestVersion: string
  updateAvailable: boolean
  installAvailable: boolean
  installReason?: string
  asset?: {
    name: string
    url: string
    sha256: string
    size: number
  }
  checkedAt: string
  source: string
  changelog: ChangelogEntry[]
}

export interface UpdateDownloadResult {
  version: string
  path: string
  name: string
  sha256: string
  bytes: number
  ready: boolean
}

export interface InstalledExtension {
  id: string
  name: string
  version: string
  description?: string
  manifestVersion: number
  installedAt: string
  path: string
  assignedProfileIds: string[]
  bundled: boolean
}

export interface AppPreferences {
  accentColor: string
}

export interface DiagnosticCheck {
  id: string
  label: string
  status: 'pass' | 'warning' | 'fail'
  detail: string
}

export interface DiagnosticIncident {
  at: string
  scope: string
  message: string
}

export interface SystemDiagnostics {
  generatedAt: string
  status: 'ready' | 'attention' | 'blocked'
  checks: DiagnosticCheck[]
  incidents: DiagnosticIncident[]
}

export interface BootstrapState {
  profiles: NativeProfile[]
  account?: DiscordAccount
  plan: PlanStatus
  preferences: AppPreferences
  updates: UpdateStatus
  extensions: InstalledExtension[]
  oauthConfigured: boolean
  settingsPath: string
  telemetry: DashboardTelemetry
  diagnostics: SystemDiagnostics
}

export interface KeyClaims {
  discord_id: string
  plan: LicensePlan
  created_at: number
  expires_at: number
  key_id: string
}

export interface KeyHistoryEntry {
  claims: KeyClaims
  key: string
  generated_at: number
}

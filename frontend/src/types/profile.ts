export type Platform =
  | 'instagram'
  | 'x'
  | 'outlook'
  | 'facebook'
  | 'google'
  | 'tiktok'

export type ProfileStatus = 'starting' | 'warming' | 'fixed' | 'farm'

export type RiskLevel = 'low' | 'medium' | 'high'

export type ProxyMode = 'direct' | 'http' | 'socks5'

export type DNSPreset = 'light' | 'normal' | 'high' | 'pro' | 'pro_plus'

export interface ProfileTag {
  id: string
  label: string
  color: string
  kind: 'status' | 'custom'
}

export interface ProfileProxy {
  mode: Exclude<ProxyMode, 'direct'>
  host: string
  port: number
  username: string
  hasPassword: boolean
  bypassList: string[]
  location: string
  countryCode: string
  endpoint: string
  latencyMs: number
}

export interface ProxyDraft {
  profileId: string
  mode: ProxyMode
  dnsPreset: DNSPreset
  host: string
  port: string
  username: string
  password: string
  clearPassword: boolean
  bypassList: string
}

export interface BrowserProfile {
  id: string
  name: string
  color: string
  createdAt: string
  platforms: Platform[]
  status: ProfileStatus
  tags: ProfileTag[]
  notes: string
  startUrl: string
  dnsPreset?: DNSPreset
  proxy: ProfileProxy | null
  fingerprintScore: number
  sessions: number
  lastSeen: string
  risk: RiskLevel
  running?: boolean
  engine?: string
  fingerprintLabel?: string
  riskReasons?: string[]
}

export interface ProfileDraft {
  name: string
  color: string
  platforms: Platform[]
  status: ProfileStatus
  tagIds: string[]
  notes: string
  startUrl: string
}

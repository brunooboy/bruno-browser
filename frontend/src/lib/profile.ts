import type { BrowserProfile, ProfileStatus } from '../types/profile'

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

export const statusLabels: Record<ProfileStatus, string> = {
  starting: 'Iniciando',
  warming: 'Aquecendo',
  fixed: 'Operação fixa',
  farm: 'Farm',
}

export function getAgeMs(createdAt: string, now = Date.now()) {
  return Math.max(0, now - new Date(createdAt).getTime())
}

export function formatAge(createdAt: string, now = Date.now()) {
  const age = getAgeMs(createdAt, now)

  if (age < HOUR) {
    return `${Math.max(1, Math.floor(age / MINUTE))} min`
  }

  if (age < 48 * HOUR) {
    const hours = Math.floor(age / HOUR)
    const minutes = Math.floor((age % HOUR) / MINUTE)
    return `${hours}h ${minutes.toString().padStart(2, '0')}m`
  }

  const days = Math.floor(age / DAY)
  const hours = Math.floor((age % DAY) / HOUR)
  return `${days}d ${hours}h`
}

export function maturityFor(createdAt: string, now = Date.now()) {
  const days = getAgeMs(createdAt, now) / DAY
  const percentage = Math.min(100, Math.max(4, Math.round((days / 30) * 100)))

  if (days < 1) return { percentage, label: 'Nova' }
  if (days < 7) return { percentage, label: 'Em aquecimento' }
  if (days < 30) return { percentage, label: 'Em consolidação' }
  return { percentage, label: 'Madura' }
}

export function sortProfiles(
  profiles: BrowserProfile[],
  order: 'recent' | 'mature' | 'latency',
  now = Date.now(),
) {
  return [...profiles].sort((a, b) => {
    if (order === 'mature') {
      return getAgeMs(b.createdAt, now) - getAgeMs(a.createdAt, now)
    }
    if (order === 'latency') {
      return (a.proxy?.latencyMs ?? Number.MAX_SAFE_INTEGER) -
        (b.proxy?.latencyMs ?? Number.MAX_SAFE_INTEGER)
    }
    return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
  })
}

export function createProfileId() {
  const suffix = crypto.randomUUID().split('-')[0].toUpperCase()
  return `BB-${suffix}`
}

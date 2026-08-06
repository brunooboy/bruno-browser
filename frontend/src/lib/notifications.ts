import type { AppNotification } from '../types/system'

export const notificationStorageKey = 'bruno-browser.notifications.v1'
export const maxStoredNotifications = 10

type NotificationStorage = Pick<Storage, 'getItem' | 'setItem'>

function isNotification(value: unknown): value is AppNotification {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Partial<AppNotification>
  return typeof candidate.id === 'string' && typeof candidate.message === 'string' &&
    typeof candidate.createdAt === 'string' && typeof candidate.read === 'boolean' &&
    ['success', 'info', 'warning'].includes(candidate.tone ?? '')
}

export function loadNotifications(storage: NotificationStorage): AppNotification[] {
  try {
    const parsed: unknown = JSON.parse(storage.getItem(notificationStorageKey) ?? '[]')
    if (!Array.isArray(parsed)) return []
    return parsed.filter(isNotification).slice(0, maxStoredNotifications)
  } catch {
    return []
  }
}

export function prependNotification(
  current: AppNotification[],
  notification: AppNotification,
): AppNotification[] {
  return [notification, ...current].slice(0, maxStoredNotifications)
}

export function persistNotifications(
  storage: NotificationStorage,
  notifications: AppNotification[],
): AppNotification[] {
  const kept = notifications.slice(0, maxStoredNotifications)
  storage.setItem(notificationStorageKey, JSON.stringify(kept))
  return kept
}

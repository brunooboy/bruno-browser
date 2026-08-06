import assert from 'node:assert/strict'
import test from 'node:test'
import {
  loadNotifications,
  maxStoredNotifications,
  notificationStorageKey,
  persistNotifications,
  prependNotification,
} from '../src/lib/notifications.ts'

function memoryStorage() {
  const values = new Map()
  return {
    getItem(key) { return values.get(key) ?? null },
    setItem(key, value) { values.set(key, value) },
  }
}

function notification(number) {
  return {
    id: String(number),
    message: `notificação ${number}`,
    tone: 'info',
    createdAt: new Date(2026, 7, 5, 12, number).toISOString(),
    read: false,
  }
}

test('keeps and persists only the ten newest notifications', () => {
  const storage = memoryStorage()
  let current = []
  for (let number = 1; number <= 12; number += 1) {
    current = prependNotification(current, notification(number))
  }
  const persisted = persistNotifications(storage, current)

  assert.equal(persisted.length, maxStoredNotifications)
  assert.deepEqual(persisted.map((item) => item.id), ['12', '11', '10', '9', '8', '7', '6', '5', '4', '3'])
  assert.equal(JSON.parse(storage.getItem(notificationStorageKey)).length, maxStoredNotifications)
  assert.deepEqual(loadNotifications(storage), persisted)
})

test('ignores malformed local data', () => {
  const storage = memoryStorage()
  storage.setItem(notificationStorageKey, '{invalid')
  assert.deepEqual(loadNotifications(storage), [])
})

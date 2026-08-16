import assert from 'node:assert/strict'
import test from 'node:test'

import { backupPasswordValid, eligibleProfileIds, exportBackupReady } from '../src/lib/backups.ts'

const profiles = [
  { id: 'closed-a', running: false },
  { id: 'running-b', running: true },
  { id: 'closed-c' },
]

test('backup selection excludes running and unknown profiles', () => {
  assert.deepEqual(eligibleProfileIds(profiles, ['closed-a', 'running-b', 'missing', 'closed-a']), ['closed-a'])
})

test('backup requires a matching password with at least eight characters', () => {
  assert.equal(backupPasswordValid('1234567'), false)
  assert.equal(backupPasswordValid('12345678'), true)
  assert.equal(exportBackupReady(profiles, ['closed-c'], 'senha-123', 'senha-123'), true)
  assert.equal(exportBackupReady(profiles, ['running-b'], 'senha-123', 'senha-123'), false)
  assert.equal(exportBackupReady(profiles, ['closed-c'], 'senha-123', 'outra-123'), false)
})

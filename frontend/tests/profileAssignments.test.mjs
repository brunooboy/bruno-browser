import assert from 'node:assert/strict'
import test from 'node:test'
import { retainExistingProfileIds } from '../src/lib/profileAssignments.ts'

test('removes assignments for profiles that no longer exist', () => {
  const result = retainExistingProfileIds(
    ['profile-old', 'profile-current'],
    [{ id: 'profile-current' }, { id: 'profile-new' }],
  )

  assert.deepEqual(result, ['profile-current'])
})

test('keeps a newly-created profile available for assignment', () => {
  const result = retainExistingProfileIds(
    ['profile-current', 'profile-new'],
    [{ id: 'profile-current' }, { id: 'profile-new' }],
  )

  assert.deepEqual(result, ['profile-current', 'profile-new'])
})

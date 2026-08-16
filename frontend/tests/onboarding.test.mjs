import assert from 'node:assert/strict'
import test from 'node:test'
import { activePlan, onboardingProgress } from '../src/lib/onboarding.ts'

const diagnostics = {
  generatedAt: '2026-08-16T00:00:00Z',
  status: 'attention',
  incidents: [],
  checks: [
    { id: 'storage', label: 'Armazenamento', status: 'pass', detail: 'ok' },
    { id: 'engine', label: 'Engine', status: 'pass', detail: 'ok' },
  ],
}

test('recommends the first incomplete real setup step', () => {
  const progress = onboardingProgress({
    diagnostics,
    plan: { activated: false, status: 'none' },
    profileCount: 0,
    now: Date.UTC(2026, 7, 16),
  })
  assert.equal(progress.recommendedIndex, 1)
  assert.equal(progress.steps[0].complete, true)
  assert.equal(progress.steps[1].id, 'account')
})

test('marks setup complete only with system, account, active license and profile', () => {
  const progress = onboardingProgress({
    account: { id: '123', username: 'bruno', loggedInAt: '2026-08-16T00:00:00Z', isAdmin: false },
    diagnostics,
    plan: { activated: true, status: 'active', plan: '30', expires_at: Math.floor(Date.UTC(2026, 8, 16) / 1000) },
    profileCount: 1,
    now: Date.UTC(2026, 7, 16),
  })
  assert.equal(progress.complete, true)
  assert.equal(progress.completedSteps, 4)
})

test('does not accept an expired plan as active', () => {
  assert.equal(activePlan({ activated: true, status: 'active', plan: '1', expires_at: 100 }, 101_000), false)
})

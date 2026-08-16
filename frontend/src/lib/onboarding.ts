import type { DiscordAccount, PlanStatus, SystemDiagnostics } from '../types/system'

export const onboardingStorageKey = 'bruno-browser:onboarding-v1'

export type OnboardingStepId = 'system' | 'account' | 'license' | 'profile'

export interface OnboardingProgressInput {
  account?: DiscordAccount
  diagnostics: SystemDiagnostics
  plan: PlanStatus
  profileCount: number
  now?: number
}

export interface OnboardingStepState {
  id: OnboardingStepId
  complete: boolean
}

export interface OnboardingProgress {
  complete: boolean
  completedSteps: number
  recommendedIndex: number
  steps: OnboardingStepState[]
}

export function activePlan(plan: PlanStatus, now = Date.now()) {
  return plan.activated
    && plan.status === 'active'
    && (!plan.expires_at || plan.expires_at * 1000 > now)
}

export function onboardingProgress(input: OnboardingProgressInput): OnboardingProgress {
  const requiredSystemChecks = ['storage', 'engine']
  const systemReady = requiredSystemChecks.every((id) => input.diagnostics.checks.some((check) => check.id === id && check.status === 'pass'))
  const steps: OnboardingStepState[] = [
    { id: 'system', complete: systemReady },
    { id: 'account', complete: Boolean(input.account?.id) },
    { id: 'license', complete: activePlan(input.plan, input.now) },
    { id: 'profile', complete: input.profileCount > 0 },
  ]
  const recommendedIndex = steps.findIndex((step) => !step.complete)
  return {
    complete: recommendedIndex === -1,
    completedSteps: steps.filter((step) => step.complete).length,
    recommendedIndex: recommendedIndex === -1 ? steps.length : recommendedIndex,
    steps,
  }
}

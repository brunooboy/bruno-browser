interface ProfileIdentity {
  id: string
}

export function retainExistingProfileIds(
  assignments: string[],
  profiles: readonly ProfileIdentity[],
): string[] {
  const existing = new Set(profiles.map((profile) => profile.id))
  const retained = assignments.filter((profileId) => existing.has(profileId))
  return retained.length === assignments.length ? assignments : retained
}

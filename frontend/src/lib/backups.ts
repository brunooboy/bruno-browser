export interface BackupProfileState {
  id: string
  running?: boolean
}

export function eligibleProfileIds(profiles: BackupProfileState[], selected: string[]) {
  const closed = new Set(profiles.filter((profile) => !profile.running).map((profile) => profile.id))
  return [...new Set(selected)].filter((id) => closed.has(id))
}

export function backupPasswordValid(password: string) {
  return [...password].length >= 8 && [...password].length <= 256
}

export function exportBackupReady(profiles: BackupProfileState[], selected: string[], password: string, confirmation: string) {
  return eligibleProfileIds(profiles, selected).length > 0 && backupPasswordValid(password) && password === confirmation
}

import type { Platform } from '../../types/profile'

const platformLabels: Record<Platform, string> = {
  instagram: 'Instagram',
  x: 'X',
  outlook: 'Outlook',
  facebook: 'Facebook',
  google: 'Google',
  tiktok: 'TikTok',
}

export const allPlatforms = Object.keys(platformLabels) as Platform[]

export function PlatformIcon({ platform, size = 'md' }: { platform: Platform; size?: 'sm' | 'md' | 'lg' }) {
  const content = (() => {
    switch (platform) {
      case 'instagram':
        return (
          <svg viewBox="0 0 24 24">
            <rect height="15" rx="4" width="15" x="4.5" y="4.5" />
            <circle cx="12" cy="12" r="3.3" />
            <circle className="platform-fill" cx="17.2" cy="6.9" r="1" />
          </svg>
        )
      case 'x':
        return <span className="platform-letter platform-letter--x">𝕏</span>
      case 'outlook':
        return (
          <svg viewBox="0 0 24 24">
            <path className="platform-fill" d="M3 6h10v12H3z" />
            <path d="m4 8 4 4 4-4M13 8h8v9h-8" />
            <path className="platform-fill platform-fill--soft" d="M15 4h6v4h-6z" />
          </svg>
        )
      case 'facebook':
        return <span className="platform-letter platform-letter--facebook">f</span>
      case 'google':
        return <span className="platform-letter platform-letter--google">G</span>
      case 'tiktok':
        return (
          <svg viewBox="0 0 24 24">
            <path d="M14 4v10.5a4 4 0 1 1-3-3.9V14a1.8 1.8 0 1 0 1 1.6V4Zm0 0c1 2.5 2.5 4 5 4" />
          </svg>
        )
    }
  })()

  return (
    <span
      aria-label={platformLabels[platform]}
      className={`platform-icon platform-icon--${platform} platform-icon--${size}`}
      role="img"
      title={platformLabels[platform]}
    >
      {content}
    </span>
  )
}

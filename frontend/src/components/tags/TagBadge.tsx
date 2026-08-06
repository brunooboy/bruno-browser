import type { ProfileTag } from '../../types/profile'

export function TagBadge({ tag, removable = false, onRemove }: { tag: ProfileTag; removable?: boolean; onRemove?: () => void }) {
  return (
    <span
      className="tag-badge"
      style={{
        '--tag-color': tag.color,
        '--tag-bg': `${tag.color}18`,
        '--tag-border': `${tag.color}55`,
      } as React.CSSProperties}
    >
      <span className="tag-badge__dot" />
      {tag.label}
      {removable && (
        <button aria-label={`Remover ${tag.label}`} onClick={onRemove} type="button">
          ×
        </button>
      )}
    </span>
  )
}

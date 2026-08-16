import type { SVGProps } from 'react'

export type IconName =
  | 'activity'
  | 'alert'
  | 'bell'
  | 'broom'
  | 'check'
  | 'chevronDown'
  | 'clock'
  | 'cookie'
  | 'download'
  | 'edit'
  | 'extensions'
  | 'globe'
  | 'grid'
  | 'layers'
  | 'list'
  | 'more'
  | 'network'
  | 'plus'
  | 'play'
  | 'refresh'
  | 'search'
  | 'settings'
  | 'shield'
  | 'sliders'
  | 'tag'
  | 'trash'
  | 'trend'
  | 'x'
  | 'zap'

const paths: Record<IconName, string[]> = {
  activity: ['M3 12h4l2.4-7 4.2 14 2.4-7H21'],
  alert: ['M12 9v4', 'M12 17h.01', 'M10.3 3.8 2.2 18a2 2 0 0 0 1.7 3h16.2a2 2 0 0 0 1.7-3L13.7 3.8a2 2 0 0 0-3.4 0Z'],
  bell: ['M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9', 'M13.7 21a2 2 0 0 1-3.4 0'],
  broom: ['m3 21 8-8', 'm14 10-4-4 7-3 4 4-3 7Z', 'm5 16 3 3'],
  check: ['m5 12 4 4L19 6'],
  chevronDown: ['m6 9 6 6 6-6'],
  clock: ['M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20Z', 'M12 6v6l4 2'],
  cookie: ['M21 12a9 9 0 1 1-9-9 4 4 0 0 0 4 5 4 4 0 0 0 5 4Z', 'M8.5 8.5h.01', 'M8 15h.01', 'M15 16h.01'],
  download: ['M12 3v12', 'm7 10 5 5 5-5', 'M5 21h14'],
  edit: ['M12 20h9', 'M16.5 3.5a2.1 2.1 0 0 1 3 3L8 18l-4 1 1-4Z'],
  extensions: ['M8 3h4v4h4v4h-4v4H8v-4H4V7h4Z'],
  globe: ['M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20Z', 'M2 12h20', 'M12 2a15 15 0 0 1 0 20', 'M12 2a15 15 0 0 0 0 20'],
  grid: ['M4 4h6v6H4Z', 'M14 4h6v6h-6Z', 'M4 14h6v6H4Z', 'M14 14h6v6h-6Z'],
  layers: ['m12 2 9 5-9 5-9-5Z', 'm3 12 9 5 9-5', 'm3 17 9 5 9-5'],
  list: ['M8 6h13', 'M8 12h13', 'M8 18h13', 'M3 6h.01', 'M3 12h.01', 'M3 18h.01'],
  more: ['M5 12h.01', 'M12 12h.01', 'M19 12h.01'],
  network: ['M5 12h14', 'M12 5v14', 'M5 5l14 14', 'M19 5 5 19'],
  plus: ['M12 5v14', 'M5 12h14'],
  play: ['m8 5 11 7-11 7Z'],
  refresh: ['M20 7h-5V2', 'M4 17h5v5', 'M19 12a7 7 0 0 0-12-5L5 9', 'M5 12a7 7 0 0 0 12 5l2-2'],
  search: ['M21 21 16.7 16.7', 'M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14Z'],
  settings: ['M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z', 'M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2 3.4-.2-.1a1.7 1.7 0 0 0-1.9.3l-.7.4a1.7 1.7 0 0 0-.9 1.6v.2h-4v-.2A1.7 1.7 0 0 0 9 21l-.7-.4a1.7 1.7 0 0 0-1.9-.3l-.2.1-2-3.4.1-.1a1.7 1.7 0 0 0 .3-1.9l-.4-.7a1.7 1.7 0 0 0-1.5-.9h-.2v-4h.2a1.7 1.7 0 0 0 1.5-.9l.4-.7A1.7 1.7 0 0 0 4.3 6l-.1-.1 2-3.4.2.1a1.7 1.7 0 0 0 1.9-.3L9 2a1.7 1.7 0 0 0 .9-1.6V.2h4v.2A1.7 1.7 0 0 0 15 2l.7.4a1.7 1.7 0 0 0 1.9.3l.2-.1 2 3.4-.1.1a1.7 1.7 0 0 0-.3 1.9l.4.7a1.7 1.7 0 0 0 1.5.9h.2v4h-.2a1.7 1.7 0 0 0-1.5.9Z'],
  shield: ['M12 22s8-4 8-11V5l-8-3-8 3v6c0 7 8 11 8 11Z', 'm9 12 2 2 4-5'],
  sliders: ['M4 21v-7', 'M4 10V3', 'M12 21v-9', 'M12 8V3', 'M20 21v-5', 'M20 12V3', 'M1 14h6', 'M9 8h6', 'M17 16h6'],
  tag: ['M20 13 13 20l-9-9V4h7Z', 'M8.5 8.5h.01'],
  trash: ['M3 6h18', 'M8 6V4h8v2', 'm19 6-1 15H6L5 6', 'M10 11v5', 'M14 11v5'],
  trend: ['m3 17 6-6 4 4 8-9', 'M15 6h6v6'],
  x: ['M6 6l12 12', 'M18 6 6 18'],
  zap: ['M13 2 3 14h9l-1 8 10-12h-9Z'],
}

interface IconProps extends SVGProps<SVGSVGElement> {
  name: IconName
  size?: number
}

export function Icon({ name, size = 18, ...props }: IconProps) {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height={size}
      viewBox="0 0 24 24"
      width={size}
      {...props}
    >
      {paths[name].map((path, index) => (
        <path
          d={path}
          key={`${name}-${index}`}
          stroke="currentColor"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth="1.75"
        />
      ))}
    </svg>
  )
}

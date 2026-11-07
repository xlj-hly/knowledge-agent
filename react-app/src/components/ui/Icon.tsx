import type { ReactNode } from 'react'
type IconName =
  | 'grid'
  | 'building'
  | 'users'
  | 'settings'
  | 'user'
  | 'lock'
  | 'logout'
  | 'plus'
  | 'search'
  | 'arrow'
  | 'chevron'
  | 'shield'
  | 'check'
  | 'x'
  | 'alert'
  | 'globe'
  | 'spark'
  | 'refresh'
  | 'book'
  | 'database'
  | 'file'
  | 'chat'
  | 'stop'
  | 'trash'
  | 'menu'
  | 'more'
  | 'copy'
const paths: Record<IconName, ReactNode> = {
  menu: <path d="M4 6h16M4 12h16M4 18h16" />,
  more: <><circle cx="5" cy="12" r="1" /><circle cx="12" cy="12" r="1" /><circle cx="19" cy="12" r="1" /></>,
  copy: <><rect x="8" y="8" width="12" height="13" rx="2" /><path d="M16 8V3H3v13h5" /></>,
  chat: <path d="M21 11.5a8.4 8.4 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.4 8.4 0 0 1-3.8-.9L3 21l1.9-5.7a8.4 8.4 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.4 8.4 0 0 1 3.8-.9h.5a8.5 8.5 0 0 1 8 8Z" />,
  stop: <rect x="6" y="6" width="12" height="12" rx="2" />,
  trash: <><path d="M3 6h18M9 6V4h6v2M5 6l1 15h12l1-15M10 10v7M14 10v7" /></>,
  book: <><path d="M12 6C9 3 5 3 3 4v15c3-1 6-1 9 2 3-3 6-3 9-2V4c-2-1-6-1-9 2Zm0 0v15" /></>,
  database: <><ellipse cx="12" cy="5" rx="8" ry="3" /><path d="M4 5v14c0 4 16 4 16 0V5M4 12c0 4 16 4 16 0" /></>,
  file: <><path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9Zm0 0v6h6M8 13h8M8 17h5" /></>,
  grid: (
    <>
      <rect x="3" y="3" width="7" height="7" rx="2" />
      <rect x="14" y="3" width="7" height="7" rx="2" />
      <rect x="3" y="14" width="7" height="7" rx="2" />
      <rect x="14" y="14" width="7" height="7" rx="2" />
    </>
  ),
  building: (
    <>
      <rect x="5" y="3" width="14" height="18" rx="2" />
      <path d="M9 7h1m4 0h1m-6 4h1m4 0h1m-6 4h1m4 0h1M10 21v-3h4v3" />
    </>
  ),
  users: (
    <>
      <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2m20 0v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" />
      <circle cx="9" cy="7" r="4" />
    </>
  ),
  settings: (
    <>
      <path d="m10 3-.6 2.1-2 .9-2-.6L3.4 9l1.5 1.5v3L3.4 15l2 3.6 2-.6 2 .9.6 2.1h4l.6-2.1 2-.9 2 .6 2-3.6-1.5-1.5v-3L20.6 9l-2-3.6-2 .6-2-.9L14 3Z" />
      <circle cx="12" cy="12" r="3" />
    </>
  ),
  user: (
    <>
      <circle cx="12" cy="8" r="4" />
      <path d="M4 21v-2a8 8 0 0 1 16 0v2" />
    </>
  ),
  lock: (
    <>
      <rect x="4" y="10" width="16" height="11" rx="2" />
      <path d="M8 10V7a4 4 0 0 1 8 0v3m-4 5v2" />
    </>
  ),
  logout: (
    <>
      <path d="M9 5H5a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h4m5-14 7 7-7 7m7-7H9" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  search: (
    <>
      <circle cx="10.8" cy="10.8" r="7.3" />
      <path d="m16 16 4.5 4.5" />
    </>
  ),
  arrow: <path d="M4 12h16m-6-6 6 6-6 6" />,
  chevron: <path d="m9 5 7 7-7 7" />,
  shield: (
    <>
      <path d="m12 3 8 3v5c0 5-4 8-8 10-4-2-8-5-8-10V6Z" />
      <path d="m8 12 3 3 5-6" />
    </>
  ),
  check: <path d="m5 12 4 4L19 6" />,
  x: <path d="m6 6 12 12M6 18 18 6" />,
  alert: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v6m0 3v.1" />
    </>
  ),
  globe: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3a19 19 0 0 1 0 18 19 19 0 0 1 0-18" />
    </>
  ),
  spark: (
    <>
      <path d="m12 3 2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5Z" />
    </>
  ),
  refresh: (
    <>
      <path d="M20 7v5h-5M4 17v-5h5" />
      <path d="M5.2 7a8 8 0 0 1 13.2-2L20 7M4 17l1.6 2A8 8 0 0 0 19 17" />
    </>
  ),
}
export function Icon({ name, size = 20 }: { name: IconName; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.7"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[name]}
    </svg>
  )
}

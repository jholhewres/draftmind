import type { ReactNode } from 'react'
import { useI18n } from '../i18n'
import { tabLabel, tabs, type Tab } from './route'

const svg = (children: ReactNode) => (
  <svg aria-hidden="true" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    {children}
  </svg>
)

const icons: Record<Tab, ReactNode> = {
  draft: svg(
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 3v4M12 17v4M3 12h4M17 12h4" />
    </>,
  ),
  coach: svg(<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />),
  heroes: svg(
    <>
      <rect x="3" y="3" width="7" height="7" />
      <rect x="14" y="3" width="7" height="7" />
      <rect x="14" y="14" width="7" height="7" />
      <rect x="3" y="14" width="7" height="7" />
    </>,
  ),
  patches: svg(
    <>
      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
      <path d="M14 2v6h6M8 13h8M8 17h5" />
    </>,
  ),
}

// Bottom bar on a phone, left rail from 1024 px (see theme.css). current
// is null on a screen outside the tabs.
export default function TabBar({ current }: { current: Tab | null }) {
  const { t } = useI18n()
  return (
    <nav className="tabbar" aria-label={t('nav.label')}>
      {tabs.map((id) => (
        <a key={id} href={`#/${id}`} aria-current={id === current ? 'page' : undefined}>
          {icons[id]}
          {t(tabLabel[id])}
        </a>
      ))}
    </nav>
  )
}

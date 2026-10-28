import { useSyncExternalStore } from 'react'
import type { MessageKey } from '../locales/en'

export const tabs = ['draft', 'coach', 'heroes', 'patches'] as const
export type Tab = (typeof tabs)[number]

// Each tab's label, also its screen title.
export const tabLabel: Record<Tab, MessageKey> = {
  draft: 'nav.draft',
  coach: 'nav.coach',
  heroes: 'nav.heroes',
  patches: 'nav.patches',
}

const isTab = (id: string): id is Tab => (tabs as readonly string[]).includes(id)

// "#/heroes" names the Heroes tab; an empty or unknown hash opens Draft.
export function tabFromHash(hash: string): Tab {
  const id = hash.replace(/^#\/?/, '')
  return isTab(id) ? id : 'draft'
}

const subscribe = (onChange: () => void) => {
  window.addEventListener('hashchange', onChange)
  return () => window.removeEventListener('hashchange', onChange)
}

// The tab in the URL hash, updated on every hash change.
export const useTab = () => useSyncExternalStore(subscribe, () => tabFromHash(window.location.hash))

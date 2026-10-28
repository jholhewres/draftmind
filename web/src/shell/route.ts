import { useMemo, useSyncExternalStore } from 'react'
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

export type Route = { tab: Tab; hero: string | null }

// "#/heroes" names the Heroes tab and "#/heroes/<slug>" a hero in it; an
// empty or unknown hash opens Draft.
export function routeFromHash(hash: string): Route {
  const [id, slug] = hash.replace(/^#\/?/, '').split('/')
  if (!isTab(id)) return { tab: 'draft', hero: null }
  return { tab: id, hero: id === 'heroes' && slug ? decodeSlug(slug) : null }
}

// A malformed percent sequence opens the hero list rather than throwing.
function decodeSlug(slug: string): string | null {
  try {
    return decodeURIComponent(slug)
  } catch {
    return null
  }
}

export const tabFromHash = (hash: string): Tab => routeFromHash(hash).tab

const subscribe = (onChange: () => void) => {
  window.addEventListener('hashchange', onChange)
  return () => window.removeEventListener('hashchange', onChange)
}

const useHash = () => useSyncExternalStore(subscribe, () => window.location.hash)

// The route in the URL hash, updated on every hash change.
export function useRoute(): Route {
  const hash = useHash()
  return useMemo(() => routeFromHash(hash), [hash])
}

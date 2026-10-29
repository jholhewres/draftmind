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

// The sign-in screen: a link's token to spend, or Steam's failed return.
export type SignInRoute = { token: string | null; failed: boolean }

// landing is the root ("#/" or no hash): the landing page for a visitor,
// Draft for a signed-in player.
export type Route = { tab: Tab; hero: string | null; manual: boolean; signin?: SignInRoute; landing?: true }

// "#/heroes" names the Heroes tab and "#/heroes/<slug>" a hero in it;
// "#/draft/manual" is the draft board built by hand. "#/signin" is the
// sign-in screen, outside the tabs: "#/signin/link/<token>" an e-mailed
// link, "#/signin/failed" a Steam sign-in that did not finish. A query
// after "?" is the screen's own. An empty or unknown hash opens Draft.
export function routeFromHash(hash: string): Route {
  const [id, sub, arg] = hash.replace(/^#\/?/, '').split('?')[0].split('/')
  if (id === 'signin')
    return { tab: 'draft', hero: null, manual: false, signin: { token: sub === 'link' && arg ? arg : null, failed: sub === 'failed' } }
  if (id === '') return { tab: 'draft', hero: null, manual: false, landing: true }
  if (!isTab(id)) return { tab: 'draft', hero: null, manual: false }
  return { tab: id, hero: id === 'heroes' && sub ? decodeSlug(sub) : null, manual: id === 'draft' && sub === 'manual' }
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

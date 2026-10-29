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
// coach is a screen of the Coach tab past the match itself: its end, or the history.
export type CoachView = 'end' | 'history'

export type Route = { tab: Tab; hero: string | null; manual: boolean; signin?: SignInRoute; landing?: true; coach?: CoachView; settings?: true }

// "#/heroes" names the Heroes tab and "#/heroes/<slug>" a hero in it;
// "#/draft/manual" is the draft board built by hand; "#/coach/end" ends the
// match and "#/coach/history" lists the player's matches; "#/settings" is
// the coach persona. "#/signin" is the
// sign-in screen, outside the tabs: "#/signin/link/<token>" an e-mailed
// link, "#/signin/failed" a Steam sign-in that did not finish. A query
// after "?" is the screen's own. An empty hash is the root (landing); an
// unknown one opens Draft.
export function routeFromHash(hash: string): Route {
  const [id, sub, arg] = hash.replace(/^#\/?/, '').split('?')[0].split('/')
  if (id === 'signin')
    return { tab: 'draft', hero: null, manual: false, signin: { token: sub === 'link' && arg ? arg : null, failed: sub === 'failed' } }
  if (id === '') return { tab: 'draft', hero: null, manual: false, landing: true }
  // The coach settings sit under the Coach tab.
  if (id === 'settings') return { tab: 'coach', hero: null, manual: false, settings: true }
  if (!isTab(id)) return { tab: 'draft', hero: null, manual: false }
  const route: Route = { tab: id, hero: id === 'heroes' && sub ? decodeSlug(sub) : null, manual: id === 'draft' && sub === 'manual' }
  if (id === 'coach' && (sub === 'end' || sub === 'history')) route.coach = sub
  return route
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

// The source catalog: every key the UI uses, in English.
export const en = {
  'app.health.checking': 'API: checking…',
  'app.health.ok': 'API: {status}, database {database}',
  'app.health.unreachable': 'API: unreachable ({error})',
  'app.trademark': 'Not affiliated with or endorsed by Valve. Dota 2 is a Valve trademark. Never reads or changes the game client.',
  'screen.soon': 'Coming soon.',
  'nav.label': 'Main',
  'nav.draft': 'Draft',
  'nav.coach': 'Coach',
  'nav.heroes': 'Heroes',
  'nav.patches': 'Patches',
  'heroes.loadError': 'Could not load heroes: {error}',
  'heroes.loading': 'Loading heroes…',
  'heroes.search': 'Search heroes',
  'heroes.patch': 'Patch {patch}',
  'heroes.notSynced': 'Not synced yet',
  'heroes.attr.label': 'Filter by attribute',
  'heroes.attr.any': 'All',
  'heroes.attr.str': 'Strength',
  'heroes.attr.agi': 'Agility',
  'heroes.attr.int': 'Intelligence',
  'heroes.attr.all': 'Universal',
  'heroes.list': 'Heroes',
  'heroes.noMatch': 'No hero matches “{query}”.',
  'heroes.noMatchAttr': 'No {attr} hero matches “{query}”.',
  'heroes.noneAttr': 'No {attr} hero.',
}

export type MessageKey = keyof typeof en
export type Messages = Record<MessageKey, string>

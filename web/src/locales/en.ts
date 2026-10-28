// The source catalog: every key the UI uses, in English.
export const en = {
  'app.tagline': 'Draft help and an in-match coach, from a photo of the pick screen.',
  'app.health.checking': 'API: checking…',
  'app.health.ok': 'API: {status}, database {database}',
  'app.health.unreachable': 'API: unreachable ({error})',
  'app.trademark':
    'Draftmind is an independent tool, not affiliated with or endorsed by Valve Corporation. Dota 2 is a trademark of Valve. Draftmind runs on a separate device or tab and never reads, modifies or overlays the game client.',
  'heroes.loadError': 'Could not load heroes: {error}',
  'heroes.loading': 'Loading heroes…',
  'heroes.search': 'Search heroes',
  'heroes.patch': 'Patch {patch}',
  'heroes.notSynced': 'Not synced yet',
  'heroes.list': 'Heroes',
  'heroes.noMatch': 'No hero matches “{query}”.',
}

export type MessageKey = keyof typeof en
export type Messages = Record<MessageKey, string>

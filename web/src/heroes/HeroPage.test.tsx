import { render, screen, within } from '@testing-library/react'
import { vi } from 'vitest'
import type { HeroDetail } from '../api'
import I18nProvider from '../I18nProvider'
import HeroPage from './HeroPage'

const base = {
  id: 13,
  slug: 'puck',
  primaryAttr: 'int',
  attackType: 'Ranged',
  roles: ['Initiator', 'Escape'],
  img: '/apps/dota2/images/dota_react/heroes/puck.png',
  icon: '/apps/dota2/images/dota_react/heroes/icons/puck.png',
  patch: '7.41',
  complexity: 2,
  stats: {
    strBase: 17, strGain: 2.4, agiBase: 14, agiGain: 2.1, intBase: 23, intGain: 3.8, armor: 2.3333335,
    magicResistance: 25, damageMin: 45, damageMax: 53, attackRate: 1.7, attackRange: 550, movementSpeed: 290,
    maxHealth: 494, healthRegen: 2.2, maxMana: 351, manaRegen: 1.65,
  },
}

const english: HeroDetail = {
  ...base,
  name: 'Puck',
  lang: 'en',
  hype: 'Puck has a talent for confounding its enemies.',
  abilities: [
    { id: 5069, kind: 'ability', name: 'Illusory Orb', description: 'Puck launches a magic orb.', lore: '', cooldowns: [12, 11, 10, 9], manaCosts: [90, 100, 110, 120] },
    { id: 1222, kind: 'innate', name: 'Puckish', description: 'Whenever Puck disjoints an attack, it restores health.', lore: '', cooldowns: [], manaCosts: [] },
  ],
  talents: [{ id: 1057, level: 10, name: '+35 Illusory Orb Damage' }],
}

const portuguese: HeroDetail = {
  ...base,
  name: 'Puck',
  lang: 'pt-BR',
  hype: 'Puck tem um talento para confundir os inimigos.',
  abilities: [
    { id: 5069, kind: 'ability', name: 'Orbe Ilusório', description: 'Puck lança um orbe mágico.', lore: '', cooldowns: [12, 11, 10, 9], manaCosts: [90, 100, 110, 120] },
    { id: 1222, kind: 'innate', name: 'Puckz Grila', description: 'Sempre que Puck anular um ataque, restaura vida.', lore: '', cooldowns: [], manaCosts: [] },
  ],
  talents: [{ id: 1057, level: 10, name: '+35 de dano para Orbe Ilusório' }],
}

// Answers the hero in the language the request asks for.
function mockApi() {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const url = new URL(String(input), 'http://localhost')
    if (url.pathname !== '/api/heroes/puck') return new Response('{}', { status: 404 })
    const body = url.searchParams.get('lang') === 'pt-BR' ? portuguese : english
    return new Response(JSON.stringify(body), { status: 200 })
  })
}

afterEach(() => vi.restoreAllMocks())

test('the hero page renders the abilities in Portuguese when the UI is in pt-BR', async () => {
  vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['pt-BR', 'en-US'])
  const fetch = mockApi()
  render(
    <I18nProvider>
      <HeroPage slug="puck" />
    </I18nProvider>,
  )
  const abilities = await screen.findByRole('region', { name: 'Habilidades' })
  expect(fetch).toHaveBeenCalledWith('/api/heroes/puck?lang=pt-BR', expect.anything())
  expect(within(abilities).getByRole('heading', { name: 'Orbe Ilusório' })).toBeInTheDocument()
  expect(within(abilities).getByText('Puck lança um orbe mágico.')).toBeInTheDocument()
  expect(within(abilities).getByText('Recarga: 12 / 11 / 10 / 9')).toBeInTheDocument()
  expect(within(abilities).getByText('Inata')).toBeInTheDocument()
  expect(screen.getByText('Inteligência · À distância · Iniciador, Fuga')).toBeInTheDocument()
  expect(within(screen.getByRole('region', { name: 'Atributos' })).getByText('2,3')).toBeInTheDocument()
  expect(screen.getByText('+35 de dano para Orbe Ilusório')).toBeInTheDocument()
})

test('the hero page shows the banner, stats and abilities with cooldown and mana', async () => {
  mockApi()
  render(<HeroPage slug="puck" />)
  expect(await screen.findByRole('heading', { level: 1, name: 'Puck' })).toBeInTheDocument()
  expect(screen.getByText('Intelligence · Ranged · Initiator, Escape')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Back to heroes' })).toHaveAttribute('href', '#/heroes')

  const stats = screen.getByRole('region', { name: 'Stats' })
  const value = (term: string) => within(stats).getByText(term).nextElementSibling?.textContent
  expect(value('Health')).toBe('494')
  expect(value('Mana')).toBe('351')
  expect(value('Damage')).toBe('45–53')
  expect(value('Attack range')).toBe('550')
  expect(value('Move speed')).toBe('290')
  expect(value('Armor')).toBe('2.3')

  const abilities = screen.getByRole('region', { name: 'Abilities' })
  const orb = within(abilities).getByRole('heading', { name: 'Illusory Orb' }).closest('li')!
  expect(within(orb).getByText('Cooldown: 12 / 11 / 10 / 9')).toBeInTheDocument()
  expect(within(orb).getByText('Mana: 90 / 100 / 110 / 120')).toBeInTheDocument()
  const puckish = within(abilities).getByRole('heading', { name: 'Puckish' }).closest('li')!
  expect(within(puckish).queryByText(/Cooldown/)).not.toBeInTheDocument()
})

test('an unknown hero shows a not-found message', async () => {
  mockApi()
  render(<HeroPage slug="nobody" />)
  expect(await screen.findByText('No hero “nobody” on the current patch.')).toBeInTheDocument()
})

test('a hero whose details are not synced yet says so', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ ...english, stats: null, abilities: [], talents: [] }), { status: 200 }),
  )
  render(<HeroPage slug="puck" />)
  expect(await screen.findByText('Stats and abilities are not synced yet.')).toBeInTheDocument()
})

import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { vi } from 'vitest'
import DraftBoard from './DraftBoard'
import I18nProvider from '../I18nProvider'
import { sseResponse } from '../test/sse'

const hero = (id: number, slug: string, name: string) => ({
  id,
  slug,
  name,
  primaryAttr: 'str',
  attackType: 'Melee',
  roles: [],
  img: `/${slug}.png`,
  icon: `/${slug}_i.png`,
})

const catalog = {
  patch: '7.41',
  heroes: [
    hero(1, 'antimage', 'Anti-Mage'),
    hero(2, 'axe', 'Axe'),
    hero(5, 'crystal_maiden', 'Crystal Maiden'),
    hero(7, 'earthshaker', 'Earthshaker'),
    hero(8, 'juggernaut', 'Juggernaut'),
    hero(9, 'mirana', 'Mirana'),
  ],
}

const suggestion = (heroId: number, score: number, enemies: { heroId: number; delta: number }[]) => ({
  heroId,
  score,
  counter: enemies.reduce((s, e) => s + e.delta, 0),
  winRate: 51,
  roleFit: 1,
  enemies: enemies.map((e) => ({ ...e, games: 100 })),
})

// Eight suggestions, as the API returns by default, in Jev's order; the board shows five.
const ranked = {
  patch: '7.41',
  source: 'jev',
  confidence: 0.4,
  suggestions: [
    suggestion(8, 9.04, [{ heroId: 2, delta: 3.06 }]),
    suggestion(1, 4, [{ heroId: 2, delta: -0.62 }]),
    suggestion(9, 3, [{ heroId: 2, delta: 0 }]),
    suggestion(5, 2, [{ heroId: 2, delta: 1 }]),
    suggestion(7, 1, [{ heroId: 2, delta: 1 }]),
    suggestion(3, 0.5, []),
    suggestion(4, 0.4, []),
    suggestion(6, 0.3, []),
  ],
}

type Asked = { allies: number[]; enemies: number[]; bans: number[]; role: number }

// Streams the code ranking, then result as the decision.
function mockApi(suggestStatus = 200, result: object = ranked) {
  const asked: Asked[] = []
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    if (String(input).startsWith('/api/draft/stream')) {
      asked.push(JSON.parse(String(init?.body)) as Asked)
      if (suggestStatus !== 200) return new Response('{"error":"down"}', { status: suggestStatus })
      return sseResponse([['ranking', { patch: ranked.patch, suggestions: ranked.suggestions }], ['decision', result], ['done']])
    }
    return new Response(JSON.stringify(catalog), { status: 200 })
  })
  return asked
}

afterEach(() => vi.restoreAllMocks())

const group = (name: string) => screen.getByRole('group', { name })

async function pick(side: 'Allies' | 'Enemies' | 'Bans', slotName: string, search: string, heroName: string) {
  fireEvent.click(within(group(side)).getAllByRole('button', { name: slotName })[0])
  const dialog = await screen.findByRole('dialog')
  fireEvent.change(within(dialog).getByRole('searchbox', { name: 'Search heroes' }), { target: { value: search } })
  fireEvent.click(within(dialog).getByRole('button', { name: heroName }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
}

test('tapping an empty enemy slot opens a searchable picker and fills the slot with the chosen hero', async () => {
  mockApi()
  render(<DraftBoard />)
  fireEvent.click((await within(await screen.findByRole('group', { name: 'Enemies' })).findAllByRole('button', { name: 'Add enemy' }))[0])
  const dialog = await screen.findByRole('dialog', { name: 'Choose an enemy' })
  expect(within(dialog).getAllByRole('button', { name: /^(Anti-Mage|Axe|Crystal Maiden|Earthshaker|Juggernaut|Mirana)$/ })).toHaveLength(6)
  fireEvent.change(within(dialog).getByRole('searchbox', { name: 'Search heroes' }), { target: { value: 'AX' } })
  expect(within(dialog).getAllByRole('button', { name: /^(Anti-Mage|Axe|Crystal Maiden|Earthshaker|Juggernaut|Mirana)$/ })).toHaveLength(1)
  fireEvent.click(within(dialog).getByRole('button', { name: 'Axe' }))

  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(within(group('Enemies')).getByRole('button', { name: 'Enemy: Axe' })).toBeInTheDocument()
  expect(within(group('Enemies')).getAllByRole('button', { name: 'Add enemy' })).toHaveLength(4)
})

test('the picker leaves out heroes already in the draft, and a filled slot can be cleared', async () => {
  mockApi()
  render(<DraftBoard />)
  await screen.findByRole('group', { name: 'Enemies' })
  await pick('Enemies', 'Add enemy', 'axe', 'Axe')
  fireEvent.click(within(group('Allies')).getAllByRole('button', { name: 'Add ally' })[0])
  const dialog = await screen.findByRole('dialog', { name: 'Choose an ally' })
  expect(within(dialog).queryByRole('button', { name: 'Axe' })).not.toBeInTheDocument()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Close' }))

  fireEvent.click(within(group('Enemies')).getByRole('button', { name: 'Enemy: Axe' }))
  fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Remove' }))
  expect(within(group('Enemies')).getAllByRole('button', { name: 'Add enemy' })).toHaveLength(5)
})

test('the draft and the position go to /api/draft/stream, and the top 5 show a counter chip per enemy and the score', async () => {
  const asked = mockApi()
  render(<DraftBoard />)
  await screen.findByRole('group', { name: 'Enemies' })
  await pick('Enemies', 'Add enemy', 'axe', 'Axe')
  await pick('Allies', 'Add ally', 'crys', 'Crystal Maiden')
  await pick('Bans', 'Add ban', 'earth', 'Earthshaker')
  fireEvent.click(screen.getByRole('radio', { name: 'Position 3: Offlane' }))

  await waitFor(() => expect(asked.at(-1)).toEqual({ allies: [5], enemies: [2], bans: [7], role: 3 }))
  const list = await screen.findByRole('list', { name: 'Suggestions' })
  const items = within(list).getAllByRole('listitem')
  expect(items).toHaveLength(5)
  expect(within(items[0]).getByText('Juggernaut')).toBeInTheDocument()
  expect(within(items[0]).getByText('▲ Axe +3.1')).toBeInTheDocument()
  expect(within(items[0]).getByText('9.0')).toBeInTheDocument()
  expect(within(items[1]).getByText('▼ Axe −0.6')).toBeInTheDocument()
  expect(within(items[2]).getByText('Axe 0.0')).toBeInTheDocument()
})

test('the position starts at 1, Carry, and a new position asks again', async () => {
  const asked = mockApi()
  render(<DraftBoard />)
  await waitFor(() => expect(asked.at(-1)?.role).toBe(1))
  expect(screen.getByRole('radio', { name: 'Position 1: Carry' })).toBeChecked()
  fireEvent.click(screen.getByRole('radio', { name: 'Position 5: Hard support' }))
  await waitFor(() => expect(asked.at(-1)?.role).toBe(5))
})

test('each position button shows its number above the role name', async () => {
  mockApi()
  render(<DraftBoard />)
  const options = (await screen.findByRole('group', { name: 'Your position' })).querySelectorAll('.role-option')
  expect([...options].map((o) => [o.querySelector('.role-num')?.textContent, o.querySelector('.role-name')?.textContent])).toEqual([
    ['1', 'Carry'],
    ['2', 'Mid'],
    ['3', 'Offlane'],
    ['4', 'Support'],
    ['5', 'Hard support'],
  ])
})

test('a failed ranking shows an error', async () => {
  mockApi(500)
  render(<DraftBoard />)
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not rank the heroes')
})

test('with the browser in pt-BR, the board is in Portuguese and deltas use a decimal comma', async () => {
  vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['pt-BR'])
  mockApi()
  render(
    <I18nProvider>
      <DraftBoard />
    </I18nProvider>,
  )
  await screen.findByRole('group', { name: 'Inimigos' })
  fireEvent.click(within(group('Inimigos')).getAllByRole('button', { name: 'Adicionar inimigo' })[0])
  fireEvent.click(within(await screen.findByRole('dialog', { name: 'Escolha um inimigo' })).getByRole('button', { name: 'Axe' }))
  const list = await screen.findByRole('list', { name: 'Sugestões' })
  expect(within(list).getByText('▲ Axe +3,1')).toBeInTheDocument()
  expect(within(list).getByText('▼ Axe −0,6')).toBeInTheDocument()
  expect(screen.getByRole('group', { name: 'Sua posição' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: 'Posição 4: Suporte' })).toBeInTheDocument()
  expect(screen.getByRole('radio', { name: 'Posição 5: Hard suporte' })).toBeInTheDocument()
})

test("the code ranking kept when Jev is late carries a small label, and Jev's order carries none", async () => {
  mockApi(200, { ...ranked, source: 'ranking', reason: 'timeout', confidence: undefined })
  const { unmount } = render(<DraftBoard />)
  const label = await screen.findByText('Ranked from stats')
  expect(label).toHaveAttribute('title', 'The decision model did not answer in time.')
  expect(within(screen.getByRole('region', { name: 'Suggestions' })).getByText('Ranked from stats')).toBe(label)
  unmount()
  vi.restoreAllMocks()

  mockApi()
  const second = render(<DraftBoard />)
  await screen.findByRole('list', { name: 'Suggestions' })
  await waitFor(() => expect(screen.queryByText('Refining…')).not.toBeInTheDocument())
  expect(screen.queryByText('Ranked from stats')).not.toBeInTheDocument()
  second.unmount()
  vi.restoreAllMocks()

  // Below Jev's confidence floor, the code order stands with its own reason.
  mockApi(200, { ...ranked, source: 'ranking', reason: 'low_confidence', confidence: 0.3 })
  render(<DraftBoard />)
  expect(await screen.findByText('Ranked from stats')).toHaveAttribute('title', 'The decision model was not sure enough, so the stats order stands.')
})

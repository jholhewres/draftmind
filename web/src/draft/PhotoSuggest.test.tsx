import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { vi } from 'vitest'
import PhotoSuggest from './PhotoSuggest'

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
    hero(14, 'pudge', 'Pudge'),
  ],
}

const slot = (heroId?: number) => (heroId ? { heroId, status: 'hero', confidence: 0.9 } : { status: 'empty', confidence: 0.9 })
const unread = { status: 'unknown', confidence: 0.3 }

// Radiant: Anti-Mage, an empty slot (the player's), Crystal Maiden; dire:
// Axe, Pudge, one slot the reader could not read, two still empty.
const read = {
  patch: '7.41',
  reader: 'jev',
  radiant: [slot(1), slot(), slot(5), slot(), slot()],
  dire: [slot(2), slot(14), unread, slot(), slot()],
}

const suggestion = (heroId: number, score: number) => ({ heroId, score, counter: 0, winRate: 51, roleFit: 1, enemies: [] })

type Asked = { allies: number[]; enemies: number[]; bans: number[]; role: number }

// Answers the API; each suggest answer lists ranks in order, one list per call.
function mockApi(opts: { read?: object; readStatus?: number; ranks?: number[][] } = {}) {
  const asked: Asked[] = []
  const uploads: FormData[] = []
  const ranks = opts.ranks ?? [[8, 9, 7]]
  let gate: (() => void) | null = null
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = String(input)
    if (url === '/api/draft/read') {
      uploads.push(init?.body as FormData)
      return new Response(JSON.stringify(opts.read ?? read), { status: opts.readStatus ?? 200 })
    }
    if (url === '/api/draft/suggest') {
      asked.push(JSON.parse(String(init?.body)) as Asked)
      const ids = ranks[Math.min(asked.length - 1, ranks.length - 1)]
      if (asked.length > 1) await new Promise<void>((resolve) => (gate = resolve)) // a re-rank takes a moment
      return new Response(JSON.stringify({ patch: '7.41', source: 'jev', suggestions: ids.map((id, i) => suggestion(id, 9 - i)) }))
    }
    return new Response(JSON.stringify(catalog))
  })
  return { asked, uploads, release: () => gate?.() }
}

afterEach(() => vi.restoreAllMocks())

const photo = new File([new Uint8Array([0xff, 0xd8, 0xff])], 'IMG_0001.jpg', { type: 'image/jpeg' })
const prepared = new Blob(['downscaled'], { type: 'image/jpeg' })

function renderScreen() {
  const prepare = vi.fn(async () => prepared)
  render(<PhotoSuggest photo={photo} prepare={prepare} onNewPhoto={() => {}} />)
  return prepare
}

const group = (name: string) => screen.getByRole('group', { name })
const names = () => within(screen.getByRole('list', { name: 'Suggestions' })).getAllByRole('listitem').map((li) => li.querySelector('.suggestion-name')?.textContent)

test('the read draft shows as your team and the enemy, and the ranking renders as soon as the read returns', async () => {
  const { asked } = mockApi()
  renderScreen()
  expect(await screen.findByRole('list', { name: 'Suggestions' })).toBeInTheDocument()
  expect(names()).toEqual(['Juggernaut', 'Mirana', 'Earthshaker'])
  expect(asked).toEqual([{ allies: [1, 5], enemies: [2, 14], bans: [], role: 1 }])

  const team = group('Your team')
  expect(within(team).getByText('You')).toBeInTheDocument()
  expect(within(team).getByRole('button', { name: 'Ally: Anti-Mage' })).toBeInTheDocument()
  expect(within(team).getByRole('button', { name: 'Ally: Crystal Maiden' })).toBeInTheDocument()
  expect(within(team).getAllByRole('button', { name: 'Add ally' })).toHaveLength(2)
  const enemy = group('Enemy')
  expect(within(enemy).getByRole('button', { name: 'Enemy: Axe' })).toBeInTheDocument()
  expect(within(enemy).getByRole('button', { name: 'Enemy: Pudge' })).toBeInTheDocument()
})

test('an unread slot is flagged with a + and left out of the ranking, never guessed', async () => {
  const { asked } = mockApi()
  renderScreen()
  const flagged = await within(await screen.findByRole('group', { name: 'Enemy' })).findByRole('button', { name: 'Unread hero, choose' })
  expect(flagged).toHaveClass('flagged')
  expect(flagged.querySelector('img')).toBeNull()
  expect(screen.getByText('1 hero was not read. Tap + to choose it; the ranking already uses the others.')).toBeInTheDocument()
  await waitFor(() => expect(asked).toHaveLength(1))
  expect(asked[0].enemies).toEqual([2, 14])
})

test('fixing an unread slot re-ranks in place: the old list stays until the new one arrives', async () => {
  const { asked, release } = mockApi({ ranks: [[8, 9, 7], [7, 8, 9]] })
  renderScreen()
  await screen.findByRole('list', { name: 'Suggestions' })
  fireEvent.click(within(group('Enemy')).getByRole('button', { name: 'Unread hero, choose' }))
  const dialog = await screen.findByRole('dialog', { name: 'Choose an enemy' })
  expect(within(dialog).queryByRole('button', { name: 'Axe' })).not.toBeInTheDocument() // already in the draft
  fireEvent.click(within(dialog).getByRole('button', { name: 'Earthshaker' }))

  await waitFor(() => expect(asked).toHaveLength(2))
  expect(asked[1].enemies).toEqual([2, 14, 7])
  expect(within(group('Enemy')).getByRole('button', { name: 'Enemy: Earthshaker' })).toBeInTheDocument()
  expect(screen.queryByText(/not read/)).not.toBeInTheDocument()
  expect(names()).toEqual(['Juggernaut', 'Mirana', 'Earthshaker']) // still the old list
  expect(screen.queryByRole('status')).not.toBeInTheDocument()
  release()
  await waitFor(() => expect(names()).toEqual(['Earthshaker', 'Juggernaut', 'Mirana']))
})

test('the photo is uploaded once, as the downscaled copy, never the original file', async () => {
  const { uploads } = mockApi()
  const prepare = renderScreen()
  await screen.findByRole('list', { name: 'Suggestions' })
  expect(prepare).toHaveBeenCalledWith(photo)
  expect(uploads).toHaveLength(1)
  const sent = uploads[0].get('image') as File
  expect(sent.size).toBe(prepared.size)
  expect(sent.name).not.toContain('IMG_0001')
})

test('swapping teams makes the other side your team', async () => {
  const { asked } = mockApi()
  renderScreen()
  await screen.findByRole('list', { name: 'Suggestions' })
  fireEvent.click(screen.getByRole('button', { name: 'Swap teams' }))
  await waitFor(() => expect(asked.at(-1)).toEqual({ allies: [2, 14], enemies: [1, 5], bans: [], role: 1 }))
})

test('a photo that cannot be read says so and offers the board by hand', async () => {
  mockApi({ readStatus: 502 })
  renderScreen()
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not read the photo')
  expect(screen.getByRole('link', { name: 'Build by hand' })).toHaveAttribute('href', '#/draft/manual')
})

test('an unread seat on your team is yours when no slot is empty, so every read ally stays in the ranking', async () => {
  const { asked } = mockApi({
    read: { patch: '7.41', reader: 'jev', radiant: [unread, slot(1), slot(5), slot(7), slot(14)], dire: [slot(2), slot(), slot(), slot(), slot()] },
  })
  renderScreen()
  await waitFor(() => expect(asked).toHaveLength(1))
  expect(asked[0].allies).toEqual([1, 5, 7, 14])
  expect(within(group('Your team')).getByText('You')).toBeInTheDocument()
  expect(within(group('Your team')).getAllByRole('button')).toHaveLength(4)
})

test('a flagged enemy slot can be set to empty from the picker', async () => {
  mockApi()
  renderScreen()
  fireEvent.click(await within(await screen.findByRole('group', { name: 'Enemy' })).findByRole('button', { name: 'Unread hero, choose' }))
  fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Remove' }))
  await waitFor(() => expect(within(group('Enemy')).getAllByRole('button', { name: 'Add enemy' })).toHaveLength(3))
  expect(screen.queryByText(/not read/)).not.toBeInTheDocument()
})

test('two unread slots read in the plural', async () => {
  mockApi({ read: { ...read, radiant: [slot(1), slot(), unread, slot(), slot()] } })
  renderScreen()
  expect(await screen.findByText('2 heroes were not read. Tap + to choose them; the ranking already uses the others.')).toBeInTheDocument()
})

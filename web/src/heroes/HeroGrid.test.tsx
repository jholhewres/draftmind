import { fireEvent, render, screen, within } from '@testing-library/react'
import { vi } from 'vitest'
import HeroGrid from './HeroGrid'
import I18nProvider from '../I18nProvider'

const payload = {
  patch: '7.41',
  heroes: [
    { id: 1, slug: 'antimage', name: 'Anti-Mage', primaryAttr: 'agi', attackType: 'Melee', roles: ['Carry'], img: '/am.png', icon: '/am_i.png' },
    { id: 2, slug: 'axe', name: 'Axe', primaryAttr: 'str', attackType: 'Melee', roles: ['Initiator'], img: '/axe.png', icon: '/axe_i.png' },
    { id: 5, slug: 'crystal_maiden', name: 'Crystal Maiden', primaryAttr: 'int', attackType: 'Ranged', roles: ['Support'], img: '/cm.png', icon: '/cm_i.png' },
    { id: 7, slug: 'earthshaker', name: 'Earthshaker', primaryAttr: 'str', attackType: 'Melee', roles: ['Initiator'], img: '/es.png', icon: '/es_i.png' },
    { id: 3, slug: 'bane', name: 'Bane', primaryAttr: 'all', attackType: 'Ranged', roles: ['Support'], img: '/bane.png', icon: '/bane_i.png' },
  ],
}

afterEach(() => vi.restoreAllMocks())

test('the hero grid filters by name', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
  render(<HeroGrid />)
  const list = await screen.findByRole('list', { name: /heroes/i })
  expect(within(list).getAllByRole('listitem')).toHaveLength(5)
  expect(screen.getByText(/patch 7\.41/i)).toBeInTheDocument()

  fireEvent.change(screen.getByRole('searchbox', { name: /search heroes/i }), { target: { value: 'CRYS' } })

  expect(within(list).getAllByRole('listitem')).toHaveLength(1)
  expect(within(list).getByText('Crystal Maiden')).toBeInTheDocument()
})

const names = (list: HTMLElement) => within(list).getAllByRole('listitem').map((li) => li.textContent)

test("each hero shows its portrait from its image path on the CDN, under the Heroes heading", async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
  render(<HeroGrid />)
  const list = await screen.findByRole('list', { name: /heroes/i })
  expect(screen.getByRole('heading', { level: 1, name: 'Heroes' })).toBeInTheDocument()
  const srcs = within(list).getAllByRole('listitem').map((li) => li.querySelector('img')?.getAttribute('src'))
  expect(srcs).toEqual([
    'https://cdn.cloudflare.steamstatic.com/am.png',
    'https://cdn.cloudflare.steamstatic.com/axe.png',
    'https://cdn.cloudflare.steamstatic.com/cm.png',
    'https://cdn.cloudflare.steamstatic.com/es.png',
    'https://cdn.cloudflare.steamstatic.com/bane.png',
  ])
})

test('the attribute filter keeps the heroes of the pressed primary attribute', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
  render(<HeroGrid />)
  const list = await screen.findByRole('list', { name: /heroes/i })
  const group = screen.getByRole('group', { name: 'Filter by attribute' })
  expect(within(group).getByRole('button', { name: 'All' })).toHaveAttribute('aria-pressed', 'true')

  fireEvent.click(within(group).getByRole('button', { name: 'Strength' }))
  expect(names(list)).toEqual(['Axe', 'Earthshaker'])
  expect(within(group).getByRole('button', { name: 'Strength' })).toHaveAttribute('aria-pressed', 'true')
  expect(within(group).getByRole('button', { name: 'All' })).toHaveAttribute('aria-pressed', 'false')

  fireEvent.click(within(group).getByRole('button', { name: 'Agility' }))
  expect(names(list)).toEqual(['Anti-Mage'])
  fireEvent.click(within(group).getByRole('button', { name: 'Intelligence' }))
  expect(names(list)).toEqual(['Crystal Maiden'])
  fireEvent.click(within(group).getByRole('button', { name: 'Universal' }))
  expect(names(list)).toEqual(['Bane'])

  // Search narrows within the attribute.
  fireEvent.click(within(group).getByRole('button', { name: 'Strength' }))
  fireEvent.change(screen.getByRole('searchbox', { name: /search heroes/i }), { target: { value: 'earth' } })
  expect(names(list)).toEqual(['Earthshaker'])

  fireEvent.click(within(group).getByRole('button', { name: 'All' }))
  fireEvent.change(screen.getByRole('searchbox', { name: /search heroes/i }), { target: { value: '' } })
  expect(names(list)).toHaveLength(5)
})

test('the empty state names the pressed attribute', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
  render(<HeroGrid />)
  await screen.findByRole('list', { name: /heroes/i })
  fireEvent.click(screen.getByRole('button', { name: 'Agility' }))
  fireEvent.change(screen.getByRole('searchbox', { name: /search heroes/i }), { target: { value: 'axe' } })
  expect(screen.getByText('No Agility hero matches “axe”.')).toBeInTheDocument()
  fireEvent.change(screen.getByRole('searchbox', { name: /search heroes/i }), { target: { value: '' } })
  expect(screen.queryByText(/no .*hero/i)).not.toBeInTheDocument()
})

test('with an attribute that has no heroes and no search, the empty state says so', async () => {
  const onlyAxe = { patch: '7.41', heroes: payload.heroes.filter((h) => h.slug === 'axe') }
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(onlyAxe), { status: 200 }))
  render(<HeroGrid />)
  await screen.findByRole('list', { name: /heroes/i })
  fireEvent.click(screen.getByRole('button', { name: 'Intelligence' }))
  expect(screen.getByText('No Intelligence hero.')).toBeInTheDocument()
})

test('shows an empty state when no hero matches', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
  render(<HeroGrid />)
  await screen.findByRole('list', { name: /heroes/i })
  fireEvent.change(screen.getByRole('searchbox', { name: /search heroes/i }), { target: { value: 'zzz' } })
  expect(screen.getByText(/no hero matches/i)).toBeInTheDocument()
})

test('with the browser in pt-BR, the search label, patch label and empty state are in Portuguese', async () => {
  vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['pt-BR', 'en-US'])
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
  render(
    <I18nProvider>
      <HeroGrid />
    </I18nProvider>,
  )
  await screen.findByRole('list', { name: 'Heróis' })
  expect(screen.getByRole('group', { name: 'Filtrar por atributo' })).toBeInTheDocument()
  for (const name of ['Todos', 'Força', 'Agilidade', 'Inteligência', 'Universal']) {
    expect(screen.getByRole('button', { name })).toBeInTheDocument()
  }
  expect(screen.getByText('Patch 7.41')).toBeInTheDocument()
  fireEvent.change(screen.getByRole('searchbox', { name: 'Buscar heróis' }), { target: { value: 'zzz' } })
  expect(screen.getByText('Nenhum herói encontrado para “zzz”.')).toBeInTheDocument()
})

test('with the browser in pt-BR and no patch synced, the patch label is in Portuguese', async () => {
  vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['pt-BR'])
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ patch: '', heroes: [] }), { status: 200 }))
  render(
    <I18nProvider>
      <HeroGrid />
    </I18nProvider>,
  )
  expect(await screen.findByText('Ainda não sincronizado')).toBeInTheDocument()
})

test('each hero links to its page', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
  render(<HeroGrid />)
  const list = await screen.findByRole('list', { name: /heroes/i })
  expect(within(list).getByRole('link', { name: 'Crystal Maiden' })).toHaveAttribute('href', '#/heroes/crystal_maiden')
})

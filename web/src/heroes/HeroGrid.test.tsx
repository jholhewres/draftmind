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
  ],
}

afterEach(() => vi.restoreAllMocks())

test('the hero grid filters by name', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
  render(<HeroGrid />)
  const list = await screen.findByRole('list', { name: /heroes/i })
  expect(within(list).getAllByRole('listitem')).toHaveLength(3)
  expect(screen.getByText(/patch 7\.41/i)).toBeInTheDocument()

  fireEvent.change(screen.getByRole('searchbox', { name: /search heroes/i }), { target: { value: 'CRYS' } })

  expect(within(list).getAllByRole('listitem')).toHaveLength(1)
  expect(within(list).getByText('Crystal Maiden')).toBeInTheDocument()
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

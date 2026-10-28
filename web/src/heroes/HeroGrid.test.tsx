import { fireEvent, render, screen, within } from '@testing-library/react'
import { vi } from 'vitest'
import HeroGrid from './HeroGrid'

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

import { render, screen, within } from '@testing-library/react'
import TabBar from './TabBar'
import { routeFromHash, tabFromHash } from './route'

test('the tab bar links the four tabs and marks the current one with aria-current', () => {
  render(<TabBar current="heroes" />)
  const nav = screen.getByRole('navigation', { name: 'Main' })
  const links = within(nav).getAllByRole('link')
  expect(links.map((l) => l.textContent)).toEqual(['Draft', 'Coach', 'Heroes', 'Patches'])
  expect(links.map((l) => l.getAttribute('href'))).toEqual(['#/draft', '#/coach', '#/heroes', '#/patches'])
  expect(within(nav).getByRole('link', { name: 'Heroes' })).toHaveAttribute('aria-current', 'page')
  for (const name of ['Draft', 'Coach', 'Patches']) {
    expect(within(nav).getByRole('link', { name })).not.toHaveAttribute('aria-current')
  }
})

test('the hash names the tab, and an empty or unknown hash opens Draft', () => {
  expect(tabFromHash('#/coach')).toBe('coach')
  expect(tabFromHash('#/patches')).toBe('patches')
  expect(tabFromHash('')).toBe('draft')
  expect(tabFromHash('#/nope')).toBe('draft')
})

test('a hero hash opens that hero under the Heroes tab', () => {
  expect(routeFromHash('#/heroes/puck')).toEqual({ tab: 'heroes', hero: 'puck', manual: false })
  expect(routeFromHash('#/heroes')).toEqual({ tab: 'heroes', hero: null, manual: false })
  expect(routeFromHash('#/heroes/')).toEqual({ tab: 'heroes', hero: null, manual: false })
  expect(routeFromHash('#/draft/puck')).toEqual({ tab: 'draft', hero: null, manual: false })
})

test('#/draft/manual opens the draft board built by hand', () => {
  expect(routeFromHash('#/draft/manual')).toEqual({ tab: 'draft', hero: null, manual: true })
  expect(routeFromHash('#/heroes/manual')).toEqual({ tab: 'heroes', hero: 'manual', manual: false })
})

test('a malformed hero hash opens the hero list instead of throwing', () => {
  expect(routeFromHash('#/heroes/%E0%A4%A')).toEqual({ tab: 'heroes', hero: null, manual: false })
})

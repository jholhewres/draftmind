import { render, screen } from '@testing-library/react'
import { vi } from 'vitest'
import App from './App'

afterEach(() => vi.restoreAllMocks())

test('shows API health, the hero grid and the trademark note', async () => {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const body = String(input).endsWith('/api/heroes')
      ? { patch: '7.41', heroes: [] }
      : { status: 'ok', database: 'up' }
    return new Response(JSON.stringify(body), { status: 200 })
  })
  render(<App />)
  expect(await screen.findByText(/ok, database up/)).toBeInTheDocument()
  expect(await screen.findByRole('searchbox', { name: /search heroes/i })).toBeInTheDocument()
  expect(screen.getByText(/not affiliated with or endorsed by Valve/)).toBeInTheDocument()
})

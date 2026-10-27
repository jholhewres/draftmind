import { render, screen } from '@testing-library/react'
import { vi } from 'vitest'
import App from './App'

afterEach(() => vi.restoreAllMocks())

test('shows API health and the trademark note', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ status: 'ok', database: 'up' }), { status: 200 }),
  )
  render(<App />)
  expect(await screen.findByText(/ok, database up/)).toBeInTheDocument()
  expect(screen.getByText(/not affiliated with or endorsed by Valve/)).toBeInTheDocument()
})

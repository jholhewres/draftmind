import { render, screen } from '@testing-library/react'
import { vi } from 'vitest'
import App from './App'
import I18nProvider from './I18nProvider'

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

test('with the browser in pt-BR, the shell and the trademark note are in Portuguese', async () => {
  vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['pt-BR'])
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const body = String(input).endsWith('/api/heroes')
      ? { patch: '7.41', heroes: [] }
      : { status: 'ok', database: 'up' }
    return new Response(JSON.stringify(body), { status: 200 })
  })
  render(
    <I18nProvider>
      <App />
    </I18nProvider>,
  )
  expect(await screen.findByText('API: ok, banco de dados up')).toBeInTheDocument()
  expect(screen.getByText(/sem afiliação com a Valve Corporation/)).toBeInTheDocument()
  expect(screen.getByText(/foto da tela de picks/)).toBeInTheDocument()
  expect(document.documentElement.lang).toBe('pt-BR')
})

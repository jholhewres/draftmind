import { render, screen, within } from '@testing-library/react'
import { vi } from 'vitest'
import App from './App'
import I18nProvider from './I18nProvider'

beforeEach(() => {
  window.location.hash = '#/draft'
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    if (String(input).startsWith('/api/heroes/')) return new Response('{}', { status: 404 })
    const body = String(input).endsWith('/api/heroes') ? { patch: '7.41', heroes: [] } : { status: 'ok', database: 'up' }
    return new Response(JSON.stringify(body), { status: 200 })
  })
})
afterEach(() => vi.restoreAllMocks())

test('the footer shows API health under the trademark note', async () => {
  render(<App />)
  const footer = screen.getByRole('contentinfo')
  expect(await within(footer).findByText('API: ok, database up')).toBeInTheDocument()
  expect(within(footer).getByText(/Dota 2 is a Valve trademark/)).toBeInTheDocument()
})

test('with the browser in pt-BR, the footer is in Portuguese', async () => {
  vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['pt-BR'])
  render(
    <I18nProvider>
      <App />
    </I18nProvider>,
  )
  expect(await screen.findByText('API: ok, banco de dados up')).toBeInTheDocument()
  expect(screen.getByText(/Sem relação com a Valve/)).toBeInTheDocument()
  expect(document.documentElement.lang).toBe('pt-BR')
})

test('a hero hash opens the hero page under the Heroes tab', async () => {
  window.location.hash = '#/heroes/axe'
  render(<App />)
  expect(await screen.findByText('No hero “axe” on the current patch.')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Heroes' })).toHaveAttribute('aria-current', 'page')
})

import { render, screen, within } from '@testing-library/react'
import { vi } from 'vitest'
import App from './App'
import I18nProvider from './I18nProvider'

beforeEach(() => {
  window.location.hash = ''
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const body = String(input).endsWith('/api/heroes') ? { patch: '7.41', heroes: [] } : { status: 'ok', database: 'up' }
    return new Response(JSON.stringify(body), { status: 200 })
  })
})
afterEach(() => vi.restoreAllMocks())

test('the footer shows API health under the trademark note', async () => {
  render(<App />)
  const footer = screen.getByRole('contentinfo')
  expect(await within(footer).findByText('API: ok, database up')).toBeInTheDocument()
  expect(within(footer).getByText(/Not affiliated with Valve/)).toBeInTheDocument()
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

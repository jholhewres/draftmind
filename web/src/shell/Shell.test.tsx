import { fireEvent, render, screen, within } from '@testing-library/react'
import { vi } from 'vitest'
import App from '../App'
import I18nProvider from '../I18nProvider'

const catalog = {
  patch: '7.41',
  heroes: [{ id: 2, slug: 'axe', name: 'Axe', primaryAttr: 'str', attackType: 'Melee', roles: [], img: '/axe.png', icon: '/axe_i.png' }],
}

beforeEach(() => {
  window.location.hash = ''
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const body = String(input).endsWith('/api/heroes') ? catalog : { status: 'ok', database: 'up' }
    return new Response(JSON.stringify(body), { status: 200 })
  })
})
afterEach(() => vi.restoreAllMocks())

const tabBar = () => screen.getByRole('navigation', { name: 'Main' })
const current = () => within(tabBar()).getByRole('link', { current: 'page' })

test('the tab bar switches between Draft, Coach, Heroes and Patches and marks the current tab', async () => {
  render(<App />)
  expect(screen.getByRole('heading', { level: 1, name: 'Photograph the pick' })).toBeInTheDocument()
  expect(current()).toHaveTextContent('Draft')

  fireEvent.click(within(tabBar()).getByRole('link', { name: 'Coach' }))
  expect(await screen.findByRole('heading', { level: 1, name: 'Coach' })).toBeInTheDocument()
  expect(current()).toHaveTextContent('Coach')
  expect(screen.getByText('Coming soon.')).toBeInTheDocument()

  fireEvent.click(within(tabBar()).getByRole('link', { name: 'Heroes' }))
  expect(await screen.findByRole('list', { name: 'Heroes' })).toBeInTheDocument()
  expect(current()).toHaveTextContent('Heroes')

  fireEvent.click(within(tabBar()).getByRole('link', { name: 'Patches' }))
  expect(await screen.findByRole('heading', { level: 1, name: 'Patches' })).toBeInTheDocument()
  expect(current()).toHaveTextContent('Patches')
})

test('the trademark and fair-play note renders on every main tab', async () => {
  render(<App />)
  for (const tab of ['Draft', 'Coach', 'Heroes', 'Patches']) {
    fireEvent.click(within(tabBar()).getByRole('link', { name: tab }))
    await screen.findByRole('link', { name: tab, current: 'page' })
    expect(within(screen.getByRole('contentinfo')).getByText('Not affiliated with or endorsed by Valve. Dota 2 is a Valve trademark. Never reads or changes the game client.')).toBeInTheDocument()
  }
})

test('with the browser in pt-BR, the tabs, placeholder and note are in Portuguese', async () => {
  vi.spyOn(navigator, 'languages', 'get').mockReturnValue(['pt-BR'])
  render(
    <I18nProvider>
      <App />
    </I18nProvider>,
  )
  const nav = screen.getByRole('navigation', { name: 'Principal' })
  fireEvent.click(within(nav).getByRole('link', { name: 'Heróis' }))
  expect(await screen.findByRole('heading', { level: 1, name: 'Heróis' })).toBeInTheDocument()
  fireEvent.click(within(nav).getByRole('link', { name: 'Patches' }))
  expect(await screen.findByText('Em breve.')).toBeInTheDocument()
  expect(screen.getByText('Sem relação com a Valve nem com o jogo, e sem endosso da Valve. Dota 2 é marca registrada da Valve. Não lê nem altera o cliente do jogo.')).toBeInTheDocument()
})

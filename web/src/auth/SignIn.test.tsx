import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { vi } from 'vitest'
import SignIn from './SignIn'
import { trackHistory } from '../shell/nav'

beforeEach(() => {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
    const url = String(input)
    if (url.endsWith('/api/auth/methods')) return Response.json({ emailLink: true })
    if (url.endsWith('/api/auth/email/verify')) return Response.json({ email: 'a@b.c' })
    return new Response('{}', { status: 404 })
  })
})
afterEach(() => vi.restoreAllMocks())

test('after an e-mailed link signs in, back leaves the sign-in screen instead of reopening the link', async () => {
  // The link opened in a new tab: the app starts on it, with no history of its own.
  history.replaceState(null, '', '#/signin/link/tok123')
  trackHistory()
  const onAccount = vi.fn()
  render(<SignIn token="tok123" failed={false} account={null} onAccount={onAccount} />)
  await waitFor(() => expect(onAccount).toHaveBeenCalledWith({ email: 'a@b.c' }))
  // The spent token is replaced in the address bar, not pushed after it.
  await waitFor(() => expect(window.location.hash).toBe('#/signin'))
  const back = vi.spyOn(history, 'back')
  await act(async () => fireEvent.click(screen.getByRole('link', { name: 'Back' })))
  expect(back).not.toHaveBeenCalled()
  await waitFor(() => expect(window.location.hash).toBe('#/'))
  expect(vi.mocked(fetch).mock.calls.filter(([u]) => String(u).endsWith('/email/verify'))).toHaveLength(1)
})

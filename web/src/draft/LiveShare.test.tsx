import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { vi } from 'vitest'
import LiveDraft from './LiveShare'
import PhotoCapture from './PhotoCapture'

// A shared window: one video track that can end, and a stop on every track.
function fakeStream(surface = 'window') {
  const track = Object.assign(new EventTarget(), {
    label: 'Game',
    stop: vi.fn(),
    getSettings: () => ({ displaySurface: surface }),
  })
  return { track, stream: { getTracks: () => [track], getVideoTracks: () => [track] } as unknown as MediaStream }
}

function shareWith(getDisplayMedia: () => Promise<MediaStream>) {
  Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: { getDisplayMedia } })
}

beforeEach(() => vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue())
afterEach(() => vi.restoreAllMocks())

test('a refused share says nothing, and a whole screen is stopped and refused', async () => {
  const onLive = vi.fn()
  shareWith(() => Promise.reject(new DOMException('denied', 'NotAllowedError')))
  const { unmount } = render(<PhotoCapture onPhoto={vi.fn()} onLive={onLive} />)
  fireEvent.click(screen.getByRole('button', { name: /Live from screen/ }))
  await act(async () => {})
  expect(screen.queryByRole('alert')).toBeNull()
  unmount()

  const { track, stream } = fakeStream('monitor')
  shareWith(() => Promise.resolve(stream))
  render(<PhotoCapture onPhoto={vi.fn()} onLive={onLive} />)
  fireEvent.click(screen.getByRole('button', { name: /Live from screen/ }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Share the game window, not a whole screen or a tab.')
  expect(track.stop).toHaveBeenCalled()
  expect(onLive).not.toHaveBeenCalled()
})

test('Stop ends the share, and so does the browser ending the window share', async () => {
  const one = fakeStream()
  const onStop = vi.fn()
  render(<LiveDraft stream={one.stream} onStop={onStop} />)
  expect(screen.getByRole('status')).toHaveTextContent('Watching the game window.')
  fireEvent.click(screen.getByRole('button', { name: 'Stop sharing' }))
  expect(one.track.stop).toHaveBeenCalled()
  expect(onStop).toHaveBeenCalledTimes(1)

  const two = fakeStream()
  const ended = vi.fn()
  const { unmount } = render(<LiveDraft stream={two.stream} onStop={ended} />)
  act(() => void two.track.dispatchEvent(new Event('ended')))
  expect(ended).toHaveBeenCalledTimes(1)
  unmount()
  await waitFor(() => expect(two.track.stop).toHaveBeenCalled())
})

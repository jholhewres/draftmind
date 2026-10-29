// Screen sharing on a desktop browser; a phone or tablet takes a photo.
export const canShareScreen = () =>
  typeof navigator !== 'undefined' &&
  typeof navigator.mediaDevices?.getDisplayMedia === 'function' &&
  !window.matchMedia?.('(pointer: coarse)').matches

export class NotAWindow extends Error {}

// Asks the player to share one window, the game's; a whole screen or a tab
// is refused, so nothing else on the desktop is ever seen.
export async function shareWindow(): Promise<MediaStream> {
  const stream = await navigator.mediaDevices.getDisplayMedia({
    video: { displaySurface: 'window', frameRate: 1 },
    audio: false,
    selfBrowserSurface: 'exclude',
    monitorTypeSurfaces: 'exclude',
  } as DisplayMediaStreamOptions)
  const surface = stream.getVideoTracks()[0]?.getSettings().displaySurface
  if (surface && surface !== 'window') {
    stream.getTracks().forEach((t) => t.stop())
    throw new NotAWindow()
  }
  return stream
}

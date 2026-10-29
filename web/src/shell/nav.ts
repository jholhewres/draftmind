// In-app history: each entry the app made carries its depth in
// history.state, so a back control knows whether the previous entry is
// still this app (go back) or the tab was opened on a deep link (go to
// the screen's parent instead).
type NavState = { dmDepth?: number; dmModal?: true }

const depthOf = (s: unknown): number | undefined => {
  const d = (s as NavState | null)?.dmDepth
  return typeof d === 'number' ? d : undefined
}

let depth = 0

// Called once at start-up, before the app renders.
export function trackHistory() {
  const kept = depthOf(history.state)
  depth = kept ?? 0
  if (kept === undefined) history.replaceState({ ...(history.state as object), dmDepth: 0 }, '')
  window.addEventListener('popstate', () => {
    const d = depthOf(history.state)
    if (d !== undefined) depth = d
  })
  window.addEventListener('hashchange', () => {
    const d = depthOf(history.state)
    // A new entry (a link, location.hash) has no depth yet.
    if (d === undefined) {
      depth += 1
      history.replaceState({ ...(history.state as object), dmDepth: depth }, '')
    } else depth = d
  })
}

// Whether the entry before this one is a screen of this app.
export const canGoBack = () => depth > 0

// Back to the previous screen, or to parent when there is none in the app.
export function goBack(parent: string) {
  if (canGoBack()) history.back()
  else location.replace(parent)
}

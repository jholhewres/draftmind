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
// Set while replaceHash swaps the current entry, which keeps its depth.
let replacing = false

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
    // A new entry (a link, location.hash) has no depth yet; a replaced one
    // keeps the depth of the entry it took over.
    if (d === undefined) {
      if (!replacing) depth += 1
      replacing = false
      history.replaceState({ ...(history.state as object), dmDepth: depth }, '')
    } else depth = d
  })
}

// Swaps the current entry for hash, as location.replace does, keeping its depth.
export function replaceHash(hash: string) {
  if (location.hash === hash) return
  replacing = true
  location.replace(hash)
}

// Back to the previous screen, or to parent when there is none in the app.
export function goBack(parent: string) {
  if (depth > 0) history.back()
  else replaceHash(parent)
}

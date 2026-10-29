import { useEffect, useState } from 'react'
import { fetchMyMode, gameModes, type GameMode } from '../api'

const KEY = 'draftmind.mode'

function stored(): GameMode {
  try {
    const m = window.localStorage.getItem(KEY)
    if (m && (gameModes as readonly string[]).includes(m)) return m as GameMode
  } catch {
    // storage off: the default stands
  }
  return 'all_pick_ranked'
}

// The game mode of the next draft: the signed-in player's last one, else this
// browser's, else ranked All Pick. Picking one keeps it in this browser; the
// server keeps it for a signed-in player once a match starts.
export function useGameMode(): [GameMode, (m: GameMode) => void] {
  const [mode, setMode] = useState<GameMode>(stored)
  useEffect(() => {
    const ctrl = new AbortController()
    fetchMyMode(ctrl.signal)
      .then((m) => {
        if (m) setMode(m)
      })
      .catch(() => {}) // a visitor or no settings: the stored mode stands
    return () => ctrl.abort()
  }, [])
  const pick = (m: GameMode) => {
    setMode(m)
    try {
      window.localStorage.setItem(KEY, m)
    } catch {
      // storage off: kept for this page only
    }
  }
  return [mode, pick]
}

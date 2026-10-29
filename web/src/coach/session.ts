// The match session id and its timer live in localStorage, so a reload
// restores the match. Storage may be blocked; the app then forgets on reload.
const sessionKey = 'draftmind.match'
const timerKey = (id: string) => `draftmind.timer.${id}`

const read = (key: string): string | null => {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

const write = (key: string, value: string | null) => {
  try {
    if (value === null) window.localStorage.removeItem(key)
    else window.localStorage.setItem(key, value)
  } catch {
    // the session lasts until the page reloads
  }
}

export const savedSessionId = () => read(sessionKey)

export const saveSessionId = (id: string | null) => write(sessionKey, id)

// A manual match timer: elapsed time banked, and when it last started
// running (null while paused).
export type Timer = { bankedMs: number; runningSince: number | null }

export function savedTimer(id: string): Timer {
  try {
    const t = JSON.parse(read(timerKey(id)) ?? '') as Partial<Timer>
    if (typeof t.bankedMs === 'number') return { bankedMs: t.bankedMs, runningSince: typeof t.runningSince === 'number' ? t.runningSince : null }
  } catch {
    // no timer yet
  }
  return { bankedMs: 0, runningSince: null }
}

export const saveTimer = (id: string, t: Timer) => write(timerKey(id), JSON.stringify(t))

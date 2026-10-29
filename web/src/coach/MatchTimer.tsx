import { useEffect, useState } from 'react'
import { useI18n } from '../i18n'
import { saveTimer, savedTimer, type Timer } from './session'

const elapsed = (t: Timer, now: number) => t.bankedMs + (t.runningSince === null ? 0 : now - t.runningSince)

const clock = (ms: number) => {
  const s = Math.floor(ms / 1000)
  return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
}

// A match clock the player starts and pauses by hand; it survives a reload.
export default function MatchTimer({ id }: { id: string }) {
  const { t } = useI18n()
  const [timer, setTimer] = useState(() => savedTimer(id))
  const [now, setNow] = useState(() => Date.now())
  const running = timer.runningSince !== null

  useEffect(() => {
    if (!running) return
    const tick = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(tick)
  }, [running])

  const toggle = () => {
    const at = Date.now()
    const next: Timer = running ? { bankedMs: elapsed(timer, at), runningSince: null } : { bankedMs: timer.bankedMs, runningSince: at }
    setNow(at)
    setTimer(next)
    saveTimer(id, next)
  }

  return (
    <p className="match-timer">
      <span className={running ? 'live-dot on' : 'live-dot'} aria-hidden="true" />
      <span className="sr-only">{t('coach.timer.label')}</span>
      <time aria-live="off">{clock(elapsed(timer, now))}</time>
      <button type="button" className="chip" onClick={toggle} aria-pressed={running}>
        {running ? t('coach.timer.pause') : timer.bankedMs > 0 ? t('coach.timer.resume') : t('coach.timer.start')}
      </button>
    </p>
  )
}

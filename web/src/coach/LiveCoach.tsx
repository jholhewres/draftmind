import { useEffect, useRef, useState } from 'react'
import { sendLiveFrame, type BudgetUse, type MatchMessage, type MatchSession } from '../api'
import BudgetMeter from '../draft/BudgetMeter'
import { useI18n } from '../i18n'
import { canShareScreen, NotAWindow, shareWindow } from '../draft/share'
import { usePersona } from '../settings/persona'
import { grid, inGame, moved } from './liveFrame'

// The tick, in seconds; draftmind.liveTick in localStorage overrides it.
const DEFAULT_TICK_S = 30
const SAMPLE_W = 64
const SAMPLE_H = 36

function tickSeconds(): number {
  try {
    const v = Number(window.localStorage.getItem('draftmind.liveTick'))
    if (Number.isFinite(v) && v >= 10 && v <= 300) return v
  } catch {
    // storage off: the default stands
  }
  return DEFAULT_TICK_S
}

type Stats = { ticks: number; spoken: number; cost: number; budget?: BudgetUse }

// The live coach during a match (ADR-0014): with the game window shared,
// one frame every tick goes to the server when the screen changed and looks
// like a match; a spoken event lands in the chat as a live coach line. The
// bar shows ticks, spoken events and cost, and stops the share.
export default function LiveCoach({ session, onLine }: { session: MatchSession; onLine: (m: MatchMessage) => void }) {
  const { t, locale } = useI18n()
  const { persona } = usePersona()
  const [stream, setStream] = useState<MediaStream | null>(null)
  const [stats, setStats] = useState<Stats>({ ticks: 0, spoken: 0, cost: 0 })
  const [note, setNote] = useState<string | null>(null)
  const lineRef = useRef(onLine)
  // The server answers in the persona's language from the UI one.
  const ctx = useRef({ lang: locale as string, persona })
  useEffect(() => {
    lineRef.current = onLine
    ctx.current = { lang: locale, persona }
  })

  const stop = (s: MediaStream | null = stream) => {
    s?.getTracks().forEach((tr) => tr.stop())
    setStream(null)
  }

  useEffect(() => {
    if (!stream) return
    const video = document.createElement('video')
    video.muted = true
    video.srcObject = stream
    void video.play().catch(() => {})
    const small = document.createElement('canvas')
    small.width = SAMPLE_W
    small.height = SAMPLE_H
    const sctx = small.getContext('2d', { willReadFrequently: true })
    const ctrl = new AbortController()
    let last: number[] | null = null
    let busy = false
    const tick = () => {
      if (busy || !sctx || video.videoWidth === 0) return
      sctx.drawImage(video, 0, 0, SAMPLE_W, SAMPLE_H)
      const g = grid(sctx.getImageData(0, 0, SAMPLE_W, SAMPLE_H))
      // Unchanged or not a match: no call, no cost.
      if (!inGame(g) || (last && !moved(last, g))) return
      last = g
      const full = document.createElement('canvas')
      full.width = Math.min(1600, video.videoWidth)
      full.height = Math.round((video.videoHeight * full.width) / video.videoWidth)
      full.getContext('2d')?.drawImage(video, 0, 0, full.width, full.height)
      busy = true
      full.toBlob((b) => {
        if (!b) return void (busy = false)
        sendLiveFrame(session.id, b, ctx.current.lang, ctx.current.persona, ctrl.signal)
          .then((r) => {
            if (!r) {
              setNote('off')
              stop(stream)
              return
            }
            setStats((s) => ({ ticks: s.ticks + 1, spoken: s.spoken + (r.message ? 1 : 0), cost: s.cost + (r.cost ?? 0), budget: r.budget ?? s.budget }))
            if (r.message) lineRef.current(r.message)
            // The match has a result: the coach stops looking.
            if (r.reason === 'ended') stop(stream)
          })
          .catch(() => {}) // fail silent
          .finally(() => (busy = false))
      }, 'image/jpeg', 0.85)
    }
    const timer = window.setInterval(tick, tickSeconds() * 1000)
    const track = stream.getVideoTracks()[0]
    const ended = () => setStream(null)
    track?.addEventListener('ended', ended)
    return () => {
      ctrl.abort()
      window.clearInterval(timer)
      track?.removeEventListener('ended', ended)
      video.srcObject = null
    }
  }, [stream, session.id])

  // Leaving the Coach tab ends the share.
  const streamRef = useRef(stream)
  useEffect(() => {
    streamRef.current = stream
  })
  useEffect(() => () => streamRef.current?.getTracks().forEach((tr) => tr.stop()), [])

  if (!canShareScreen()) return null
  const cost = new Intl.NumberFormat(locale, { style: 'currency', currency: 'USD', maximumFractionDigits: 3 }).format(stats.cost)
  return (
    <section className={stream ? 'live-bar coach-live on' : 'live-bar coach-live'} aria-label={t('liveCoach.bar')}>
      {stream ? (
        <>
          <span className="live-tag">
            <span className="share-dot" aria-hidden="true" />
            {t('live.on')}
          </span>
          <span className="muted live-stats">{t('liveCoach.stats', { ticks: stats.ticks, spoken: stats.spoken, cost })}</span>
          <BudgetMeter budget={stats.budget} />
          <button type="button" className="chip live-stop" onClick={() => stop()}>
            {t('live.stop')}
          </button>
        </>
      ) : (
        <>
          <span className="muted live-stats">{note === 'off' ? t('liveCoach.off') : (note ?? t('liveCoach.hint'))}</span>
          <button
            type="button"
            className="chip live-stop"
            onClick={() => {
              setNote(null)
              shareWindow()
                .then(setStream)
                .catch((e: unknown) => {
                  if (e instanceof NotAWindow) setNote(t('live.notWindow'))
                })
            }}
          >
            {t('liveCoach.start')}
          </button>
        </>
      )}
    </section>
  )
}

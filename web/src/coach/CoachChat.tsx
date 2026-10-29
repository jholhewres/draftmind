import { useEffect, useRef, useState, type FormEvent } from 'react'
import { sendMatchMessage, type Category, type MatchMessage, type MatchSession, type Priority } from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'

const categoryLabel: Record<Category, MessageKey> = {
  lane: 'coach.category.lane',
  teamfight: 'coach.category.teamfight',
  items: 'coach.category.items',
  objective: 'coach.category.objective',
  other: 'coach.category.other',
}

const priorityLabel: Record<Priority, MessageKey> = {
  low: 'coach.priority.low',
  medium: 'coach.priority.medium',
  high: 'coach.priority.high',
}

type Line = Omit<MatchMessage, 'id'> & { key: string }

const lineOf = (m: MatchMessage): Line => ({ ...m, key: `m${m.id}` })

// The chat with the coach: the player's messages, the coach's answers with
// their category and priority, and the box to write the next one. The
// push-to-talk button (B3) takes the slot beside the box.
export default function CoachChat({ session }: { session: MatchSession }) {
  const { t, locale } = useI18n()
  const [lines, setLines] = useState<Line[]>(() => session.messages.map(lineOf))
  const [text, setText] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const ctrl = useRef<AbortController | null>(null)
  const end = useRef<HTMLDivElement>(null)

  useEffect(() => () => ctrl.current?.abort(), [])
  useEffect(() => {
    end.current?.scrollIntoView?.({ block: 'end' })
  }, [lines, pending])

  const send = (e: FormEvent) => {
    e.preventDefault()
    const msg = text.trim()
    if (!msg || pending) return
    const mine = `p${Date.now()}`
    setLines((cur) => [...cur, { key: mine, author: 'player', kind: 'text', text: msg }])
    setText('')
    setPending(true)
    setError(null)
    ctrl.current = new AbortController()
    sendMatchMessage(
      session.id,
      msg,
      locale,
      (ev) => {
        if (ev.type === 'reply') setLines((cur) => [...cur, { ...ev.message, key: `c${Date.now()}` }])
      },
      ctrl.current.signal,
    )
      .catch((err: unknown) => {
        if (!ctrl.current?.signal.aborted) setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => setPending(false))
  }

  const coachText = (l: Line) => {
    if (l.kind === 'followup') return t('coach.followup')
    return l.text || t('coach.template')
  }

  return (
    <section className="coach-chat" aria-label={t('coach.chat')}>
      <ol className="chat" aria-live="polite">
        {lines.map((l) => (
          <li key={l.key} className={l.author === 'player' ? 'bubble mine' : `bubble coach ${l.kind}`}>
            {l.author === 'coach' && l.kind === 'answer' && l.category && l.priority && (
              <span className="advice-tag">
                {t('coach.tag', { category: t(categoryLabel[l.category]), priority: t(priorityLabel[l.priority]) })}
              </span>
            )}
            <span className="bubble-text">{l.author === 'player' ? l.text : coachText(l)}</span>
          </li>
        ))}
        {pending && (
          <li className="bubble coach pending">
            <span className="bubble-text">{t('coach.thinking')}</span>
          </li>
        )}
      </ol>
      <div ref={end} />
      {error && <p role="alert">{t('coach.sendError', { error })}</p>}
      <form className="chat-input" onSubmit={send}>
        <label className="chat-field">
          <span className="sr-only">{t('coach.input')}</span>
          <input
            type="text"
            value={text}
            maxLength={500}
            placeholder={t('coach.placeholder')}
            onChange={(e) => setText(e.target.value)}
          />
        </label>
        <button type="submit" className="send" disabled={pending || text.trim() === ''} aria-label={t('coach.send')}>
          <svg aria-hidden="true" width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M22 2L11 13M22 2l-7 20-4-9-9-4 20-7z" />
          </svg>
        </button>
        <div className="ptt-slot" data-slot="push-to-talk" />
      </form>
    </section>
  )
}

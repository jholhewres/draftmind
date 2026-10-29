import { useEffect, useRef, useState, type FormEvent } from 'react'
import {
  sendMatchMessage,
  sendMatchVoice,
  VoiceError,
  type Category,
  type CoachEvent,
  type MatchMessage,
  type MatchSession,
  type Priority,
  type QuickAction,
} from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'
import ItemCard from './ItemCard'
import PushToTalk, { type MicNote } from './PushToTalk'
import { canSpeak, speak } from './speech'
import { coachLang, usePersona } from '../settings/persona'

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

// One tap sends the chip's own words as the player's message; a chip with an
// action is answered by the server's code, with no model call.
const quickActions: { key: MessageKey; action?: QuickAction }[] = [
  { key: 'coach.quick.lane' },
  { key: 'coach.quick.items', action: 'items' },
  { key: 'coach.quick.teamfight' },
  { key: 'coach.quick.objective' },
]

// How far below the window's bottom the chat's end may sit while the player
// still counts as reading the newest message.
const PINNED_PX = 120
const atEnd = (end: HTMLElement | null) => !end || end.getBoundingClientRect().top <= window.innerHeight + PINNED_PX

type Turn = { run: (onEvent: (ev: CoachEvent) => void, signal: AbortSignal) => Promise<void>; spoken: boolean }

const lineOf = (m: MatchMessage): Line => ({ ...m, key: `m${m.id}` })

const noteText: Record<MicNote | 'unheard', MessageKey> = {
  denied: 'coach.mic.denied',
  unsupported: 'coach.mic.unsupported',
  short: 'coach.mic.short',
  unheard: 'coach.mic.unheard',
}

const micIcon = (
  <svg aria-hidden="true" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
    <path d="M19 10v2a7 7 0 0 1-14 0v-2M12 19v4M8 23h8" />
  </svg>
)

// The chat with the coach: the player's messages, the coach's answers with
// their category and priority, and the box to write the next one, with the
// push-to-talk button beside it. A spoken message shows its transcript and
// the answer to it is read aloud in the coach's language.
export default function CoachChat({ session }: { session: MatchSession }) {
  const { t, locale } = useI18n()
  const { persona } = usePersona()
  // The coach writes, and its text is spoken, in the persona's language; a
  // template the UI words is spoken in the UI's.
  const lang = coachLang(persona, locale)
  const [lines, setLines] = useState<Line[]>(() => session.messages.map(lineOf))
  const [text, setText] = useState('')
  const [pending, setPending] = useState(false)
  // The coach's decision, once it streams in: its answer is being written.
  const [writing, setWriting] = useState<{ category: Category; priority: Priority } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [note, setNote] = useState<MicNote | 'unheard' | null>(null)
  const [voicePending, setVoicePending] = useState(false)
  // A line arrived while the player was reading further up.
  const [unseen, setUnseen] = useState(false)
  const ctrl = useRef<AbortController | null>(null)
  const last = useRef<Turn | null>(null)
  const end = useRef<HTMLDivElement>(null)
  // Whether the player was at the newest message before this render.
  const pinned = useRef(true)

  useEffect(() => () => ctrl.current?.abort(), [])
  useEffect(() => {
    const onScroll = () => {
      pinned.current = atEnd(end.current)
      if (pinned.current) setUnseen(false)
    }
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])
  // The newest message stays in view unless the player scrolled up to read;
  // their own message always brings them back down.
  const toEnd = () => end.current?.scrollIntoView?.({ block: 'end' })
  useEffect(() => {
    if (pinned.current || lines.at(-1)?.author === 'player') toEnd()
    else setUnseen(true)
  }, [lines, pending, writing, error])

  const coachText = (l: Omit<Line, 'key'>) => {
    if (l.kind === 'followup') return t('coach.followup')
    if (!l.text && l.reason === 'quick') return t(l.items?.length ? 'coach.quickItems' : 'coach.quickItemsNone')
    return l.text || t('coach.template')
  }

  // Runs one coach turn; a spoken turn adds its transcript when the server
  // sends it and reads the answer aloud.
  const turn = ({ run, spoken }: Turn) => {
    last.current = { run, spoken }
    pinned.current = pinned.current || atEnd(end.current)
    setPending(true)
    setVoicePending(spoken)
    setWriting(null)
    setError(null)
    // A note about the microphone stays while the player types instead.
    if (spoken) setNote(null)
    ctrl.current = new AbortController()
    run((ev) => {
      if (ev.type === 'message' && spoken) {
        setVoicePending(false)
        setLines((cur) => [...cur, lineOf(ev.message)])
      }
      if (ev.type === 'decision') setWriting({ category: ev.category, priority: ev.priority })
      if (ev.type === 'reply') {
        setWriting(null)
        setLines((cur) => [...cur, { ...ev.message, key: `c${Date.now()}` }])
        if (spoken) speak(coachText(ev.message), ev.message.text ? lang : locale)
      }
    }, ctrl.current.signal)
      .catch((err: unknown) => {
        if (ctrl.current?.signal.aborted) return
        if (err instanceof VoiceError && err.status === 422) setNote('unheard')
        else setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setPending(false)
        setVoicePending(false)
        setWriting(null)
      })
  }

  const say = (raw: string, action?: QuickAction) => {
    const msg = raw.trim()
    if (!msg || pending) return false
    setLines((cur) => [...cur, { key: `p${Date.now()}`, author: 'player', kind: 'text', text: msg }])
    turn({ run: (onEvent, signal) => sendMatchMessage(session.id, msg, locale, persona, onEvent, signal, action), spoken: false })
    return true
  }

  const send = (e: FormEvent) => {
    e.preventDefault()
    if (say(text)) setText('')
  }

  const sendVoice = (wav: Blob) => {
    if (pending) return
    turn({ run: (onEvent, signal) => sendMatchVoice(session.id, wav, locale, persona, onEvent, signal), spoken: true })
  }

  // The failed turn again, without adding the player's message twice.
  const retry = () => {
    if (last.current && !pending) turn(last.current)
  }

  const tag = (category: Category, priority: Priority) => (
    <span className="advice-tag">{t('coach.tag', { category: t(categoryLabel[category]), priority: t(priorityLabel[priority]) })}</span>
  )

  return (
    <section className="coach-chat" aria-label={t('coach.chat')}>
      <ol className="chat" aria-live="polite">
        {lines.map((l) => (
          <li key={l.key} className={l.author === 'player' ? 'bubble mine' : `bubble coach ${l.kind}`}>
            {l.author === 'coach' && l.kind === 'answer' && l.category && l.priority && tag(l.category, l.priority)}
            {l.kind === 'voice' && (
              <span className="voice-tag">
                {micIcon}
                {t('coach.voice')}
              </span>
            )}
            <span className="bubble-text">{l.author === 'player' ? l.text : coachText(l)}</span>
            {l.author === 'coach' && l.items && l.items.length > 0 && (
              <ul className="item-cards" aria-label={t('coach.items')}>
                {l.items.map((it) => (
                  <li key={it.id}>
                    <ItemCard item={it} />
                  </li>
                ))}
              </ul>
            )}
            {l.author === 'coach' && l.kind === 'answer' && canSpeak() && (
              <button type="button" className="listen" onClick={() => speak(coachText(l), l.text ? lang : locale)}>
                <svg aria-hidden="true" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M11 5L6 9H2v6h4l5 4V5z" />
                  <path d="M15.54 8.46a5 5 0 0 1 0 7.07" />
                </svg>
                {t('coach.listen')}
              </button>
            )}
          </li>
        ))}
        {pending && (
          <li className={voicePending ? 'bubble mine pending' : 'bubble coach pending'} role="status">
            {writing && tag(writing.category, writing.priority)}
            <span className="typing">
              <span className="typing-dots" aria-hidden="true">
                <span />
                <span />
                <span />
              </span>
              {t(voicePending ? 'coach.transcribing' : writing ? 'coach.writing' : 'coach.thinking')}
            </span>
          </li>
        )}
        {error && !pending && (
          <li className="bubble coach failed" role="alert">
            <span className="bubble-text">{t('coach.sendError', { error })}</span>
            <button type="button" className="listen" onClick={retry}>
              {t('coach.retry')}
            </button>
          </li>
        )}
      </ol>
      <div ref={end} className="chat-end" />
      {unseen && (
        <button
          type="button"
          className="chip chat-unseen"
          onClick={() => {
            setUnseen(false)
            toEnd()
          }}
        >
          {t('coach.newMessages')}
        </button>
      )}
      {note && (
        <p role="alert" className="mic-note">
          {t(noteText[note])}
        </p>
      )}
      <div className="composer">
        <div className="quick-actions" role="group" aria-label={t('coach.quick')}>
          {quickActions.map(({ key, action }) => (
            <button key={key} type="button" className="chip" disabled={pending} onClick={() => say(t(key), action)}>
              {t(key)}
            </button>
          ))}
        </div>
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
          <PushToTalk disabled={pending} onClip={sendVoice} onNote={setNote} />
        </form>
      </div>
    </section>
  )
}

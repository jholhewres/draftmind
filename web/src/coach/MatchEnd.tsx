import { useEffect, useRef, useState, type FormEvent } from 'react'
import { parseEndMessage, readEndScreen, saveMatchResult, type MatchResult, type MatchSession, type Outcome, type ResultFill } from '../api'
import { prepareImage } from '../draft/prepare'
import { useI18n } from '../i18n'
import BackButton from '../shell/BackButton'
import { clock } from './clock'
import { saveSessionId } from './session'
import { usePersona } from '../settings/persona'

type Source = MatchResult['source']

// Seconds in "38:12" or "1:02:05"; undefined when it is neither.
function seconds(text: string): number | undefined {
  const m = /^(?:(\d{1,2}):)?(\d{1,3}):([0-5]\d)$/.exec(text.trim())
  if (!m) return undefined
  const s = Number(m[1] ?? 0) * 3600 + Number(m[2]) * 60 + Number(m[3])
  return s > 0 && s <= 4 * 3600 ? s : undefined
}

const stat = (text: string): number | undefined => {
  const n = Number(text)
  return text.trim() !== '' && Number.isInteger(n) && n >= 0 && n <= 999 ? n : undefined
}

type Props = { session: MatchSession; heroName: string; signedIn: boolean; onEnded: (r: MatchResult) => void }

// The end of a match: the player tells the result with a tap, a photo of
// the end screen or a message, checks what was read, and confirms. Nothing
// is saved until then; the saved result comes back with the coach's notes.
export default function MatchEnd({ session, heroName, signedIn, onEnded }: Props) {
  const { t, locale } = useI18n()
  const { persona } = usePersona()
  const [result, setResult] = useState<Outcome | null>(null)
  const [source, setSource] = useState<Source>('tap')
  const [duration, setDuration] = useState('')
  const [kda, setKda] = useState({ kills: '', deaths: '', assists: '' })
  const [fromPhoto, setFromPhoto] = useState(false)
  const [busy, setBusy] = useState<'photo' | 'message' | 'save' | null>(null)
  const [note, setNote] = useState<string | null>(null)
  const [message, setMessage] = useState('')
  const [saved, setSaved] = useState<MatchResult | null>(null)
  const ctrl = useRef<AbortController | null>(null)

  useEffect(() => () => ctrl.current?.abort(), [])

  const fill = (f: ResultFill) => {
    if (f.result) setResult(f.result)
    if (f.durationS) setDuration(clock(f.durationS))
    if (f.kills !== undefined && f.deaths !== undefined && f.assists !== undefined)
      setKda({ kills: String(f.kills), deaths: String(f.deaths), assists: String(f.assists) })
  }

  const run = <T,>(kind: 'photo' | 'message', work: (signal: AbortSignal) => Promise<T>, done: (v: T) => void) => {
    ctrl.current?.abort()
    ctrl.current = new AbortController()
    const signal = ctrl.current.signal
    setBusy(kind)
    setNote(null)
    work(signal)
      .then((v) => {
        if (!signal.aborted) done(v)
      })
      .catch(() => {
        if (!signal.aborted) setNote(t(kind === 'photo' ? 'end.photoError' : 'end.messageError'))
      })
      .finally(() => {
        if (!signal.aborted) setBusy(null)
      })
  }

  const onPhoto = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    run(
      'photo',
      async (signal) => readEndScreen(session.id, await prepareImage(file), signal),
      (f) => {
        fill(f)
        setSource('photo')
        setFromPhoto(true)
        if (!f.result) setNote(t('end.photoNoResult'))
      },
    )
  }

  const onMessage = (e: FormEvent) => {
    e.preventDefault()
    const text = message.trim()
    if (!text) return
    run(
      'message',
      (signal) => parseEndMessage(session.id, text, signal),
      (r) => {
        if (!r) return setNote(t('end.unclear'))
        setResult(r)
        setSource('message')
      },
    )
  }

  const tap = (r: Outcome) => {
    setResult(r)
    setSource('tap')
  }

  const save = () => {
    if (!result || busy) return
    const k = stat(kda.kills)
    const d = stat(kda.deaths)
    const a = stat(kda.assists)
    const all = k !== undefined && d !== undefined && a !== undefined
    setBusy('save')
    setNote(null)
    saveMatchResult(session.id, { result, source, durationS: seconds(duration), ...(all ? { kills: k, deaths: d, assists: a } : {}) }, locale, persona)
      .then((r) => {
        saveSessionId(null) // the match is over: the Coach tab starts afresh
        setSaved(r)
        onEnded(r)
      })
      .catch(() => setNote(t('end.saveError')))
      .finally(() => setBusy(null))
  }

  const back = <BackButton parent="#/coach" label={t('end.back')} />

  if (saved) {
    return (
      <section className="screen match-end">
        <header className="screen-head with-back">
          {back}
          <h1>{t('end.title')}</h1>
        </header>
        <p className={`end-saved ${saved.result}`} role="status">
          {t('end.saved', { result: t(saved.result === 'win' ? 'end.win' : 'end.loss'), hero: heroName })}
        </p>
        {saved.notes.length > 0 && (
          <section className="end-notes" aria-labelledby="end-notes-title">
            <h2 id="end-notes-title">{t('end.notes')}</h2>
            <ul>
              {saved.notes.map((n) => (
                <li key={n}>{n}</li>
              ))}
            </ul>
          </section>
        )}
        {signedIn ? (
          <a className="end-link" href="#/coach/history">
            {t('end.toHistory')}
          </a>
        ) : (
          <p className="sign-in-prompt">
            {t('end.visitor')} <a href="#/signin">{t('account.signIn')}</a>
          </p>
        )}
        <a className="end-link" href="#/draft">
          {t('end.next')}
        </a>
      </section>
    )
  }

  return (
    <section className="screen match-end">
      <header className="screen-head with-back">
        {back}
        <h1>{t('end.title')}</h1>
      </header>
      <p className="muted lead">{t('end.lead')}</p>
      <fieldset className="end-result">
        <legend>{t('end.result')}</legend>
        {(['win', 'loss'] as const).map((r) => (
          <label key={r} className={`end-choice ${r}`}>
            <input type="radio" name="result" checked={result === r} onChange={() => tap(r)} />
            <span>{t(r === 'win' ? 'end.win' : 'end.loss')}</span>
          </label>
        ))}
      </fieldset>
      <div className="end-inputs">
        <label className="option">
          <input type="file" accept="image/*" className="sr-only" onChange={onPhoto} disabled={busy !== null} />
          <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
            <circle cx="12" cy="13" r="4" />
          </svg>
          {busy === 'photo' ? t('end.reading') : t('end.photo')}
        </label>
        <form className="end-message" onSubmit={onMessage}>
          <label className="chat-field">
            <span className="sr-only">{t('end.message')}</span>
            <input type="text" value={message} maxLength={500} placeholder={t('end.messagePlaceholder')} onChange={(e) => setMessage(e.target.value)} />
          </label>
          <button type="submit" className="chip" disabled={busy !== null || message.trim() === ''}>
            {busy === 'message' ? t('end.reading') : t('end.readMessage')}
          </button>
        </form>
      </div>
      <section className="end-details" aria-labelledby="end-details-title">
        <h2 id="end-details-title">
          {fromPhoto ? t('end.fromPhoto') : t('end.details')}
          {fromPhoto && <span className="muted"> · {t('end.check')}</span>}
        </h2>
        <div className="end-fields">
          <label>
            <span>{t('end.duration')}</span>
            <input type="text" inputMode="numeric" value={duration} placeholder="38:12" onChange={(e) => setDuration(e.target.value)} />
          </label>
          {(['kills', 'deaths', 'assists'] as const).map((k) => (
            <label key={k}>
              <span>{t(`end.${k}`)}</span>
              <input type="number" min={0} max={999} value={kda[k]} onChange={(e) => setKda((cur) => ({ ...cur, [k]: e.target.value }))} />
            </label>
          ))}
        </div>
      </section>
      {note && <p role="alert">{note}</p>}
      <button type="button" className="start-match" disabled={!result || busy !== null} onClick={save}>
        {busy === 'save' ? t('end.saving') : t('end.save')}
      </button>
      {!signedIn && (
        <p className="sign-in-prompt muted">
          {t('end.visitor')} <a href="#/signin">{t('account.signIn')}</a>
        </p>
      )}
    </section>
  )
}

import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { useI18n } from '../i18n'
import { toWav } from './wav'

// Why the microphone could not record: no permission, no microphone or
// recorder in this browser, or a press too short to hold any words.
export type MicNote = 'denied' | 'unsupported' | 'short'

// The longest clip; the server takes up to 30 s.
const maxClipMs = 29_000
// A shorter press is a tap, not a message.
const minClipMs = 400

type Props = {
  disabled: boolean
  onClip: (wav: Blob) => void
  onNote: (note: MicNote | null) => void
}

// Hold to record, release to send (ADR-0005). The browser records with
// MediaRecorder; the clip goes up as 16 kHz mono WAV.
export default function PushToTalk({ disabled, onClip, onNote }: Props) {
  const { t } = useI18n()
  const [recording, setRecording] = useState(false)
  const held = useRef(false)
  const rec = useRef<{ recorder: MediaRecorder; stream: MediaStream; started: number; timer: number } | null>(null)

  const release = (stream: MediaStream) => stream.getTracks().forEach((tr) => tr.stop())

  const stop = () => {
    held.current = false
    const r = rec.current
    if (!r) return
    rec.current = null
    window.clearTimeout(r.timer)
    if (r.recorder.state !== 'inactive') r.recorder.stop()
    setRecording(false)
  }

  useEffect(
    () => () => {
      const r = rec.current
      if (!r) return
      rec.current = null
      window.clearTimeout(r.timer)
      r.recorder.ondataavailable = null
      r.recorder.onstop = null
      if (r.recorder.state !== 'inactive') r.recorder.stop()
      release(r.stream)
    },
    [],
  )

  const start = async () => {
    if (disabled || held.current || rec.current) return
    held.current = true
    onNote(null)
    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
      held.current = false
      onNote('unsupported')
      return
    }
    let stream: MediaStream
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true })
    } catch (err) {
      held.current = false
      const name = err instanceof DOMException ? err.name : ''
      onNote(name === 'NotAllowedError' || name === 'SecurityError' ? 'denied' : 'unsupported')
      return
    }
    // Released while the permission prompt was open.
    if (!held.current) {
      release(stream)
      onNote('short')
      return
    }
    const recorder = new MediaRecorder(stream)
    const chunks: Blob[] = []
    const started = Date.now()
    recorder.ondataavailable = (e) => {
      if (e.data.size > 0) chunks.push(e.data)
    }
    recorder.onstop = () => {
      release(stream)
      if (Date.now() - started < minClipMs || chunks.length === 0) {
        onNote('short')
        return
      }
      toWav(new Blob(chunks, { type: recorder.mimeType }))
        .then(onClip)
        .catch(() => onNote('unsupported'))
    }
    rec.current = { recorder, stream, started, timer: window.setTimeout(stop, maxClipMs) }
    recorder.start()
    setRecording(true)
  }

  const onKey = (e: KeyboardEvent, down: boolean) => {
    if (e.key !== ' ' && e.key !== 'Enter') return
    e.preventDefault()
    if (down && !e.repeat) void start()
    if (!down) stop()
  }

  return (
    <button
      type="button"
      className={recording ? 'ptt recording' : 'ptt'}
      aria-label={t('coach.ptt')}
      aria-pressed={recording}
      disabled={disabled}
      onPointerDown={(e) => {
        if (e.button !== 0) return
        e.currentTarget.setPointerCapture?.(e.pointerId)
        void start()
      }}
      onPointerUp={stop}
      onPointerCancel={stop}
      onKeyDown={(e) => onKey(e, true)}
      onKeyUp={(e) => onKey(e, false)}
      onContextMenu={(e) => e.preventDefault()}
    >
      <svg aria-hidden="true" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
        <path d="M19 10v2a7 7 0 0 1-14 0v-2M12 19v4M8 23h8" />
      </svg>
    </button>
  )
}

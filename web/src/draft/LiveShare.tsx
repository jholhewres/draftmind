import { useEffect, useRef, useState } from 'react'
import type { StreamRead } from '../api'
import { useI18n } from '../i18n'
import { signature, watcher } from './frameDiff'
import PhotoSuggest from './PhotoSuggest'
import { fitWithin } from './prepare'

// How often the shared window is sampled, in the browser, for free.
const SAMPLE_MS = 2000
// The sample the change detector reads: enough for the ten portraits.
const SAMPLE_W = 160
const SAMPLE_H = 90

// Samples the shared window every SAMPLE_MS and hands over a full frame, at
// most 1600 px on its longest side, only when a hero slot changed. Frames
// live in memory until read and are never stored.
function useLiveFrames(stream: MediaStream, onFrame: (f: File) => void, onEnd: () => void) {
  const frameRef = useRef(onFrame)
  const endRef = useRef(onEnd)
  useEffect(() => {
    frameRef.current = onFrame
    endRef.current = onEnd
  })
  const stopping = useRef(0)
  useEffect(() => {
    window.clearTimeout(stopping.current)
    const video = document.createElement('video')
    video.muted = true
    video.srcObject = stream
    void video.play().catch(() => {})
    const small = document.createElement('canvas')
    small.width = SAMPLE_W
    small.height = SAMPLE_H
    const sctx = small.getContext('2d', { willReadFrequently: true })
    const w = watcher()
    const tick = () => {
      if (!sctx || video.videoWidth === 0) return
      sctx.drawImage(video, 0, 0, SAMPLE_W, SAMPLE_H)
      if (!w.next(signature(sctx.getImageData(0, 0, SAMPLE_W, SAMPLE_H)))) return
      const { width, height } = fitWithin(video.videoWidth, video.videoHeight)
      const full = document.createElement('canvas')
      full.width = width
      full.height = height
      full.getContext('2d')?.drawImage(video, 0, 0, width, height)
      full.toBlob((b) => b && frameRef.current(new File([b], 'live.jpg', { type: 'image/jpeg' })), 'image/jpeg', 0.9)
    }
    const timer = window.setInterval(tick, SAMPLE_MS)
    const track = stream.getVideoTracks()[0]
    const ended = () => endRef.current()
    track?.addEventListener('ended', ended)
    return () => {
      window.clearInterval(timer)
      track?.removeEventListener('ended', ended)
      video.srcObject = null
      // Leaving the live screen, by Stop or by another tab, ends the share;
      // deferred so a remount of the same screen (StrictMode) keeps it.
      stopping.current = window.setTimeout(() => stream.getTracks().forEach((t) => t.stop()), 0)
    }
  }, [stream])
}

// The live draft: the shared game window read whenever a slot changes,
// through the photo screen's read → ranking → decision → tip stream. The
// "Live" bar with its Stop button stays on top the whole time.
export default function LiveDraft({ stream, onStop }: { stream: MediaStream; onStop: () => void }) {
  const { t, locale } = useI18n()
  const [frame, setFrame] = useState<File | null>(null)
  const [reads, setReads] = useState({ count: 0, cost: 0 })
  const stop = () => {
    stream.getTracks().forEach((tr) => tr.stop())
    onStop()
  }
  useLiveFrames(stream, setFrame, onStop)
  const onRead = (r: StreamRead) => setReads((c) => ({ count: c.count + 1, cost: c.cost + (r.cost ?? 0) }))
  const label = stream.getVideoTracks()[0]?.label ?? ''
  const cost = new Intl.NumberFormat(locale, { style: 'currency', currency: 'USD', maximumFractionDigits: 3 }).format(reads.cost)

  return (
    <div className="live-draft">
      <section className="live-bar" aria-label={t('live.bar')}>
        <span className="live-tag">
          <span className="live-dot" aria-hidden="true" />
          {t('live.on')}
        </span>
        <span className="muted live-stats">
          {label ? t('live.window', { name: label }) + ' · ' : ''}
          {t('live.reads', { count: reads.count, cost })}
        </span>
        <button type="button" className="chip live-stop" onClick={stop}>
          {t('live.stop')}
        </button>
      </section>
      {frame ? (
        <PhotoSuggest photo={frame} onNewPhoto={stop} onRead={onRead} />
      ) : (
        <p role="status" className="muted live-wait">
          {t('live.watching')}
        </p>
      )}
    </div>
  )
}

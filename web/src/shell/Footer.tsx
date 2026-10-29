import { useEffect, useState } from 'react'
import { fetchSyncStatus, type SyncStatus } from '../api'
import { useI18n, type Locale } from '../i18n'
import LanguageSwitch from './LanguageSwitch'

// "3 hours ago" in the UI language, in the largest unit that fits.
function since(at: number, now: number, locale: Locale): string {
  const minutes = Math.max(1, Math.round((now - at) / 60_000))
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'always' })
  if (minutes < 60) return rtf.format(-minutes, 'minute')
  const hours = Math.round(minutes / 60)
  if (hours < 24) return rtf.format(-hours, 'hour')
  return rtf.format(-Math.round(hours / 24), 'day')
}

// The trademark note, the current patch with how long ago it was synced,
// and the language switch.
export default function Footer({ signedIn }: { signedIn: boolean }) {
  const { t, locale } = useI18n()
  const [status, setStatus] = useState<SyncStatus | null>(null)
  // When the status arrived: the age is read against it, not each render.
  const [now, setNow] = useState(0)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    const ctrl = new AbortController()
    fetchSyncStatus(ctrl.signal)
      .then((s) => {
        setStatus(s)
        setNow(Date.now())
      })
      .catch(() => {
        if (!ctrl.signal.aborted) setFailed(true)
      })
    return () => ctrl.abort()
  }, [])

  let line = t('app.sync.checking')
  if (failed) line = t('app.sync.unavailable')
  else if (status && !status.patch) line = t('app.sync.none')
  else if (status?.updatedAt)
    line = t('app.sync.status', { patch: status.patch, ago: since(Date.parse(status.updatedAt), now, locale) })
  else if (status) line = t('app.sync.patch', { patch: status.patch })

  return (
    <footer>
      <p className="note">{t('app.trademark')}</p>
      <p className="status" role="status">
        {line}
      </p>
      <LanguageSwitch signedIn={signedIn} />
    </footer>
  )
}

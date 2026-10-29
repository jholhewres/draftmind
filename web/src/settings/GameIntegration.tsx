import { useEffect, useState } from 'react'
import { fetchGsiStatus, issueGsiToken, revokeGsiToken, type GsiStatus } from '../api'
import { useI18n } from '../i18n'

// How often the status is read again while the screen is open.
const POLL_MS = 10_000

// Game State Integration (ADR-0015): makes the account's config file for
// the game's cfg folder, shows the install steps and whether the game is
// posting. The file is shown once; making a new one revokes the old.
export default function GameIntegration() {
  const { t } = useI18n()
  const [status, setStatus] = useState<GsiStatus | null>(null)
  const [file, setFile] = useState<{ filename: string; config: string } | null>(null)
  const [error, setError] = useState(false)

  useEffect(() => {
    const ctrl = new AbortController()
    const read = () =>
      fetchGsiStatus(ctrl.signal)
        .then(setStatus)
        .catch(() => {})
    void read()
    const timer = window.setInterval(read, POLL_MS)
    return () => {
      ctrl.abort()
      window.clearInterval(timer)
    }
  }, [])
  if (!status) return null

  const issue = () => {
    setError(false)
    issueGsiToken()
      .then((f) => {
        setFile(f)
        setStatus({ configured: true, receiving: false })
      })
      .catch(() => setError(true))
  }
  const revoke = () => {
    revokeGsiToken()
      .then(() => {
        setFile(null)
        setStatus({ configured: false, receiving: false })
      })
      .catch(() => setError(true))
  }
  const href = file ? 'data:text/plain;charset=utf-8,' + encodeURIComponent(file.config) : undefined

  return (
    <section className="game-integration" aria-labelledby="gsi-title">
      <h2 id="gsi-title">{t('gsi.title')}</h2>
      <p className="muted">{t('gsi.lead')}</p>
      <p className={status.receiving ? 'gsi-status on' : 'gsi-status'} role="status">
        <span className="gsi-dot" aria-hidden="true" />
        {t(status.receiving ? 'gsi.receiving' : status.configured ? 'gsi.waiting' : 'gsi.off')}
      </p>
      {file && (
        <ol className="gsi-steps">
          <li>
            {t('gsi.step.download')}{' '}
            <a className="chip" href={href} download={file.filename}>
              {file.filename}
            </a>
          </li>
          <li>
            {t('gsi.step.copy')} <code>…/steamapps/common/dota 2 beta/game/dota/cfg/gamestate_integration/</code>
          </li>
          <li>
            {t('gsi.step.launch')} <code>-gamestateintegration</code>
          </li>
          <li>{t('gsi.step.play')}</li>
        </ol>
      )}
      <div className="gsi-actions">
        <button type="button" className="chip" onClick={issue}>
          {t(status.configured ? 'gsi.rotate' : 'gsi.create')}
        </button>
        {status.configured && (
          <button type="button" className="chip" onClick={revoke}>
            {t('gsi.revoke')}
          </button>
        )}
      </div>
      {error && <p role="alert">{t('gsi.error')}</p>}
      <p className="muted gsi-note">{t('gsi.note')}</p>
    </section>
  )
}

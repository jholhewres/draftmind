import { useI18n } from '../i18n'

type Props = { onPhoto: (photo: File) => void }

// The Draft tab's first screen: photograph the pick screen, pick a photo
// from the gallery, or build the draft by hand.
export default function PhotoCapture({ onPhoto }: Props) {
  const { t } = useI18n()
  const take = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = '' // the same photo may be chosen again
    if (file) onPhoto(file)
  }
  return (
    <section className="screen photo-capture">
      <h1>{t('photo.title')}</h1>
      <p className="muted lead">{t('photo.lead')}</p>
      <label className="camera">
        <input type="file" accept="image/*" capture="environment" className="sr-only" onChange={take} />
        <span className="corner tl" aria-hidden="true" />
        <span className="corner tr" aria-hidden="true" />
        <span className="corner bl" aria-hidden="true" />
        <span className="corner br" aria-hidden="true" />
        <span className="camera-icon" aria-hidden="true">
          <svg width="38" height="38" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
            <circle cx="12" cy="13" r="4" />
          </svg>
        </span>
        <span className="camera-text">{t('photo.camera')}</span>
        <span className="muted camera-hint">{t('photo.cameraHint')}</span>
      </label>
      <div className="photo-options">
        <label className="option">
          <input type="file" accept="image/*" className="sr-only" onChange={take} />
          <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <rect x="3" y="3" width="18" height="18" rx="2" />
            <circle cx="8.5" cy="8.5" r="1.5" />
            <path d="M21 15l-5-5L5 21" />
          </svg>
          {t('photo.gallery')}
        </label>
        <a className="option" href="#/draft/manual">
          <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z" />
          </svg>
          {t('photo.manual')}
        </a>
      </div>
      <p className="privacy">
        <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
        </svg>
        {t('photo.privacy')}
      </p>
    </section>
  )
}

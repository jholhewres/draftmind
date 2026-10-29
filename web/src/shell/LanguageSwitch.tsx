import { saveUILanguage } from '../api'
import { useI18n, type Locale } from '../i18n'

const options: { locale: Locale; label: string }[] = [
  { locale: 'pt-BR', label: 'PT' },
  { locale: 'en', label: 'EN' },
]

// PT | EN: switches the whole app at once and keeps the choice in this
// browser, and on the server for a signed-in player.
export default function LanguageSwitch({ signedIn }: { signedIn: boolean }) {
  const { t, locale, setLocale } = useI18n()
  const pick = (l: Locale) => {
    if (l === locale) return
    setLocale(l)
    // The browser keeps it either way; a failed save only loses the sync.
    if (signedIn) saveUILanguage(l).catch(() => {})
  }
  return (
    <div className="lang-switch" role="group" aria-label={t('app.language')}>
      {options.map((o) => (
        <button key={o.locale} type="button" lang={o.locale} aria-pressed={o.locale === locale} onClick={() => pick(o.locale)}>
          {o.label}
        </button>
      ))}
    </div>
  )
}

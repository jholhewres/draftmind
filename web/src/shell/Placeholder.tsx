import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'

// A tab whose screen is not built yet.
export default function Placeholder({ title }: { title: MessageKey }) {
  const { t } = useI18n()
  return (
    <section className="screen">
      <header className="screen-head">
        <h1>{t(title)}</h1>
      </header>
      <p className="muted">{t('screen.soon')}</p>
    </section>
  )
}

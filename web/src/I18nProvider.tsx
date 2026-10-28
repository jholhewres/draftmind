import { useEffect, useMemo, type ReactNode } from 'react'
import { I18nContext, i18nFor, pickLocale } from './i18n'

const browserLanguages = () => (navigator.languages?.length ? navigator.languages : [navigator.language])

// Picks the catalog from the browser languages once, on mount.
export default function I18nProvider({ children }: { children: ReactNode }) {
  const value = useMemo(() => i18nFor(pickLocale(browserLanguages())), [])
  useEffect(() => {
    document.documentElement.lang = value.locale
  }, [value.locale])
  return <I18nContext value={value}>{children}</I18nContext>
}

import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { I18nContext, i18nFor, pickLocale, storedLocale, storeLocale, type Locale } from './i18n'

const browserLanguages = () => (navigator.languages?.length ? navigator.languages : [navigator.language])

// Holds the UI language: the one picked in this browser, else the browser
// languages'. Switching it re-renders every string and every request that
// carries lang, with no reload.
export default function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setState] = useState<Locale>(() => storedLocale() ?? pickLocale(browserLanguages()))
  const setLocale = useCallback((l: Locale) => {
    storeLocale(l)
    setState(l)
  }, [])
  const value = useMemo(() => i18nFor(locale, setLocale), [locale, setLocale])
  useEffect(() => {
    document.documentElement.lang = locale
  }, [locale])
  return <I18nContext value={value}>{children}</I18nContext>
}

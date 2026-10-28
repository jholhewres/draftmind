import { createContext, useContext } from 'react'
import { en, type MessageKey, type Messages } from './locales/en'
import { ptBR } from './locales/pt-BR'

export const catalogs = { en, 'pt-BR': ptBR } satisfies Record<string, Messages>

// Keys of source that target lacks or leaves empty.
export function missingKeys(source: Record<string, string>, target: Record<string, string>): string[] {
  return Object.keys(source).filter((k) => !target[k])
}

export type Locale = keyof typeof catalogs

export const fallbackLocale: Locale = 'en'
const locales = Object.keys(catalogs) as Locale[]
const primary = (tag: string) => tag.toLowerCase().split('-')[0]

// The first browser language with a catalog, by exact tag and then by
// primary language (pt-PT gets pt-BR); English when none matches.
export function pickLocale(languages: readonly string[]): Locale {
  for (const lang of languages) {
    const exact = locales.find((l) => l.toLowerCase() === lang.toLowerCase())
    if (exact) return exact
    const near = locales.find((l) => primary(l) === primary(lang))
    if (near) return near
  }
  return fallbackLocale
}

export type Vars = Record<string, string | number>

// The message for key in locale (English if the catalog lacks it), with
// each {name} replaced by vars.name; unknown names stay as written.
export function translate(locale: Locale, key: MessageKey, vars?: Vars): string {
  const msg = catalogs[locale][key] || catalogs[fallbackLocale][key]
  return vars ? msg.replace(/\{(\w+)\}/g, (m, name: string) => (Object.hasOwn(vars, name) ? String(vars[name]) : m)) : msg
}

export type I18n = { locale: Locale; t: (key: MessageKey, vars?: Vars) => string }

export const i18nFor = (locale: Locale): I18n => ({ locale, t: (key, vars) => translate(locale, key, vars) })

// English until an I18nProvider says otherwise.
export const I18nContext = createContext<I18n>(i18nFor(fallbackLocale))

export const useI18n = () => useContext(I18nContext)

import { useState } from 'react'
import type { Persona } from '../api'
import { useI18n } from '../i18n'
import BackButton from '../shell/BackButton'
import type { MessageKey } from '../locales/en'
import GameIntegration from './GameIntegration'
import { usePersona } from './persona'

type Field = keyof Persona

// Each setting's label and its values' labels, in the order shown.
const options: { [F in Field]: { label: MessageKey; values: Record<Persona[F], MessageKey> } } = {
  tone: {
    label: 'persona.tone',
    values: { direct: 'persona.tone.direct', encouraging: 'persona.tone.encouraging', analytic: 'persona.tone.analytic' },
  },
  language: { label: 'persona.language', values: { auto: 'persona.language.auto', en: 'persona.language.en', 'pt-BR': 'persona.language.pt-BR' } },
  verbosity: { label: 'persona.verbosity', values: { short: 'persona.verbosity.short', normal: 'persona.verbosity.normal' } },
  focus: {
    label: 'persona.focus',
    values: { laning: 'persona.focus.laning', teamfights: 'persona.focus.teamfights', items: 'persona.focus.items', macro: 'persona.focus.macro' },
  },
}

const entries = <K extends string>(r: Record<K, MessageKey>) => Object.entries(r) as [K, MessageKey][]

const fields: Field[] = ['tone', 'language', 'verbosity', 'focus']

// The coach settings: tone, language, verbosity and focus. They change how
// the coach words its answers, never what it advises.
export default function PersonaSettings({ signedIn }: { signedIn: boolean }) {
  const { t } = useI18n()
  const { persona, save } = usePersona()
  const [status, setStatus] = useState<'saved' | 'error' | null>(null)

  const choose = (p: Persona) => {
    setStatus(null)
    save(p)
      .then(() => setStatus('saved'))
      .catch(() => setStatus('error'))
  }

  return (
    <section className="screen persona-settings">
      <header className="screen-head with-back">
        <BackButton parent="#/coach" label={t('persona.back')} />
        <h1>{t('persona.title')}</h1>
      </header>
      <p className="muted lead">{t('persona.lead')}</p>
      {fields.map((f) => (
        <fieldset key={f} className="persona-field">
          <legend>{t(options[f].label)}</legend>
          <div className="persona-options">
            {entries<string>(options[f].values).map(([v, label]) => (
              <label key={v} className="persona-option">
                <input type="radio" name={f} checked={persona[f] === v} onChange={() => choose({ ...persona, [f]: v })} />
                <span>{t(label)}</span>
              </label>
            ))}
          </div>
        </fieldset>
      ))}
      <p className="muted persona-note" role="status">
        {status === 'error' ? t('persona.error') : status === 'saved' ? t(signedIn ? 'persona.savedAccount' : 'persona.savedDevice') : ''}
      </p>
      {!signedIn && (
        <p className="sign-in-prompt">
          {t('persona.visitor')} <a href="#/signin">{t('account.signIn')}</a>
        </p>
      )}
      {signedIn && <GameIntegration />}
    </section>
  )
}

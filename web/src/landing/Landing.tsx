import { useEffect, useState } from 'react'
import { fetchHeroes, heroImageBase, type Hero } from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'
import LanguageSwitch from '../shell/LanguageSwitch'

// The Radiant and Dire art behind the hero section, from the game's CDN.
export const heroArt = 'https://cdn.cloudflare.steamstatic.com/apps/dota2/images/dota_react/home/radiant_dire5.jpg'

const steps: [MessageKey, MessageKey][] = [
  ['landing.step1.title', 'landing.step1.text'],
  ['landing.step2.title', 'landing.step2.text'],
  ['landing.step3.title', 'landing.step3.text'],
]

const bullets: MessageKey[] = ['landing.patch.bullet1', 'landing.patch.bullet2', 'landing.patch.bullet3']

const check = (
  <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
    <path d="M20 6L9 17l-5-5" />
  </svg>
)

// The visitor's entry at the root, after the prototype's Landing page:
// the hero over the art with the call to action, how it works in three
// steps, and the current patch with portraits from the database.
export default function Landing() {
  const { t } = useI18n()
  const [portraits, setPortraits] = useState<Hero[]>([])

  useEffect(() => {
    const ctrl = new AbortController()
    fetchHeroes(ctrl.signal)
      .then((c) => setPortraits(c.heroes.slice(0, 6)))
      .catch(() => {}) // the section reads well without them
    return () => ctrl.abort()
  }, [])

  const showHow = () => document.getElementById('how-it-works')?.scrollIntoView({ behavior: 'smooth' })

  return (
    <div className="landing">
      <section className="landing-hero" style={{ backgroundImage: `url(${heroArt})` }}>
        <header className="landing-top">
          <span className="brand">
            <span className="brand-mark" aria-hidden="true">
              D
            </span>
            <span className="brand-name">Draftmind</span>
          </span>
          <a className="ghost small" href="#/signin">
            {t('account.signIn')}
          </a>
        </header>
        <div className="landing-pitch">
          <span className="eyebrow">{t('landing.eyebrow')}</span>
          <h1>{t('landing.title')}</h1>
          <p>{t('landing.lead')}</p>
          <div className="landing-ctas">
            <a className="cta" href="#/draft">
              {t('landing.start')}
            </a>
            <button type="button" className="ghost" onClick={showHow}>
              {t('landing.how')}
            </button>
          </div>
        </div>
      </section>

      <section id="how-it-works" className="landing-section" aria-labelledby="how-title">
        <h2 id="how-title">{t('landing.how')}</h2>
        <ol className="landing-steps">
          {steps.map(([title, text], i) => (
            <li key={title}>
              <span className="step-number" aria-hidden="true">
                {i + 1}
              </span>
              <div>
                <h3>{t(title)}</h3>
                <p>{t(text)}</p>
              </div>
            </li>
          ))}
        </ol>
      </section>

      <section className="landing-section" aria-labelledby="patch-title">
        <h2 id="patch-title">{t('landing.patch.title')}</h2>
        {portraits.length > 0 && (
          <div className="landing-portraits" aria-hidden="true">
            {portraits.map((h) => (
              <img key={h.id} src={heroImageBase + h.img} alt="" width={128} height={72} />
            ))}
          </div>
        )}
        <ul className="landing-bullets">
          {bullets.map((b) => (
            <li key={b}>
              {check}
              {t(b)}
            </li>
          ))}
        </ul>
        <a className="cta" href="#/signin">
          {t('landing.createAccount')}
        </a>
      </section>

      <footer className="landing-foot">
        <p>{t('landing.note')}</p>
        <nav aria-label={t('landing.footerNav')}>
          <a href="#/heroes">{t('nav.heroes')}</a>
          <a href="#/patches">{t('nav.patches')}</a>
          <a href="#/signin">{t('account.signIn')}</a>
        </nav>
        <LanguageSwitch signedIn={false} />
      </footer>
    </div>
  )
}

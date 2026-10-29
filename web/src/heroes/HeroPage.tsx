import { useEffect, useState } from 'react'
import { fetchHero, fetchHeroHistory, heroImageBase, type HeroDetail, type HeroHistory, type HeroStats } from '../api'
import { useI18n } from '../i18n'
import { en, type MessageKey } from '../locales/en'
import NoteList from '../patches/NoteList'
import Counters from './Counters'

const attrLabel: Record<string, MessageKey> = {
  str: 'heroes.attr.str',
  agi: 'heroes.attr.agi',
  int: 'heroes.attr.int',
  all: 'heroes.attr.all',
}

// The catalog key for a game term, when the catalogs have one.
const termKey = (prefix: string, term: string): MessageKey | null => {
  const key = `${prefix}.${term}`
  return Object.hasOwn(en, key) ? (key as MessageKey) : null
}

type State = { slug: string; lang: string } & (
  | { status: 'loading' }
  | { status: 'error'; error: string }
  | { status: 'ready'; hero: HeroDetail | null }
)

export default function HeroPage({ slug }: { slug: string }) {
  const { t, locale } = useI18n()
  const [state, setState] = useState<State>({ slug, lang: locale, status: 'loading' })

  useEffect(() => {
    const ctrl = new AbortController()
    fetchHero(slug, locale, ctrl.signal)
      .then((hero) => setState({ slug, lang: locale, status: 'ready', hero }))
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) setState({ slug, lang: locale, status: 'error', error: e instanceof Error ? e.message : String(e) })
      })
    return () => ctrl.abort()
  }, [slug, locale])

  const current = state.slug === slug && state.lang === locale ? state : { status: 'loading' as const }
  if (current.status === 'loading') return <p role="status">{t('hero.loading')}</p>
  if (current.status === 'error') return <p role="alert">{t('hero.loadError', { error: current.error })}</p>
  if (!current.hero) {
    return (
      <section className="screen">
        <a className="back-link" href="#/heroes">
          {t('hero.back')}
        </a>
        <p className="muted">{t('hero.notFound', { slug })}</p>
      </section>
    )
  }
  return <Hero hero={current.hero} />
}

function Hero({ hero }: { hero: HeroDetail }) {
  const { t, locale } = useI18n()
  const num = (n: number) => new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(n)
  const nums = (ns: number[]) => ns.map(num).join(' / ')
  const term = (prefix: string, value: string) => {
    const key = termKey(prefix, value)
    return key ? t(key) : value
  }
  const attr = attrLabel[hero.primaryAttr]
  const meta = [
    attr ? t(attr) : hero.primaryAttr,
    term('hero.attack', hero.attackType),
    hero.roles.map((r) => term('hero.role', r)).join(', '),
  ]
    .filter(Boolean)
    .join(' · ')

  return (
    <article className="hero-page">
      <section className="hero-banner">
        <img src={heroImageBase + hero.img} alt="" width={256} height={144} />
        <a className="back" href="#/heroes" aria-label={t('hero.back')}>
          <svg aria-hidden="true" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M19 12H5M12 19l-7-7 7-7" />
          </svg>
        </a>
        <div className="hero-banner-text">
          <h1>{hero.name}</h1>
          <p className="hero-meta">
            <span className={`attr-icon attr-${hero.primaryAttr}`} aria-hidden="true" />
            <span>{meta}</span>
          </p>
        </div>
      </section>
      {hero.hype && <p className="hero-hype">{hero.hype}</p>}
      <Counters slug={hero.slug} />
      {!hero.stats ? (
        <p className="muted">{t('hero.notSynced')}</p>
      ) : (
        <div className="hero-body">
          <div className="hero-side">
            <Stats stats={hero.stats} primary={hero.primaryAttr} num={num} />
            {hero.talents.length > 0 && (
              <section className="panel" aria-labelledby="hero-talents">
                <h2 id="hero-talents">{t('hero.talents')}</h2>
                <ol className="talents">
                  {hero.talents.map((tl) => (
                    <li key={tl.id}>
                      <span className="talent-level">{t('hero.talentLevel', { level: tl.level })}</span>
                      <span>{tl.name}</span>
                    </li>
                  ))}
                </ol>
              </section>
            )}
          </div>
          <section className="panel" aria-labelledby="hero-abilities">
            <h2 id="hero-abilities">{t('hero.abilities')}</h2>
            <ul className="abilities">
              {hero.abilities.map((a) => (
                <li key={a.id} className="ability">
                  <div className="ability-head">
                    <h3>{a.name}</h3>
                    {a.kind === 'innate' && <span className="tag">{t('hero.innate')}</span>}
                  </div>
                  <p className="ability-desc">{a.description}</p>
                  {(a.cooldowns.length > 0 || a.manaCosts.length > 0) && (
                    <p className="ability-cost">
                      {a.cooldowns.length > 0 && <span>{t('hero.cooldown', { values: nums(a.cooldowns) })}</span>}
                      {a.manaCosts.length > 0 && <span>{t('hero.manaCost', { values: nums(a.manaCosts) })}</span>}
                    </p>
                  )}
                </li>
              ))}
            </ul>
          </section>
        </div>
      )}
      <History slug={hero.slug} />
    </article>
  )
}

// Patches shown before "show all".
const HISTORY_SHOWN = 5

// The hero's changes, newest patch first; nothing while it loads or when
// none are synced.
function History({ slug }: { slug: string }) {
  const { t, locale } = useI18n()
  const [history, setHistory] = useState<{ key: string; data: HeroHistory | null } | null>(null)
  const [all, setAll] = useState(false)
  const key = `${slug}:${locale}`

  useEffect(() => {
    const ctrl = new AbortController()
    fetchHeroHistory(slug, locale, ctrl.signal)
      .then((data) => setHistory({ key: `${slug}:${locale}`, data }))
      .catch(() => {}) // the page stands without it
    return () => ctrl.abort()
  }, [slug, locale])

  const patches = history?.key === key ? (history.data?.patches ?? []) : []
  if (patches.length === 0) return null
  const shown = all ? patches : patches.slice(0, HISTORY_SHOWN)
  return (
    <section className="panel hero-history" aria-labelledby="hero-history">
      <h2 id="hero-history">{t('hero.history')}</h2>
      <ol className="history">
        {shown.map((p) => (
          <li key={p.name}>
            <span className="history-patch">{p.name}</span>
            <NoteList notes={p.notes} />
          </li>
        ))}
      </ol>
      {!all && patches.length > HISTORY_SHOWN && (
        <button type="button" className="chip" onClick={() => setAll(true)}>
          {t('hero.historyAll', { count: patches.length })}
        </button>
      )}
    </section>
  )
}

function Stats({ stats: s, primary, num }: { stats: HeroStats; primary: string; num: (n: number) => string }) {
  const { t } = useI18n()
  const attrs = [
    ['str', s.strBase, s.strGain],
    ['agi', s.agiBase, s.agiGain],
    ['int', s.intBase, s.intGain],
  ] as const
  const rows: [MessageKey, string][] = [
    ['hero.stat.health', num(s.maxHealth)],
    ['hero.stat.mana', num(s.maxMana)],
    ['hero.stat.armor', num(Math.round(s.armor * 10) / 10)],
    ['hero.stat.magicResistance', `${num(s.magicResistance)}%`],
    ['hero.stat.damage', `${num(s.damageMin)}–${num(s.damageMax)}`],
    ['hero.stat.attackRange', num(s.attackRange)],
    ['hero.stat.attackRate', `${num(s.attackRate)}s`],
    ['hero.stat.moveSpeed', num(s.movementSpeed)],
  ]
  return (
    <section className="panel" aria-labelledby="hero-stats">
      <h2 id="hero-stats">{t('hero.stats')}</h2>
      <ul className="hero-attrs">
        {attrs.map(([a, baseValue, gain]) => (
          <li key={a} aria-current={a === primary ? 'true' : undefined}>
            <span className={`attr-icon attr-${a}`} aria-hidden="true" />
            <span className="sr-only">{t(attrLabel[a])}</span>
            <span>{num(baseValue)}</span>
            <span className="muted">+{num(gain)}</span>
          </li>
        ))}
      </ul>
      <dl className="stats">
        {rows.map(([key, value]) => (
          <div key={key}>
            <dt>{t(key)}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}

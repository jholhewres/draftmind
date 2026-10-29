import { heroImageBase, type FallbackReason, type Hero, type Suggestion, type Tip } from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'
import { formatDelta, formatScore } from './format'
import type { SuggestStream } from './useSuggestStream'

const SHOWN = 5
// How many top heroes the server writes a tip for.
const TIPPED = 3

// What the fallback label says when hovered: why the code ranking stands.
const fallbackReason: Record<FallbackReason, MessageKey> = {
  timeout: 'draft.source.timeout',
  error: 'draft.source.error',
  unavailable: 'draft.source.unavailable',
  few_candidates: 'draft.source.fewCandidates',
  low_confidence: 'draft.source.lowConfidence',
}

type Props = SuggestStream & { byId: Map<number, Hero> }

// The top of the ranking with each hero's counter chips, score and tip.
// Items are keyed by hero, so a new order or a tip updates them in place.
export default function SuggestionList({ result, error, tips, streaming, byId }: Props) {
  const { t, locale } = useI18n()
  const name = (id: number) => byId.get(id)?.name ?? `#${id}`

  // A template tip, worded here from the hero's reasons (the server's entity
  // check rejected the written one, or the writer was late or failed).
  const template = (s: Suggestion) => {
    const winRate = new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 1 }).format(s.winRate / 100)
    const good = s.enemies
      .filter((e) => e.delta > 0)
      .sort((a, b) => b.delta - a.delta)
      .slice(0, 2)
    if (good.length === 0) return t('draft.tip.patch', { hero: name(s.heroId), winRate })
    const enemies = new Intl.ListFormat(locale, { type: 'conjunction' }).format(good.map((e) => name(e.heroId)))
    return t('draft.tip.counter', { hero: name(s.heroId), enemies, winRate })
  }

  const tipFor = (s: Suggestion, rank: number, tip: Tip | undefined) => {
    if (tip) return <p className="tip">{tip.source === 'writer' && tip.text ? tip.text : template(s)}</p>
    if (streaming && rank < TIPPED) return <p className="tip pending">{t('draft.tip.writing')}</p>
    return null
  }

  return (
    <section className="suggestions" aria-label={t('draft.suggestions')}>
      <div className="suggestions-head">
        <h2>{t('draft.suggestions')}</h2>
        {result?.deciding && !error && <span className="source-label refining">{t('draft.refining')}</span>}
        {result && !result.deciding && result.source === 'ranking' && !error && (
          <span className="source-label" title={t(fallbackReason[result.reason ?? 'error'] ?? 'draft.source.error')}>
            {t('draft.source.ranking')}
          </span>
        )}
      </div>
      {error ? (
        <p role="alert">{t('draft.rankError', { error })}</p>
      ) : !result ? (
        <p role="status">{t('draft.ranking')}</p>
      ) : result.suggestions.length === 0 ? (
        <p className="muted">{t('draft.none')}</p>
      ) : (
        <ol className="suggestion-list" aria-label={t('draft.suggestions')}>
          {result.suggestions.slice(0, SHOWN).map((s, rank) => {
            const hero = byId.get(s.heroId)
            return (
              <li key={s.heroId} className="suggestion">
                <div className="suggestion-row">
                  {hero && <img src={heroImageBase + hero.img} alt="" width={128} height={72} />}
                  <div className="suggestion-main">
                    <span className="suggestion-name">{name(s.heroId)}</span>
                    {s.enemies.length > 0 && (
                      <span className="deltas">
                        {s.enemies.map((e) => {
                          const { arrow, text } = formatDelta(locale, e.delta)
                          const dir = arrow === '▲' ? 'up' : arrow === '▼' ? 'down' : 'even'
                          return (
                            <span key={e.heroId} className={`delta ${dir}`}>
                              {t('draft.delta', { arrow, hero: name(e.heroId), delta: text }).trim()}
                            </span>
                          )
                        })}
                      </span>
                    )}
                  </div>
                  <span className="score">
                    <span className="score-label">{t('draft.score')}</span>
                    <span className="score-value">{formatScore(locale, s.score)}</span>
                  </span>
                </div>
                {tipFor(s, rank, tips.get(s.heroId))}
              </li>
            )
          })}
        </ol>
      )}
    </section>
  )
}

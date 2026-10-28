import { useEffect, useId, useMemo, useState, type ReactNode } from 'react'
import { fetchHeroes, fetchSuggestions, heroImageBase, type Hero, type HeroCatalog, type SuggestResult } from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'
import { formatDelta, formatScore } from './format'
import HeroPicker from './HeroPicker'

type Side = 'ally' | 'enemy' | 'ban'
type Board = { ally: (number | null)[]; enemy: (number | null)[]; ban: number[] }
type Slot = { side: Side; index: number }

// The player's own slot is the one suggested, so four allies; ranked games
// ban at most ten heroes.
const ALLIES = 4
const ENEMIES = 5
const MAX_BANS = 10
const SHOWN = 5
const roles = [1, 2, 3, 4, 5]

const emptyBoard = (): Board => ({ ally: Array(ALLIES).fill(null), enemy: Array(ENEMIES).fill(null), ban: [] })

const label: Record<Side, { row: MessageKey; add: MessageKey; slot: MessageKey; choose: MessageKey }> = {
  ally: { row: 'draft.allies', add: 'draft.add.ally', slot: 'draft.slot.ally', choose: 'draft.choose.ally' },
  enemy: { row: 'draft.enemies', add: 'draft.add.enemy', slot: 'draft.slot.enemy', choose: 'draft.choose.enemy' },
  ban: { row: 'draft.bans', add: 'draft.add.ban', slot: 'draft.slot.ban', choose: 'draft.choose.ban' },
}

const ids = (slots: (number | null)[]) => slots.filter((id): id is number => id !== null)

function place(board: Board, { side, index }: Slot, id: number | null): Board {
  if (side === 'ban') {
    const ban = [...board.ban]
    if (id === null) ban.splice(index, 1)
    else ban[index] = id
    return { ...board, ban }
  }
  const slots = [...board[side]]
  slots[index] = id
  return { ...board, [side]: slots }
}

export default function DraftBoard() {
  const { t, locale } = useI18n()
  const [catalog, setCatalog] = useState<HeroCatalog | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [board, setBoard] = useState<Board>(emptyBoard)
  const [role, setRole] = useState(1)
  const [picking, setPicking] = useState<Slot | null>(null)
  const [result, setResult] = useState<SuggestResult | null>(null)
  const [rankError, setRankError] = useState<string | null>(null)

  useEffect(() => {
    const ctrl = new AbortController()
    fetchHeroes(ctrl.signal)
      .then(setCatalog)
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) setError(e instanceof Error ? e.message : String(e))
      })
    return () => ctrl.abort()
  }, [])

  const input = useMemo(
    () => ({ allies: ids(board.ally), enemies: ids(board.enemy), bans: board.ban, role }),
    [board, role],
  )

  useEffect(() => {
    const ctrl = new AbortController()
    fetchSuggestions(input, ctrl.signal)
      .then((r) => {
        setResult(r)
        setRankError(null)
      })
      .catch((e: unknown) => {
        if (!ctrl.signal.aborted) setRankError(e instanceof Error ? e.message : String(e))
      })
    return () => ctrl.abort()
  }, [input])

  const byId = useMemo(() => new Map((catalog?.heroes ?? []).map((h) => [h.id, h])), [catalog])
  const taken = new Set([...input.allies, ...input.enemies, ...input.bans])
  const name = (id: number) => byId.get(id)?.name ?? `#${id}`

  const slotButton = (side: Side, index: number, id: number | null) => {
    const hero = id === null ? undefined : byId.get(id)
    return (
      <button
        key={`${side}-${index}`}
        type="button"
        className={`slot ${id === null ? 'empty' : 'filled'}`}
        aria-label={id === null ? t(label[side].add) : t(label[side].slot, { hero: name(id) })}
        onClick={() => setPicking({ side, index })}
      >
        {hero ? (
          <img src={heroImageBase + hero.img} alt="" width={128} height={72} />
        ) : (
          <svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
            <path d="M12 5v14M5 12h14" />
          </svg>
        )}
      </button>
    )
  }

  const pickingId = picking ? (picking.side === 'ban' ? board.ban[picking.index] : board[picking.side][picking.index]) : undefined

  return (
    <section className="screen draft">
      <header className="screen-head">
        <h1>{t('nav.draft')}</h1>
        {catalog?.patch && <span className="muted">{t('heroes.patch', { patch: catalog.patch })}</span>}
      </header>
      {error ? (
        <p role="alert">{t('heroes.loadError', { error })}</p>
      ) : !catalog ? (
        <p role="status">{t('heroes.loading')}</p>
      ) : (
        <div className="draft-layout">
          <div className="draft-board">
            <section className="board" aria-label={t('draft.board')}>
              <Row side="ally" title={t(label.ally.row)}>
                <span className="slot you">{t('draft.you')}</span>
                {board.ally.map((id, i) => slotButton('ally', i, id))}
              </Row>
              <Row side="enemy" title={t(label.enemy.row)}>
                {board.enemy.map((id, i) => slotButton('enemy', i, id))}
              </Row>
              <Row side="ban" title={t(label.ban.row)}>
                {board.ban.map((id, i) => slotButton('ban', i, id))}
                {board.ban.length < MAX_BANS && slotButton('ban', board.ban.length, null)}
              </Row>
            </section>
            <fieldset className="roles">
              <legend className="sr-only">{t('draft.role')}</legend>
              <div className="role-options">
                {roles.map((r) => (
                  <label key={r} className="role-option">
                    <input type="radio" name="role" value={r} checked={role === r} onChange={() => setRole(r)} />
                    {t('draft.pos', { n: r })}
                  </label>
                ))}
              </div>
            </fieldset>
            <p className="muted hint">{t('draft.hint')}</p>
          </div>
          <section className="suggestions" aria-label={t('draft.suggestions')}>
            <h2>{t('draft.suggestions')}</h2>
            {rankError ? (
              <p role="alert">{t('draft.rankError', { error: rankError })}</p>
            ) : !result ? (
              <p role="status">{t('draft.ranking')}</p>
            ) : result.suggestions.length === 0 ? (
              <p className="muted">{t('draft.none')}</p>
            ) : (
              <ol className="suggestion-list" aria-label={t('draft.suggestions')}>
                {result.suggestions.slice(0, SHOWN).map((s) => {
                  const hero = byId.get(s.heroId)
                  return (
                    <li key={s.heroId} className="suggestion">
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
                                  {[arrow, name(e.heroId), text].filter(Boolean).join(' ')}
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
                    </li>
                  )
                })}
              </ol>
            )}
          </section>
        </div>
      )}
      {picking && catalog && (
        <HeroPicker
          title={t(label[picking.side].choose)}
          heroes={catalog.heroes.filter((h: Hero) => !taken.has(h.id))}
          filled={pickingId !== undefined && pickingId !== null}
          onPick={(id) => {
            setBoard((b) => place(b, picking, id))
            setPicking(null)
          }}
          onRemove={() => {
            setBoard((b) => place(b, picking, null))
            setPicking(null)
          }}
          onClose={() => setPicking(null)}
        />
      )}
    </section>
  )
}

function Row({ side, title, children }: { side: Side; title: string; children: ReactNode }) {
  const id = useId()
  return (
    <div className={`slot-row ${side}`} role="group" aria-labelledby={id}>
      <span id={id} className="row-label">
        {title}
      </span>
      <div className="slots">{children}</div>
    </div>
  )
}

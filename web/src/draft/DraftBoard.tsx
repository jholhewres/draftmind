import { useEffect, useMemo, useState } from 'react'
import { fetchHeroes, pooledModes, type Hero, type HeroCatalog, type StreamRequest } from '../api'
import { useI18n } from '../i18n'
import BackButton from '../shell/BackButton'
import HeroPicker from './HeroPicker'
import { sideLabel as label, type Side } from './labels'
import { Row, SlotButton } from './slots'
import RoleOptions from './RoleOptions'
import { useStartMatch } from '../coach/useStartMatch'
import SuggestionList from './Suggestions'
import { useSuggestStream } from './useSuggestStream'
import { useGameMode } from './mode'
import ModePicker from './ModePicker'

type Board = { ally: (number | null)[]; enemy: (number | null)[]; ban: number[]; pool: number[]; you: number | null }
// 'pool' holds the heroes offered in a pooled mode; 'you' is the hero All Random gave.
type Slot = { side: Side | 'pool' | 'you'; index: number }

// The player's own slot is the one suggested, so four allies; ranked games
// ban at most ten heroes.
const ALLIES = 4
const ENEMIES = 5
const MAX_BANS = 24
// Captains Draft offers 27 heroes; Single Draft 3, Random Draft 24.
const MAX_POOL = 27

const emptyBoard = (): Board => ({ ally: Array(ALLIES).fill(null), enemy: Array(ENEMIES).fill(null), ban: [], pool: [], you: null })

const ids = (slots: (number | null)[]) => slots.filter((id): id is number => id !== null)

function place(board: Board, { side, index }: Slot, id: number | null): Board {
  if (side === 'you') return { ...board, you: id }
  if (side === 'ban' || side === 'pool') {
    const list = [...board[side]]
    if (id === null) list.splice(index, 1)
    else list[index] = id
    return { ...board, [side]: list }
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
  const [mode, setMode] = useGameMode()
  const pooled = pooledModes.includes(mode)
  const [picking, setPicking] = useState<Slot | null>(null)

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
    () => ({ allies: ids(board.ally), enemies: ids(board.enemy), bans: board.ban, role, mode, ...(pooled ? { pool: board.pool } : {}) }),
    [board, role, mode, pooled],
  )

  // A pooled mode ranks nothing until the offered heroes are in; All Random
  // ranks nothing at all: its hero is given.
  const request = useMemo(
    (): StreamRequest | null => (mode === 'all_random' || (pooled && board.pool.length === 0) ? null : { kind: 'draft', draft: input, lang: locale }),
    [input, locale, mode, pooled, board.pool.length],
  )
  const ranking = useSuggestStream(request)
  const match = useStartMatch(input)

  const byId = useMemo(() => new Map((catalog?.heroes ?? []).map((h) => [h.id, h])), [catalog])
  const taken = new Set([...input.allies, ...input.enemies, ...input.bans, ...(board.you === null ? [] : [board.you])])

  const slotButton = (side: Slot['side'], index: number, id: number | null) => (
    <SlotButton
      key={`${side}-${index}`}
      side={side === 'pool' || side === 'you' ? 'ally' : side}
      heroId={id}
      hero={id === null ? undefined : byId.get(id)}
      onClick={() => setPicking({ side, index })}
    />
  )

  const pickingId = picking ? (picking.side === 'you' ? board.you : board[picking.side][picking.index]) : undefined
  const pickerTitle = (side: Slot['side']) => (side === 'pool' ? t('mode.pool') : side === 'you' ? t('mode.yourHero') : t(label[side].choose))

  return (
    <section className="screen draft">
      <header className="screen-head with-back">
        <BackButton parent="#/draft" label={t('draft.back')} />
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
            <ModePicker mode={mode} onChange={setMode} />
            <section className="board" aria-label={t('draft.board')}>
              {pooled && (
                <Row side="ally" title={t('mode.pool')}>
                  {board.pool.map((id, i) => slotButton('pool', i, id))}
                  {board.pool.length < MAX_POOL && slotButton('pool', board.pool.length, null)}
                </Row>
              )}
              {mode === 'all_random' && (
                <Row side="ally" title={t('mode.yourHero')}>
                  {slotButton('you', 0, board.you)}
                </Row>
              )}
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
            <RoleOptions name="role" role={role} onChange={setRole} />
            <p className="muted hint">{pooled ? t('mode.poolHint') : t('draft.hint')}</p>
          </div>
          {mode === 'all_random' ? (
            <section className="suggestions" aria-label={t('draft.suggestions')}>
              <p className="muted">{t('mode.allRandom')}</p>
              {board.you !== null && (
                <button type="button" className="start-match" disabled={match.starting !== null} onClick={() => match.start(board.you!)}>
                  {match.starting === board.you ? t('draft.starting') : t('draft.start', { hero: byId.get(board.you)?.name ?? `#${board.you}` })}
                </button>
              )}
              {match.error && <p role="alert">{t('draft.startError', { error: match.error })}</p>}
            </section>
          ) : (
            <SuggestionList {...ranking} byId={byId} onStart={match.start} starting={match.starting} startError={match.error} turbo={mode === 'turbo'} hasBans={mode === 'captains_mode' || mode === 'captains_draft'} />
          )}
        </div>
      )}
      {picking && catalog && (
        <HeroPicker
          title={pickerTitle(picking.side)}
          heroes={catalog.heroes.filter((h: Hero) => !taken.has(h.id) && !(picking.side === 'pool' && board.pool.includes(h.id)))}
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

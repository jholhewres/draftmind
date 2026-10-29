import { useEffect, useMemo, useState } from 'react'
import { fetchHeroes, type Hero, type HeroCatalog, type StreamRequest } from '../api'
import { useI18n } from '../i18n'
import BackButton from '../shell/BackButton'
import HeroPicker from './HeroPicker'
import { sideLabel as label, type Side } from './labels'
import { Row, SlotButton } from './slots'
import RoleOptions from './RoleOptions'
import { useStartMatch } from '../coach/useStartMatch'
import SuggestionList from './Suggestions'
import { useSuggestStream } from './useSuggestStream'

type Board = { ally: (number | null)[]; enemy: (number | null)[]; ban: number[] }
type Slot = { side: Side; index: number }

// The player's own slot is the one suggested, so four allies; ranked games
// ban at most ten heroes.
const ALLIES = 4
const ENEMIES = 5
const MAX_BANS = 10

const emptyBoard = (): Board => ({ ally: Array(ALLIES).fill(null), enemy: Array(ENEMIES).fill(null), ban: [] })

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

  const request = useMemo((): StreamRequest => ({ kind: 'draft', draft: input, lang: locale }), [input, locale])
  const ranking = useSuggestStream(request)
  const match = useStartMatch(input)

  const byId = useMemo(() => new Map((catalog?.heroes ?? []).map((h) => [h.id, h])), [catalog])
  const taken = new Set([...input.allies, ...input.enemies, ...input.bans])

  const slotButton = (side: Side, index: number, id: number | null) => (
    <SlotButton
      key={`${side}-${index}`}
      side={side}
      heroId={id}
      hero={id === null ? undefined : byId.get(id)}
      onClick={() => setPicking({ side, index })}
    />
  )

  const pickingId = picking ? (picking.side === 'ban' ? board.ban[picking.index] : board[picking.side][picking.index]) : undefined

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
            <RoleOptions name="role" role={role} onChange={setRole} />
            <p className="muted hint">{t('draft.hint')}</p>
          </div>
          <SuggestionList {...ranking} byId={byId} onStart={match.start} starting={match.starting} startError={match.error} />
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

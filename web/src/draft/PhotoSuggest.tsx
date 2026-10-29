import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { fetchHeroes, type DraftInput, type HeroCatalog, type ReadSlot, type StreamRead, type StreamRequest } from '../api'
import { useI18n } from '../i18n'
import HeroPicker from './HeroPicker'
import { prepareImage } from './prepare'
import { sideLabel } from './labels'
import { Row, SlotButton } from './slots'
import RoleOptions from './RoleOptions'
import { useStartMatch } from '../coach/useStartMatch'
import SuggestionList from './Suggestions'
import { useSuggestStream } from './useSuggestStream'

type Props = {
  photo: File
  // Downscales the photo and strips its metadata before upload.
  prepare?: (photo: File) => Promise<Blob>
  onNewPhoto: () => void
}

type Team = 'radiant' | 'dire'

// The ten read slots, radiant 0-4 then dire 5-9, as the player fixes them.
type Slots = ReadSlot[]

const ALLIES = 4
const sideOf = (team: Team) => (team === 'radiant' ? [0, 1, 2, 3, 4] : [5, 6, 7, 8, 9])
// Your team's first empty slot is yours, else its first unread one; the
// other four are your allies.
function seats(slots: Slots | null, team: Team) {
  const own = sideOf(team)
  const enemyAt = sideOf(team === 'radiant' ? 'dire' : 'radiant')
  const you = slots
    ? (own.find((i) => slots[i].status === 'empty') ?? own.find((i) => slots[i].status === 'unknown'))
    : undefined
  return { own, you, allyAt: own.filter((i) => i !== you).slice(0, ALLIES), enemyAt }
}

const sameIds = (a: number[], b: number[]) => a.length === b.length && a.every((id, i) => id === b[i])
const sameDraft = (a: DraftInput, b: DraftInput) =>
  a.role === b.role && sameIds(a.allies, b.allies) && sameIds(a.enemies, b.enemies) && sameIds(a.bans, b.bans)

const heroIds = (slots: Slots, at: number[]) => at.flatMap((i) => (slots[i].status === 'hero' ? [slots[i].heroId as number] : []))

// The suggestions screen for a photo: the draft read from it at the top,
// with every slot the reader could not read flagged, and the suggestions
// below. One stream reads the photo and ranks the draft the server seats
// the player in; fixing a slot, the team or the position streams the new
// draft, and the list updates in place.
export default function PhotoSuggest({ photo, prepare = prepareImage, onNewPhoto }: Props) {
  const { t, locale } = useI18n()
  const [catalog, setCatalog] = useState<HeroCatalog | null>(null)
  const [slots, setSlots] = useState<Slots | null>(null)
  const [team, setTeam] = useState<Team>('radiant')
  const [role, setRole] = useState(1)
  const [picking, setPicking] = useState<number | null>(null)

  useEffect(() => {
    const ctrl = new AbortController()
    fetchHeroes(ctrl.signal)
      .then(setCatalog)
      .catch(() => {}) // the strip falls back to hero ids
    return () => ctrl.abort()
  }, [])

  // The photo is read once, with the team, position and language set when
  // its upload starts.
  const [photoRequest, setPhotoRequest] = useState<StreamRequest | null>(null)
  const [prepareError, setPrepareError] = useState<string | null>(null)
  const atUpload = useRef({ team, role, lang: locale as string })
  useEffect(() => {
    atUpload.current = { team, role, lang: locale }
  })
  useEffect(() => {
    let live = true
    prepare(photo)
      .then((image) => live && setPhotoRequest({ kind: 'photo', image, ...atUpload.current }))
      .catch((e: unknown) => live && setPrepareError(e instanceof Error ? e.message : String(e)))
    return () => {
      live = false
    }
  }, [photo, prepare])

  // The draft the photo's stream ranked, while the screen still shows it.
  const [seated, setSeated] = useState<DraftInput | null>(null)
  const onRead = useCallback((read: StreamRead) => {
    setSlots([...read.radiant, ...read.dire])
    setSeated(read.draft)
  }, [])

  const { own, you, allyAt, enemyAt } = useMemo(() => seats(slots, team), [slots, team])
  const input = useMemo(
    () => (slots ? { allies: heroIds(slots, allyAt), enemies: heroIds(slots, enemyAt), bans: [], role } : null),
    [slots, allyAt, enemyAt, role],
  )
  const draftRequest = useMemo((): StreamRequest | null => (input ? { kind: 'draft', draft: input, lang: locale } : null), [input, locale])
  // Any change the player makes streams the new draft; going back to the
  // seated draft never reads the photo again.
  const followsPhoto = !input || (seated !== null && sameDraft(input, seated))
  const changeTeam = () => {
    setSeated(null)
    setTeam((s) => (s === 'radiant' ? 'dire' : 'radiant'))
  }
  const changeRole = (r: number) => {
    setSeated(null)
    setRole(r)
  }
  const ranking = useSuggestStream(followsPhoto ? photoRequest : draftRequest, onRead)
  const match = useStartMatch(input)
  const readError = prepareError ?? (slots ? null : ranking.error)
  const byId = useMemo(() => new Map((catalog?.heroes ?? []).map((h) => [h.id, h])), [catalog])

  const unread = slots ? [...allyAt, ...enemyAt].filter((i) => slots[i].status === 'unknown').length : 0
  const taken = new Set(input ? [...input.allies, ...input.enemies] : [])

  const slotAt = (side: 'ally' | 'enemy', i: number) => {
    const s = slots![i]
    const id = s.status === 'hero' ? (s.heroId as number) : null
    return (
      <SlotButton
        key={i}
        side={side}
        heroId={id}
        hero={id === null ? undefined : byId.get(id)}
        flagged={s.status === 'unknown'}
        onClick={() => setPicking(i)}
      />
    )
  }

  const set = (i: number, s: ReadSlot) => {
    setSeated(null)
    setSlots((cur) => cur && cur.map((old, j) => (j === i ? s : old)))
  }
  const pickingSide = picking !== null && enemyAt.includes(picking) ? 'enemy' : 'ally'

  return (
    <section className="screen draft photo-suggest">
      <header className="screen-head with-back">
        <button type="button" className="back" aria-label={t('photo.back')} onClick={onNewPhoto}>
          <svg aria-hidden="true" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M19 12H5M12 19l-7-7 7-7" />
          </svg>
        </button>
        <h1>{t('draft.suggestions')}</h1>
      </header>
      <div className="draft-layout">
        <div className="draft-board">
          <section className="board read-draft" aria-label={t('photo.read')}>
            {readError ? (
              <>
                <p role="alert">{t('photo.readError', { error: readError })}</p>
                <a className="chip" href="#/draft/manual">
                  {t('photo.manual')}
                </a>
              </>
            ) : !slots ? (
              <p role="status">{t('photo.reading')}</p>
            ) : (
              <>
                <Row side="ally" title={t('photo.yourTeam')}>
                  {own.map((i) =>
                    i === you ? (
                      <span key={i} className="slot you">
                        {t('draft.you')}
                      </span>
                    ) : allyAt.includes(i) ? (
                      slotAt('ally', i)
                    ) : null,
                  )}
                </Row>
                <Row side="enemy" title={t('photo.enemy')}>
                  {enemyAt.map((i) => slotAt('enemy', i))}
                </Row>
                {unread > 0 && (
                  <p className="unread-note">{t(new Intl.PluralRules(locale).select(unread) === 'one' ? 'photo.unread.one' : 'photo.unread.other', { count: unread })}</p>
                )}
                <button type="button" className="chip swap" onClick={changeTeam}>
                  {t('photo.swap')}
                </button>
              </>
            )}
          </section>
          <RoleOptions name="photo-role" role={role} onChange={changeRole} />
        </div>
        {slots && <SuggestionList {...ranking} byId={byId} onStart={match.start} starting={match.starting} startError={match.error} />}
      </div>
      {picking !== null && slots && catalog && (
        <HeroPicker
          title={t(sideLabel[pickingSide].choose)}
          heroes={catalog.heroes.filter((h) => !taken.has(h.id))}
          filled={slots[picking].status !== 'empty'} // an unread slot may turn out empty
          onPick={(id) => {
            set(picking, { heroId: id, status: 'hero', confidence: 1 })
            setPicking(null)
          }}
          onRemove={() => {
            set(picking, { status: 'empty', confidence: 1 })
            setPicking(null)
          }}
          onClose={() => setPicking(null)}
        />
      )}
    </section>
  )
}

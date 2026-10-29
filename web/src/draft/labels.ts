import type { MessageKey } from '../locales/en'

export type Side = 'ally' | 'enemy' | 'ban'

// Each side's row title, slot labels and picker title.
export const sideLabel: Record<Side, { row: MessageKey; add: MessageKey; slot: MessageKey; choose: MessageKey }> = {
  ally: { row: 'draft.allies', add: 'draft.add.ally', slot: 'draft.slot.ally', choose: 'draft.choose.ally' },
  enemy: { row: 'draft.enemies', add: 'draft.add.enemy', slot: 'draft.slot.enemy', choose: 'draft.choose.enemy' },
  ban: { row: 'draft.bans', add: 'draft.add.ban', slot: 'draft.slot.ban', choose: 'draft.choose.ban' },
}

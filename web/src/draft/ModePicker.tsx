import { gameModes, type GameMode } from '../api'
import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'

const modeLabel: Record<GameMode, MessageKey> = {
  all_pick_ranked: 'mode.all_pick_ranked',
  all_pick: 'mode.all_pick',
  turbo: 'mode.turbo',
  captains_mode: 'mode.captains_mode',
  captains_draft: 'mode.captains_draft',
  single_draft: 'mode.single_draft',
  random_draft: 'mode.random_draft',
  all_random: 'mode.all_random',
}

// The game mode of the draft. Ability Draft and custom games are listed as
// not supported, so a player looking for them knows why.
export default function ModePicker({ mode, onChange }: { mode: GameMode; onChange: (m: GameMode) => void }) {
  const { t } = useI18n()
  return (
    <div className="mode-picker">
      <label>
        <span className="row-label">{t('mode.label')}</span>
        <select value={mode} onChange={(e) => onChange(e.target.value as GameMode)}>
          {gameModes.map((m) => (
            <option key={m} value={m}>
              {t(modeLabel[m])}
            </option>
          ))}
          <option value="ability_draft" disabled>
            {t('mode.ability_draft')}
          </option>
          <option value="custom" disabled>
            {t('mode.custom')}
          </option>
        </select>
      </label>
      <p className="muted mode-note">{t('mode.unsupported')}</p>
    </div>
  )
}

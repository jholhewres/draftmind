import { useI18n } from '../i18n'
import type { MessageKey } from '../locales/en'

const roles = [1, 2, 3, 4, 5] as const
const roleName: Record<(typeof roles)[number], MessageKey> = {
  1: 'draft.position.1',
  2: 'draft.position.2',
  3: 'draft.position.3',
  4: 'draft.position.4',
  5: 'draft.position.5',
}

type Props = { name: string; role: number; onChange: (role: number) => void }

// The player's position, 1 to 5: each button shows the number small above
// the role's name.
export default function RoleOptions({ name, role, onChange }: Props) {
  const { t } = useI18n()
  return (
    <fieldset className="roles">
      <legend className="sr-only">{t('draft.role')}</legend>
      <div className="role-options">
        {roles.map((r) => (
          <label key={r} className="role-option">
            <input
              type="radio"
              name={name}
              value={r}
              checked={role === r}
              onChange={() => onChange(r)}
              aria-label={t('draft.pos', { n: r, name: t(roleName[r]) })}
            />
            <span className="role-num" aria-hidden="true">
              {r}
            </span>
            <span className="role-name" aria-hidden="true">
              {t(roleName[r])}
            </span>
          </label>
        ))}
      </div>
    </fieldset>
  )
}

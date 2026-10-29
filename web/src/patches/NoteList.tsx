import type { PatchNote } from '../api'
import { useI18n } from '../i18n'

// A patch's notes, each run of notes on one ability or on the talents
// under its name.
export default function NoteList({ notes }: { notes: PatchNote[] }) {
  const { t } = useI18n()
  const groups: { label: string; notes: PatchNote[] }[] = []
  for (const n of notes) {
    const label = n.kind === 'ability' ? (n.ability ?? '') : n.kind === 'talent' ? t('hero.talents') : ''
    const last = groups.at(-1)
    if (last && last.label === label) last.notes.push(n)
    else groups.push({ label, notes: [n] })
  }
  return (
    <div className="notes">
      {groups.map((g, i) => (
        <div key={i} className="note-group">
          {g.label && <span className="note-label">{g.label}</span>}
          <ul>
            {g.notes.map((n, j) => (
              <li key={j} className={n.indent > 1 ? 'indent' : undefined}>
                {n.text}
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  )
}

import type { BudgetUse } from '../api'
import { useI18n } from '../i18n'

// How much of the model spend cap the live session used; past the cap the
// code ranking and templates keep answering.
// A refusal over any cap (the client's may run out before the session's)
// shows as used up.
export default function BudgetMeter({ budget, refused = false }: { budget?: BudgetUse; refused?: boolean }) {
  const { t, locale } = useI18n()
  if (!budget || budget.cap <= 0) return null
  const usd = (v: number) => new Intl.NumberFormat(locale, { style: 'currency', currency: 'USD', maximumFractionDigits: 2 }).format(v)
  const out = refused || budget.spent >= budget.cap
  return (
    <span className={out ? 'live-budget out' : 'live-budget'}>
      <meter min={0} max={budget.cap} value={Math.min(budget.spent, budget.cap)} aria-hidden="true" />
      {out ? t('live.budgetOut') : t('live.budget', { spent: usd(budget.spent), cap: usd(budget.cap) })}
    </span>
  )
}

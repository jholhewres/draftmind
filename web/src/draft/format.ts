// Formats a counter delta in win-rate points with one decimal, its sign and
// a true minus sign: +3.1, −0.6, 0.0 (pt-BR: +3,1). The arrow says which way
// it points, or is empty when it rounds to zero.
export function formatDelta(locale: string, delta: number): { arrow: '▲' | '▼' | ''; text: string } {
  const text = new Intl.NumberFormat(locale, { minimumFractionDigits: 1, maximumFractionDigits: 1, signDisplay: 'exceptZero' })
    .format(delta)
    .replace('-', '−')
  const tenths = Math.round(delta * 10)
  return { arrow: tenths > 0 ? '▲' : tenths < 0 ? '▼' : '', text }
}

export const formatScore = (locale: string, score: number) =>
  new Intl.NumberFormat(locale, { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(score)

// Formats a counter delta in win-rate points with one decimal, its sign and
// a true minus sign: +3.1, −0.6, 0.0 (pt-BR: +3,1). The arrow follows the
// sign of that text, so it is empty exactly when the text reads zero.
export function formatDelta(locale: string, delta: number): { arrow: '▲' | '▼' | ''; text: string } {
  const text = new Intl.NumberFormat(locale, { minimumFractionDigits: 1, maximumFractionDigits: 1, signDisplay: 'exceptZero' })
    .format(delta)
    .replace('-', '−')
  return { arrow: text.startsWith('+') ? '▲' : text.startsWith('−') ? '▼' : '', text }
}

export const formatScore = (locale: string, score: number) =>
  new Intl.NumberFormat(locale, { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(score)

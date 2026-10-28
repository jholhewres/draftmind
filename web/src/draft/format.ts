// Formats a counter delta in win-rate points with one decimal, its sign and
// a true minus sign: +3.1, −0.6, 0.0 (pt-BR: +3,1). The arrow follows the
// sign part of that text, so it is empty exactly when the text reads zero.
export function formatDelta(locale: string, delta: number): { arrow: '▲' | '▼' | ''; text: string } {
  const parts = new Intl.NumberFormat(locale, {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
    signDisplay: 'exceptZero',
  }).formatToParts(delta)
  const sign = parts.find((p) => p.type === 'plusSign' || p.type === 'minusSign')?.type
  const text = parts.map((p) => (p.type === 'minusSign' ? '−' : p.value)).join('')
  return { arrow: sign === 'plusSign' ? '▲' : sign === 'minusSign' ? '▼' : '', text }
}

export const formatScore = (locale: string, score: number) =>
  new Intl.NumberFormat(locale, { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(score)

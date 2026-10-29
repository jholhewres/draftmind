// The Speaker of ADR-0005: the browser's speech synthesis, free and with
// no round trip. A server voice can replace it behind this function.
export function canSpeak(): boolean {
  return typeof window !== 'undefined' && 'speechSynthesis' in window && 'SpeechSynthesisUtterance' in window
}

// Speaks text in lang (a BCP 47 tag), cutting off whatever was speaking.
export function speak(text: string, lang: string) {
  if (!canSpeak() || !text) return
  const u = new SpeechSynthesisUtterance(text)
  u.lang = lang
  window.speechSynthesis.cancel()
  window.speechSynthesis.speak(u)
}

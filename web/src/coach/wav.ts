// The rate every clip is sent at: enough for speech, and small.
export const clipRate = 16000

// Converts what MediaRecorder recorded (WebM, Ogg or MP4 by browser) to
// 16 kHz mono 16-bit WAV, the one format the server takes (ADR-0005).
export async function toWav(recorded: Blob): Promise<Blob> {
  const ctx = new AudioContext()
  let decoded: AudioBuffer
  try {
    decoded = await ctx.decodeAudioData(await recorded.arrayBuffer())
  } finally {
    void ctx.close()
  }
  const frames = Math.max(1, Math.ceil(decoded.duration * clipRate))
  const offline = new OfflineAudioContext(1, frames, clipRate)
  const src = offline.createBufferSource()
  src.buffer = decoded
  src.connect(offline.destination)
  src.start()
  return encodeWav((await offline.startRendering()).getChannelData(0), clipRate)
}

// PCM samples in -1..1 as a 16-bit mono WAV file.
export function encodeWav(samples: Float32Array, rate: number): Blob {
  const out = new DataView(new ArrayBuffer(44 + samples.length * 2))
  const text = (at: number, s: string) => [...s].forEach((c, i) => out.setUint8(at + i, c.charCodeAt(0)))
  text(0, 'RIFF')
  out.setUint32(4, 36 + samples.length * 2, true)
  text(8, 'WAVEfmt ')
  out.setUint32(16, 16, true)
  out.setUint16(20, 1, true) // PCM
  out.setUint16(22, 1, true) // mono
  out.setUint32(24, rate, true)
  out.setUint32(28, rate * 2, true)
  out.setUint16(32, 2, true)
  out.setUint16(34, 16, true)
  text(36, 'data')
  out.setUint32(40, samples.length * 2, true)
  samples.forEach((v, i) => {
    const x = Math.max(-1, Math.min(1, v))
    out.setInt16(44 + i * 2, x < 0 ? x * 0x8000 : x * 0x7fff, true)
  })
  return new Blob([out], { type: 'audio/wav' })
}

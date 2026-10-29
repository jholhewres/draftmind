// The longest side the browser sends: enough for the readers, small enough
// to upload fast on a phone.
export const MAX_SIDE = 1600

// Scale (w, h) down so the longest side is at most max.
export function fitWithin(w: number, h: number, max = MAX_SIDE): { width: number; height: number } {
  const s = Math.min(1, max / Math.max(w, h))
  return { width: Math.max(1, Math.round(w * s)), height: Math.max(1, Math.round(h * s)) }
}

// Downscales a photo to MAX_SIDE and re-encodes it as JPEG. Drawing it on a
// canvas keeps only the pixels, upright: EXIF (place, time, device) is gone.
export async function prepareImage(photo: Blob): Promise<Blob> {
  const bitmap = await createImageBitmap(photo, { imageOrientation: 'from-image' })
  const { width, height } = fitWithin(bitmap.width, bitmap.height)
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('no canvas')
  ctx.drawImage(bitmap, 0, 0, width, height)
  bitmap.close()
  return new Promise((resolve, reject) =>
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('could not encode the photo'))), 'image/jpeg', 0.9),
  )
}

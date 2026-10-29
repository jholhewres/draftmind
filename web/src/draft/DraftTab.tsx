import { useState } from 'react'
import DraftBoard from './DraftBoard'
import PhotoCapture from './PhotoCapture'
import PhotoSuggest from './PhotoSuggest'

// The Draft tab: a photo of the pick screen leads to its suggestions; the
// board by hand lives at #/draft/manual. The photo is held in memory only.
export default function DraftTab({ manual }: { manual: boolean }) {
  const [photo, setPhoto] = useState<File | null>(null)
  if (manual) return <DraftBoard />
  return photo ? <PhotoSuggest photo={photo} onNewPhoto={() => setPhoto(null)} /> : <PhotoCapture onPhoto={setPhoto} />
}

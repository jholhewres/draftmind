import { useState } from 'react'
import DraftBoard from './DraftBoard'
import LiveDraft from './LiveShare'
import PhotoCapture from './PhotoCapture'
import PhotoSuggest from './PhotoSuggest'

// The Draft tab: a photo of the pick screen, or the shared game window read
// live, leads to the suggestions; the board by hand lives at #/draft/manual.
// The photo and the frames are held in memory only.
export default function DraftTab({ manual }: { manual: boolean }) {
  const [photo, setPhoto] = useState<File | null>(null)
  const [live, setLive] = useState<MediaStream | null>(null)
  if (manual) return <DraftBoard />
  if (live) return <LiveDraft stream={live} onStop={() => setLive(null)} />
  return photo ? <PhotoSuggest photo={photo} onNewPhoto={() => setPhoto(null)} /> : <PhotoCapture onPhoto={setPhoto} onLive={setLive} />
}

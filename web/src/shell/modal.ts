import { useEffect, useRef, useState } from 'react'

// The back() that dropped the last closed modal's entry, until its popstate
// lands: a modal opened meanwhile must neither push before it nor take that
// popstate as its own browser back.
let settling: Promise<void> = Promise.resolve()

function dropEntry() {
  settling = new Promise((resolve) => {
    const done = () => {
      window.removeEventListener('popstate', done)
      window.clearTimeout(timer)
      resolve()
    }
    const timer = window.setTimeout(done, 500)
    window.addEventListener('popstate', done)
  })
  history.back()
}

const isModalEntry = () => (history.state as { dmModal?: true } | null)?.dmModal === true

// Makes the mounted modal or sheet close on Esc and on browser back (it
// adds a history entry of its own), and gives focus back to the control
// that opened it once it closes.
export function useModal(onClose: () => void) {
  const close = useRef(onClose)
  useEffect(() => {
    close.current = onClose
  })
  const [opener] = useState(() => document.activeElement as HTMLElement | null)

  useEffect(() => {
    let live = true
    let pushed = false
    let popped = false
    const onPop = () => {
      popped = true
      close.current()
    }
    // After a pending back and a tick, so a mount undone at once
    // (StrictMode) leaves no entry behind.
    void settling.then(() =>
      window.setTimeout(() => {
        if (!live) return
        history.pushState({ ...(history.state as object), dmModal: true }, '')
        pushed = true
        window.addEventListener('popstate', onPop)
      }),
    )
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      e.preventDefault()
      close.current()
    }
    document.addEventListener('keydown', onKey)
    return () => {
      live = false
      window.removeEventListener('popstate', onPop)
      document.removeEventListener('keydown', onKey)
      // Closed in the app: drop the modal's own entry.
      if (pushed && !popped && isModalEntry()) dropEntry()
      if (opener?.isConnected) opener.focus()
    }
  }, [opener])
}

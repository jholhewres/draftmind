import type { MouseEvent } from 'react'
import { goBack } from './nav'

type Props = { parent: string; label: string; className?: string }

// The back control of a screen that is not a main tab: the previous screen
// when it is this app's, else parent (a hash such as "#/heroes").
export default function BackButton({ parent, label, className = 'back' }: Props) {
  const onClick = (e: MouseEvent<HTMLAnchorElement>) => {
    if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    e.preventDefault()
    goBack(parent)
  }
  return (
    <a className={className} href={parent} aria-label={label} onClick={onClick}>
      <svg aria-hidden="true" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        <path d="M19 12H5M12 19l-7-7 7-7" />
      </svg>
    </a>
  )
}

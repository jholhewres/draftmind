import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { fetchPersona, savePersona, type Account, type Persona } from '../api'
import { PersonaContext, storedPersona, storePersona } from './persona'

// Holds the coach persona: from the server once signed in, so it follows the
// player to a new device, else from this browser. account is undefined
// while it is checked.
export default function PersonaProvider({ account, children }: { account: Account | null | undefined; children: ReactNode }) {
  const [persona, setPersona] = useState<Persona>(storedPersona)
  const signedIn = !!account

  useEffect(() => {
    if (!signedIn) return
    const ctrl = new AbortController()
    fetchPersona(ctrl.signal)
      .then(setPersona)
      .catch(() => {})
    return () => ctrl.abort()
  }, [signedIn])

  const save = useCallback(
    async (p: Persona) => {
      setPersona(p)
      if (signedIn) await savePersona(p)
      else storePersona(p)
    },
    [signedIn],
  )
  const value = useMemo(() => ({ persona, save }), [persona, save])
  return <PersonaContext value={value}>{children}</PersonaContext>
}

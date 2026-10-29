import { useEffect, useState } from 'react'
import { fetchHeroBuild, type HeroBuild } from '../api'
import { useI18n } from '../i18n'
import PhaseList from '../coach/PhaseList'
import HeroBuilds from './HeroBuilds'

// The hero's most bought items by phase on the current patch, labelled with
// their source; nothing while it loads or when no popularity is synced.
export default function PopularBuild({ slug }: { slug: string }) {
  const { t } = useI18n()
  const [build, setBuild] = useState<{ slug: string; data: HeroBuild | null } | null>(null)

  useEffect(() => {
    const ctrl = new AbortController()
    fetchHeroBuild(slug, ctrl.signal)
      .then((data) => setBuild({ slug, data }))
      .catch(() => {}) // the page stands without it
    return () => ctrl.abort()
  }, [slug])

  const data = build?.slug === slug ? build.data : null
  if (!data || data.core.every((p) => p.items.length === 0)) return null
  return (
    <>
      <section className="panel popular-build" aria-labelledby="hero-build">
        <h2 id="hero-build">{t('build.popular', { patch: data.patch })}</h2>
        <PhaseList phases={data.core} counts />
        <p className="muted build-source">{t('build.source')}</p>
      </section>
      {data.builds && data.builds.length > 0 && <HeroBuilds builds={data.builds} />}
    </>
  )
}

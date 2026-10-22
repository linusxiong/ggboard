/**
 * Landing features grid. Static and in the route chunk (not with the lazy plans): on phones its first card is
 * in the initial viewport, so it must be part of the prerendered first paint — a lazily revealed block there
 * becomes a later, larger LCP candidate. The animated spotlight cards replace the static twins after idle.
 */
import { MonitorSmartphone, Route as RouteIcon, ShieldCheck } from './icons'
import { SpotlightCard, WhenIdle } from '@/components/reactbits'
import { useT } from '@/i18n'
import { useResolvedTheme } from '@/lib/theme'

const FEATURES = [
  { key: 'protocols', icon: ShieldCheck, tint: 'bg-system-blue' },
  { key: 'rules', icon: RouteIcon, tint: 'bg-system-indigo' },
  { key: 'clients', icon: MonitorSmartphone, tint: 'bg-system-green' },
] as const

export function Features() {
  const t = useT('landing')
  const theme = useResolvedTheme()
  return (
    <section aria-labelledby="features-title" className="mx-auto max-w-6xl px-4 pb-20 sm:px-6">
      <h2 id="features-title" className="mb-8 text-center text-title1 text-label">
        {t('features.title')}
      </h2>
      <ul className="grid gap-4 md:grid-cols-3">
        {FEATURES.map(({ key, icon: Icon, tint }) => {
          const body = (
            <>
              <span
                className={`mb-4 flex size-10 items-center justify-center rounded-[12px] text-white ${tint}`}
                aria-hidden="true"
              >
                <Icon className="size-5" />
              </span>
              <h3 className="text-headline text-label">{t(`features.items.${key}.title`)}</h3>
              <p className="mt-2 text-subhead text-label-secondary">{t(`features.items.${key}.body`)}</p>
            </>
          )
          return (
            <li key={key}>
              <WhenIdle
                fallback={
                  <div className="surface-card relative isolate h-full overflow-hidden p-6">{body}</div>
                }
              >
                <SpotlightCard theme={theme} className="surface-card h-full !rounded-card !p-6">
                  {body}
                </SpotlightCard>
              </WhenIdle>
            </li>
          )
        })}
      </ul>
    </section>
  )
}

/**
 * Below-the-fold landing sections (features + plans), split from the route chunk so the
 * hero renders with the smallest possible first-load bundle. Files in `-landing/` are ignored
 * by the route generator (`-` prefix).
 */
import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { Money } from '@/components/Formatters'
import { Check, MonitorSmartphone, Route as RouteIcon, ShieldCheck } from './icons'
import { BlurText, SpotlightCard, WhenIdle } from '@/components/reactbits'
import { useT } from '@/i18n'
import { getGuestPlans, usePromise } from '@/lib/publicData'
import { PLAN_PERIODS, type Plan, type PlanPeriod } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'
import { useResolvedTheme } from '@/lib/theme'

interface SectionsProps {
  signedIn: boolean
  currency?: string | undefined
  symbol?: string | undefined
}

export default function LandingSections({ signedIn, currency, symbol }: SectionsProps) {
  return (
    <>
      <Features />
      <Plans signedIn={signedIn} currency={currency} symbol={symbol} />
    </>
  )
}

const FEATURES = [
  { key: 'protocols', icon: ShieldCheck, tint: 'bg-system-blue' },
  { key: 'rules', icon: RouteIcon, tint: 'bg-system-indigo' },
  { key: 'clients', icon: MonitorSmartphone, tint: 'bg-system-green' },
] as const

function Features() {
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
              <span className={`mb-4 flex size-10 items-center justify-center rounded-[12px] text-white ${tint}`} aria-hidden="true">
                <Icon className="size-5" />
              </span>
              <h3 className="text-headline text-label">{t(`features.items.${key}.title`)}</h3>
              <p className="mt-2 text-subhead text-label-secondary">{t(`features.items.${key}.body`)}</p>
            </>
          )
          return (
            <li key={key}>
              <WhenIdle fallback={<div className="surface-card relative isolate h-full overflow-hidden p-6">{body}</div>}>
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

function firstPrice(plan: Plan): { period: PlanPeriod; cents: number } | null {
  for (const period of PLAN_PERIODS) {
    const cents = plan.prices[period]
    if (typeof cents === 'number') return { period, cents }
  }
  return null
}

function Plans({ signedIn, currency, symbol }: SectionsProps) {
  const t = useT('landing')
  const { data: plans, pending: isPending, error } = usePromise(getGuestPlans)
  const isError = Boolean(error)

  return (
    <section id="plans" aria-labelledby="plans-title" className="scroll-mt-20 bg-elevated py-20">
      <div className="mx-auto max-w-6xl px-4 sm:px-6">
        <h2 id="plans-title" className="text-center text-title1 text-label">
          {t('plans.title')}
        </h2>
        <WhenIdle fallback={<p className="mt-2 text-center text-subhead text-label-secondary">{t('plans.subtitle')}</p>}>
          <BlurText
            text={t('plans.subtitle')}
            className="mt-2 justify-center text-center text-subhead text-label-secondary"
            delay={60}
            animateBy="words"
          />
        </WhenIdle>

        <div className="mt-10 min-h-[320px]">
          {isPending ? (
            <ul className="grid gap-4 md:grid-cols-3" aria-hidden="true">
              {[0, 1, 2].map((i) => (
                <li key={i} className="h-[320px] animate-pulse rounded-card bg-fill-quaternary" />
              ))}
            </ul>
          ) : isError || !plans || plans.length === 0 ? (
            <p className="py-16 text-center text-subhead text-label-secondary">{isError ? t('plans.error') : t('plans.empty')}</p>
          ) : (
            <ul className="grid gap-4 md:grid-cols-3">
              {plans.map((plan) => {
                const price = firstPrice(plan)
                return (
                  <li key={plan.id} className="flex flex-col rounded-card bg-canvas p-6 hairline">
                    <div className="flex items-start justify-between gap-2">
                      <h3 className="text-title3 text-label">{plan.name}</h3>
                      {plan.tags[0] ? (
                        <span className="rounded-full bg-accent-soft px-2.5 py-0.5 text-caption font-semibold text-label-blue">
                          {plan.tags[0]}
                        </span>
                      ) : null}
                    </div>
                    {price ? (
                      <p className="mt-4 flex items-baseline gap-1">
                        <Money
                          cents={price.cents}
                          currency={currency}
                          symbol={symbol}
                          trimZeros
                          className="text-large-title text-label"
                        />
                        <span className="text-subhead text-label-secondary">/ {t(`periodUnit.${price.period}`)}</span>
                      </p>
                    ) : null}
                    {plan.content ? <p className="mt-3 text-subhead text-label-secondary">{plan.content}</p> : null}
                    <ul className="mt-5 flex flex-1 flex-col gap-2 text-subhead text-label">
                      <Feature>{t('plans.traffic', { gb: plan.transfer_enable })}</Feature>
                      <Feature>
                        {plan.device_limit ? t('plans.devices', { count: plan.device_limit }) : t('plans.unlimitedDevices')}
                      </Feature>
                      <Feature>{plan.speed_limit ? t('plans.speed', { mbps: plan.speed_limit }) : t('plans.unlimitedSpeed')}</Feature>
                    </ul>
                    <Link
                      to={signedIn ? '/shop' : '/register'}
                      className={buttonClass({ fullWidth: true, className: 'mt-6' })}
                      aria-label={t('plans.choose', { name: plan.name })}
                    >
                      {t('plans.choose', { name: plan.name })}
                    </Link>
                  </li>
                )
              })}
            </ul>
          )}
        </div>
      </div>
    </section>
  )
}

function Feature({ children }: { children: ReactNode }) {
  return (
    <li className="flex items-center gap-2">
      <Check className="size-4 shrink-0 text-label-green" aria-hidden="true" />
      <span>{children}</span>
    </li>
  )
}


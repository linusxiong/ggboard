import { createFileRoute, Link } from '@tanstack/react-router'
import { lazy, type ReactNode, Suspense, useEffect } from 'react'
import { LocaleToggle } from '@/components/LocaleToggle'
import { Logo } from '@/components/Logo'
import { GradientText, ShinyText, WhenIdle } from '@/components/reactbits'
import { ThemeToggle } from '@/components/ThemeToggle'
import { useCssVar } from '@/hooks/useCssVar'
import { loadNamespaces, useT } from '@/i18n'
import { getGuestConfig, getGuestPlans, withDeadline } from '@/lib/publicData'
import { buttonClass } from '@/lib/button'
import { Features } from './-landing/Features'
import { useAuthStore } from '@/stores/auth'

export const Route = createFileRoute('/')({
  // Config + dictionary are awaited (parallel to the chunk download); plans warm in the background.
  loader: async () => {
    getGuestPlans().catch(noop) // warm the cache for the lazy plans section
    const [config] = await Promise.all([withDeadline(getGuestConfig()), loadNamespaces(['landing'])])
    return { config }
  },
  component: LandingPage,
})

const LandingSections = lazy(() => import('./-landing/Sections'))
const noop = () => undefined
const GRADIENT = ['var(--sys-blue)', 'var(--sys-indigo)', 'var(--sys-purple)', 'var(--sys-pink)']

function LandingPage() {
  const t = useT(['landing', 'core'])
  const { config } = Route.useLoaderData()
  const signedIn = useAuthStore((s) => Boolean(s.token))
  const appName = config?.app_name || t('core:appName')

  useEffect(() => {
    document.title = `${appName} — ${t('hero.eyebrow')}`
  }, [appName, t])

  return (
    <div className="flex min-h-dvh flex-col">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-[10px] focus:bg-elevated focus:px-3 focus:py-2"
      >
        {t('core:a11y.skipToContent')}
      </a>
      <header className="glass glass-border-b sticky top-0 z-30">
        <div className="mx-auto flex h-14 max-w-6xl items-center justify-between gap-3 px-4 sm:px-6">
          <Link to="/" className="rounded-[10px]">
            <Logo withText text={appName} />
          </Link>
          <nav aria-label={t('nav.plans')} className="flex items-center gap-1">
            <a
              href="#plans"
              className="hidden rounded-[10px] px-3 py-1.5 text-subhead text-label hover:bg-fill-quaternary sm:block"
            >
              {t('nav.plans')}
            </a>
            <LocaleToggle compact />
            <ThemeToggle />
            {signedIn ? (
              <Link to="/dashboard" className={buttonClass({ size: 'sm' })}>
                {t('nav.console')}
              </Link>
            ) : (
              <Link to="/login" className={buttonClass({ size: 'sm', variant: 'secondary' })}>
                {t('nav.login')}
              </Link>
            )}
          </nav>
        </div>
      </header>

      <main id="main" className="flex-1">
        <Hero signedIn={signedIn} />
        <Features />
        {/* Reserved height keeps the footer from jumping while the plans chunk loads (CLS 0). */}
        <Suspense fallback={<div className="min-h-[880px]" aria-hidden="true" />}>
          <LandingSections signedIn={signedIn} currency={config?.currency} symbol={config?.currency_symbol} />
        </Suspense>
      </main>

      <footer className="hairline-t">
        <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-2 px-4 py-8 text-footnote text-label-secondary sm:flex-row sm:px-6">
          <p>{t('footer.rights', { year: new Date().getFullYear(), name: appName })}</p>
          <div className="flex items-center gap-4">
            {config?.tos_url ? (
              <a href={config.tos_url} className="underline-offset-2 hover:underline" rel="noopener">
                {t('footer.tos')}
              </a>
            ) : null}
            <span>{t('footer.openSource')}</span>
          </div>
        </div>
      </footer>
    </div>
  )
}

function Hero({ signedIn }: { signedIn: boolean }) {
  const t = useT('landing')
  const shine = useCssVar('--label-secondary', '#636369')
  const shineHighlight = useCssVar('--label', '#000000')
  return (
    <section className="mx-auto flex max-w-4xl flex-col items-center px-4 pt-20 pb-16 text-center sm:px-6 sm:pt-28 sm:pb-24">
      <p className="mb-5 rounded-full bg-fill-quaternary px-3.5 py-1 text-footnote font-semibold">
        <WhenIdle fallback={<span className="text-label-secondary">{t('hero.eyebrow')}</span>}>
          <ShinyText text={t('hero.eyebrow')} color={shine} shineColor={shineHighlight} speed={3} delay={2} />
        </WhenIdle>
      </p>
      <h1 className="text-[2.5rem] leading-[1.08] font-bold tracking-[-0.02em] text-label sm:text-[4rem]">
        {t('hero.title')}
        <br />
        <WhenIdle fallback={<GradientFallback>{t('hero.highlight')}</GradientFallback>}>
          <GradientText colors={GRADIENT} animationSpeed={10}>
            {t('hero.highlight')}
          </GradientText>
        </WhenIdle>
      </h1>
      <p className="mt-6 max-w-2xl text-body text-label-secondary sm:text-title3 sm:font-normal">
        {t('hero.subtitle')}
      </p>
      <div className="mt-10 flex flex-wrap items-center justify-center gap-3">
        <Link
          to={signedIn ? '/dashboard' : '/register'}
          className={buttonClass({ size: 'lg', className: 'px-6' })}
        >
          {t('hero.ctaPrimary')}
        </Link>
        {signedIn ? null : (
          <Link to="/login" className={buttonClass({ size: 'lg', variant: 'secondary', className: 'px-6' })}>
            {t('hero.ctaSecondary')}
          </Link>
        )}
      </div>
    </section>
  )
}

/** Static twin of <GradientText> (same box) shown until the animated one loads. */
function GradientFallback({ children }: { children: ReactNode }) {
  return (
    <span className="relative isolate inline-flex max-w-full items-center justify-center">
      <span className="relative inline-block">
        <span
          className="inline-block bg-clip-text [-webkit-background-clip:text] [-webkit-text-fill-color:transparent]"
          style={{ backgroundImage: `linear-gradient(90deg, ${GRADIENT.join(', ')}, ${GRADIENT[0]})` }}
        >
          {children}
        </span>
      </span>
    </span>
  )
}

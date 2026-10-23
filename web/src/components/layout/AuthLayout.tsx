import { Link } from '@tanstack/react-router'
import { type ReactNode, useEffect } from 'react'
import { LocaleToggle } from '@/components/LocaleToggle'
import { Logo } from '@/components/Logo'
import { ThemeToggle } from '@/components/ThemeToggle'
import { useT } from '@/i18n'

interface AuthLayoutProps {
  title: string
  subtitle?: ReactNode
  children: ReactNode
  /** Links under the card ("No account? Create one"). */
  footer?: ReactNode
  /** Large symbol above the title (status cards such as "sign-ups are closed"). */
  icon?: ReactNode
}

/** Centered card layout for login / register / forgot password. */
export function AuthLayout({ title, subtitle, children, footer, icon }: AuthLayoutProps) {
  const t = useT('core')
  useEffect(() => {
    document.title = `${title} · ${t('appName')}`
  }, [title, t])

  return (
    <div className="flex min-h-dvh flex-col">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-[10px] focus:bg-elevated focus:px-3 focus:py-2"
      >
        {t('a11y.skipToContent')}
      </a>
      {/* Same box as the landing header's inner row (routes/index.tsx), so the logo and toggles don't jump
          when moving between the landing page and sign-in / sign-up. */}
      <header className="mx-auto flex h-14 w-full max-w-6xl items-center justify-between gap-3 px-4 sm:px-6">
        <Link to="/" className="rounded-[10px]" aria-label={t('appName')}>
          <Logo withText text={t('appName')} />
        </Link>
        <div className="flex items-center gap-1">
          <LocaleToggle compact />
          <ThemeToggle />
        </div>
      </header>
      <main
        id="main"
        className="flex flex-1 items-start justify-center px-4 pt-[6vh] pb-16 sm:items-center sm:pt-0"
      >
        <div className="w-full max-w-[420px]">
          <div className="surface-card px-6 py-8 sm:px-8">
            <div className="mb-6 flex flex-col items-center text-center">
              {icon ? (
                <span
                  className="mb-4 flex size-16 items-center justify-center rounded-full bg-fill-tertiary text-label-secondary [&_svg]:size-8"
                  aria-hidden="true"
                >
                  {icon}
                </span>
              ) : null}
              <h1 className="text-title1 text-label">{title}</h1>
              {subtitle ? <p className="mt-2 text-subhead text-label-secondary">{subtitle}</p> : null}
            </div>
            {children}
          </div>
          {footer ? <div className="mt-6 text-center text-subhead text-label-secondary">{footer}</div> : null}
        </div>
      </main>
    </div>
  )
}

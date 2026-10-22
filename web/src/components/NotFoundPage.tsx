import { Link } from '@tanstack/react-router'
import { Compass } from 'lucide-react'
import { useEffect } from 'react'
import { useT } from '@/i18n'
import { buttonClass } from '@/lib/button'
import { EmptyState } from './EmptyState'

/** 404 — used as the router's default `notFoundComponent`. */
export default function NotFoundPage() {
  const t = useT('common')
  useEffect(() => {
    document.title = `404 · ${t('states.notFound')}`
  }, [t])
  return (
    <main id="main" className="flex min-h-[70dvh] items-center justify-center px-4">
      <EmptyState
        icon={<Compass className="size-7" strokeWidth={1.75} />}
        title={<span className="text-title2">{t('states.notFound')}</span>}
        description={t('states.notFoundHint')}
        action={
          <Link to="/" className={buttonClass({ variant: 'primary' })}>
            {t('actions.goHome')}
          </Link>
        }
      />
    </main>
  )
}

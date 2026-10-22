import { Link } from '@tanstack/react-router'
import { BookOpen, ChevronRight, Download, ShoppingBag } from 'lucide-react'
import type { ReactNode } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { useT } from '@/i18n'

function ActionLink({
  to,
  icon,
  children,
}: {
  to: '/subscription' | '/shop' | '/knowledge'
  icon: ReactNode
  children: ReactNode
}) {
  return (
    <Link
      to={to}
      className="flex items-center gap-3 rounded-[12px] px-3 py-2.5 text-subhead text-label transition-colors hover:bg-fill-quaternary"
    >
      <span
        className="flex size-8 items-center justify-center rounded-[10px] bg-fill-tertiary text-label-secondary"
        aria-hidden="true"
      >
        {icon}
      </span>
      <span className="flex-1">{children}</span>
      <ChevronRight className="size-4 text-label-tertiary" aria-hidden="true" />
    </Link>
  )
}

export function QuickActions({ subscribeUrl }: { subscribeUrl: string | undefined }) {
  const t = useT('dashboard')
  return (
    <section className="surface-card flex flex-col gap-3 p-6" aria-labelledby="actions-title">
      <h2 id="actions-title" className="text-headline text-label">
        {t('actions.title')}
      </h2>
      {subscribeUrl ? (
        <CopyButton value={subscribeUrl} variant="primary" size="md" className="w-full">
          {t('actions.copyLink')}
        </CopyButton>
      ) : null}
      <nav aria-label={t('actions.title')} className="-mx-3 flex flex-col">
        <ActionLink to="/subscription" icon={<Download className="size-4" />}>
          {t('actions.import')}
        </ActionLink>
        <ActionLink to="/shop" icon={<ShoppingBag className="size-4" />}>
          {t('actions.shop')}
        </ActionLink>
        <ActionLink to="/knowledge" icon={<BookOpen className="size-4" />}>
          {t('actions.docs')}
        </ActionLink>
      </nav>
    </section>
  )
}

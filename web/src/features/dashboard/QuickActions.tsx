import { Link } from '@tanstack/react-router'
import { BookOpen, Check, ChevronRight, Copy, Download, ShoppingBag } from 'lucide-react'
import { type ReactNode, useEffect, useRef, useState } from 'react'
import { useT } from '@/i18n'
import { copyText } from '@/lib/clipboard'
import { cx } from '@/lib/cn'
import { toast } from '@/lib/toast'

const ROW =
  'flex w-full items-center gap-3 px-5 py-2.5 text-start text-subhead text-label transition-colors hover:bg-fill-quaternary active:bg-fill-tertiary'

/** Settings-style symbol badge: white glyph on a colored squircle. */
function Badge({ tint, children }: { tint: string; children: ReactNode }) {
  return (
    <span
      className={cx('flex size-[30px] shrink-0 items-center justify-center rounded-[8px] text-white', tint)}
      aria-hidden="true"
    >
      {children}
    </span>
  )
}

function ActionLink({
  to,
  icon,
  tint,
  children,
}: {
  to: '/subscription' | '/shop' | '/knowledge'
  icon: ReactNode
  tint: string
  children: ReactNode
}) {
  return (
    <li>
      <Link to={to} className={ROW}>
        <Badge tint={tint}>{icon}</Badge>
        <span className="flex-1">{children}</span>
        <ChevronRight className="size-4 text-label-secondary" aria-hidden="true" />
      </Link>
    </li>
  )
}

function CopyRow({ value, children }: { value: string; children: ReactNode }) {
  const t = useT('common')
  const [copied, setCopied] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(timer.current), [])
  return (
    <li>
      <button
        type="button"
        className={ROW}
        onClick={() =>
          void copyText(value).then((ok) => {
            if (!ok) {
              toast.error(t('copy.failed'))
              return
            }
            toast.success(t('copy.success'))
            setCopied(true)
            window.clearTimeout(timer.current)
            timer.current = window.setTimeout(() => setCopied(false), 1500)
          })
        }
      >
        <Badge tint="bg-system-blue">
          {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
        </Badge>
        <span className="flex-1 font-semibold text-label-blue">{children}</span>
        <span className="text-footnote text-label-secondary">
          {copied ? t('actions.copied') : t('actions.copy')}
        </span>
      </button>
    </li>
  )
}

/** Grouped list (iOS inset style): copy the subscription link, import, shop, docs. */
export function QuickActions({ subscribeUrl }: { subscribeUrl: string | undefined }) {
  const t = useT('dashboard')
  return (
    <section className="surface-card flex h-full flex-col overflow-hidden" aria-labelledby="actions-title">
      <h2 id="actions-title" className="px-5 pt-5 pb-2 text-headline text-label">
        {t('actions.title')}
      </h2>
      <nav aria-labelledby="actions-title" className="pb-2">
        <ul className="separated flex flex-col [--separator-inset:62px]">
          {subscribeUrl ? <CopyRow value={subscribeUrl}>{t('actions.copyLink')}</CopyRow> : null}
          <ActionLink to="/subscription" tint="bg-system-green" icon={<Download className="size-4" />}>
            {t('actions.import')}
          </ActionLink>
          <ActionLink to="/shop" tint="bg-system-orange" icon={<ShoppingBag className="size-4" />}>
            {t('actions.shop')}
          </ActionLink>
          <ActionLink to="/knowledge" tint="bg-system-indigo" icon={<BookOpen className="size-4" />}>
            {t('actions.docs')}
          </ActionLink>
        </ul>
      </nav>
    </section>
  )
}

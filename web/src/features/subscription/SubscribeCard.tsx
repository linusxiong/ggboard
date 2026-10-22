/**
 * Subscription link: copy, QR (lazy), one-click import deep links, copy-only clients and "use a template without
 * saving" (adds `profile=` to the link). FE-S: one card per subscription profile; reset/rotate live in AccessCard.
 */
import { Button } from '@heroui/react'
import { QrCode } from 'lucide-react'
import { lazy, type ReactNode, Suspense, useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { useLocale, useT } from '@/i18n'
import type { RulesetCatalog } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'
import { OptionSelect } from './controls'
import { COPY_ONLY_CLIENTS, IMPORT_CLIENTS, importLink } from './importLinks'
import { localized } from './ruleProfile'

const QRCode = lazy(() => import('@/components/QRCode').then((m) => ({ default: m.QRCode })))

/** `""` = the saved rule profile; otherwise a template id applied per request. */
export const SAVED_PROFILE = ''

export function SubscribeCard({
  url,
  appName,
  catalog,
  override,
  onOverride,
  allowOverride,
  actions,
}: {
  /** Subscription URL including the override query (when chosen). */
  url: string
  appName: string
  catalog: RulesetCatalog | undefined
  override: string
  onOverride: (template: string) => void
  allowOverride: boolean
  /** FE-S: header actions (the reset moved to the security card). */
  actions?: ReactNode
}) {
  const t = useT(['subscription', 'common'])
  const locale = useLocale()
  const [showQr, setShowQr] = useState(false)
  const profileOptions = [
    { value: SAVED_PROFILE, label: t('link.savedProfile') },
    ...(catalog?.templates ?? []).map((tpl) => ({
      value: tpl.id,
      label: t('link.templateOnce', { name: localized(tpl.name, locale, tpl.id) }),
    })),
  ]

  return (
    <section className="surface-card flex flex-col gap-6 p-6" aria-labelledby="link-title">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 id="link-title" className="text-title3 text-label">
            {t('link.title')}
          </h2>
          <p className="mt-1 text-subhead text-label-secondary">{t('link.subtitle')}</p>
        </div>
        {actions}
      </div>

      <div className="flex flex-col gap-2">
        <label htmlFor="subscribe-url" className="text-subhead font-medium text-label">
          {t('link.url')}
        </label>
        <div className="flex flex-wrap items-stretch gap-2">
          <input
            id="subscribe-url"
            readOnly
            value={url}
            onFocus={(e) => e.currentTarget.select()}
            className="min-w-0 flex-1 basis-full truncate rounded-input bg-fill-tertiary px-3 py-2 font-mono text-footnote text-label outline-none focus-visible:ring-2 focus-visible:ring-focus sm:basis-0"
          />
          <CopyButton value={url} variant="primary" size="md">
            {t('common:actions.copy')}
          </CopyButton>
          <Button
            variant="secondary"
            size="md"
            aria-expanded={showQr}
            aria-controls="subscribe-qr"
            onPress={() => setShowQr((v) => !v)}
          >
            <QrCode className="size-4" aria-hidden="true" />
            {showQr ? t('link.hideQr') : t('common:actions.showQr')}
          </Button>
        </div>
        {allowOverride ? (
          <OptionSelect
            label={t('link.rules')}
            value={override}
            options={profileOptions}
            onChange={onOverride}
            className="max-w-sm"
          />
        ) : null}
        <p className="text-footnote text-label-secondary">
          {override ? t('link.overrideHint') : t('link.savedHint')}
        </p>
      </div>

      {showQr ? (
        <div
          id="subscribe-qr"
          className="flex flex-col items-center gap-2 sm:flex-row sm:items-center sm:gap-4"
        >
          <Suspense
            fallback={<div className="size-[192px] animate-pulse rounded-[12px] bg-fill-quaternary" />}
          >
            <QRCode value={url} size={192} label={t('link.qrLabel')} />
          </Suspense>
          <p className="max-w-xs text-footnote text-label-secondary">{t('link.qrHint')}</p>
        </div>
      ) : null}

      <div className="flex flex-col gap-3">
        <h3 className="text-headline text-label">{t('import.title')}</h3>
        <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          {IMPORT_CLIENTS.map((client) => (
            <li key={client.id}>
              <a
                href={importLink(client.id, url, appName)}
                className={buttonClass({
                  variant: 'secondary',
                  fullWidth: true,
                  className: 'h-auto! flex-col! items-start! gap-0! py-2.5! text-start',
                })}
              >
                <span className="text-subhead font-semibold">
                  {t('import.openIn', { client: client.name })}
                </span>
                <span className="text-caption font-normal text-label-secondary">{client.platforms}</span>
              </a>
            </li>
          ))}
        </ul>
        <p className="text-footnote text-label-secondary">
          {t('import.copyOnly', { clients: COPY_ONLY_CLIENTS.join(' · ') })}
        </p>
      </div>
    </section>
  )
}

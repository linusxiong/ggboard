/**
 * Renders a checkout result (docs/PAYMENTS.md): `qrcode` → QR + amount + "I have paid";
 * `html` → the gateway's page in a sandboxed iframe (no same-origin access to the app);
 * `url` → "redirecting…" with a manual link (the redirect itself happens in the page).
 */
import { Button } from '@heroui/react'
import { ExternalLink, RefreshCw } from 'lucide-react'
import { QRCode } from '@/components/QRCode'
import { useT } from '@/i18n'
import type { CheckoutResult } from '@/lib/api/types'
import { SiteMoney } from '@/features/shop/SiteMoney'

interface CheckoutResultViewProps {
  result: CheckoutResult
  /** Amount charged (cents). */
  amount: number
  methodName?: string | undefined
  onCheckNow: () => void
  isChecking?: boolean
}

export function CheckoutResultView({
  result,
  amount,
  methodName,
  onCheckNow,
  isChecking,
}: CheckoutResultViewProps) {
  const t = useT('orders')

  if (result.type === 'qrcode') {
    return (
      <div className="flex flex-col items-center gap-4 text-center">
        <QRCode value={result.data} size={208} label={t('pay.qrLabel', { method: methodName ?? '' })} />
        <div>
          <p className="text-footnote text-label-secondary">{t('pay.amount')}</p>
          <SiteMoney cents={amount} className="text-title2 text-label" />
        </div>
        <p className="max-w-xs text-subhead text-label-secondary">
          {t('pay.qrHint', { method: methodName ?? t('pay.app') })}
        </p>
        <Button variant="secondary" onPress={onCheckNow} isPending={isChecking}>
          <RefreshCw className="size-4" aria-hidden="true" />
          {t('pay.iHavePaid')}
        </Button>
      </div>
    )
  }

  if (result.type === 'html') {
    return (
      <div className="flex flex-col gap-3">
        <iframe
          title={t('pay.htmlTitle')}
          srcDoc={result.data}
          // No allow-same-origin: the gateway markup can run its form/script but never reads the app's storage.
          sandbox="allow-forms allow-scripts allow-popups allow-popups-to-escape-sandbox allow-top-navigation"
          referrerPolicy="no-referrer"
          className="h-[520px] w-full rounded-[14px] border-[0.5px] border-hairline bg-white"
        />
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-footnote text-label-secondary">{t('pay.htmlHint')}</p>
          <Button size="sm" variant="secondary" onPress={onCheckNow} isPending={isChecking}>
            <RefreshCw className="size-4" aria-hidden="true" />
            {t('pay.iHavePaid')}
          </Button>
        </div>
      </div>
    )
  }

  if (result.type === 'url') {
    return (
      <div className="flex flex-col items-center gap-3 py-4 text-center" role="status">
        <p className="text-subhead text-label">{t('pay.redirecting')}</p>
        {isHttpUrl(result.data) ? (
          <a
            href={result.data}
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 text-subhead text-label-blue underline underline-offset-2"
          >
            {t('pay.openGateway')}
            <ExternalLink className="size-4" aria-hidden="true" />
          </a>
        ) : null}
      </div>
    )
  }

  return null
}

function isHttpUrl(value: string): boolean {
  try {
    const url = new URL(value, window.location.href)
    return url.protocol === 'https:' || url.protocol === 'http:'
  } catch {
    return false
  }
}

import { Money } from '@/components/Formatters'
import { useSite } from './site'

/** `<Money>` in the site currency. */
export function SiteMoney({
  cents,
  className,
  trimZeros,
}: {
  cents: number
  className?: string
  trimZeros?: boolean
}) {
  const { currency, symbol } = useSite()
  return (
    <Money
      cents={cents}
      currency={currency}
      symbol={symbol}
      className={className}
      {...(trimZeros ? { trimZeros } : {})}
    />
  )
}

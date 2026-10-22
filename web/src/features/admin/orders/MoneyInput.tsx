import type { ReactNode } from 'react'
import { NumberInput } from '@/features/admin/settings/inputs/Controlled'
import { centsToUnits, unitsToCents } from './useCurrency'

interface MoneyInputProps {
  label: ReactNode
  /** Integer cents (`null` = empty). */
  value: number | null
  onChange: (cents: number | null) => void
  currency: string
  /** Site currency symbol (`currency_symbol`); derived from `currency` when omitted. */
  symbol?: string
  name?: string
  description?: ReactNode
  error?: string
  isRequired?: boolean
  isDisabled?: boolean
  minValue?: number
  placeholder?: string
  className?: string
}

/** Narrow currency symbol for an ISO code (`CNY` → `¥`), or the code itself. */
function symbolOf(currency: string): string {
  try {
    return (
      new Intl.NumberFormat(undefined, { style: 'currency', currency, currencyDisplay: 'narrowSymbol' })
        .formatToParts(0)
        .find((p) => p.type === 'currency')?.value ?? currency
    )
  } catch {
    return currency
  }
}

/**
 * Currency amount entered as decimal units (`15` → `¥ 15.00`) and stored as integer cents (1500).
 * The symbol is an adornment, so typing never fights the formatter.
 */
export function MoneyInput({ value, onChange, currency, symbol, minValue = 0, ...rest }: MoneyInputProps) {
  return (
    <NumberInput
      {...rest}
      value={centsToUnits(value)}
      onChange={(units) => onChange(unitsToCents(units))}
      minValue={minValue}
      step={0.01}
      prefix={symbol || symbolOf(currency)}
      formatOptions={{ minimumFractionDigits: 2, maximumFractionDigits: 2, useGrouping: false }}
    />
  )
}

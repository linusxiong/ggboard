import type { ReactNode } from 'react'
import { NumberInput } from '@/features/admin/settings/inputs/Controlled'
import { centsToUnits, unitsToCents } from './useCurrency'

interface MoneyInputProps {
  label: ReactNode
  /** Integer cents (`null` = empty). */
  value: number | null
  onChange: (cents: number | null) => void
  currency: string
  name?: string
  description?: ReactNode
  error?: string
  isRequired?: boolean
  isDisabled?: boolean
  minValue?: number
  className?: string
}

/** Currency amount entered in units (12.50) and stored as integer cents (1250). */
export function MoneyInput({ value, onChange, currency, minValue = 0, ...rest }: MoneyInputProps) {
  let formatOptions: Intl.NumberFormatOptions
  try {
    new Intl.NumberFormat(undefined, { style: 'currency', currency })
    formatOptions = { style: 'currency', currency, currencyDisplay: 'narrowSymbol' }
  } catch {
    formatOptions = { minimumFractionDigits: 0, maximumFractionDigits: 2 }
  }
  return (
    <NumberInput
      {...rest}
      value={centsToUnits(value)}
      onChange={(units) => onChange(unitsToCents(units))}
      minValue={minValue}
      step={0.01}
      steppers={false}
      formatOptions={formatOptions}
    />
  )
}

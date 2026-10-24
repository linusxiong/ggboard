/**
 * Multi-select as toggle chips (a small, fixed set of options: notification events, weekdays…).
 * Each chip is a real `<button aria-pressed>` inside a labelled group; order of the value follows
 * the option order.
 *
 *   <ChipSelect label="通知事件" options={[{ value: 'order_paid', label: '订单支付' }]} value={v} onChange={set} />
 */
import { type ReactNode, useId } from 'react'
import { HelpTip } from '@/components/HelpTip'
import { cx } from '@/lib/cn'

export interface ChipOption {
  value: string
  label: ReactNode
}

export interface ChipSelectProps {
  label: ReactNode
  options: readonly ChipOption[]
  value: readonly string[]
  onChange: (value: string[]) => void
  description?: ReactNode
  error?: string
  isDisabled?: boolean
  /** Used as the group's id (anchors / server error mapping). */
  name?: string
  className?: string
}

export function ChipSelect({
  label,
  options,
  value,
  onChange,
  description,
  error,
  isDisabled,
  name,
  className,
}: ChipSelectProps) {
  const id = useId()
  const toggle = (v: string) => {
    const set = new Set(value)
    if (set.has(v)) set.delete(v)
    else set.add(v)
    // Keep the option order (plus unknown values the backend may already hold, at the end).
    const known = options.map((o) => o.value).filter((o) => set.has(o))
    onChange([...known, ...value.filter((x) => set.has(x) && !known.includes(x))])
  }
  return (
    <div
      className={cx('gg-field', className)}
      data-invalid={error ? true : undefined}
      data-disabled={isDisabled || undefined}
    >
      {description ? (
        <span className="field-label-row">
          <span id={`${id}-label`} className="label">
            {label}
          </span>
          <HelpTip subject={typeof label === 'string' ? label : undefined}>{description}</HelpTip>
        </span>
      ) : (
        <span id={`${id}-label`} className="label">
          {label}
        </span>
      )}
      <div
        id={name}
        role="group"
        aria-labelledby={`${id}-label`}
        aria-describedby={error ? `${id}-err` : description ? `${id}-desc` : undefined}
        className="flex flex-wrap gap-2"
      >
        {options.map((o) => {
          const on = value.includes(o.value)
          return (
            <button
              key={o.value}
              type="button"
              aria-pressed={on}
              disabled={isDisabled}
              onClick={() => toggle(o.value)}
              className="gg-chip-toggle"
            >
              <svg viewBox="0 0 12 12" aria-hidden="true" className="gg-chip-toggle__check">
                <path
                  d="M2.5 6.2l2.3 2.3 4.7-5"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.75"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </svg>
              {o.label}
            </button>
          )
        })}
      </div>
      {description && !error ? (
        <p id={`${id}-desc`} className="sr-only">
          {description}
        </p>
      ) : null}
      {error ? (
        <p id={`${id}-err`} className="field-error" data-visible="true" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  )
}

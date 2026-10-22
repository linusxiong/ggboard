import { useId } from 'react'
import { cx } from '@/lib/cn'

interface LogoProps {
  /** Pixel size of the mark. */
  size?: number
  /** Show the "GG Bond" word mark next to the icon. */
  withText?: boolean
  /** Override the word mark (e.g. site `app_name`). */
  text?: string
  className?: string
}

/** Brand mark (squircle + snout) — the same artwork as `public/favicon.svg`. */
export function Logo({ size = 28, withText = false, text = 'GG Bond', className }: LogoProps) {
  const gradient = useId()
  return (
    <span className={cx('inline-flex items-center gap-2', className)}>
      <svg
        width={size}
        height={size}
        viewBox="0 0 64 64"
        aria-hidden="true"
        focusable="false"
        className="shrink-0"
      >
        <defs>
          <linearGradient id={gradient} x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stopColor="#0A84FF" />
            <stop offset="1" stopColor="#5E5CE6" />
          </linearGradient>
        </defs>
        <rect width="64" height="64" rx="15" fill={`url(#${gradient})`} />
        <ellipse cx="32" cy="34" rx="19" ry="14" fill="#fff" />
        <ellipse cx="25" cy="34" rx="4" ry="5.5" fill="#0A84FF" />
        <ellipse cx="39" cy="34" rx="4" ry="5.5" fill="#5E5CE6" />
      </svg>
      {withText ? (
        <span className="text-headline tracking-tight text-label">{text}</span>
      ) : (
        <span className="sr-only">{text}</span>
      )}
    </span>
  )
}

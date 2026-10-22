/**
 * Renders any `Payment.icon` value at a fixed size: a preset mark, an image URL, an emoji, or the
 * neutral fallback glyph (empty / unknown values, images that fail to load). Always decorative
 * (`aria-hidden`): the payment name is next to it.
 *
 *   <PaymentIcon icon={method.icon} size={32} />
 */
import { useState } from 'react'
import { cx } from '@/lib/cn'
import { FallbackGlyph } from './glyphs'
import { parsePaymentIcon } from './presets'

export type PaymentIconSize = 20 | 24 | 32 | 40

/** Box + corner radius (25 %, like the preset tiles' rx = 6 / 24). */
const BOX: Record<PaymentIconSize, string> = {
  20: 'size-5 rounded-[5px]',
  24: 'size-6 rounded-[6px]',
  32: 'size-8 rounded-[8px]',
  40: 'size-10 rounded-[10px]',
}
const EMOJI_TEXT: Record<PaymentIconSize, string> = {
  20: 'text-[14px]',
  24: 'text-[17px]',
  32: 'text-[22px]',
  40: 'text-[28px]',
}
const GLYPH: Record<PaymentIconSize, string> = {
  20: 'size-4',
  24: 'size-[18px]',
  32: 'size-6',
  40: 'size-7',
}

export interface PaymentIconProps {
  icon: string | null | undefined
  size?: PaymentIconSize
  className?: string
}

export function PaymentIcon({ icon, size = 32, className }: PaymentIconProps) {
  const parsed = parsePaymentIcon(icon)
  if (parsed.kind === 'preset' && parsed.preset) {
    const Glyph = parsed.preset.Component
    return (
      <Glyph width={size} height={size} className={cx('shrink-0', className)} data-payment-icon={parsed.id} />
    )
  }
  if (parsed.kind === 'url')
    return <UrlIcon key={parsed.src} src={parsed.src} size={size} className={className} />
  if (parsed.kind === 'emoji') {
    return (
      <span
        aria-hidden="true"
        className={cx(
          'flex shrink-0 items-center justify-center bg-fill-tertiary leading-none select-none',
          BOX[size],
          EMOJI_TEXT[size],
          className,
        )}
      >
        {parsed.text}
      </span>
    )
  }
  return <Fallback size={size} className={className} />
}

function UrlIcon({ src, size, className }: { src: string; size: PaymentIconSize; className?: string }) {
  const [failed, setFailed] = useState(false)
  if (failed) return <Fallback size={size} className={className} />
  return (
    <span
      aria-hidden="true"
      className={cx(
        'flex shrink-0 items-center justify-center overflow-hidden bg-fill-tertiary',
        BOX[size],
        className,
      )}
    >
      <img
        src={src}
        alt=""
        width={size}
        height={size}
        loading="lazy"
        decoding="async"
        referrerPolicy="no-referrer"
        className="size-full object-contain"
        onError={() => setFailed(true)}
      />
    </span>
  )
}

function Fallback({ size, className }: { size: PaymentIconSize; className?: string }) {
  return (
    <span
      aria-hidden="true"
      data-payment-icon="fallback"
      className={cx(
        'flex shrink-0 items-center justify-center bg-fill-secondary text-label-secondary',
        BOX[size],
        className,
      )}
    >
      <FallbackGlyph className={GLYPH[size]} />
    </span>
  )
}

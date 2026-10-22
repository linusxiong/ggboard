/** A card whose surface lights up around the pointer (CSS variables + one pointer handler).
 *  Original GG Bond component, GPL-3.0. */
import { useCallback, type CSSProperties, type HTMLAttributes, type PointerEvent } from 'react'
import { cx } from '@/lib/cn'
import './effects.css'

export type SpotlightCardProps = HTMLAttributes<HTMLDivElement> & {
  /** Glow colour; defaults depend on `theme`. */
  spotlightColor?: string
  /** Glow diameter in px. */
  spotlightSize?: number
  theme?: 'light' | 'dark'
}

export default function SpotlightCard({
  spotlightColor,
  spotlightSize = 420,
  theme = 'light',
  className,
  style,
  children,
  onPointerMove,
  ...rest
}: SpotlightCardProps) {
  const glow = spotlightColor ?? (theme === 'dark' ? 'rgba(10, 132, 255, 0.28)' : 'rgba(0, 122, 255, 0.16)')
  const handleMove = useCallback(
    (event: PointerEvent<HTMLDivElement>) => {
      const el = event.currentTarget
      const rect = el.getBoundingClientRect()
      el.style.setProperty('--gg-x', `${event.clientX - rect.left}px`)
      el.style.setProperty('--gg-y', `${event.clientY - rect.top}px`)
      onPointerMove?.(event)
    },
    [onPointerMove],
  )
  const vars = { '--gg-glow': glow, '--gg-size': `${spotlightSize}px`, ...style } as CSSProperties
  return (
    <div className={cx('gg-spotlight', className)} style={vars} onPointerMove={handleMove} {...rest}>
      <div className="gg-spotlight-glow" aria-hidden="true" />
      <div className="relative">{children}</div>
    </div>
  )
}

/** A text run with a highlight sweeping across it (CSS only). Original GG Bond component, GPL-3.0. */
import type { CSSProperties, ReactNode } from 'react'
import { cx } from '@/lib/cn'
import './effects.css'

export interface ShinyTextProps {
  text?: string
  children?: ReactNode
  /** Base text colour (any CSS colour; defaults to the inherited label colour). */
  color?: string
  /** Highlight colour. */
  shineColor?: string
  /** Seconds per sweep. */
  speed?: number
  /** Seconds before the first sweep. */
  delay?: number
  /** Gradient angle in degrees. */
  angle?: number
  className?: string
  style?: CSSProperties
}

export default function ShinyText({
  text,
  children,
  color = 'currentColor',
  shineColor = 'rgba(255,255,255,0.9)',
  speed = 3,
  delay = 0,
  angle = 110,
  className,
  style,
}: ShinyTextProps) {
  const vars = {
    '--gg-color': color,
    '--gg-shine': shineColor,
    '--gg-speed': `${speed}s`,
    '--gg-delay': `${delay}s`,
    '--gg-angle': `${angle}deg`,
    ...style,
  } as CSSProperties
  return (
    <span className={cx('gg-shine inline-block', className)} style={vars}>
      {text ?? children}
    </span>
  )
}

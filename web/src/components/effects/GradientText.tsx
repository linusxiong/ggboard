/** Text filled with a slowly drifting multi-stop gradient (CSS only). Original GG Bond component, GPL-3.0. */
import type { CSSProperties, ReactNode } from 'react'
import { cx } from '@/lib/cn'
import './effects.css'

export interface GradientTextProps {
  children?: ReactNode
  /** Gradient stops in order; the first one is repeated at the end so the loop is seamless. */
  colors?: string[]
  /** Seconds per full cycle. */
  animationSpeed?: number
  /** Gradient angle in degrees. */
  angle?: number
  className?: string
  style?: CSSProperties
}

export default function GradientText({
  children,
  colors = ['#007AFF', '#AF52DE', '#FF2D55'],
  animationSpeed = 8,
  angle = 90,
  className,
  style,
}: GradientTextProps) {
  const stops = colors.length > 1 ? [...colors, colors[0]] : [colors[0] ?? '#007AFF', colors[0] ?? '#007AFF']
  const vars = {
    backgroundImage: `linear-gradient(${angle}deg, ${stops.join(', ')})`,
    '--gg-speed': `${animationSpeed}s`,
    ...style,
  } as CSSProperties
  return (
    <span className="relative isolate inline-flex max-w-full items-center justify-center">
      <span className={cx('gg-gradient relative inline-block', className)} style={vars}>
        {children}
      </span>
    </span>
  )
}

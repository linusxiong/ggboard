/** Reveals text word by word (or letter by letter) with a blur-to-sharp fade once it scrolls into view.
 *  Original GG Bond component, GPL-3.0. */
import { useEffect, useRef, type CSSProperties } from 'react'
import { cx } from '@/lib/cn'
import { useInView } from './useInView'
import './effects.css'

export interface BlurTextProps {
  text?: string
  className?: string
  /** Milliseconds between consecutive items. */
  delay?: number
  /** Milliseconds each item takes. */
  duration?: number
  animateBy?: 'words' | 'letters'
  direction?: 'top' | 'bottom'
  /** Initial blur radius in px. */
  blur?: number
  onAnimationComplete?: () => void
}

export default function BlurText({
  text = '',
  className,
  delay = 80,
  duration = 600,
  animateBy = 'words',
  direction = 'top',
  blur = 8,
  onAnimationComplete,
}: BlurTextProps) {
  const ref = useRef<HTMLParagraphElement>(null)
  const visible = useInView(ref)
  const items = animateBy === 'words' ? text.split(' ') : Array.from(text)
  const total = items.length

  useEffect(() => {
    if (!visible || !onAnimationComplete) return
    const timer = window.setTimeout(onAnimationComplete, duration + delay * Math.max(0, total - 1))
    return () => window.clearTimeout(timer)
  }, [visible, onAnimationComplete, duration, delay, total])

  const vars = {
    '--gg-duration': `${duration}ms`,
    '--gg-blur': `${blur}px`,
    '--gg-shift': direction === 'top' ? '-8px' : '8px',
  } as CSSProperties
  return (
    <p ref={ref} className={cx('gg-reveal flex flex-wrap', visible && 'is-visible', className)} style={vars}>
      {items.map((item, i) => (
        <span
          key={`${i}-${item}`}
          className="gg-reveal-item"
          style={{ '--gg-item-delay': `${i * delay}ms` } as CSSProperties}
        >
          {item === ' ' ? ' ' : item}
          {animateBy === 'words' && i < total - 1 ? ' ' : null}
        </span>
      ))}
    </p>
  )
}

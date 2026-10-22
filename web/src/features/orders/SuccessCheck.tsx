/**
 * Apple-style "done" check: the circle fills, then the tick draws itself (motion, ~0.6 s).
 * Lazy-loaded by the order page; with `prefers-reduced-motion` the final state renders at once.
 */
import { domAnimation, LazyMotion, MotionConfig } from 'motion/react'
import * as m from 'motion/react-m'
import { useReducedMotion } from '@/lib/motion'

export default function SuccessCheck({ size = 72, label }: { size?: number; label: string }) {
  const reduced = useReducedMotion()
  return (
    <MotionConfig reducedMotion="user">
      <LazyMotion features={domAnimation}>
        <svg
          width={size}
          height={size}
          viewBox="0 0 72 72"
          role="img"
          aria-label={label}
          className="text-system-green"
        >
          <m.circle
            cx="36"
            cy="36"
            r="34"
            fill="currentColor"
            initial={reduced ? false : { scale: 0.6, opacity: 0 }}
            animate={{ scale: 1, opacity: 1 }}
            transition={{ duration: 0.28, ease: [0.22, 1, 0.36, 1] }}
            style={{ transformOrigin: '36px 36px' }}
          />
          <m.path
            d="M22 37.5 L31.5 47 L50 27"
            fill="none"
            stroke="white"
            strokeWidth="6"
            strokeLinecap="round"
            strokeLinejoin="round"
            initial={reduced ? false : { pathLength: 0 }}
            animate={{ pathLength: 1 }}
            transition={{ duration: 0.32, delay: reduced ? 0 : 0.22, ease: 'easeOut' }}
          />
        </svg>
      </LazyMotion>
    </MotionConfig>
  )
}

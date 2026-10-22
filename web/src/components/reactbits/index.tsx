/**
 * React Bits components — always consumed lazily so they never land in a route's initial chunk.
 *
 *   <WhenIdle fallback={<span className="...">GG Bond</span>}>
 *     <ShinyText text="GG Bond" />
 *   </WhenIdle>
 *
 * `WhenIdle` renders `fallback` until the page is idle (requestIdleCallback after `load`) and
 * motion is allowed; with `prefers-reduced-motion: reduce` the fallback stays forever.
 * Make the fallback visually identical in size to avoid layout shift (CLS 0).
 */
import { lazy, type ReactNode, Suspense } from 'react'
import { useAnimationsReady } from '@/lib/motion'

export const ShinyText = lazy(() => import('./ShinyText'))
export const GradientText = lazy(() => import('./GradientText'))
export const BlurText = lazy(() => import('./BlurText'))
export const SpotlightCard = lazy(() => import('./SpotlightCard'))
export const CountUp = lazy(() => import('./CountUp'))

export function WhenIdle({
  children,
  fallback,
  timeout,
}: {
  children: ReactNode
  fallback: ReactNode
  timeout?: number
}) {
  const ready = useAnimationsReady(timeout)
  if (!ready) return <>{fallback}</>
  return <Suspense fallback={fallback}>{children}</Suspense>
}

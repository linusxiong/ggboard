import { useEffect, useRef } from 'react'

/**
 * Calls `onSettle(value)` once `value` has stopped changing for `delay` ms (skips the first render
 * and values equal to the last settled one).
 */
export function useDebouncedEffect<T>(value: T, delay: number, onSettle: (value: T) => void) {
  const callback = useRef(onSettle)
  const last = useRef(value)
  useEffect(() => {
    callback.current = onSettle
  })
  useEffect(() => {
    if (Object.is(value, last.current)) return
    const timer = window.setTimeout(() => {
      last.current = value
      callback.current(value)
    }, delay)
    return () => window.clearTimeout(timer)
  }, [value, delay])
}

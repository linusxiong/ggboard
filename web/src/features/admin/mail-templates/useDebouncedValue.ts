import { useEffect, useState } from 'react'

/**
 * `value` once it has stopped changing for `delay` ms (the first value is returned at once). Pass a
 * memoised object: a new identity restarts the timer.
 */
export function useDebouncedValue<T>(value: T, delay: number): T {
  const [settled, setSettled] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setSettled(value), delay)
    return () => window.clearTimeout(timer)
  }, [value, delay])
  return settled
}

import { useCallback, useEffect, useState } from 'react'

const now = () => Date.now()

function read(key: string): number {
  try {
    return Number(sessionStorage.getItem(key)) || 0
  } catch {
    return 0
  }
}

/**
 * Countdown for "resend code" buttons. Survives reloads (sessionStorage) so users cannot bypass
 * the backend's 60s per-email cooldown by refreshing.
 *
 *   const { remaining, start } = useCooldown('register-code')
 *   <Button isDisabled={remaining > 0}>{remaining ? t('code.resend', { seconds: remaining }) : t('code.send')}</Button>
 */
export function useCooldown(id: string, seconds = 60) {
  const key = `gg-cooldown:${id}`
  const [endsAt, setEndsAt] = useState(() => read(key))
  const [tick, setTick] = useState(now)

  useEffect(() => {
    if (endsAt <= now()) return
    const timer = window.setInterval(() => {
      const current = now()
      setTick(current)
      if (current >= endsAt) window.clearInterval(timer)
    }, 1000)
    return () => window.clearInterval(timer)
  }, [endsAt])

  const start = useCallback(
    (duration = seconds) => {
      const end = now() + duration * 1000
      try {
        sessionStorage.setItem(key, String(end))
      } catch {
        /* ignore */
      }
      setTick(now())
      setEndsAt(end)
    },
    [key, seconds],
  )

  const remaining = Math.max(0, Math.ceil((endsAt - tick) / 1000))
  return { remaining, start, active: remaining > 0 }
}

/**
 * Toast renderer (lazy, mounted by `ToastMount` in the root route). Apple-style capsules: frosted
 * material, hairline border, soft shadow, status-tinted symbol, 15 px medium text. They spring in
 * from the top on desktop and from the bottom (above the tab bar) on phones; reduced motion → fade.
 *
 * Timers pause while the pointer or focus is inside the stack and while the page is hidden.
 */
import './toast.css'
import { type ReactNode, useCallback, useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { useT } from '@/i18n'
import { type ToastRecord, type ToastTone, toastStore } from '@/lib/toast'

const EMPTY: readonly ToastRecord[] = []
const LEAVE_MS = 200

export default function ToastHost() {
  const t = useT('core')
  const items = useSyncExternalStore(toastStore.subscribe, toastStore.getSnapshot, () => EMPTY)
  const [paused, setPaused] = useState(false)
  const [hidden, setHidden] = useState(() => typeof document !== 'undefined' && document.hidden)

  useEffect(() => {
    const onVisibility = () => setHidden(document.hidden)
    document.addEventListener('visibilitychange', onVisibility)
    return () => document.removeEventListener('visibilitychange', onVisibility)
  }, [])

  return (
    <section
      aria-label={t('a11y.notifications')}
      className="gg-toaster"
      onPointerEnter={() => setPaused(true)}
      onPointerLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget)) setPaused(false)
      }}
    >
      <div className="gg-toaster__list">
        {items.map((item) => (
          // Keyed by version: a repeated message / settled promise re-enters with a fresh timer.
          <Toast
            key={`${item.id}:${item.version}`}
            item={item}
            paused={paused || hidden}
            dismissLabel={t('a11y.dismiss')}
          />
        ))}
      </div>
    </section>
  )
}

function Toast({ item, paused, dismissLabel }: { item: ToastRecord; paused: boolean; dismissLabel: string }) {
  const [leaving, setLeaving] = useState(false)
  const remaining = useRef(item.timeout)
  const startedAt = useRef(0)

  const close = useCallback(() => {
    setLeaving(true)
    window.setTimeout(() => toastStore.dismiss(item.id), LEAVE_MS)
  }, [item.id])

  useEffect(() => {
    if (item.timeout <= 0 || paused || leaving) return
    startedAt.current = Date.now()
    const timer = window.setTimeout(close, remaining.current)
    return () => {
      window.clearTimeout(timer)
      remaining.current = Math.max(400, remaining.current - (Date.now() - startedAt.current))
    }
  }, [item.timeout, paused, leaving, close])

  return (
    <div
      role="status"
      aria-live="polite"
      aria-atomic="true"
      className="gg-toast"
      data-tone={item.tone}
      data-leaving={leaving || undefined}
      onKeyDown={(e) => {
        if (e.key === 'Escape') close()
      }}
    >
      <ToneIcon tone={item.tone} />
      <div className="gg-toast__text">
        <p className="gg-toast__title">{item.message}</p>
        {item.description ? <p className="gg-toast__description">{item.description}</p> : null}
      </div>
      <button type="button" className="gg-toast__close" aria-label={dismissLabel} onClick={close}>
        <svg viewBox="0 0 16 16" aria-hidden="true" className="size-3">
          <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" />
        </svg>
      </button>
    </div>
  )
}

/** SF Symbols–style filled glyphs (`checkmark.circle.fill`, `xmark.octagon.fill`, …), drawn inline. */
function ToneIcon({ tone }: { tone: ToastTone }) {
  if (tone === 'neutral') return null
  if (tone === 'loading') return <span className="gg-toast__spinner" aria-hidden="true" />
  const glyph: Record<Exclude<ToastTone, 'neutral' | 'loading'>, ReactNode> = {
    success: <path d="M6.2 10.3l2.5 2.5 5.1-5.6" />,
    error: <path d="M7.2 7.2l5.6 5.6M12.8 7.2l-5.6 5.6" />,
    warning: (
      <>
        <path d="M10 5.8v5" />
        <circle cx="10" cy="14" r="0.4" />
      </>
    ),
    info: (
      <>
        <circle cx="10" cy="6.2" r="0.4" />
        <path d="M10 9v5.2" />
      </>
    ),
  }
  return (
    <svg viewBox="0 0 20 20" className="gg-toast__icon" aria-hidden="true">
      <circle cx="10" cy="10" r="10" className="gg-toast__icon-bg" />
      <g fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
        {glyph[tone]}
      </g>
    </svg>
  )
}

/**
 * Prerendered snapshot hand-over (see main.tsx and scripts/prerender.mjs).
 *
 * `carryInputs(root)` (lib/snapshot.ts) reads what the visitor typed into the static snapshot before the app
 * took over; `<FirstCommit>` runs once after the first commit: restores those values (native setter + `input` event,
 *   so React state picks them up) and, while prerendering, hands the committed markup to the script.
 */
import { type ReactNode, useEffect } from 'react'
import { type Carried, restoreInputs } from '@/lib/snapshot'

export function FirstCommit({
  root,
  carried,
  prerendering,
  children,
}: {
  root: HTMLElement
  carried: Carried | null
  prerendering: boolean
  children: ReactNode
}) {
  useEffect(() => {
    if (carried && (carried.values.size > 0 || carried.focused)) restoreInputs(root, carried)
    if (!prerendering) return
    const w = window as { __GG_SNAPSHOT__?: { html: string; title: string; at: number } }
    w.__GG_SNAPSHOT__ = { html: root.innerHTML, title: document.title, at: performance.now() }
    window.dispatchEvent(new Event('gg:prerendered'))
  }, [root, carried, prerendering])
  return children
}

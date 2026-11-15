import { type RefObject, useEffect } from 'react'

/** Scroll-driven animations (Chrome 115, Safari 26, Firefox 159) move the fades in CSS, with no script per scroll. */
const NATIVE =
  typeof CSS !== 'undefined' &&
  typeof CSS.supports === 'function' &&
  CSS.supports('animation-timeline: scroll()')

/**
 * Edge fades for a horizontal strip whose scroll bar is hidden (`.gg-scroll-fade` in app.css): the content fades
 * out at an edge while there is more to scroll that way — the only cue that the strip scrolls at all.
 *
 * Marks the strip `data-scrollable` while its content overflows (the mask is applied only then). The fade widths
 * follow the scroll position through a scroll-driven animation; browsers without one get `data-fade-start` /
 * `data-fade-end` from a passive scroll listener instead. `key`: measure again when the content changes.
 */
export function useScrollFade(ref: RefObject<HTMLElement | null>, key?: unknown) {
  useEffect(() => {
    const scroller = ref.current
    if (!scroller) return
    const update = () => {
      const max = scroller.scrollWidth - scroller.clientWidth
      scroller.toggleAttribute('data-scrollable', max > 1)
      if (NATIVE) return
      scroller.toggleAttribute('data-fade-start', max > 1 && scroller.scrollLeft > 1)
      scroller.toggleAttribute('data-fade-end', max > 1 && scroller.scrollLeft < max - 1)
    }
    update()
    const observer = typeof ResizeObserver === 'function' ? new ResizeObserver(update) : null
    observer?.observe(scroller)
    if (scroller.firstElementChild) observer?.observe(scroller.firstElementChild)
    if (!NATIVE) scroller.addEventListener('scroll', update, { passive: true })
    return () => {
      observer?.disconnect()
      scroller.removeEventListener('scroll', update)
    }
  }, [ref, key])
}

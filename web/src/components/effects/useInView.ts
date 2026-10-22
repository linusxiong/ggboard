import { useEffect, useState, type RefObject } from 'react'

/** `true` once `ref` has entered the viewport (sticky). Without IntersectionObserver it is `true` immediately. */
export function useInView<T extends Element>(
  ref: RefObject<T | null>,
  rootMargin = '0px 0px -10% 0px',
): boolean {
  const [seen, setSeen] = useState(() => typeof IntersectionObserver === 'undefined')
  useEffect(() => {
    const el = ref.current
    if (!el || seen) return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          setSeen(true)
          io.disconnect()
        }
      },
      { rootMargin },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [ref, rootMargin, seen])
  return seen
}

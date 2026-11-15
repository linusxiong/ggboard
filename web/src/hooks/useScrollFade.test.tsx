import { fireEvent, render } from '@testing-library/react'
import { useRef } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

/** The hook decides once, at import, whether CSS can drive the fades: load it under a given answer. */
async function loadStrip(scrollTimelines: boolean) {
  vi.resetModules()
  vi.stubGlobal('CSS', {
    supports: (query: string) => scrollTimelines && query.includes('animation-timeline'),
  })
  const { useScrollFade } = await import('./useScrollFade')
  return function Strip({ items }: { items: number }) {
    const ref = useRef<HTMLUListElement>(null)
    useScrollFade(ref, items)
    return (
      <ul ref={ref} data-testid="strip">
        {Array.from({ length: items }, (_, i) => (
          <li key={i}>{i}</li>
        ))}
      </ul>
    )
  }
}

/** jsdom lays nothing out: give the strip a width, a content width and a scroll position. */
function size(el: HTMLElement, box: { client: number; scroll: number; left?: number }) {
  Object.defineProperty(el, 'clientWidth', { configurable: true, value: box.client })
  Object.defineProperty(el, 'scrollWidth', { configurable: true, value: box.scroll })
  el.scrollLeft = box.left ?? 0
}
const marks = (el: HTMLElement) =>
  ['data-scrollable', 'data-fade-start', 'data-fade-end'].filter((name) => el.hasAttribute(name))

describe('useScrollFade', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('marks nothing while the content fits', async () => {
    const Strip = await loadStrip(false)
    const { getByTestId } = render(<Strip items={3} />)
    expect(marks(getByTestId('strip'))).toEqual([])
  })

  it('without scroll-driven animations: fades the side with more to scroll and follows the scroll position', async () => {
    const Strip = await loadStrip(false)
    const { getByTestId, rerender } = render(<Strip items={3} />)
    const strip = getByTestId('strip')
    size(strip, { client: 300, scroll: 500 })
    rerender(<Strip items={8} />) // the content changed: measured again
    expect(marks(strip)).toEqual(['data-scrollable', 'data-fade-end'])

    size(strip, { client: 300, scroll: 500, left: 120 })
    fireEvent.scroll(strip)
    expect(marks(strip)).toEqual(['data-scrollable', 'data-fade-start', 'data-fade-end'])

    size(strip, { client: 300, scroll: 500, left: 200 })
    fireEvent.scroll(strip)
    expect(marks(strip)).toEqual(['data-scrollable', 'data-fade-start'])

    size(strip, { client: 300, scroll: 300 })
    rerender(<Strip items={2} />)
    expect(marks(strip)).toEqual([])
  })

  it('with scroll-driven animations: only says that the strip scrolls, CSS does the rest', async () => {
    const Strip = await loadStrip(true)
    const { getByTestId, rerender } = render(<Strip items={3} />)
    const strip = getByTestId('strip')
    size(strip, { client: 300, scroll: 500, left: 120 })
    rerender(<Strip items={8} />)
    fireEvent.scroll(strip)
    expect(marks(strip)).toEqual(['data-scrollable'])
  })
})

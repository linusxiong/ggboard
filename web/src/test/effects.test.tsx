import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BlurText from '@/components/effects/BlurText'
import CountUp from '@/components/effects/CountUp'
import GradientText from '@/components/effects/GradientText'
import ShinyText from '@/components/effects/ShinyText'
import SpotlightCard from '@/components/effects/SpotlightCard'

/** IntersectionObserver that reports every observed element as visible at once. */
class VisibleIO {
  private readonly cb: IntersectionObserverCallback
  constructor(cb: IntersectionObserverCallback) {
    this.cb = cb
  }
  observe(el: Element) {
    this.cb(
      [{ isIntersecting: true, target: el } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    )
  }
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return []
  }
}

describe('effects', () => {
  beforeEach(() => {
    vi.stubGlobal('IntersectionObserver', VisibleIO)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('ShinyText renders its text with the colour variables', () => {
    render(<ShinyText text="GG Bond" color="#111" shineColor="#fff" speed={2} delay={1} />)
    const el = screen.getByText('GG Bond')
    expect(el.className).toContain('gg-shine')
    expect(el.style.getPropertyValue('--gg-speed')).toBe('2s')
    expect(el.style.getPropertyValue('--gg-delay')).toBe('1s')
  })

  it('GradientText repeats the first stop for a seamless loop', () => {
    render(<GradientText colors={['#f00', '#0f0']}>Hi</GradientText>)
    const el = screen.getByText('Hi')
    expect(el.getAttribute('style')).toMatch(
      /#f00, #0f0, #f00|rgb\(255, 0, 0\), rgb\(0, 255, 0\), rgb\(255, 0, 0\)/,
    )
  })

  it('BlurText splits into words with staggered delays', () => {
    render(<BlurText text="fast simple panel" delay={50} />)
    const items = document.querySelectorAll('.gg-reveal-item')
    expect(items).toHaveLength(3)
    expect((items[2] as HTMLElement).style.getPropertyValue('--gg-item-delay')).toBe('100ms')
    expect(screen.getByText(/fast/)).toBeInTheDocument()
  })

  it('SpotlightCard renders children and forwards attributes', () => {
    render(
      <SpotlightCard data-testid="card" theme="dark" className="extra">
        <p>Body</p>
      </SpotlightCard>,
    )
    const card = screen.getByTestId('card')
    expect(card.className).toContain('gg-spotlight')
    expect(card.className).toContain('extra')
    expect(screen.getByText('Body')).toBeInTheDocument()
  })

  it('CountUp jumps to the final value when there is no duration and uses the formatter', async () => {
    render(<CountUp to={1234.5} duration={0} formatter={(v) => `¥${v.toFixed(1)}`} />)
    expect(await screen.findByText('¥1234.5')).toBeInTheDocument()
  })

  it('CountUp honours prefers-reduced-motion', async () => {
    vi.spyOn(window, 'matchMedia').mockImplementation(
      (query: string) =>
        ({
          matches: true,
          media: query,
          addEventListener: () => {},
          removeEventListener: () => {},
        }) as unknown as MediaQueryList,
    )
    render(<CountUp to={42} duration={5} separator="" />)
    expect(await screen.findByText('42')).toBeInTheDocument()
  })
})

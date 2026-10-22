import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import ToastHost from '@/components/feedback/ToastHost'
import { loadNamespaces } from '@/i18n'
import { useUiStore } from '@/stores/ui'
import { MAX_TOASTS, toast, toastStore } from './toast'

const snapshot = () => toastStore.getSnapshot()

describe('toast store', () => {
  afterEach(() => toastStore.clear())

  it('exposes one API for every status (danger is an alias of error)', () => {
    toast('plain')
    toast.success('saved')
    toast.error('failed')
    expect(snapshot().map((t) => [t.tone, t.message])).toEqual([
      ['neutral', 'plain'],
      ['success', 'saved'],
      ['error', 'failed'],
    ])
    toastStore.clear()
    toast.danger('boom')
    toast.info('fyi')
    toast.warning('careful')
    expect(snapshot().map((t) => t.tone)).toEqual(['error', 'info', 'warning'])
    expect(toastStore.isRequested()).toBe(true)
  })

  it('keeps at most three and refreshes a repeated message instead of stacking it', () => {
    toast('one')
    toast('two')
    toast('three')
    const firstVersion = snapshot()[2]!.version
    toast('three')
    expect(snapshot()).toHaveLength(3)
    expect(snapshot()[2]!.version).toBeGreaterThan(firstVersion)
    toast('four')
    expect(snapshot()).toHaveLength(MAX_TOASTS)
    expect(snapshot().map((t) => t.message)).toEqual(['two', 'three', 'four'])
  })

  it('auto-dismisses after 3–4 s by default; 0 keeps it', () => {
    const ok = toast.success('ok')
    toast.error('bad')
    toast('sticky', { timeout: 0 })
    const [a, b, c] = snapshot()
    expect(a!.timeout).toBeGreaterThanOrEqual(3000)
    expect(a!.timeout).toBeLessThanOrEqual(4000)
    expect(b!.timeout).toBeLessThanOrEqual(4000)
    expect(c!.timeout).toBe(0)
    toast.dismiss(ok)
    expect(snapshot().map((t) => t.message)).toEqual(['bad', 'sticky'])
  })

  it('promise(): a loading toast that turns into success or error in place', async () => {
    let resolve!: (v: number) => void
    const p = new Promise<number>((r) => {
      resolve = r
    })
    void toast.promise(p, { loading: 'Saving…', success: (n) => `Saved ${n}`, error: 'Nope' })
    expect(snapshot()).toMatchObject([{ tone: 'loading', message: 'Saving…', timeout: 0 }])
    const id = snapshot()[0]!.id
    resolve(3)
    await p
    await Promise.resolve()
    expect(snapshot()).toMatchObject([{ id, tone: 'success', message: 'Saved 3' }])

    const failing = Promise.reject(new Error('x'))
    void toast.promise(failing, { loading: 'Again…', success: 'ok', error: (e) => (e as Error).message })
    await failing.catch(() => undefined)
    await Promise.resolve()
    expect(snapshot().at(-1)).toMatchObject({ tone: 'error', message: 'x' })
  })
})

describe('<ToastHost/>', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'en' })
    await loadNamespaces(['core'], 'en')
  })
  afterEach(() => {
    vi.useRealTimers()
    act(() => toastStore.clear())
  })

  it('renders capsules with role=status, a status symbol and a dismiss button', async () => {
    const user = userEvent.setup()
    render(<ToastHost />)
    act(() => {
      toast.success('Profile saved', { description: 'Changes are live.' })
    })
    const status = screen.getByRole('status')
    expect(status).toHaveTextContent('Profile saved')
    expect(status).toHaveTextContent('Changes are live.')
    expect(status).toHaveAttribute('data-tone', 'success')
    expect(screen.getByRole('region', { name: 'Notifications' })).toBeInTheDocument()

    vi.useFakeTimers({ shouldAdvanceTime: true })
    await user.click(screen.getByRole('button', { name: 'Dismiss notification' }))
    act(() => {
      vi.advanceTimersByTime(250)
    })
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('auto-dismisses, but not while the pointer is over the stack', () => {
    vi.useFakeTimers()
    render(<ToastHost />)
    act(() => {
      toast.info('Heads up')
    })
    const region = screen.getByRole('region', { name: 'Notifications' })
    fireEvent.pointerEnter(region)
    act(() => {
      vi.advanceTimersByTime(10_000)
    })
    expect(screen.getByText('Heads up')).toBeInTheDocument()
    fireEvent.pointerLeave(region)
    act(() => {
      vi.advanceTimersByTime(5_000)
    })
    expect(screen.queryByText('Heads up')).not.toBeInTheDocument()
  })
})

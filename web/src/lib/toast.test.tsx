import { AlertDialog } from '@heroui/react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import ToastHost from '@/components/feedback/ToastHost'
import { LiveRegion } from '@/components/feedback/ToastMount'
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

  const capsule = (text: string) => screen.getByText(text).closest('.gg-toast')

  it('renders capsules with a status symbol and a dismiss button', async () => {
    const user = userEvent.setup()
    render(<ToastHost />)
    act(() => {
      toast.success('Profile saved', { description: 'Changes are live.' })
    })
    const toastEl = capsule('Profile saved')
    expect(toastEl).toHaveTextContent('Changes are live.')
    expect(toastEl).toHaveAttribute('data-tone', 'success')
    expect(screen.getByRole('region', { name: 'Notifications' })).toBeInTheDocument()

    vi.useFakeTimers({ shouldAdvanceTime: true })
    await user.click(screen.getByRole('button', { name: 'Dismiss notification' }))
    act(() => {
      vi.advanceTimersByTime(250)
    })
    expect(screen.queryByText('Profile saved')).not.toBeInTheDocument()
  })

  it('announces the visible text once through the live region that was there first', () => {
    vi.useFakeTimers()
    render(
      <>
        <LiveRegion />
        <ToastHost />
      </>,
    )
    const region = document.getElementById('gg-live')
    expect(region).toHaveAttribute('aria-live', 'polite')
    expect(region).toBeEmptyDOMElement()

    act(() => {
      toast.success('Profile saved', { description: 'Changes are live.' })
    })
    expect(region).toHaveTextContent('Profile saved Changes are live.')
    // The capsule is not a live region itself: one announcement, without the dismiss button's name.
    expect(capsule('Profile saved')).not.toHaveAttribute('role')
    expect(capsule('Profile saved')).not.toHaveAttribute('aria-live')
    expect(region).not.toHaveTextContent('Dismiss')

    // The same message again is announced again (a new node), and old announcements do not pile up.
    act(() => {
      toast.success('Profile saved', { description: 'Changes are live.' })
    })
    expect(region?.children).toHaveLength(2)
    act(() => {
      vi.advanceTimersByTime(10_000)
    })
    expect(region).toBeEmptyDOMElement()
  })

  it('uses document.ariaNotify where the browser has it', () => {
    const ariaNotify = vi.fn()
    Object.defineProperty(document, 'ariaNotify', { value: ariaNotify, configurable: true })
    try {
      render(
        <>
          <LiveRegion />
          <ToastHost />
        </>,
      )
      act(() => {
        toast.error('Could not save')
      })
      expect(ariaNotify).toHaveBeenCalledExactlyOnceWith('Could not save')
      expect(document.getElementById('gg-live')).toBeEmptyDOMElement()
    } finally {
      Reflect.deleteProperty(document, 'ariaNotify')
    }
  })

  it('hands focus on when a focused toast is dismissed', async () => {
    const user = userEvent.setup()
    render(
      <>
        <button type="button">Save</button>
        <ToastHost />
      </>,
    )
    const save = screen.getByRole('button', { name: 'Save' })
    save.focus()
    act(() => {
      toast.info('First')
      toast.info('Second')
    })
    const [first, second] = screen.getAllByRole('button', { name: 'Dismiss notification' })
    // Tab from the page into the stack, then dismiss: focus goes to the toast that is left…
    first!.focus()
    await user.keyboard('{Escape}')
    expect(second).toHaveFocus()
    // …and from the last one back to where the user came from, not to <body>.
    await user.keyboard('{Escape}')
    expect(save).toHaveFocus()
  })

  it('stays reachable while a dialog is open', async () => {
    // React Aria hides everything outside an open modal (inert, or aria-hidden as here in jsdom). A toast
    // raised from inside the dialog, such as a failed confirm, must not be hidden with the page.
    render(
      <div>
        <AlertDialog.Backdrop isOpen>
          <AlertDialog.Container>
            <AlertDialog.Dialog>
              <AlertDialog.Heading>Delete the plan?</AlertDialog.Heading>
            </AlertDialog.Dialog>
          </AlertDialog.Container>
        </AlertDialog.Backdrop>
        <ToastHost />
      </div>,
    )
    await screen.findByRole('alertdialog')
    act(() => {
      toast.error('Could not delete')
    })
    expect(screen.getByRole('region', { name: 'Notifications' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Dismiss notification' })).toBeInTheDocument()
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

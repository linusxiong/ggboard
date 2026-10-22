import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { callsTo, mockApi, renderApp, resetApp, urlOfCall } from '@/features/shop/testing'
import { type SegmentItem, SegmentedControl, SegmentedTabs } from './SegmentedControl'

type Fruit = 'apple' | 'pear' | 'plum' | 'fig'
/** Radios reachable with Tab (disabled buttons are skipped by the browser). */
const tabStops = () =>
  screen.getAllByRole('radio').filter((r) => r.tabIndex === 0 && !(r as HTMLButtonElement).disabled)

const ITEMS: SegmentItem<Fruit>[] = [
  { id: 'apple', label: 'Apple' },
  { id: 'pear', label: 'Pear', count: 3 },
  { id: 'plum', label: 'Plum', isDisabled: true },
  { id: 'fig', label: 'Fig' },
]

function Controlled({ onChange }: { onChange?: (v: Fruit) => void }) {
  const [value, setValue] = useState<Fruit>('apple')
  return (
    <SegmentedControl
      aria-label="Fruit"
      items={ITEMS}
      value={value}
      onChange={(v) => {
        setValue(v)
        onChange?.(v)
      }}
    />
  )
}

describe('SegmentedControl', () => {
  it('is a labelled radio group with one checked segment, counts and a single tab stop', () => {
    render(<Controlled />)
    const group = screen.getByRole('radiogroup', { name: 'Fruit' })
    expect(group).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Apple' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('radio', { name: /Pear/ })).toHaveTextContent('Pear3')
    expect(screen.getByRole('radio', { name: 'Plum' })).toBeDisabled()
    expect(tabStops().map((r) => r.textContent)).toEqual(['Apple'])
  })

  it('selects on click', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<Controlled onChange={onChange} />)
    await user.click(screen.getByRole('radio', { name: /Pear/ }))
    expect(onChange).toHaveBeenLastCalledWith('pear')
    expect(screen.getByRole('radio', { name: /Pear/ })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('radio', { name: 'Apple' })).toHaveAttribute('aria-checked', 'false')
  })

  it('moves and selects with the arrow keys (wrapping, skipping disabled), Home and End', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(<Controlled onChange={onChange} />)
    await user.tab()
    expect(screen.getByRole('radio', { name: 'Apple' })).toHaveFocus()

    await user.keyboard('{ArrowRight}')
    expect(onChange).toHaveBeenLastCalledWith('pear')
    expect(screen.getByRole('radio', { name: /Pear/ })).toHaveFocus()

    await user.keyboard('{ArrowRight}') // Plum is disabled → Fig
    expect(onChange).toHaveBeenLastCalledWith('fig')
    expect(screen.getByRole('radio', { name: 'Fig' })).toHaveAttribute('aria-checked', 'true')

    await user.keyboard('{ArrowRight}') // wraps
    expect(onChange).toHaveBeenLastCalledWith('apple')

    await user.keyboard('{ArrowLeft}') // wraps backwards
    expect(onChange).toHaveBeenLastCalledWith('fig')

    await user.keyboard('{Home}')
    expect(onChange).toHaveBeenLastCalledWith('apple')
    await user.keyboard('{End}')
    expect(onChange).toHaveBeenLastCalledWith('fig')
    expect(screen.getByRole('radio', { name: 'Fig' })).toHaveFocus()
    expect(tabStops()).toHaveLength(1)
  })

  it('shows a pick at once while the owner commits it later', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    // The owner ignores the change (e.g. a router transition still running): the pick shows for a moment.
    render(<SegmentedControl aria-label="Fruit" items={ITEMS} value="apple" onChange={onChange} />)
    await user.click(screen.getByRole('radio', { name: 'Fig' }))
    expect(onChange).toHaveBeenCalledWith('fig')
    expect(screen.getByRole('radio', { name: 'Fig' })).toHaveAttribute('aria-checked', 'true')
    await waitFor(
      () => expect(screen.getByRole('radio', { name: 'Apple' })).toHaveAttribute('aria-checked', 'true'),
      {
        timeout: 4000,
      },
    )
  })

  it('renders a visible label that names the group', () => {
    render(<SegmentedControl label="Sort" items={ITEMS} value="apple" onChange={() => {}} />)
    expect(screen.getByRole('radiogroup', { name: 'Sort' })).toBeInTheDocument()
  })
})

describe('SegmentedTabs', () => {
  function Tabs() {
    const [tab, setTab] = useState<'one' | 'two'>('one')
    return (
      <SegmentedTabs value={tab} onChange={setTab}>
        <SegmentedTabs.List
          aria-label="Sections"
          items={[
            { id: 'one', label: 'One' },
            { id: 'two', label: 'Two' },
          ]}
        />
        <SegmentedTabs.Panel id="one">First panel</SegmentedTabs.Panel>
        <SegmentedTabs.Panel id="two">Second panel</SegmentedTabs.Panel>
      </SegmentedTabs>
    )
  }

  it('is a tablist whose tabs switch the panel (click and arrows)', async () => {
    const user = userEvent.setup()
    render(<Tabs />)
    expect(screen.getByRole('tablist', { name: 'Sections' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'One' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tabpanel')).toHaveTextContent('First panel')

    await user.click(screen.getByRole('tab', { name: 'Two' }))
    expect(screen.getByRole('tabpanel')).toHaveTextContent('Second panel')

    await user.keyboard('{ArrowLeft}')
    expect(screen.getByRole('tab', { name: 'One' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tabpanel')).toHaveTextContent('First panel')
  })
})

describe('orders page status filter', () => {
  afterEach(resetApp)
  const T = { timeout: 4000 }

  it('reflects ?status= and writes the pick back to the URL and the list query', async () => {
    const fetchMock = mockApi({ 'GET /user/orders': () => [] })
    const { router } = renderApp('/orders?status=0')
    const user = userEvent.setup()

    const group = await screen.findByRole('radiogroup', { name: '按状态筛选' }, T)
    expect(group).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: '待支付' })).toHaveAttribute('aria-checked', 'true')
    await waitFor(() =>
      expect(
        callsTo(fetchMock, 'GET', '/user/orders').some(
          (c) => urlOfCall(c)?.searchParams.get('status') === '0',
        ),
      ).toBe(true),
    )

    await user.click(screen.getByRole('radio', { name: '已完成' }))
    await waitFor(() => expect(router.state.location.search).toEqual({ status: 3 }))
    await waitFor(() =>
      expect(
        callsTo(fetchMock, 'GET', '/user/orders').some(
          (c) => urlOfCall(c)?.searchParams.get('status') === '3',
        ),
      ).toBe(true),
    )
    expect(screen.getByRole('radio', { name: '已完成' })).toHaveAttribute('aria-checked', 'true')

    // Keyboard: Home → "全部" drops the param.
    act(() => screen.getByRole('radio', { name: '已完成' }).focus())
    await user.keyboard('{Home}')
    await waitFor(() => expect(router.state.location.search).toEqual({}))
    expect(screen.getByRole('radio', { name: '全部' })).toHaveAttribute('aria-checked', 'true')
  })
})

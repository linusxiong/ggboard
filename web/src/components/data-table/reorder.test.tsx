import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import { DataTable } from './DataTable'
import { DragHandle } from './DragHandle'
import { createDataTableColumns } from './features'
import { reorderIds, sameOrder } from './reorder'
import { useRowReorder } from './useRowReorder'

describe('reorderIds', () => {
  const ids = [1, 2, 3, 4, 5]

  it('moves one id before or after the target', () => {
    expect(reorderIds(ids, [1], 3, 'after')).toEqual([2, 3, 1, 4, 5])
    expect(reorderIds(ids, [1], 3, 'before')).toEqual([2, 1, 3, 4, 5])
    expect(reorderIds(ids, [5], 1, 'before')).toEqual([5, 1, 2, 3, 4])
    expect(reorderIds(ids, [2], 5, 'after')).toEqual([1, 3, 4, 5, 2])
  })

  it('keeps several moved ids together in their current order', () => {
    expect(reorderIds(ids, [4, 1], 2, 'after')).toEqual([2, 1, 4, 3, 5])
  })

  it('returns an unchanged copy for no-op drops', () => {
    expect(reorderIds(ids, [3], 3, 'before')).toEqual(ids)
    expect(reorderIds(ids, [9], 3, 'before')).toEqual(ids)
    expect(reorderIds(ids, [1], 9, 'after')).toEqual(ids)
    expect(reorderIds(ids, [2], 1, 'after')).toEqual(ids)
    expect(reorderIds(ids, [], 1, 'after')).not.toBe(ids)
  })

  it('compares orders', () => {
    expect(sameOrder([1, 2], [1, 2])).toBe(true)
    expect(sameOrder([1, 2], [2, 1])).toBe(false)
    expect(sameOrder([1], [1, 2])).toBe(false)
  })
})

interface Item {
  id: number
  name: string
}
const ITEMS: Item[] = [
  { id: 1, name: 'Alpha' },
  { id: 2, name: 'Bravo' },
  { id: 3, name: 'Charlie' },
]
const col = createDataTableColumns<Item>()
const columns = col.columns([
  col.display({
    id: 'order',
    header: 'Order',
    cell: (c) => <DragHandle label={`Reorder ${c.row.original.name}`} />,
  }),
  col.accessor('name', { header: 'Name', meta: { isRowHeader: true } }),
])

function Harness({
  isDisabled = false,
  onReorder,
}: {
  isDisabled?: boolean
  onReorder: (ids: string[]) => void
}) {
  const [order, setOrder] = useState(() => ITEMS.map((i) => String(i.id)))
  const byId = new Map(ITEMS.map((i) => [String(i.id), i]))
  const dnd = useRowReorder({
    ids: order,
    getLabel: (id) => byId.get(id)?.name ?? id,
    onReorder: (ids) => {
      onReorder(ids)
      setOrder(ids)
    },
    isDisabled,
  })
  return (
    <DataTable
      aria-label="Items"
      columns={columns}
      data={order.map((id) => byId.get(id)!)}
      getRowId={(i) => String(i.id)}
      dragAndDropHooks={dnd}
    />
  )
}

const rowNames = () =>
  within(screen.getByRole('grid', { name: 'Items' }))
    .getAllByRole('rowheader')
    .map((cell) => cell.textContent)

/** Minimal DataTransfer: jsdom has none. */
const dataTransfer = () => ({
  clearData: vi.fn(),
  setData: vi.fn(),
  setDragImage: vi.fn(),
  items: { add: vi.fn() },
  types: [],
  effectAllowed: 'all',
  dropEffect: 'none',
})

describe('DataTable drag to reorder', () => {
  beforeAll(async () => {
    await loadNamespaces(['common', 'core'], 'zh-CN')
  })
  beforeEach(() => {
    window.matchMedia = vi.fn().mockImplementation((query: string) => ({
      matches: query.includes('min-width: 768px'),
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))
  })

  it('reorders with the keyboard through the drag handle', async () => {
    const user = userEvent.setup()
    const onReorder = vi.fn()
    render(<Harness onReorder={onReorder} />)
    const handle = await screen.findByRole('button', { name: 'Reorder Alpha' })
    expect(handle).toBeEnabled()
    // Grid keyboard model: Tab → row, → → first cell, Tab → the handle inside it.
    await user.tab()
    await user.keyboard('{ArrowRight}')
    await user.tab()
    await waitFor(() => expect(handle).toHaveFocus())
    await user.keyboard('{Enter}')
    // The drag starts on the gap right after the row (a no-op drop); ↓ moves to the next gap.
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Insert between Alpha and Bravo' })).toHaveFocus(),
    )
    await user.keyboard('{ArrowDown}')
    expect(screen.getByRole('button', { name: 'Insert between Bravo and Charlie' })).toHaveFocus()
    await user.keyboard('{Enter}')
    await waitFor(() => expect(onReorder).toHaveBeenCalledTimes(1))
    expect(onReorder).toHaveBeenCalledWith(['2', '1', '3'])
    expect(rowNames()).toEqual(['Bravo', 'Alpha', 'Charlie'])
  })

  it('does not report a drop that changes nothing', async () => {
    const user = userEvent.setup()
    const onReorder = vi.fn()
    render(<Harness onReorder={onReorder} />)
    const handle = await screen.findByRole('button', { name: 'Reorder Alpha' })
    await user.tab()
    await user.keyboard('{ArrowRight}')
    await user.tab()
    await waitFor(() => expect(handle).toHaveFocus())
    await user.keyboard('{Enter}')
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Insert between Alpha and Bravo' })).toHaveFocus(),
    )
    await user.keyboard('{Enter}')
    await waitFor(() => expect(screen.queryByRole('button', { name: /^Insert / })).not.toBeInTheDocument())
    expect(onReorder).not.toHaveBeenCalled()
    expect(rowNames()).toEqual(['Alpha', 'Bravo', 'Charlie'])
  })

  it('disables the handles and dragging when isDisabled', async () => {
    render(<Harness isDisabled onReorder={vi.fn()} />)
    for (const name of ['Alpha', 'Bravo', 'Charlie'])
      expect(await screen.findByRole('button', { name: `Reorder ${name}` })).toBeDisabled()
    const row = screen.getByRole('row', { name: /Alpha/ })
    expect(row).not.toHaveAttribute('draggable', 'true')
  })

  it('starts pointer drags only from the handle', async () => {
    render(<Harness onReorder={vi.fn()} />)
    await screen.findByRole('button', { name: 'Reorder Alpha' })
    const row = screen.getByRole('row', { name: /Alpha/ })
    expect(row).toHaveAttribute('draggable', 'true')

    // Press on the name cell, then move: the drag is cancelled.
    const nameCell = within(row).getByRole('rowheader')
    fireEvent.pointerDown(nameCell)
    expect(fireEvent.dragStart(nameCell, { dataTransfer: dataTransfer() })).toBe(false)

    // Press on the handle (its wrapper takes the pointer: the button has pointer-events: none).
    const wrapper = within(row).getByRole('button', { name: 'Reorder Alpha' }).parentElement!
    expect(wrapper).toHaveAttribute('data-drag-handle')
    fireEvent.pointerDown(wrapper)
    expect(fireEvent.dragStart(wrapper, { dataTransfer: dataTransfer() })).toBe(true)
  })
})

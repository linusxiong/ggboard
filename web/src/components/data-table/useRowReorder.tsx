/**
 * Drag-to-reorder for `<DataTable>` rows, built on React Aria's drag and drop (mouse, touch,
 * keyboard and screen readers):
 *
 *   const dnd = useRowReorder({ ids, getLabel, onReorder: (ids) => save(ids), isDisabled: filtered })
 *   col.display({ id: 'order', cell: (c) => <DragHandle label={t('reorder', { name })} /> })
 *   <DataTable dragAndDropHooks={dnd} getRowId={(r) => String(r.id)} … />
 *
 * `ids` are the `getRowId` values of the full list in its current order; `onReorder` gets the whole
 * new order. Keyboard: Tab to the handle, Enter, ↑/↓ to choose the gap, Enter to drop, Esc to cancel.
 * Desktop table only; the mobile card list has no reordering.
 */
import { type DragAndDropHooks, DropIndicator, useDragAndDrop } from 'react-aria-components'
import { RowDragPreview } from './DragHandle'
import { reorderIds, sameOrder } from './reorder'

export interface RowReorderOptions {
  /** Row ids (as returned by `getRowId`) of the whole list, in its current order. */
  ids: readonly string[]
  /** Text for the drag preview and the dragged data (`text/plain`), e.g. the row's name. */
  getLabel: (id: string) => string
  /** Called once per drop with the full new order (not called when nothing moved). */
  onReorder: (ids: string[]) => void
  /** Disable dragging (e.g. while filtered or while a save is pending). */
  isDisabled?: boolean
}

export function useRowReorder({ ids, getLabel, onReorder, isDisabled }: RowReorderOptions): DragAndDropHooks {
  const { dragAndDropHooks } = useDragAndDrop({
    isDisabled,
    getItems: (keys) => [...keys].map((key) => ({ 'text/plain': getLabel(String(key)) })),
    onReorder: (e) => {
      const { key, dropPosition } = e.target
      if (dropPosition === 'on') return
      const next = reorderIds(ids, [...e.keys].map(String), String(key), dropPosition)
      if (!sameOrder(next, ids)) onReorder(next)
    },
    // The indicator is a 0-height table row between two rows; the active one draws a 2 px systemBlue
    // insertion line, absolutely positioned so it never shifts the rows (no jitter while dragging).
    renderDropIndicator: (target) => (
      <DropIndicator target={target}>
        {({ isDropTarget }) =>
          isDropTarget ? (
            <div className="relative h-0">
              <div className="absolute inset-x-2 -top-px h-0.5 rounded-full bg-system-blue" />
            </div>
          ) : null
        }
      </DropIndicator>
    ),
    renderDragPreview: (items) => <RowDragPreview label={items[0]?.['text/plain'] ?? ''} />,
  })
  return dragAndDropHooks
}

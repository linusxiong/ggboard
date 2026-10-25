import { Button } from '@heroui/react'
import { GripVertical } from 'lucide-react'
import { DRAG_HANDLE_ATTR } from './reorder'

/**
 * Grip button for a row of a `<DataTable dragAndDropHooks={useRowReorder(...)}>`. React Aria wires
 * it as the row's drag button (`slot="drag"`): keyboard and screen-reader users focus it and press
 * Enter to pick the row up. It has `pointer-events: none` (React Aria), so the wrapper is what the
 * pointer hits: DataTable starts pointer drags only from inside it.
 */
export function DragHandle({ label }: { label: string }) {
  return (
    <span
      {...{ [DRAG_HANDLE_ATTR]: '' }}
      // The row opts out of being a drag source (DataTable); in Chromium/WebKit the handle is the source.
      className="inline-flex cursor-grab touch-none rounded-[10px] [-webkit-user-drag:element] has-[[data-disabled]]:cursor-default has-[[data-disabled]]:[-webkit-user-drag:none]"
    >
      <Button
        slot="drag"
        size="sm"
        variant="ghost"
        isIconOnly
        aria-label={label}
        className="text-label-secondary"
      >
        <GripVertical className="size-4" aria-hidden="true" />
      </Button>
    </span>
  )
}

/** What a pointer drag carries: grip + row label on the overlay surface, readable in both themes. */
export function RowDragPreview({ label }: { label: string }) {
  return (
    <div className="flex max-w-72 items-center gap-2 rounded-[12px] border border-hairline bg-overlay-surface px-3 py-2 text-subhead font-medium text-label">
      <GripVertical className="size-4 shrink-0 text-label-secondary" aria-hidden="true" />
      <span className="truncate">{label}</span>
    </div>
  )
}

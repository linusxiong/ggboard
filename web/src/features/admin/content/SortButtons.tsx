import { Button } from '@heroui/react'
import { ChevronDown, ChevronUp } from 'lucide-react'

interface SortButtonsProps {
  /** Accessible item name, e.g. the row title. */
  itemLabel: string
  canUp: boolean
  canDown: boolean
  onMove: (dir: -1 | 1) => void
  isDisabled?: boolean
  upLabel: string
  downLabel: string
}

/** Up/down reorder buttons for unpaginated admin lists (notices, knowledge, payments). */
export function SortButtons({
  itemLabel,
  canUp,
  canDown,
  onMove,
  isDisabled,
  upLabel,
  downLabel,
}: SortButtonsProps) {
  return (
    <div className="flex items-center gap-0.5">
      <Button
        size="sm"
        variant="ghost"
        isIconOnly
        aria-label={`${upLabel}: ${itemLabel}`}
        isDisabled={isDisabled || !canUp}
        onPress={() => onMove(-1)}
      >
        <ChevronUp className="size-4" aria-hidden />
      </Button>
      <Button
        size="sm"
        variant="ghost"
        isIconOnly
        aria-label={`${downLabel}: ${itemLabel}`}
        isDisabled={isDisabled || !canDown}
        onPress={() => onMove(1)}
      >
        <ChevronDown className="size-4" aria-hidden />
      </Button>
    </div>
  )
}

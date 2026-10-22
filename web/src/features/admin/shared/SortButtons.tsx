import { Button } from '@heroui/react'
import { ChevronDown, ChevronUp } from 'lucide-react'

/** Up/down reorder buttons for sortable admin lists. */
export function SortButtons({
  name,
  isFirst,
  isLast,
  isDisabled,
  onMove,
  upLabel,
  downLabel,
  vertical = false,
}: {
  name: string
  isFirst: boolean
  isLast: boolean
  isDisabled?: boolean
  onMove: (direction: -1 | 1) => void
  upLabel: (name: string) => string
  downLabel: (name: string) => string
  /** Stack the buttons (narrow columns). */
  vertical?: boolean
}) {
  return (
    <span className={vertical ? 'flex flex-col items-center' : 'flex items-center'}>
      <Button
        size="sm"
        variant="ghost"
        isIconOnly
        aria-label={upLabel(name)}
        isDisabled={isDisabled || isFirst}
        onPress={() => onMove(-1)}
      >
        <ChevronUp className="size-4" aria-hidden="true" />
      </Button>
      <Button
        size="sm"
        variant="ghost"
        isIconOnly
        aria-label={downLabel(name)}
        isDisabled={isDisabled || isLast}
        onPress={() => onMove(1)}
      >
        <ChevronDown className="size-4" aria-hidden="true" />
      </Button>
    </span>
  )
}

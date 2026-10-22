import { ToggleButton, ToggleButtonGroup } from '@heroui/react'

interface SegmentedProps<K extends string> {
  'aria-label': string
  value: K
  onChange: (value: K) => void
  options: ReadonlyArray<{ value: K; label: string }>
  size?: 'sm' | 'md'
  className?: string
}

/** iOS-style segmented control (single-selection ToggleButtonGroup that never empties). */
export function Segmented<K extends string>({
  value,
  onChange,
  options,
  size = 'sm',
  className,
  ...rest
}: SegmentedProps<K>) {
  return (
    <ToggleButtonGroup
      aria-label={rest['aria-label']}
      selectionMode="single"
      disallowEmptySelection
      selectedKeys={[value]}
      onSelectionChange={(keys) => {
        const next = [...keys][0]
        if (next !== undefined && next !== value) onChange(String(next) as K)
      }}
      size={size}
      className={className}
    >
      {options.map((option) => (
        <ToggleButton key={option.value} id={option.value}>
          {option.label}
        </ToggleButton>
      ))}
    </ToggleButtonGroup>
  )
}

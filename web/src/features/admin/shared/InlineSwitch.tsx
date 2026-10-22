import { Switch } from '@heroui/react'

/** Label-less switch for table cells (the accessible name comes from `aria-label`). */
export function InlineSwitch({
  isSelected,
  onChange,
  isDisabled,
  ...rest
}: {
  'aria-label': string
  isSelected: boolean
  onChange: (value: boolean) => void
  isDisabled?: boolean
}) {
  return (
    <Switch
      aria-label={rest['aria-label']}
      isSelected={isSelected}
      onChange={onChange}
      isDisabled={isDisabled}
      size="sm"
    >
      <Switch.Content>
        <Switch.Control>
          <Switch.Thumb />
        </Switch.Control>
      </Switch.Content>
    </Switch>
  )
}

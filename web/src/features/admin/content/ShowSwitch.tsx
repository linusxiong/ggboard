import { Switch } from '@heroui/react'

interface ShowSwitchProps {
  isSelected: boolean
  onChange: (value: boolean) => void
  /** Accessible name, e.g. "显示：<title>". */
  'aria-label': string
  isDisabled?: boolean
}

/** Label-less inline switch for "show/enabled" columns. */
export function ShowSwitch({ isSelected, onChange, isDisabled, ...rest }: ShowSwitchProps) {
  return (
    <Switch
      size="sm"
      isSelected={isSelected}
      onChange={onChange}
      isDisabled={isDisabled}
      aria-label={rest['aria-label']}
    >
      <Switch.Content>
        <Switch.Control>
          <Switch.Thumb />
        </Switch.Control>
      </Switch.Content>
    </Switch>
  )
}

import { Button, FieldError, InputGroup, TextField } from '@heroui/react'
import { Eye, EyeOff } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { FieldHint, FieldLabel } from '@/components/form/parts'
import { useT } from '@/i18n'

export interface SecretInputProps {
  label: ReactNode
  value: string
  onChange: (value: string) => void
  name?: string
  description?: ReactNode
  error?: string
  isRequired?: boolean
  isDisabled?: boolean
  placeholder?: string
  /** Show a copy button (default true when there is a value). */
  copyable?: boolean
  /** Extra controls after reveal/copy (e.g. "regenerate"). */
  actions?: ReactNode
  className?: string
  /** Controlled reveal state (optional). */
  revealed?: boolean
  onRevealedChange?: (revealed: boolean) => void
  monospace?: boolean
}

/**
 * Password-style input for secrets (`captcha_secret`, `email_password`, `server_token`, gateway keys):
 * masked by default, with reveal + copy. Never autocompleted by the browser. The description sits
 * behind the label's ⓘ.
 * Needs the `admin-settings` i18n namespace (preload it in the route loader).
 */
export function SecretInput({
  label,
  value,
  onChange,
  name,
  description,
  error,
  isRequired,
  isDisabled,
  placeholder,
  copyable = true,
  actions,
  className,
  revealed: revealedProp,
  onRevealedChange,
  monospace = true,
}: SecretInputProps) {
  const t = useT('admin-settings')
  const [revealedState, setRevealedState] = useState(false)
  const revealed = revealedProp ?? revealedState
  const setRevealed = (next: boolean) => {
    setRevealedState(next)
    onRevealedChange?.(next)
  }
  return (
    <TextField
      name={name}
      id={name}
      type={revealed ? 'text' : 'password'}
      value={value}
      onChange={onChange}
      isInvalid={Boolean(error)}
      isRequired={isRequired}
      isDisabled={isDisabled}
      validationBehavior="aria"
      fullWidth
      className={className}
    >
      <FieldLabel label={label} help={description} />
      <InputGroup fullWidth>
        <InputGroup.Input
          className={monospace ? 'w-full font-mono' : 'w-full'}
          placeholder={placeholder}
          autoComplete="new-password"
          spellCheck={false}
          data-1p-ignore
        />
        <InputGroup.Suffix className="gap-1">
          <Button
            size="sm"
            variant="ghost"
            isIconOnly
            aria-label={revealed ? t('inputs.hide') : t('inputs.show')}
            aria-pressed={revealed}
            onPress={() => setRevealed(!revealed)}
          >
            {revealed ? <EyeOff className="size-4" aria-hidden /> : <Eye className="size-4" aria-hidden />}
          </Button>
          {copyable && value ? <CopyButton value={value} variant="ghost" /> : null}
          {actions}
        </InputGroup.Suffix>
      </InputGroup>
      {error ? null : <FieldHint>{description}</FieldHint>}
      <FieldError>{error}</FieldError>
    </TextField>
  )
}

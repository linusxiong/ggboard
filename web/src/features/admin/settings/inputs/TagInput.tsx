import type { ReactNode } from 'react'
import { TagInput as BaseTagInput } from '@/components/form/TagInput'
import { useT } from '@/i18n'

export interface TagInputProps {
  label: ReactNode
  value: readonly string[]
  onChange: (value: string[]) => void
  name?: string
  description?: ReactNode
  error?: string
  placeholder?: string
  isDisabled?: boolean
  /** Returns an error message for an item that must not be added. */
  validate?: (item: string) => string | undefined
  /** Cleans an item before it is added (e.g. lower-case a domain). */
  normalize?: (item: string) => string
  /** Quick-add chips under the field. */
  suggestions?: readonly string[]
  /** Monospace tags (URLs, IPs, codes). */
  monospace?: boolean
  className?: string
}

/**
 * Localized wrapper of the shared token field (`@/components/form/TagInput`) for `strings`
 * settings and other admin tag lists. Needs the `admin-settings` i18n namespace.
 */
export function TagInput({ placeholder, ...props }: TagInputProps) {
  const t = useT('admin-settings')
  return (
    <BaseTagInput
      {...props}
      id={props.name}
      placeholder={placeholder ?? t('inputs.addPlaceholder')}
      removeLabel={(item) => t('inputs.remove', { item })}
      addLabel={t('inputs.add')}
    />
  )
}

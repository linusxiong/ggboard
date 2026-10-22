import { Button, Description, FieldError, Input, Label, TextField } from '@heroui/react'
import { ChevronDown, ChevronUp, Plus, Trash2 } from 'lucide-react'
import { type ReactNode, useId } from 'react'
import { useT } from '@/i18n'

interface UrlListEditorProps {
  label: ReactNode
  name?: string
  value: readonly string[]
  onChange: (value: string[]) => void
  description?: ReactNode
  error?: string
  isDisabled?: boolean
  placeholder?: string
}

const isHttpUrl = (v: string) => /^https?:\/\/[^\s/$.?#][^\s]*$/i.test(v.trim())

/**
 * Ordered list of URLs (`subscribe_url`: alternative subscription hosts; the first is the default
 * shown to users, empty list → `app_url`). Rows can be added, removed and reordered.
 */
export function UrlListEditor({
  label,
  name,
  value,
  onChange,
  description,
  error,
  isDisabled,
  placeholder,
}: UrlListEditorProps) {
  const t = useT('admin-settings')
  const id = useId()
  const set = (i: number, v: string) => onChange(value.map((x, j) => (j === i ? v : x)))
  const move = (i: number, d: -1 | 1) => {
    const next = [...value]
    const j = i + d
    if (j < 0 || j >= next.length) return
    ;[next[i], next[j]] = [next[j] as string, next[i] as string]
    onChange(next)
  }
  return (
    <div role="group" aria-labelledby={`${id}-label`} className="flex flex-col gap-2">
      <Label id={`${id}-label`} elementType="span">
        {label}
      </Label>
      {value.length === 0 ? (
        <p className="text-footnote text-label-secondary">{t('urlList.empty')}</p>
      ) : (
        <ol className="flex flex-col gap-2">
          {value.map((url, i) => {
            const invalid = url.trim() !== '' && !isHttpUrl(url)
            return (
              <li key={i} className="flex items-start gap-1">
                <TextField
                  name={name ? `${name}.${i}` : undefined}
                  aria-label={t('urlList.item', { n: i + 1 })}
                  value={url}
                  onChange={(v) => set(i, v)}
                  isInvalid={invalid}
                  isDisabled={isDisabled}
                  validationBehavior="aria"
                  fullWidth
                  className="min-w-0 flex-1"
                >
                  <Input
                    type="url"
                    placeholder={placeholder}
                    className="w-full font-mono"
                    autoComplete="off"
                  />
                  <FieldError>{invalid ? t('urlList.invalid') : undefined}</FieldError>
                </TextField>
                <Button
                  size="sm"
                  variant="ghost"
                  isIconOnly
                  aria-label={t('urlList.up', { n: i + 1 })}
                  isDisabled={isDisabled || i === 0}
                  onPress={() => move(i, -1)}
                >
                  <ChevronUp className="size-4" aria-hidden />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  isIconOnly
                  aria-label={t('urlList.down', { n: i + 1 })}
                  isDisabled={isDisabled || i === value.length - 1}
                  onPress={() => move(i, 1)}
                >
                  <ChevronDown className="size-4" aria-hidden />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  isIconOnly
                  aria-label={t('urlList.remove', { n: i + 1 })}
                  isDisabled={isDisabled}
                  onPress={() => onChange(value.filter((_, j) => j !== i))}
                >
                  <Trash2 className="size-4" aria-hidden />
                </Button>
              </li>
            )
          })}
        </ol>
      )}
      <div>
        <Button
          size="sm"
          variant="secondary"
          isDisabled={isDisabled}
          onPress={() => onChange([...value, ''])}
        >
          <Plus className="size-4" aria-hidden />
          {t('urlList.add')}
        </Button>
      </div>
      {description && !error ? <Description>{description}</Description> : null}
      {error ? (
        <p className="text-footnote text-danger" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  )
}

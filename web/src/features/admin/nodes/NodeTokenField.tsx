/**
 * Node token (`token`) for `form.AppField`: the settings `SecretInput` (masked, reveal, copy, never
 * autofilled) plus "generate a random token" and "back to the shared server_token". Replacing a
 * saved token asks first, like regenerating `server_token`. Needs the `admin-settings` namespace
 * (SecretInput's reveal labels; preloaded by the `/admin/nodes` route).
 */
import { Button } from '@heroui/react'
import type { AnyFieldApi } from '@tanstack/react-form'
import { RefreshCw, X } from 'lucide-react'
import { useState } from 'react'
import { confirm } from '@/components/feedback/confirm'
import { useFieldContext } from '@/components/form'
import { fieldErrorMessage } from '@/components/form/fieldError'
import { SecretInput } from '@/features/admin/settings/inputs/SecretInput'
import { randomToken } from '@/features/admin/settings/settingsMeta'
import { useT } from '@/i18n'
import { NODE_TOKEN_LENGTH } from './nodeForm'

function clearServerError(field: AnyFieldApi) {
  if (field.state.meta.errorMap.onServer) {
    field.setMeta((meta) => ({ ...meta, errorMap: { ...meta.errorMap, onServer: undefined } }))
  }
}

interface NodeTokenFieldProps {
  /** The token stored on the server (`''` for none): replacing it asks first. */
  saved: string
}

export function NodeTokenField({ saved }: NodeTokenFieldProps) {
  const t = useT('admin-nodes')
  const field = useFieldContext<string>()
  const [revealed, setRevealed] = useState(false)
  const value = field.state.value ?? ''
  const set = (next: string) => {
    field.handleChange(next)
    clearServerError(field)
  }
  const generate = async () => {
    if (saved && value === saved) {
      const ok = await confirm({
        title: t('nodes.fields.tokenReplaceTitle'),
        description: t('nodes.fields.tokenReplaceHint'),
        confirmLabel: t('nodes.fields.tokenReplace'),
        tone: 'danger',
      })
      if (!ok) return
    }
    set(randomToken(NODE_TOKEN_LENGTH))
    setRevealed(true)
  }
  return (
    <SecretInput
      name={field.name}
      label={t('nodes.fields.token')}
      value={value}
      onChange={set}
      description={t('nodes.fields.tokenHint')}
      error={fieldErrorMessage(field.state.meta)}
      placeholder={t('nodes.fields.tokenShared')}
      revealed={revealed}
      onRevealedChange={setRevealed}
      actions={
        <>
          <Button
            size="sm"
            variant="ghost"
            isIconOnly
            aria-label={t('nodes.fields.tokenGenerate')}
            onPress={() => void generate()}
          >
            <RefreshCw className="size-4" aria-hidden />
          </Button>
          {value ? (
            <Button
              size="sm"
              variant="ghost"
              isIconOnly
              aria-label={t('nodes.fields.tokenClear')}
              onPress={() => set('')}
            >
              <X className="size-4" aria-hidden />
            </Button>
          ) : null}
        </>
      }
    />
  )
}

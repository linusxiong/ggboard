import { FieldError, TextArea, TextField } from '@heroui/react'
import { useState } from 'react'
import { FieldHint, FieldLabel } from '@/components/form/parts'
import { useT } from '@/i18n'
import type { Settings } from './protocol'

interface RawJsonEditorProps {
  value: Settings
  onChange: (next: Settings) => void
  /** Validation messages (path → message) listed under the editor. */
  issues: Record<string, string>
}

/** Raw `protocol_settings` editor: every valid JSON object is applied immediately. Remount (key) to re-sync. */
export function RawJsonEditor({ value, onChange, issues }: RawJsonEditorProps) {
  const t = useT('admin-nodes')
  const [text, setText] = useState(() => JSON.stringify(value, null, 2))
  const [error, setError] = useState<string>()
  const entries = Object.entries(issues)
  return (
    <div className="flex flex-col gap-3">
      <TextField
        value={text}
        onChange={(next) => {
          setText(next)
          try {
            const parsed: unknown = JSON.parse(next)
            if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed))
              throw new Error('object')
            setError(undefined)
            onChange(parsed as Settings)
          } catch {
            setError(t('nodes.json.invalid'))
          }
        }}
        isInvalid={Boolean(error)}
        validationBehavior="aria"
        fullWidth
      >
        <FieldLabel label={t('nodes.json.label')} help={t('nodes.json.hint')} />
        <TextArea rows={18} className="w-full font-mono text-footnote" spellCheck={false} />
        {error ? null : <FieldHint>{t('nodes.json.hint')}</FieldHint>}
        <FieldError>{error}</FieldError>
      </TextField>
      {entries.length ? (
        <div role="alert" className="rounded-[12px] bg-fill-quaternary p-3 text-footnote">
          <p className="font-semibold text-label-red">{t('nodes.json.issues')}</p>
          <ul className="mt-1 list-disc ps-4 text-label">
            {entries.map(([path, message]) => (
              <li key={path}>
                <code className="font-mono">{path || 'protocol_settings'}</code>: {message}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  )
}

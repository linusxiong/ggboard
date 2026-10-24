/**
 * Lightweight Markdown editor: a textarea with a small formatting toolbar, optional snippet
 * buttons and a lazily loaded preview (the existing `Markdown` component → `marked` + `dompurify`
 * load only when the preview opens). No editor dependency.
 *
 *   <MarkdownEditor label="内容" value={v} onChange={setV} snippets={[{ label: '门禁', text: '…' }]} />
 *
 * Keyboard: ⌘/Ctrl+B bold, ⌘/Ctrl+I italic, ⌘/Ctrl+K link, ⌘/Ctrl+Shift+P toggles the preview.
 * Strings come from the `admin-support` namespace (`editor.*`) — preload it, or pass `labels`.
 */
import { Button, FieldError, TextArea, TextField } from '@heroui/react'
import { Bold, Code, Eye, Heading2, Italic, Link2, List, ListOrdered, PencilLine, Quote } from 'lucide-react'
import { lazy, type ReactNode, Suspense, useLayoutEffect, useRef, useState } from 'react'
import { FieldHint, FieldLabel } from '@/components/form/parts'
import { HelpTip } from '@/components/HelpTip'
import { SegmentedControl } from '@/components/SegmentedControl'
import { useT } from '@/i18n'
import { type EditResult, insertText, prefixLines, wrapSelection } from './markdownEdit'
import { cx } from '@/lib/cn'

const Markdown = lazy(() => import('@/components/Markdown'))

export interface MarkdownSnippet {
  /** Button text. */
  label: string
  /** Inserted at the cursor (replaces the selection). With `wrap`, the selection is wrapped instead. */
  text: string
  /** `[before, after]`: wrap the selection (or `placeholder`) instead of inserting `text`. */
  wrap?: readonly [string, string]
  /** Marks the button with a red `*` (e.g. a placeholder the body must contain); say so in `ariaLabel`. */
  required?: boolean
  /** Accessible name when it should say more than `label` (keep `label` at its start). */
  ariaLabel?: string
}

export interface MarkdownEditorProps {
  label: ReactNode
  value: string
  onChange: (value: string) => void
  name?: string
  /** What the field is for: behind the label's ⓘ (and the textarea's accessible description). */
  description?: ReactNode
  error?: string
  isRequired?: boolean
  placeholder?: string
  rows?: number
  snippets?: readonly MarkdownSnippet[]
  /**
   * Reference for the snippet buttons (e.g. what each placeholder expands to): an ⓘ at the end of
   * the toolbar opens it, so the explanation stays off the page's first level.
   */
  help?: ReactNode
  className?: string
}

type Mode = 'write' | 'preview'

export function MarkdownEditor({
  label,
  value,
  onChange,
  name,
  description,
  error,
  isRequired,
  placeholder,
  rows = 12,
  snippets,
  help,
  className,
}: MarkdownEditorProps) {
  const t = useT('admin-support')
  const ref = useRef<HTMLTextAreaElement>(null)
  const pendingSelection = useRef<[number, number] | null>(null)
  const [mode, setMode] = useState<Mode>('write')

  // Restore the caret/selection after a toolbar edit re-rendered the controlled textarea.
  useLayoutEffect(() => {
    const sel = pendingSelection.current
    const el = ref.current
    if (!sel || !el) return
    pendingSelection.current = null
    el.focus()
    el.setSelectionRange(sel[0], sel[1])
  }, [value])

  const apply = (edit: (v: string, s: number, e: number) => EditResult) => {
    const el = ref.current
    const start = el?.selectionStart ?? value.length
    const end = el?.selectionEnd ?? value.length
    const result = edit(value, start, end)
    pendingSelection.current = [result.selectionStart, result.selectionEnd]
    onChange(result.value)
  }

  const actions = {
    bold: () => apply((v, s, e) => wrapSelection(v, s, e, '**', '**', t('editor.sample'))),
    italic: () => apply((v, s, e) => wrapSelection(v, s, e, '*', '*', t('editor.sample'))),
    heading: () => apply((v, s, e) => prefixLines(v, s, e, () => '## ')),
    link: () => apply((v, s, e) => wrapSelection(v, s, e, '[', '](https://)', t('editor.linkText'))),
    list: () => apply((v, s, e) => prefixLines(v, s, e, () => '- ')),
    ordered: () => apply((v, s, e) => prefixLines(v, s, e, (i) => `${i + 1}. `)),
    quote: () => apply((v, s, e) => prefixLines(v, s, e, () => '> ')),
    code: () => apply((v, s, e) => wrapSelection(v, s, e, '`', '`', 'code')),
  }

  const tools: Array<{ key: keyof typeof actions; icon: typeof Bold; label: string; kbd?: string }> = [
    { key: 'heading', icon: Heading2, label: t('editor.heading') },
    { key: 'bold', icon: Bold, label: t('editor.bold'), kbd: 'B' },
    { key: 'italic', icon: Italic, label: t('editor.italic'), kbd: 'I' },
    { key: 'link', icon: Link2, label: t('editor.link'), kbd: 'K' },
    { key: 'list', icon: List, label: t('editor.list') },
    { key: 'ordered', icon: ListOrdered, label: t('editor.orderedList') },
    { key: 'quote', icon: Quote, label: t('editor.quote') },
    { key: 'code', icon: Code, label: t('editor.code') },
  ]

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (!(e.metaKey || e.ctrlKey)) return
    const k = e.key.toLowerCase()
    if (e.shiftKey && k === 'p') {
      e.preventDefault()
      setMode((m) => (m === 'write' ? 'preview' : 'write'))
    } else if (!e.shiftKey && k === 'b') {
      e.preventDefault()
      actions.bold()
    } else if (!e.shiftKey && k === 'i') {
      e.preventDefault()
      actions.italic()
    } else if (!e.shiftKey && k === 'k') {
      e.preventDefault()
      actions.link()
    }
  }

  const minHeight = `${Math.max(rows, 4) * 1.5 + 1.5}rem`

  return (
    <TextField
      name={name}
      id={name}
      value={value}
      onChange={onChange}
      isInvalid={Boolean(error)}
      isRequired={isRequired}
      validationBehavior="aria"
      fullWidth
      className={cx('flex flex-col gap-2', className)}
    >
      <div className="flex flex-wrap items-end justify-between gap-2">
        <FieldLabel label={label} help={description} />
        <SegmentedControl
          aria-label={t('editor.mode')}
          size="sm"
          value={mode}
          onChange={setMode}
          items={[
            { id: 'write', label: t('editor.write'), icon: <PencilLine /> },
            { id: 'preview', label: t('editor.preview'), icon: <Eye /> },
          ]}
        />
      </div>

      {mode === 'write' ? (
        <>
          <div role="toolbar" aria-label={t('editor.toolbar')} className="flex flex-wrap items-center gap-1">
            {tools.map(({ key, icon: Icon, label: toolLabel, kbd }) => (
              <Button
                key={key}
                size="sm"
                variant="ghost"
                isIconOnly
                aria-label={kbd ? `${toolLabel} (⌘${kbd})` : toolLabel}
                onPress={actions[key]}
              >
                <Icon className="size-4" aria-hidden />
              </Button>
            ))}
            {snippets?.length ? <span className="mx-1 h-5 w-px bg-separator" aria-hidden /> : null}
            {snippets?.map((s) => (
              <Button
                key={s.label}
                size="sm"
                variant="tertiary"
                aria-label={s.ariaLabel}
                onPress={() =>
                  apply((v, st, en) =>
                    s.wrap
                      ? wrapSelection(v, st, en, s.wrap[0], s.wrap[1], s.text)
                      : insertText(v, st, en, s.text),
                  )
                }
              >
                {s.label}
                {s.required ? (
                  <span className="-ms-0.5 text-label-red" aria-hidden="true">
                    *
                  </span>
                ) : null}
              </Button>
            ))}
            {help ? <HelpTip subject={typeof label === 'string' ? label : undefined}>{help}</HelpTip> : null}
          </div>
          <TextArea
            ref={ref}
            rows={rows}
            placeholder={placeholder}
            onKeyDown={onKeyDown}
            className="w-full font-mono text-subhead leading-6"
            style={{ minHeight }}
          />
        </>
      ) : (
        <div
          className="markdown-preview rounded-input border-[0.5px] border-hairline bg-elevated-secondary p-4"
          style={{ minHeight }}
          aria-live="polite"
        >
          {value.trim() ? (
            <Suspense fallback={<div className="h-4 w-1/2 shimmer rounded bg-fill-tertiary" />}>
              <Markdown source={value} />
            </Suspense>
          ) : (
            <p className="text-subhead text-label-secondary">{t('editor.nothing')}</p>
          )}
        </div>
      )}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          {error ? null : <FieldHint>{description}</FieldHint>}
          <FieldError>{error}</FieldError>
        </div>
        <span className="shrink-0 text-caption tabular text-label-secondary" aria-live="off">
          {t('editor.count', { count: value.length })}
        </span>
      </div>
    </TextField>
  )
}

export default MarkdownEditor

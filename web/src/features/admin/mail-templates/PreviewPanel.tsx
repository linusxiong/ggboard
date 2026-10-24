import { Chip, Skeleton, Spinner } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { Eye, FileText } from 'lucide-react'
import { useState } from 'react'
import { TitleWithHelp } from '@/components/HelpTip'
import { SegmentedControl } from '@/components/SegmentedControl'
import { useT } from '@/i18n'
import { adminMailTemplatePreviewQuery } from '@/lib/api/queries'
import type { MailTemplateDraft, MailTemplatePreview } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { useDebouncedValue } from './useDebouncedValue'
import { asPlaceholder } from './variables'

/** Debounce before the draft is rendered again (one request per pause in typing). */
export const PREVIEW_DEBOUNCE_MS = 400

type Mode = 'html' | 'text'

interface PreviewPanelProps {
  name: string
  /** Current editor content (memoised: a new object restarts the debounce). */
  draft: MailTemplateDraft
}

/**
 * Live preview: the draft rendered by the server with the template's sample data (`POST …/preview`), shown
 * in a fully sandboxed iframe (no scripts, no same-origin, links inert), plus the plain-text part and the
 * placeholder problems the server found (`unknown`, `missing`) as inline chips. Errors stay in the panel.
 */
export function PreviewPanel({ name, draft }: PreviewPanelProps) {
  const t = useT('admin-settings')
  const settled = useDebouncedValue(draft, PREVIEW_DEBOUNCE_MS)
  const preview = useQuery(adminMailTemplatePreviewQuery(name, settled))
  const [mode, setMode] = useState<Mode>('html')
  // Keep the last successful render on screen when a later one fails.
  const [lastGood, setLastGood] = useState<MailTemplatePreview>()
  if (preview.data && !preview.isPlaceholderData && preview.data !== lastGood) setLastGood(preview.data)
  const shown = preview.data ?? lastGood
  const updating = preview.isFetching || settled !== draft
  const unknown = (shown?.unknown ?? []).map(asPlaceholder)
  const missing = (shown?.missing ?? []).map(asPlaceholder)

  return (
    <section aria-labelledby="mail-template-preview" className="flex flex-col gap-3">
      <header className="flex flex-wrap items-end justify-between gap-x-4 gap-y-2 px-1">
        <TitleWithHelp help={t('mailTemplates.preview.help')} subject={t('mailTemplates.preview.title')}>
          <h3 id="mail-template-preview" className="flex items-center gap-2 text-headline text-label">
            {t('mailTemplates.preview.title')}
            {updating ? <Spinner size="sm" color="current" aria-hidden="true" /> : null}
          </h3>
        </TitleWithHelp>
        <SegmentedControl
          aria-label={t('mailTemplates.preview.mode')}
          size="sm"
          value={mode}
          onChange={setMode}
          items={[
            { id: 'html', label: t('mailTemplates.preview.html'), icon: <Eye /> },
            { id: 'text', label: t('mailTemplates.preview.text'), icon: <FileText /> },
          ]}
        />
      </header>

      <div className="surface-card flex flex-col gap-3 p-4 sm:p-5">
        {preview.isError ? (
          <p className="text-footnote text-label-red" role="status">
            {t('mailTemplates.preview.failed', { message: errorMessage(preview.error) })}
          </p>
        ) : null}

        {/* Always mounted (announced on change); out of the flow while empty. */}
        <div
          aria-live="polite"
          className={cx('flex flex-col gap-2', !missing.length && !unknown.length && 'sr-only')}
        >
          {missing.length ? (
            <IssueRow
              label={t('mailTemplates.preview.missing')}
              hint={t('mailTemplates.preview.missingHint')}
              names={missing}
              color="warning"
            />
          ) : null}
          {unknown.length ? (
            <IssueRow
              label={t('mailTemplates.preview.unknown')}
              hint={t('mailTemplates.preview.unknownHint')}
              names={unknown}
              color="danger"
            />
          ) : null}
        </div>

        {shown ? (
          <>
            <p className="flex min-w-0 gap-2 text-subhead">
              <span className="shrink-0 text-label-secondary">{t('mailTemplates.preview.subject')}</span>
              <span className="min-w-0 font-semibold break-words text-label">
                {shown.subject || t('mailTemplates.preview.noSubject')}
              </span>
            </p>
            <div
              className={cx(
                'overflow-hidden rounded-[14px] border-[0.5px] border-hairline transition-opacity duration-200',
                preview.isPlaceholderData && 'opacity-60',
              )}
              aria-busy={updating || undefined}
            >
              {mode === 'html' ? (
                <iframe
                  title={t('mailTemplates.preview.frameTitle', { subject: shown.subject })}
                  srcDoc={shown.html}
                  // Empty sandbox: no scripts, forms, popups, navigation or same-origin access.
                  sandbox=""
                  referrerPolicy="no-referrer"
                  className="block h-[min(75vh,760px)] min-h-[560px] w-full bg-canvas"
                />
              ) : (
                <pre className="h-[min(75vh,760px)] min-h-[560px] overflow-auto bg-elevated-secondary p-4 font-mono text-footnote leading-5 whitespace-pre-wrap break-words text-label">
                  {shown.text}
                </pre>
              )}
            </div>
          </>
        ) : preview.isError ? null : (
          <div className="flex flex-col gap-3" aria-busy="true">
            <Skeleton className="h-5 w-2/3 rounded" />
            <Skeleton className="h-[560px] w-full rounded-[14px]" />
          </div>
        )}
      </div>
    </section>
  )
}

function IssueRow({
  label,
  hint,
  names,
  color,
}: {
  label: string
  hint: string
  names: readonly string[]
  color: 'warning' | 'danger'
}) {
  return (
    <div className="flex flex-col gap-1">
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-footnote font-semibold text-label">{label}</span>
        {names.map((n) => (
          <Chip key={n} size="sm" variant="soft" color={color}>
            <span className="font-mono">{n}</span>
          </Chip>
        ))}
      </div>
      <p className="text-caption text-label-secondary">{hint}</p>
    </div>
  )
}

import { Button, Chip } from '@heroui/react'
import { useQueryClient } from '@tanstack/react-query'
import { useSelector } from '@tanstack/react-form'
import { MailCheck, RotateCcw } from 'lucide-react'
import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { MarkdownEditor, type MarkdownSnippet } from '@/components/editor/MarkdownEditor'
import { confirm } from '@/components/feedback/confirm'
import { applyServerErrors, FormError, useAppForm } from '@/components/form'
import { fieldErrorMessage } from '@/components/form/fieldError'
import { SaveBar } from '@/components/form/SaveBar'
import { submitError } from '@/components/form/submitError'
import { useLocale, useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type {
  AdminMailTemplate,
  AdminMailTemplateSummary,
  MailTemplateDraft,
  MailTemplateVariable,
} from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { formatDate } from '@/lib/format'
import { toast } from '@/lib/toast'
import { PreviewPanel } from './PreviewPanel'
import { editorVariables, placeholder, variableLabel } from './variables'

const TestTemplateDialog = lazy(() => import('./TestTemplateDialog'))

/** `subject` limit of the API (trimmed, single line). */
export const SUBJECT_MAX = 255

interface TemplateEditorProps {
  template: AdminMailTemplate
  /** The list entry (variables). */
  summary: AdminMailTemplateSummary
  /** Human title of the template (toasts, confirmations). */
  title: string
  /** Reports whether the draft differs from the saved template. */
  onDirtyChange: (dirty: boolean) => void
}

/** Writes the saved state of one locale into the cached list (badges update without a refetch flash). */
function patchSummary(
  list: AdminMailTemplateSummary[] | undefined,
  saved: Pick<AdminMailTemplate, 'name' | 'locale' | 'custom' | 'updated_at'>,
) {
  return list?.map((item) =>
    item.name === saved.name
      ? {
          ...item,
          locales: {
            ...item.locales,
            [saved.locale]: { custom: saved.custom, updated_at: saved.updated_at },
          },
        }
      : item,
  )
}

/**
 * Editor for one template in one locale (mounted per `name:locale`): subject + Markdown body with the
 * template's `{{variables}}` as insert buttons, the live preview, save (sticky bar, ⌘S), reset to the
 * built-in template and a test mail of the current draft.
 */
export function TemplateEditor({ template, summary, title, onDirtyChange }: TemplateEditorProps) {
  const t = useT(['admin-settings', 'common'])
  const uiLocale = useLocale()
  const qc = useQueryClient()
  const { name, locale } = template
  const localeLabel = t(`mailTemplates.locales.${locale}`)
  const [formError, setFormError] = useState<string>()
  const [testOpen, setTestOpen] = useState(false)
  const [resetting, setResetting] = useState(false)

  const form = useAppForm({
    defaultValues: { subject: template.subject, body: template.body },
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      try {
        const saved = await adminApi.saveMailTemplate(name, {
          locale,
          subject: value.subject.trim(),
          body: value.body,
        })
        qc.setQueryData(qk.admin.mailTemplate(name, locale), saved)
        qc.setQueryData<AdminMailTemplateSummary[]>(qk.admin.mailTemplates(), (old) =>
          patchSummary(old, saved),
        )
        formApi.reset({ subject: saved.subject, body: saved.body })
        toast.success(t('mailTemplates.save.saved', { template: title, locale: localeLabel }))
        void qc.invalidateQueries({ queryKey: qk.admin.mailTemplates(), exact: true })
      } catch (e) {
        setFormError(submitError(formApi, e))
      }
    },
  })

  const values = useSelector(form.store, (s) => s.values)
  const isSubmitting = useSelector(form.store, (s) => s.isSubmitting)
  const dirty = values.subject !== template.subject || values.body !== template.body
  useEffect(() => {
    onDirtyChange(dirty)
  }, [dirty, onDirtyChange])
  useEffect(() => () => onDirtyChange(false), [onDirtyChange])

  const draft = useMemo<MailTemplateDraft>(
    () => ({ locale, subject: values.subject, body: values.body }),
    [locale, values.subject, values.body],
  )

  const variables = useMemo(() => editorVariables(summary.variables), [summary.variables])
  const snippets: MarkdownSnippet[] = variables.map((v) => {
    const label = variableLabel(t, name, v.name)
    return {
      label: placeholder(v.name),
      text: placeholder(v.name),
      required: v.required,
      ariaLabel: t(v.required ? 'mailTemplates.snippetRequired' : 'mailTemplates.snippet', {
        placeholder: placeholder(v.name),
        label,
      }),
    }
  })

  const discard = () => {
    form.reset({ subject: template.subject, body: template.body })
    setFormError(undefined)
  }

  const resetToDefault = async () => {
    const ok = await confirm({
      title: t('mailTemplates.reset.title', { template: title }),
      description: t('mailTemplates.reset.hint', { locale: localeLabel }),
      confirmLabel: t('mailTemplates.reset.confirm'),
      tone: 'danger',
    })
    if (!ok) return
    setResetting(true)
    try {
      await adminApi.resetMailTemplate(name, locale)
      const fresh: AdminMailTemplate = {
        ...template,
        custom: false,
        updated_at: null,
        subject: template.default.subject,
        body: template.default.body,
      }
      qc.setQueryData(qk.admin.mailTemplate(name, locale), fresh)
      qc.setQueryData<AdminMailTemplateSummary[]>(qk.admin.mailTemplates(), (old) => patchSummary(old, fresh))
      form.reset({ subject: fresh.subject, body: fresh.body })
      setFormError(undefined)
      toast.success(t('mailTemplates.reset.done', { template: title, locale: localeLabel }))
      void qc.invalidateQueries({ queryKey: qk.admin.mailTemplates() })
    } catch (e) {
      toast.error(errorMessage(e))
    } finally {
      setResetting(false)
    }
  }

  const required = (msg: string) => ({
    onSubmit: ({ value }: { value: string }) => (value.trim() ? undefined : msg),
  })

  return (
    <>
      <form
        noValidate
        aria-label={t('mailTemplates.formLabel', { template: title, locale: localeLabel })}
        className="flex min-w-0 flex-col gap-6"
        onSubmit={(e) => {
          e.preventDefault()
          void form.handleSubmit()
        }}
        onKeyDown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
            e.preventDefault()
            if (dirty) void form.handleSubmit()
          }
        }}
      >
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="flex min-w-0 flex-wrap items-center gap-2 text-footnote text-label-secondary">
            <Chip size="sm" variant="soft" color={template.custom ? 'accent' : 'default'}>
              {template.custom ? t('mailTemplates.status.custom') : t('mailTemplates.status.default')}
            </Chip>
            <span>
              {template.custom && template.updated_at
                ? t('mailTemplates.status.updatedAt', {
                    time: formatDate(template.updated_at, uiLocale, 'datetime'),
                  })
                : t('mailTemplates.status.builtin')}
            </span>
          </p>
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" size="sm" onPress={() => setTestOpen(true)}>
              <MailCheck className="size-4" aria-hidden="true" />
              {t('mailTemplates.actions.test')}
            </Button>
            <Button
              variant="tertiary"
              size="sm"
              isDisabled={!template.custom || resetting}
              isPending={resetting}
              onPress={() => void resetToDefault()}
            >
              <RotateCcw className="size-4" aria-hidden="true" />
              {t('mailTemplates.actions.reset')}
            </Button>
          </div>
        </div>

        <FormError message={formError} />

        <div className="surface-card flex flex-col gap-5 p-5 sm:p-6">
          <form.AppField
            name="subject"
            validators={{
              onSubmit: ({ value }) =>
                !value.trim()
                  ? t('mailTemplates.fields.subjectRequired')
                  : value.trim().length > SUBJECT_MAX
                    ? t('mailTemplates.fields.subjectTooLong', { max: SUBJECT_MAX })
                    : undefined,
            }}
          >
            {(f) => (
              <f.TextField
                label={t('mailTemplates.fields.subject')}
                description={t('mailTemplates.fields.subjectHint')}
                isRequired
                maxLength={SUBJECT_MAX}
                autoComplete="off"
              />
            )}
          </form.AppField>
          <form.Field name="body" validators={required(t('mailTemplates.fields.bodyRequired'))}>
            {(f) => (
              <MarkdownEditor
                label={t('mailTemplates.fields.body')}
                name="body"
                value={f.state.value}
                onChange={(v) => {
                  f.handleChange(v)
                  if (f.state.meta.errorMap.onServer)
                    f.setMeta((m) => ({ ...m, errorMap: { ...m.errorMap, onServer: undefined } }))
                }}
                isRequired
                rows={14}
                snippets={snippets}
                help={<VariablesHelp template={name} variables={variables} />}
                error={fieldErrorMessage(f.state.meta)}
              />
            )}
          </form.Field>
        </div>

        <PreviewPanel name={name} draft={draft} />

        <SaveBar
          dirty={dirty}
          saving={isSubmitting}
          dirtyLabel={t('mailTemplates.save.dirty')}
          cleanLabel={template.custom ? t('mailTemplates.save.clean') : t('mailTemplates.save.cleanDefault')}
          hint={t('mailTemplates.save.shortcut')}
          saveLabel={t('common:actions.save')}
          discardLabel={t('mailTemplates.actions.discard')}
          onDiscard={discard}
        />
      </form>

      {/* Outside the <form>: React events from the portal would otherwise bubble into its handlers. */}
      {testOpen ? (
        <Suspense fallback={null}>
          <TestTemplateDialog
            isOpen={testOpen}
            onOpenChange={setTestOpen}
            name={name}
            title={`${title} · ${localeLabel}`}
            draft={draft}
            onDraftErrors={(e) => {
              applyServerErrors(form, e)
            }}
          />
        </Suspense>
      ) : null}
    </>
  )
}

/** Behind the toolbar's ⓘ: what placeholders do, then each variable with its meaning. */
function VariablesHelp({
  template,
  variables,
}: {
  template: string
  variables: readonly MailTemplateVariable[]
}) {
  const t = useT('admin-settings')
  return (
    <div className="flex flex-col gap-2">
      <p>{t('mailTemplates.placeholdersHelp')}</p>
      <ul className="flex flex-col gap-0.5" aria-label={t('mailTemplates.variablesLabel')}>
        {variables.map((v) => (
          <li key={v.name} className="flex flex-wrap items-baseline gap-x-1.5">
            <code className="font-mono text-label">{placeholder(v.name)}</code>
            <span className="text-label-secondary">
              {variableLabel(t, template, v.name)}
              {v.required ? (
                <span className="text-label-red">
                  {' '}
                  <span aria-hidden="true">*</span>
                  <span className="sr-only">{t('mailTemplates.required')}</span>
                </span>
              ) : null}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

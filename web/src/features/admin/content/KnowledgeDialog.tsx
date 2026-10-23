import { Button, Modal, Skeleton } from '@heroui/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { MarkdownEditor, type MarkdownSnippet } from '@/components/editor/MarkdownEditor'
import { FormError, useAppForm } from '@/components/form'
import { fieldErrorMessage } from '@/components/form/fieldError'
import { submitError } from '@/components/form/submitError'
import { ComboInput } from '@/features/admin/settings/inputs/ComboInput'
import { useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { AdminKnowledge, AdminKnowledgeDetail, KnowledgeInput } from '@/lib/api/types'
import { toast } from '@/lib/toast'
import { isAdmin, useAuthStore } from '@/stores/auth'
import { ACCESS_END, ACCESS_START, knowledgePlaceholders, LANGUAGE_SUGGESTIONS } from './knowledge'
import { ModalClose } from '@/components/feedback/ModalClose'

interface KnowledgeDialogProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  /** List row to edit (the body is fetched). */
  article?: AdminKnowledge | null
  /** Existing categories per language (suggestions). */
  categories: ReadonlyMap<string, readonly string[]>
  defaultLanguage?: string
}

export function KnowledgeDialog(props: KnowledgeDialogProps) {
  const t = useT(['admin-support', 'common'])
  const id = props.article?.id
  const detail = useQuery({
    queryKey: qk.admin.article(id ?? 0),
    queryFn: ({ signal }) => adminApi.knowledge(id ?? 0, signal),
    enabled: props.isOpen && id !== undefined,
  })
  const loading = id !== undefined && !detail.data
  return (
    <Modal.Backdrop isOpen={props.isOpen} onOpenChange={props.onOpenChange}>
      <Modal.Container size="lg" scroll="inside">
        <Modal.Dialog>
          <ModalClose />
          <Modal.Header>
            <Modal.Heading>{id ? t('knowledge.editTitle') : t('knowledge.createTitle')}</Modal.Heading>
          </Modal.Header>
          {loading ? (
            <Modal.Body className="flex flex-col gap-3" aria-busy="true">
              {detail.isError ? (
                <p className="text-subhead text-label-red">{t('knowledge.loadFailed')}</p>
              ) : (
                <>
                  <Skeleton className="h-10 w-full rounded-input" />
                  <Skeleton className="h-10 w-full rounded-input" />
                  <Skeleton className="h-64 w-full rounded-input" />
                </>
              )}
            </Modal.Body>
          ) : (
            <KnowledgeForm key={id ?? 'new'} {...props} detail={detail.data} />
          )}
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}

function KnowledgeForm({
  onOpenChange,
  detail,
  categories,
  defaultLanguage,
}: KnowledgeDialogProps & { detail?: AdminKnowledgeDetail }) {
  const t = useT(['admin-support', 'common'])
  const qc = useQueryClient()
  const admin = useAuthStore((s) => isAdmin(s.user))
  const [formError, setFormError] = useState<string>()
  const form = useAppForm({
    defaultValues: {
      language: detail?.language ?? defaultLanguage ?? 'zh-CN',
      category: detail?.category ?? '',
      title: detail?.title ?? '',
      body: detail?.body ?? '',
      show: detail?.show ?? true,
    },
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      const input: KnowledgeInput = {
        ...value,
        language: value.language.trim(),
        category: value.category.trim(),
        title: value.title.trim(),
      }
      try {
        if (detail) await adminApi.updateKnowledge(detail.id, input)
        else await adminApi.createKnowledge(input)
        toast.success(detail ? t('knowledge.saved') : t('knowledge.created'))
        await qc.invalidateQueries({ queryKey: ['admin', 'knowledge'] })
        onOpenChange(false)
      } catch (e) {
        setFormError(submitError(formApi, e))
      }
    },
  })

  const snippets: MarkdownSnippet[] = [
    // Staff get no subscription-URL placeholders: the API refuses them (403 + `errors.body`, shown on the body field).
    ...knowledgePlaceholders(admin).map((p) => ({ label: p, text: p })),
    {
      label: t('knowledge.accessGate'),
      text: t('knowledge.accessSample'),
      wrap: [`${ACCESS_START}\n`, `\n${ACCESS_END}`] as const,
    },
  ]

  const required = (msg: string) => ({
    onSubmit: ({ value }: { value: string }) => (value.trim() ? undefined : msg),
  })

  return (
    <form
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        void form.handleSubmit()
      }}
    >
      <Modal.Body className="flex flex-col gap-4">
        <FormError message={formError} />
        <div className="grid gap-4 sm:grid-cols-2">
          <form.Field name="language" validators={required(t('knowledge.languageRequired'))}>
            {(f) => (
              <ComboInput
                label={t('knowledge.fields.language')}
                name="language"
                value={f.state.value}
                onChange={(v) => f.handleChange(v)}
                options={LANGUAGE_SUGGESTIONS.map((l) => ({ value: l, hint: t(`knowledge.languages.${l}`) }))}
                isRequired
                description={t('knowledge.languageHint')}
                error={fieldErrorMessage(f.state.meta)}
              />
            )}
          </form.Field>
          <form.Subscribe selector={(s) => s.values.language}>
            {(language) => (
              <form.Field name="category" validators={required(t('knowledge.categoryRequired'))}>
                {(f) => (
                  <ComboInput
                    label={t('knowledge.fields.category')}
                    name="category"
                    value={f.state.value}
                    onChange={(v) => f.handleChange(v)}
                    options={categories.get(language) ?? []}
                    isRequired
                    description={t('knowledge.categoryHint')}
                    error={fieldErrorMessage(f.state.meta)}
                  />
                )}
              </form.Field>
            )}
          </form.Subscribe>
        </div>
        <form.AppField name="title" validators={required(t('knowledge.titleRequired'))}>
          {(f) => <f.TextField label={t('knowledge.fields.title')} isRequired maxLength={255} />}
        </form.AppField>
        <form.Field name="body" validators={required(t('knowledge.bodyRequired'))}>
          {(f) => (
            <MarkdownEditor
              label={t('knowledge.fields.body')}
              name="body"
              value={f.state.value}
              onChange={(v) => f.handleChange(v)}
              isRequired
              rows={14}
              snippets={snippets}
              help={t('knowledge.placeholdersHelp')}
              error={fieldErrorMessage(f.state.meta)}
            />
          )}
        </form.Field>
        <form.AppField name="show">
          {(f) => <f.SwitchField label={t('knowledge.fields.show')} description={t('knowledge.showHint')} />}
        </form.AppField>
      </Modal.Body>
      <Modal.Footer>
        <Button slot="close" variant="secondary">
          {t('common:actions.cancel')}
        </Button>
        <form.AppForm>
          <form.SubmitButton size="md">
            {detail ? t('common:actions.save') : t('knowledge.publish')}
          </form.SubmitButton>
        </form.AppForm>
      </Modal.Footer>
    </form>
  )
}

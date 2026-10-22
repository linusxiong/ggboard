import { Button, Modal } from '@heroui/react'
import { useQueryClient } from '@tanstack/react-query'
import { ImageOff } from 'lucide-react'
import { useState } from 'react'
import { MarkdownEditor } from '@/components/editor/MarkdownEditor'
import { FormError, useAppForm } from '@/components/form'
import { fieldErrorMessage } from '@/components/form/fieldError'
import { submitError } from '@/components/form/submitError'
import { TextInput } from '@/features/admin/settings/inputs/Controlled'
import { TagInput } from '@/features/admin/settings/inputs/TagInput'
import { useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { AdminNotice, NoticeInput } from '@/lib/api/types'
import { toast } from '@/lib/toast'
import { ModalClose } from '@/components/feedback/ModalClose'

interface NoticeDialogProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  notice?: AdminNotice | null
}

export function NoticeDialog(props: NoticeDialogProps) {
  return <NoticeDialogInner key={props.isOpen ? (props.notice?.id ?? 'new') : 'closed'} {...props} />
}

const isUrl = (v: string) => /^(https?:\/\/|\/)\S+$/.test(v)

function NoticeDialogInner({ isOpen, onOpenChange, notice }: NoticeDialogProps) {
  const t = useT(['admin-support', 'common'])
  const qc = useQueryClient()
  const [formError, setFormError] = useState<string>()
  const form = useAppForm({
    defaultValues: {
      title: notice?.title ?? '',
      content: notice?.content ?? '',
      img_url: notice?.img_url ?? '',
      tags: notice?.tags ?? ([] as string[]),
      show: notice?.show ?? true,
    },
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      const input: NoticeInput = { ...value, title: value.title.trim(), img_url: value.img_url.trim() }
      try {
        if (notice) await adminApi.updateNotice(notice.id, input)
        else await adminApi.createNotice(input)
        toast.success(notice ? t('notices.saved') : t('notices.created'))
        await qc.invalidateQueries({ queryKey: qk.admin.notices() })
        onOpenChange(false)
      } catch (e) {
        setFormError(submitError(formApi, e))
      }
    },
  })

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="lg" scroll="inside">
        <Modal.Dialog>
          <ModalClose />
          <Modal.Header>
            <Modal.Heading>{notice ? t('notices.editTitle') : t('notices.createTitle')}</Modal.Heading>
          </Modal.Header>
          <form
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              void form.handleSubmit()
            }}
          >
            <Modal.Body className="flex flex-col gap-4">
              <FormError message={formError} />
              <form.AppField
                name="title"
                validators={{
                  onSubmit: ({ value }) => (value.trim() ? undefined : t('notices.titleRequired')),
                }}
              >
                {(f) => (
                  <f.TextField label={t('notices.fields.title')} isRequired autoFocus maxLength={255} />
                )}
              </form.AppField>
              <form.Field
                name="content"
                validators={{
                  onSubmit: ({ value }) => (value.trim() ? undefined : t('notices.contentRequired')),
                }}
              >
                {(f) => (
                  <MarkdownEditor
                    label={t('notices.fields.content')}
                    name="content"
                    value={f.state.value}
                    onChange={(v) => f.handleChange(v)}
                    isRequired
                    rows={10}
                    error={fieldErrorMessage(f.state.meta)}
                  />
                )}
              </form.Field>
              <form.Field
                name="img_url"
                validators={{
                  onSubmit: ({ value }) =>
                    value.trim() && !isUrl(value.trim()) ? t('notices.urlInvalid') : undefined,
                }}
              >
                {(f) => (
                  <div className="flex items-start gap-3">
                    <TextInput
                      label={t('notices.fields.img_url')}
                      name="img_url"
                      type="url"
                      value={f.state.value}
                      onChange={(v) => f.handleChange(v)}
                      placeholder="https://"
                      description={t('notices.imgHint')}
                      error={fieldErrorMessage(f.state.meta)}
                      className="min-w-0 flex-1"
                    />
                    <span className="mt-6 flex h-16 w-28 shrink-0 items-center justify-center overflow-hidden rounded-[10px] bg-fill-tertiary">
                      {isUrl(f.state.value.trim()) ? (
                        <img
                          src={f.state.value.trim()}
                          alt={t('notices.imgPreview')}
                          width={112}
                          height={64}
                          className="h-16 w-28 object-cover"
                        />
                      ) : (
                        <ImageOff className="size-4 text-label-secondary" aria-hidden />
                      )}
                    </span>
                  </div>
                )}
              </form.Field>
              <form.Field name="tags">
                {(f) => (
                  <TagInput
                    label={t('notices.fields.tags')}
                    name="tags"
                    value={f.state.value}
                    onChange={(v) => f.handleChange(v)}
                    description={t('notices.tagsHint')}
                    error={fieldErrorMessage(f.state.meta)}
                  />
                )}
              </form.Field>
              <form.AppField name="show">
                {(f) => (
                  <f.SwitchField label={t('notices.fields.show')} description={t('notices.showHint')} />
                )}
              </form.AppField>
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary">
                {t('common:actions.cancel')}
              </Button>
              <form.AppForm>
                <form.SubmitButton size="md">
                  {notice ? t('common:actions.save') : t('notices.publish')}
                </form.SubmitButton>
              </form.AppForm>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}

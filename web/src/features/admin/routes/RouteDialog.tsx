import { Button, Modal } from '@heroui/react'
import { useState } from 'react'
import { FormError, useAppForm } from '@/components/form'
import { submitError } from '@/components/form/submitError'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { qk } from '@/lib/api/queries'
import type { NodeRoute } from '@/lib/api/types'
import { useAdminMutation } from '../shared/useAdminMutation'
import { checkMatch, MAX_MATCH, routeToValues, valuesToRouteInput } from './routeForm'

interface RouteDialogProps {
  /** `null` = create. */
  route: NodeRoute | null
  isOpen: boolean
  onOpenChange: (open: boolean) => void
}

export function RouteDialog({ route, isOpen, onOpenChange }: RouteDialogProps) {
  const t = useT(['admin-nodes', 'common'])
  const [formError, setFormError] = useState<string>()
  const save = useAdminMutation({
    mutationFn: (input: ReturnType<typeof valuesToRouteInput>) =>
      route ? adminApi.updateRoute(route.id, input) : adminApi.createRoute(input),
    success: route ? t('routes.updated') : t('routes.created'),
    invalidate: [qk.admin.routes(), qk.admin.nodes()],
    silentError: true,
  })

  const form = useAppForm({
    defaultValues: routeToValues(route),
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      try {
        await save.mutateAsync(valuesToRouteInput(value))
        onOpenChange(false)
      } catch (error) {
        setFormError(submitError(formApi, error))
      }
    },
  })

  const matchMessage = (text: string, action: 'block' | 'dns') => {
    const issue = checkMatch(text, action)
    if (!issue) return undefined
    if (issue.key === 'tooMany') return t('routes.errors.tooMany', { max: issue.max })
    if (issue.key === 'regexp') return t('routes.errors.regexp', { detail: issue.detail })
    return t('routes.errors.required')
  }

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="md" scroll="inside">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <Modal.Header>
            <Modal.Heading>{route ? t('routes.edit') : t('routes.create')}</Modal.Heading>
          </Modal.Header>
          <form
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              void form.handleSubmit()
            }}
            className="contents"
          >
            <Modal.Body className="flex flex-col gap-4">
              <FormError message={formError} />
              <form.AppField name="remarks">
                {(f) => (
                  <f.TextField label={t('routes.remarks')} placeholder={t('routes.remarksPlaceholder')} />
                )}
              </form.AppField>
              <form.AppField name="action">
                {(f) => (
                  <f.SelectField
                    label={t('routes.action')}
                    options={[
                      { value: 'block', label: t('routes.actions.block') },
                      { value: 'dns', label: t('routes.actions.dns') },
                    ]}
                    description={t('routes.actionHint')}
                  />
                )}
              </form.AppField>
              <form.Subscribe selector={(s) => s.values.action}>
                {(action) => (
                  <>
                    <form.AppField
                      name="match"
                      validators={{
                        onChangeListenTo: ['action'],
                        onChange: ({ value, fieldApi }) =>
                          fieldApi.state.meta.isTouched ? matchMessage(value, action) : undefined,
                        onSubmit: ({ value }) => matchMessage(value, action),
                      }}
                    >
                      {(f) => (
                        <f.TextAreaField
                          label={t('routes.match')}
                          rows={6}
                          placeholder={
                            action === 'dns'
                              ? 'netflix.com\nnflxvideo.net'
                              : '(.*\\.)?doubleclick\\.net\ndomain:ads.example.com'
                          }
                          description={t(action === 'dns' ? 'routes.matchHintDns' : 'routes.matchHintBlock', {
                            max: MAX_MATCH,
                          })}
                          isRequired
                        />
                      )}
                    </form.AppField>
                    {action === 'dns' ? (
                      <form.AppField
                        name="action_value"
                        validators={{
                          onSubmit: ({ value }) =>
                            value.trim() ? undefined : t('routes.errors.dnsRequired'),
                        }}
                      >
                        {(f) => (
                          <f.TextField
                            label={t('routes.dnsServer')}
                            placeholder="1.1.1.1"
                            description={t('routes.dnsHint')}
                            isRequired
                          />
                        )}
                      </form.AppField>
                    ) : null}
                  </>
                )}
              </form.Subscribe>
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="tertiary">
                {t('common:actions.cancel')}
              </Button>
              <form.AppForm>
                <form.SubmitButton size="md">{t('common:actions.save')}</form.SubmitButton>
              </form.AppForm>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}

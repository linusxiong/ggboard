/**
 * Profile switcher (Sub-Store-like multiple subscriptions): tabs for the default + user profiles, "new
 * profile", and a menu for the active one (rename / duplicate / set default / delete). The active profile's
 * content is rendered in the tab panel by the page.
 */
import { Button, Dropdown, FieldError, Input, Label, Modal, TextField } from '@heroui/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Copy, Ellipsis, PenLine, Plus, Star, Trash2 } from 'lucide-react'
import { type FormEvent, type ReactNode, useState } from 'react'
import { FormError } from '@/components/FormError'
import { confirm } from '@/components/feedback/confirm'
import { ModalClose } from '@/components/feedback/ModalClose'
import { FieldHint, FieldLabel } from '@/components/form/parts'
import { HelpTip } from '@/components/HelpTip'
import { SegmentedTabs } from '@/components/SegmentedControl'
import { useT } from '@/i18n'
import { isApiError } from '@/lib/api/client'
import { qk } from '@/lib/api/queries'
import type { RuleProfileSummary } from '@/lib/api/types'
import {
  createRuleProfile,
  deleteRuleProfile,
  setDefaultRuleProfile,
  updateRuleProfile,
} from '@/lib/api/user'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { OptionSelect } from './controls'
import { MAX_PROFILES, profileIssues, suggestSlug, tabKey } from './profiles'
import { profileKey, profilesKey } from './queries'

/** Sentinel select key for "start from the site's default template" (keys must not be empty). */
const TEMPLATE_BASE = '__template__'
const tabSlug = (p: RuleProfileSummary) => p.slug || TEMPLATE_BASE

type Dialog = { mode: 'create' } | { mode: 'duplicate' } | { mode: 'rename' } | null

export interface ProfileTabsProps {
  profiles: RuleProfileSummary[]
  active: RuleProfileSummary
  /** Template ids: `?profile=` also accepts them, so slugs must not shadow one. */
  reserved: readonly string[]
  /** Management needs the profiles API and `allow_user_rule_profile`. */
  canManage: boolean
  onSelect: (profile: RuleProfileSummary) => void
  children: ReactNode
}

export function ProfileTabs({ profiles, active, reserved, canManage, onSelect, children }: ProfileTabsProps) {
  const t = useT(['subscription', 'common'])
  const queryClient = useQueryClient()
  const [dialog, setDialog] = useState<Dialog>(null)
  const label = (p: RuleProfileSummary) => p.name || t('profiles.default')
  const full = profiles.filter((p) => p.slug).length >= MAX_PROFILES

  const refresh = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: profilesKey() }),
      queryClient.invalidateQueries({ queryKey: qk.user.subscription() }),
    ])

  const makeDefault = useMutation({
    mutationFn: (p: RuleProfileSummary) => setDefaultRuleProfile(p.slug),
    onSuccess: async (_r, p) => {
      toast.success(t('profiles.defaultChanged', { name: label(p) }))
      await Promise.all([refresh(), queryClient.invalidateQueries({ queryKey: qk.user.ruleProfile() })])
    },
    onError: (error) => toast.danger(errorMessage(error)),
  })
  const remove = useMutation({
    mutationFn: (p: RuleProfileSummary) => deleteRuleProfile(p.slug),
    onSuccess: async (_r, p) => {
      toast.success(t('profiles.deleted', { name: label(p) }))
      queryClient.removeQueries({ queryKey: profileKey(p.slug) })
      const fallback = profiles.find((x) => x.is_default)
      if (fallback) onSelect(fallback)
      await refresh()
    },
    onError: (error) => toast.danger(errorMessage(error)),
  })

  const onAction = async (action: string) => {
    if (action === 'rename' || action === 'duplicate') return setDialog({ mode: action })
    if (action === 'default') {
      const ok = await confirm({
        title: t('profiles.setDefaultTitle', { name: label(active) }),
        description: t('profiles.setDefaultHint'),
        confirmLabel: t('profiles.setDefaultConfirm'),
      })
      if (ok) makeDefault.mutate(active)
    }
    if (action === 'delete') {
      const ok = await confirm({
        title: t('profiles.deleteTitle', { name: label(active) }),
        description: t('profiles.deleteHint'),
        confirmLabel: t('profiles.delete'),
        tone: 'danger',
      })
      if (ok) remove.mutate(active)
    }
  }

  const disabledKeys = [
    ...(active.slug ? [] : ['rename', 'duplicate']),
    ...(active.is_default || !active.slug ? ['default', 'delete'] : []),
    ...(full ? ['duplicate'] : []),
  ]

  return (
    <>
      <SegmentedTabs
        optimistic={false}
        value={tabKey(active)}
        onChange={(key) => {
          const next = profiles.find((p) => tabKey(p) === key)
          if (next && next !== active) onSelect(next)
        }}
        className="flex flex-col gap-4"
      >
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <SegmentedTabs.List
              aria-label={t('profiles.tabs')}
              className="min-w-0"
              items={profiles.map((p) => ({
                id: tabKey(p),
                label: (
                  <>
                    {p.is_default ? (
                      <Star className="size-3.5 fill-current" aria-label={t('profiles.defaultBadge')} />
                    ) : null}
                    {label(p)}
                  </>
                ),
              }))}
            />
            {canManage ? (
              <div className="flex items-center gap-2">
                <HelpTip subject={t('profiles.tabs')}>{t('profiles.hint')}</HelpTip>
                <Button
                  size="sm"
                  variant="secondary"
                  isDisabled={full}
                  onPress={() => setDialog({ mode: 'create' })}
                >
                  <Plus className="size-4" aria-hidden="true" />
                  {t('profiles.create')}
                </Button>
                <Dropdown>
                  <Button
                    size="sm"
                    variant="ghost"
                    isIconOnly
                    aria-label={t('profiles.actions', { name: label(active) })}
                  >
                    <Ellipsis className="size-4" aria-hidden="true" />
                  </Button>
                  <Dropdown.Popover placement="bottom end">
                    <Dropdown.Menu disabledKeys={disabledKeys} onAction={(key) => void onAction(String(key))}>
                      <Dropdown.Item id="rename" textValue={t('profiles.rename')}>
                        <PenLine className="size-4" aria-hidden="true" />
                        <Label>{t('profiles.rename')}</Label>
                      </Dropdown.Item>
                      <Dropdown.Item id="duplicate" textValue={t('profiles.duplicate')}>
                        <Copy className="size-4" aria-hidden="true" />
                        <Label>{t('profiles.duplicate')}</Label>
                      </Dropdown.Item>
                      <Dropdown.Item id="default" textValue={t('profiles.setDefault')}>
                        <Star className="size-4" aria-hidden="true" />
                        <Label>{t('profiles.setDefault')}</Label>
                      </Dropdown.Item>
                      <Dropdown.Item id="delete" textValue={t('profiles.delete')} variant="danger">
                        <Trash2 className="size-4" aria-hidden="true" />
                        <Label>{t('profiles.delete')}</Label>
                      </Dropdown.Item>
                    </Dropdown.Menu>
                  </Dropdown.Popover>
                </Dropdown>
              </div>
            ) : null}
          </div>
          {canManage && full ? (
            <p className="text-footnote text-label-secondary">{t('profiles.limit', { max: MAX_PROFILES })}</p>
          ) : null}
        </div>

        <SegmentedTabs.Panel id={tabKey(active)} className="flex flex-col gap-6">
          {children}
        </SegmentedTabs.Panel>
      </SegmentedTabs>
      {/* Outside <Tabs>: React events from the portal would otherwise bubble into the tab list's handlers. */}
      {dialog ? (
        <ProfileDialog
          mode={dialog.mode}
          profiles={profiles}
          active={active}
          reserved={reserved}
          onClose={() => setDialog(null)}
          onCreated={(created) => {
            setDialog(null)
            onSelect(created)
          }}
        />
      ) : null}
    </>
  )
}

function ProfileDialog({
  mode,
  profiles,
  active,
  reserved,
  onClose,
  onCreated,
}: {
  mode: 'create' | 'duplicate' | 'rename'
  profiles: RuleProfileSummary[]
  active: RuleProfileSummary
  reserved: readonly string[]
  onClose: () => void
  onCreated: (profile: RuleProfileSummary) => void
}) {
  const t = useT(['subscription', 'common'])
  const queryClient = useQueryClient()
  const label = (p: RuleProfileSummary) => p.name || t('profiles.default')
  const [name, setName] = useState(mode === 'rename' ? active.name : '')
  const [slug, setSlug] = useState('')
  const [base, setBase] = useState(mode === 'duplicate' ? tabSlug(active) : TEMPLATE_BASE)
  const [touched, setTouched] = useState(false)
  const [serverErrors, setServerErrors] = useState<Partial<Record<'name' | 'slug', string>>>({})
  const [formError, setFormError] = useState<string>()
  const [pending, setPending] = useState(false)

  const local = profileIssues(
    { name, slug: mode === 'rename' ? '' : slug },
    profiles,
    reserved,
    mode === 'rename' ? active : undefined,
  )
  const fieldError = (field: 'name' | 'slug') => {
    const issue = local[field]
    if (issue && (touched || (field === 'slug' && slug))) return t(`profiles.issues.${issue}`)
    return serverErrors[field]
  }
  const suggestion = suggestSlug(name, profiles, reserved)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setTouched(true)
    if (Object.keys(local).length > 0 || pending) return
    setPending(true)
    setFormError(undefined)
    try {
      if (mode === 'rename') {
        await updateRuleProfile(active.slug, { name: name.trim() })
        toast.success(t('profiles.renamed', { name: name.trim() }))
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: profilesKey() }),
          queryClient.invalidateQueries({ queryKey: qk.user.subscription() }),
          queryClient.invalidateQueries({ queryKey: qk.user.ruleProfile() }),
        ])
        onClose()
      } else {
        const chosenSlug = slug.trim() || suggestion
        const created = await createRuleProfile({
          name: name.trim(),
          ...(chosenSlug ? { slug: chosenSlug } : {}),
          ...(base !== TEMPLATE_BASE ? { copy_from: base } : {}),
        })
        toast.success(t('profiles.created', { name: created.name || name.trim() }))
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: profilesKey() }),
          queryClient.invalidateQueries({ queryKey: qk.user.subscription() }),
        ])
        onCreated(created)
      }
    } catch (error) {
      if (isApiError(error) && (error.fieldError('name') || error.fieldError('slug'))) {
        setServerErrors({
          ...(error.fieldError('name') ? { name: error.fieldError('name') } : {}),
          ...(error.fieldError('slug') ? { slug: error.fieldError('slug') } : {}),
        })
      } else setFormError(errorMessage(error))
    } finally {
      setPending(false)
    }
  }

  const title =
    mode === 'create'
      ? t('profiles.createTitle')
      : mode === 'rename'
        ? t('profiles.renameTitle', { name: label(active) })
        : t('profiles.duplicateTitle', { name: label(active) })

  return (
    <Modal.Backdrop isOpen onOpenChange={(open) => (open ? undefined : onClose())}>
      <Modal.Container size="md" scroll="inside">
        <Modal.Dialog>
          <ModalClose />
          <Modal.Header>
            <Modal.Heading>{title}</Modal.Heading>
          </Modal.Header>
          <form noValidate onSubmit={(e) => void submit(e)} className="contents">
            <Modal.Body className="flex flex-col gap-4">
              <FormError message={formError} />
              <TextField
                name="profile-name"
                value={name}
                onChange={(value) => {
                  setName(value)
                  setServerErrors((e) => ({ ...e, name: undefined }))
                }}
                isInvalid={Boolean(fieldError('name'))}
                validationBehavior="aria"
                variant="secondary"
                autoFocus
                isRequired
              >
                <Label>{t('profiles.name')}</Label>
                <Input autoComplete="off" placeholder={t('profiles.namePlaceholder')} maxLength={64} />
                <FieldError>{fieldError('name')}</FieldError>
              </TextField>
              {mode === 'rename' ? (
                <p className="text-footnote text-label-secondary">
                  {t('profiles.slugFixed', { slug: active.slug })}
                </p>
              ) : (
                <TextField
                  name="profile-slug"
                  value={slug}
                  onChange={(value) => {
                    setSlug(value.toLowerCase())
                    setServerErrors((e) => ({ ...e, slug: undefined }))
                  }}
                  isInvalid={Boolean(fieldError('slug'))}
                  validationBehavior="aria"
                  variant="secondary"
                >
                  <FieldLabel label={t('profiles.slug')} help={t('profiles.slugHint')} />
                  <Input
                    autoComplete="off"
                    spellCheck={false}
                    autoCapitalize="none"
                    placeholder={suggestion || 'my-profile'}
                    maxLength={32}
                    className="font-mono"
                  />
                  {fieldError('slug') ? null : <FieldHint>{t('profiles.slugHint')}</FieldHint>}
                  <FieldError>{fieldError('slug')}</FieldError>
                </TextField>
              )}
              {mode === 'create' ? (
                <OptionSelect
                  label={t('profiles.base')}
                  value={base}
                  onChange={setBase}
                  variant="secondary"
                  options={[
                    { value: TEMPLATE_BASE, label: t('profiles.baseTemplate') },
                    ...profiles
                      .filter((p) => p.slug)
                      .map((p) => ({ value: p.slug, label: t('profiles.baseCopy', { name: label(p) }) })),
                  ]}
                />
              ) : null}
            </Modal.Body>
            <Modal.Footer className="mt-5">
              <Button slot="close" variant="secondary">
                {t('common:actions.cancel')}
              </Button>
              <Button type="submit" isPending={pending}>
                {mode === 'rename' ? t('profiles.submitRename') : t('profiles.submitCreate')}
              </Button>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}

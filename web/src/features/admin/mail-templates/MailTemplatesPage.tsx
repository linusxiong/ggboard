import { Alert, Button, Skeleton } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { MailX } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { confirm } from '@/components/feedback/confirm'
import { PageShell } from '@/components/PageShell'
import { SegmentedControl } from '@/components/SegmentedControl'
import { useLocale, useT } from '@/i18n'
import { isApiError } from '@/lib/api'
import { adminMailTemplateQuery, adminMailTemplatesQuery } from '@/lib/api/queries'
import type { Locale } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { MAIL_LOCALES } from './search'
import { TemplateEditor } from './TemplateEditor'
import { TemplateList } from './TemplateList'
import { templateDescription, templateTitle } from './variables'

export interface MailTemplateSelection {
  name: string
  locale: Locale
}

interface MailTemplatesPageProps {
  /** Selected template (URL `?name=`); unknown or missing → the first one. */
  name?: string
  /** Selected locale (URL `?locale=`); missing → the interface language. */
  locale?: Locale
  /** Commits a new selection (the route writes it to the URL). Unsaved edits are confirmed before. */
  onSelect: (next: MailTemplateSelection) => void
  /** Rendered with the dirty state (the route mounts a leave-page guard there). */
  renderGuard?: (dirty: boolean) => ReactNode
  /** Link back to Settings → Email (a router `<Link>`). */
  settingsLink?: ReactNode
}

/**
 * Admin e-mail templates, System Settings style: the six templates on the left (state per locale), the
 * selected one on the right — locale switch, subject + Markdown body with `{{variable}}` buttons, live
 * preview, test mail, reset to the built-in template. Switching template or locale with unsaved edits asks
 * first (there is one draft at a time).
 */
export function MailTemplatesPage({
  name,
  locale,
  onSelect,
  renderGuard,
  settingsLink,
}: MailTemplatesPageProps) {
  const t = useT(['admin-settings', 'common'])
  const uiLocale = useLocale()
  const list = useQuery(adminMailTemplatesQuery())
  const [dirty, setDirty] = useState(false)

  const items = list.data ?? []
  const active = items.find((i) => i.name === name) ?? items[0]
  const activeLocale: Locale = locale ?? (MAIL_LOCALES.includes(uiLocale) ? uiLocale : 'zh-CN')
  const detail = useQuery({
    ...adminMailTemplateQuery(active?.name ?? '', activeLocale),
    enabled: Boolean(active),
  })

  const select = async (next: Partial<MailTemplateSelection>) => {
    if (!active) return
    const target = { name: next.name ?? active.name, locale: next.locale ?? activeLocale }
    if (target.name === active.name && target.locale === activeLocale) return
    if (dirty) {
      const discard = await confirm({
        title: t('mailTemplates.leave.title'),
        description: t('mailTemplates.leave.switchHint'),
        confirmLabel: t('mailTemplates.leave.confirm'),
        tone: 'danger',
      })
      if (!discard) return
    }
    onSelect(target)
  }

  const title = active ? templateTitle(t, active.name) : ''
  const unsupported = isApiError(list.error) && list.error.status === 404

  return (
    <PageShell
      title={t('mailTemplates.title')}
      description={t('mailTemplates.description')}
      actions={settingsLink}
    >
      {renderGuard?.(dirty)}
      {unsupported ? (
        <EmptyState
          icon={<MailX />}
          title={t('mailTemplates.unsupported')}
          description={t('mailTemplates.unsupportedHint')}
        />
      ) : list.isError ? (
        <Alert status="danger">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>{t('mailTemplates.loadFailed')}</Alert.Title>
            <Alert.Description>{errorMessage(list.error)}</Alert.Description>
          </Alert.Content>
          <Button size="sm" variant="secondary" onPress={() => void list.refetch()}>
            {t('common:actions.retry')}
          </Button>
        </Alert>
      ) : list.isPending ? (
        <div className="grid gap-8 md:grid-cols-[240px_minmax(0,1fr)]" aria-busy="true">
          <Skeleton className="hidden h-96 w-full rounded-card md:block" />
          <div className="flex flex-col gap-6">
            <Skeleton className="h-8 w-48" />
            <Skeleton className="h-96 w-full rounded-card" />
            <Skeleton className="h-64 w-full rounded-card" />
          </div>
        </div>
      ) : !active ? (
        <EmptyState icon={<MailX />} title={t('mailTemplates.empty')} />
      ) : (
        <div className="grid grid-cols-[minmax(0,1fr)] items-start gap-6 md:grid-cols-[240px_minmax(0,1fr)] md:gap-8">
          <TemplateList
            items={items}
            active={active.name}
            dirty={dirty}
            onSelect={(n) => void select({ name: n })}
          />

          <section aria-labelledby="mail-template-title" className="flex min-w-0 flex-col gap-6">
            <header className="flex flex-wrap items-end justify-between gap-3">
              <div className="min-w-0 flex-1 basis-60">
                <h2 id="mail-template-title" className="text-title2 text-label">
                  {title}
                </h2>
                {templateDescription(t, active.name) ? (
                  <p className="mt-1 text-subhead text-label-secondary">
                    {templateDescription(t, active.name)}
                  </p>
                ) : null}
              </div>
              <SegmentedControl
                aria-label={t('mailTemplates.localeLabel')}
                value={activeLocale}
                onChange={(l) => void select({ locale: l })}
                // A pick may be refused by the unsaved-changes confirm: show it only once committed.
                optimistic={!dirty}
                items={MAIL_LOCALES.map((l) => ({
                  id: l,
                  label: t(`mailTemplates.locales.${l}`),
                  adornment: active.locales[l]?.custom ? (
                    <span
                      className="size-1.5 rounded-full bg-system-blue"
                      role="img"
                      aria-label={t('mailTemplates.status.custom')}
                    />
                  ) : undefined,
                }))}
              />
            </header>

            {detail.isError ? (
              <Alert status="danger">
                <Alert.Indicator />
                <Alert.Content>
                  <Alert.Title>{t('mailTemplates.templateLoadFailed')}</Alert.Title>
                  <Alert.Description>{errorMessage(detail.error)}</Alert.Description>
                </Alert.Content>
                <Button size="sm" variant="secondary" onPress={() => void detail.refetch()}>
                  {t('common:actions.retry')}
                </Button>
              </Alert>
            ) : !detail.data ? (
              <div className="flex flex-col gap-6" aria-busy="true">
                <Skeleton className="h-8 w-64" />
                <Skeleton className="h-[30rem] w-full rounded-card" />
                <Skeleton className="h-[36rem] w-full rounded-card" />
              </div>
            ) : (
              <TemplateEditor
                key={`${detail.data.name}:${detail.data.locale}`}
                template={detail.data}
                summary={active}
                title={title}
                onDirtyChange={setDirty}
              />
            )}
          </section>
        </div>
      )}
    </PageShell>
  )
}

/**
 * age-encrypted subscriptions (`/user/subscription/age`): generate a key pair (it starts disabled), switch
 * encryption on/off, show the public key, reveal/copy the `AGE-SECRET-KEY-…` identity, regenerate or delete it,
 * and per-client setup (docs/SUBSCRIPTION.md → "Encrypted subscriptions (age)").
 */
import { Button, Chip } from '@heroui/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, Eye, EyeOff, KeyRound, Lock, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { HelpTip, TitleWithHelp } from '@/components/HelpTip'
import { DateTime } from '@/components/Formatters'
import { confirm } from '@/components/feedback/confirm'
import { useT } from '@/i18n'
import { isApiError } from '@/lib/api/client'
import type { AgeKey, SubscriptionFormatInfo } from '@/lib/api/types'
import { createAgeKey, deleteAgeKey, setAgeEnabled } from '@/lib/api/user'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { ageClients, ageSnippet } from './accessLog'
import { Toggle } from './controls'
import { ageKeyKey, ageKeyQuery, isMissing } from './queries'

/** `AGE-SECRET-KEY-1QQQ…` → `AGE-SECRET-KEY-1` + dots (nothing secret is rendered). */
const maskSecret = (key: string) => `${key.slice(0, 'AGE-SECRET-KEY-1'.length)}${'•'.repeat(24)}`

const NO_KEY = { public_key: null, private_key: null, created_at: null, enabled: false } as const

export function AgeCard({
  subscribeUrl,
  formats,
  optional,
  strict,
}: {
  /** Plain subscription link (for the proxy-providers example). */
  subscribeUrl: string | undefined
  /** `GET /user/subscription` → `formats` (fallback when the age view has no `formats`). */
  formats: readonly SubscriptionFormatInfo[] | undefined
  /** Guest config fallbacks for the age view's `optional` / `strict`. */
  optional: boolean | undefined
  strict: boolean | undefined
}) {
  const t = useT(['subscription', 'common'])
  const queryClient = useQueryClient()
  const query = useQuery(ageKeyQuery())
  const [revealed, setRevealed] = useState(false)
  const [unsupported, setUnsupported] = useState(false)
  const key = query.data ?? null
  const hasKey = Boolean(key?.public_key)
  const isStrict = key?.strict ?? strict ?? false
  const isOptional = key?.optional ?? optional ?? true

  const store = (next: AgeKey | null) => queryClient.setQueryData(ageKeyKey(), next)
  const onError = (error: unknown) => toast.danger(errorMessage(error))

  const create = useMutation({
    mutationFn: (regenerate: boolean) => createAgeKey(regenerate ? { regenerate: true } : {}),
    onSuccess: (result, regenerate) => {
      store(result)
      setRevealed(false)
      toast.success(regenerate ? t('age.regenerated') : t('age.generated'))
    },
    onError: (error) => {
      if (isMissing(error)) setUnsupported(true)
      else if (isApiError(error) && error.status === 409) void query.refetch()
      onError(error)
    },
  })
  const toggle = useMutation({
    mutationFn: (enabled: boolean) => setAgeEnabled(enabled),
    onMutate: (enabled) => {
      const previous = queryClient.getQueryData<AgeKey | null>(ageKeyKey())
      if (previous) store({ ...previous, enabled })
      return { previous }
    },
    onSuccess: (result, enabled) => {
      if (result && typeof result === 'object' && 'public_key' in result) store(result)
      toast.success(enabled ? t('age.enabled') : t('age.disabled'))
    },
    onError: (error, _enabled, context) => {
      if (context?.previous !== undefined) store(context.previous)
      onError(error)
    },
  })
  const remove = useMutation({
    mutationFn: deleteAgeKey,
    onSuccess: (result) => {
      const previous = queryClient.getQueryData<AgeKey | null>(ageKeyKey())
      store(
        result && typeof result === 'object' && 'public_key' in result
          ? (result as AgeKey)
          : previous
            ? { ...previous, ...NO_KEY }
            : null,
      )
      setRevealed(false)
      toast.success(t('age.removed'))
    },
    onError,
  })

  const regenerate = async () => {
    const ok = await confirm({
      title: t('age.regenerateTitle'),
      description: t('age.regenerateHint'),
      confirmLabel: t('age.regenerateConfirm'),
      tone: 'danger',
    })
    if (ok) create.mutate(true)
  }
  const removeKey = async () => {
    const ok = await confirm({
      title: t('age.removeTitle'),
      description: t('age.removeHint'),
      confirmLabel: t('age.remove'),
      tone: 'danger',
    })
    if (ok) remove.mutate()
  }

  const clients = ageClients(key?.formats, formats)
  const enabled = hasKey && Boolean(key?.enabled)
  const privateKey = key?.private_key ?? null
  // What it does, then which formats get encrypted and what the others receive (second level, ⓘ).
  const about = unsupported
    ? t('age.intro')
    : [
        t('age.intro'),
        '',
        t('age.clients', { clients: clients.join(' · ') }),
        isStrict ? t('age.strict') : t('age.plainFallback'),
        ...(isOptional ? [t('age.optional')] : []),
      ].join('\n')

  return (
    <section className="surface-card flex min-w-0 flex-col gap-4 p-6" aria-labelledby="age-title">
      <div className="flex items-start gap-3">
        <span
          className="flex size-9 shrink-0 items-center justify-center rounded-[10px] bg-fill-tertiary text-label-secondary"
          aria-hidden="true"
        >
          <Lock className="size-5" />
        </span>
        <TitleWithHelp help={about} subject={t('age.title')} className="self-center">
          <h2 id="age-title" className="flex flex-wrap items-center gap-2 text-headline text-label">
            {t('age.title')}
            {hasKey ? (
              <Chip size="sm" variant="soft" color={enabled ? 'success' : 'default'}>
                {enabled ? t('age.on') : t('age.off')}
              </Chip>
            ) : null}
          </h2>
        </TitleWithHelp>
      </div>

      {unsupported ? (
        <p className="rounded-[12px] bg-fill-quaternary px-4 py-3 text-subhead text-label-secondary">
          {t('age.unavailable')}
        </p>
      ) : query.isPending ? (
        <div className="h-28 shimmer rounded-[12px] bg-fill-quaternary" aria-busy="true" />
      ) : query.isError ? (
        <div role="alert" className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-subhead text-label-red">{errorMessage(query.error)}</p>
          <Button size="sm" variant="secondary" onPress={() => void query.refetch()}>
            {t('common:actions.retry')}
          </Button>
        </div>
      ) : !hasKey || !key ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-[12px] bg-fill-quaternary px-4 py-3">
          <p className="text-subhead text-label">{t('age.noKey')}</p>
          <Button size="sm" isPending={create.isPending} onPress={() => create.mutate(false)}>
            <KeyRound className="size-4" aria-hidden="true" />
            {t('age.generate')}
          </Button>
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          <Toggle isSelected={enabled} onChange={(next) => toggle.mutate(next)} isDisabled={toggle.isPending}>
            {t('age.toggle')}
          </Toggle>

          <dl className="flex flex-col gap-3">
            <div className="flex flex-col gap-1.5">
              <dt className="text-subhead font-medium text-label">{t('age.publicKey')}</dt>
              <dd className="flex items-center gap-2">
                <code className="min-w-0 flex-1 break-all rounded-input bg-fill-tertiary px-3 py-2 font-mono text-footnote text-label">
                  {key.public_key}
                </code>
                <CopyButton value={key.public_key ?? ''} />
              </dd>
            </div>
            <div className="flex flex-col gap-1.5">
              <dt className="flex items-center gap-0.5 text-subhead font-medium text-label">
                {t('age.privateKey')}
                {privateKey ? <HelpTip subject={t('age.privateKey')}>{t('age.privateHint')}</HelpTip> : null}
              </dt>
              {privateKey ? (
                <dd className="flex flex-wrap items-center gap-2">
                  <code
                    data-testid="age-private-key"
                    aria-label={revealed ? undefined : t('age.privateHidden')}
                    className="min-w-0 flex-1 basis-full break-all rounded-input bg-fill-tertiary px-3 py-2 font-mono text-footnote text-label"
                  >
                    {revealed ? privateKey : maskSecret(privateKey)}
                  </code>
                  <Button
                    size="sm"
                    variant="secondary"
                    aria-pressed={revealed}
                    onPress={() => setRevealed((v) => !v)}
                  >
                    {revealed ? (
                      <EyeOff className="size-4" aria-hidden="true" />
                    ) : (
                      <Eye className="size-4" aria-hidden="true" />
                    )}
                    {revealed ? t('age.hide') : t('age.reveal')}
                  </Button>
                  <CopyButton value={privateKey}>{t('age.copyPrivate')}</CopyButton>
                </dd>
              ) : (
                <dd className="text-footnote text-label-orange">{t('age.privateUnavailable')}</dd>
              )}
            </div>
          </dl>

          {key.created_at ? (
            <p className="text-footnote text-label-secondary">
              {t('age.createdAt')} <DateTime value={key.created_at} format="datetime" />
            </p>
          ) : null}

          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="secondary"
              isPending={create.isPending}
              onPress={() => void regenerate()}
            >
              <RefreshCw className="size-4" aria-hidden="true" />
              {t('age.regenerate')}
            </Button>
            <Button
              size="sm"
              variant="danger-soft"
              isPending={remove.isPending}
              onPress={() => void removeKey()}
            >
              <Trash2 className="size-4" aria-hidden="true" />
              {t('age.remove')}
            </Button>
          </div>
        </div>
      )}

      {unsupported ? null : (
        <>
          <details className="group rounded-[12px] bg-fill-quaternary px-4 py-3">
            <summary className="flex cursor-pointer list-none items-center justify-between gap-2 text-subhead font-medium text-label [&::-webkit-details-marker]:hidden">
              {t('age.howto')}
              <ChevronDown className="size-4 transition-transform group-open:rotate-180" aria-hidden="true" />
            </summary>
            <div className="mt-3 flex flex-col gap-3 text-footnote text-label">
              <p>{t('age.howtoCommon')}</p>
              <ul className="flex list-disc flex-col gap-1.5 pl-5">
                <li>{t('age.howtoClients.party')}</li>
                <li>{t('age.howtoClients.verge')}</li>
                <li>{t('age.howtoClients.mihomo')}</li>
                <li className="text-label-secondary">{t('age.howtoClients.others')}</li>
              </ul>
              <div className="flex flex-col gap-1.5">
                <div className="flex items-center justify-between gap-2">
                  <span className="font-medium">{t('age.snippet')}</span>
                  <CopyButton value={ageSnippet(subscribeUrl)} />
                </div>
                <pre
                  tabIndex={0}
                  aria-label={t('age.snippet')}
                  className="overflow-x-auto rounded-[10px] bg-elevated p-3 font-mono text-caption leading-relaxed text-label"
                >
                  {ageSnippet(subscribeUrl)}
                </pre>
                <p className="text-label-secondary">{t('age.snippetKey')}</p>
              </div>
              <p className="text-label-secondary">{t('age.howtoOld')}</p>
            </div>
          </details>
        </>
      )}
    </section>
  )
}

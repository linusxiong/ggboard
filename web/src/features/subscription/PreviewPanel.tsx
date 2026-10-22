/**
 * Generated-config preview (`GET /user/subscription/preview?client=…`). Lazy-loaded by the subscription page
 * and only fetched once opened.
 */
import { Button } from '@heroui/react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { useT } from '@/i18n'
import { qk } from '@/lib/api/queries'
import { previewSubscription } from '@/lib/api/user'
import { errorMessage } from '@/lib/errors'
import { OptionSelect } from './controls'

const PREVIEW_CLIENTS = [
  { value: 'clash-meta', label: 'Clash Meta / mihomo' },
  { value: 'stash', label: 'Stash' },
  { value: 'singbox', label: 'sing-box' },
  { value: 'surge', label: 'Surge' },
  { value: 'quantumultx', label: 'Quantumult X' },
  { value: 'shadowrocket', label: 'Shadowrocket' },
  { value: 'loon', label: 'Loon' },
  { value: 'clash', label: 'Clash (classic)' },
  { value: 'v2ray', label: 'v2rayN / base64' },
] as const

const MAX_CHARS = 200_000

export default function PreviewPanel({ profile }: { profile?: string | undefined }) {
  const t = useT(['subscription', 'common'])
  const [client, setClient] = useState<string>('clash-meta')
  const query = useQuery({
    queryKey: [...qk.user.subscription(), 'preview', client, profile ?? ''] as const,
    queryFn: ({ signal }) => previewSubscription({ client, ...(profile ? { profile } : {}) }, signal),
    placeholderData: keepPreviousData,
    staleTime: 60_000,
    retry: false,
  })
  const text = query.data ?? ''
  const shown = text.length > MAX_CHARS ? `${text.slice(0, MAX_CHARS)}\n…` : text

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-end gap-2">
        <OptionSelect
          label={t('preview.client')}
          value={client}
          options={PREVIEW_CLIENTS}
          onChange={setClient}
          className="w-60"
        />
        <Button
          variant="secondary"
          size="md"
          isPending={query.isFetching}
          onPress={() => void query.refetch()}
        >
          <RefreshCw className="size-4" aria-hidden="true" />
          {t('common:actions.refresh')}
        </Button>
        {text ? (
          <CopyButton value={text} size="md">
            {t('common:actions.copy')}
          </CopyButton>
        ) : null}
      </div>
      {query.isError ? (
        <p role="alert" className="rounded-[12px] bg-fill-quaternary px-4 py-3 text-subhead text-label-red">
          {errorMessage(query.error)}
        </p>
      ) : (
        <pre
          aria-label={t('preview.output', { client })}
          aria-busy={query.isPending || undefined}
          tabIndex={0}
          className="max-h-[28rem] min-h-40 overflow-auto rounded-[12px] bg-fill-quaternary p-4 font-mono text-caption leading-relaxed text-label"
        >
          {query.isPending ? t('common:states.loading') : shown || t('preview.empty')}
        </pre>
      )}
    </div>
  )
}

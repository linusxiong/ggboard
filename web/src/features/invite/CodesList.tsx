/** Unused invite codes (`/user/invite` only lists unused ones) with visit counts and copy. */
import { Button } from '@heroui/react'
import { Plus } from 'lucide-react'
import { CopyButton } from '@/components/CopyButton'
import { EmptyState } from '@/components/EmptyState'
import { TitleWithHelp } from '@/components/HelpTip'
import { DateTime } from '@/components/Formatters'
import { useT } from '@/i18n'
import type { InviteCode } from '@/lib/api/types'
import { useSite } from '@/features/shop/site'
import { buildInviteLink } from './inviteLink'

interface CodesListProps {
  codes: readonly InviteCode[]
  limit: number
  onGenerate: () => void
  isGenerating: boolean
}

export function CodesList({ codes, limit, onGenerate, isGenerating }: CodesListProps) {
  const t = useT(['invite', 'common'])
  const { appUrl } = useSite()
  const atLimit = limit > 0 && codes.length >= limit
  return (
    <section className="flex flex-col gap-3" aria-labelledby="invite-codes-title">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <TitleWithHelp help={t('codes.subtitle')} subject={t('codes.title')}>
            <h2 id="invite-codes-title" className="text-title3 text-label">
              {t('codes.title')}
            </h2>
          </TitleWithHelp>
          {limit > 0 ? (
            <p className="text-footnote text-label-secondary">
              {t('codes.usage', { count: codes.length, limit })}
            </p>
          ) : null}
        </div>
        <Button
          size="sm"
          variant="secondary"
          onPress={onGenerate}
          isPending={isGenerating}
          isDisabled={atLimit}
        >
          <Plus className="size-4" aria-hidden="true" />
          {atLimit ? t('codes.limitReached') : t('codes.generate')}
        </Button>
      </div>
      {codes.length === 0 ? (
        <div className="surface-card">
          <EmptyState compact title={t('codes.empty')} />
        </div>
      ) : (
        <ul className="inset-group shadow-card">
          {codes.map((code) => (
            <li key={code.code} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
              <div className="min-w-0">
                <p className="truncate font-mono text-subhead text-label">{code.code}</p>
                <p className="text-footnote text-label-secondary">
                  {t('codes.visits', { count: code.pv })} · <DateTime value={code.created_at} format="date" />
                </p>
              </div>
              <div className="flex items-center gap-1">
                <CopyButton value={code.code} variant="ghost">
                  {t('codes.copyCode')}
                </CopyButton>
                <CopyButton value={buildInviteLink(appUrl, code.code)} variant="ghost">
                  {t('codes.copyLink')}
                </CopyButton>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

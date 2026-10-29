import { Link } from '@tanstack/react-router'
import { CopyButton } from '@/components/CopyButton'
import { HelpTip, TitleWithHelp } from '@/components/HelpTip'
import { useT } from '@/i18n'
import type { NodeType } from '@/lib/api/types'
import { useSite } from '../shared/useSite'
import { integrationSnippets } from './integration'
import type { TokenOwner } from './nodeForm'

function Snippet({
  title,
  code,
  hint,
  unsupported,
}: {
  title: string
  code: string | null
  hint?: string
  unsupported: string
}) {
  return (
    <section className="flex flex-col gap-2 rounded-[14px] bg-fill-quaternary p-4">
      <div className="flex items-center justify-between gap-2">
        <TitleWithHelp help={code ? hint : undefined} subject={title}>
          <h3 className="text-headline text-label">{title}</h3>
        </TitleWithHelp>
        {code ? <CopyButton value={code} /> : null}
      </div>
      {code ? (
        <pre className="overflow-x-auto rounded-[10px] bg-elevated p-3 font-mono text-footnote text-label">
          <code>{code}</code>
        </pre>
      ) : (
        <p className="text-subhead text-label-secondary">{unsupported}</p>
      )}
    </section>
  )
}

/**
 * How to connect XrayR / V2bX / soga to this node (placeholders only): `<server_token>` from
 * Settings → server, or `<node_token>` when the node (a child: its parent) has its own token.
 */
export function NodeIntegration({
  type,
  nodeId,
  version,
  isChild,
  tokenOwner = null,
}: {
  type: NodeType
  nodeId: number | null
  version?: unknown
  isChild: boolean
  tokenOwner?: TokenOwner
}) {
  const t = useT('admin-nodes')
  const { appUrl } = useSite()
  const snippets = integrationSnippets({ type, nodeId, version, appUrl, nodeToken: tokenOwner !== null })
  const unsupported = t('nodes.integration.unsupported')
  return (
    <div className="flex flex-col gap-4">
      <p className="text-subhead text-label-secondary">
        {tokenOwner?.kind === 'node' ? (
          t('nodes.integration.nodeTokenIntro')
        ) : tokenOwner?.kind === 'parent' ? (
          t('nodes.integration.parentTokenIntro', { name: tokenOwner.name })
        ) : (
          <>
            {t('nodes.integration.intro')}{' '}
            <Link to="/admin/settings" className="text-label-blue underline underline-offset-2">
              {t('nodes.integration.settingsLink')}
            </Link>
          </>
        )}
        {isChild ? (
          <HelpTip subject={t('nodes.integration.childSubject')} className="ms-1 align-middle">
            {t('nodes.integration.child')}
          </HelpTip>
        ) : null}
      </p>
      {nodeId === null ? (
        <p className="text-footnote text-label-orange">{t('nodes.integration.unsaved')}</p>
      ) : null}
      <Snippet
        title="XrayR"
        code={snippets.xrayr}
        hint={t('nodes.integration.xrayrHint')}
        unsupported={unsupported}
      />
      <Snippet
        title="V2bX"
        code={snippets.v2bx}
        hint={t('nodes.integration.v2bxHint')}
        unsupported={unsupported}
      />
      <Snippet
        title="soga"
        code={snippets.soga}
        hint={t('nodes.integration.sogaHint')}
        unsupported={unsupported}
      />
    </div>
  )
}

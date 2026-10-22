import { Link } from '@tanstack/react-router'
import { CopyButton } from '@/components/CopyButton'
import { useT } from '@/i18n'
import type { NodeType } from '@/lib/api/types'
import { useSite } from '../shared/useSite'
import { integrationSnippets } from './integration'

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
        <h3 className="text-headline text-label">{title}</h3>
        {code ? <CopyButton value={code} /> : null}
      </div>
      {code ? (
        <pre className="overflow-x-auto rounded-[10px] bg-elevated p-3 font-mono text-footnote text-label">
          <code>{code}</code>
        </pre>
      ) : (
        <p className="text-subhead text-label-secondary">{unsupported}</p>
      )}
      {hint && code ? <p className="text-footnote text-label-secondary">{hint}</p> : null}
    </section>
  )
}

/** How to connect XrayR / V2bX / soga to this node (placeholders only; the token is in Settings → server). */
export function NodeIntegration({
  type,
  nodeId,
  version,
  isChild,
}: {
  type: NodeType
  nodeId: number | null
  version?: unknown
  isChild: boolean
}) {
  const t = useT('admin-nodes')
  const { appUrl } = useSite()
  const snippets = integrationSnippets({ type, nodeId, version, appUrl })
  const unsupported = t('nodes.integration.unsupported')
  return (
    <div className="flex flex-col gap-4">
      <p className="text-subhead text-label-secondary">
        {t('nodes.integration.intro')}{' '}
        <Link to="/admin/settings" className="text-label-blue underline underline-offset-2">
          {t('nodes.integration.settingsLink')}
        </Link>
      </p>
      {nodeId === null ? (
        <p className="text-footnote text-label-orange">{t('nodes.integration.unsaved')}</p>
      ) : null}
      {isChild ? <p className="text-footnote text-label-secondary">{t('nodes.integration.child')}</p> : null}
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

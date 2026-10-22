/** One knowledge article: title, date, sanitised Markdown and a "get a plan" callout for gated blocks. */
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Lock } from 'lucide-react'
import { lazy, Suspense } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { DateTime } from '@/components/Formatters'
import { useT } from '@/i18n'
import { knowledgeQuery } from '@/lib/api/queries'
import type { ID } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'
import { errorMessage } from '@/lib/errors'

const KnowledgeMarkdown = lazy(() => import('./KnowledgeMarkdown'))

function BodySkeleton() {
  return (
    <div className="flex flex-col gap-2" aria-busy="true">
      <div className="h-4 w-3/4 shimmer rounded bg-fill-tertiary" />
      <div className="h-4 w-full shimmer rounded bg-fill-tertiary" />
      <div className="h-4 w-5/6 shimmer rounded bg-fill-tertiary" />
    </div>
  )
}

export function KnowledgeArticleView({ id }: { id: ID }) {
  const t = useT('content')
  const article = useQuery(knowledgeQuery(id))

  if (article.isPending) {
    return (
      <article className="flex flex-col gap-4" aria-busy="true">
        <div className="h-7 w-1/2 shimmer rounded bg-fill-tertiary" />
        <BodySkeleton />
      </article>
    )
  }
  if (article.isError) {
    return <EmptyState compact title={t('knowledge.loadError')} description={errorMessage(article.error)} />
  }

  const { data } = article
  const gated = data.body.includes('knowledge-no-access')
  return (
    <article className="flex flex-col gap-4" aria-labelledby={`article-${data.id}`}>
      <header className="flex flex-col gap-1">
        <p className="text-footnote text-label-secondary">{data.category}</p>
        <h2 id={`article-${data.id}`} className="text-title2 text-label">
          {data.title}
        </h2>
        <p className="text-footnote text-label-secondary">
          {t('knowledge.updated')} <DateTime value={data.updated_at} format="date" />
        </p>
      </header>
      {gated ? (
        <div className="flex flex-col gap-3 rounded-[14px] bg-accent-soft px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="flex items-center gap-2 text-subhead text-label">
            <Lock className="size-4 shrink-0 text-system-blue" aria-hidden="true" />
            {t('knowledge.gated')}
          </p>
          <Link to="/shop" className={buttonClass({ size: 'sm' })}>
            {t('knowledge.toShop')}
          </Link>
        </div>
      ) : null}
      <Suspense fallback={<BodySkeleton />}>
        <KnowledgeMarkdown source={data.body} />
      </Suspense>
    </article>
  )
}

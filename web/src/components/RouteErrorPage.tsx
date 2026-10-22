import { type ErrorComponentProps, useRouter } from '@tanstack/react-router'
import { useQueryErrorResetBoundary } from '@tanstack/react-query'
import { TriangleAlert } from 'lucide-react'
import { useEffect } from 'react'
import { useT } from '@/i18n'
import { errorMessage } from '@/lib/errors'
import { buttonClass } from '@/lib/button'
import { EmptyState } from './EmptyState'

/** Router `errorComponent`: friendly message + retry that also resets React Query errors. */
export default function RouteErrorPage({ error, reset }: ErrorComponentProps) {
  const t = useT('common')
  const router = useRouter()
  const { reset: resetQueries } = useQueryErrorResetBoundary()

  useEffect(() => {
    resetQueries()
  }, [resetQueries])

  return (
    <div className="flex min-h-[50vh] items-center justify-center px-4" role="alert">
      <EmptyState
        icon={<TriangleAlert className="size-7 text-label-orange" strokeWidth={1.75} />}
        title={t('states.error')}
        description={errorMessage(error) || t('states.errorHint')}
        action={
          <button
            type="button"
            className={buttonClass({ variant: 'secondary' })}
            onClick={() => {
              reset()
              void router.invalidate()
            }}
          >
            {t('actions.retry')}
          </button>
        }
      />
    </div>
  )
}

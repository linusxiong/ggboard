import { createFileRoute } from '@tanstack/react-router'
import { ComingSoon } from '@/components/ComingSoon'
import { useT } from '@/i18n'

/** Placeholder — replace the component body with the real page (see web/README.md). */
export const Route = createFileRoute('/_user/subscription')({
  component: SubscriptionPage,
})

function SubscriptionPage() {
  const t = useT('nav')
  return <ComingSoon title={t('user.subscription')} />
}

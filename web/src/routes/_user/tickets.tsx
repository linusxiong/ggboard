import { createFileRoute } from '@tanstack/react-router'
import { ComingSoon } from '@/components/ComingSoon'
import { useT } from '@/i18n'

/** Placeholder — replace the component body with the real page (see web/README.md). */
export const Route = createFileRoute('/_user/tickets')({
  component: TicketsPage,
})

function TicketsPage() {
  const t = useT('nav')
  return <ComingSoon title={t('user.tickets')} />
}

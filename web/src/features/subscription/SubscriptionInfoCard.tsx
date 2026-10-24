/** Profile → subscription: current link and UUID, with the reset action. */
import { Link } from '@tanstack/react-router'
import { CopyButton } from '@/components/CopyButton'
import { TitleWithHelp } from '@/components/HelpTip'
import { useT } from '@/i18n'
import { buttonClass } from '@/lib/button'
import { ResetSubscription } from './ResetSubscription'

function ReadOnlyRow({ id, label, value }: { id: string; label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-subhead font-medium text-label">
        {label}
      </label>
      <div className="flex items-stretch gap-2">
        <input
          id={id}
          readOnly
          value={value}
          onFocus={(e) => e.currentTarget.select()}
          className="min-w-0 flex-1 truncate rounded-input bg-fill-tertiary px-3 py-2 font-mono text-footnote text-label outline-none focus-visible:ring-2 focus-visible:ring-focus"
        />
        <CopyButton value={value} size="md" />
      </div>
    </div>
  )
}

export function SubscriptionInfoCard({ url, uuid }: { url: string | undefined; uuid: string }) {
  const t = useT(['subscription', 'common'])
  return (
    <section className="surface-card flex flex-col gap-5 p-6" aria-labelledby="sub-info-title">
      <TitleWithHelp help={t('info.hint')} subject={t('info.title')}>
        <h2 id="sub-info-title" className="text-headline text-label">
          {t('info.title')}
        </h2>
      </TitleWithHelp>
      {url ? <ReadOnlyRow id="profile-subscribe-url" label={t('link.url')} value={url} /> : null}
      <ReadOnlyRow id="profile-uuid" label={t('info.uuid')} value={uuid} />
      <div className="flex flex-wrap items-center gap-2">
        <ResetSubscription />
        <Link to="/subscription" className={buttonClass({ variant: 'secondary' })}>
          {t('info.manage')}
        </Link>
      </div>
    </section>
  )
}

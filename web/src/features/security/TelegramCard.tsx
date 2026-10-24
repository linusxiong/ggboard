/**
 * Profile → Telegram (`/user/telegram`): bind with a one-time code (`t.me/<bot>?start=<code>` deep link + QR),
 * polling `GET /user/telegram` every 3 s while the dialog is open; unbind; discussion-group link.
 * Hidden when the server has no Telegram endpoint (404); `enabled: false` (no bot token) shows a muted state
 * instead of the bind button (a bound account can still unbind).
 */
import { Button, Chip, Modal, Spinner } from '@heroui/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ExternalLink, Send } from 'lucide-react'
import { lazy, Suspense, useEffect, useRef, useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { confirm } from '@/components/feedback/confirm'
import { ModalClose } from '@/components/feedback/ModalClose'
import { TitleWithHelp } from '@/components/HelpTip'
import { isMissing, telegramKey, telegramQuery } from '@/features/subscription/queries'
import { useT } from '@/i18n'
import { isApiError } from '@/lib/api/client'
import type { TelegramBindCode, TelegramStatus } from '@/lib/api/types'
import { telegramBindCode, unbindTelegram } from '@/lib/api/user'
import { buttonClass } from '@/lib/button'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'

const QRCode = lazy(() => import('@/components/QRCode').then((m) => ({ default: m.QRCode })))

/** While the bind dialog is open (contract: every 3 s). */
export const TELEGRAM_POLL_MS = 3000

const at = (name: string | null | undefined) => (name ? (name.startsWith('@') ? name : `@${name}`) : '')

export function TelegramCard() {
  const t = useT(['security', 'common'])
  const queryClient = useQueryClient()
  const [binding, setBinding] = useState(false)
  const status = useQuery({
    ...telegramQuery(),
    refetchInterval: binding ? TELEGRAM_POLL_MS : false,
    refetchIntervalInBackground: binding,
  })
  const data = status.data

  const unbind = useMutation({
    mutationFn: unbindTelegram,
    onSuccess: async () => {
      queryClient.setQueryData<TelegramStatus>(telegramKey(), (old) =>
        old ? { ...old, bound: false, username: null } : old,
      )
      toast.success(t('telegram.unbindDone'))
      await queryClient.invalidateQueries({ queryKey: telegramKey() })
    },
    onError: (error) => toast.danger(errorMessage(error)),
  })

  if (status.isError && isMissing(status.error)) return null

  const onUnbind = async () => {
    const ok = await confirm({
      title: t('telegram.unbindTitle'),
      description: t('telegram.unbindHint'),
      confirmLabel: t('telegram.unbindConfirm'),
      tone: 'danger',
    })
    if (ok) unbind.mutate()
  }

  return (
    <section className="surface-card flex flex-col gap-4 p-6" aria-labelledby="telegram-title">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <span
            className="flex size-9 shrink-0 items-center justify-center rounded-[10px] bg-fill-tertiary text-label-secondary"
            aria-hidden="true"
          >
            <Send className="size-5" />
          </span>
          <div className="min-w-0">
            <TitleWithHelp help={t('telegram.hint')} subject={t('telegram.title')}>
              <h2 id="telegram-title" className="flex flex-wrap items-center gap-2 text-headline text-label">
                {t('telegram.title')}
                {data ? (
                  <Chip size="sm" variant="soft" color={data.bound ? 'success' : 'default'}>
                    {data.bound ? t('telegram.bound') : t('telegram.unbound')}
                  </Chip>
                ) : null}
              </h2>
            </TitleWithHelp>
            {data?.bound ? (
              <p className="mt-1 text-footnote text-label-secondary">
                {data.username
                  ? t('telegram.boundAs', { name: at(data.username) })
                  : t('telegram.boundNoName')}
              </p>
            ) : null}
          </div>
        </div>
        {data ? (
          data.bound ? (
            <Button
              size="sm"
              variant="danger-soft"
              isPending={unbind.isPending}
              onPress={() => void onUnbind()}
            >
              {t('telegram.unbind')}
            </Button>
          ) : data.enabled === false ? null : (
            <Button size="sm" onPress={() => setBinding(true)}>
              <Send className="size-4" aria-hidden="true" />
              {t('telegram.bind')}
            </Button>
          )
        ) : null}
      </div>

      {data && data.enabled === false && !data.bound ? (
        <p className="rounded-[12px] bg-fill-quaternary px-4 py-3 text-subhead text-label-secondary">
          {t('telegram.disabled')}
        </p>
      ) : null}

      {status.isPending ? (
        <div className="h-8 shimmer rounded-[10px] bg-fill-quaternary" aria-busy="true" />
      ) : status.isError ? (
        <div role="alert" className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-footnote text-label-red">{errorMessage(status.error)}</p>
          <Button size="sm" variant="secondary" onPress={() => void status.refetch()}>
            {t('common:actions.retry')}
          </Button>
        </div>
      ) : null}

      {data?.discuss_link ? (
        <a
          href={data.discuss_link}
          target="_blank"
          rel="noopener noreferrer"
          className={buttonClass({ variant: 'secondary', size: 'sm', className: 'self-start' })}
        >
          {t('telegram.discuss')}
          <ExternalLink className="size-3.5" aria-hidden="true" />
        </a>
      ) : null}

      {binding ? (
        <BindDialog
          bound={data?.bound === true}
          onClose={() => setBinding(false)}
          onBound={() => {
            setBinding(false)
            toast.success(t('telegram.success'))
          }}
        />
      ) : null}
    </section>
  )
}

function BindDialog({
  bound,
  onClose,
  onBound,
}: {
  /** Live status from the (polled) card query. */
  bound: boolean
  onClose: () => void
  onBound: () => void
}) {
  const t = useT(['security', 'common'])
  const [code, setCode] = useState<TelegramBindCode | null>(null)
  const [error, setError] = useState<string>()
  const started = useRef(false)

  const fetchCode = useMutation({
    mutationFn: telegramBindCode,
    onSuccess: (result) => {
      setCode(result)
      setError(undefined)
    },
    onError: (e) =>
      setError(
        isApiError(e) && e.code === 'telegram_unavailable'
          ? t('telegram.unavailable')
          : isApiError(e) && e.status === 403
            ? t('telegram.disabled')
            : `${t('telegram.codeError')} ${errorMessage(e)}`,
      ),
  })
  const { mutate } = fetchCode

  useEffect(() => {
    if (started.current) return
    started.current = true
    mutate()
  }, [mutate])

  const finished = useRef(false)
  useEffect(() => {
    if (bound && !finished.current) {
      finished.current = true
      onBound()
    }
  }, [bound, onBound])

  const bot = code ? at(code.bot_username) : ''
  const command = code ? `/start ${code.code}` : ''

  return (
    <Modal.Backdrop isOpen onOpenChange={(open) => (open ? undefined : onClose())}>
      <Modal.Container size="sm" scroll="inside">
        <Modal.Dialog>
          <ModalClose />
          <Modal.Header>
            <TitleWithHelp help={t('telegram.expires')} subject={t('telegram.bindTitle')}>
              <Modal.Heading>{t('telegram.bindTitle')}</Modal.Heading>
            </TitleWithHelp>
          </Modal.Header>
          <Modal.Body className="flex flex-col items-center gap-4 text-center">
            {error ? (
              <p role="alert" className="text-subhead text-label-red">
                {error}
              </p>
            ) : !code ? (
              <div className="flex h-64 items-center justify-center" aria-busy="true">
                <Spinner size="md" />
              </div>
            ) : (
              <>
                <p className="text-subhead text-label">{t('telegram.step1', { bot })}</p>
                <Suspense
                  fallback={<div className="size-[176px] shimmer rounded-[12px] bg-fill-quaternary" />}
                >
                  <QRCode value={code.deep_link} size={176} label={t('telegram.qrLabel')} />
                </Suspense>
                <a
                  href={code.deep_link}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={buttonClass({ variant: 'primary', fullWidth: true })}
                >
                  <Send className="size-4" aria-hidden="true" />
                  {t('telegram.open')}
                </a>
                <p className="text-footnote text-label-secondary">{t('telegram.step2')}</p>
                <div className="flex w-full flex-col gap-1.5 text-start">
                  <p className="text-footnote text-label-secondary">{t('telegram.manual', { bot })}</p>
                  <div className="flex items-center gap-2">
                    <code className="min-w-0 flex-1 truncate rounded-input bg-fill-tertiary px-3 py-2 font-mono text-footnote text-label">
                      {command}
                    </code>
                    <CopyButton value={command} />
                  </div>
                </div>
                <p
                  className="flex items-center gap-2 text-footnote text-label-secondary"
                  role="status"
                  aria-live="polite"
                >
                  <Spinner size="sm" />
                  {t('telegram.waiting')}
                </p>
              </>
            )}
          </Modal.Body>
          <Modal.Footer>
            <Button variant="secondary" isPending={fetchCode.isPending} onPress={() => fetchCode.mutate()}>
              {t('telegram.newCode')}
            </Button>
            <Button slot="close" variant="tertiary">
              {t('common:actions.close')}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}

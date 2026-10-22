import { Button } from '@heroui/react'
import { useMutation } from '@tanstack/react-query'
import { Send, Webhook } from 'lucide-react'
import { useState } from 'react'
import { useT } from '@/i18n'
import { api, isApiError } from '@/lib/api/client'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'

type Action = 'test' | 'webhook'

/** `POST /admin/settings/telegram/test` */
interface TestReport {
  sent?: number
  failed?: number
  results?: Array<{ chat_id: string | number; ok: boolean; error?: string }>
}
/** `POST /admin/settings/telegram/webhook` */
interface WebhookResult {
  ok?: boolean
  description?: string
  error_code?: number
  url?: string
}

interface Outcome {
  tone: 'ok' | 'error'
  text: string
  /** Per-chat failures of a test send. */
  details?: string[]
}

type T = (key: string, params?: Record<string, string | number>) => string

/** Reads the documented shapes defensively (any missing field just falls back). */
function describe(action: Action, data: unknown, t: T): Outcome {
  if (action === 'test') {
    const r = (data ?? {}) as TestReport
    const failed = r.failed ?? r.results?.filter((x) => !x.ok).length ?? 0
    const sent = r.sent ?? r.results?.filter((x) => x.ok).length ?? 0
    return {
      tone: failed > 0 || sent === 0 ? 'error' : 'ok',
      text: t('telegram.sentSummary', { sent, failed }),
      details: (r.results ?? [])
        .filter((x) => !x.ok)
        .map((x) => t('telegram.chatFailed', { chat: String(x.chat_id), error: x.error ?? '—' })),
    }
  }
  const r = (data ?? {}) as WebhookResult
  return r.ok
    ? { tone: 'ok', text: t('telegram.webhookOk', { url: r.url ?? '' }) }
    : {
        tone: 'error',
        text: t('telegram.webhookFailed', {
          description: [r.error_code, r.description].filter(Boolean).join(' ') || '—',
        }),
      }
}

/**
 * Shown in the Telegram settings group (only rendered when the backend sends a `telegram` group):
 * `POST /admin/settings/telegram/test` and `POST /admin/settings/telegram/webhook`, result inline.
 * 400 field errors (e.g. missing `app_url`) go to the fields via `onFieldErrors`.
 */
export function TelegramActions({
  dirty,
  onFieldErrors,
}: {
  dirty: boolean
  onFieldErrors?: (errors: Record<string, string>) => void
}) {
  const t = useT('admin-settings')
  const [result, setResult] = useState<Outcome>()
  const run = useMutation({
    mutationFn: async (action: Action) => ({
      action,
      data: await api.post<unknown>(`/admin/settings/telegram/${action}`),
    }),
    onSuccess: ({ action, data }) => {
      const outcome = describe(action, data, t)
      setResult(outcome)
      if (outcome.tone === 'ok') toast.success(outcome.text)
      else toast.error(outcome.text)
    },
    onError: (e) => {
      if (isApiError(e) && Object.keys(e.errors).length) {
        const fields: Record<string, string> = {}
        for (const [k, msgs] of Object.entries(e.errors)) if (msgs[0]) fields[k] = msgs[0]
        onFieldErrors?.(fields)
      }
      const text = errorMessage(e)
      setResult({ tone: 'error', text })
      toast.error(text)
    },
  })
  const pending = run.isPending ? run.variables : undefined

  return (
    <section aria-labelledby="telegram-actions" className="flex flex-col gap-2">
      <div className="px-1">
        <h3 id="telegram-actions" className="text-headline text-label">
          {t('telegram.title')}
        </h3>
        <p className="text-footnote text-label-secondary">{t('telegram.caption')}</p>
      </div>
      <div className="surface-card flex flex-col gap-3 px-5 py-4 sm:px-6">
        <div className="flex flex-wrap gap-2">
          <Button
            variant="secondary"
            isDisabled={dirty || run.isPending}
            isPending={pending === 'test'}
            onPress={() => run.mutate('test')}
          >
            <Send className="size-4" aria-hidden />
            {t('telegram.test')}
          </Button>
          <Button
            variant="secondary"
            isDisabled={dirty || run.isPending}
            isPending={pending === 'webhook'}
            onPress={() => run.mutate('webhook')}
          >
            <Webhook className="size-4" aria-hidden />
            {t('telegram.webhook')}
          </Button>
        </div>
        {dirty ? <p className="text-footnote text-label-secondary">{t('telegram.unsaved')}</p> : null}
        {result ? (
          <div role="status" className="flex flex-col gap-1 text-footnote">
            <p className={result.tone === 'ok' ? 'text-label-green' : 'text-label-red'}>{result.text}</p>
            {result.details?.length ? (
              <ul className="flex flex-col gap-0.5 font-mono text-label-secondary">
                {result.details.map((d) => (
                  <li key={d}>{d}</li>
                ))}
              </ul>
            ) : null}
          </div>
        ) : null}
      </div>
    </section>
  )
}

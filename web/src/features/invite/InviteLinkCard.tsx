/** Invite link builder: pick a code → `<app_url>/register?code=…` with copy + QR. */
import { Button, Label, ListBox, Select } from '@heroui/react'
import { Link2, QrCode } from 'lucide-react'
import { useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { QRCode } from '@/components/QRCode'
import { useT } from '@/i18n'
import type { InviteCode } from '@/lib/api/types'
import { useSite } from '@/features/shop/site'
import { buildInviteLink } from './inviteLink'

interface InviteLinkCardProps {
  codes: readonly InviteCode[]
  canGenerate: boolean
  onGenerate: () => void
  isGenerating: boolean
}

export function InviteLinkCard({ codes, canGenerate, onGenerate, isGenerating }: InviteLinkCardProps) {
  const t = useT(['invite', 'common'])
  const { appUrl } = useSite()
  const [selected, setSelected] = useState<string | null>(null)
  const [showQr, setShowQr] = useState(false)
  const code = codes.find((c) => c.code === selected)?.code ?? codes[0]?.code
  const link = code ? buildInviteLink(appUrl, code) : null

  return (
    <section className="surface-card flex flex-col gap-4 p-5" aria-labelledby="invite-link-title">
      <div className="flex items-center gap-3">
        <span
          className="flex size-9 items-center justify-center rounded-[10px] bg-system-indigo text-white"
          aria-hidden="true"
        >
          <Link2 className="size-5" />
        </span>
        <div>
          <h2 id="invite-link-title" className="text-headline text-label">
            {t('link.title')}
          </h2>
          <p className="text-footnote text-label-secondary">{t('link.subtitle')}</p>
        </div>
      </div>

      {link && code ? (
        <>
          {codes.length > 1 ? (
            <Select
              value={code}
              onChange={(key) => setSelected(key === null ? null : String(key))}
              className="w-full"
            >
              <Label>{t('link.code')}</Label>
              <Select.Trigger>
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {codes.map((c) => (
                    <ListBox.Item key={c.code} id={c.code} textValue={c.code}>
                      <span className="font-mono">{c.code}</span>
                      <ListBox.ItemIndicator />
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
          ) : null}
          <div className="flex items-center gap-2 rounded-[12px] bg-fill-quaternary py-1.5 ps-3 pe-1.5">
            <output
              className="min-w-0 flex-1 truncate font-mono text-footnote text-label"
              aria-label={t('link.label')}
            >
              {link}
            </output>
            <CopyButton value={link} variant="primary">
              {t('link.copy')}
            </CopyButton>
          </div>
          <div className="flex flex-col items-center gap-3">
            <Button size="sm" variant="ghost" onPress={() => setShowQr((v) => !v)} aria-expanded={showQr}>
              <QrCode className="size-4" aria-hidden="true" />
              {showQr ? t('link.hideQr') : t('link.showQr')}
            </Button>
            {showQr ? <QRCode value={link} size={176} label={t('link.qrLabel')} /> : null}
          </div>
        </>
      ) : (
        <div className="flex flex-col items-start gap-3">
          <p className="text-subhead text-label-secondary">{t('link.noCode')}</p>
          <Button onPress={onGenerate} isPending={isGenerating} isDisabled={!canGenerate}>
            {t('codes.generate')}
          </Button>
        </div>
      )}
    </section>
  )
}

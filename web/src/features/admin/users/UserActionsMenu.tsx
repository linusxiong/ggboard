import { Button, Dropdown, Label, Modal } from '@heroui/react'
import { Ban, Ellipsis, KeyRound, LogIn, RotateCcw, ShieldCheck, Trash2 } from 'lucide-react'
import { CopyButton } from '@/components/CopyButton'
import { TitleWithHelp } from '@/components/HelpTip'
import { useT } from '@/i18n'
import { buttonClass } from '@/lib/button'
import type { AdminUser } from '@/lib/api/types'
import type { UserAction } from './useUserActions'
import { ModalClose } from '@/components/feedback/ModalClose'

/** "More" dropdown with the admin actions for one user. */
export function UserActionsMenu({
  user,
  onAction,
}: {
  user: AdminUser
  onAction: (action: UserAction) => void
}) {
  const t = useT(['admin-users', 'common'])
  return (
    <Dropdown>
      <Button size="sm" variant="ghost" isIconOnly aria-label={t('actions.moreFor', { name: user.email })}>
        <Ellipsis className="size-4" aria-hidden="true" />
      </Button>
      <Dropdown.Popover placement="bottom end">
        <Dropdown.Menu onAction={(key) => onAction(String(key) as UserAction)}>
          <Dropdown.Item id="loginAs" textValue={t('actions.loginAs')}>
            <LogIn className="size-4" aria-hidden="true" />
            <Label>{t('actions.loginAs')}</Label>
          </Dropdown.Item>
          <Dropdown.Item id="resetSecret" textValue={t('actions.resetSecret')}>
            <KeyRound className="size-4" aria-hidden="true" />
            <Label>{t('actions.resetSecret')}</Label>
          </Dropdown.Item>
          <Dropdown.Item id="resetTraffic" textValue={t('actions.resetTraffic')}>
            <RotateCcw className="size-4" aria-hidden="true" />
            <Label>{t('actions.resetTraffic')}</Label>
          </Dropdown.Item>
          {user.banned ? (
            <Dropdown.Item id="unban" textValue={t('actions.unban')}>
              <ShieldCheck className="size-4" aria-hidden="true" />
              <Label>{t('actions.unban')}</Label>
            </Dropdown.Item>
          ) : (
            <Dropdown.Item id="ban" textValue={t('actions.ban')} variant="danger">
              <Ban className="size-4" aria-hidden="true" />
              <Label>{t('actions.ban')}</Label>
            </Dropdown.Item>
          )}
          <Dropdown.Item id="delete" textValue={t('common:actions.delete')} variant="danger">
            <Trash2 className="size-4" aria-hidden="true" />
            <Label>{t('common:actions.delete')}</Label>
          </Dropdown.Item>
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  )
}

/**
 * One-time login link for "log in as". Opening it in this browser profile replaces the current
 * (admin) session, so the dialog recommends a private window and offers copy + open.
 */
export function LoginAsDialog({
  value,
  onClose,
}: {
  value: { email: string; url: string } | null
  onClose: () => void
}) {
  const t = useT(['admin-users', 'common'])
  return (
    <Modal.Backdrop isOpen={value !== null} onOpenChange={(open) => !open && onClose()}>
      <Modal.Container size="md">
        <Modal.Dialog>
          <ModalClose />
          <Modal.Header>
            <TitleWithHelp
              help={t('loginAs.help')}
              subject={t('loginAs.title', { name: value?.email ?? '' })}
            >
              <Modal.Heading>{t('loginAs.title', { name: value?.email ?? '' })}</Modal.Heading>
            </TitleWithHelp>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-3">
            <p className="text-subhead text-label-secondary">{t('loginAs.hint')}</p>
            <div className="flex items-center gap-2 rounded-[12px] bg-fill-quaternary p-3">
              <code className="min-w-0 flex-1 break-all font-mono text-footnote text-label">
                {value?.url}
              </code>
              {value ? <CopyButton value={value.url} /> : null}
            </div>
          </Modal.Body>
          <Modal.Footer>
            <Button slot="close" variant="tertiary">
              {t('common:actions.close')}
            </Button>
            {value ? (
              <a
                href={value.url}
                target="_blank"
                rel="noopener noreferrer"
                className={buttonClass({ size: 'md' })}
                onClick={onClose}
              >
                {t('loginAs.open')}
              </a>
            ) : null}
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}

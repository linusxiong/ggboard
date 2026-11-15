import { Button } from '@heroui/react'
import { Check, Copy } from 'lucide-react'
import { type ReactNode, useEffect, useRef, useState } from 'react'
import { useT } from '@/i18n'
import { announce } from '@/lib/announce'
import { copyText } from '@/lib/clipboard'
import { toast } from '@/lib/toast'

interface CopyButtonProps {
  value: string
  /** Visible label; omit for an icon-only button (then `aria-label` = `label`, "复制" by default). */
  children?: ReactNode
  /** Name of an icon-only button; say what it copies when several sit next to each other ("复制邀请码"). */
  label?: string
  size?: 'sm' | 'md' | 'lg'
  variant?: 'primary' | 'secondary' | 'tertiary' | 'outline' | 'ghost'
  className?: string
  /** Toast on success (default true). */
  notify?: boolean
}

export function CopyButton({
  value,
  children,
  label,
  size = 'sm',
  variant = 'secondary',
  className,
  notify = true,
}: CopyButtonProps) {
  const t = useT('common')
  const [copied, setCopied] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(timer.current), [])

  const onPress = async () => {
    const ok = await copyText(value)
    if (!ok) {
      toast.danger(t('copy.failed'))
      return
    }
    // Either way the result is announced once: by the toast, or directly when the caller wants no toast.
    if (notify) toast.success(t('copy.success'))
    else announce(t('actions.copied'))
    setCopied(true)
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setCopied(false), 1500)
  }

  const Icon = copied ? Check : Copy
  return (
    <Button
      size={size}
      variant={variant}
      isIconOnly={!children}
      // The name stays put: a label that flips to "已复制" is not reliably read out (see `announce`).
      aria-label={children ? undefined : (label ?? t('actions.copy'))}
      onPress={() => void onPress()}
      className={className}
    >
      <Icon className="size-4" aria-hidden="true" />
      {children}
    </Button>
  )
}

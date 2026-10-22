import { Button } from '@heroui/react'
import { Check, Copy } from 'lucide-react'
import { type ReactNode, useEffect, useRef, useState } from 'react'
import { useT } from '@/i18n'
import { copyText } from '@/lib/clipboard'
import { toast } from '@/lib/toast'

interface CopyButtonProps {
  value: string
  /** Visible label; omit for an icon-only button (then `aria-label` = "复制"). */
  children?: ReactNode
  size?: 'sm' | 'md' | 'lg'
  variant?: 'primary' | 'secondary' | 'tertiary' | 'outline' | 'ghost'
  className?: string
  /** Toast on success (default true). */
  notify?: boolean
}

export function CopyButton({
  value,
  children,
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
    if (notify) toast.success(t('copy.success'))
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
      aria-label={children ? undefined : copied ? t('actions.copied') : t('actions.copy')}
      onPress={() => void onPress()}
      className={className}
    >
      <Icon className="size-4" aria-hidden="true" />
      {children}
    </Button>
  )
}

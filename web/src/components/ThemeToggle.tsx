import { Monitor, Moon, Sun } from '@/components/icons/inline'
import { NativeButton } from '@/components/native/NativeButton'
import { useT } from '@/i18n'
import { useUiStore } from '@/stores/ui'

const ICONS = { system: Monitor, light: Sun, dark: Moon } as const

/**
 * Cycles system → light → dark. Native button + HeroUI button CSS: it sits on every page,
 * including the public ones, so it must not pull React Aria into the first load.
 */
export function ThemeToggle({ className }: { className?: string }) {
  const t = useT('core')
  const theme = useUiStore((s) => s.theme)
  const cycleTheme = useUiStore((s) => s.cycleTheme)
  const Icon = ICONS[theme]
  const label = t('theme.toggle', { mode: t(`theme.${theme}`) })
  return (
    <NativeButton iconOnly variant="ghost" size="sm" aria-label={label} title={label} onClick={cycleTheme} className={className}>
      <Icon className="size-[18px]" aria-hidden="true" />
    </NativeButton>
  )
}

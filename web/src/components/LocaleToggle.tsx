import { Languages } from '@/components/icons/inline'
import { NativeButton } from '@/components/native/NativeButton'
import { useT } from '@/i18n'
import { useUiStore } from '@/stores/ui'

/** Switches between the two bundled locales; the visible text is the target language. */
export function LocaleToggle({ className, compact = false }: { className?: string; compact?: boolean }) {
  const t = useT('core')
  const locale = useUiStore((s) => s.locale)
  const setLocale = useUiStore((s) => s.setLocale)
  const next = locale === 'zh-CN' ? 'en' : 'zh-CN'
  return (
    <NativeButton
      variant="ghost"
      size="sm"
      iconOnly={compact}
      aria-label={compact ? `${t('locale.toggle')}: ${t(`locale.${next}`)}` : undefined}
      title={t('locale.toggle')}
      onClick={() => setLocale(next)}
      className={className}
    >
      <Languages className="size-[18px]" aria-hidden="true" />
      {compact ? null : <span lang={next}>{t(`locale.${next}`)}</span>}
    </NativeButton>
  )
}

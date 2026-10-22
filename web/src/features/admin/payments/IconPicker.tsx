/**
 * Payment icon picker: a grid of the built-in marks (`PAYMENT_ICON_PRESETS`) plus "custom URL", which
 * reveals a URL field (an image URL, a `/path`, a data URI or a single emoji). The stored value is
 * `preset:<id>` or the custom string (see `components/icons/payments/presets.ts`).
 */
import { Description, FieldError, Label, Radio, RadioGroup } from '@heroui/react'
import { Link2 } from 'lucide-react'
import type { ReactNode } from 'react'
import { PAYMENT_ICON_PRESETS, PaymentIcon, parsePaymentIcon } from '@/components/icons/payments'
import { TextInput } from '@/features/admin/settings/inputs/Controlled'
import { useLocale, useT } from '@/i18n'
import { cx } from '@/lib/cn'

/** Picker choice for "custom URL" (never a registry id). */
export const CUSTOM_ICON = '__custom__'

export interface IconPickerProps {
  /** A preset id, `CUSTOM_ICON`, or null (no icon). */
  choice: string | null
  customValue: string
  onChoiceChange: (choice: string) => void
  onCustomChange: (value: string) => void
  error?: string | undefined
}

export function IconPicker({ choice, customValue, onChoiceChange, onCustomChange, error }: IconPickerProps) {
  const t = useT(['admin-payments', 'common'])
  const locale = useLocale()
  const custom = choice === CUSTOM_ICON
  const customPreview = parsePaymentIcon(customValue).kind === 'none' ? null : customValue

  return (
    <div className="flex flex-col gap-3">
      <RadioGroup
        name="icon_choice"
        value={choice}
        onChange={onChoiceChange}
        orientation="horizontal"
        isInvalid={Boolean(error) && !custom}
        validationBehavior="aria"
        className="flex-col flex-nowrap gap-2"
      >
        <Label>{t('icons.label')}</Label>
        <div className="grid grid-cols-4 gap-1.5 sm:grid-cols-6">
          {PAYMENT_ICON_PRESETS.map((preset) => (
            <IconTile
              key={preset.id}
              value={preset.id}
              label={preset.name[locale]}
              icon={<preset.Component width={32} height={32} className="shrink-0" />}
            />
          ))}
          <IconTile
            value={CUSTOM_ICON}
            label={t('icons.custom')}
            icon={
              customPreview ? (
                <PaymentIcon icon={customPreview} size={32} />
              ) : (
                <span
                  aria-hidden="true"
                  className="flex size-8 shrink-0 items-center justify-center rounded-[8px] bg-fill-secondary text-label-secondary"
                >
                  <Link2 className="size-[18px]" strokeWidth={1.75} />
                </span>
              )
            }
          />
        </div>
        {error && !custom ? null : <Description>{t('icons.hint')}</Description>}
        <FieldError>{error}</FieldError>
      </RadioGroup>
      {custom ? (
        <TextInput
          label={t('form.icon')}
          name="icon"
          type="url"
          value={customValue}
          onChange={onCustomChange}
          placeholder="https://"
          error={error}
          description={t('icons.customHint')}
          maxLength={255}
          monospace
        />
      ) : null}
    </div>
  )
}

function IconTile({ value, label, icon }: { value: string; label: string; icon: ReactNode }) {
  return (
    <Radio value={value} className="group min-w-0">
      <Radio.Content
        className={cx(
          'flex w-full min-w-0 flex-col items-center gap-1.5 rounded-[12px] px-1 pt-2.5 pb-2 ring-1 ring-transparent',
          'transition-[box-shadow,background-color] duration-200 ease-apple-out hover:bg-fill-quaternary',
          'group-data-selected:bg-selection group-data-selected:ring-2 group-data-selected:ring-tint',
          'data-focus-visible:ring-2 data-focus-visible:ring-tint',
        )}
      >
        {icon}
        <span
          title={label}
          className="line-clamp-2 w-full text-center text-caption2 leading-tight [overflow-wrap:anywhere] text-label-secondary group-data-selected:font-semibold group-data-selected:text-label-blue-soft"
        >
          {label}
        </span>
      </Radio.Content>
    </Radio>
  )
}

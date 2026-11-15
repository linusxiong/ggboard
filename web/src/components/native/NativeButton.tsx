/**
 * Native `<button>` with HeroUI's button CSS (no React Aria JS) — for the public pages and tiny
 * chrome controls (theme / locale toggles). Use HeroUI `<Button>` everywhere else.
 * Same four roles as `<Button>`: primary · secondary · tertiary · danger (+ ghost for chrome icons).
 */
import 'heroui-css:button'
import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { type ButtonClassOptions, buttonClass } from '@/lib/button'

export interface NativeButtonProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'className'>, Omit<ButtonClassOptions, 'className'> {
  className?: string
  /** Shows a spinner and blocks presses (like HeroUI `isPending`). */
  pending?: boolean
  /**
   * Unavailable for now but still focusable: presses are blocked like `pending`. Prefer it to `disabled` for a
   * button that turns unavailable while it has focus (a cooldown) — `disabled` would drop focus to `<body>`.
   */
  'aria-disabled'?: boolean
  children?: ReactNode
}

export function NativeButton({
  variant = 'primary',
  size = 'md',
  fullWidth,
  iconOnly,
  className,
  pending = false,
  disabled,
  type = 'button',
  children,
  'aria-disabled': ariaDisabled = false,
  ...rest
}: NativeButtonProps) {
  const blocked = pending || ariaDisabled
  return (
    <button
      {...rest}
      type={type}
      disabled={disabled}
      aria-disabled={blocked || undefined}
      aria-busy={pending || undefined}
      data-pending={pending || undefined}
      onClick={blocked ? (e) => e.preventDefault() : rest.onClick}
      className={buttonClass({ variant, size, fullWidth, iconOnly, className })}
    >
      {/* The spinner is drawn by the global button skin (`.button[data-pending]::before`). */}
      {children}
    </button>
  )
}

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
  ...rest
}: NativeButtonProps) {
  return (
    <button
      {...rest}
      type={type}
      disabled={disabled}
      aria-disabled={pending || undefined}
      aria-busy={pending || undefined}
      data-pending={pending || undefined}
      onClick={pending ? (e) => e.preventDefault() : rest.onClick}
      className={buttonClass({ variant, size, fullWidth, iconOnly, className })}
    >
      {/* The spinner is drawn by the global button skin (`.button[data-pending]::before`). */}
      {children}
    </button>
  )
}

/**
 * HeroUI button look for non-button elements (router links) — just the BEM classes from
 * `@heroui/styles`, zero JS. Use HeroUI `<Button>` for actions, this for navigation:
 *
 *   <Link to="/register" className={buttonClass({ size: 'lg' })}>Get started</Link>
 */
import { cx } from './cn'

export interface ButtonClassOptions {
  variant?: 'primary' | 'secondary' | 'tertiary' | 'outline' | 'ghost' | 'danger' | 'danger-soft'
  size?: 'sm' | 'md' | 'lg'
  fullWidth?: boolean
  iconOnly?: boolean
  className?: string
}

export function buttonClass({
  variant = 'primary',
  size = 'md',
  fullWidth,
  iconOnly,
  className,
}: ButtonClassOptions = {}) {
  return cx(
    'button',
    `button--${variant}`,
    `button--${size}`,
    fullWidth && 'button--full-width',
    iconOnly && 'button--icon-only',
    className,
  )
}

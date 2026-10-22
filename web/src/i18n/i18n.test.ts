import { readdirSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import { useUiStore } from '@/stores/ui'
import { interpolate, loadNamespaces, registerNamespace, t } from './index'

type Dict = { [key: string]: string | Dict }
const dir = path.resolve(import.meta.dirname)

function keys(dict: Dict, prefix = ''): string[] {
  return Object.entries(dict).flatMap(([k, v]) =>
    typeof v === 'string' ? [`${prefix}${k}`] : keys(v, `${prefix}${k}.`),
  )
}

describe('i18n', () => {
  it('interpolates {params}', () => {
    expect(interpolate('{seconds} 秒后重发', { seconds: 30 })).toBe('30 秒后重发')
    expect(interpolate('keep {missing}', {})).toBe('keep {missing}')
  })

  it('loads namespaces lazily and translates nested keys', async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    await loadNamespaces(['auth', 'core'])
    expect(t('auth', 'login.title')).toBe('登录')
    expect(t('auth', 'code.resend', { seconds: 9 })).toBe('9 秒后重发')
    expect(t('auth', 'core:theme.dark')).toBe('深色')
  })

  it('falls back to English, then to the key', () => {
    registerNamespace('test', 'en', {
      only: { en: 'English only' },
      n_one: '{count} item',
      n_other: '{count} items',
    })
    registerNamespace('test', 'zh-CN', {})
    expect(t('test', 'only.en', undefined, 'zh-CN')).toBe('English only')
    expect(t('test', 'does.not.exist', undefined, 'zh-CN')).toBe('does.not.exist')
    expect(t('test', 'n', { count: 1 }, 'en')).toBe('1 item')
    expect(t('test', 'n', { count: 3 }, 'en')).toBe('3 items')
  })

  it('has the same keys in every locale', () => {
    const locales = readdirSync(dir).filter((d) => !d.includes('.'))
    const [base, ...rest] = locales
    expect(base).toBeDefined()
    for (const file of readdirSync(path.join(dir, base!))) {
      const ref = keys(JSON.parse(readFileSync(path.join(dir, base!, file), 'utf8')) as Dict).sort()
      for (const locale of rest) {
        const other = keys(JSON.parse(readFileSync(path.join(dir, locale, file), 'utf8')) as Dict).sort()
        expect(other, `${locale}/${file}`).toEqual(ref)
      }
    }
  })
})

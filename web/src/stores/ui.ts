/**
 * UI preferences: theme (system | light | dark), locale, sidebar collapsed. Persisted (`gg-ui`).
 * Side effects (applying `data-theme`, `lang`) live in `src/lib/theme.ts` / `src/i18n`.
 */
import { create } from 'zustand'
import type { Locale } from '@/lib/api/types'
import { persistStore } from './storage'

export type ThemePreference = 'system' | 'light' | 'dark'
export const LOCALES: readonly Locale[] = ['zh-CN', 'en']
export const DEFAULT_LOCALE: Locale = 'zh-CN'
export const UI_STORAGE_KEY = 'gg-ui'

export interface UiState {
  theme: ThemePreference
  locale: Locale
  sidebarCollapsed: boolean
  setTheme: (theme: ThemePreference) => void
  /** system → light → dark → system */
  cycleTheme: () => void
  setLocale: (locale: Locale) => void
  toggleSidebar: () => void
  setSidebarCollapsed: (collapsed: boolean) => void
}

export function isLocale(value: unknown): value is Locale {
  return typeof value === 'string' && (LOCALES as readonly string[]).includes(value)
}

const NEXT_THEME: Record<ThemePreference, ThemePreference> = { system: 'light', light: 'dark', dark: 'system' }

export const useUiStore = create<UiState>()((set) => ({
  theme: 'system',
  locale: DEFAULT_LOCALE,
  sidebarCollapsed: false,
  setTheme: (theme) => set({ theme }),
  cycleTheme: () => set((s) => ({ theme: NEXT_THEME[s.theme] })),
  setLocale: (locale) => set({ locale: isLocale(locale) ? locale : DEFAULT_LOCALE }),
  toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
  setSidebarCollapsed: (sidebarCollapsed) => set({ sidebarCollapsed }),
}))

persistStore(useUiStore, {
  key: UI_STORAGE_KEY,
  pick: (s) => ({ theme: s.theme, locale: s.locale, sidebarCollapsed: s.sidebarCollapsed }),
  merge: (persisted) => {
    const p = (persisted ?? {}) as Partial<UiState>
    return {
      ...(p.theme && p.theme in NEXT_THEME ? { theme: p.theme } : {}),
      ...(isLocale(p.locale) ? { locale: p.locale } : {}),
      ...(typeof p.sidebarCollapsed === 'boolean' ? { sidebarCollapsed: p.sidebarCollapsed } : {}),
    }
  },
})

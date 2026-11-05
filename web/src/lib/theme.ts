import { useSyncExternalStore } from 'react'

/**
 * Light, dark, or whatever the system says — as the person chose it.
 *
 * The choice lives in localStorage and is applied as a `dark` class on
 * <html>, which is what globals.css's dark tokens key off. index.html applies
 * the same rule in an inline script before the bundle loads, so a dark-mode
 * user is never flashed a white page on refresh; keep the two in step.
 *
 * One module-level store rather than state per component, so the top bar's
 * switch and Configuración › Apariencia always agree.
 */
export type Theme = 'light' | 'dark'
export type ThemePreference = Theme | 'system'

export const THEME_STORAGE_KEY = 'tema'

function readPreference(): ThemePreference {
  try {
    const value = localStorage.getItem(THEME_STORAGE_KEY)
    return value === 'light' || value === 'dark' ? value : 'system'
  } catch {
    return 'system'
  }
}

function media(): MediaQueryList | undefined {
  return typeof window !== 'undefined' ? window.matchMedia?.('(prefers-color-scheme: dark)') : undefined
}

function resolve(preference: ThemePreference): Theme {
  if (preference !== 'system') {
    return preference
  }
  return media()?.matches ? 'dark' : 'light'
}

function apply(theme: Theme) {
  document.documentElement.classList.toggle('dark', theme === 'dark')
  document.documentElement.style.colorScheme = theme
}

const listeners = new Set<() => void>()
let snapshot = { preference: readPreference(), theme: resolve(readPreference()) }

function publish() {
  const preference = readPreference()
  snapshot = { preference, theme: resolve(preference) }
  apply(snapshot.theme)
  listeners.forEach((listener) => listener())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  const query = media()
  // Follow the system while the preference is 'system'.
  query?.addEventListener?.('change', publish)
  return () => {
    listeners.delete(listener)
    query?.removeEventListener?.('change', publish)
  }
}

export function setThemePreference(preference: ThemePreference) {
  try {
    if (preference === 'system') {
      localStorage.removeItem(THEME_STORAGE_KEY)
    } else {
      localStorage.setItem(THEME_STORAGE_KEY, preference)
    }
  } catch {
    // Private mode or blocked storage: the choice still applies to this visit.
  }
  publish()
}

export function useTheme() {
  const current = useSyncExternalStore(subscribe, () => snapshot)

  return {
    theme: current.theme,
    preference: current.preference,
    setPreference: setThemePreference,
    toggle: () => setThemePreference(current.theme === 'dark' ? 'light' : 'dark'),
  }
}

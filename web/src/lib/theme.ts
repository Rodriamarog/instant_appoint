import { useCallback, useEffect, useState } from 'react'

/**
 * Light or dark, as the person chose it — or, until they choose, whatever
 * their system is set to.
 *
 * The choice lives in localStorage and is applied as a `dark` class on
 * <html>, which is what globals.css's dark tokens key off. index.html applies
 * the same rule in an inline script before the bundle loads, so a dark-mode
 * user is never flashed a white page on refresh; keep the two in step.
 */
export type Theme = 'light' | 'dark'

export const THEME_STORAGE_KEY = 'tema'

function stored(): Theme | null {
  try {
    const value = localStorage.getItem(THEME_STORAGE_KEY)
    return value === 'light' || value === 'dark' ? value : null
  } catch {
    return null
  }
}

function system(): Theme {
  return typeof window !== 'undefined' &&
    window.matchMedia?.('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light'
}

export function currentTheme(): Theme {
  return stored() ?? system()
}

export function applyTheme(theme: Theme) {
  document.documentElement.classList.toggle('dark', theme === 'dark')
  document.documentElement.style.colorScheme = theme
}

export function useTheme() {
  const [theme, setTheme] = useState<Theme>(currentTheme)

  // Follow the system while nothing has been chosen explicitly.
  useEffect(() => {
    const media = window.matchMedia?.('(prefers-color-scheme: dark)')
    if (!media) {
      return
    }
    const follow = () => {
      if (!stored()) {
        setTheme(system())
      }
    }
    media.addEventListener?.('change', follow)
    return () => media.removeEventListener?.('change', follow)
  }, [])

  useEffect(() => applyTheme(theme), [theme])

  const toggle = useCallback(() => {
    setTheme((current) => {
      const next: Theme = current === 'dark' ? 'light' : 'dark'
      try {
        localStorage.setItem(THEME_STORAGE_KEY, next)
      } catch {
        // Private mode or blocked storage: the switch still works for this visit.
      }
      return next
    })
  }, [])

  return { theme, toggle }
}

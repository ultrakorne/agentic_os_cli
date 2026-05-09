import type { ThemeDef } from '../theme'
import { gruvbox } from './gruvbox'
import { tokyonight } from './tokyonight'

export const THEMES: ThemeDef[] = [gruvbox, tokyonight]

export const DEFAULT_THEME_ID = gruvbox.id

export function findTheme(id: string | undefined | null): ThemeDef {
  if (!id) return getDefaultTheme()
  return THEMES.find((t) => t.id === id) ?? getDefaultTheme()
}

export function getDefaultTheme(): ThemeDef {
  return THEMES.find((t) => t.id === DEFAULT_THEME_ID) ?? THEMES[0]
}

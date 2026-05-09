import type { Theme } from '@shared/theme'

const VAR_MAP: Array<[keyof Theme['colors'], string]> = [
  ['bg', '--theme-bg'],
  ['fg', '--theme-fg'],
  ['accent', '--theme-accent'],
  ['cursor', '--theme-cursor'],
  ['selectionBg', '--theme-selection-bg'],
  ['selectionFg', '--theme-selection-fg'],
  ['color0', '--theme-c0'],
  ['color1', '--theme-c1'],
  ['color2', '--theme-c2'],
  ['color3', '--theme-c3'],
  ['color4', '--theme-c4'],
  ['color5', '--theme-c5'],
  ['color6', '--theme-c6'],
  ['color7', '--theme-c7'],
  ['color8', '--theme-c8'],
  ['color9', '--theme-c9'],
  ['color10', '--theme-c10'],
  ['color11', '--theme-c11'],
  ['color12', '--theme-c12'],
  ['color13', '--theme-c13'],
  ['color14', '--theme-c14'],
  ['color15', '--theme-c15']
]

export function applyTheme(theme: Theme): void {
  const root = document.documentElement
  for (const [key, varName] of VAR_MAP) {
    root.style.setProperty(varName, theme.colors[key])
  }
  root.dataset.themeName = theme.name
  root.dataset.themeSource = theme.source
}

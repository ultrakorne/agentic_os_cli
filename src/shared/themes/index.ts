import type { ThemeDef } from '../theme'
import { catppuccin } from './catppuccin'
import { catppuccinLatte } from './catppuccin-latte'
import { ethereal } from './ethereal'
import { everforest } from './everforest'
import { flexokiLight } from './flexoki-light'
import { gruvbox } from './gruvbox'
import { hackerman } from './hackerman'
import { kanagawa } from './kanagawa'
import { lumon } from './lumon'
import { matteBlack } from './matte-black'
import { miasma } from './miasma'
import { nord } from './nord'
import { osakaJade } from './osaka-jade'
import { retro82 } from './retro-82'
import { ristretto } from './ristretto'
import { rosePine } from './rose-pine'
import { tokyoNight } from './tokyo-night'
import { vantablack } from './vantablack'
import { white } from './white'

export const THEMES: ThemeDef[] = [
  catppuccin,
  catppuccinLatte,
  ethereal,
  everforest,
  flexokiLight,
  gruvbox,
  hackerman,
  kanagawa,
  lumon,
  matteBlack,
  miasma,
  nord,
  osakaJade,
  retro82,
  ristretto,
  rosePine,
  tokyoNight,
  vantablack,
  white
]

export const DEFAULT_THEME_ID = gruvbox.id

export function findTheme(id: string | undefined | null): ThemeDef {
  if (!id) return getDefaultTheme()
  return THEMES.find((t) => t.id === id) ?? getDefaultTheme()
}

export function getDefaultTheme(): ThemeDef {
  return THEMES.find((t) => t.id === DEFAULT_THEME_ID) ?? THEMES[0]
}

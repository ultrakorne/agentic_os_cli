import { promises as fs, watch, type FSWatcher } from 'fs'
import { homedir } from 'os'
import { join } from 'path'
import type { Theme, ThemeColors } from '../../shared/theme'

const COLORS_PATH = join(homedir(), '.config/omarchy/current/theme/colors.toml')
const THEME_NAME_PATH = join(homedir(), '.config/omarchy/current/theme.name')

const FALLBACK: ThemeColors = {
  bg: '#16161e',
  fg: '#c0caf5',
  accent: '#bb9af7',
  cursor: '#c0caf5',
  selectionFg: '#c0caf5',
  selectionBg: '#283457',
  color0: '#15161e',
  color1: '#f7768e',
  color2: '#9ece6a',
  color3: '#e0af68',
  color4: '#7aa2f7',
  color5: '#bb9af7',
  color6: '#7dcfff',
  color7: '#a9b1d6',
  color8: '#414868',
  color9: '#f7768e',
  color10: '#9ece6a',
  color11: '#e0af68',
  color12: '#7aa2f7',
  color13: '#bb9af7',
  color14: '#7dcfff',
  color15: '#c0caf5'
}

function parseColorsToml(content: string): Partial<Record<string, string>> {
  const result: Record<string, string> = {}
  for (const line of content.split('\n')) {
    const m = line.match(/^\s*([a-z0-9_]+)\s*=\s*"([^"]+)"\s*$/i)
    if (m) result[m[1]] = m[2]
  }
  return result
}

function asColors(raw: Partial<Record<string, string>>): ThemeColors {
  const pick = (key: string, fallback: string): string => raw[key] ?? fallback
  return {
    bg: pick('background', FALLBACK.bg),
    fg: pick('foreground', FALLBACK.fg),
    accent: pick('accent', FALLBACK.accent),
    cursor: pick('cursor', FALLBACK.cursor),
    selectionFg: pick('selection_foreground', FALLBACK.selectionFg),
    selectionBg: pick('selection_background', FALLBACK.selectionBg),
    color0: pick('color0', FALLBACK.color0),
    color1: pick('color1', FALLBACK.color1),
    color2: pick('color2', FALLBACK.color2),
    color3: pick('color3', FALLBACK.color3),
    color4: pick('color4', FALLBACK.color4),
    color5: pick('color5', FALLBACK.color5),
    color6: pick('color6', FALLBACK.color6),
    color7: pick('color7', FALLBACK.color7),
    color8: pick('color8', FALLBACK.color8),
    color9: pick('color9', FALLBACK.color9),
    color10: pick('color10', FALLBACK.color10),
    color11: pick('color11', FALLBACK.color11),
    color12: pick('color12', FALLBACK.color12),
    color13: pick('color13', FALLBACK.color13),
    color14: pick('color14', FALLBACK.color14),
    color15: pick('color15', FALLBACK.color15)
  }
}

async function readName(): Promise<string> {
  try {
    return (await fs.readFile(THEME_NAME_PATH, 'utf8')).trim()
  } catch {
    return 'fallback'
  }
}

export async function loadTheme(): Promise<Theme> {
  try {
    const content = await fs.readFile(COLORS_PATH, 'utf8')
    const raw = parseColorsToml(content)
    return {
      name: await readName(),
      colors: asColors(raw),
      source: 'omarchy'
    }
  } catch {
    return { name: 'fallback', colors: FALLBACK, source: 'fallback' }
  }
}

export class ThemeWatcher {
  private watchers: FSWatcher[] = []
  private debounce: NodeJS.Timeout | null = null

  start(onChange: (theme: Theme) => void): void {
    const fire = (): void => {
      if (this.debounce) clearTimeout(this.debounce)
      this.debounce = setTimeout(() => {
        void loadTheme().then(onChange)
      }, 80)
    }
    for (const path of [COLORS_PATH, THEME_NAME_PATH]) {
      try {
        this.watchers.push(watch(path, () => fire()))
      } catch {
        // file missing on this system — fallback theme is active, watcher skipped
      }
    }
  }

  stop(): void {
    for (const w of this.watchers) {
      try {
        w.close()
      } catch {
        /* noop */
      }
    }
    this.watchers = []
    if (this.debounce) clearTimeout(this.debounce)
  }
}

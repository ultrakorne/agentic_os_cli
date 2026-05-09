import { promises as fs } from 'fs'
import { dirname } from 'path'
import type { Theme, ThemeSummary } from '../../shared/theme'
import { DEFAULT_THEME_ID, THEMES, findTheme } from '../../shared/themes'

export type ThemeStore = {
  load(): Promise<Theme>
  set(id: string): Promise<Theme>
  list(): ThemeSummary[]
}

export function listThemes(): ThemeSummary[] {
  return THEMES.map((t) => ({ id: t.id, name: t.name }))
}

export function createThemeStore(prefsPath: string): ThemeStore {
  let cachedId: string | null = null

  async function readId(): Promise<string> {
    if (cachedId) return cachedId
    try {
      const raw = await fs.readFile(prefsPath, 'utf8')
      const parsed = JSON.parse(raw) as { id?: unknown }
      if (typeof parsed.id === 'string') {
        cachedId = parsed.id
        return cachedId
      }
    } catch {
      // missing or unreadable — fall through to default
    }
    cachedId = DEFAULT_THEME_ID
    return cachedId
  }

  async function writeId(id: string): Promise<void> {
    await fs.mkdir(dirname(prefsPath), { recursive: true })
    await fs.writeFile(prefsPath, JSON.stringify({ id }, null, 2), 'utf8')
    cachedId = id
  }

  return {
    async load(): Promise<Theme> {
      const id = await readId()
      return findTheme(id)
    },
    async set(id: string): Promise<Theme> {
      const theme = findTheme(id)
      await writeId(theme.id)
      return theme
    },
    list(): ThemeSummary[] {
      return listThemes()
    }
  }
}

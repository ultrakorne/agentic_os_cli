import { promises as fs, constants as fsc } from 'fs'
import { basename, extname, join } from 'path'

const SUPPORTED_EXTS = new Set(['.sh', '.bash', '.zsh', ''])
const DEFAULT_SECTION = 'Agents'

export type ScriptInfo = {
  id: string
  scriptPath: string
  section: string
}

/**
 * Walks <userData>/agents/. Top-level scripts get section "Agents"; scripts
 * inside a first-level subdirectory adopt that directory's name as their
 * section. Deeper nesting is ignored. IDs (filename minus extension) must
 * be unique across the whole tree — duplicates are dropped with a console
 * warning so the dashboard never shows two agents with the same id.
 */
export async function scanScripts(agentsDir: string): Promise<ScriptInfo[]> {
  await fs.mkdir(agentsDir, { recursive: true })
  const out: ScriptInfo[] = []
  const seen = new Map<string, string>() // id -> first scriptPath we saw

  await collectInto(agentsDir, DEFAULT_SECTION, out, seen)

  let entries: import('fs').Dirent[]
  try {
    entries = await fs.readdir(agentsDir, { withFileTypes: true })
  } catch {
    entries = []
  }
  for (const e of entries) {
    if (!e.isDirectory()) continue
    if (e.name.startsWith('.')) continue
    await collectInto(join(agentsDir, e.name), e.name, out, seen)
  }

  out.sort((a, b) => a.id.localeCompare(b.id))
  return out
}

async function collectInto(
  dir: string,
  section: string,
  out: ScriptInfo[],
  seen: Map<string, string>
): Promise<void> {
  let entries: string[]
  try {
    entries = await fs.readdir(dir)
  } catch {
    return
  }
  for (const name of entries) {
    if (name.startsWith('.')) continue
    if (name.toLowerCase() === 'readme.md') continue

    const ext = extname(name)
    if (!SUPPORTED_EXTS.has(ext)) continue

    const fullPath = join(dir, name)
    let stat: import('fs').Stats
    try {
      stat = await fs.stat(fullPath)
    } catch {
      continue
    }
    if (!stat.isFile()) continue
    if (!(await isExecutable(fullPath))) continue

    const id = basename(name, ext)
    if (id.startsWith('__')) {
      console.warn(
        `[scanner] reserved agent id "${id}" — the __-prefix is for internal markers; ignoring ${fullPath}`
      )
      continue
    }
    const previous = seen.get(id)
    if (previous) {
      console.warn(
        `[scanner] duplicate agent id "${id}" — keeping ${previous}, ignoring ${fullPath}`
      )
      continue
    }
    seen.set(id, fullPath)
    out.push({ id, scriptPath: fullPath, section })
  }
}

async function isExecutable(path: string): Promise<boolean> {
  try {
    await fs.access(path, fsc.X_OK)
    return true
  } catch {
    return false
  }
}

export async function ensureAgentsDir(agentsDir: string, seedFrom: string): Promise<void> {
  await fs.mkdir(agentsDir, { recursive: true })
  const existing = await fs.readdir(agentsDir).catch(() => [] as string[])
  if (existing.length > 0) return
  let seedEntries: string[]
  try {
    seedEntries = await fs.readdir(seedFrom)
  } catch {
    return
  }
  for (const name of seedEntries) {
    const src = join(seedFrom, name)
    const dst = join(agentsDir, name)
    try {
      const data = await fs.readFile(src)
      await fs.writeFile(dst, data)
      const stat = await fs.stat(src)
      await fs.chmod(dst, stat.mode)
    } catch {
      /* skip un-readable seeds */
    }
  }
}

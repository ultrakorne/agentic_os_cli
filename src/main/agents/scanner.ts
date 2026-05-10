import { promises as fs, constants as fsc } from 'fs'
import { basename, extname, join } from 'path'
import type { AgentMeta, AgentScanIssue } from '../../shared/scheduler'

const SUPPORTED_EXTS = new Set(['.sh', '.bash', '.zsh', ''])
const DEFAULT_SECTION = 'Agents'

export const META_SUFFIX = '.meta.json'

export type AgentEntry = {
  id: string
  scriptPath: string
  section: string
  metaPath: string
  meta: AgentMeta
}

/**
 * Walks <userData>/agents/. Top-level scripts get section "Agents"; scripts
 * inside a first-level subdirectory adopt that directory's name as their
 * section. Deeper nesting is ignored. IDs (filename minus extension) must
 * be unique across the whole tree — duplicates are dropped with a console
 * warning so the dashboard never shows two agents with the same id.
 *
 * For each script, a sibling `<id>.meta.json` is read if present and folded
 * in as `meta`. Missing or malformed sidecars degrade to an empty meta.
 */
export async function scanScripts(agentsDir: string): Promise<AgentEntry[]> {
  await fs.mkdir(agentsDir, { recursive: true })
  const out: AgentEntry[] = []
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
  out: AgentEntry[],
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
    if (name.endsWith(META_SUFFIX)) continue

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
    const metaPath = join(dir, `${id}${META_SUFFIX}`)
    const meta = await readMeta(metaPath)
    out.push({ id, scriptPath: fullPath, section, metaPath, meta })
  }
}

/**
 * Returns scripts that look like they were *intended* to be agents but were
 * silently dropped by `scanScripts`. Today: files with a supported extension
 * (or no extension and a `#!` shebang) that lack the executable bit. Walks
 * the same top-level + first-level-subdir layout as `scanScripts`.
 */
export async function findScanIssues(agentsDir: string): Promise<AgentScanIssue[]> {
  const issues: AgentScanIssue[] = []
  await collectIssues(agentsDir, issues)

  let entries: import('fs').Dirent[]
  try {
    entries = await fs.readdir(agentsDir, { withFileTypes: true })
  } catch {
    return issues
  }
  for (const e of entries) {
    if (!e.isDirectory()) continue
    if (e.name.startsWith('.')) continue
    await collectIssues(join(agentsDir, e.name), issues)
  }
  issues.sort((a, b) => a.path.localeCompare(b.path))
  return issues
}

async function collectIssues(dir: string, issues: AgentScanIssue[]): Promise<void> {
  let entries: string[]
  try {
    entries = await fs.readdir(dir)
  } catch {
    return
  }
  for (const name of entries) {
    if (name.startsWith('.')) continue
    if (name.toLowerCase() === 'readme.md') continue
    if (name.endsWith(META_SUFFIX)) continue

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
    if (await isExecutable(fullPath)) continue

    // No-extension files without a shebang aren't agents — skip silently.
    if (ext === '' && !(await hasShebang(fullPath))) continue

    issues.push({ kind: 'not-executable', path: fullPath })
  }
}

async function hasShebang(path: string): Promise<boolean> {
  try {
    const fh = await fs.open(path, 'r')
    try {
      const buf = Buffer.alloc(2)
      const { bytesRead } = await fh.read(buf, 0, 2, 0)
      return bytesRead === 2 && buf[0] === 0x23 && buf[1] === 0x21
    } finally {
      await fh.close()
    }
  } catch {
    return false
  }
}

async function readMeta(metaPath: string): Promise<AgentMeta> {
  let txt: string
  try {
    txt = await fs.readFile(metaPath, 'utf8')
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === 'ENOENT') return {}
    console.warn(`[scanner] failed to read ${metaPath}:`, err)
    return {}
  }
  try {
    const parsed = JSON.parse(txt)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as AgentMeta
    }
    console.warn(`[scanner] ${metaPath} is not a JSON object — ignoring`)
    return {}
  } catch (err) {
    console.warn(`[scanner] failed to parse ${metaPath}:`, err)
    return {}
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

import { promises as fs, constants as fsc } from 'fs'
import { basename, extname, join } from 'path'
import type { Agent } from '../scheduler/types'

// Agents are shell scripts. Non-shell tools (Python, Node, Ruby, …) belong
// behind a one-line bash shim — the explicit contract is clearer than a
// magical "any executable" rule, and matches what users see when they run
// `crontab -l` or read the agents/ folder by hand.
const SUPPORTED_EXTS = new Set(['.sh', '.bash', '.zsh', ''])

export type AgentMetaFile = {
  title?: string
  description?: string
  section?: string
}

export async function scanAgents(agentsDir: string): Promise<Agent[]> {
  await fs.mkdir(agentsDir, { recursive: true })
  let entries: string[]
  try {
    entries = await fs.readdir(agentsDir)
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === 'ENOENT') return []
    throw err
  }

  const out: Agent[] = []
  for (const name of entries) {
    if (name.startsWith('.')) continue
    if (name.endsWith('.meta.json')) continue
    if (name.toLowerCase() === 'readme.md') continue

    const ext = extname(name)
    if (!SUPPORTED_EXTS.has(ext)) continue

    const fullPath = join(agentsDir, name)
    let stat: import('fs').Stats
    try {
      stat = await fs.stat(fullPath)
    } catch {
      continue
    }
    if (!stat.isFile()) continue

    if (!(await isExecutable(fullPath))) continue

    const id = basename(name, ext)
    const meta = await readMeta(join(agentsDir, `${id}.meta.json`))

    out.push({
      id,
      title: meta?.title ?? humanize(id),
      description: meta?.description ?? '',
      section: meta?.section ?? 'Agents',
      scriptPath: fullPath
    })
  }

  out.sort((a, b) => a.id.localeCompare(b.id))
  return out
}

async function isExecutable(path: string): Promise<boolean> {
  try {
    await fs.access(path, fsc.X_OK)
    return true
  } catch {
    return false
  }
}

async function readMeta(path: string): Promise<AgentMetaFile | null> {
  try {
    const txt = await fs.readFile(path, 'utf8')
    const parsed = JSON.parse(txt) as AgentMetaFile
    return parsed
  } catch {
    return null
  }
}

function humanize(id: string): string {
  return id
    .replace(/[-_]+/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .trim()
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

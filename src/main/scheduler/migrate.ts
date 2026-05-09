import { promises as fs } from 'fs'
import { dirname, join } from 'path'
import type { JobRun, Schedule } from './types'

type LegacyShape = {
  schedules?: Schedule[]
  state?: Record<string, { lastFiredAt: string | null }>
  runs?: JobRun[]
}

export type DataPaths = {
  schedules: string
  state: string
  runs: string
  runsDir: string
}

export function defaultDataPaths(dataDir: string): DataPaths {
  return {
    schedules: join(dataDir, 'schedules.json'),
    state: join(dataDir, 'state.json'),
    runs: join(dataDir, 'runs.jsonl'),
    runsDir: join(dataDir, 'runs')
  }
}

/**
 * Splits an old combined state.json (schedules + state + runs) into the new
 * three-file layout. Detects legacy by content shape (presence of `schedules`
 * or `runs` keys at the root of state.json).
 */
export async function migrateLegacyStateIfNeeded(paths: DataPaths): Promise<boolean> {
  let raw: unknown
  try {
    const txt = await fs.readFile(paths.state, 'utf8')
    raw = JSON.parse(txt)
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === 'ENOENT') return false
    throw err
  }

  if (!isLegacyShape(raw)) return false

  try {
    await fs.access(paths.schedules)
    return false
  } catch {
    /* schedules.json absent — safe to migrate */
  }

  const legacy = raw as LegacyShape
  await fs.mkdir(dirname(paths.state), { recursive: true })

  await fs.rename(paths.state, `${paths.state}.legacy`)
  await writeJsonAtomic(paths.schedules, { schedules: legacy.schedules ?? [] })
  await writeJsonAtomic(paths.state, { state: legacy.state ?? {} })

  if (legacy.runs && legacy.runs.length > 0) {
    const lines = legacy.runs.map((r) => JSON.stringify(r)).join('\n') + '\n'
    await fs.writeFile(paths.runs, lines)
  }

  return true
}

function isLegacyShape(raw: unknown): boolean {
  if (!raw || typeof raw !== 'object') return false
  return 'schedules' in raw || 'runs' in raw
}

async function writeJsonAtomic(path: string, data: unknown): Promise<void> {
  const tmp = `${path}.tmp`
  await fs.writeFile(tmp, JSON.stringify(data, null, 2))
  await fs.rename(tmp, path)
}

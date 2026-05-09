import { promises as fs, watch, type FSWatcher } from 'fs'
import { join } from 'path'
import type { JobRun } from './types'

const DEFAULT_CACHE_LIMIT = 500
const DEBOUNCE_MS = 250
const OUTPUT_TAIL_BYTES = 4096
const RUNS_HARD_CAP = 2000 // max distinct runs (each run = 1 .json + up to 1 .out)

export class RunsStore {
  private cache = new Map<string, JobRun>()
  private legacy: JobRun[] = []
  private watcher: FSWatcher | null = null
  private debounceTimer: NodeJS.Timeout | null = null
  private onChangeCb: (() => void) | null = null

  constructor(
    private runsDir: string,
    private legacyJsonl: string,
    private cacheLimit: number = DEFAULT_CACHE_LIMIT
  ) {}

  async load(): Promise<void> {
    await fs.mkdir(this.runsDir, { recursive: true })
    await this.loadLegacy()
    await this.indexRunsDir()
    await this.gcRunsDir()
  }

  private async loadLegacy(): Promise<void> {
    try {
      const txt = await fs.readFile(this.legacyJsonl, 'utf8')
      const lines = txt.split('\n').filter((l) => l.length > 0)
      const recent = lines.slice(-this.cacheLimit)
      const out: JobRun[] = []
      for (const l of recent) {
        try {
          const r = JSON.parse(l) as Partial<JobRun>
          if (typeof r.id !== 'string') continue
          out.push(normalizeRun(r))
        } catch {
          /* skip malformed */
        }
      }
      this.legacy = out
    } catch (err) {
      if ((err as NodeJS.ErrnoException).code !== 'ENOENT') throw err
    }
  }

  private async indexRunsDir(): Promise<void> {
    let entries: string[]
    try {
      entries = await fs.readdir(this.runsDir)
    } catch {
      return
    }
    for (const name of entries) {
      if (!name.endsWith('.json')) continue
      await this.ingestRunFile(name)
    }
    this.trimCache()
  }

  async ingestRunFile(name: string): Promise<void> {
    const fullPath = join(this.runsDir, name)
    try {
      const txt = await fs.readFile(fullPath, 'utf8')
      const raw = JSON.parse(txt) as Partial<JobRun>
      if (typeof raw.id !== 'string') return
      const run = normalizeRun(raw)
      if (run.status !== 'running' && run.outputPath) {
        run.output = await readTail(join(this.runsDir, run.outputPath))
      }
      this.cache.set(run.id, run)
    } catch {
      /* file may have been removed between listing and read */
    }
  }

  list(jobId?: string, limit = 100): JobRun[] {
    const out: JobRun[] = []
    for (const r of this.cache.values()) {
      if (!jobId || r.jobId === jobId) out.push(r)
    }
    for (const r of this.legacy) {
      if (this.cache.has(r.id)) continue
      if (!jobId || r.jobId === jobId) out.push(r)
    }
    out.sort((a, b) => b.startedAt.localeCompare(a.startedAt))
    return out.slice(0, limit).map((r) => ({ ...r }))
  }

  async readOutput(runId: string): Promise<string | null> {
    const cached = this.cache.get(runId)
    if (cached?.outputPath) {
      try {
        return await fs.readFile(join(this.runsDir, cached.outputPath), 'utf8')
      } catch {
        return null
      }
    }
    const legacy = this.legacy.find((r) => r.id === runId)
    return legacy?.output ?? null
  }

  startWatching(onChange: () => void): void {
    this.onChangeCb = onChange
    try {
      this.watcher = watch(this.runsDir, { persistent: false }, () => {
        this.scheduleRescan()
      })
    } catch (err) {
      console.error('[runs-store] watch failed:', err)
    }
  }

  stopWatching(): void {
    if (this.debounceTimer) {
      clearTimeout(this.debounceTimer)
      this.debounceTimer = null
    }
    if (this.watcher) {
      this.watcher.close()
      this.watcher = null
    }
    this.onChangeCb = null
  }

  private scheduleRescan(): void {
    if (this.debounceTimer) clearTimeout(this.debounceTimer)
    this.debounceTimer = setTimeout(() => {
      void this.rescan()
    }, DEBOUNCE_MS)
  }

  private async rescan(): Promise<void> {
    await this.indexRunsDir()
    this.onChangeCb?.()
  }

  private trimCache(): void {
    if (this.cache.size <= this.cacheLimit) return
    const sorted = [...this.cache.values()].sort((a, b) =>
      a.startedAt.localeCompare(b.startedAt)
    )
    const toRemove = sorted.length - this.cacheLimit
    for (let i = 0; i < toRemove; i++) this.cache.delete(sorted[i].id)
  }

  private async gcRunsDir(): Promise<void> {
    let entries: string[]
    try {
      entries = await fs.readdir(this.runsDir)
    } catch {
      return
    }

    // Group <run-id>.json + <run-id>.out together so the GC always deletes
    // them as a pair — never leaves an orphan .out behind when its .json
    // ages out (or vice versa).
    const runs = new Map<string, { files: string[]; mtime: number }>()
    for (const name of entries) {
      const dot = name.lastIndexOf('.')
      if (dot < 0) continue
      const stem = name.slice(0, dot)
      const ext = name.slice(dot)
      if (ext !== '.json' && ext !== '.out') continue
      let entry = runs.get(stem)
      if (!entry) {
        entry = { files: [], mtime: 0 }
        runs.set(stem, entry)
      }
      entry.files.push(name)
      try {
        const s = await fs.stat(join(this.runsDir, name))
        if (s.mtimeMs > entry.mtime) entry.mtime = s.mtimeMs
      } catch {
        /* file removed mid-scan */
      }
    }

    if (runs.size <= RUNS_HARD_CAP) return

    const sorted = [...runs.values()].sort((a, b) => a.mtime - b.mtime)
    const drop = sorted.slice(0, sorted.length - RUNS_HARD_CAP)
    for (const run of drop) {
      for (const f of run.files) {
        await fs.unlink(join(this.runsDir, f)).catch(() => {})
      }
    }
  }
}

function normalizeRun(r: Partial<JobRun>): JobRun {
  return {
    id: String(r.id ?? ''),
    jobId: String(r.jobId ?? ''),
    scheduleId: r.scheduleId ?? null,
    trigger: (r.trigger as JobRun['trigger']) ?? 'schedule',
    startedAt: String(r.startedAt ?? new Date().toISOString()),
    endedAt: r.endedAt ?? null,
    status: (r.status as JobRun['status']) ?? 'running',
    output: r.output ?? '',
    error: r.error ?? null,
    exitCode: r.exitCode ?? null,
    outputPath: r.outputPath ?? null
  }
}

async function readTail(path: string): Promise<string> {
  try {
    const txt = await fs.readFile(path, 'utf8')
    if (txt.length <= OUTPUT_TAIL_BYTES) return txt
    return '… ' + txt.slice(-OUTPUT_TAIL_BYTES)
  } catch {
    return ''
  }
}

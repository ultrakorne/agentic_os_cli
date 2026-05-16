import { promises as fs, watch, type FSWatcher } from 'fs'
import { join } from 'path'
import { execCapture } from '../exec'
import type { JobRun } from './types'

const DEFAULT_LIMIT = 500
const DEBOUNCE_MS = 250

// Read-side cache over `<aos_home>/runs/`. The directory watcher debounces
// filesystem events and asks `aos runs --json --limit N` for the current
// snapshot; the CLI owns parsing, normalization, sort order, and any disk
// hygiene (logtrim/GC). The renderer is a view: we only watch the dir, hold
// the latest snapshot in memory, and read .out files on demand when a row
// expands.
export class RunsStore {
  private cache: JobRun[] = []
  private watcher: FSWatcher | null = null
  private debounceTimer: NodeJS.Timeout | null = null
  private onChangeCb: (() => void) | null = null

  constructor(
    private aosBin: string,
    private runsDir: string,
    private limit: number = DEFAULT_LIMIT
  ) {}

  async load(): Promise<void> {
    await fs.mkdir(this.runsDir, { recursive: true })
    await this.reindex()
  }

  list(jobId?: string, limit = 100): JobRun[] {
    const filtered = jobId ? this.cache.filter((r) => r.jobId === jobId) : this.cache
    return filtered.slice(0, limit).map((r) => ({ ...r }))
  }

  // readOutput reads <aos_home>/runs/<run-id>.out (or the path the CLI
  // recorded in run.outputPath). Plain fs.readFile is fine — the renderer
  // is a view, reading is not a system mutation.
  async readOutput(runId: string): Promise<string | null> {
    const cached = this.cache.find((r) => r.id === runId)
    const name = cached?.outputPath ?? `${runId}.out`
    try {
      return await fs.readFile(join(this.runsDir, name), 'utf8')
    } catch {
      return null
    }
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
    await this.reindex()
    this.onChangeCb?.()
  }

  private async reindex(): Promise<void> {
    const res = await execCapture(this.aosBin, ['runs', '--json', '--limit', String(this.limit)])
    if (res.code !== 0) return
    try {
      const parsed = JSON.parse(res.stdout) as { runs?: JobRun[] }
      if (Array.isArray(parsed.runs)) this.cache = parsed.runs
    } catch {
      /* malformed output — keep prior cache, the next tick will retry */
    }
  }
}

import { promises as fs, watch, type FSWatcher } from 'fs'
import { join } from 'path'
import type { MissedRun } from './types'

const DEBOUNCE_MS = 250

// Reads <aos_home>/misses/ — files written by `aos tick` / `aos refresh`,
// one per outstanding missed run. The CLI rebuilds this directory each
// invocation, so the dashboard never re-derives misses on its own.
export class MissesStore {
  private cache: MissedRun[] = []
  private watcher: FSWatcher | null = null
  private debounceTimer: NodeJS.Timeout | null = null
  private onChangeCb: (() => void) | null = null

  constructor(private missesDir: string) {}

  async load(): Promise<void> {
    await fs.mkdir(this.missesDir, { recursive: true })
    await this.reindex()
  }

  list(): MissedRun[] {
    return this.cache.map((m) => ({ ...m }))
  }

  startWatching(onChange: () => void): void {
    this.onChangeCb = onChange
    try {
      this.watcher = watch(this.missesDir, { persistent: false }, () => {
        this.scheduleRescan()
      })
    } catch (err) {
      console.error('[misses-store] watch failed:', err)
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
    let entries: string[]
    try {
      entries = await fs.readdir(this.missesDir)
    } catch {
      this.cache = []
      return
    }
    const next: MissedRun[] = []
    for (const name of entries) {
      if (!name.endsWith('.json')) continue
      try {
        const txt = await fs.readFile(join(this.missesDir, name), 'utf8')
        const raw = JSON.parse(txt) as Partial<MissedRun>
        if (typeof raw.agentId === 'string' && typeof raw.expectedAt === 'string') {
          next.push({ agentId: raw.agentId, expectedAt: raw.expectedAt })
        }
      } catch {
        /* skip malformed sidecars — the CLI will rewrite on the next tick */
      }
    }
    // Newest first, matching the dashboard's existing missed-list ordering.
    next.sort((a, b) => b.expectedAt.localeCompare(a.expectedAt))
    this.cache = next
  }
}

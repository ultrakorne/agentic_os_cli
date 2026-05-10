import { promises as fs } from 'fs'
import { dirname } from 'path'
import type { AgentMeta, ScheduleSpec } from './types'

/**
 * Reads and writes per-agent sidecar files (`<id>.meta.json`). Each path has
 * its own write queue so concurrent updates to the same file serialize, and
 * writes use temp+rename so a crash never leaves a half-written sidecar.
 *
 * The store has no in-memory cache — the scanner reads sidecars during the
 * directory walk, and the engine's in-memory entries are the single source
 * of truth between writes.
 */
export class AgentMetaStore {
  private queues = new Map<string, Promise<void>>()

  async read(metaPath: string): Promise<AgentMeta> {
    try {
      const txt = await fs.readFile(metaPath, 'utf8')
      const parsed = JSON.parse(txt)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return parsed as AgentMeta
      }
      return {}
    } catch (err) {
      if ((err as NodeJS.ErrnoException).code === 'ENOENT') return {}
      throw err
    }
  }

  setSchedule(metaPath: string, schedule: ScheduleSpec | null): Promise<AgentMeta> {
    return this.mutate(metaPath, (cur) => {
      if (schedule == null) {
        const { schedule: _s, scheduledAt: _t, ...rest } = cur
        void _s
        void _t
        return rest
      }
      const specChanged = !specsEqual(cur.schedule, schedule)
      const scheduledAt = specChanged
        ? new Date().toISOString()
        : (cur.scheduledAt ?? new Date().toISOString())
      return { ...cur, schedule, scheduledAt }
    })
  }

  setDescription(metaPath: string, description: string): Promise<AgentMeta> {
    return this.mutate(metaPath, (cur) => ({ ...cur, description }))
  }

  private mutate(
    metaPath: string,
    fn: (cur: AgentMeta) => AgentMeta
  ): Promise<AgentMeta> {
    let next: AgentMeta = {}
    const prev = this.queues.get(metaPath) ?? Promise.resolve()
    const job = prev.then(async () => {
      const cur = await this.read(metaPath)
      next = fn(cur)
      await writeJson(metaPath, next)
    })
    this.queues.set(
      metaPath,
      job.catch(() => {
        /* keep the chain alive even if a write fails */
      })
    )
    return job.then(() => next)
  }
}

async function writeJson(path: string, data: unknown): Promise<void> {
  await fs.mkdir(dirname(path), { recursive: true })
  if (isEmpty(data)) {
    // An empty meta is the same as no sidecar. Remove the file rather than
    // leave a stub `{}` lying around next to the script.
    await fs.rm(path, { force: true })
    return
  }
  const tmp = `${path}.tmp`
  await fs.writeFile(tmp, JSON.stringify(data, null, 2))
  await fs.rename(tmp, path)
}

function isEmpty(data: unknown): boolean {
  return (
    data !== null &&
    typeof data === 'object' &&
    !Array.isArray(data) &&
    Object.keys(data as Record<string, unknown>).length === 0
  )
}

function specsEqual(a: ScheduleSpec | undefined, b: ScheduleSpec | undefined): boolean {
  if (!a || !b) return a === b
  if (a.kind !== b.kind) return false
  switch (a.kind) {
    case 'hourly': {
      if (b.kind !== 'hourly') return false
      return a.everyHours === b.everyHours && a.minute === b.minute
    }
    case 'daily': {
      if (b.kind !== 'daily') return false
      if (a.hour !== b.hour || a.minute !== b.minute) return false
      if (a.days.length !== b.days.length) return false
      const set = new Set(a.days)
      return b.days.every((d) => set.has(d))
    }
    default: {
      const _exhaustive: never = a
      void _exhaustive
      return false
    }
  }
}

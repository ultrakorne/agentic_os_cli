import { promises as fs } from 'fs'
import { dirname } from 'path'
import type { AgentConfig, ScheduleSpec } from './types'

type ConfigsFile = { agents: AgentConfig[] }

export class AgentConfigStore {
  private configs: AgentConfig[] = []
  private writeQueue: Promise<void> = Promise.resolve()

  constructor(private path: string) {}

  async load(): Promise<void> {
    await fs.mkdir(dirname(this.path), { recursive: true })
    this.configs = await readJson<ConfigsFile>(this.path, { agents: [] }).then(
      (d) => d.agents ?? []
    )
    // Migration: backfill `scheduledAt` for any pre-existing schedule that
    // lacks one, so missed-runs detection doesn't retroactively flag ticks
    // from before this field existed.
    const nowIso = new Date().toISOString()
    let migrated = false
    for (const cfg of this.configs) {
      if (cfg.schedule && !cfg.scheduledAt) {
        cfg.scheduledAt = nowIso
        migrated = true
      }
    }
    if (migrated) await this.persist()
  }

  list(): AgentConfig[] {
    return this.configs.map((c) => ({ ...c }))
  }

  get(id: string): AgentConfig | undefined {
    const c = this.configs.find((x) => x.id === id)
    return c ? { ...c } : undefined
  }

  upsert(config: AgentConfig): Promise<void> {
    return this.enqueue(() => {
      const idx = this.configs.findIndex((c) => c.id === config.id)
      if (idx >= 0) this.configs[idx] = config
      else this.configs.push(config)
    })
  }

  remove(id: string): Promise<void> {
    return this.enqueue(() => {
      this.configs = this.configs.filter((c) => c.id !== id)
    })
  }

  setSchedule(id: string, schedule: ScheduleSpec | null): Promise<void> {
    return this.enqueue(() => {
      const idx = this.configs.findIndex((c) => c.id === id)
      if (idx >= 0) {
        const cur = this.configs[idx]
        if (schedule == null) {
          const { schedule: _s, scheduledAt: _t, ...rest } = cur
          void _s
          void _t
          this.configs[idx] = rest
        } else {
          const specChanged = !specsEqual(cur.schedule, schedule)
          const scheduledAt = specChanged
            ? new Date().toISOString()
            : (cur.scheduledAt ?? new Date().toISOString())
          this.configs[idx] = { ...cur, schedule, scheduledAt }
        }
      } else if (schedule != null) {
        this.configs.push({ id, schedule, scheduledAt: new Date().toISOString() })
      }
    })
  }

  setDescription(id: string, description: string): Promise<void> {
    return this.enqueue(() => {
      const idx = this.configs.findIndex((c) => c.id === id)
      if (idx >= 0) {
        this.configs[idx] = { ...this.configs[idx], description }
      } else {
        this.configs.push({ id, description })
      }
    })
  }

  private enqueue(mutate: () => void): Promise<void> {
    this.writeQueue = this.writeQueue.then(async () => {
      mutate()
      await this.persist()
    })
    return this.writeQueue
  }

  private async persist(): Promise<void> {
    await writeJson(this.path, { agents: this.configs })
  }
}

async function readJson<T>(path: string, fallback: T): Promise<T> {
  try {
    const txt = await fs.readFile(path, 'utf8')
    return JSON.parse(txt) as T
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code !== 'ENOENT') throw err
    return fallback
  }
}

async function writeJson(path: string, data: unknown): Promise<void> {
  const tmp = `${path}.tmp`
  await fs.writeFile(tmp, JSON.stringify(data, null, 2))
  await fs.rename(tmp, path)
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

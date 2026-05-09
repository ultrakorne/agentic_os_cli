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
          const { schedule: _drop, ...rest } = cur
          void _drop
          this.configs[idx] = rest
        } else {
          this.configs[idx] = { ...cur, schedule }
        }
      } else if (schedule != null) {
        this.configs.push({ id, schedule })
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

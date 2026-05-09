import { promises as fs } from 'fs'
import { dirname } from 'path'
import type { Schedule } from './types'

type SchedulesFile = { schedules: Schedule[] }
type StateFile = { state: Record<string, { lastFiredAt: string | null }> }

export class ScheduleStore {
  private schedules: Schedule[] = []
  private state: Record<string, { lastFiredAt: string | null }> = {}
  private schedulesQueue: Promise<void> = Promise.resolve()
  private stateQueue: Promise<void> = Promise.resolve()

  constructor(
    private schedulesPath: string,
    private statePath: string
  ) {}

  async load(): Promise<void> {
    await fs.mkdir(dirname(this.schedulesPath), { recursive: true })
    this.schedules = await readJson<SchedulesFile>(this.schedulesPath, { schedules: [] }).then(
      (d) => d.schedules ?? []
    )
    this.state = await readJson<StateFile>(this.statePath, { state: {} }).then(
      (d) => d.state ?? {}
    )
  }

  private async persistSchedules(): Promise<void> {
    await writeJson(this.schedulesPath, { schedules: this.schedules })
  }

  private async persistState(): Promise<void> {
    await writeJson(this.statePath, { state: this.state })
  }

  private enqueueSchedules(mutate: () => void): Promise<void> {
    this.schedulesQueue = this.schedulesQueue.then(async () => {
      mutate()
      await this.persistSchedules()
    })
    return this.schedulesQueue
  }

  private enqueueState(mutate: () => void): Promise<void> {
    this.stateQueue = this.stateQueue.then(async () => {
      mutate()
      await this.persistState()
    })
    return this.stateQueue
  }

  listSchedules(): Schedule[] {
    return this.schedules.map((s) => ({ ...s }))
  }

  getSchedule(id: string): Schedule | undefined {
    const s = this.schedules.find((x) => x.id === id)
    return s ? { ...s } : undefined
  }

  upsertSchedule(sched: Schedule): Promise<void> {
    return this.enqueueSchedules(() => {
      const idx = this.schedules.findIndex((s) => s.id === sched.id)
      if (idx >= 0) this.schedules[idx] = sched
      else this.schedules.push(sched)
    }).then(() =>
      this.enqueueState(() => {
        if (!this.state[sched.id]) {
          this.state[sched.id] = { lastFiredAt: null }
        }
      })
    )
  }

  removeSchedule(id: string): Promise<void> {
    return this.enqueueSchedules(() => {
      this.schedules = this.schedules.filter((s) => s.id !== id)
    }).then(() =>
      this.enqueueState(() => {
        delete this.state[id]
      })
    )
  }

  getLastFired(scheduleId: string): string | null {
    return this.state[scheduleId]?.lastFiredAt ?? null
  }

  setLastFired(scheduleId: string, iso: string): Promise<void> {
    return this.enqueueState(() => {
      this.state[scheduleId] = { lastFiredAt: iso }
    })
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

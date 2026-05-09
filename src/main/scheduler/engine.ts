import { spawn } from 'child_process'
import { promises as fs } from 'fs'
import { join } from 'path'
import { Cron } from 'croner'
import type { ScheduleStore } from './schedule-store'
import type { RunsStore } from './runs-store'
import type {
  Agent,
  CrontabStatus,
  JobRun,
  MissedRun,
  Schedule,
  ScheduleSpec
} from './types'
import { compileToCron } from './spec'
import {
  syncCrontab,
  readCrontab,
  extractManaged,
  type CrontabEntry,
  type SyncResult
} from './crontab'
import { detectMissed, missedEqual } from './missed'
import { scanAgents, ensureAgentsDir } from '../agents/scanner'

const SWEEP_INTERVAL_MS = 5 * 60 * 1000

export type EngineOpts = {
  schedules: ScheduleStore
  runs: RunsStore
  dataDir: string
  agentsDir: string
  resourcesDir: string
  onChange?: () => void
}

const newId = (): string =>
  `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`

export class SchedulerEngine {
  private agents: Agent[] = []
  private missed: MissedRun[] = []
  private sweepTimer: NodeJS.Timeout | null = null
  private wrapperOk = false
  private pythonOk = false
  private crontabOk = false
  private crontabConflict = false
  private crontabError: string | null = null

  constructor(private opts: EngineOpts) {}

  get wrapperPath(): string {
    return join(this.opts.dataDir, 'wrapper.sh')
  }

  async start(): Promise<void> {
    await Promise.all([this.opts.schedules.load(), this.opts.runs.load()])
    await this.installWrapper()
    await ensureAgentsDir(this.opts.agentsDir, join(this.opts.resourcesDir, 'agents'))
    await this.refreshAgents()
    this.pythonOk = await detectBin('python3')
    this.crontabOk = await detectBin('crontab')
    if (!this.pythonOk) {
      console.warn('[engine] python3 not found on PATH — wrapper cannot record runs')
    }
    if (!this.crontabOk) {
      console.warn('[engine] crontab not found on PATH — schedules will not be installed')
    }
    await this.runSync()
    this.opts.runs.startWatching(() => this.opts.onChange?.())
    await this.runSweep()
    this.sweepTimer = setInterval(() => {
      void this.runSweep()
    }, SWEEP_INTERVAL_MS)
  }

  stop(): void {
    if (this.sweepTimer) {
      clearInterval(this.sweepTimer)
      this.sweepTimer = null
    }
    this.opts.runs.stopWatching()
  }

  async refreshAgents(): Promise<Agent[]> {
    this.agents = await scanAgents(this.opts.agentsDir)
    return this.agents.map((a) => ({ ...a }))
  }

  listAgents(): Agent[] {
    return this.agents.map((a) => ({ ...a }))
  }

  listSchedules(): Schedule[] {
    const known = new Set(this.agents.map((a) => a.id))
    return this.opts.schedules.listSchedules().map((s) => ({
      ...s,
      orphaned: !known.has(s.jobId)
    }))
  }

  async upsertSchedule(sched: Schedule): Promise<void> {
    const { orphaned: _orphaned, ...clean } = sched
    void _orphaned
    await this.opts.schedules.upsertSchedule(clean as Schedule)
    await this.runSync()
    this.opts.onChange?.()
  }

  async removeSchedule(id: string): Promise<void> {
    await this.opts.schedules.removeSchedule(id)
    await this.runSync()
    this.opts.onChange?.()
  }

  nextRunFor(spec: ScheduleSpec, now = new Date()): Date | null {
    return new Cron(compileToCron(spec)).nextRun(now)
  }

  listRuns(jobId?: string): JobRun[] {
    return this.opts.runs.list(jobId)
  }

  readOutput(runId: string): Promise<string | null> {
    return this.opts.runs.readOutput(runId)
  }

  listMissed(): MissedRun[] {
    return this.missed.map((m) => ({ ...m }))
  }

  async runManually(jobId: string): Promise<JobRun> {
    const stub: JobRun = {
      id: newId(),
      jobId,
      scheduleId: null,
      trigger: 'manual',
      startedAt: new Date().toISOString(),
      endedAt: null,
      status: 'running',
      output: '',
      error: null,
      exitCode: null,
      outputPath: null
    }

    const agent = this.agents.find((a) => a.id === jobId)
    if (!agent || !agent.scriptPath) {
      stub.status = 'error'
      stub.error = `no agent script for "${jobId}"`
      stub.endedAt = new Date().toISOString()
      return stub
    }
    if (!this.wrapperOk) {
      stub.status = 'error'
      stub.error = 'wrapper.sh not installed'
      stub.endedAt = new Date().toISOString()
      return stub
    }
    if (!this.pythonOk) {
      stub.status = 'error'
      stub.error = 'python3 not found on PATH'
      stub.endedAt = new Date().toISOString()
      return stub
    }

    const cp = spawn(
      this.wrapperPath,
      [this.opts.dataDir, '', jobId, agent.scriptPath, stub.id],
      {
        detached: true,
        stdio: 'ignore',
        env: { ...process.env, AGENTIC_OS_TRIGGER: 'manual' }
      }
    )
    cp.unref()
    return stub
  }

  async crontabStatus(): Promise<CrontabStatus> {
    if (!this.crontabOk) {
      return {
        managed: false,
        hasMarkers: false,
        conflict: false,
        wrapperOk: this.wrapperOk,
        pythonOk: this.pythonOk,
        crontabOk: false,
        error: this.crontabError ?? 'crontab not found on PATH'
      }
    }
    try {
      const txt = await readCrontab()
      const ex = extractManaged(txt)
      return {
        managed: ex.hasMarkers && ex.managed.length > 0,
        hasMarkers: ex.hasMarkers,
        conflict: ex.conflict || this.crontabConflict,
        wrapperOk: this.wrapperOk,
        pythonOk: this.pythonOk,
        crontabOk: this.crontabOk,
        error: this.crontabError
      }
    } catch (err) {
      return {
        managed: false,
        hasMarkers: false,
        conflict: false,
        wrapperOk: this.wrapperOk,
        pythonOk: this.pythonOk,
        crontabOk: this.crontabOk,
        error: (err as Error).message
      }
    }
  }

  async reconcileCrontab(): Promise<SyncResult> {
    const result = await this.runSync({ force: true })
    this.crontabConflict = result.conflict
    this.opts.onChange?.()
    return result
  }

  private async installWrapper(): Promise<void> {
    const src = join(this.opts.resourcesDir, 'wrapper.sh')
    try {
      const data = await fs.readFile(src)
      await fs.mkdir(this.opts.dataDir, { recursive: true })
      await fs.writeFile(this.wrapperPath, data)
      await fs.chmod(this.wrapperPath, 0o755)
      this.wrapperOk = true
    } catch (err) {
      console.error('[engine] failed to install wrapper:', err)
      this.wrapperOk = false
    }
  }

  private async runSync(opts: { force?: boolean } = {}): Promise<SyncResult> {
    if (!this.wrapperOk) {
      this.crontabError = 'wrapper.sh not installed'
      return { wrote: false, conflict: false }
    }
    if (!this.pythonOk) {
      this.crontabError = 'python3 missing — install python3 to enable scheduled runs'
      return { wrote: false, conflict: false }
    }
    if (!this.crontabOk) {
      this.crontabError = 'crontab not found — install cron (e.g. `cronie` on Arch) to enable scheduling'
      return { wrote: false, conflict: false }
    }

    const entries: CrontabEntry[] = []
    for (const s of this.listSchedules()) {
      if (s.orphaned) continue
      const agent = this.agents.find((a) => a.id === s.jobId)
      if (!agent?.scriptPath) continue
      entries.push({ schedule: s, scriptPath: agent.scriptPath })
    }

    try {
      const result = await syncCrontab({
        entries,
        wrapperPath: this.wrapperPath,
        dataDir: this.opts.dataDir,
        force: opts.force
      })
      this.crontabConflict = result.conflict
      this.crontabError = result.conflict
        ? result.reason ?? 'managed section damaged'
        : null
      return result
    } catch (err) {
      this.crontabError = (err as Error).message
      console.error('[engine] crontab sync failed:', err)
      return { wrote: false, conflict: false }
    }
  }

  async runSweep(): Promise<void> {
    const schedules = this.listSchedules()
    const runs = this.opts.runs.list(undefined, 1000)
    const next = detectMissed(schedules, runs)
    if (!missedEqual(this.missed, next)) {
      this.missed = next
      this.opts.onChange?.()
    }
  }
}

async function detectBin(bin: string): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    const cp = spawn(bin, ['--version'], { stdio: 'ignore' })
    cp.on('error', () => resolve(false))
    cp.on('close', (code) => resolve(code === 0 || code === 1))
  })
}

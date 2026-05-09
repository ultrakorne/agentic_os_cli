import { spawn } from 'child_process'
import { promises as fs } from 'fs'
import { join } from 'path'
import { Cron } from 'croner'
import type { AgentConfigStore } from './agent-config-store'
import type { RunsStore } from './runs-store'
import type {
  Agent,
  AgentConfig,
  CrontabStatus,
  JobRun,
  MissedRun,
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
import { scanScripts, ensureAgentsDir, type ScriptInfo } from '../agents/scanner'

const SWEEP_INTERVAL_MS = 5 * 60 * 1000

export type EngineOpts = {
  configs: AgentConfigStore
  runs: RunsStore
  dataDir: string
  agentsDir: string
  resourcesDir: string
  tickCommand?: string | null
  onChange?: () => void
}

const newId = (): string =>
  `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`

export class SchedulerEngine {
  private scripts: ScriptInfo[] = []
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
    await Promise.all([this.opts.configs.load(), this.opts.runs.load()])
    await this.installWrapper()
    await ensureAgentsDir(this.opts.agentsDir, join(this.opts.resourcesDir, 'agents'))
    await this.refreshScripts()
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

  async refreshScripts(): Promise<Agent[]> {
    this.scripts = await scanScripts(this.opts.agentsDir)
    return this.listAgents()
  }

  listAgents(): Agent[] {
    const byId = new Map<string, Agent>()

    for (const cfg of this.opts.configs.list()) {
      byId.set(cfg.id, this.makeAgentFromConfig(cfg))
    }
    for (const script of this.scripts) {
      const existing = byId.get(script.id)
      if (existing) {
        existing.scriptPath = script.scriptPath
        existing.section = script.section
        existing.orphaned = false
      } else {
        byId.set(script.id, {
          id: script.id,
          title: humanize(script.id),
          description: '',
          section: script.section,
          scriptPath: script.scriptPath,
          schedule: undefined,
          scheduled: false,
          orphaned: false
        })
      }
    }

    return [...byId.values()].sort((a, b) => a.id.localeCompare(b.id))
  }

  private makeAgentFromConfig(cfg: AgentConfig): Agent {
    return {
      id: cfg.id,
      title: cfg.title ?? humanize(cfg.id),
      description: cfg.description ?? '',
      section: 'Agents', // overridden when a matching script is merged in
      scriptPath: undefined,
      schedule: cfg.schedule,
      scheduled: !!cfg.schedule,
      orphaned: !!cfg.schedule
    }
  }

  async setSchedule(agentId: string, schedule: ScheduleSpec | null): Promise<void> {
    await this.opts.configs.setSchedule(agentId, schedule)
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

  async runManually(agentId: string): Promise<JobRun> {
    const stub: JobRun = {
      id: newId(),
      jobId: agentId,
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

    const script = this.scripts.find((s) => s.id === agentId)
    if (!script) {
      stub.status = 'error'
      stub.error = `no script found for agent "${agentId}"`
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
      [this.opts.dataDir, '', agentId, script.scriptPath, stub.id],
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
      let existing: Buffer | null = null
      try {
        existing = await fs.readFile(this.wrapperPath)
      } catch {
        /* missing — fall through and install */
      }
      if (!existing || !existing.equals(data)) {
        await fs.writeFile(this.wrapperPath, data)
        await fs.chmod(this.wrapperPath, 0o755)
      }
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
    for (const agent of this.listAgents()) {
      if (!agent.schedule || !agent.scriptPath) continue
      entries.push({
        agentId: agent.id,
        scriptPath: agent.scriptPath,
        spec: agent.schedule
      })
    }

    try {
      const result = await syncCrontab({
        entries,
        wrapperPath: this.wrapperPath,
        dataDir: this.opts.dataDir,
        tickCommand: this.opts.tickCommand,
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
    const agents = this.listAgents()
    const runs = this.opts.runs.list(undefined, 1000)
    const next = detectMissed(agents, runs)
    if (!missedEqual(this.missed, next)) {
      this.missed = next
      this.opts.onChange?.()
    }
  }
}

function humanize(id: string): string {
  return id
    .replace(/[-_]+/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .trim()
}

async function detectBin(bin: string): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    const cp = spawn(bin, ['--version'], { stdio: 'ignore' })
    cp.on('error', () => resolve(false))
    cp.on('close', (code) => resolve(code === 0 || code === 1))
  })
}

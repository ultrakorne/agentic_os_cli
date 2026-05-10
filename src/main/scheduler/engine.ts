import { spawn } from 'child_process'
import { promises as fs } from 'fs'
import { join } from 'path'
import { Cron } from 'croner'
import type { AgentMetaStore } from './agent-meta-store'
import type { RunsStore } from './runs-store'
import type {
  Agent,
  AgentScanIssue,
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
import {
  scanScripts,
  findScanIssues,
  ensureAgentsDir,
  type AgentEntry
} from '../agents/scanner'

const SWEEP_INTERVAL_MS = 5 * 60 * 1000

export type EngineOpts = {
  meta: AgentMetaStore
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
  private entries: AgentEntry[] = []
  private scanIssues: AgentScanIssue[] = []
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
    await this.opts.runs.load()
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
    this.opts.runs.startWatching(() => {
      void this.handleRunsChanged()
    })
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
    const [entries, issues] = await Promise.all([
      scanScripts(this.opts.agentsDir),
      findScanIssues(this.opts.agentsDir)
    ])
    this.entries = entries
    this.scanIssues = issues
    return this.listAgents()
  }

  listAgents(): Agent[] {
    return this.entries
      .map((e) => entryToAgent(e))
      .sort((a, b) => a.id.localeCompare(b.id))
  }

  listScanIssues(): AgentScanIssue[] {
    return this.scanIssues.map((i) => ({ ...i }))
  }

  async setSchedule(agentId: string, schedule: ScheduleSpec | null): Promise<void> {
    const entry = this.entries.find((e) => e.id === agentId)
    if (!entry) throw new Error(`unknown agent "${agentId}"`)
    entry.meta = await this.opts.meta.setSchedule(entry.metaPath, schedule)
    await this.runSync()
    // Recompute missed runs immediately so the dashboard reflects the new
    // schedule without waiting for the next 5-minute sweep. We suppress the
    // sweep's own notify so the renderer sees a single coalesced update.
    await this.runSweep({ notify: false })
    this.opts.onChange?.()
  }

  async setDescription(agentId: string, description: string): Promise<void> {
    const entry = this.entries.find((e) => e.id === agentId)
    if (!entry) throw new Error(`unknown agent "${agentId}"`)
    entry.meta = await this.opts.meta.setDescription(entry.metaPath, description)
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

    const entry = this.entries.find((e) => e.id === agentId)
    if (!entry) {
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
      [this.opts.dataDir, '', agentId, entry.scriptPath, stub.id],
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
    const daemonOk = await detectCronDaemon()
    if (!this.crontabOk) {
      return {
        managed: false,
        hasMarkers: false,
        conflict: false,
        wrapperOk: this.wrapperOk,
        pythonOk: this.pythonOk,
        crontabOk: false,
        daemonOk,
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
        daemonOk,
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
        daemonOk,
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
    for (const e of this.entries) {
      if (!e.meta.schedule) continue
      entries.push({
        agentId: e.id,
        scriptPath: e.scriptPath,
        spec: e.meta.schedule
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

  private async handleRunsChanged(): Promise<void> {
    // Refresh missed runs first so the dashboard sees a single coalesced
    // update — clicking "run now" should clear any prior missed slot for
    // this agent without waiting for the next 5-minute sweep.
    await this.runSweep({ notify: false })
    this.opts.onChange?.()
  }

  async runSweep({ notify = true }: { notify?: boolean } = {}): Promise<void> {
    const agents = this.listAgents()
    const runs = this.opts.runs.list(undefined, 1000)
    const next = detectMissed(agents, runs)
    if (!missedEqual(this.missed, next)) {
      this.missed = next
      if (notify) this.opts.onChange?.()
    }
  }
}

function entryToAgent(e: AgentEntry): Agent {
  return {
    id: e.id,
    title: e.meta.title ?? humanize(e.id),
    description: e.meta.description ?? '',
    section: e.section,
    scriptPath: e.scriptPath,
    schedule: e.meta.schedule,
    scheduledAt: e.meta.scheduledAt,
    scheduled: !!e.meta.schedule
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

// `null` means we can't determine — don't false-alarm the UI.
async function detectCronDaemon(): Promise<boolean | null> {
  if (process.platform === 'win32') return null
  // cronie ships /usr/bin/crond; vixie-cron and macOS use `cron`; cover both.
  const names = ['crond', 'cron', 'cronie']
  let pgrepWorked = false
  for (const name of names) {
    const r = await runPgrep(name)
    if (r === 'match') return true
    if (r === 'nomatch') pgrepWorked = true
  }
  return pgrepWorked ? false : null
}

function runPgrep(name: string): Promise<'match' | 'nomatch' | 'unavailable'> {
  return new Promise((resolve) => {
    const cp = spawn('pgrep', ['-x', name], { stdio: 'ignore' })
    cp.on('error', () => resolve('unavailable'))
    cp.on('close', (code) => {
      // pgrep: 0 = match, 1 = no match, 2/3 = error.
      if (code === 0) resolve('match')
      else if (code === 1) resolve('nomatch')
      else resolve('unavailable')
    })
  })
}

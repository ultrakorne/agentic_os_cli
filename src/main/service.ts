import { spawn } from 'child_process'
import { promises as fs } from 'fs'
import { join } from 'path'
import { Cron } from 'croner'
import type {
  Agent,
  AgentScanIssue,
  JobRun,
  MissedRun,
  RefreshSummary,
  ScheduleSpec,
  SystemStatus
} from '../shared/scheduler'
import { AgentMetaStore } from './scheduler/agent-meta-store'
import { RunsStore } from './scheduler/runs-store'
import { MissesStore } from './scheduler/misses-store'
import { compileToCron } from './scheduler/spec'
import {
  scanScripts,
  findScanIssues,
  type AgentEntry
} from './agents/scanner'

// One-shot wait for a child process's stdout/stderr to drain and the exit code
// to be available. Returns ({code, stdout, stderr}); never throws.
function execCapture(
  bin: string,
  args: string[]
): Promise<{ code: number; stdout: string; stderr: string }> {
  return new Promise((resolve) => {
    const cp = spawn(bin, args, { stdio: ['ignore', 'pipe', 'pipe'] })
    let stdout = ''
    let stderr = ''
    cp.stdout.on('data', (chunk: Buffer) => {
      stdout += chunk.toString('utf8')
    })
    cp.stderr.on('data', (chunk: Buffer) => {
      stderr += chunk.toString('utf8')
    })
    cp.on('error', (err) => {
      resolve({ code: -1, stdout, stderr: stderr || err.message })
    })
    cp.on('close', (code) => {
      resolve({ code: code ?? -1, stdout, stderr })
    })
  })
}

// Parses the single-line summary printed by `aos refresh`, of the form
//   aos refresh agents=N scheduled=N issues=N cron=X wrapper=X python3=X daemon=X log=X
// Returns null if the line doesn't match the expected shape.
export function parseRefreshSummary(line: string): RefreshSummary | null {
  const trimmed = line.trim()
  if (!trimmed.startsWith('aos refresh ')) return null
  const fields: Record<string, string> = {}
  for (const part of trimmed.slice('aos refresh '.length).split(/\s+/)) {
    const eq = part.indexOf('=')
    if (eq < 0) continue
    fields[part.slice(0, eq)] = part.slice(eq + 1)
  }
  const num = (k: string): number => {
    const v = parseInt(fields[k] ?? '', 10)
    return Number.isFinite(v) ? v : 0
  }
  return {
    agents: num('agents'),
    scheduled: num('scheduled'),
    issues: num('issues'),
    cron: fields.cron ?? 'unknown',
    wrapper: fields.wrapper ?? 'unknown',
    python3: fields.python3 ?? 'unknown',
    daemon: fields.daemon ?? 'unknown',
    log: fields.log ?? 'unknown'
  }
}

const newId = (): string =>
  `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`

export type AppServiceOpts = {
  aosBin: string
  aosHome: string
  meta: AgentMetaStore
  runs: RunsStore
  misses: MissesStore
  onChange?: () => void
}

// Thin service layer for the renderer. Reads agents/runs/misses from the
// filesystem, writes meta sidecars, and delegates anything that mutates cron
// (or anything else system-wide) to the `aos` CLI by spawning it. The CLI
// owns the misses/ directory — the dashboard never re-derives it.
export class AppService {
  private entries: AgentEntry[] = []
  private scanIssues: AgentScanIssue[] = []
  private lastRefresh: RefreshSummary | null = null
  private lastRefreshError: string | null = null

  constructor(private opts: AppServiceOpts) {}

  get aosHome(): string {
    return this.opts.aosHome
  }

  get wrapperPath(): string {
    return join(this.opts.aosHome, 'wrapper.sh')
  }

  async start(): Promise<void> {
    await Promise.all([this.opts.runs.load(), this.opts.misses.load()])
    await this.refreshScripts()
    this.opts.runs.startWatching(() => {
      this.opts.onChange?.()
    })
    this.opts.misses.startWatching(() => {
      this.opts.onChange?.()
    })
  }

  stop(): void {
    this.opts.runs.stopWatching()
    this.opts.misses.stopWatching()
  }

  async refreshScripts(): Promise<Agent[]> {
    const agentsDir = join(this.opts.aosHome, 'agents')
    const [entries, issues] = await Promise.all([
      scanScripts(agentsDir),
      findScanIssues(agentsDir)
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

  listMissed(): MissedRun[] {
    return this.opts.misses.list()
  }

  listRuns(jobId?: string): JobRun[] {
    return this.opts.runs.list(jobId)
  }

  readOutput(runId: string): Promise<string | null> {
    return this.opts.runs.readOutput(runId)
  }

  nextRunFor(spec: ScheduleSpec, now = new Date()): Date | null {
    return new Cron(compileToCron(spec)).nextRun(now)
  }

  status(): SystemStatus {
    return {
      cliMissing: false,
      aosBin: this.opts.aosBin,
      aosHome: this.opts.aosHome,
      lastRefresh: this.lastRefresh,
      lastRefreshError: this.lastRefreshError
    }
  }

  // Re-scan agents from disk AND ask the CLI to reconcile cron. The CLI is
  // the source of truth for everything in the user's crontab; the renderer
  // only watches the resulting summary line so it can flag missing wrapper /
  // python3 / cron daemon.
  async refresh(): Promise<RefreshSummary | null> {
    await this.refreshScripts()
    const res = await execCapture(this.opts.aosBin, ['refresh'])
    const line = lastNonEmptyLine(res.stdout)
    if (res.code === 0 && line) {
      const parsed = parseRefreshSummary(line)
      if (parsed) {
        this.lastRefresh = parsed
        this.lastRefreshError = null
        this.opts.onChange?.()
        return parsed
      }
      this.lastRefreshError = `unexpected aos refresh output: ${line}`
    } else {
      this.lastRefreshError =
        res.stderr.trim() || `aos refresh exited ${res.code}`
    }
    this.opts.onChange?.()
    return this.lastRefresh
  }

  async setSchedule(agentId: string, schedule: ScheduleSpec | null): Promise<void> {
    const entry = this.entries.find((e) => e.id === agentId)
    if (!entry) throw new Error(`unknown agent "${agentId}"`)
    entry.meta = await this.opts.meta.setSchedule(entry.metaPath, schedule)
    // Schedule changed → cron needs to be reconciled and misses recomputed.
    // Fire-and-forget; aos refresh rewrites <aos_home>/misses, which the
    // MissesStore picks up via fs.watch.
    void this.refresh()
    this.opts.onChange?.()
  }

  async setDescription(agentId: string, description: string): Promise<void> {
    const entry = this.entries.find((e) => e.id === agentId)
    if (!entry) throw new Error(`unknown agent "${agentId}"`)
    entry.meta = await this.opts.meta.setDescription(entry.metaPath, description)
    this.opts.onChange?.()
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
    try {
      await fs.access(this.wrapperPath)
    } catch {
      stub.status = 'error'
      stub.error = `${this.wrapperPath} missing — run \`aos init <path>\``
      stub.endedAt = new Date().toISOString()
      return stub
    }

    const cp = spawn(
      this.wrapperPath,
      [this.opts.aosHome, '', agentId, entry.scriptPath, stub.id],
      {
        detached: true,
        stdio: 'ignore',
        env: { ...process.env, AGENTIC_OS_TRIGGER: 'manual' }
      }
    )
    cp.unref()
    return stub
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

function lastNonEmptyLine(text: string): string | null {
  const lines = text.split('\n')
  for (let i = lines.length - 1; i >= 0; i--) {
    const l = lines[i].trim()
    if (l) return l
  }
  return null
}

import { spawn } from 'child_process'
import { Cron } from 'croner'
import type {
  Agent,
  JobRun,
  MissedRun,
  RefreshSummary,
  ScheduleSpec,
  SystemStatus
} from '../shared/scheduler'
import { RunsStore } from './scheduler/runs-store'
import { MissesStore } from './scheduler/misses-store'
import { compileToCron } from './scheduler/spec'
import { parseAgentList, scheduleToArgs } from './agents/agent-list'

// One-shot wait for a child process's stdout/stderr to drain and the exit code
// to be available. Returns ({code, stdout, stderr}); never throws.
export function execCapture(
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

export type AppServiceOpts = {
  aosBin: string
  aosHome: string
  runs: RunsStore
  misses: MissesStore
  onChange?: () => void
}

// Thin service layer for the renderer. Holds an in-memory cache of the
// agents list (re-fetched from `aos list --json` whenever something
// changes) and proxies every sidecar write through the `aos` CLI. This is
// the "view" half of the system; the CLI owns the agents tree, the meta
// sidecars, the managed crontab block, and the misses dir.
export class AppService {
  private agents: Agent[] = []
  private lastRefresh: RefreshSummary | null = null
  private lastRefreshError: string | null = null

  constructor(private opts: AppServiceOpts) {}

  get aosHome(): string {
    return this.opts.aosHome
  }

  async start(): Promise<void> {
    await Promise.all([this.opts.runs.load(), this.opts.misses.load()])
    await this.refreshAgents()
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

  // Re-fetch agents from the CLI. The CLI scanner is the single source of
  // truth — section detection, executable check, sidecar fold-in, and the
  // not-executable warning all live in `aos list --json`.
  private async refreshAgents(): Promise<void> {
    const res = await execCapture(this.opts.aosBin, ['list', '--json'])
    if (res.code !== 0) {
      this.lastRefreshError = res.stderr.trim() || `aos list exited ${res.code}`
      return
    }
    try {
      this.agents = parseAgentList(res.stdout)
    } catch (err) {
      this.lastRefreshError = `aos list parse: ${(err as Error).message}`
    }
  }

  listAgents(): Agent[] {
    return this.agents.map((a) => ({ ...a, warnings: [...a.warnings] }))
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

  // Ask the CLI to rescan and reconcile cron, then re-pull the agent list.
  async refresh(): Promise<RefreshSummary | null> {
    const res = await execCapture(this.opts.aosBin, ['refresh', '--json'])
    if (res.code === 0 && res.stdout.trim()) {
      try {
        this.lastRefresh = JSON.parse(res.stdout) as RefreshSummary
        this.lastRefreshError = null
      } catch {
        this.lastRefreshError = `unexpected aos refresh output: ${res.stdout.trim().slice(0, 200)}`
      }
    } else {
      this.lastRefreshError = res.stderr.trim() || `aos refresh exited ${res.code}`
    }
    await this.refreshAgents()
    this.opts.onChange?.()
    return this.lastRefresh
  }

  async setSchedule(agentId: string, schedule: ScheduleSpec | null): Promise<void> {
    const args = ['schedule', agentId, '--json', ...scheduleToArgs(schedule)]
    const res = await execCapture(this.opts.aosBin, args)
    if (res.code !== 0) {
      throw new Error(res.stderr.trim() || `aos schedule exited ${res.code}`)
    }
    // The CLI auto-refreshes cron and embeds the resulting summary, so we
    // pluck it here instead of spawning another `aos refresh`. JSON parse
    // failure isn't fatal — the write itself already succeeded.
    try {
      const parsed = JSON.parse(res.stdout) as {
        refresh?: RefreshSummary | { error: string }
      }
      if (parsed.refresh && 'error' in parsed.refresh) {
        this.lastRefreshError = parsed.refresh.error
      } else if (parsed.refresh) {
        this.lastRefresh = parsed.refresh
        this.lastRefreshError = null
      }
    } catch {
      /* keep prior summary */
    }
    await this.refreshAgents()
    this.opts.onChange?.()
  }

  async setDescription(agentId: string, description: string): Promise<void> {
    const res = await execCapture(this.opts.aosBin, ['describe', agentId, description, '--json'])
    if (res.code !== 0) {
      throw new Error(res.stderr.trim() || `aos describe exited ${res.code}`)
    }
    await this.refreshAgents()
    this.opts.onChange?.()
  }

  // Manual runs are owned by `aos run`: it picks the run id, spawns the
  // wrapper detached, and prints the JobRun stub. Keeping the cron/manual
  // invocation forms in one place stops them from drifting when the wrapper
  // grows new argv or env.
  async runManually(agentId: string): Promise<JobRun> {
    const res = await execCapture(this.opts.aosBin, ['run', agentId, '--json'])
    if (res.code !== 0) {
      throw new Error(res.stderr.trim() || `aos run exited ${res.code}`)
    }
    return JSON.parse(res.stdout) as JobRun
  }
}

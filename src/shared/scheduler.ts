export type Weekday = 'mon' | 'tue' | 'wed' | 'thu' | 'fri' | 'sat' | 'sun'

export type ScheduleSpec =
  | { kind: 'hourly'; everyHours: number; minute: number }
  | { kind: 'daily'; days: Weekday[]; hour: number; minute: number }

export type AgentMeta = {
  schedule?: ScheduleSpec
  scheduledAt?: string
  title?: string
  description?: string
}

export type JobRunTrigger = 'schedule' | 'manual' | 'catch-up'
// 'missed' is written by `aos tick` / `aos refresh` when a scheduled slot
// fires while the wrapper isn't running. startedAt holds the expected slot;
// endedAt, exitCode, outputPath, error are all null. Only one missed record
// per agent exists at a time — newer misses replace older ones so the
// dashboard's "behind" banner is one row per agent.
export type JobRunStatus = 'running' | 'success' | 'error' | 'missed'

export type JobRun = {
  id: string
  jobId: string
  scheduleId: string | null
  trigger: JobRunTrigger
  startedAt: string
  endedAt: string | null
  status: JobRunStatus
  output: string
  error: string | null
  exitCode: number | null
  outputPath: string | null
}

export type Agent = {
  id: string
  title: string
  description: string
  section: string
  scriptPath: string
  schedule?: ScheduleSpec
  scheduledAt?: string
  scheduled: boolean
  // Per-agent problems surfaced by the CLI scanner (e.g. "not-executable").
  // Empty when the script is fine to run. The dashboard renders these on the
  // card and the cron block omits warned agents on the next refresh.
  warnings: string[]
}

// One-line summary of an `aos refresh` invocation, with each key=value field
// parsed back into its own property. Strings are kept as raw CLI output (e.g.
// cron='skipped:no-crontab-bin', daemon='down') so the renderer can branch on
// the exact cause without re-deriving it.
export type RefreshSummary = {
  agents: number
  scheduled: number
  issues: number
  cron: string
  wrapper: string
  python3: string
  daemon: string
  log: string
  runs: string
}

// Runtime view of the CLI: if cliMissing the renderer must block all
// agent-management UI and point the user at the install instructions.
export type SystemStatus = {
  cliMissing: boolean
  aosBin: string | null
  aosHome: string | null
  lastRefresh: RefreshSummary | null
  lastRefreshError: string | null
}

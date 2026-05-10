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
export type JobRunStatus = 'running' | 'success' | 'error'

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
}

export type MissedRun = {
  agentId: string
  expectedAt: string
}

export type CrontabStatus = {
  managed: boolean
  hasMarkers: boolean
  conflict: boolean
  wrapperOk: boolean
  pythonOk: boolean
  crontabOk: boolean
  // null when we can't determine (e.g. pgrep unavailable, Windows).
  daemonOk: boolean | null
  error: string | null
}

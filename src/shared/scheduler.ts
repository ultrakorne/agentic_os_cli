export type Weekday = 'mon' | 'tue' | 'wed' | 'thu' | 'fri' | 'sat' | 'sun'

export type ScheduleSpec =
  | { kind: 'hourly'; everyHours: number; minute: number }
  | { kind: 'daily'; days: Weekday[]; hour: number; minute: number }

export type Schedule = {
  id: string
  jobId: string
  spec: ScheduleSpec
  orphaned?: boolean
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
  scriptPath?: string
}

export type MissedRun = {
  scheduleId: string
  jobId: string
  expectedAt: string
}

export type CrontabStatus = {
  managed: boolean
  hasMarkers: boolean
  conflict: boolean
  wrapperOk: boolean
  pythonOk: boolean
  crontabOk: boolean
  error: string | null
}

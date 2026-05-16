import type { Agent, ScheduleSpec } from '../../shared/scheduler'

// Shape `aos list --json` actually emits. Optional fields are omitted
// (not null) when unset, so every property is `?:`.
export type RawAgent = {
  id: string
  section: string
  scriptPath: string
  schedule?: ScheduleSpec
  scheduledAt?: string
  cron?: string
  title?: string
  description?: string
  warnings?: string[]
}

type ListPayload = {
  agents?: RawAgent[]
  issues?: unknown[]
}

// parseAgentList converts the stdout of `aos list --json` into the
// renderer's Agent[] shape. Throws on malformed JSON or missing top-level
// shape so the caller can surface a clear error instead of silently
// rendering an empty dashboard.
export function parseAgentList(stdout: string): Agent[] {
  const parsed = JSON.parse(stdout) as ListPayload
  if (!parsed || typeof parsed !== 'object' || !Array.isArray(parsed.agents)) {
    throw new Error('aos list --json: missing "agents" array')
  }
  return parsed.agents.map(toAgent)
}

function toAgent(raw: RawAgent): Agent {
  return {
    id: raw.id,
    title: raw.title ?? humanize(raw.id),
    description: raw.description ?? '',
    section: raw.section,
    scriptPath: raw.scriptPath,
    schedule: raw.schedule,
    scheduledAt: raw.scheduledAt,
    scheduled: !!raw.schedule,
    warnings: raw.warnings ?? []
  }
}

// Title fallback when the sidecar's title is unset — same rule the previous
// in-process scanner used.
function humanize(id: string): string {
  return id
    .replace(/[-_]+/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .trim()
}

// scheduleToArgs converts the renderer's ScheduleSpec | null into the
// flag form `aos schedule` accepts. The CLI infers the kind from which
// flags are present, so we just emit the relevant ones (or --off).
export function scheduleToArgs(schedule: ScheduleSpec | null): string[] {
  if (schedule === null) return ['--off']
  switch (schedule.kind) {
    case 'hourly':
      return ['--every-hours', String(schedule.everyHours), '--minute', String(schedule.minute)]
    case 'daily':
      return [
        '--hour',
        String(schedule.hour),
        '--minute',
        String(schedule.minute),
        '--days',
        schedule.days.join(',')
      ]
  }
}

import { Cron } from 'croner'
import type { Agent, JobRun, MissedRun } from './types'
import { compileToCron } from './spec'

export type DetectOpts = {
  now?: Date
  windowMs?: number
  toleranceMs?: number
  maxTicksPerSchedule?: number
}

const DEFAULT_WINDOW_MS = 24 * 60 * 60 * 1000
const DEFAULT_TOLERANCE_MS = 90 * 1000
const DEFAULT_MAX_TICKS = 500

export function detectMissed(
  agents: Agent[],
  runs: JobRun[],
  opts: DetectOpts = {}
): MissedRun[] {
  const now = opts.now ?? new Date()
  const windowMs = opts.windowMs ?? DEFAULT_WINDOW_MS
  const toleranceMs = opts.toleranceMs ?? DEFAULT_TOLERANCE_MS
  const maxTicks = opts.maxTicksPerSchedule ?? DEFAULT_MAX_TICKS
  const cutoff = now.getTime() - windowMs

  const out: MissedRun[] = []

  for (const agent of agents) {
    if (!agent.schedule || !agent.scriptPath) continue

    let cron: Cron
    try {
      cron = new Cron(compileToCron(agent.schedule))
    } catch {
      continue
    }

    // Don't report misses for ticks that occurred before this schedule was set
    // (e.g. just after the user created or changed the schedule). Without this,
    // a brand-new schedule would retroactively flag every cron tick in the
    // last 24 h.
    const scheduledAtMs = agent.scheduledAt
      ? new Date(agent.scheduledAt).getTime()
      : null
    const earliestMs =
      scheduledAtMs != null ? Math.max(cutoff, scheduledAtMs) : cutoff

    const expected: Date[] = []
    let cursor = new Date(earliestMs - 1)
    let i = 0
    while (i < maxTicks) {
      const next: Date | null = cron.nextRun(cursor)
      if (!next || next.getTime() > now.getTime()) break
      expected.push(next)
      cursor = next
      i += 1
    }
    if (expected.length === 0) continue

    const actuals = runs
      .filter((r) => r.jobId === agent.id)
      .map((r) => new Date(r.startedAt).getTime())
      .sort((a, b) => a - b)

    // Any run started at-or-after an expected tick (within tolerance) "covers"
    // it — including a manual run made now after a missed scheduled slot.
    // Once the user has run the agent, prior gaps are considered acknowledged.
    const latestRunMs = actuals.length > 0 ? actuals[actuals.length - 1] : null

    for (const exp of expected) {
      const expMs = exp.getTime()
      if (now.getTime() - expMs < toleranceMs) continue
      if (latestRunMs !== null && latestRunMs >= expMs - toleranceMs) continue
      out.push({ agentId: agent.id, expectedAt: exp.toISOString() })
    }
  }

  out.sort((a, b) => b.expectedAt.localeCompare(a.expectedAt))
  return out
}

export function missedEqual(a: MissedRun[], b: MissedRun[]): boolean {
  if (a.length !== b.length) return false
  const key = (m: MissedRun): string => `${m.agentId}|${m.expectedAt}`
  const seen = new Set(a.map(key))
  for (const m of b) if (!seen.has(key(m))) return false
  return true
}

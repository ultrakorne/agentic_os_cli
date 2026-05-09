import { Cron } from 'croner'
import type { JobRun, MissedRun, Schedule } from './types'
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
  schedules: Schedule[],
  runs: JobRun[],
  opts: DetectOpts = {}
): MissedRun[] {
  const now = opts.now ?? new Date()
  const windowMs = opts.windowMs ?? DEFAULT_WINDOW_MS
  const toleranceMs = opts.toleranceMs ?? DEFAULT_TOLERANCE_MS
  const maxTicks = opts.maxTicksPerSchedule ?? DEFAULT_MAX_TICKS
  const cutoff = now.getTime() - windowMs

  const out: MissedRun[] = []

  for (const sched of schedules) {
    if (sched.orphaned) continue

    let cron: Cron
    try {
      cron = new Cron(compileToCron(sched.spec))
    } catch {
      continue
    }

    const expected: Date[] = []
    let cursor = new Date(cutoff - 1)
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
      .filter((r) => r.scheduleId === sched.id)
      .map((r) => new Date(r.startedAt).getTime())
      .sort((a, b) => a - b)

    for (const exp of expected) {
      const expMs = exp.getTime()
      if (now.getTime() - expMs < toleranceMs) continue
      const matched = anyWithin(actuals, expMs, toleranceMs)
      if (!matched) {
        out.push({
          scheduleId: sched.id,
          jobId: sched.jobId,
          expectedAt: exp.toISOString()
        })
      }
    }
  }

  out.sort((a, b) => b.expectedAt.localeCompare(a.expectedAt))
  return out
}

function anyWithin(sorted: number[], target: number, tolerance: number): boolean {
  if (sorted.length === 0) return false
  let lo = 0
  let hi = sorted.length - 1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    const v = sorted[mid]
    if (Math.abs(v - target) <= tolerance) return true
    if (v < target) lo = mid + 1
    else hi = mid - 1
  }
  return false
}

export function missedEqual(a: MissedRun[], b: MissedRun[]): boolean {
  if (a.length !== b.length) return false
  const key = (m: MissedRun): string => `${m.scheduleId}|${m.expectedAt}`
  const seen = new Set(a.map(key))
  for (const m of b) if (!seen.has(key(m))) return false
  return true
}

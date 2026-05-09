import { describe, it, expect } from 'vitest'
import { detectMissed, missedEqual } from './missed'
import type { JobRun, Schedule } from './types'

function run(scheduleId: string, jobId: string, startedAt: Date): JobRun {
  return {
    id: `r-${startedAt.getTime()}`,
    jobId,
    scheduleId,
    trigger: 'schedule',
    startedAt: startedAt.toISOString(),
    endedAt: new Date(startedAt.getTime() + 1000).toISOString(),
    status: 'success',
    output: '',
    error: null,
    exitCode: 0,
    outputPath: null
  }
}

describe('detectMissed', () => {
  const hourly = (id: string, jobId: string): Schedule => ({
    id,
    jobId,
    spec: { kind: 'hourly', everyHours: 1, minute: 0 }
  })

  it('returns empty when every expected tick has a matching run', () => {
    const now = new Date('2026-05-09T05:30:00Z')
    const sched = hourly('s', 'ping')
    const runs = [
      run('s', 'ping', new Date('2026-05-09T00:00:01Z')),
      run('s', 'ping', new Date('2026-05-09T01:00:00Z')),
      run('s', 'ping', new Date('2026-05-09T02:00:01Z')),
      run('s', 'ping', new Date('2026-05-09T03:00:01Z')),
      run('s', 'ping', new Date('2026-05-09T04:00:00Z')),
      run('s', 'ping', new Date('2026-05-09T05:00:00Z'))
    ]
    expect(detectMissed([sched], runs, { now, windowMs: 6 * 3600_000 })).toEqual([])
  })

  it('flags ticks with no nearby run', () => {
    const now = new Date('2026-05-09T05:30:00Z')
    const sched = hourly('s', 'ping')
    // Missing the 02:00 and 04:00 ticks
    const runs = [
      run('s', 'ping', new Date('2026-05-09T00:00:01Z')),
      run('s', 'ping', new Date('2026-05-09T01:00:00Z')),
      run('s', 'ping', new Date('2026-05-09T03:00:01Z')),
      run('s', 'ping', new Date('2026-05-09T05:00:00Z'))
    ]
    const missed = detectMissed([sched], runs, { now, windowMs: 6 * 3600_000 })
    const expectedSet = missed.map((m) => m.expectedAt).sort()
    expect(expectedSet).toContain('2026-05-09T02:00:00.000Z')
    expect(expectedSet).toContain('2026-05-09T04:00:00.000Z')
    expect(expectedSet).toHaveLength(2)
  })

  it('does not flag ticks within tolerance even if not exact', () => {
    const now = new Date('2026-05-09T05:30:00Z')
    const sched = hourly('s', 'ping')
    const runs = [
      // 30s late on 04:00
      run('s', 'ping', new Date('2026-05-09T04:00:30Z')),
      run('s', 'ping', new Date('2026-05-09T05:00:00Z'))
    ]
    const missed = detectMissed([sched], runs, {
      now,
      windowMs: 2 * 3600_000,
      toleranceMs: 90_000
    })
    expect(missed).toEqual([])
  })

  it('does not flag a tick less than tolerance ago (cron may not have fired yet)', () => {
    const now = new Date('2026-05-09T05:00:30Z') // 30s past 05:00 tick, within tolerance
    const sched = hourly('s', 'ping')
    const runs: JobRun[] = []
    const missed = detectMissed([sched], runs, {
      now,
      windowMs: 60 * 60_000,
      toleranceMs: 90_000
    })
    // 05:00 should be skipped (too recent), 04:00 may be flagged
    const expectedAts = missed.map((m) => m.expectedAt)
    expect(expectedAts).not.toContain('2026-05-09T05:00:00.000Z')
  })

  it('skips orphan schedules', () => {
    const now = new Date('2026-05-09T05:30:00Z')
    const sched: Schedule = { ...hourly('s', 'gone'), orphaned: true }
    const missed = detectMissed([sched], [], { now, windowMs: 3600_000 })
    expect(missed).toEqual([])
  })

  it('returns missed runs sorted newest first', () => {
    const now = new Date('2026-05-09T05:30:00Z')
    const sched = hourly('s', 'ping')
    const missed = detectMissed([sched], [], { now, windowMs: 4 * 3600_000 })
    expect(missed.length).toBeGreaterThan(1)
    for (let i = 1; i < missed.length; i++) {
      expect(missed[i - 1].expectedAt >= missed[i].expectedAt).toBe(true)
    }
  })
})

describe('missedEqual', () => {
  it('returns true for empty arrays', () => {
    expect(missedEqual([], [])).toBe(true)
  })
  it('returns false on length mismatch', () => {
    expect(
      missedEqual([], [{ scheduleId: 's', jobId: 'j', expectedAt: 'x' }])
    ).toBe(false)
  })
  it('ignores order', () => {
    const a = [
      { scheduleId: 's', jobId: 'j', expectedAt: 'x' },
      { scheduleId: 's', jobId: 'j', expectedAt: 'y' }
    ]
    const b = [
      { scheduleId: 's', jobId: 'j', expectedAt: 'y' },
      { scheduleId: 's', jobId: 'j', expectedAt: 'x' }
    ]
    expect(missedEqual(a, b)).toBe(true)
  })
})

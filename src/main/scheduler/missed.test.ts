import { describe, it, expect } from 'vitest'
import { detectMissed, missedEqual } from './missed'
import type { Agent, JobRun } from './types'

function run(jobId: string, startedAt: Date): JobRun {
  return {
    id: `r-${startedAt.getTime()}`,
    jobId,
    scheduleId: jobId,
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

function hourlyAgent(id: string, scriptPath = `/agents/${id}.sh`): Agent {
  return {
    id,
    title: id,
    description: '',
    section: 'Agents',
    scriptPath,
    schedule: { kind: 'hourly', everyHours: 1, minute: 0 },
    scheduled: true,
    orphaned: false
  }
}

describe('detectMissed', () => {
  it('returns empty when every expected tick has a matching run', () => {
    const now = new Date('2026-05-09T05:30:00Z')
    const agent = hourlyAgent('ping')
    const runs = [
      run('ping', new Date('2026-05-09T00:00:01Z')),
      run('ping', new Date('2026-05-09T01:00:00Z')),
      run('ping', new Date('2026-05-09T02:00:01Z')),
      run('ping', new Date('2026-05-09T03:00:01Z')),
      run('ping', new Date('2026-05-09T04:00:00Z')),
      run('ping', new Date('2026-05-09T05:00:00Z'))
    ]
    expect(detectMissed([agent], runs, { now, windowMs: 6 * 3600_000 })).toEqual([])
  })

  it('flags only ticks after the most recent run', () => {
    const now = new Date('2026-05-09T05:30:00Z')
    const agent = hourlyAgent('ping')
    // last run was at 03:00:01 — earlier gaps are considered acknowledged,
    // so only 04:00 and 05:00 remain missed.
    const runs = [
      run('ping', new Date('2026-05-09T00:00:01Z')),
      run('ping', new Date('2026-05-09T01:00:00Z')),
      run('ping', new Date('2026-05-09T03:00:01Z'))
    ]
    const missed = detectMissed([agent], runs, { now, windowMs: 6 * 3600_000 })
    const times = missed.map((m) => m.expectedAt).sort()
    expect(times).toEqual([
      '2026-05-09T04:00:00.000Z',
      '2026-05-09T05:00:00.000Z'
    ])
  })

  it('clears prior missed ticks when a later run exists (e.g. manual)', () => {
    const now = new Date('2026-05-09T05:30:00Z')
    const agent = hourlyAgent('ping')
    // Schedule fired at 00:00..05:00 with no on-time runs, but a manual run
    // at 05:25 covers the gap and clears the banner.
    const runs = [run('ping', new Date('2026-05-09T05:25:00Z'))]
    const missed = detectMissed([agent], runs, { now, windowMs: 6 * 3600_000 })
    expect(missed).toEqual([])
  })

  it('does not flag ticks within tolerance even if not exact', () => {
    const now = new Date('2026-05-09T05:30:00Z')
    const agent = hourlyAgent('ping')
    const runs = [
      run('ping', new Date('2026-05-09T04:00:30Z')),
      run('ping', new Date('2026-05-09T05:00:00Z'))
    ]
    const missed = detectMissed([agent], runs, {
      now,
      windowMs: 2 * 3600_000,
      toleranceMs: 90_000
    })
    expect(missed).toEqual([])
  })

  it('does not flag a tick less than tolerance ago', () => {
    const now = new Date('2026-05-09T05:00:30Z')
    const agent = hourlyAgent('ping')
    const missed = detectMissed([agent], [], {
      now,
      windowMs: 60 * 60_000,
      toleranceMs: 90_000
    })
    expect(missed.map((m) => m.expectedAt)).not.toContain('2026-05-09T05:00:00.000Z')
  })

  it('skips agents with no schedule', () => {
    const agent: Agent = { ...hourlyAgent('ping'), schedule: undefined, scheduled: false }
    const missed = detectMissed([agent], [], {
      now: new Date('2026-05-09T05:30:00Z'),
      windowMs: 3600_000
    })
    expect(missed).toEqual([])
  })

  it('skips orphan agents (no scriptPath)', () => {
    const agent: Agent = { ...hourlyAgent('gone'), scriptPath: undefined, orphaned: true }
    const missed = detectMissed([agent], [], {
      now: new Date('2026-05-09T05:30:00Z'),
      windowMs: 3600_000
    })
    expect(missed).toEqual([])
  })

  it('does not flag ticks from before scheduledAt', () => {
    const now = new Date('2026-05-09T05:30:00Z')
    const agent: Agent = {
      ...hourlyAgent('ping'),
      // schedule was created at 04:30 — only 05:00 should be considered
      scheduledAt: '2026-05-09T04:30:00Z'
    }
    const missed = detectMissed([agent], [], { now, windowMs: 6 * 3600_000 })
    const times = missed.map((m) => m.expectedAt)
    expect(times).toEqual(['2026-05-09T05:00:00.000Z'])
  })

  it('returns missed runs sorted newest first', () => {
    const now = new Date('2026-05-09T05:30:00Z')
    const agent = hourlyAgent('ping')
    const missed = detectMissed([agent], [], { now, windowMs: 4 * 3600_000 })
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
    expect(missedEqual([], [{ agentId: 'a', expectedAt: 'x' }])).toBe(false)
  })
  it('ignores order', () => {
    const a = [
      { agentId: 'p', expectedAt: 'x' },
      { agentId: 'p', expectedAt: 'y' }
    ]
    const b = [
      { agentId: 'p', expectedAt: 'y' },
      { agentId: 'p', expectedAt: 'x' }
    ]
    expect(missedEqual(a, b)).toBe(true)
  })
})

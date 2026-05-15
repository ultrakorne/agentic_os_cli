import { describe, expect, it } from 'vitest'
import { parseRefreshSummary } from './service'

describe('parseRefreshSummary', () => {
  it('parses the canonical happy-path summary line', () => {
    const s = parseRefreshSummary(
      'aos refresh agents=3 scheduled=2 issues=0 cron=wrote wrapper=ok python3=ok daemon=ok log=trimmed'
    )
    expect(s).toEqual({
      agents: 3,
      scheduled: 2,
      issues: 0,
      cron: 'wrote',
      wrapper: 'ok',
      python3: 'ok',
      daemon: 'ok',
      log: 'trimmed'
    })
  })

  it('preserves multi-token status strings like skipped:no-crontab-bin', () => {
    const s = parseRefreshSummary(
      'aos refresh agents=0 scheduled=0 issues=0 cron=skipped:no-crontab-bin wrapper=ok python3=missing daemon=unknown log=untouched'
    )
    expect(s?.cron).toBe('skipped:no-crontab-bin')
    expect(s?.python3).toBe('missing')
    expect(s?.daemon).toBe('unknown')
  })

  it('ignores leading/trailing whitespace', () => {
    const s = parseRefreshSummary(
      '   aos refresh agents=1 scheduled=1 issues=0 cron=unchanged wrapper=ok python3=ok daemon=ok log=untouched   '
    )
    expect(s?.agents).toBe(1)
    expect(s?.cron).toBe('unchanged')
  })

  it('returns null when the line does not start with the marker', () => {
    expect(parseRefreshSummary('error: something blew up')).toBeNull()
    expect(parseRefreshSummary('')).toBeNull()
  })

  it('defaults missing fields to zero/unknown rather than throwing', () => {
    const s = parseRefreshSummary('aos refresh agents=2')
    expect(s).toEqual({
      agents: 2,
      scheduled: 0,
      issues: 0,
      cron: 'unknown',
      wrapper: 'unknown',
      python3: 'unknown',
      daemon: 'unknown',
      log: 'unknown'
    })
  })
})

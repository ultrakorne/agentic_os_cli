import { describe, it, expect } from 'vitest'
import { compileToCron, previousTick } from './spec'

describe('compileToCron', () => {
  describe('hourly', () => {
    it('every 1h at minute 0 collapses to *', () => {
      expect(compileToCron({ kind: 'hourly', everyHours: 1, minute: 0 })).toBe('0 * * * *')
    })

    it('every 3h at minute 15', () => {
      expect(compileToCron({ kind: 'hourly', everyHours: 3, minute: 15 })).toBe('15 */3 * * *')
    })

    it('every 12h', () => {
      expect(compileToCron({ kind: 'hourly', everyHours: 12, minute: 0 })).toBe('0 */12 * * *')
    })

    it('rejects out-of-range everyHours', () => {
      expect(() => compileToCron({ kind: 'hourly', everyHours: 0, minute: 0 })).toThrow()
      expect(() => compileToCron({ kind: 'hourly', everyHours: 13, minute: 0 })).toThrow()
    })

    it('rejects out-of-range minute', () => {
      expect(() => compileToCron({ kind: 'hourly', everyHours: 1, minute: -1 })).toThrow()
      expect(() => compileToCron({ kind: 'hourly', everyHours: 1, minute: 60 })).toThrow()
    })
  })

  describe('daily', () => {
    it('Mon–Fri at 09:00', () => {
      expect(
        compileToCron({
          kind: 'daily',
          days: ['mon', 'tue', 'wed', 'thu', 'fri'],
          hour: 9,
          minute: 0
        })
      ).toBe('0 9 * * 1,2,3,4,5')
    })

    it('Sun only at 23:30', () => {
      expect(
        compileToCron({ kind: 'daily', days: ['sun'], hour: 23, minute: 30 })
      ).toBe('30 23 * * 0')
    })

    it('dedupes and sorts day list', () => {
      expect(
        compileToCron({
          kind: 'daily',
          days: ['fri', 'mon', 'mon', 'wed'],
          hour: 8,
          minute: 0
        })
      ).toBe('0 8 * * 1,3,5')
    })

    it('rejects empty days', () => {
      expect(() =>
        compileToCron({ kind: 'daily', days: [], hour: 8, minute: 0 })
      ).toThrow(/at least one weekday/)
    })

    it('rejects out-of-range hour and minute', () => {
      expect(() =>
        compileToCron({ kind: 'daily', days: ['mon'], hour: 24, minute: 0 })
      ).toThrow()
      expect(() =>
        compileToCron({ kind: 'daily', days: ['mon'], hour: 0, minute: 60 })
      ).toThrow()
    })
  })
})

describe('previousTick', () => {
  it('hourly: every 3h at minute 0, now 14:37 → 12:00 today', () => {
    const now = new Date('2026-05-08T14:37:00')
    const prev = previousTick({ kind: 'hourly', everyHours: 3, minute: 0 }, now)!
    expect(prev.getHours()).toBe(12)
    expect(prev.getMinutes()).toBe(0)
    expect(prev.toDateString()).toBe(now.toDateString())
  })

  it('hourly: every 1h at minute 30, now 09:15 → 08:30', () => {
    const now = new Date('2026-05-08T09:15:00')
    const prev = previousTick({ kind: 'hourly', everyHours: 1, minute: 30 }, now)!
    expect(prev.getHours()).toBe(8)
    expect(prev.getMinutes()).toBe(30)
  })

  it('hourly: at exact tick, returns that tick', () => {
    const now = new Date('2026-05-08T12:00:00')
    const prev = previousTick({ kind: 'hourly', everyHours: 3, minute: 0 }, now)!
    expect(prev.getTime()).toBe(now.getTime())
  })

  it('daily: weekday match before time-of-day → previous matching day', () => {
    // 2026-05-08 is Friday. 09:00. Looking for Mon-Fri at 09:00.
    // At 08:59 Friday, previous tick is Thursday 09:00.
    const now = new Date('2026-05-08T08:59:00')
    const prev = previousTick(
      { kind: 'daily', days: ['mon', 'tue', 'wed', 'thu', 'fri'], hour: 9, minute: 0 },
      now
    )!
    expect(prev.getDay()).toBe(4) // Thursday
    expect(prev.getHours()).toBe(9)
  })

  it('daily: weekday match after time-of-day → today', () => {
    // 2026-05-08 is Friday. 10:00. Mon-Fri 09:00 → today 09:00.
    const now = new Date('2026-05-08T10:00:00')
    const prev = previousTick(
      { kind: 'daily', days: ['mon', 'tue', 'wed', 'thu', 'fri'], hour: 9, minute: 0 },
      now
    )!
    expect(prev.toDateString()).toBe(now.toDateString())
    expect(prev.getHours()).toBe(9)
  })

  it('daily: across weekend gap', () => {
    // 2026-05-11 is Monday 08:00. Mon-Fri 09:00 → Friday 09:00.
    const now = new Date('2026-05-11T08:00:00')
    const prev = previousTick(
      { kind: 'daily', days: ['mon', 'tue', 'wed', 'thu', 'fri'], hour: 9, minute: 0 },
      now
    )!
    expect(prev.getDay()).toBe(5) // Friday
    expect(prev.toDateString()).toBe(new Date('2026-05-08').toDateString())
  })
})

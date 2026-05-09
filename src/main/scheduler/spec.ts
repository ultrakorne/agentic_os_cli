import type { ScheduleSpec, Weekday } from './types'

const WEEKDAY_TO_CRON: Record<Weekday, number> = {
  sun: 0,
  mon: 1,
  tue: 2,
  wed: 3,
  thu: 4,
  fri: 5,
  sat: 6
}

export function previousTick(spec: ScheduleSpec, now: Date = new Date()): Date | null {
  if (spec.kind === 'hourly') {
    const { everyHours, minute } = spec
    const candidate = new Date(now)
    candidate.setSeconds(0, 0)
    candidate.setMinutes(minute)
    const h = candidate.getHours()
    candidate.setHours(h - (h % everyHours))
    if (candidate.getTime() > now.getTime()) {
      candidate.setHours(candidate.getHours() - everyHours)
    }
    return candidate
  }

  const dayNums = new Set(spec.days.map((d) => WEEKDAY_TO_CRON[d]))
  for (let i = 0; i < 8; i++) {
    const d = new Date(now)
    d.setDate(d.getDate() - i)
    d.setHours(spec.hour, spec.minute, 0, 0)
    if (d.getTime() <= now.getTime() && dayNums.has(d.getDay())) {
      return d
    }
  }
  return null
}

export function compileToCron(spec: ScheduleSpec): string {
  if (spec.kind === 'hourly') {
    if (!Number.isInteger(spec.everyHours) || spec.everyHours < 1 || spec.everyHours > 12) {
      throw new Error(`hourly.everyHours must be an integer in 1..12, got ${spec.everyHours}`)
    }
    if (!Number.isInteger(spec.minute) || spec.minute < 0 || spec.minute > 59) {
      throw new Error(`hourly.minute must be an integer in 0..59, got ${spec.minute}`)
    }
    const hourField = spec.everyHours === 1 ? '*' : `*/${spec.everyHours}`
    return `${spec.minute} ${hourField} * * *`
  }

  if (spec.days.length === 0) {
    throw new Error('daily.days must include at least one weekday')
  }
  if (!Number.isInteger(spec.hour) || spec.hour < 0 || spec.hour > 23) {
    throw new Error(`daily.hour must be an integer in 0..23, got ${spec.hour}`)
  }
  if (!Number.isInteger(spec.minute) || spec.minute < 0 || spec.minute > 59) {
    throw new Error(`daily.minute must be an integer in 0..59, got ${spec.minute}`)
  }

  const days = [...new Set(spec.days)]
    .map((d) => WEEKDAY_TO_CRON[d])
    .sort((a, b) => a - b)
    .join(',')
  return `${spec.minute} ${spec.hour} * * ${days}`
}

export const DEFAULT_HOURLY: Extract<ScheduleSpec, { kind: 'hourly' }> = {
  kind: 'hourly',
  everyHours: 1,
  minute: 0
}

export const DEFAULT_DAILY: Extract<ScheduleSpec, { kind: 'daily' }> = {
  kind: 'daily',
  days: ['mon', 'tue', 'wed', 'thu', 'fri'],
  hour: 9,
  minute: 0
}

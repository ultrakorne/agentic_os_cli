import type { ScheduleSpec, Weekday } from '@shared/scheduler'

const DAY_LABELS: Record<Weekday, string> = {
  mon: 'Mon',
  tue: 'Tue',
  wed: 'Wed',
  thu: 'Thu',
  fri: 'Fri',
  sat: 'Sat',
  sun: 'Sun'
}

const DAY_ORDER: Weekday[] = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun']

const WEEKDAYS: Weekday[] = ['mon', 'tue', 'wed', 'thu', 'fri']
const WEEKEND: Weekday[] = ['sat', 'sun']

function eq<T>(a: T[], b: T[]): boolean {
  if (a.length !== b.length) return false
  const set = new Set(a)
  return b.every((x) => set.has(x))
}

export function describeSpec(spec: ScheduleSpec): string {
  if (spec.kind === 'hourly') {
    const minute = spec.minute.toString().padStart(2, '0')
    if (spec.everyHours === 1) return `every hour @ :${minute}`
    return `every ${spec.everyHours}h @ :${minute}`
  }
  const time = `${spec.hour.toString().padStart(2, '0')}:${spec.minute.toString().padStart(2, '0')}`
  const ordered = DAY_ORDER.filter((d) => spec.days.includes(d))
  if (ordered.length === 7) return `daily @ ${time}`
  if (eq(spec.days, WEEKDAYS)) return `weekdays @ ${time}`
  if (eq(spec.days, WEEKEND)) return `weekends @ ${time}`
  return `${ordered.map((d) => DAY_LABELS[d].slice(0, 3)).join(' ')} @ ${time}`
}

export function describeSchedule(spec: ScheduleSpec | undefined): string {
  if (!spec) return 'unscheduled'
  return describeSpec(spec)
}

export function relativeFromNow(iso: string | null, now = new Date()): string {
  if (!iso) return '—'
  const target = new Date(iso).getTime()
  const diff = target - now.getTime()
  const abs = Math.abs(diff)

  const minute = 60_000
  const hour = 60 * minute
  const day = 24 * hour

  let text: string
  if (abs < minute) text = '<1m'
  else if (abs < hour) text = `${Math.round(abs / minute)}m`
  else if (abs < day) {
    const h = Math.floor(abs / hour)
    const m = Math.round((abs % hour) / minute)
    text = m ? `${h}h${m}m` : `${h}h`
  } else {
    const d = Math.floor(abs / day)
    const h = Math.round((abs % day) / hour)
    text = h ? `${d}d${h}h` : `${d}d`
  }
  return diff >= 0 ? `in ${text}` : `${text} ago`
}

export function formatClock(iso: string | null): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString(undefined, {
    weekday: 'short',
    hour: '2-digit',
    minute: '2-digit'
  })
}

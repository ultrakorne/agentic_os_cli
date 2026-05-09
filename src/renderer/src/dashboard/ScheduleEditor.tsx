import { useEffect, useMemo, useState, type CSSProperties, type JSX } from 'react'
import type { Agent, Schedule, ScheduleSpec, Weekday } from '@shared/scheduler'
import { formatClock, relativeFromNow } from '../lib/format'

type Mode = 'hourly' | 'daily'

const DAYS: { key: Weekday; label: string }[] = [
  { key: 'mon', label: 'M' },
  { key: 'tue', label: 'T' },
  { key: 'wed', label: 'W' },
  { key: 'thu', label: 'T' },
  { key: 'fri', label: 'F' },
  { key: 'sat', label: 'S' },
  { key: 'sun', label: 'S' }
]

type Props = {
  agent: Agent
  current: Schedule | undefined
  onClose: () => void
}

function initial(spec: ScheduleSpec | undefined): {
  mode: Mode
  hourly: { everyHours: number; minute: number }
  daily: { days: Weekday[]; hour: number; minute: number }
} {
  if (spec?.kind === 'daily') {
    return {
      mode: 'daily',
      hourly: { everyHours: 1, minute: 0 },
      daily: { days: spec.days, hour: spec.hour, minute: spec.minute }
    }
  }
  if (spec?.kind === 'hourly') {
    return {
      mode: 'hourly',
      hourly: { everyHours: spec.everyHours, minute: spec.minute },
      daily: { days: ['mon', 'tue', 'wed', 'thu', 'fri'], hour: 9, minute: 0 }
    }
  }
  return {
    mode: 'daily',
    hourly: { everyHours: 1, minute: 0 },
    daily: { days: ['mon', 'tue', 'wed', 'thu', 'fri'], hour: 9, minute: 0 }
  }
}

const frameStyle: CSSProperties = {
  ['--glow' as never]: 'var(--color-hot)'
}

export function ScheduleEditor({ agent, current, onClose }: Props): JSX.Element {
  const seed = useMemo(() => initial(current?.spec), [current])
  const [mode, setMode] = useState<Mode>(seed.mode)
  const [hourly, setHourly] = useState(seed.hourly)
  const [daily, setDaily] = useState(seed.daily)
  const [busy, setBusy] = useState(false)
  const [nextIso, setNextIso] = useState<string | null>(null)

  const spec: ScheduleSpec | null = useMemo(() => {
    if (mode === 'hourly') {
      return { kind: 'hourly', everyHours: hourly.everyHours, minute: hourly.minute }
    }
    if (daily.days.length === 0) return null
    return { kind: 'daily', days: daily.days, hour: daily.hour, minute: daily.minute }
  }, [mode, hourly, daily])

  useEffect(() => {
    let cancelled = false
    if (!spec) {
      setNextIso(null)
      return
    }
    void window.api.scheduler.nextRun(spec).then((iso) => {
      if (!cancelled) setNextIso(iso)
    })
    return () => {
      cancelled = true
    }
  }, [spec])

  const toggleDay = (day: Weekday): void => {
    setDaily((d) =>
      d.days.includes(day)
        ? { ...d, days: d.days.filter((x) => x !== day) }
        : { ...d, days: [...d.days, day] }
    )
  }

  const save = async (): Promise<void> => {
    if (!spec) return
    setBusy(true)
    try {
      await window.api.scheduler.upsert({ id: agent.id, jobId: agent.id, spec })
      onClose()
    } finally {
      setBusy(false)
    }
  }

  const clear = async (): Promise<void> => {
    setBusy(true)
    try {
      await window.api.scheduler.remove(agent.id)
      onClose()
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      className="bg-card-2 neon-edge-strong relative p-5 text-xs"
      style={frameStyle}
    >
      <CornerBrackets />

      {/* header */}
      <div className="flex items-baseline gap-3 border-b border-[var(--color-rule)] pb-3">
        <span
          className="font-display text-[15px] font-bold uppercase text-[var(--color-hot)] neon-text"
          style={{ letterSpacing: '0.28em' }}
        >
          schedule
        </span>
        <span className="text-[var(--color-rule-bright)]">/</span>
        <span
          className="font-display text-[13px] font-bold uppercase text-[var(--color-fg)]"
          style={{ letterSpacing: '0.18em' }}
        >
          {agent.id}
        </span>
        <span
          className="ml-auto font-display text-[10px] uppercase text-[var(--color-fg-faint)]"
          style={{ letterSpacing: '0.24em' }}
        >
          esc to close
        </span>
      </div>

      {/* body */}
      <div className="mt-4 grid grid-cols-1 gap-5 md:grid-cols-[auto_1fr_auto] md:items-start">
        <ModeToggle mode={mode} onChange={setMode} />

        <div className="min-w-0">
          {mode === 'hourly' ? (
            <HourlyControls
              everyHours={hourly.everyHours}
              minute={hourly.minute}
              onEveryHours={(everyHours) => setHourly((h) => ({ ...h, everyHours }))}
              onMinute={(minute) => setHourly((h) => ({ ...h, minute }))}
            />
          ) : (
            <DailyControls
              days={daily.days}
              hour={daily.hour}
              minute={daily.minute}
              onToggleDay={toggleDay}
              onTime={(hour, minute) => setDaily((d) => ({ ...d, hour, minute }))}
            />
          )}
        </div>

        <div className="flex flex-col items-start gap-1 md:items-end">
          <span
            className="font-display text-[10px] uppercase text-[var(--color-fg-faint)]"
            style={{ letterSpacing: '0.24em' }}
          >
            next run
          </span>
          <span className="font-display text-[15px] font-bold tabular text-[var(--color-cool)] neon-text">
            {spec ? (nextIso ? formatClock(nextIso) : '…') : 'pick a day'}
          </span>
          {spec && nextIso && (
            <span className="tabular text-[var(--color-fg-dim)]">
              {relativeFromNow(nextIso)}
            </span>
          )}
        </div>
      </div>

      {/* actions */}
      <div className="mt-5 flex items-center gap-3 border-t border-[var(--color-rule)] pt-4">
        <ArcadeButton tone="hot" onClick={save} disabled={busy || !spec}>
          ▶ save
        </ArcadeButton>

        <ArcadeButton tone="ghost" onClick={onClose} disabled={busy}>
          cancel
        </ArcadeButton>

        {current && (
          <span className="ml-auto">
            <ArcadeButton tone="danger" onClick={clear} disabled={busy}>
              clear
            </ArcadeButton>
          </span>
        )}
      </div>
    </div>
  )
}

function CornerBrackets(): JSX.Element {
  const base = 'pointer-events-none absolute size-3 border-[var(--glow)]'
  return (
    <>
      <span className={`${base} left-0 top-0 border-l border-t`} />
      <span className={`${base} right-0 top-0 border-r border-t`} />
      <span className={`${base} bottom-0 left-0 border-b border-l`} />
      <span className={`${base} bottom-0 right-0 border-b border-r`} />
    </>
  )
}

function ArcadeButton({
  tone,
  onClick,
  disabled,
  children
}: {
  tone: 'hot' | 'ghost' | 'danger'
  onClick: () => void
  disabled?: boolean
  children: React.ReactNode
}): JSX.Element {
  const palette: Record<typeof tone, string> = {
    hot:
      'border-[var(--color-hot)] text-[var(--color-hot)] hover:bg-[var(--color-hot)] hover:text-[var(--color-bg)] hover:shadow-[0_0_20px_-2px_var(--color-hot)]',
    ghost:
      'border-[var(--color-rule-bright)] text-[var(--color-fg-dim)] hover:border-[var(--color-cool)] hover:text-[var(--color-cool)]',
    danger:
      'border-[var(--color-rule-bright)] text-[var(--color-fg-dim)] hover:border-[var(--color-danger)] hover:text-[var(--color-danger)] hover:shadow-[0_0_18px_-4px_var(--color-danger)]'
  }
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={`inline-flex items-center gap-1 border px-3 py-1.5 font-display text-[10px] font-bold uppercase transition-all active:translate-y-px disabled:opacity-40 disabled:active:translate-y-0 ${palette[tone]}`}
      style={{ letterSpacing: '0.24em' }}
    >
      {children}
    </button>
  )
}

function ModeToggle({
  mode,
  onChange
}: {
  mode: Mode
  onChange: (m: Mode) => void
}): JSX.Element {
  return (
    <div className="inline-flex">
      {(['hourly', 'daily'] as const).map((m, i) => {
        const active = mode === m
        return (
          <button
            key={m}
            type="button"
            onClick={() => onChange(m)}
            className={`border px-3 py-1.5 font-display text-[10px] font-bold uppercase transition-all ${
              i === 0 ? '' : '-ml-px'
            } ${
              active
                ? 'border-[var(--color-hot)] bg-[var(--color-hot)] text-[var(--color-bg)] shadow-[0_0_18px_-4px_var(--color-hot)]'
                : 'border-[var(--color-rule-bright)] text-[var(--color-fg-dim)] hover:border-[var(--color-cool)] hover:text-[var(--color-cool)]'
            }`}
            style={{ letterSpacing: '0.24em' }}
          >
            {m}
          </button>
        )
      })}
    </div>
  )
}

function HourlyControls({
  everyHours,
  minute,
  onEveryHours,
  onMinute
}: {
  everyHours: number
  minute: number
  onEveryHours: (n: number) => void
  onMinute: (n: number) => void
}): JSX.Element {
  return (
    <div className="flex flex-wrap items-baseline gap-x-4 gap-y-3 text-xs">
      <Label>every</Label>
      <Stepper value={everyHours} min={1} max={12} onChange={onEveryHours} />
      <span className="text-[var(--color-fg-dim)]">{everyHours === 1 ? 'hour' : 'hours'}</span>
      <Label>at minute</Label>
      <Stepper value={minute} min={0} max={59} step={5} onChange={onMinute} pad />
    </div>
  )
}

function DailyControls({
  days,
  hour,
  minute,
  onToggleDay,
  onTime
}: {
  days: Weekday[]
  hour: number
  minute: number
  onToggleDay: (d: Weekday) => void
  onTime: (h: number, m: number) => void
}): JSX.Element {
  return (
    <div className="flex flex-wrap items-baseline gap-x-4 gap-y-3 text-xs">
      <div className="inline-flex gap-1">
        {DAYS.map((d) => {
          const active = days.includes(d.key)
          return (
            <button
              key={d.key}
              type="button"
              onClick={() => onToggleDay(d.key)}
              aria-pressed={active}
              className={`size-7 border font-display text-[11px] font-bold uppercase transition-all ${
                active
                  ? 'border-[var(--color-hot)] bg-[var(--color-hot)] text-[var(--color-bg)] shadow-[0_0_14px_-3px_var(--color-hot)]'
                  : 'border-[var(--color-rule-bright)] text-[var(--color-fg-dim)] hover:border-[var(--color-cool)] hover:text-[var(--color-cool)]'
              }`}
            >
              {d.label}
            </button>
          )
        })}
      </div>
      <Label>at</Label>
      <Stepper value={hour} min={0} max={23} onChange={(h) => onTime(h, minute)} pad />
      <span className="text-[var(--color-fg-faint)]">:</span>
      <Stepper value={minute} min={0} max={59} step={5} onChange={(m) => onTime(hour, m)} pad />
    </div>
  )
}

function Label({ children }: { children: React.ReactNode }): JSX.Element {
  return (
    <span
      className="font-display text-[10px] uppercase text-[var(--color-fg-faint)]"
      style={{ letterSpacing: '0.24em' }}
    >
      {children}
    </span>
  )
}

function Stepper({
  value,
  min,
  max,
  step = 1,
  onChange,
  pad = false
}: {
  value: number
  min: number
  max: number
  step?: number
  onChange: (n: number) => void
  pad?: boolean
}): JSX.Element {
  const display = pad ? value.toString().padStart(2, '0') : value.toString()
  return (
    <span className="inline-flex items-center border border-[var(--color-rule-bright)] tabular">
      <button
        type="button"
        onClick={() => onChange(Math.max(min, value - step))}
        className="px-2 py-0.5 text-[var(--color-fg-dim)] transition-colors hover:bg-[var(--color-surface-2)] hover:text-[var(--color-cool)]"
        aria-label="decrease"
      >
        −
      </button>
      <span className="min-w-[2.5ch] border-x border-[var(--color-rule-bright)] px-2 py-0.5 text-center font-display text-[13px] font-bold text-[var(--color-cool)] neon-text-soft">
        {display}
      </span>
      <button
        type="button"
        onClick={() => onChange(Math.min(max, value + step))}
        className="px-2 py-0.5 text-[var(--color-fg-dim)] transition-colors hover:bg-[var(--color-surface-2)] hover:text-[var(--color-cool)]"
        aria-label="increase"
      >
        +
      </button>
    </span>
  )
}

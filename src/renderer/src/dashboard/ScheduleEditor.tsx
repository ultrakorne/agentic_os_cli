import { useEffect, useMemo, useRef, useState, type JSX } from 'react'
import type { Agent, ScheduleSpec, Weekday } from '@shared/scheduler'
import { CornerBrackets } from './CornerBrackets'
import { glowFrame } from './styles'

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
  embedded?: boolean
}

const AUTOSAVE_DEBOUNCE_MS = 1000

function initial(spec: ScheduleSpec | undefined): {
  mode: Mode | null
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
    mode: null,
    hourly: { everyHours: 1, minute: 0 },
    daily: { days: ['mon', 'tue', 'wed', 'thu', 'fri'], hour: 9, minute: 0 }
  }
}

export function ScheduleEditor({ agent, embedded = false }: Props): JSX.Element {
  const seed = useMemo(() => initial(agent.schedule), [agent.schedule])
  const [mode, setModeRaw] = useState<Mode | null>(seed.mode)
  const [hourly, setHourlyRaw] = useState(seed.hourly)
  const [daily, setDailyRaw] = useState(seed.daily)
  const editTokenRef = useRef(0)
  const lastSavedTokenRef = useRef(0)

  const setMode: typeof setModeRaw = (m) => {
    editTokenRef.current += 1
    setModeRaw(m)
  }
  const setHourly: typeof setHourlyRaw = (u) => {
    editTokenRef.current += 1
    setHourlyRaw(u)
  }
  const setDaily: typeof setDailyRaw = (u) => {
    editTokenRef.current += 1
    setDailyRaw(u)
  }

  const spec: ScheduleSpec | null = useMemo(() => {
    if (mode === null) return null
    if (mode === 'hourly') {
      return { kind: 'hourly', everyHours: hourly.everyHours, minute: hourly.minute }
    }
    if (daily.days.length === 0) return null
    return { kind: 'daily', days: daily.days, hour: daily.hour, minute: daily.minute }
  }, [mode, hourly, daily])

  const pendingFlushRef = useRef<{ spec: ScheduleSpec | null } | null>(null)

  // Auto-save on edit. Skips renders that weren't triggered by user input
  // (e.g. when the panel re-renders due to an external schedule change).
  useEffect(() => {
    if (editTokenRef.current === lastSavedTokenRef.current) {
      pendingFlushRef.current = null
      return
    }
    const myToken = editTokenRef.current
    pendingFlushRef.current = { spec }
    const t = setTimeout(() => {
      pendingFlushRef.current = null
      void window.api.agents.setSchedule(agent.id, spec).then(() => {
        lastSavedTokenRef.current = myToken
      })
    }, AUTOSAVE_DEBOUNCE_MS)
    return () => clearTimeout(t)
  }, [spec, agent.id])

  // Flush a pending save if the user closes the panel mid-edit.
  useEffect(() => {
    const id = agent.id
    return () => {
      const pending = pendingFlushRef.current
      if (pending) {
        void window.api.agents.setSchedule(id, pending.spec)
        pendingFlushRef.current = null
      }
    }
  }, [agent.id])

  const toggleDay = (day: Weekday): void => {
    setDaily((d) =>
      d.days.includes(day)
        ? { ...d, days: d.days.filter((x) => x !== day) }
        : { ...d, days: [...d.days, day] }
    )
  }

  const inner = (
    <>
      {!embedded && (
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
      )}

      {mode === null ? (
        <div className={`${embedded ? '' : 'mt-4'} flex flex-col gap-3`}>
          <ModeToggle mode={mode} onChange={setMode} />
          <p
            className="font-display text-[10px] uppercase text-[var(--color-fg-faint)]"
            style={{ letterSpacing: '0.22em' }}
          >
            {'// click hourly or daily to enable a schedule'}
          </p>
        </div>
      ) : (
        <div
          className={`${embedded ? '' : 'mt-4'} grid grid-cols-1 gap-5 md:grid-cols-[auto_1fr] md:items-start`}
        >
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
        </div>
      )}
    </>
  )

  if (embedded) {
    return <div className="text-xs">{inner}</div>
  }

  return (
    <div
      className="bg-card-2 neon-edge-strong relative p-5 text-xs"
      style={glowFrame('var(--color-hot)')}
    >
      <CornerBrackets />
      {inner}
    </div>
  )
}

function ModeToggle({
  mode,
  onChange
}: {
  mode: Mode | null
  onChange: (m: Mode | null) => void
}): JSX.Element {
  return (
    <div className="inline-flex">
      {(['hourly', 'daily'] as const).map((m, i) => {
        const active = mode === m
        return (
          <button
            key={m}
            type="button"
            onClick={() => onChange(active ? null : m)}
            aria-pressed={active}
            title={active ? `click to unschedule` : `set ${m} schedule`}
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
  const inc = (): void => {
    const next = value + step
    onChange(next > max ? min : next)
  }
  const dec = (): void => {
    const next = value - step
    if (next >= min) {
      onChange(next)
      return
    }
    const stepsInRange = Math.floor((max - min) / step)
    onChange(min + stepsInRange * step)
  }

  const display = pad ? value.toString().padStart(2, '0') : value.toString()
  const [draft, setDraft] = useState<string | null>(null)

  const commit = (raw: string): void => {
    setDraft(null)
    const trimmed = raw.trim()
    if (trimmed === '') return
    const n = parseInt(trimmed, 10)
    if (Number.isNaN(n)) return
    onChange(Math.min(max, Math.max(min, n)))
  }

  return (
    <span className="inline-flex items-center border border-[var(--color-rule-bright)] tabular">
      <button
        type="button"
        onClick={dec}
        tabIndex={-1}
        className="px-2 py-0.5 text-[var(--color-fg-dim)] transition-colors hover:bg-[var(--color-surface-2)] hover:text-[var(--color-cool)]"
        aria-label="decrease"
      >
        −
      </button>
      <input
        type="text"
        inputMode="numeric"
        size={2}
        value={draft ?? display}
        onFocus={(e) => {
          e.currentTarget.select()
        }}
        onChange={(e) => {
          const raw = e.target.value.replace(/[^\d]/g, '').slice(0, 2)
          setDraft(raw)
          if (raw === '') return
          const n = parseInt(raw, 10)
          if (!Number.isNaN(n) && n >= min && n <= max) onChange(n)
        }}
        onBlur={(e) => commit(e.currentTarget.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.currentTarget.blur()
          } else if (e.key === 'ArrowUp') {
            e.preventDefault()
            setDraft(null)
            inc()
          } else if (e.key === 'ArrowDown') {
            e.preventDefault()
            setDraft(null)
            dec()
          } else if (e.key === 'Escape') {
            setDraft(null)
            e.currentTarget.blur()
          }
        }}
        aria-label="value"
        className="min-w-[2.5ch] border-x border-[var(--color-rule-bright)] bg-transparent px-2 py-0.5 text-center font-display text-[13px] font-bold text-[var(--color-cool)] neon-text-soft outline-none focus:bg-[var(--color-surface-2)]"
      />
      <button
        type="button"
        onClick={inc}
        tabIndex={-1}
        className="px-2 py-0.5 text-[var(--color-fg-dim)] transition-colors hover:bg-[var(--color-surface-2)] hover:text-[var(--color-cool)]"
        aria-label="increase"
      >
        +
      </button>
    </span>
  )
}

import { useEffect, useState, type JSX } from 'react'
import type { Agent, JobRun } from '@shared/scheduler'
import { describeSchedule, relativeFromNow } from '../lib/format'
import { CornerBrackets } from './CornerBrackets'
import { glowFrame } from './styles'

type Props = {
  agent: Agent
  recentRun: JobRun | undefined
  missedCount: number
  selected: boolean
  onSelect: () => void
}

type Status = JobRun['status'] | undefined

export function AgentCard({
  agent,
  recentRun,
  missedCount,
  selected,
  onSelect
}: Props): JSX.Element {
  const schedule = agent.schedule
  const [pendingSince, setPendingSince] = useState<number | null>(null)
  const [nextRunIso, setNextRunIso] = useState<string | null>(null)
  const [launchError, setLaunchError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    if (!schedule) {
      setNextRunIso(null)
      return
    }
    void window.api.scheduler.nextRun(schedule).then((iso) => {
      if (!cancelled) setNextRunIso(iso)
    })
    return () => {
      cancelled = true
    }
  }, [schedule])

  useEffect(() => {
    if (!launchError) return
    const t = setTimeout(() => setLaunchError(null), 5000)
    return () => clearTimeout(t)
  }, [launchError])

  // clear pending once a newer run shows up in the store, with a failsafe so
  // we don't get stuck on if the run JSON never arrives.
  useEffect(() => {
    if (pendingSince === null) return
    if (recentRun && new Date(recentRun.startedAt).getTime() >= pendingSince - 1000) {
      setPendingSince(null)
    }
  }, [recentRun, pendingSince])
  useEffect(() => {
    if (pendingSince === null) return
    const t = setTimeout(() => setPendingSince(null), 10000)
    return () => clearTimeout(t)
  }, [pendingSince])

  const handleRun = async (e: React.MouseEvent | React.KeyboardEvent): Promise<void> => {
    e.stopPropagation()
    setPendingSince(Date.now())
    setLaunchError(null)
    try {
      const res = await window.api.scheduler.runNow(agent.id)
      if (res.status === 'error') {
        setLaunchError(res.error ?? 'run failed to launch')
        setPendingSince(null)
      }
    } catch (err) {
      setLaunchError(err instanceof Error ? err.message : 'run failed to launch')
      setPendingSince(null)
    }
  }

  const pending = pendingSince !== null
  const status: Status = recentRun?.status
  const live = status === 'running' || pending
  const glow = pickGlow(status, !!schedule, selected, live)

  return (
    <button
      type="button"
      onClick={onSelect}
      style={glowFrame(`var(${glow})`)}
      data-selected={selected}
      className="bg-card card-frame group relative flex h-full flex-col gap-3 p-4 text-left transition-transform duration-150 hover:-translate-y-0.5"
    >
      {/* corner brackets — arcade frame */}
      <CornerBrackets subtle />

      {/* live scanline only when running */}
      {live && <span className="scanline-host pointer-events-none absolute inset-0" />}

      {/* header: status + id + run */}
      <div className="relative flex items-start gap-2.5">
        <StatusGlyph
          status={status}
          scheduled={!!schedule}
          live={live}
          missed={missedCount > 0}
        />
        <span className="min-w-0 flex-1">
          <span
            className={`font-display block truncate text-[13px] font-bold uppercase ${
              selected
                ? 'self-start bg-[var(--color-hot)] px-1.5 text-[var(--color-bg)]'
                : 'text-[var(--color-fg)]'
            }`}
            style={{ letterSpacing: '0.14em' }}
          >
            {agent.id}
          </span>
          <span className="mt-0.5 block text-[10px] text-[var(--color-fg-faint)] tabular">
            {recentRun ? relativeFromNow(recentRun.startedAt) : 'never run'}
          </span>
        </span>
        <RunButton onClick={handleRun} running={pending} />
      </div>

      {/* description */}
      <p
        aria-hidden={!agent.description}
        className="relative truncate min-h-[20px] text-[12px] leading-relaxed text-[var(--color-fg-dim)]"
      >
        {agent.description ?? ''}
      </p>

      {launchError && (
        <p
          role="alert"
          className="relative truncate text-[10px] uppercase text-[var(--color-danger)] neon-text-soft"
          style={{ letterSpacing: '0.16em' }}
          title={launchError}
        >
          ▲ {launchError}
        </p>
      )}

      {/* footer: schedule + next */}
      <div
        className="relative mt-auto flex items-center gap-2 border-t border-[var(--color-rule)] pt-2 text-[10px] uppercase tabular"
        style={{ letterSpacing: '0.16em' }}
      >
        <span
          className={
            schedule
              ? 'text-[var(--color-cool)] neon-text-soft'
              : 'text-[var(--color-fg-faint)]'
          }
        >
          {describeSchedule(schedule)}
        </span>
        <span className="ml-auto text-[var(--color-fg-dim)]">
          {nextRunIso ? (
            <>
              <span className="text-[var(--color-fg-faint)]">▸</span>{' '}
              <span>{relativeFromNow(nextRunIso)}</span>
            </>
          ) : (
            <span className="text-[var(--color-fg-faint)]">◇ idle</span>
          )}
        </span>
      </div>
    </button>
  )
}

function pickGlow(
  status: Status,
  scheduled: boolean,
  selected: boolean,
  live: boolean
): string {
  if (selected) return '--color-hot'
  if (live) return '--color-cool'
  if (status === 'error') return '--color-danger'
  if (status === 'success') return '--color-success'
  if (scheduled) return '--color-cool'
  return '--color-rule-bright'
}

function RunButton({
  onClick,
  running
}: {
  onClick: (e: React.MouseEvent | React.KeyboardEvent) => void
  running: boolean
}): JSX.Element {
  return (
    <span
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') onClick(e)
      }}
      className={`relative inline-flex w-[68px] shrink-0 items-center justify-center gap-1 border px-2 py-1 font-display text-[10px] font-bold uppercase transition-colors ${
        running
          ? 'border-[var(--color-fg-faint)] text-[var(--color-fg-faint)]'
          : 'border-[var(--color-hot)] text-[var(--color-hot)] hover:bg-[var(--color-hot)] hover:text-[var(--color-bg)] hover:shadow-[0_0_18px_-2px_var(--color-hot)] active:translate-y-px'
      }`}
      style={{ letterSpacing: '0.22em' }}
      aria-label={running ? 'running' : 'run now'}
    >
      <span aria-hidden>{running ? '◌' : '▶'}</span>
      <span>{running ? 'wait' : 'run'}</span>
    </span>
  )
}

function StatusGlyph({
  status,
  scheduled,
  live,
  missed
}: {
  status: Status
  scheduled: boolean
  live: boolean
  missed: boolean
}): JSX.Element {
  if (live) {
    return (
      <span
        aria-label="running"
        className="pulse-soft mt-0.5 inline-flex size-4 items-center justify-center text-base text-[var(--color-cool)] neon-text"
      >
        ●
      </span>
    )
  }
  if (missed) {
    return (
      <span
        aria-label="missed scheduled run"
        title="missed scheduled run"
        className="mt-0.5 inline-flex size-4 items-center justify-center text-base text-[var(--color-hot)] neon-text-soft"
      >
        ▽
      </span>
    )
  }
  if (status === 'error') {
    return (
      <span
        aria-label="last run failed"
        className="mt-0.5 inline-flex size-4 items-center justify-center text-base text-[var(--color-danger)] neon-text-soft"
      >
        ▲
      </span>
    )
  }
  if (status === 'success') {
    return (
      <span
        aria-label="last run succeeded"
        className="mt-0.5 inline-flex size-4 items-center justify-center text-base text-[var(--color-success)] neon-text-soft"
      >
        ◆
      </span>
    )
  }
  return (
    <span
      aria-label={scheduled ? 'scheduled' : 'idle'}
      className={`mt-0.5 inline-flex size-4 items-center justify-center text-base ${
        scheduled
          ? 'text-[var(--color-cool)] neon-text-soft'
          : 'text-[var(--color-fg-faint)]'
      }`}
    >
      {scheduled ? '◇' : '·'}
    </span>
  )
}

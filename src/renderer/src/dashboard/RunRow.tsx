import type { JSX } from 'react'
import type { JobRun } from '@shared/scheduler'
import { formatClock, relativeFromNow } from '../lib/format'

const MAX_INLINE_LINES = 40
const MAX_INLINE_CHARS = 4000

type Props = {
  run: JobRun
  expanded: boolean
  output: string | null
  loading: boolean
  onToggle: () => void
  onViewFull: () => void
}

export function RunRow({
  run,
  expanded,
  output,
  loading,
  onToggle,
  onViewFull
}: Props): JSX.Element {
  const live = run.status === 'running'
  const duration = formatDuration(run)

  return (
    <div className="border border-[var(--color-rule)] bg-[var(--color-surface)]">
      <button
        type="button"
        onClick={onToggle}
        className="grid w-full grid-cols-[auto_1fr_auto_auto_auto_auto] items-center gap-3 px-3 py-2 text-left transition-colors hover:bg-[var(--color-surface-2)]"
      >
        <StatusGlyph status={run.status} />
        <span className="flex min-w-0 flex-col">
          <span className="font-display text-[11px] font-bold uppercase text-[var(--color-fg)] tabular" style={{ letterSpacing: '0.14em' }}>
            {formatClock(run.startedAt)}
          </span>
          <span className="text-[10px] text-[var(--color-fg-dim)] tabular">
            {relativeFromNow(run.startedAt)}
          </span>
        </span>
        <TriggerBadge trigger={run.trigger} />
        <span
          className="font-display text-[10px] uppercase text-[var(--color-fg-dim)] tabular min-w-[3.5em] text-right"
          style={{ letterSpacing: '0.18em' }}
          title="duration"
        >
          {duration}
        </span>
        <span
          className="font-display text-[10px] uppercase tabular min-w-[2em] text-right"
          style={{
            letterSpacing: '0.18em',
            color:
              run.exitCode === null
                ? 'var(--color-fg-faint)'
                : run.exitCode === 0
                  ? 'var(--color-success)'
                  : 'var(--color-danger)'
          }}
          title="exit code"
        >
          {run.exitCode === null ? '—' : run.exitCode}
        </span>
        <span
          aria-hidden
          className={`text-[var(--color-fg-faint)] transition-transform ${expanded ? 'rotate-90' : ''}`}
        >
          ▸
        </span>
      </button>

      {expanded && (
        <div className="border-t border-[var(--color-rule)] bg-[var(--color-surface-2)] px-3 py-3">
          {run.error && (
            <p
              className="mb-2 text-[11px] uppercase text-[var(--color-danger)] neon-text-soft"
              style={{ letterSpacing: '0.16em' }}
            >
              ▲ {run.error}
            </p>
          )}
          {live ? (
            <pre className="whitespace-pre-wrap break-words font-mono text-[11px] leading-relaxed text-[var(--color-fg-dim)]">
              {run.output || '// no output yet'}
              <span className="ml-1 inline-block animate-pulse text-[var(--color-cool)]">●</span>
            </pre>
          ) : loading ? (
            <p
              className="font-display text-[10px] uppercase text-[var(--color-fg-faint)]"
              style={{ letterSpacing: '0.24em' }}
            >
              loading…
            </p>
          ) : output ? (
            <TruncatedOutput output={output} onViewFull={onViewFull} />
          ) : (
            <p
              className="font-display text-[10px] uppercase text-[var(--color-fg-faint)]"
              style={{ letterSpacing: '0.24em' }}
            >
              {'// no output recorded'}
            </p>
          )}
        </div>
      )}
    </div>
  )
}

function TruncatedOutput({
  output,
  onViewFull
}: {
  output: string
  onViewFull: () => void
}): JSX.Element {
  const lines = output.split('\n')
  const overflows =
    lines.length > MAX_INLINE_LINES || output.length > MAX_INLINE_CHARS
  const display = overflows
    ? lines.slice(0, MAX_INLINE_LINES).join('\n').slice(0, MAX_INLINE_CHARS)
    : output

  return (
    <div className="flex flex-col gap-2">
      <pre className="whitespace-pre-wrap break-words font-mono text-[11px] leading-relaxed text-[var(--color-fg-dim)]">
        {display}
        {overflows && <span className="text-[var(--color-fg-faint)]">{'\n…'}</span>}
      </pre>
      {overflows && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            onViewFull()
          }}
          className="self-start border border-[var(--color-rule-bright)] px-2.5 py-1 font-display text-[10px] font-bold uppercase text-[var(--color-fg-dim)] transition-all hover:border-[var(--color-cool)] hover:text-[var(--color-cool)] hover:shadow-[0_0_18px_-4px_var(--color-cool)]"
          style={{ letterSpacing: '0.24em' }}
        >
          view full ▸
        </button>
      )}
    </div>
  )
}

function formatDuration(run: JobRun): string {
  if (!run.endedAt) return '—'
  const ms = new Date(run.endedAt).getTime() - new Date(run.startedAt).getTime()
  if (ms < 0 || !Number.isFinite(ms)) return '—'
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  const m = Math.floor(ms / 60_000)
  const s = Math.round((ms % 60_000) / 1000)
  return s ? `${m}m${s}s` : `${m}m`
}

function StatusGlyph({ status }: { status: JobRun['status'] }): JSX.Element {
  if (status === 'running') {
    return (
      <span
        aria-label="running"
        className="inline-flex size-4 items-center justify-center text-base text-[var(--color-cool)] neon-text animate-pulse"
      >
        ●
      </span>
    )
  }
  if (status === 'error') {
    return (
      <span
        aria-label="error"
        className="inline-flex size-4 items-center justify-center text-base text-[var(--color-danger)] neon-text-soft"
      >
        ▲
      </span>
    )
  }
  return (
    <span
      aria-label="success"
      className="inline-flex size-4 items-center justify-center text-base text-[var(--color-success)] neon-text-soft"
    >
      ◆
    </span>
  )
}

function TriggerBadge({ trigger }: { trigger: JobRun['trigger'] }): JSX.Element {
  const label = trigger
  const color =
    trigger === 'manual'
      ? 'var(--color-hot)'
      : trigger === 'catch-up'
        ? 'var(--color-warn)'
        : 'var(--color-cool)'
  return (
    <span
      className="inline-block border px-1.5 py-0.5 font-display text-[9px] uppercase"
      style={{ letterSpacing: '0.22em', color, borderColor: color }}
    >
      {label}
    </span>
  )
}
